import {
  env,
  evictDurableObject,
  runDurableObjectAlarm,
  runInDurableObject,
} from "cloudflare:test";
import { beforeEach, describe, expect, it } from "vitest";
import type { BackfillChannelDurableObject, StartResult } from "../../src/do/backfill-channel";
import type { StartRunRequest } from "../../src/domain/run";
import { channelObjectName } from "../../src/env";
import { InjectedCrash } from "../../src/ports";
import {
  type DiscordMessage,
  FakeBudget,
  FakeClock,
  FakeDiscord,
  makeMessages,
  OneShotFault,
  snowflake,
} from "../helpers/fakes";
import { purgeObservations } from "../helpers/r2";

const ARCHIVE_PREFIX = "observations/v1/source=http_backfill/";

beforeEach(purgeObservations);

type Harness = {
  readonly stub: DurableObjectStub<BackfillChannelDurableObject>;
  readonly clock: FakeClock;
  readonly discord: FakeDiscord;
  readonly budget: FakeBudget;
  readonly faults: OneShotFault;
  /** Re-installs the fakes (needed after an eviction, which rebuilds the instance). */
  install(): Promise<void>;
  start(overrides?: Partial<StartRunRequest>): Promise<StartResult>;
  alarm(): Promise<boolean>;
  alarmDirect(): Promise<void>;
  scheduledAlarm(): Promise<number | null>;
};

async function harness(
  channelId: string,
  messages: readonly DiscordMessage[] = makeMessages(7),
): Promise<Harness> {
  const clock = new FakeClock();
  const discord = new FakeDiscord(messages, clock);
  const budget = new FakeBudget();
  const faults = new OneShotFault(null);
  const stub = env.BACKFILL_CHANNEL.get(
    env.BACKFILL_CHANNEL.idFromName(channelObjectName("dev", channelId)),
  );
  const h: Harness = {
    stub,
    clock,
    discord,
    budget,
    faults,
    async install() {
      await runInDurableObject(stub, (instance: BackfillChannelDurableObject) => {
        instance.useTestPorts({ discord, budget, clock, faults });
      });
    },
    async start(overrides = {}) {
      return await stub.start({
        guild_id: snowflake(1),
        channel_id: snowflake(channelId),
        range: { after: null, before: null },
        page_limit: 3,
        ...overrides,
      });
    },
    alarm() {
      return runDurableObjectAlarm(stub);
    },
    alarmDirect() {
      return runInDurableObject(stub, (instance: BackfillChannelDurableObject) => instance.alarm());
    },
    scheduledAlarm() {
      return runInDurableObject(stub, (_instance: BackfillChannelDurableObject, state) =>
        state.storage.getAlarm(),
      );
    },
  };
  await h.install();
  return h;
}

async function archived() {
  const listed = await env.OBSERVATIONS.list({
    prefix: ARCHIVE_PREFIX,
    include: ["customMetadata"],
  });
  return listed.objects;
}

async function currentRun(h: Harness) {
  const run = await h.stub.currentRun();
  if (run === null) throw new Error("no run");
  return run;
}

describe("BackfillChannelDurableObject — happy path", () => {
  it("crawls newest→oldest across bounded alarms and completes from durable progress", async () => {
    const h = await harness("100", makeMessages(20));
    const started = await h.start();
    expect(started.ok).toBe(true);
    expect(await h.scheduledAlarm()).not.toBeNull();

    expect(await h.alarm()).toBe(true);
    let run = await currentRun(h);
    expect(run.state).toBe("running");
    expect(run.pages_archived).toBe(5);
    expect(run.cursor_before).toBe(snowflake(1_000_005));
    expect(await h.scheduledAlarm()).not.toBeNull();

    expect(await h.alarm()).toBe(true);
    run = await currentRun(h);
    expect(run.state).toBe("completed");
    expect(run.pages_archived).toBe(7);
    expect(run.last_archived).not.toBeNull();
    expect(await h.scheduledAlarm()).toBeNull();

    expect(h.discord.requests.map((r) => r.before)).toEqual([
      null,
      snowflake(1_000_017),
      snowflake(1_000_014),
      snowflake(1_000_011),
      snowflake(1_000_008),
      snowflake(1_000_005),
      snowflake(1_000_002),
    ]);
    const objects = await archived();
    expect(objects).toHaveLength(7);
    expect(objects.some((o) => o.key === run.last_archived?.archive_key)).toBe(true);
  });

  it("refuses a second active run on the same channel", async () => {
    const h = await harness("101");
    const first = await h.start();
    const second = await h.start();
    expect(first.ok).toBe(true);
    expect(second).toMatchObject({ ok: false, reason: "active_run_exists" });
  });

  it("tolerates duplicate alarm execution", async () => {
    const h = await harness("102");
    await h.start();
    await h.alarmDirect();
    const requestsAfterFirst = h.discord.requests.length;
    await h.alarmDirect();
    await h.alarmDirect();
    expect(h.discord.requests.length).toBe(requestsAfterFirst);
    expect((await currentRun(h)).state).toBe("completed");
    expect(await archived()).toHaveLength(3);
  });

  it("bounds a 'before' range and honours the 'after' cut", async () => {
    const h = await harness("103", makeMessages(10));
    await h.start({ range: { after: snowflake(1_000_003), before: snowflake(1_000_008) } });
    await h.alarm();
    const run = await currentRun(h);
    expect(run.state).toBe("completed");
    expect(h.discord.requests.map((r) => r.before)).toEqual([
      snowflake(1_000_008),
      snowflake(1_000_005),
    ]);
    expect(run.pages_archived).toBe(2);
  });
});

