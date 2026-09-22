import { describe, expect, it } from "vitest";
import {
  MalformedRowError,
  mapRowToCanonicalMessage,
  mapRowToChannelMessageCount,
} from "../../src/domain/message";

// Shaped exactly as the real R2 SQL HTTP API returned it in the real R2
// environment (see the PR description evidence): Snowflakes and timestamps as
// JSON strings, message_type as a JSON number, booleans as JSON booleans.
const REAL_ROW = {
  message_id: "1551378258329600000",
  channel_id: "200000000000000002",
  guild_id: "100000000000000001",
  author_id: "300000000000000000",
  author_username: "user00",
  author_is_bot: false,
  content: "message 0 seen on page 0",
  created_at: "2026-09-20T23:43:20.000000Z",
  edited_at: "2026-09-20T23:59:30.000000Z",
  pinned: true,
  message_type: 0,
  observation_id: "01a0c7a2-c41a-79ee-a788-dd1b25ac07c2",
  observed_at: "2026-09-21T00:00:00.000000Z",
  source_kind: "http_backfill",
  projection_version: "http-message-v1",
};

describe("mapRowToCanonicalMessage", () => {
  it("maps a real R2 SQL row to the Canonical Message domain shape unchanged", () => {
    expect(mapRowToCanonicalMessage(REAL_ROW)).toEqual(REAL_ROW);
  });

  it("maps a null edited_at / author_username / content (unedited, unauthenticated-shape row)", () => {
    const row = { ...REAL_ROW, edited_at: null, author_username: null, content: null };
    const mapped = mapRowToCanonicalMessage(row);
    expect(mapped.edited_at).toBeNull();
    expect(mapped.author_username).toBeNull();
    expect(mapped.content).toBeNull();
  });

  it("coerces a stringified message_type and boolean columns defensively", () => {
    const row = { ...REAL_ROW, message_type: "0", author_is_bot: "true", pinned: "false" };
    const mapped = mapRowToCanonicalMessage(row);
    expect(mapped.message_type).toBe(0);
    expect(mapped.author_is_bot).toBe(true);
    expect(mapped.pinned).toBe(false);
  });

  it("throws MalformedRowError when a required column is missing", () => {
    const { message_id: _drop, ...row } = REAL_ROW;
    expect(() => mapRowToCanonicalMessage(row)).toThrow(MalformedRowError);
  });

  it("throws MalformedRowError when a required column has the wrong JSON type", () => {
    const row = { ...REAL_ROW, pinned: "not-a-boolean" };
    expect(() => mapRowToCanonicalMessage(row)).toThrow(MalformedRowError);
  });
});

describe("mapRowToChannelMessageCount", () => {
  it("maps a GROUP BY channel_id aggregate row", () => {
    expect(
      mapRowToChannelMessageCount({ channel_id: "200000000000000002", message_count: 85 }),
    ).toEqual({
      channel_id: "200000000000000002",
      message_count: 85,
    });
  });

  it("coerces a stringified count", () => {
    expect(mapRowToChannelMessageCount({ channel_id: "1", message_count: "85" })).toEqual({
      channel_id: "1",
      message_count: 85,
    });
  });

  it("throws MalformedRowError on a missing message_count", () => {
    expect(() => mapRowToChannelMessageCount({ channel_id: "1" })).toThrow(MalformedRowError);
  });
});
