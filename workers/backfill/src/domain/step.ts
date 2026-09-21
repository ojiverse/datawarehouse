import type { ArchiveObject } from "../observation/archive-object";
import { buildArchiveObject } from "../observation/archive-object";
import type { HttpCapabilities } from "../observation/envelope";
import { buildHttpBackfillEnvelope } from "../observation/envelope";
import type { BackfillPorts, DiscordHttpResponse } from "../ports";
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
import { nextRequestBefore, type RunRecord, type TerminalError, transientBackoffMs } from "./run";

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
): PageOutcome {
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

async function reportQuietly(
  ports: BackfillPorts,
  response: DiscordHttpResponse,
  retryAfterMs: number | null,
) {
  try {
    await ports.budget.report({
      status: response.status,
      scope: response.rate_limit.scope,
      global: response.rate_limit.global,
      retry_after_ms: retryAfterMs,
    });
  } catch (error) {
    // Budget bookkeeping is advisory; losing one report must not block durable progress.
    console.warn(
      JSON.stringify({
        event: "budget_report_failed",
        error: error instanceof Error ? error.message : String(error),
      }),
    );
  }
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
      await reportQuietly(ports, response, null);
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
      await reportQuietly(ports, response, retryAfterMs);
      return {
        kind: "deferred",
        reason: "rate_limited",
        detail: `429 scope=${response.rate_limit.scope ?? "unknown"} invalid=${countsAsInvalidRequest(429, response.rate_limit.scope)}`,
        next_eligible_at: completedAt + retryAfterMs,
        route_bucket: routeBucket,
      };
    }
    case "credential_failure":
      await reportQuietly(ports, response, null);
      return terminal(
        "credential_failure",
        response.status,
        "Discord rejected the bot credential",
        completedAt,
      );
    case "scope_forbidden":
      await reportQuietly(ports, response, null);
      return terminal(
        "scope_inaccessible",
        response.status,
        "channel history is not accessible",
        completedAt,
      );
    case "scope_not_found":
      await reportQuietly(ports, response, null);
      return terminal(
        "scope_not_found",
        response.status,
        "channel does not exist or is not visible",
        completedAt,
      );
    case "client_error":
      await reportQuietly(ports, response, null);
      return terminal(
        "invalid_request",
        response.status,
        "Discord rejected the request",
        completedAt,
      );
    case "server_error":
      await reportQuietly(ports, response, null);
      return {
        kind: "retry",
        detail: `discord ${response.status}`,
        next_eligible_at: completedAt + transientBackoffMs(run.attempt),
      };
  }
}
