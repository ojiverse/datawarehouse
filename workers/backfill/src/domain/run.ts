import type { Snowflake, UuidV7 } from "./ids";

/**
 * Backfill run model (docs/architecture/cloudflare/backfill/execution.md, "Durable Progress").
 *
 * A run is the unit of durable progress owned by one Backfill Channel Durable Object.
 * Every field below is persisted in the Durable Object's SQLite storage; nothing here lives
 * only in memory.
 */

/** Requested crawl range expressed as exclusive Snowflake bounds. */
export type BackfillRange = {
  /** Exclusive lower bound: messages with an ID at or below this value are not requested. */
  readonly after: Snowflake | null;
  /** Exclusive upper bound: the first page is requested with this value as `before`. */
  readonly before: Snowflake | null;
};

/** Run states. `running` and `waiting` are active; the other three are terminal. */
export type RunState = "running" | "waiting" | "completed" | "failed" | "halted";

export const ACTIVE_RUN_STATES: readonly RunState[] = ["running", "waiting"];

export function isActiveState(state: RunState): boolean {
  return ACTIVE_RUN_STATES.includes(state);
}

/** Reasons a run reaches a terminal state other than `completed`. */
export type TerminalErrorKind =
  | "credential_failure"
  | "scope_inaccessible"
  | "scope_not_found"
  | "invalid_request"
  | "archive_invariant_violation";

export type TerminalError = {
  readonly kind: TerminalErrorKind;
  readonly http_status: number | null;
  readonly message: string;
  readonly occurred_at: string;
};

/** The last page that reached Durable Acceptance (R2 commit) for this run. */
export type ArchivedPageRef = {
  readonly observation_id: UuidV7;
  readonly archive_key: string;
};

/**
 * A safety-critical Budget report (401 / 403 / 429) that could not be delivered, together with
 * the outcome that must be applied once it is. While this is set the run issues no Discord
 * request: the invalid-request budget and the global pause are shared state, and acting
 * before the owner has recorded them would let every other Channel keep going.
 */
export type PendingCoordination = {
  readonly report: {
    readonly status: number;
    readonly scope: "user" | "global" | "shared" | null;
    readonly global: boolean;
    readonly retry_after_ms: number | null;
  };
  readonly deferred_outcome: DeferredOutcome;
};

/** Serialisable subset of the page outcome that a pending coordination replays later. */
export type DeferredOutcome =
  | { readonly kind: "terminal"; readonly error: TerminalError }
  | {
      readonly kind: "deferred";
      readonly reason: "rate_limited";
      readonly detail: string;
      readonly next_eligible_at: number;
      readonly route_bucket: null;
    };

export type RunRecord = {
  readonly run_id: UuidV7;
  readonly guild_id: Snowflake;
  readonly channel_id: Snowflake;
  readonly range: BackfillRange;
  readonly page_limit: number;
  readonly state: RunState;
  /**
   * `before` parameter of the next HTTP request. `null` means "not advanced yet": the next
   * request uses `range.before`. Advanced only after the previous page's R2 commit.
   */
  readonly cursor_before: Snowflake | null;
  readonly pages_archived: number;
  readonly last_archived: ArchivedPageRef | null;
  /** Unix milliseconds before which no HTTP request may be issued for this run. */
  readonly next_eligible_at: number;
  /** Consecutive transient failures since the last successful page. */
  readonly attempt: number;
  readonly terminal_error: TerminalError | null;
  readonly pending_coordination: PendingCoordination | null;
  readonly created_at: string;
  readonly updated_at: string;
};

/** Parameters accepted when a new run is started. */
export type StartRunRequest = {
  readonly guild_id: Snowflake;
  readonly channel_id: Snowflake;
  readonly range: BackfillRange;
  readonly page_limit: number;
};

/** Returns the `before` value for the next request of `run`. */
export function nextRequestBefore(run: RunRecord): Snowflake | null {
  return run.cursor_before ?? run.range.before;
}

/**
 * Exponential backoff for transient failures (network errors, 5xx).
 * Bounded so that a persistent outage keeps polling at a slow, predictable rate.
 */
export function transientBackoffMs(attempt: number): number {
  const base = 2_000;
  const max = 5 * 60_000;
  const exp = Math.min(attempt, 10);
  return Math.min(max, base * 2 ** exp);
}
