import { describe, expect, it } from "vitest";
import { R2SqlError } from "../../src/adapters/r2sql-client";
import type { R2SqlRow } from "../../src/domain/message";
import type { Env } from "../../src/env";
import { route } from "../../src/worker/api";

const ENV: Env = {
  ENVIRONMENT: "test",
  R2_SQL_ACCOUNT_ID: "acct",
  R2_SQL_BUCKET: "bkt",
  R2_SQL_NAMESPACE: "dwh_test",
  R2_SQL_TABLE: "message",
  QUERY_DEFAULT_PAGE_LIMIT: "100",
  QUERY_MAX_PAGE_LIMIT: "500",
  R2_SQL_TOKEN: "unused-in-these-tests",
  QUERY_API_TOKEN: "test-api-token",
};

const AUTH = { authorization: "Bearer test-api-token" };

const REAL_ROW: R2SqlRow = {
  message_id: "1551378258329600000",
  channel_id: "200000000000000002",
  guild_id: "100000000000000001",
  author_id: "300000000000000000",
  author_username: "user00",
  author_is_bot: false,
  content: "hello",
  created_at: "2026-09-20T23:43:20.000000Z",
  edited_at: null,
  pinned: false,
  message_type: 0,
  observation_id: "01a0c7a2-c41a-79ee-a788-dd1b25ac07c2",
  observed_at: "2026-09-21T00:00:00.000000Z",
  source_kind: "http_backfill",
  projection_version: "http-message-v1",
};

class FakeR2Sql {
  public queries: string[] = [];
  private readonly rows: readonly R2SqlRow[];
  private readonly error: Error | undefined;

  constructor(rows: readonly R2SqlRow[] = [], error?: Error) {
    this.rows = rows;
    this.error = error;
  }

  async query(sql: string): Promise<readonly R2SqlRow[]> {
    this.queries.push(sql);
    if (this.error) throw this.error;
    return this.rows;
  }
}

function get(path: string, headers: Record<string, string> = AUTH): Request {
  return new Request(`https://query.test${path}`, { headers });
}

