import { describe, expect, it } from "vitest";
import { parseUuidV7, type Snowflake, type UuidV7 } from "../../src/domain/ids";
import type { RunRecord } from "../../src/domain/run";
import { executePage } from "../../src/domain/step";
import type { ArchiveObject } from "../../src/observation/archive-object";
import {
  type ArchiveCommitResult,
  ArchiveInvariantViolation,
  NO_FAULTS,
  type ObservationArchive,
} from "../../src/ports";
import { FakeBudget, FakeClock, FakeDiscord, makeMessages, snowflake } from "../helpers/fakes";

class MemoryArchive implements ObservationArchive {
  readonly objects: ArchiveObject[] = [];
  failWith: Error | null = null;
  async commit(object: ArchiveObject): Promise<ArchiveCommitResult> {
    if (this.failWith) throw this.failWith;
    this.objects.push(object);
    return { key: object.key, outcome: "created" };
  }
}

const config = { discord_api_version: "10", capabilities: { message_content: true } };

function run(overrides: Partial<RunRecord> = {}): RunRecord {
  return {
    run_id: "0199a1b2-0000-7000-8000-000000000001" as UuidV7,
    guild_id: snowflake(1),
    channel_id: snowflake(2),
    range: { after: null, before: null },
    page_limit: 3,
    state: "running",
    cursor_before: null,
    pages_archived: 0,
    last_archived: null,
    next_eligible_at: 0,
    attempt: 0,
    terminal_error: null,
    pending_coordination: null,
    created_at: "2026-09-21T00:00:00.000Z",
    updated_at: "2026-09-21T00:00:00.000Z",
    ...overrides,
  };
}

function harness(messages = makeMessages(7)) {
  const clock = new FakeClock();
  const discord = new FakeDiscord(messages, clock);
  const archive = new MemoryArchive();
  const budget = new FakeBudget();
  const ports = { discord, archive, budget, clock, faults: NO_FAULTS };
  return { clock, discord, archive, budget, ports };
}

describe("executePage", () => {
  it("archives a page and advances to the oldest message", async () => {
    const h = harness();
    const outcome = await executePage(h.ports, config, run());
    expect(outcome.kind).toBe("archived");
    if (outcome.kind !== "archived") return;
    expect(h.archive.objects).toHaveLength(1);
    expect(outcome.next_before).toBe(snowflake(1_000_004));
    expect(outcome.completed).toBe(false);
    expect(h.discord.requests[0]).toEqual({ channel_id: snowflake(2), before: null, limit: 3 });
    expect(h.budget.acquires).toBe(1);
    expect(h.budget.reports[0]?.status).toBe(200);
  });

  it("uses the durable cursor as the before parameter", async () => {
    const h = harness();
    await executePage(h.ports, config, run({ cursor_before: snowflake(1_000_004) }));
    expect(h.discord.requests[0]?.before).toBe(snowflake(1_000_004));
  });

  it("completes when the last short page is archived", async () => {
    const h = harness(makeMessages(2));
    const outcome = await executePage(h.ports, config, run());
    expect(outcome).toMatchObject({ kind: "archived", completed: true, next_before: null });
    expect(h.archive.objects).toHaveLength(1);
  });

  it("defers without a request when the budget denies", async () => {
    const h = harness();
    h.budget.decision = { granted: false, reason: "global_ceiling", retry_after_ms: 400 };
    const outcome = await executePage(h.ports, config, run());
    expect(outcome).toMatchObject({ kind: "deferred", reason: "budget", detail: "global_ceiling" });
    expect(h.discord.requests).toHaveLength(0);
    if (outcome.kind === "deferred") expect(outcome.next_eligible_at).toBe(h.clock.nowMs() + 400);
  });

  it("records a 429 as pending coordination carrying the Retry-After pause", async () => {
    const h = harness();
    h.discord.enqueue({
      kind: "status",
      status: 429,
      body: { retry_after: 2.5, global: false },
      headers: { scope: "user" },
    });
    const outcome = await executePage(h.ports, config, run());
    expect(outcome.kind).toBe("coordination_pending");
    if (outcome.kind !== "coordination_pending") return;
    expect(outcome.report).toMatchObject({ status: 429, scope: "user", retry_after_ms: 2500 });
    expect(outcome.deferred_outcome).toMatchObject({ kind: "deferred", reason: "rate_limited" });
    if (outcome.deferred_outcome.kind === "deferred") {
      expect(outcome.deferred_outcome.next_eligible_at).toBe(h.clock.nowMs() + 2500);
    }
    expect(h.archive.objects).toHaveLength(0);
  });

  it("records a 401 as pending coordination with a credential-failure outcome", async () => {
    const h = harness();
    h.discord.enqueue({ kind: "status", status: 401 });
    const outcome = await executePage(h.ports, config, run());
    expect(outcome).toMatchObject({
      kind: "coordination_pending",
      report: { status: 401 },
      deferred_outcome: {
        kind: "terminal",
        error: { kind: "credential_failure", http_status: 401 },
      },
    });
  });

  it("records a 403 as pending coordination and fails 404 directly", async () => {
    const h = harness();
    h.discord.enqueue({ kind: "status", status: 403 }, { kind: "status", status: 404 });
    expect(await executePage(h.ports, config, run())).toMatchObject({
      kind: "coordination_pending",
      deferred_outcome: { kind: "terminal", error: { kind: "scope_inaccessible" } },
    });
    expect(await executePage(h.ports, config, run())).toMatchObject({
      kind: "terminal",
      error: { kind: "scope_not_found" },
    });
  });

  it("retries with backoff on 5xx and transport errors", async () => {
    const h = harness();
    h.discord.enqueue(
      { kind: "status", status: 502 },
      { kind: "throw", message: "connection reset" },
    );
    const first = await executePage(h.ports, config, run({ attempt: 0 }));
    expect(first).toMatchObject({ kind: "retry" });
    if (first.kind === "retry") expect(first.next_eligible_at).toBe(h.clock.nowMs() + 2_000);
    const beforeSecond = h.clock.nowMs();
    const second = await executePage(h.ports, config, run({ attempt: 1 }));
    expect(second).toMatchObject({ kind: "retry" });
    if (second.kind === "retry")
      expect(second.next_eligible_at).toBeGreaterThanOrEqual(beforeSecond + 4_000);
    expect(h.archive.objects).toHaveLength(0);
  });

  it("retries when the archive commit fails and never reports progress", async () => {
    const h = harness();
    h.archive.failWith = new Error("r2 unavailable");
    const outcome = await executePage(h.ports, config, run());
    expect(outcome.kind).toBe("retry");
  });

  it("stops on an archive invariant violation", async () => {
    const h = harness();
    h.archive.failWith = new ArchiveInvariantViolation("hash mismatch");
    const outcome = await executePage(h.ports, config, run());
    expect(outcome).toMatchObject({
      kind: "terminal",
      error: { kind: "archive_invariant_violation" },
    });
  });

  it("waits for the route bucket reset when the last request exhausted it", async () => {
    const h = harness();
    h.discord.setSuccessHeaders({ remaining: 0, reset_after_seconds: 1.2, bucket: "b1", limit: 5 });
    const outcome = await executePage(h.ports, config, run());
    if (outcome.kind !== "archived") throw new Error("expected archived");
    expect(outcome.next_eligible_at).toBe(h.clock.nowMs() + 1200);
    expect(outcome.route_bucket).toMatchObject({ bucket: "b1", remaining: 0, limit: 5 });
  });

  it("gives every fetched page a fresh Observation ID", async () => {
    const h = harness();
    await executePage(h.ports, config, run());
    await executePage(h.ports, config, run());
    const [a, b] = h.archive.objects;
    expect(a?.observation_id).not.toBe(b?.observation_id);
    expect(a?.key).not.toBe(b?.key);
  });

  it("treats a non-array 2xx body as a terminal contract break", async () => {
    const h = harness();
    h.discord.enqueue({ kind: "status", status: 200, body: { unexpected: true } });
    const outcome = await executePage(h.ports, config, run());
    expect(outcome).toMatchObject({ kind: "terminal", error: { kind: "invalid_request" } });
  });

  it("keeps a 2xx page when the advisory budget report fails", async () => {
    const h = harness();
    h.budget.reportFailure = new Error("budget unreachable");
    const outcome = await executePage(h.ports, config, run());
    expect(outcome.kind).toBe("archived");
  });
});

