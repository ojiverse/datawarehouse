import { DurableObject } from "cloudflare:workers";
import { BudgetDurableObjectClient } from "../adapters/budget-client";
import { FetchDiscordMessagesClient } from "../adapters/discord-client";
import { R2ObservationArchive } from "../adapters/r2-archive";
import { generateUuidV7, type Snowflake, type UuidV7 } from "../domain/ids";
import type { RouteBucketState } from "../domain/rate-limit";
import {
  isActiveState,
  type PendingCoordination,
  type RunRecord,
  type RunState,
  type StartRunRequest,
  type TerminalError,
  transientBackoffMs,
} from "../domain/run";
import { executePage, type PageOutcome, type StepConfig } from "../domain/step";
import { budgetObjectName, type Env, readBoolean, readPositiveInt } from "../env";
import { type BackfillPorts, InjectedCrash, NO_FAULTS } from "../ports";

/**
 * Backfill Channel Durable Object (execution.md "Component Ownership").
 *
 * One instance per environment + Discord Channel. SQLite storage is the only progress
 * authority; the Alarm is the only continuation mechanism. Every alarm execution is
 * idempotent: it re-reads the run from storage, does bounded work and re-arms itself.
 */

export type StartResult =
  | { readonly ok: true; readonly run: RunRecord }
  | { readonly ok: false; readonly reason: "active_run_exists"; readonly run: RunRecord };

type RunRow = {
  run_id: string;
  guild_id: string;
  channel_id: string;
  range_after: string | null;
  range_before: string | null;
  page_limit: number;
  state: string;
  cursor_before: string | null;
  pages_archived: number;
  last_archived_observation_id: string | null;
  last_archived_key: string | null;
  next_eligible_at: number;
  attempt: number;
  terminal_error: string | null;
  pending_coordination: string | null;
  created_at: string;
  updated_at: string;
};

function rowToRun(row: RunRow): RunRecord {
  return {
    run_id: row.run_id as UuidV7,
    guild_id: row.guild_id as Snowflake,
    channel_id: row.channel_id as Snowflake,
    range: {
      after: row.range_after as Snowflake | null,
      before: row.range_before as Snowflake | null,
    },
    page_limit: row.page_limit,
    state: row.state as RunState,
    cursor_before: row.cursor_before as Snowflake | null,
    pages_archived: row.pages_archived,
    last_archived:
      row.last_archived_observation_id !== null && row.last_archived_key !== null
        ? {
            observation_id: row.last_archived_observation_id as UuidV7,
            archive_key: row.last_archived_key,
          }
        : null,
    next_eligible_at: row.next_eligible_at,
    attempt: row.attempt,
    terminal_error:
      row.terminal_error === null ? null : (JSON.parse(row.terminal_error) as TerminalError),
    pending_coordination:
      row.pending_coordination === null
        ? null
        : (JSON.parse(row.pending_coordination) as PendingCoordination),
    created_at: row.created_at,
    updated_at: row.updated_at,
  };
}

export class BackfillChannelDurableObject extends DurableObject<Env> {
  private ports: BackfillPorts;
  private stepConfig: StepConfig;
  private readonly pagesPerAlarm: number;

  constructor(ctx: DurableObjectState, env: Env) {
    super(ctx, env);
    this.pagesPerAlarm = readPositiveInt(env.BACKFILL_PAGES_PER_ALARM, 5);
    this.stepConfig = {
      discord_api_version: env.DISCORD_API_VERSION ?? "10",
      capabilities: { message_content: readBoolean(env.DISCORD_MESSAGE_CONTENT_ENABLED, false) },
    };
    this.ports = {
      discord: new FetchDiscordMessagesClient({
        base_url: env.DISCORD_API_BASE_URL ?? "https://discord.com/api/v10",
        bot_token: env.DISCORD_BOT_TOKEN ?? "",
        user_agent: "DiscordBot (https://github.com/ojiverse/datawarehouse, backfill)",
      }),
      archive: new R2ObservationArchive(env.OBSERVATIONS),
      budget: new BudgetDurableObjectClient(
        env.DISCORD_HTTP_BUDGET.get(
          env.DISCORD_HTTP_BUDGET.idFromName(budgetObjectName(env.ENVIRONMENT ?? "dev")),
        ),
      ),
      clock: {
        nowMs: () => Date.now(),
        randomBytes: (n) => crypto.getRandomValues(new Uint8Array(n)),
      },
      faults: NO_FAULTS,
    };
    ctx.blockConcurrencyWhile(async () => {
      this.migrate();
      // Restart re-discovery: an active run whose alarm was lost (crash between the progress
      // commit and setAlarm, or a platform eviction) must be re-armed from storage alone.
      await this.ensureScheduled();
    });
  }

