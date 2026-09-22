import { describe, expect, it } from "vitest";
import {
  EMPTY_RATE_LIMIT_HEADERS,
  nextEligibleAfterSuccess,
  parseRateLimitHeaders,
  retryAfterMsFor429,
  routeBucketFromHeaders,
} from "../../src/domain/rate-limit";

describe("parseRateLimitHeaders", () => {
  it("reads every documented header", () => {
    const headers = new Headers({
      "X-RateLimit-Limit": "5",
      "X-RateLimit-Remaining": "0",
      "X-RateLimit-Reset-After": "1.250",
      "X-RateLimit-Bucket": "abcd1234",
      "X-RateLimit-Global": "true",
      "X-RateLimit-Scope": "shared",
      "Retry-After": "2",
    });
    expect(parseRateLimitHeaders(headers)).toEqual({
      limit: 5,
      remaining: 0,
      reset_after_seconds: 1.25,
      bucket: "abcd1234",
      global: true,
      scope: "shared",
      retry_after_seconds: 2,
    });
  });

  it("tolerates missing or malformed headers", () => {
    expect(parseRateLimitHeaders(new Headers({ "X-RateLimit-Remaining": "n/a" }))).toEqual(
      EMPTY_RATE_LIMIT_HEADERS,
    );
  });
});

describe("retryAfterMsFor429", () => {
  it("prefers the sub-second body value over the header", () => {
    expect(
      retryAfterMsFor429(
        { ...EMPTY_RATE_LIMIT_HEADERS, retry_after_seconds: 3 },
        { retry_after: 0.75 },
      ),
    ).toBe(750);
  });

  it("falls back to the Retry-After header", () => {
    expect(retryAfterMsFor429({ ...EMPTY_RATE_LIMIT_HEADERS, retry_after_seconds: 3 }, null)).toBe(
      3000,
    );
  });

  it("never returns an immediate retry", () => {
    expect(retryAfterMsFor429(EMPTY_RATE_LIMIT_HEADERS, {})).toBeGreaterThan(0);
  });
});

describe("nextEligibleAfterSuccess", () => {
  it("waits for the reset when the bucket is exhausted", () => {
    expect(
      nextEligibleAfterSuccess(10_000, {
        ...EMPTY_RATE_LIMIT_HEADERS,
        remaining: 0,
        reset_after_seconds: 1.5,
      }),
    ).toBe(11_500);
  });

  it("does not wait while requests remain", () => {
    expect(
      nextEligibleAfterSuccess(10_000, {
        ...EMPTY_RATE_LIMIT_HEADERS,
        remaining: 2,
        reset_after_seconds: 1.5,
      }),
    ).toBe(10_000);
  });
});

describe("routeBucketFromHeaders", () => {
  it("materialises reset_at from reset-after", () => {
    const bucket = routeBucketFromHeaders("route", 10_000, {
      ...EMPTY_RATE_LIMIT_HEADERS,
      bucket: "b",
      limit: 5,
      remaining: 4,
      reset_after_seconds: 2,
    });
    expect(bucket).toEqual({
      route_key: "route",
      bucket: "b",
      limit: 5,
      remaining: 4,
      reset_at: 12_000,
      observed_at: 10_000,
    });
  });
});
