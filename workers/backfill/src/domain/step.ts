import type { ArchiveObject } from "../observation/archive-object";
import { buildArchiveObject } from "../observation/archive-object";
import type { HttpCapabilities } from "../observation/envelope";
import { buildHttpBackfillEnvelope } from "../observation/envelope";
import type { BackfillPorts, DiscordHttpResponse, RequestReport } from "../ports";
import { ArchiveInvariantViolation, InjectedCrash } from "../ports";
import { classifyStatus, countsAsInvalidRequest } from "./classify";
import { advanceCursor } from "./cursor";
import { generateUuidV7, parseSnowflake, type Snowflake } from "./ids";
import {
  nextEligibleAfterSuccess,
  type RateLimitedBody,
  type RouteBucketState,
  retryAfterMsFor429,
  routeBucketFromHeaders,
} from "./rate-limit";
import {
  type DeferredOutcome,
  nextRequestBefore,
  type RunRecord,
  type TerminalError,
  transientBackoffMs,
} from "./run";

/**
 * One page of the Page Commit Order (execution.md):
 *   read cursor → fetch page → build Envelope → R2 commit → (caller advances progress).
 *
 * This module never touches Durable Object storage. It returns a value describing what the
 * caller must persist, so the same logic is testable without a runtime and the storage
 * transaction stays in one place.
 */

export type StepConfig = {
  readonly discord_api_version: string;
  readonly capabilities: HttpCapabilities;
};

export type PageOutcome =
  | {
      readonly kind: "archived";
      readonly archived: ArchiveObject;
      readonly next_before: Snowflake | null;
      readonly completed: boolean;
      readonly next_eligible_at: number;
      readonly route_bucket: RouteBucketState;
    }
  | {
      readonly kind: "deferred";
      readonly reason: "budget" | "rate_limited";
      readonly detail: string;
      readonly next_eligible_at: number;
      readonly route_bucket: RouteBucketState | null;
    }
  | {
      readonly kind: "retry";
      readonly detail: string;
      readonly next_eligible_at: number;
    }
  | {
      readonly kind: "terminal";
      readonly error: TerminalError;
    }
  | {
      /**
       * A safety-critical report could not reach the Budget owner. The caller persists the
       * report and the deferred outcome, issues no request, and retries the report first.
       */
      readonly kind: "coordination_pending";
      readonly report: RequestReport;
      readonly deferred_outcome: DeferredOutcome;
      readonly detail: string;
      readonly next_eligible_at: number;
    }
  | {
      /**
       * A pending report has now been recorded by the Budget owner. The caller must clear the
       * pending coordination and apply the deferred outcome in one storage transaction, so a
       * crash in between can never leave the run without the outcome.
       */
      readonly kind: "coordination_resolved";
      readonly deferred_outcome: DeferredOutcome;
    };

export const ROUTE_KEY_CHANNEL_MESSAGES = "GET /channels/{channel_id}/messages";

function parseMessageIds(payload: unknown): readonly Snowflake[] {
  if (!Array.isArray(payload)) {
    throw new Error("Discord page payload is not an array of messages");
  }
  return payload.map((message: unknown) => {
    const id =
      typeof message === "object" && message !== null && "id" in message
        ? parseSnowflake((message as { id?: unknown }).id)
        : undefined;
    if (id === undefined) {
      throw new Error("Discord message without a valid Snowflake id");
    }
    return id;
  });
}

function parseJsonBody(body: Uint8Array): unknown {
  return JSON.parse(new TextDecoder("utf-8", { fatal: true, ignoreBOM: false }).decode(body));
}

function terminal(
  kind: TerminalError["kind"],
  status: number | null,
  message: string,
  occurredAtMs: number,
): DeferredOutcome & { readonly kind: "terminal" } {
  return {
    kind: "terminal",
    error: {
      kind,
      http_status: status,
      message,
      occurred_at: new Date(occurredAtMs).toISOString(),
    },
  };
}

function reportFor(response: DiscordHttpResponse, retryAfterMs: number | null): RequestReport {
  return {
    status: response.status,
    scope: response.rate_limit.scope,
    global: response.rate_limit.global,
    retry_after_ms: retryAfterMs,
  };
}

/**
 * Advisory report (2xx, 5xx): the Budget owner only uses it for diagnostics, so a failed
 * delivery must not block durable progress.
 */
async function reportAdvisory(ports: BackfillPorts, response: DiscordHttpResponse) {
  try {
    await ports.budget.report(reportFor(response, null));
  } catch (error) {
    console.warn(
      JSON.stringify({
        event: "budget_report_failed",
        status: response.status,
        error: error instanceof Error ? error.message : String(error),
      }),
    );
  }
}

/**
 * Fail-closed report for 401 / 403 / 429. These feed the application-wide invalid-request
 * budget, the global pause and the credential halt; if the owner cannot record them the run
 * must stop issuing requests and replay the report before doing anything else.
 */
async function reportSafetyCritical(
  ports: BackfillPorts,
  run: RunRecord,
  response: DiscordHttpResponse,
  retryAfterMs: number | null,
  deferred: DeferredOutcome,
): Promise<PageOutcome> {
  const report = reportFor(response, retryAfterMs);
  try {
    await ports.budget.report(report);
  } catch (error) {
    return {
      kind: "coordination_pending",
      report,
      deferred_outcome: deferred,
      detail: `budget report failed: ${error instanceof Error ? error.message : String(error)}`,
      next_eligible_at: ports.clock.nowMs() + transientBackoffMs(run.attempt),
    };
  }
  return deferred;
}