describe("BackfillChannelDurableObject — fault injection", () => {
  it("crash after HTTP fetch / before R2 write: nothing archived, nothing advanced, rerun archives once", async () => {
    const h = await harness("200");
    await h.start();
    h.faults.arm("after_fetch_before_archive");
    await expect(h.alarm()).rejects.toBeInstanceOf(InjectedCrash);

    expect(await archived()).toHaveLength(0);
    expect((await currentRun(h)).pages_archived).toBe(0);

    await h.stub.resume();
    expect(await h.alarm()).toBe(true);
    const run = await currentRun(h);
    expect(run.state).toBe("completed");
    expect(run.pages_archived).toBe(3);
    expect(await archived()).toHaveLength(3);
    expect(h.discord.requests).toHaveLength(4);
  });

  it("crash after R2 write / before progress: the page is re-fetched as a new Observation", async () => {
    const h = await harness("201");
    await h.start();
    h.faults.arm("after_archive_before_progress");
    await expect(h.alarm()).rejects.toBeInstanceOf(InjectedCrash);

    expect(await archived()).toHaveLength(1);
    expect((await currentRun(h)).pages_archived).toBe(0);

    await h.stub.resume();
    await h.alarm();
    const run = await currentRun(h);
    expect(run.state).toBe("completed");
    expect(run.pages_archived).toBe(3);

    const objects = await archived();
    expect(objects).toHaveLength(4);
    const firstPageHashes = objects.map((o) => o.customMetadata?.payload_sha256);
    const duplicates = firstPageHashes.filter((hash, i) => firstPageHashes.indexOf(hash) !== i);
    expect(duplicates).toHaveLength(1);
    const ids = new Set(objects.map((o) => o.customMetadata?.observation_id));
    expect(ids.size).toBe(4);
    expect(h.discord.requests.map((r) => r.before)).toEqual([
      null,
      null,
      snowflake(1_000_004),
      snowflake(1_000_001),
    ]);
  });

  it("crash after progress / before alarm: the restarted object re-arms itself and does not duplicate", async () => {
    const h = await harness("202");
    await h.start();
    h.faults.arm("after_progress_before_alarm");
    await expect(h.alarm()).rejects.toBeInstanceOf(InjectedCrash);

    expect(await archived()).toHaveLength(1);
    expect((await currentRun(h)).pages_archived).toBe(1);
    expect(await h.scheduledAlarm()).toBeNull();

    await evictDurableObject(h.stub);
    expect(await h.scheduledAlarm()).not.toBeNull();
    await h.install();
    await h.alarm();

    const run = await currentRun(h);
    expect(run.state).toBe("completed");
    expect(run.pages_archived).toBe(3);
    expect(await archived()).toHaveLength(3);
    expect(h.discord.requests.map((r) => r.before)).toEqual([
      null,
      snowflake(1_000_004),
      snowflake(1_000_001),
    ]);
  });

  it("HTTP 429: waits for Retry-After without archiving, then continues", async () => {
    const h = await harness("203");
    await h.start();
    h.discord.enqueue({
      kind: "status",
      status: 429,
      body: { retry_after: 2.5 },
      headers: { scope: "user" },
    });

    await h.alarm();
    let run = await currentRun(h);
    expect(run.state).toBe("waiting");
    expect(run.pages_archived).toBe(0);
    expect(run.next_eligible_at).toBe(h.clock.nowMs() + 2_500);
    expect(await h.scheduledAlarm()).toBe(run.next_eligible_at);
    expect(await archived()).toHaveLength(0);

    await h.alarmDirect();
    expect(h.discord.requests).toHaveLength(1);

    h.clock.advance(2_600);
    await h.alarm();
    run = await currentRun(h);
    expect(run.state).toBe("completed");
    expect(await archived()).toHaveLength(3);
    expect(h.budget.reports[0]).toMatchObject({ status: 429, scope: "user" });
  });

  it("runtime restart mid-run: progress and alarm survive eviction", async () => {
    const h = await harness("204", makeMessages(20));
    await h.start();
    await h.alarm();
    expect((await currentRun(h)).pages_archived).toBe(5);

    await evictDurableObject(h.stub);
    await h.install();
    expect(await h.scheduledAlarm()).not.toBeNull();
    await h.alarm();

    const run = await currentRun(h);
    expect(run.state).toBe("completed");
    expect(run.pages_archived).toBe(7);
    expect(await archived()).toHaveLength(7);
  });

  it("restart re-discovery: a lost alarm is re-armed from SQLite alone", async () => {
    const h = await harness("205");
    await h.start();
    await runInDurableObject(h.stub, (_instance: BackfillChannelDurableObject, state) =>
      state.storage.deleteAlarm(),
    );
    expect(await h.scheduledAlarm()).toBeNull();

    await evictDurableObject(h.stub);
    expect(await h.scheduledAlarm()).not.toBeNull();
    await h.install();
    await h.alarm();
    expect((await currentRun(h)).state).toBe("completed");
  });

  it("transient 5xx: retries with backoff and eventually completes", async () => {
    const h = await harness("206");
    await h.start();
    h.discord.enqueue({ kind: "status", status: 503 }, { kind: "throw", message: "reset" });

    await h.alarm();
    let run = await currentRun(h);
    expect(run.attempt).toBe(1);
    expect(run.state).toBe("running");
    h.clock.set(run.next_eligible_at);
    await h.alarm();
    run = await currentRun(h);
    expect(run.attempt).toBe(2);
    h.clock.set(run.next_eligible_at);
    await h.alarm();
    run = await currentRun(h);
    expect(run.state).toBe("completed");
    expect(run.attempt).toBe(0);
    expect(await archived()).toHaveLength(3);
  });
});

