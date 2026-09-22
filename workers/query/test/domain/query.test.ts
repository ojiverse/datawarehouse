import { describe, expect, it } from "vitest";
import {
  buildChannelMessageCountQuery,
  buildGetMessageByIdQuery,
  buildListMessagesQuery,
  parseSnowflake,
  parseTimestamp,
  type Snowflake,
} from "../../src/domain/query";

const TABLE = { namespace: "dwh", table: "message" };

/** Test-only helper: fails fast (rather than a bare `!`) when a fixture value is invalid. */
function mustSnowflake(value: string): Snowflake {
  const parsed = parseSnowflake(value);
  if (!parsed) throw new Error(`test fixture is not a Snowflake: ${value}`);
  return parsed;
}

/** Test-only helper: fails fast (rather than a bare `!`) when a fixture value is invalid. */
function mustTimestamp(value: string): Date {
  const parsed = parseTimestamp(value);
  if (!parsed) throw new Error(`test fixture is not a timestamp: ${value}`);
  return parsed;
}

describe("parseSnowflake", () => {
  it("accepts decimal digit strings", () => {
    expect(parseSnowflake("123456789012345678")).toBe("123456789012345678");
  });

  it("rejects non-digit strings, empty strings and non-strings", () => {
    expect(parseSnowflake("abc")).toBeUndefined();
    expect(parseSnowflake("")).toBeUndefined();
    expect(parseSnowflake("12 34")).toBeUndefined();
    expect(parseSnowflake("1e9")).toBeUndefined();
    expect(parseSnowflake(123)).toBeUndefined();
    expect(parseSnowflake(null)).toBeUndefined();
    // A SQL-injection attempt must never be accepted as a Snowflake.
    expect(parseSnowflake("1' OR '1'='1")).toBeUndefined();
  });
});

describe("parseTimestamp", () => {
  it("accepts ISO 8601 instants", () => {
    expect(parseTimestamp("2026-01-01T00:00:00Z")?.toISOString()).toBe("2026-01-01T00:00:00.000Z");
  });

  it("rejects unparsable values", () => {
    expect(parseTimestamp("not-a-date")).toBeUndefined();
    expect(parseTimestamp("")).toBeUndefined();
    expect(parseTimestamp(undefined)).toBeUndefined();
  });
});

describe("buildGetMessageByIdQuery", () => {
  it("selects the Canonical Message columns filtered by message_id", () => {
    const messageId = mustSnowflake("111");
    const sql = buildGetMessageByIdQuery(TABLE, messageId);
    expect(sql).toBe(
      "SELECT message_id, channel_id, guild_id, author_id, author_username, author_is_bot, content, " +
        "created_at, edited_at, pinned, message_type, observation_id, observed_at, source_kind, " +
        "projection_version FROM dwh.message WHERE message_id = '111'",
    );
  });
});

describe("buildListMessagesQuery", () => {
  it("filters by the scope column and orders/limits deterministically", () => {
    const channelId = mustSnowflake("222");
    const sql = buildListMessagesQuery(TABLE, "channel_id", channelId, undefined, { limit: 50 });
    expect(sql).toContain("WHERE channel_id = '222' ORDER BY message_id LIMIT 50");
    expect(sql).not.toMatch(/WHERE.*created_at/);
  });

  it("adds a created_at range when given one bound", () => {
    const authorId = mustSnowflake("333");
    const after = mustTimestamp("2026-01-01T00:00:00Z");
    const sql = buildListMessagesQuery(TABLE, "author_id", authorId, { after }, { limit: 10 });
    expect(sql).toContain(
      "author_id = '333' AND created_at >= TIMESTAMP '2026-01-01T00:00:00.000Z'",
    );
    expect(sql).not.toContain("created_at <");
  });

  it("adds both created_at bounds when given a full range", () => {
    const channelId = mustSnowflake("444");
    const after = mustTimestamp("2026-01-01T00:00:00Z");
    const before = mustTimestamp("2026-02-01T00:00:00Z");
    const sql = buildListMessagesQuery(
      TABLE,
      "channel_id",
      channelId,
      { after, before },
      { limit: 10 },
    );
    expect(sql).toContain(
      "created_at >= TIMESTAMP '2026-01-01T00:00:00.000Z' AND created_at < TIMESTAMP '2026-02-01T00:00:00.000Z'",
    );
  });

  it("adds a keyset pagination cursor", () => {
    const channelId = mustSnowflake("555");
    const cursor = mustSnowflake("777");
    const sql = buildListMessagesQuery(TABLE, "channel_id", channelId, undefined, {
      limit: 10,
      afterMessageId: cursor,
    });
    expect(sql).toContain("channel_id = '555' AND message_id > '777' ORDER BY message_id LIMIT 10");
  });
});

describe("buildChannelMessageCountQuery", () => {
  it("aggregates Message count grouped by channel_id", () => {
    const sql = buildChannelMessageCountQuery(TABLE, 500);
    expect(sql).toBe(
      "SELECT channel_id, COUNT(*) AS message_count FROM dwh.message GROUP BY channel_id ORDER BY channel_id LIMIT 500",
    );
  });
});
