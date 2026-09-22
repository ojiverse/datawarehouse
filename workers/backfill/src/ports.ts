import type { Snowflake, UuidV7 } from "./domain/ids";
import type { RateLimitHeaders, RateLimitScope } from "./domain/rate-limit";
import type { ArchiveObject } from "./observation/archive-object";

/**
 * Ports through which the Backfill Channel Durable Object reaches the outside world.
 * Production adapters live in ./adapters; tests substitute fakes to inject faults.
 */

export type ChannelMessagesRequest = {
  readonly channel_id: Snowflake;
  readonly before: Snowflake | null;
  readonly limit: number;
};

export type DiscordHttpResponse = {
  readonly status: number;
  readonly rate_limit: RateLimitHeaders;
  /** Raw UTF-8 response body; hashed and archived as-is. */
  readonly body: Uint8Array;
  readonly request_started_at: string;
  readonly response_completed_at: string;
  readonly response_completed_at_ms: number;
};

export interface DiscordMessagesClient {
  /**
   * Issues one Get Channel Messages request.
   * Resolves for every HTTP status; rejects only on transport failure (DNS, TLS, reset).
   * @throws {Error} on network / transport failure
   */
  fetchChannelMessages(request: ChannelMessagesRequest): Promise<DiscordHttpResponse>;
}

export type ArchiveCommitResult = {
  readonly key: string;
  /** `created` on first write, `already_present` when an identical retry found the object. */
  readonly outcome: "created" | "already_present";
};

/** Thrown when an existing object under the same key does not match the retried Observation. */
export class ArchiveInvariantViolation extends Error {
  override readonly name = "ArchiveInvariantViolation";
}

export interface ObservationArchive {
  /**
   * Create-only commit of one Observation object (Durable Acceptance on resolve).
   * Idempotent for a retry carrying the same Observation ID and payload hash.
   * @throws {ArchiveInvariantViolation} when the key exists with a different ID or hash
   * @throws {Error} on storage failure; the caller must not advance progress
   */
  commit(object: ArchiveObject): Promise<ArchiveCommitResult>;
}

export type BudgetDenialReason =
  | "global_ceiling"
  | "invalid_request_budget"
  | "global_retry_after"
  | "credential_halt";

export type BudgetDecision =
  | { readonly granted: true }
  | {
      readonly granted: false;
      readonly reason: BudgetDenialReason;
      readonly retry_after_ms: number;
    };

export type RequestReport = {
  /**
   * Stable identity of one Discord request, fixed before the request is issued. A coordination
   * retry of the same response re-sends the same id; a refetch is a new request with a new id.
   * The Budget owner uses it to make re-sends idempotent.
   */
  readonly report_id: UuidV7;
  readonly status: number;
  readonly scope: RateLimitScope | null;
  readonly global: boolean;
  readonly retry_after_ms: number | null;
};

export interface BudgetGate {
  /**
   * Reserves one request slot against the application-wide global ceiling and
   * invalid-request budget. Not idempotent: each grant consumes one slot.
   */
  acquire(): Promise<BudgetDecision>;
  /**
   * Records the outcome of a request so the Budget owner can count invalid requests,
   * honour global 429 pauses and halt on credential failure. Idempotent on `report_id`:
   * a re-sent report is a no-op success. Resolves only once the owner has durably recorded
   * the report.
   * @throws {Error} when the owner cannot be reached or cannot persist the report. Callers
   *   reporting 401 / 403 / 429 must persist the report and retry it before issuing any new
   *   request; callers reporting 2xx / 5xx may log and continue.
   */
  report(report: RequestReport): Promise<void>;
}

export interface Clock {
  nowMs(): number;
  randomBytes(length: number): Uint8Array;
}

/** Named points at which a test may terminate execution to prove crash safety. */
export type FaultPoint =
  | "after_fetch_before_archive"
  | "after_archive_before_progress"
  | "after_progress_before_alarm"
  | "after_coordination_report_before_transition";

/** Thrown by a FaultInjector; must propagate untouched so it behaves like a runtime termination. */
export class InjectedCrash extends Error {
  override readonly name = "InjectedCrash";
  constructor(readonly point: FaultPoint) {
    super(`injected crash at ${point}`);
  }
}

export interface FaultInjector {
  /**
   * Called at each fault point. A production injector is a no-op.
   * @throws {InjectedCrash} when the test asked for a crash at this point
   */
  check(point: FaultPoint): void;
}

export const NO_FAULTS: FaultInjector = { check: () => undefined };

export type BackfillPorts = {
  readonly discord: DiscordMessagesClient;
  readonly archive: ObservationArchive;
  readonly budget: BudgetGate;
  readonly clock: Clock;
  readonly faults: FaultInjector;
};