  private migrate(): void {
    this.ctx.storage.sql.exec(
      "CREATE TABLE IF NOT EXISTS runs (" +
        "run_id TEXT PRIMARY KEY, guild_id TEXT NOT NULL, channel_id TEXT NOT NULL," +
        "range_after TEXT, range_before TEXT, page_limit INTEGER NOT NULL," +
        "state TEXT NOT NULL, cursor_before TEXT, pages_archived INTEGER NOT NULL DEFAULT 0," +
        "last_archived_observation_id TEXT, last_archived_key TEXT," +
        "next_eligible_at INTEGER NOT NULL, attempt INTEGER NOT NULL DEFAULT 0," +
        "terminal_error TEXT, pending_coordination TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);" +
        "CREATE UNIQUE INDEX IF NOT EXISTS runs_single_active ON runs(channel_id) WHERE state IN ('running','waiting');" +
        "CREATE TABLE IF NOT EXISTS route_buckets (route_key TEXT PRIMARY KEY, bucket TEXT, request_limit INTEGER," +
        "remaining INTEGER, reset_at INTEGER, observed_at INTEGER NOT NULL);",
    );
  }

  /**
   * Test hook: swaps ports / config. Only reachable through runInDurableObject.
   * `budget_object_name` builds a real Budget client from this object's own binding, because
   * workerd forbids using a stub that another context created.
   */
  useTestPorts(
    ports: Partial<BackfillPorts>,
    config?: Partial<StepConfig>,
    options?: { readonly budget_object_name?: string },
  ): void {
    const budget =
      options?.budget_object_name === undefined
        ? {}
        : {
            budget: new BudgetDurableObjectClient(
              this.env.DISCORD_HTTP_BUDGET.get(
                this.env.DISCORD_HTTP_BUDGET.idFromName(options.budget_object_name),
              ),
            ),
          };
    this.ports = { ...this.ports, ...ports, ...budget };
    this.stepConfig = { ...this.stepConfig, ...config };
  }

  private loadActiveRun(): RunRecord | null {
    const rows = this.ctx.storage.sql
      .exec<RunRow>("SELECT * FROM runs WHERE state IN ('running','waiting') LIMIT 1")
      .toArray();
    const row = rows[0];
    return row === undefined ? null : rowToRun(row);
  }

  private loadRun(runId: string): RunRecord | null {
    const rows = this.ctx.storage.sql
      .exec<RunRow>("SELECT * FROM runs WHERE run_id = ?", runId)
      .toArray();
    const row = rows[0];
    return row === undefined ? null : rowToRun(row);
  }

  private async ensureScheduled(): Promise<boolean> {
    const run = this.loadActiveRun();
    if (run === null) return false;
    const existing = await this.ctx.storage.getAlarm();
    if (existing === null) {
      await this.ctx.storage.setAlarm(Math.max(run.next_eligible_at, this.ports.clock.nowMs()));
    }
    return true;
  }

  async start(request: StartRunRequest): Promise<StartResult> {
    const active = this.loadActiveRun();
    if (active !== null) {
      return { ok: false, reason: "active_run_exists", run: active };
    }
    const nowMs = this.ports.clock.nowMs();
    const nowIso = new Date(nowMs).toISOString();
    const runId = generateUuidV7(nowMs, this.ports.clock.randomBytes);
    this.ctx.storage.sql.exec(
      "INSERT INTO runs (run_id, guild_id, channel_id, range_after, range_before, page_limit, state," +
        " cursor_before, pages_archived, next_eligible_at, attempt, created_at, updated_at)" +
        " VALUES (?, ?, ?, ?, ?, ?, 'running', NULL, 0, ?, 0, ?, ?)",
      runId,
      request.guild_id,
      request.channel_id,
      request.range.after,
      request.range.before,
      request.page_limit,
      nowMs,
      nowIso,
      nowIso,
    );
    await this.ctx.storage.setAlarm(nowMs);
    const run = this.loadRun(runId);
    if (run === null) throw new Error("run vanished immediately after insert");
    return { ok: true, run };
  }

