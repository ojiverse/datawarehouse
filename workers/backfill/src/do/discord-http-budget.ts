import { DurableObject } from "cloudflare:workers";
import { countsAsInvalidRequest } from "../domain/classify";
import { type Env, readPositiveInt } from "../env";
import type { BudgetDecision, RequestReport } from "../ports";

/**
 * Discord HTTP Budget Durable Object (docs/infrastructure/cloudflare/durable-objects/README.md).
 *
 * One instance per environment / Discord application. It owns the state that is shared by
 * every Backfill Channel Durable Object: the global request ceiling, the invalid-request
 * budget, a global 429 pause and the credential-failure halt. Route buckets are *not* here;
 * they belong to the Channel owner because they are per-route, response-driven state.
 */

const GLOBAL_WINDOW_MS = 1_000;

/**
 * Fraction of the documented invalid-request budget this application allows itself to use.
 * The remainder is head-room for requests already in flight and for other producers sharing
 * the same bot token.
 */
const INVALID_BUDGET_SAFETY_RATIO = 0.9;

export type BudgetSnapshot = {
  readonly requests_in_window: number;
  readonly invalid_in_window: number;
  readonly global_retry_until: number | null;
  readonly credential_halt_at: number | null;
};

export class DiscordHttpBudgetDurableObject extends DurableObject<Env> {
  private readonly globalCeiling: number;
  private readonly invalidBudget: number;
  private readonly invalidWindowMs: number;
  private now: () => number = () => Date.now();

  constructor(ctx: DurableObjectState, env: Env) {
    super(ctx, env);
    this.globalCeiling = readPositiveInt(env.DISCORD_GLOBAL_REQUESTS_PER_SECOND, 50);
    this.invalidBudget = readPositiveInt(env.DISCORD_INVALID_REQUEST_BUDGET, 10_000);
    this.invalidWindowMs = readPositiveInt(env.DISCORD_INVALID_REQUEST_WINDOW_SECONDS, 600) * 1_000;
    ctx.blockConcurrencyWhile(async () => {
      this.migrate();
    });
  }

  private migrate(): void {
    this.ctx.storage.sql.exec(
      "CREATE TABLE IF NOT EXISTS request_slots (ts_ms INTEGER NOT NULL);" +
        "CREATE INDEX IF NOT EXISTS request_slots_ts ON request_slots(ts_ms);" +
        "CREATE TABLE IF NOT EXISTS invalid_requests (ts_ms INTEGER NOT NULL);" +
        "CREATE INDEX IF NOT EXISTS invalid_requests_ts ON invalid_requests(ts_ms);" +
        "CREATE TABLE IF NOT EXISTS budget_state (key TEXT PRIMARY KEY, value INTEGER NOT NULL);" +
        "CREATE TABLE IF NOT EXISTS processed_reports (report_id TEXT PRIMARY KEY, ts_ms INTEGER NOT NULL);" +
        "CREATE INDEX IF NOT EXISTS processed_reports_ts ON processed_reports(ts_ms);",
    );
  }

  /** Test hook: replaces the wall clock. Only reachable through runInDurableObject. */
  useClock(now: () => number): void {
    this.now = now;
  }

  private readState(key: string): number | null {
    const rows = this.ctx.storage.sql
      .exec<{ value: number }>("SELECT value FROM budget_state WHERE key = ?", key)
      .toArray();
    return rows[0]?.value ?? null;
  }

  private writeState(key: string, value: number | null): void {
    if (value === null) {
      this.ctx.storage.sql.exec("DELETE FROM budget_state WHERE key = ?", key);
      return;
    }
    this.ctx.storage.sql.exec(
      "INSERT INTO budget_state (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
      key,
      value,
    );
  }

  private prune(nowMs: number): void {
    this.ctx.storage.sql.exec(
      "DELETE FROM request_slots WHERE ts_ms <= ?",
      nowMs - GLOBAL_WINDOW_MS,
    );
    this.ctx.storage.sql.exec(
      "DELETE FROM invalid_requests WHERE ts_ms <= ?",
      nowMs - this.invalidWindowMs,
    );
    // Processed ids only need to outlive the window in which a duplicate could still matter.
    this.ctx.storage.sql.exec(
      "DELETE FROM processed_reports WHERE ts_ms <= ?",
      nowMs - this.invalidWindowMs,
    );
  }