describe("Query API Worker routing", () => {
  it("serves /healthz without authentication", async () => {
    const res = await route(get("/healthz", {}), ENV, new FakeR2Sql());
    expect(res.status).toBe(200);
    expect(await res.json()).toEqual({ ok: true });
  });

  it("rejects missing or wrong bearer tokens on every other route", async () => {
    expect((await route(get("/v1/messages/1", {}), ENV, new FakeR2Sql())).status).toBe(401);
    expect(
      (await route(get("/v1/messages/1", { authorization: "Bearer nope" }), ENV, new FakeR2Sql()))
        .status,
    ).toBe(401);
  });

  it("rejects non-GET methods", async () => {
    const req = new Request("https://query.test/v1/messages/1", { method: "POST", headers: AUTH });
    expect((await route(req, ENV, new FakeR2Sql())).status).toBe(405);
  });

  it("returns 404 for unknown routes", async () => {
    expect((await route(get("/v1/nope"), ENV, new FakeR2Sql())).status).toBe(404);
  });

  describe("Query 1: get a Message by ID", () => {
    it("returns the Canonical Message on a hit", async () => {
      const sql = new FakeR2Sql([REAL_ROW]);
      const res = await route(get("/v1/messages/1551378258329600000"), ENV, sql);
      expect(res.status).toBe(200);
      expect((await res.json()) as { message: R2SqlRow }).toEqual({ message: REAL_ROW });
      expect(sql.queries[0]).toContain("WHERE message_id = '1551378258329600000'");
    });

    it("returns 404 when no row matches", async () => {
      const res = await route(get("/v1/messages/999"), ENV, new FakeR2Sql([]));
      expect(res.status).toBe(404);
    });

    it("returns 404 for a non-digit path segment (never reaches R2 SQL)", async () => {
      const res = await route(get("/v1/messages/not-a-snowflake"), ENV, new FakeR2Sql());
      expect(res.status).toBe(404);
    });

    it("returns 400 for a digit string too long to be a Snowflake", async () => {
      const tooLong = "1".repeat(21);
      const res = await route(get(`/v1/messages/${tooLong}`), ENV, new FakeR2Sql());
      expect(res.status).toBe(400);
    });
  });

  describe("Query 2: list Messages by Channel", () => {
    it("filters by channel_id and returns the list", async () => {
      const sql = new FakeR2Sql([REAL_ROW]);
      const res = await route(get("/v1/channels/200000000000000002/messages"), ENV, sql);
      expect(res.status).toBe(200);
      expect((await res.json()) as { messages: R2SqlRow[] }).toEqual({ messages: [REAL_ROW] });
      expect(sql.queries[0]).toContain("channel_id = '200000000000000002'");
      expect(sql.queries[0]).toContain("LIMIT 100"); // QUERY_DEFAULT_PAGE_LIMIT
    });

    it("rejects a limit above QUERY_MAX_PAGE_LIMIT", async () => {
      const res = await route(get("/v1/channels/1/messages?limit=501"), ENV, new FakeR2Sql());
      expect(res.status).toBe(400);
    });

    it("rejects an invalid after cursor", async () => {
      const res = await route(get("/v1/channels/1/messages?after=nope"), ENV, new FakeR2Sql());
      expect(res.status).toBe(400);
    });

    it("applies a valid after cursor to the query", async () => {
      const sql = new FakeR2Sql([]);
      await route(get("/v1/channels/1/messages?after=42"), ENV, sql);
      expect(sql.queries[0]).toContain("message_id > '42'");
    });
  });

  describe("Query 3: list Messages by Author", () => {
    it("filters by author_id and returns the list", async () => {
      const sql = new FakeR2Sql([REAL_ROW]);
      const res = await route(get("/v1/authors/300000000000000000/messages"), ENV, sql);
      expect(res.status).toBe(200);
      expect((await res.json()) as { messages: R2SqlRow[] }).toEqual({ messages: [REAL_ROW] });
      expect(sql.queries[0]).toContain("author_id = '300000000000000000'");
    });
  });

  describe("Query 4: Discord-side created-at range filter", () => {
    it("composes with a channel listing", async () => {
      const sql = new FakeR2Sql([]);
      await route(
        get(
          "/v1/channels/1/messages?created_after=2026-01-01T00:00:00Z&created_before=2026-02-01T00:00:00Z",
        ),
        ENV,
        sql,
      );
      expect(sql.queries[0]).toContain("created_at >= TIMESTAMP '2026-01-01T00:00:00.000Z'");
      expect(sql.queries[0]).toContain("created_at < TIMESTAMP '2026-02-01T00:00:00.000Z'");
    });

    it("composes with an author listing using only one bound", async () => {
      const sql = new FakeR2Sql([]);
      await route(get("/v1/authors/1/messages?created_after=2026-01-01T00:00:00Z"), ENV, sql);
      expect(sql.queries[0]).toContain("created_at >= TIMESTAMP '2026-01-01T00:00:00.000Z'");
      expect(sql.queries[0]).not.toContain("created_at <");
    });

    it("rejects an unparsable created_after", async () => {
      const res = await route(
        get("/v1/channels/1/messages?created_after=nope"),
        ENV,
        new FakeR2Sql(),
      );
      expect(res.status).toBe(400);
    });
  });

  describe("Query 5: aggregate Message count per Channel", () => {
    it("returns the per-channel aggregate", async () => {
      const sql = new FakeR2Sql([{ channel_id: "1", message_count: 85 }]);
      const res = await route(get("/v1/channels/messages/count"), ENV, sql);
      expect(res.status).toBe(200);
      expect(await res.json()).toEqual({
        channel_message_counts: [{ channel_id: "1", message_count: 85 }],
      });
      expect(sql.queries[0]).toBe(
        "SELECT channel_id, COUNT(*) AS message_count FROM dwh_test.message GROUP BY channel_id ORDER BY channel_id LIMIT 500",
      );
    });

    it("respects a limit override", async () => {
      const sql = new FakeR2Sql([]);
      await route(get("/v1/channels/messages/count?limit=10"), ENV, sql);
      expect(sql.queries[0]).toContain("LIMIT 10");
    });
  });

  it("maps an R2SqlError to a 502 without leaking the raw query", async () => {
    const sql = new FakeR2Sql([], new R2SqlError("r2 sql http 500: boom", 500));
    const res = await route(get("/v1/messages/1"), ENV, sql);
    expect(res.status).toBe(502);
  });
});