  async status(runId: string): Promise<RunRecord | null> {
    return this.loadRun(runId);
  }

  async currentRun(): Promise<RunRecord | null> {
    const active = this.loadActiveRun();
    if (active !== null) return active;
    const rows = this.ctx.storage.sql
      .exec<RunRow>("SELECT * FROM runs ORDER BY created_at DESC LIMIT 1")
      .toArray();
    const row = rows[0];
    return row === undefined ? null : rowToRun(row);
  }

  async listRuns(): Promise<readonly RunRecord[]> {
    return this.ctx.storage.sql
      .exec<RunRow>("SELECT * FROM runs ORDER BY created_at DESC")
      .toArray()
      .map(rowToRun);
  }

  /** Re-arms the alarm for an active run; safe to call at any time. */
  async resume(): Promise<{ readonly scheduled: boolean }> {
    return { scheduled: await this.ensureScheduled() };
  }

  override async alarm(): Promise<void> {
    const run = this.loadActiveRun();
    if (run === null) return;

    let current = run;
    for (let i = 0; i < this.pagesPerAlarm; i++) {
      if (this.ports.clock.nowMs() < current.next_eligible_at) break;

      let outcome: PageOutcome;
      try {
        outcome =
          current.pending_coordination === null
            ? await executePage(this.ports, this.stepConfig, current)
            : await this.flushPendingCoordination(current, current.pending_coordination);
      } catch (error) {
        if (error instanceof InjectedCrash) throw error;
        outcome = {
          kind: "retry",
          detail: `unexpected: ${error instanceof Error ? error.message : String(error)}`,
          next_eligible_at: this.ports.clock.nowMs() + transientBackoffMs(current.attempt),
        };
      }
      current = this.applyOutcome(current, outcome);
      this.ports.faults.check("after_progress_before_alarm");
      // A freshly recorded safety-critical response is delivered to the Budget owner in the
      // same alarm (next iteration flushes it); a failed delivery waits for the backoff.
      if (outcome.kind === "coordination_pending" && !outcome.retry) continue;
      if (outcome.kind !== "archived" || outcome.completed) break;
    }

    if (isActiveState(current.state)) {
      await this.ctx.storage.setAlarm(current.next_eligible_at);
    }
  }

  /**
   * Replays a safety-critical Budget report before any new Discord request. On success the
   * pending record is left in place; clearing it and applying the deferred outcome happen
   * together in applyOutcome so that a crash between the two cannot lose the outcome.
   */
  private async flushPendingCoordination(
    run: RunRecord,
    pending: PendingCoordination,
  ): Promise<PageOutcome> {
    try {
      await this.ports.budget.report(pending.report);
    } catch (error) {
      return {
        kind: "coordination_pending",
        report: pending.report,
        deferred_outcome: pending.deferred_outcome,
        detail: `budget report still failing: ${error instanceof Error ? error.message : String(error)}`,
        next_eligible_at: this.ports.clock.nowMs() + transientBackoffMs(run.attempt),
        retry: true,
      };
    }
    this.ports.faults.check("after_coordination_report_before_transition");
    return { kind: "coordination_resolved", deferred_outcome: pending.deferred_outcome };
  }