export async function executePage(
  ports: BackfillPorts,
  config: StepConfig,
  run: RunRecord,
): Promise<PageOutcome> {
  const decision = await ports.budget.acquire();
  const nowMs = ports.clock.nowMs();
  if (!decision.granted) {
    return {
      kind: "deferred",
      reason: "budget",
      detail: decision.reason,
      next_eligible_at: nowMs + decision.retry_after_ms,
      route_bucket: null,
    };
  }

  const before = nextRequestBefore(run);
  let response: DiscordHttpResponse;
  try {
    response = await ports.discord.fetchChannelMessages({
      channel_id: run.channel_id,
      before,
      limit: run.page_limit,
    });
  } catch (error) {
    return {
      kind: "retry",
      detail: `transport failure: ${error instanceof Error ? error.message : String(error)}`,
      next_eligible_at: nowMs + transientBackoffMs(run.attempt),
    };
  }

  const completedAt = response.response_completed_at_ms;
  const routeBucket = routeBucketFromHeaders(
    ROUTE_KEY_CHANNEL_MESSAGES,
    completedAt,
    response.rate_limit,
  );

  switch (classifyStatus(response.status)) {
    case "success": {
      await reportAdvisory(ports, response);
      ports.faults.check("after_fetch_before_archive");

      let payload: unknown;
      let messageIds: readonly Snowflake[];
      try {
        payload = parseJsonBody(response.body);
        messageIds = parseMessageIds(payload);
      } catch (error) {
        // A 2xx body that is not a message array is a contract break, not a transient fault.
        return terminal(
          "invalid_request",
          response.status,
          `unparseable page: ${error instanceof Error ? error.message : String(error)}`,
          completedAt,
        );
      }

      const envelope = buildHttpBackfillEnvelope({
        observation_id: generateUuidV7(completedAt, ports.clock.randomBytes),
        run_id: run.run_id,
        discord_api_version: config.discord_api_version,
        guild_id: run.guild_id,
        channel_id: run.channel_id,
        pagination: { before, after: null },
        limit: run.page_limit,
        request_started_at: response.request_started_at,
        response_completed_at: response.response_completed_at,
        http_status: response.status,
        payload,
        capabilities: config.capabilities,
        rate_limit: response.rate_limit,
      });
      const object = await buildArchiveObject(envelope, response.body);

      try {
        await ports.archive.commit(object);
      } catch (error) {
        if (error instanceof InjectedCrash) throw error;
        if (error instanceof ArchiveInvariantViolation) {
          return terminal("archive_invariant_violation", null, error.message, completedAt);
        }
        return {
          kind: "retry",
          detail: `archive commit failed: ${error instanceof Error ? error.message : String(error)}`,
          next_eligible_at: nowMs + transientBackoffMs(run.attempt),
        };
      }

      const advance = advanceCursor(messageIds, run.page_limit, run.range);
      return {
        kind: "archived",
        archived: object,
        next_before: advance.next_before,
        completed: advance.completed,
        next_eligible_at: nextEligibleAfterSuccess(completedAt, response.rate_limit),
        route_bucket: routeBucket,
      };
    }
    case "rate_limited": {
      let body: RateLimitedBody | null = null;
      try {
        body = parseJsonBody(response.body) as RateLimitedBody;
      } catch {
        body = null;
      }
      const retryAfterMs = retryAfterMsFor429(response.rate_limit, body);
      const outcome = await reportSafetyCritical(ports, run, response, retryAfterMs, {
        kind: "deferred",
        reason: "rate_limited",
        detail: `429 scope=${response.rate_limit.scope ?? "unknown"} invalid=${countsAsInvalidRequest(429, response.rate_limit.scope)}`,
        next_eligible_at: completedAt + retryAfterMs,
        route_bucket: null,
      });
      return outcome.kind === "deferred" ? { ...outcome, route_bucket: routeBucket } : outcome;
    }
    case "credential_failure":
      return reportSafetyCritical(
        ports,
        run,
        response,
        null,
        terminal(
          "credential_failure",
          response.status,
          "Discord rejected the bot credential",
          completedAt,
        ),
      );
    case "scope_forbidden":
      return reportSafetyCritical(
        ports,
        run,
        response,
        null,
        terminal(
          "scope_inaccessible",
          response.status,
          "channel history is not accessible",
          completedAt,
        ),
      );
    case "scope_not_found":
      await reportAdvisory(ports, response);
      return terminal(
        "scope_not_found",
        response.status,
        "channel does not exist or is not visible",
        completedAt,
      );
    case "client_error":
      await reportAdvisory(ports, response);
      return terminal(
        "invalid_request",
        response.status,
        "Discord rejected the request",
        completedAt,
      );
    case "server_error":
      await reportAdvisory(ports, response);
      return {
        kind: "retry",
        detail: `discord ${response.status}`,
        next_eligible_at: completedAt + transientBackoffMs(run.attempt),
      };
  }
}
