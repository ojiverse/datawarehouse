import { type Schema, Validator } from "@cfworker/json-schema";
import { describe, expect, it } from "vitest";
import fixture from "../../../../contracts/observation-envelope/v1/http-backfill-page.json";
import schema from "../../../../contracts/observation-envelope/v1/schema.json";
import type { Snowflake, UuidV7 } from "../../src/domain/ids";
import { EMPTY_RATE_LIMIT_HEADERS } from "../../src/domain/rate-limit";
import { type BuildEnvelopeInput, buildHttpBackfillEnvelope } from "../../src/observation/envelope";

/**
 * `contracts/observation-envelope/v1/schema.json` is the authoritative wire contract
 * (ADR-0015, docs/architecture/data-contracts.md). The TypeScript envelope type is only an
 * implementation artifact, so every envelope the producer emits is validated here against the
 * schema itself. `@cfworker/json-schema` is used because it interprets the schema at runtime;
 * validators that compile to JavaScript with `new Function` cannot run inside workerd.
 */
const validator = new Validator(schema as Schema, "2020-12", true);

function validate(document: unknown) {
  return validator.validate(JSON.parse(JSON.stringify(document)));
}

function baseInput(overrides: Partial<BuildEnvelopeInput> = {}): BuildEnvelopeInput {
  return {
    observation_id: "0199a1b2-c3d4-7e5f-8a6b-7c8d9e0f1a2b" as UuidV7,
    run_id: "0199a1b2-0000-7000-8000-000000000001" as UuidV7,
    discord_api_version: "10",
    guild_id: "111111111111111111" as Snowflake,
    channel_id: "222222222222222222" as Snowflake,
    pagination: { before: "1000003" as Snowflake, after: null },
    limit: 100,
    request_started_at: "2026-09-21T10:00:00.000Z",
    response_completed_at: "2026-09-21T10:00:00.250Z",
    http_status: 200,
    payload: fixture.payload,
    capabilities: { message_content: true },
    rate_limit: {
      ...EMPTY_RATE_LIMIT_HEADERS,
      limit: 5,
      remaining: 4,
      reset_after_seconds: 1.5,
      bucket: "route-bucket-hash",
    },
    ...overrides,
  };
}

describe("Observation Envelope v1 JSON Schema", () => {
  it("accepts the shared compatibility fixture", () => {
    const result = validate(fixture);
    expect(result.errors).toEqual([]);
    expect(result.valid).toBe(true);
  });

  it("accepts the envelope produced by the TypeScript producer for the fixture inputs", () => {
    const envelope = buildHttpBackfillEnvelope(baseInput());
    const result = validate(envelope);
    expect(result.errors).toEqual([]);
    expect(result.valid).toBe(true);
    expect(JSON.parse(JSON.stringify(envelope))).toEqual(fixture);
  });

  it("accepts a first page without cursor and without rate-limit headers", () => {
    const envelope = buildHttpBackfillEnvelope(
      baseInput({
        pagination: { before: null, after: null },
        rate_limit: EMPTY_RATE_LIMIT_HEADERS,
        payload: [],
      }),
    );
    const result = validate(envelope);
    expect(result.errors).toEqual([]);
    expect(result.valid).toBe(true);
  });

  it("rejects an http_backfill envelope whose payload is not a page array", () => {
    const envelope = buildHttpBackfillEnvelope(baseInput({ payload: { unexpected: true } }));
    expect(validate(envelope).valid).toBe(false);
  });

  it("rejects a provenance that lost a required field", () => {
    const envelope = buildHttpBackfillEnvelope(baseInput());
    const { rate_limit: _dropped, ...provenance } = envelope.provenance;
    expect(validate({ ...envelope, provenance }).valid).toBe(false);
  });

  it("rejects a non-v7 observation id", () => {
    const envelope = buildHttpBackfillEnvelope(
      baseInput({ observation_id: "123e4567-e89b-12d3-a456-426614174000" as UuidV7 }),
    );
    expect(validate(envelope).valid).toBe(false);
  });
});