  private applyOutcome(run: RunRecord, outcome: PageOutcome): RunRecord {
    const nowIso = new Date(this.ports.clock.nowMs()).toISOString();
    const sql = this.ctx.storage.sql;
    switch (outcome.kind) {
      case "archived": {
        this.ports.faults.check("after_archive_before_progress");
        this.ctx.storage.transactionSync(() => {
          sql.exec(
            "UPDATE runs SET state = ?, cursor_before = ?, pages_archived = pages_archived + 1," +
              " last_archived_observation_id = ?, last_archived_key = ?, next_eligible_at = ?, attempt = 0, updated_at = ?" +
              " WHERE run_id = ?",
            outcome.completed ? "completed" : "running",
            outcome.completed ? run.cursor_before : outcome.next_before,
            outcome.archived.observation_id,
            outcome.archived.key,
            outcome.next_eligible_at,
            nowIso,
            run.run_id,
          );
          this.upsertRouteBucket(outcome.route_bucket);
        });
        break;
      }
      case "deferred": {
        if (outcome.reason === "budget" && outcome.detail === "credential_halt") {
          this.markTerminal(run, {
            kind: "credential_failure",
            http_status: null,
            message: "budget owner reported a credential failure",
            occurred_at: nowIso,
          });
          break;
        }
        this.ctx.storage.transactionSync(() => {
          sql.exec(
            "UPDATE runs SET state = 'waiting', next_eligible_at = ?, updated_at = ? WHERE run_id = ?",
            outcome.next_eligible_at,
            nowIso,
            run.run_id,
          );
          if (outcome.route_bucket !== null) this.upsertRouteBucket(outcome.route_bucket);
        });
        break;
      }
      case "retry": {
        sql.exec(
          "UPDATE runs SET state = 'running', next_eligible_at = ?, attempt = attempt + 1, updated_at = ? WHERE run_id = ?",
          outcome.next_eligible_at,
          nowIso,
          run.run_id,
        );
        console.warn(
          JSON.stringify({ event: "backfill_retry", run_id: run.run_id, detail: outcome.detail }),
        );
        break;
      }
      case "terminal": {
        this.markTerminal(run, outcome.error);
        break;
      }
      case "coordination_resolved": {
        // One transaction: the run must never be observed with the pending cleared but the
        // deferred 401 / 403 / 429 outcome not yet applied (that would allow a new request).
        this.ctx.storage.transactionSync(() => {
          sql.exec("UPDATE runs SET pending_coordination = NULL WHERE run_id = ?", run.run_id);
          const deferred = outcome.deferred_outcome;
          if (deferred.kind === "terminal") {
            this.markTerminal(run, deferred.error);
          } else {
            sql.exec(
              "UPDATE runs SET state = 'waiting', next_eligible_at = ?, updated_at = ? WHERE run_id = ?",
              deferred.next_eligible_at,
              nowIso,
              run.run_id,
            );
          }
        });
        break;
      }
      case "coordination_pending": {
        const pending: PendingCoordination = {
          report: outcome.report,
          deferred_outcome: outcome.deferred_outcome,
        };
        sql.exec(
          "UPDATE runs SET state = 'waiting', pending_coordination = ?, next_eligible_at = ?, attempt = attempt + ?, updated_at = ?" +
            " WHERE run_id = ?",
          JSON.stringify(pending),
          outcome.next_eligible_at,
          outcome.retry ? 1 : 0,
          nowIso,
          run.run_id,
        );
        console.error(
          JSON.stringify({
            event: "backfill_budget_coordination_pending",
            run_id: run.run_id,
            status: outcome.report.status,
            detail: outcome.detail,
          }),
        );
        break;
      }
    }
    const updated = this.loadRun(run.run_id);
    if (updated === null) throw new Error(`run ${run.run_id} disappeared while applying outcome`);
    return updated;
  }

  private markTerminal(run: RunRecord, error: TerminalError): void {
    const state: RunState = error.kind === "credential_failure" ? "halted" : "failed";
    this.ctx.storage.sql.exec(
      "UPDATE runs SET state = ?, terminal_error = ?, updated_at = ? WHERE run_id = ?",
      state,
      JSON.stringify(error),
      error.occurred_at,
      run.run_id,
    );
    console.error(JSON.stringify({ event: "backfill_terminal", run_id: run.run_id, state, error }));
  }

  private upsertRouteBucket(bucket: RouteBucketState): void {
    this.ctx.storage.sql.exec(
      "INSERT INTO route_buckets (route_key, bucket, request_limit, remaining, reset_at, observed_at) VALUES (?, ?, ?, ?, ?, ?)" +
        " ON CONFLICT(route_key) DO UPDATE SET bucket = excluded.bucket, request_limit = excluded.request_limit," +
        " remaining = excluded.remaining, reset_at = excluded.reset_at, observed_at = excluded.observed_at",
      bucket.route_key,
      bucket.bucket,
      bucket.limit,
      bucket.remaining,
      bucket.reset_at,
      bucket.observed_at,
    );
  }
}
