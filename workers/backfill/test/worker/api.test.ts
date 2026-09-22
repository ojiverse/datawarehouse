import { env, runInDurableObject, SELF } from "cloudflare:test";
import { describe, expect, it } from "vitest";
import type { BackfillChannelDurableObject } from "../../src/do/backfill-channel";
import { channelObjectName } from "../../src/env";
import { FakeBudget, FakeClock, FakeDiscord, makeMessages } from "../helpers/fakes";

const AUTH = { authorization: "Bearer test-api-token", "content-type": "application/json" };

function post(path: string, body: unknown, headers: Record<string, string> = AUTH) {
  return SELF.fetch(`https://backfill.test${path}`, {
    method: "POST",
    headers,
    body: JSON.stringify(body),
  });
}

describe("Backfill API Worker", () => {
  it("rejects missing or wrong bearer tokens", async () => {
    expect((await SELF.fetch("https://backfill.test/v1/backfill/channels/1")).status).toBe(401);
    expect(
      (
        await SELF.fetch("https://backfill.test/v1/backfill/channels/1", {
          headers: { authorization: "Bearer nope" },
        })
      ).status,
    ).toBe(401);
  });

  it("validates the start request", async () => {
    expect((await post("/v1/backfill/runs", { guild_id: "x", channel_id: "2" })).status).toBe(400);
    expect(
      (
        await post("/v1/backfill/runs", {
          guild_id: "1",
          channel_id: "2",
          range: { after: "9", before: "3" },
        })
      ).status,
    ).toBe(400);
    expect(
      (await post("/v1/backfill/runs", { guild_id: "1", channel_id: "2", page_limit: 101 })).status,
    ).toBe(400);
  });

  it("starts a run, exposes its status and refuses a concurrent run on the same channel", async () => {
    const channelId = "424242424242424242";
    const clock = new FakeClock();
    const stub = env.BACKFILL_CHANNEL.get(
      env.BACKFILL_CHANNEL.idFromName(channelObjectName("dev", channelId)),
    );
    await runInDurableObject(stub, (instance: BackfillChannelDurableObject) => {
      instance.useTestPorts({
        discord: new FakeDiscord(makeMessages(3), clock),
        budget: new FakeBudget(),
        clock,
      });
    });

    const started = await post("/v1/backfill/runs", {
      guild_id: "1",
      channel_id: channelId,
      page_limit: 100,
    });
    expect(started.status).toBe(202);
    const { run } = (await started.json()) as { run: { run_id: string; state: string } };
    expect(run.state).toBe("running");

    const status = await SELF.fetch(
      `https://backfill.test/v1/backfill/channels/${channelId}/runs/${run.run_id}`,
      { headers: AUTH },
    );
    expect(status.status).toBe(200);

    const conflict = await post("/v1/backfill/runs", { guild_id: "1", channel_id: channelId });
    expect(conflict.status).toBe(409);

    const current = await SELF.fetch(`https://backfill.test/v1/backfill/channels/${channelId}`, {
      headers: AUTH,
    });
    expect(((await current.json()) as { run: { run_id: string } }).run.run_id).toBe(run.run_id);
  });

  it("returns 404 for unknown runs and channels", async () => {
    const missing = await SELF.fetch(
      "https://backfill.test/v1/backfill/channels/777/runs/0199a1b2-0000-7000-8000-000000000001",
      {
        headers: AUTH,
      },
    );
    expect(missing.status).toBe(404);
    expect(
      (await SELF.fetch("https://backfill.test/v1/backfill/channels/777", { headers: AUTH }))
        .status,
    ).toBe(404);
  });
});