describe("BackfillChannelDurableObject — classification and archive safety", () => {
  it("401 halts the run and reports the credential failure", async () => {
    const h = await harness("300");
    await h.start();
    h.discord.enqueue({ kind: "status", status: 401 });
    await h.alarm();
    const run = await currentRun(h);
    expect(run.state).toBe("halted");
    expect(run.terminal_error).toMatchObject({ kind: "credential_failure", http_status: 401 });
    expect(await h.scheduledAlarm()).toBeNull();
    expect(h.budget.reports[0]?.status).toBe(401);
  });

  it("a budget-level credential halt stops the run without issuing requests", async () => {
    const h = await harness("301");
    await h.start();
    h.budget.decision = { granted: false, reason: "credential_halt", retry_after_ms: 600_000 };
    await h.alarm();
    expect((await currentRun(h)).state).toBe("halted");
    expect(h.discord.requests).toHaveLength(0);
  });

  it("403 fails the scope terminally and never retries", async () => {
    const h = await harness("302");
    await h.start();
    h.discord.enqueue({ kind: "status", status: 403 });
    await h.alarm();
    const run = await currentRun(h);
    expect(run.state).toBe("failed");
    expect(run.terminal_error?.kind).toBe("scope_inaccessible");
    await h.alarmDirect();
    expect(h.discord.requests).toHaveLength(1);
  });

  it("re-backfilling the same range appends new Observations and leaves existing objects untouched", async () => {
    const h = await harness("303");
    await h.start();
    await h.alarm();
    const first = await archived();
    expect(first).toHaveLength(3);

    const second = await h.start();
    expect(second.ok).toBe(true);
    await h.alarm();
    expect((await currentRun(h)).state).toBe("completed");

    const all = await archived();
    expect(all).toHaveLength(6);
    for (const original of first) {
      const still = all.find((o) => o.key === original.key);
      expect(still?.etag).toBe(original.etag);
    }
    expect(new Set(all.map((o) => o.customMetadata?.observation_id)).size).toBe(6);
  });

  it("records the terminal state and remains queryable by run id", async () => {
    const h = await harness("304");
    const started = await h.start();
    if (!started.ok) throw new Error("start failed");
    await h.alarm();
    const byId = await h.stub.status(started.run.run_id);
    expect(byId?.state).toBe("completed");
    expect(await h.stub.status("0199a1b2-0000-7000-8000-00000000dead")).toBeNull();
    expect(await h.stub.listRuns()).toHaveLength(1);
  });
});
