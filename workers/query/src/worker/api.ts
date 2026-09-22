import type { R2SqlPort } from "../adapters/r2sql-client";
import { R2SqlError } from "../adapters/r2sql-client";
import {
  type CanonicalMessage,
  type ChannelMessageCount,
  MalformedRowError,
  mapRowToCanonicalMessage,
  mapRowToChannelMessageCount,
} from "../domain/message";
import {
  buildChannelMessageCountQuery,
  buildGetMessageByIdQuery,
  buildListMessagesQuery,
  parseSnowflake,
  parseTimestamp,
  type Snowflake,
} from "../domain/query";
import { type Env, readPositiveInt } from "../env";

/**
 * Query API Worker: minimal REST surface over R2 SQL for the first-MVP
 * required queries (Issue #41). Query semantics (which columns, which
 * filters, best-known-state fields) follow the Canonical Message domain
 * model; this module owns only Cloudflare-specific auth, routing and
 * request/response shaping.
 */

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

function tableRef(env: Env) {
  return { namespace: env.R2_SQL_NAMESPACE, table: env.R2_SQL_TABLE };
}

type ListParams = {
  readonly range: { readonly after?: Date; readonly before?: Date } | undefined;
  readonly page: { readonly limit: number; readonly afterMessageId?: Snowflake };
};

/** Parses `created_after` / `created_before` / `limit` / `after` query params shared by list routes. */
function parseListParams(
  url: URL,
  env: Env,
):
  | { readonly ok: true; readonly value: ListParams }
  | { readonly ok: false; readonly error: string } {
  const createdAfterRaw = url.searchParams.get("created_after");
  const createdBeforeRaw = url.searchParams.get("created_before");
  let after: Date | undefined;
  let before: Date | undefined;
  if (createdAfterRaw !== null) {
    after = parseTimestamp(createdAfterRaw);
    if (!after) return { ok: false, error: "created_after must be an ISO 8601 timestamp" };
  }
  if (createdBeforeRaw !== null) {
    before = parseTimestamp(createdBeforeRaw);
    if (!before) return { ok: false, error: "created_before must be an ISO 8601 timestamp" };
  }

  const maxLimit = readPositiveInt(env.QUERY_MAX_PAGE_LIMIT, 500);
  const defaultLimit = readPositiveInt(env.QUERY_DEFAULT_PAGE_LIMIT, 100);
  let limit = defaultLimit;
  const limitRaw = url.searchParams.get("limit");
  if (limitRaw !== null) {
    const parsed = Number(limitRaw);
    if (!Number.isInteger(parsed) || parsed < 1 || parsed > maxLimit) {
      return { ok: false, error: `limit must be an integer between 1 and ${maxLimit}` };
    }
    limit = parsed;
  }

  let afterMessageId: Snowflake | undefined;
  const afterMessageIdRaw = url.searchParams.get("after");
  if (afterMessageIdRaw !== null) {
    const parsed = parseSnowflake(afterMessageIdRaw);
    if (!parsed) return { ok: false, error: "after must be a Snowflake string" };
    afterMessageId = parsed;
  }

  const range =
    after || before ? { ...(after ? { after } : {}), ...(before ? { before } : {}) } : undefined;
  const page = { limit, ...(afterMessageId ? { afterMessageId } : {}) };
  return { ok: true, value: { range, page } };
}

async function queryMessages(
  sql: R2SqlPort,
  statement: string,
): Promise<
  | { readonly ok: true; readonly rows: readonly CanonicalMessage[] }
  | { readonly ok: false; readonly status: number; readonly error: string }
> {
  try {
    const rows = await sql.query(statement);
    return { ok: true, rows: rows.map(mapRowToCanonicalMessage) };
  } catch (err) {
    if (err instanceof R2SqlError) return { ok: false, status: 502, error: err.message };
    if (err instanceof MalformedRowError) return { ok: false, status: 502, error: err.message };
    throw err;
  }
}

