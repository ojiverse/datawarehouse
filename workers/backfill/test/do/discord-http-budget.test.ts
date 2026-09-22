import { env, runInDurableObject } from "cloudflare:test";
import { describe, expect, it } from "vitest";
import type { DiscordHttpBudgetDurableObject } from "../../src/do/discord-http-budget";
import type { RequestReport } from "../../src/ports";
import { FakeClock, reportId } from "../helpers/fakes";

function report(overrides: Partial<RequestReport>): RequestReport {
  return {
    report_id: reportId(),
    status: 200,
    scope: null,
    global: false,
    retry_after_ms: null,
    ...overrides,
  };
}

async function budgetWithClock(name: string) {
  const clock = new FakeClock();
  const stub = env.DISCORD_HTTP_BUDGET.get(env.DISCORD_HTTP_BUDGET.idFromName(name));
  await runInDurableObject(stub, (instance: DiscordHttpBudgetDurableObject) => {
    instance.useClock(() => clock.nowMs());
  });
  return { stub, clock };
}

describe("DiscordHttpBudgetDurableObject", () => {
  it("grants up to the global ceiling per second and then denies with a retry hint", async () => {
    const { stub, clock } = await budgetWithClock("ceiling");
    for (let i = 0; i < 50; i++) {
      expect(await stub.acquire()).toEqual({ granted: true });
    }
    const denied = await stub.acquire();
    expect(denied).toMatchObject({ granted: false, reason: "global_ceiling" });
    if (!denied.granted) expect(denied.retry_after_ms).toBeGreaterThan(0);

    clock.advance(1_000);
    expect(await stub.acquire()).toEqual({ granted: true });
  });

  it("denies once the invalid-request budget safety threshold is reached", async () => {
    const { stub, clock } = await budgetWithClock("invalid");
    // vitest.config.ts sets DISCORD_INVALID_REQUEST_BUDGET=100; 90 % of it is the safety threshold.
    for (let i = 0; i < 90; i++) {
      await stub.report(report({ status: 403 }));
    }
    expect(await stub.acquire()).toMatchObject({
      granted: false,
      reason: "invalid_request_budget",
    });

    clock.advance(600_000 + 1);
    expect(await stub.acquire()).toEqual({ granted: true });
  });

  it("does not count shared-scope 429 as invalid", async () => {
    const { stub } = await budgetWithClock("shared");
    await stub.report(report({ status: 429, scope: "shared", retry_after_ms: 500 }));
    await stub.report(report({ status: 429, scope: "user", retry_after_ms: 500 }));
    const snapshot = await stub.snapshot();
    expect(snapshot.invalid_in_window).toBe(1);
    expect(snapshot.global_retry_until).toBeNull();
  });

  it("pauses everyone after a global 429", async () => {
    const { stub, clock } = await budgetWithClock("global");
    await stub.report(
      report({ status: 429, scope: "global", global: true, retry_after_ms: 3_000 }),
    );
    const denied = await stub.acquire();
    expect(denied).toMatchObject({
      granted: false,
      reason: "global_retry_after",
      retry_after_ms: 3_000,
    });
    clock.advance(3_000);
    expect(await stub.acquire()).toEqual({ granted: true });
  });

  it("halts every caller after a credential failure until an operator clears it", async () => {
    const { stub } = await budgetWithClock("credential");
    await stub.report(report({ status: 401 }));
    expect(await stub.acquire()).toMatchObject({ granted: false, reason: "credential_halt" });
    await stub.clearCredentialHalt();
    expect(await stub.acquire()).toEqual({ granted: true });
  });

  it("ignores a re-sent report with the same report_id (invalid count and pause unchanged)", async () => {
    const { stub, clock } = await budgetWithClock("idempotent");
    const forbidden = report({ status: 403 });
    await stub.report(forbidden);
    await stub.report(forbidden);
    await stub.report(forbidden);
    expect((await stub.snapshot()).invalid_in_window).toBe(1);

    const globalPause = report({
      status: 429,
      scope: "global",
      global: true,
      retry_after_ms: 3_000,
    });
    await stub.report(globalPause);
    const until = (await stub.snapshot()).global_retry_until;
    clock.advance(1_000);
    await stub.report(globalPause);
    expect((await stub.snapshot()).global_retry_until).toBe(until);
    expect((await stub.snapshot()).invalid_in_window).toBe(2);

    await stub.report(report({ status: 403 }));
    expect((await stub.snapshot()).invalid_in_window).toBe(3);
  });

  it("forgets processed report ids once they fall out of the invalid-request window", async () => {
    const { stub, clock } = await budgetWithClock("prune");
    const forbidden = report({ status: 403 });
    await stub.report(forbidden);
    clock.advance(600_000 + 1);
    await stub.report(forbidden);
    expect((await stub.snapshot()).invalid_in_window).toBe(1);
  });
});
