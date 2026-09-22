import { parseSnowflake } from "../domain/ids";
import type { BackfillRange, StartRunRequest } from "../domain/run";
import { channelObjectName, type Env, readPositiveInt } from "../env";

/**
 * Backfill API Worker: stateless authentication, validation and routing.
 * It owns no progress; every stateful call is forwarded to the Channel Durable Object.
 */

const MAX_PAGE_LIMIT = 100;

function json(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json; charset=utf-8" },
  });
}

async function isAuthorized(request: Request, expectedToken: string | undefined): Promise<boolean> {
  if (!expectedToken) return false;
  const header = request.headers.get("authorization") ?? "";
  const prefix = "Bearer ";
  if (!header.startsWith(prefix)) return false;
  const presented = new TextEncoder().encode(header.slice(prefix.length));
  const expected = new TextEncoder().encode(expectedToken);
  if (presented.byteLength !== expected.byteLength) return false;
  return crypto.subtle.timingSafeEqual(presented, expected);
}

type StartBody = {
  readonly guild_id?: unknown;
  readonly channel_id?: unknown;
  readonly range?: { readonly after?: unknown; readonly before?: unknown };
  readonly page_limit?: unknown;
};

type Validation<T> =
  | { readonly ok: true; readonly value: T }
  | { readonly ok: false; readonly error: string };

function validateStart(body: StartBody, defaultPageLimit: number): Validation<StartRunRequest> {
  const guildId = parseSnowflake(body.guild_id);
  if (guildId === undefined) return { ok: false, error: "guild_id must be a Snowflake string" };
  const channelId = parseSnowflake(body.channel_id);
  if (channelId === undefined) return { ok: false, error: "channel_id must be a Snowflake string" };

  const range: BackfillRange = { after: null, before: null };
  let after = range.after;
  let before = range.before;
  if (body.range?.after !== undefined) {
    const parsed = parseSnowflake(body.range.after);
    if (parsed === undefined) return { ok: false, error: "range.after must be a Snowflake string" };
    after = parsed;
  }
  if (body.range?.before !== undefined) {
    const parsed = parseSnowflake(body.range.before);
    if (parsed === undefined)
      return { ok: false, error: "range.before must be a Snowflake string" };
    before = parsed;
  }
  if (after !== null && before !== null && BigInt(after) >= BigInt(before)) {
    return { ok: false, error: "range.after must be smaller than range.before" };
  }

  let pageLimit = defaultPageLimit;
  if (body.page_limit !== undefined) {
    if (
      !Number.isInteger(body.page_limit) ||
      (body.page_limit as number) < 1 ||
      (body.page_limit as number) > MAX_PAGE_LIMIT
    ) {
      return { ok: false, error: `page_limit must be an integer between 1 and ${MAX_PAGE_LIMIT}` };
    }
    pageLimit = body.page_limit as number;
  }

  return {
    ok: true,
    value: {
      guild_id: guildId,
      channel_id: channelId,
      range: { after, before },
      page_limit: pageLimit,
    },
  };
}

function channelStub(env: Env, channelId: string) {
  return env.BACKFILL_CHANNEL.get(
    env.BACKFILL_CHANNEL.idFromName(channelObjectName(env.ENVIRONMENT ?? "dev", channelId)),
  );
}

const RUN_ROUTE = /^\/v1\/backfill\/channels\/([0-9]+)\/runs\/([0-9a-f-]+)$/;
const CHANNEL_ROUTE = /^\/v1\/backfill\/channels\/([0-9]+)$/;
const RESUME_ROUTE = /^\/v1\/backfill\/channels\/([0-9]+)\/resume$/;

export const handleApiRequest = {
  async fetch(request: Request, env: Env): Promise<Response> {
    const url = new URL(request.url);
    if (url.pathname === "/healthz") return json(200, { ok: true });

    if (!(await isAuthorized(request, env.BACKFILL_API_TOKEN))) {
      return json(401, { error: "unauthorized" });
    }

    if (request.method === "POST" && url.pathname === "/v1/backfill/runs") {
      let body: StartBody;
      try {
        body = (await request.json()) as StartBody;
      } catch {
        return json(400, { error: "body must be JSON" });
      }
      const validated = validateStart(
        body,
        readPositiveInt(env.BACKFILL_PAGE_LIMIT, MAX_PAGE_LIMIT),
      );
      if (!validated.ok) return json(400, { error: validated.error });
      const result = await channelStub(env, validated.value.channel_id).start(validated.value);
      if (!result.ok) return json(409, { error: result.reason, run: result.run });
      return json(202, { run: result.run });
    }

    const runMatch = RUN_ROUTE.exec(url.pathname);
    if (request.method === "GET" && runMatch) {
      const [, channelId, runId] = runMatch;
      const run = await channelStub(env, channelId as string).status(runId as string);
      return run === null ? json(404, { error: "run not found" }) : json(200, { run });
    }

    const channelMatch = CHANNEL_ROUTE.exec(url.pathname);
    if (request.method === "GET" && channelMatch) {
      const [, channelId] = channelMatch;
      const run = await channelStub(env, channelId as string).currentRun();
      return run === null ? json(404, { error: "no run for channel" }) : json(200, { run });
    }

    const resumeMatch = RESUME_ROUTE.exec(url.pathname);
    if (request.method === "POST" && resumeMatch) {
      const [, channelId] = resumeMatch;
      const result = await channelStub(env, channelId as string).resume();
      return json(200, result);
    }

    return json(404, { error: "not found" });
  },
} satisfies ExportedHandler<Env>;