const MESSAGE_ROUTE = /^\/v1\/messages\/([0-9]+)$/;
const CHANNEL_MESSAGES_ROUTE = /^\/v1\/channels\/([0-9]+)\/messages$/;
const AUTHOR_MESSAGES_ROUTE = /^\/v1\/authors\/([0-9]+)\/messages$/;
const CHANNEL_COUNT_ROUTE = /^\/v1\/channels\/messages\/count$/;

/**
 * Routes one incoming request against the Canonical Store via `sql`.
 * Exported as a plain function (rather than only via `ExportedHandler`) so
 * unit tests can inject a fake `R2SqlPort` without any network I/O.
 */
export async function route(request: Request, env: Env, sql: R2SqlPort): Promise<Response> {
  const url = new URL(request.url);
  if (url.pathname === "/healthz") return json(200, { ok: true });

  if (!(await isAuthorized(request, env.QUERY_API_TOKEN))) {
    return json(401, { error: "unauthorized" });
  }
  if (request.method !== "GET") return json(405, { error: "method not allowed" });

  const table = tableRef(env);

  const messageMatch = MESSAGE_ROUTE.exec(url.pathname);
  if (messageMatch) {
    const messageId = parseSnowflake(messageMatch[1]);
    if (!messageId) return json(400, { error: "message_id must be a Snowflake string" });
    const result = await queryMessages(sql, buildGetMessageByIdQuery(table, messageId));
    if (!result.ok) return json(result.status, { error: result.error });
    if (result.rows.length === 0) return json(404, { error: "message not found" });
    return json(200, { message: result.rows[0] });
  }

  const channelMatch = CHANNEL_MESSAGES_ROUTE.exec(url.pathname);
  if (channelMatch) {
    const channelId = parseSnowflake(channelMatch[1]);
    if (!channelId) return json(400, { error: "channel_id must be a Snowflake string" });
    const parsed = parseListParams(url, env);
    if (!parsed.ok) return json(400, { error: parsed.error });
    const query = buildListMessagesQuery(
      table,
      "channel_id",
      channelId,
      parsed.value.range,
      parsed.value.page,
    );
    const result = await queryMessages(sql, query);
    if (!result.ok) return json(result.status, { error: result.error });
    return json(200, { messages: result.rows });
  }

  const authorMatch = AUTHOR_MESSAGES_ROUTE.exec(url.pathname);
  if (authorMatch) {
    const authorId = parseSnowflake(authorMatch[1]);
    if (!authorId) return json(400, { error: "author_id must be a Snowflake string" });
    const parsed = parseListParams(url, env);
    if (!parsed.ok) return json(400, { error: parsed.error });
    const query = buildListMessagesQuery(
      table,
      "author_id",
      authorId,
      parsed.value.range,
      parsed.value.page,
    );
    const result = await queryMessages(sql, query);
    if (!result.ok) return json(result.status, { error: result.error });
    return json(200, { messages: result.rows });
  }

  if (CHANNEL_COUNT_ROUTE.test(url.pathname)) {
    const maxLimit = readPositiveInt(env.QUERY_MAX_PAGE_LIMIT, 500);
    let limit = maxLimit;
    const limitRaw = url.searchParams.get("limit");
    if (limitRaw !== null) {
      const parsed = Number(limitRaw);
      if (!Number.isInteger(parsed) || parsed < 1 || parsed > maxLimit) {
        return json(400, { error: `limit must be an integer between 1 and ${maxLimit}` });
      }
      limit = parsed;
    }
    try {
      const rows = await sql.query(buildChannelMessageCountQuery(table, limit));
      const counts: readonly ChannelMessageCount[] = rows.map(mapRowToChannelMessageCount);
      return json(200, { channel_message_counts: counts });
    } catch (err) {
      if (err instanceof R2SqlError || err instanceof MalformedRowError) {
        return json(502, { error: err.message });
      }
      throw err;
    }
  }

  return json(404, { error: "not found" });
}