describe("executePage — fail-closed budget coordination", () => {
  it.each([
    [401, { kind: "terminal", error: { kind: "credential_failure" } }],
    [403, { kind: "terminal", error: { kind: "scope_inaccessible" } }],
    [429, { kind: "deferred", reason: "rate_limited" }],
  ] as const)(
    "records a %i response as pending coordination without reporting it itself",
    async (status, deferred) => {
      const h = harness();
      h.discord.enqueue({
        kind: "status",
        status,
        body: status === 429 ? { retry_after: 1.5 } : {},
        headers: status === 429 ? { scope: "user" } : {},
      });
      const outcome = await executePage(h.ports, config, run({ attempt: 0 }));
      expect(outcome.kind).toBe("coordination_pending");
      if (outcome.kind !== "coordination_pending") return;
      expect(outcome.report).toMatchObject({ status });
      expect(parseUuidV7(outcome.report.report_id)).toBe(outcome.report.report_id);
      expect(outcome.deferred_outcome).toMatchObject(deferred);
      expect(outcome.retry).toBe(false);
      expect(h.budget.reports).toHaveLength(0);
      expect(h.archive.objects).toHaveLength(0);
    },
  );

  it("fixes the report identity before the request, so a refetch is a different report", async () => {
    const h = harness();
    h.discord.enqueue({ kind: "status", status: 401 }, { kind: "status", status: 401 });
    const first = await executePage(h.ports, config, run());
    const second = await executePage(h.ports, config, run());
    if (first.kind !== "coordination_pending" || second.kind !== "coordination_pending") {
      throw new Error("expected pending coordination");
    }
    expect(first.report.report_id).not.toBe(second.report.report_id);
  });

  it("tags advisory 2xx reports with the same identity scheme", async () => {
    const h = harness();
    await executePage(h.ports, config, run());
    expect(h.budget.reports).toHaveLength(1);
    expect(parseUuidV7(h.budget.reports[0]?.report_id)).toBeDefined();
  });
});

describe("executePage range handling", () => {
  it("starts from range.before and stops at range.after", async () => {
    const h = harness(makeMessages(10));
    const r = run({
      range: { after: snowflake(1_000_003), before: snowflake(1_000_008) },
      page_limit: 3,
    });
    const first = await executePage(h.ports, config, r);
    expect(h.discord.requests[0]?.before).toBe(snowflake(1_000_008));
    if (first.kind !== "archived") throw new Error("expected archived");
    expect(first.next_before).toBe(snowflake(1_000_005));
    const second = await executePage(h.ports, config, {
      ...r,
      cursor_before: first.next_before as Snowflake,
    });
    expect(second).toMatchObject({ kind: "archived", completed: true });
  });
});
