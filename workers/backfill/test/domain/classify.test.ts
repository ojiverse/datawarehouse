import { describe, expect, it } from "vitest";
import { classifyStatus, countsAsInvalidRequest } from "../../src/domain/classify";

describe("classifyStatus", () => {
  it.each([
    [200, "success"],
    [401, "credential_failure"],
    [403, "scope_forbidden"],
    [404, "scope_not_found"],
    [429, "rate_limited"],
    [400, "client_error"],
    [500, "server_error"],
    [502, "server_error"],
  ] as const)("maps %i to %s", (status, expected) => {
    expect(classifyStatus(status)).toBe(expected);
  });
});

describe("countsAsInvalidRequest", () => {
  it("counts 401, 403 and non-shared 429", () => {
    expect(countsAsInvalidRequest(401, null)).toBe(true);
    expect(countsAsInvalidRequest(403, null)).toBe(true);
    expect(countsAsInvalidRequest(429, "user")).toBe(true);
    expect(countsAsInvalidRequest(429, "global")).toBe(true);
    expect(countsAsInvalidRequest(429, null)).toBe(true);
  });

  it("excludes shared-scope 429 and successful responses", () => {
    expect(countsAsInvalidRequest(429, "shared")).toBe(false);
    expect(countsAsInvalidRequest(200, null)).toBe(false);
    expect(countsAsInvalidRequest(500, null)).toBe(false);
  });
});
