/**
 * Discord HTTP rate-limit metadata (https://docs.discord.com/developers/topics/rate-limits).
 *
 * Route limits are never hard-coded; every scheduling decision is derived from the headers of
 * the most recent response, as required by execution.md.
 */

export type RateLimitScope = "user" | "global" | "shared";

export type RateLimitHeaders = {
  readonly limit: number | null;
  readonly remaining: number | null;
  /** Seconds until the bucket resets, with sub-second precision. */
  readonly reset_after_seconds: number | null;
  readonly bucket: string | null;
  readonly global: boolean;
  readonly scope: RateLimitScope | null;
  /** `Retry-After` header in seconds. */
  readonly retry_after_seconds: number | null;
};

export const EMPTY_RATE_LIMIT_HEADERS: RateLimitHeaders = {
  limit: null,
  remaining: null,
  reset_after_seconds: null,
  bucket: null,
  global: false,
  scope: null,
  retry_after_seconds: null,
};

function parseNumber(value: string | null): number | null {
  if (value === null) return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
}

function parseScope(value: string | null): RateLimitScope | null {
  switch (value) {
    case "user":
    case "global":
    case "shared":
      return value;
    default:
      return null;
  }
}

export function parseRateLimitHeaders(headers: Headers): RateLimitHeaders {
  return {
    limit: parseNumber(headers.get("x-ratelimit-limit")),
    remaining: parseNumber(headers.get("x-ratelimit-remaining")),
    reset_after_seconds: parseNumber(headers.get("x-ratelimit-reset-after")),
    bucket: headers.get("x-ratelimit-bucket"),
    global: headers.get("x-ratelimit-global") === "true",
    scope: parseScope(headers.get("x-ratelimit-scope")),
    retry_after_seconds: parseNumber(headers.get("retry-after")),
  };
}

/** Body of a 429 response as documented by Discord. */
export type RateLimitedBody = {
  readonly retry_after?: number;
  readonly global?: boolean;
};

/**
 * Milliseconds to wait after a 429. The JSON body carries sub-second precision, so it wins
 * over the integer `Retry-After` header when both are present.
 */
export function retryAfterMsFor429(
  headers: RateLimitHeaders,
  body: RateLimitedBody | null,
): number {
  const fromBody = body?.retry_after;
  if (typeof fromBody === "number" && Number.isFinite(fromBody) && fromBody >= 0) {
    return Math.ceil(fromBody * 1000);
  }
  if (headers.retry_after_seconds !== null && headers.retry_after_seconds >= 0) {
    return Math.ceil(headers.retry_after_seconds * 1000);
  }
  // Discord always sends one of the two; a missing value is treated as a short pause rather
  // than an immediate retry so a malformed response cannot turn into a tight loop.
  return 1_000;
}

/**
 * Earliest time the same route may be requested again after a successful response.
 * When the bucket is exhausted the caller waits for the reset instead of provoking a 429.
 */
export function nextEligibleAfterSuccess(completedAtMs: number, headers: RateLimitHeaders): number {
  if (headers.remaining === 0 && headers.reset_after_seconds !== null) {
    return completedAtMs + Math.ceil(headers.reset_after_seconds * 1000);
  }
  return completedAtMs;
}

/** Route bucket snapshot stored by the Channel Durable Object for diagnostics. */
export type RouteBucketState = {
  readonly route_key: string;
  readonly bucket: string | null;
  readonly limit: number | null;
  readonly remaining: number | null;
  readonly reset_at: number | null;
  readonly observed_at: number;
};

export function routeBucketFromHeaders(
  routeKey: string,
  completedAtMs: number,
  headers: RateLimitHeaders,
): RouteBucketState {
  return {
    route_key: routeKey,
    bucket: headers.bucket,
    limit: headers.limit,
    remaining: headers.remaining,
    reset_at:
      headers.reset_after_seconds === null
        ? null
        : completedAtMs + Math.ceil(headers.reset_after_seconds * 1000),
    observed_at: completedAtMs,
  };
}