  private countSince(table: "request_slots" | "invalid_requests", sinceMs: number): number {
    const row = this.ctx.storage.sql
      .exec<{ n: number }>(`SELECT COUNT(*) AS n FROM ${table} WHERE ts_ms > ?`, sinceMs)
      .one();
    return row.n;
  }

  private oldestSince(table: "request_slots" | "invalid_requests", sinceMs: number): number | null {
    const row = this.ctx.storage.sql
      .exec<{ ts: number | null }>(`SELECT MIN(ts_ms) AS ts FROM ${table} WHERE ts_ms > ?`, sinceMs)
      .one();
    return row.ts;
  }

  async acquire(): Promise<BudgetDecision> {
    const nowMs = this.now();
    return this.ctx.storage.transactionSync(() => {
      this.prune(nowMs);

      if (this.readState("credential_halt_at") !== null) {
        return { granted: false, reason: "credential_halt", retry_after_ms: this.invalidWindowMs };
      }

      const globalRetryUntil = this.readState("global_retry_until");
      if (globalRetryUntil !== null && globalRetryUntil > nowMs) {
        return {
          granted: false,
          reason: "global_retry_after",
          retry_after_ms: globalRetryUntil - nowMs,
        };
      }

      const invalidSince = nowMs - this.invalidWindowMs;
      const invalidCount = this.countSince("invalid_requests", invalidSince);
      if (invalidCount >= Math.floor(this.invalidBudget * INVALID_BUDGET_SAFETY_RATIO)) {
        const oldest = this.oldestSince("invalid_requests", invalidSince) ?? nowMs;
        return {
          granted: false,
          reason: "invalid_request_budget",
          retry_after_ms: Math.max(1_000, oldest + this.invalidWindowMs - nowMs),
        };
      }

      const globalSince = nowMs - GLOBAL_WINDOW_MS;
      const requestCount = this.countSince("request_slots", globalSince);
      if (requestCount >= this.globalCeiling) {
        const oldest = this.oldestSince("request_slots", globalSince) ?? nowMs;
        return {
          granted: false,
          reason: "global_ceiling",
          retry_after_ms: Math.max(1, oldest + GLOBAL_WINDOW_MS - nowMs),
        };
      }

      this.ctx.storage.sql.exec("INSERT INTO request_slots (ts_ms) VALUES (?)", nowMs);
      return { granted: true };
    });
  }

  /**
   * Idempotent on `report_id`: a Channel that crashed after this call succeeded re-sends the
   * same report, and counting it twice would drain the safety budget early.
   */
  async report(report: RequestReport): Promise<void> {
    const nowMs = this.now();
    this.ctx.storage.transactionSync(() => {
      this.prune(nowMs);
      const seen = this.ctx.storage.sql
        .exec<{ n: number }>(
          "SELECT COUNT(*) AS n FROM processed_reports WHERE report_id = ?",
          report.report_id,
        )
        .one();
      if (seen.n > 0) return;
      this.ctx.storage.sql.exec(
        "INSERT INTO processed_reports (report_id, ts_ms) VALUES (?, ?)",
        report.report_id,
        nowMs,
      );
      if (countsAsInvalidRequest(report.status, report.scope)) {
        this.ctx.storage.sql.exec("INSERT INTO invalid_requests (ts_ms) VALUES (?)", nowMs);
      }
      if (report.status === 429 && (report.global || report.scope === "global")) {
        const until = nowMs + (report.retry_after_ms ?? 1_000);
        const current = this.readState("global_retry_until");
        this.writeState("global_retry_until", current === null ? until : Math.max(current, until));
      }
      if (report.status === 401) {
        this.writeState("credential_halt_at", nowMs);
      }
    });
  }

  /** Operator action after the bot token has been rotated. */
  async clearCredentialHalt(): Promise<void> {
    this.writeState("credential_halt_at", null);
  }

  async snapshot(): Promise<BudgetSnapshot> {
    const nowMs = this.now();
    return {
      requests_in_window: this.countSince("request_slots", nowMs - GLOBAL_WINDOW_MS),
      invalid_in_window: this.countSince("invalid_requests", nowMs - this.invalidWindowMs),
      global_retry_until: this.readState("global_retry_until"),
      credential_halt_at: this.readState("credential_halt_at"),
    };
  }
}
