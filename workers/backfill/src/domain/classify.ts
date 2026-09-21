import type { RateLimitScope } from "./rate-limit";

/**
 * HTTP status classification defined in execution.md ("Discord Rate Limit").
 *
 * 401 → credential failure (halt every Backfill), 403 → terminal scope failure,
 * 429 → pause per Retry-After. Everything else is either success, a permanent client error
 * (which must not be retried forever) or a transient server-side condition.
 */
export type ResponseClass =
  | "success"
  | "rate_limited"
  | "credential_failure"
  | "scope_forbidden"
  | "scope_not_found"
  | "client_error"
  | "server_error";

export function classifyStatus(status: number): ResponseClass {
  if (status >= 200 && status < 300) return "success";
  if (status === 401) return "credential_failure";
  if (status === 403) return "scope_forbidden";
  if (status === 404) return "scope_not_found";
  if (status === 429) return "rate_limited";
  if (status >= 400 && status < 500) return "client_error";
  return "server_error";
}

/**
 * Whether a response counts against Discord's invalid-request budget
 * (401 / 403 / 429, except 429 with `X-RateLimit-Scope: shared`).
 */
export function countsAsInvalidRequest(status: number, scope: RateLimitScope | null): boolean {
  if (status === 401 || status === 403) return true;
  if (status === 429) return scope !== "shared";
  return false;
}
