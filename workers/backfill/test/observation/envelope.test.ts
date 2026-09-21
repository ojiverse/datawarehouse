import { describe, expect, it } from "vitest";
import fixture from "../../../../contracts/observation-envelope/v1/http-backfill-page.json";
import type { Snowflake, UuidV7 } from "../../src/domain/ids";
import { EMPTY_RATE_LIMIT_HEADERS } from "../../src/domain/rate-limit";
import { buildHttpBackfillEnvelope } from "../../src/observation/envelope";

describe("Envelope v1 (http_backfill)", () => {
  it("reproduces the shared contract fixture", () => {
    const envelope = buildHttpBackfillEnvelope({
      observation_id: fixture.observation_id as UuidV7,
      run_id: fixture.provenance.run_id as UuidV7,
      discord_api_version: "10",
      guild_id: fixture.provenance.guild_id as Snowflake,
      channel_id: fixture.provenance.channel_id as Snowflake,
      pagination: { before: fixture.provenance.pagination.before as Snowflake, after: null },
      limit: 100,
      request_started_at: fixture.provenance.request_started_at,
      response_completed_at: fixture.provenance.response_completed_at,
      http_status: 200,
      payload: fixture.payload,
      capabilities: { message_content: true },
      rate_limit: {
        ...EMPTY_RATE_LIMIT_HEADERS,
        limit: 5,
        remaining: 4,
        reset_after_seconds: 1.5,
        bucket: "route-bucket-hash",
        scope: "user",
        retry_after_seconds: 7,
      },
    });
    expect(JSON.parse(JSON.stringify(envelope))).toEqual(fixture);
  });

  it("uses response completion as Observed At", () => {
    const envelope = buildHttpBackfillEnvelope({
      observation_id: "0199a1b2-c3d4-7e5f-8a6b-7c8d9e0f1a2b" as UuidV7,
      run_id: "0199a1b2-0000-7000-8000-000000000001" as UuidV7,
      discord_api_version: "10",
      guild_id: "1" as Snowflake,
      channel_id: "2" as Snowflake,
      pagination: { before: null, after: null },
      limit: 50,
      request_started_at: "2026-01-01T00:00:00.000Z",
      response_completed_at: "2026-01-01T00:00:01.000Z",
      http_status: 200,
      payload: [],
      capabilities: { message_content: false },
      rate_limit: EMPTY_RATE_LIMIT_HEADERS,
    });
    expect(envelope.observed_at).toBe("2026-01-01T00:00:01.000Z");
    expect(JSON.stringify(envelope)).not.toMatch(/authorization|token|cookie/i);
  });
});
