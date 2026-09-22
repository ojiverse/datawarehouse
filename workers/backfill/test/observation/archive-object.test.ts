import { describe, expect, it } from "vitest";
import fixture from "../../../../contracts/observation-envelope/v1/http-backfill-page.json";
import type { UuidV7 } from "../../src/domain/ids";
import {
  archiveKeyFor,
  buildArchiveObject,
  gunzipBytes,
  sha256Hex,
} from "../../src/observation/archive-object";
import type { HttpBackfillEnvelopeV1 } from "../../src/observation/envelope";

const envelope = fixture as unknown as HttpBackfillEnvelopeV1;

describe("archiveKeyFor", () => {
  it("partitions by Observed At in UTC", () => {
    expect(
      archiveKeyFor(
        "http_backfill",
        "2026-09-21T23:59:59.999+09:00",
        "0199a1b2-c3d4-7e5f-8a6b-7c8d9e0f1a2b" as UuidV7,
      ),
    ).toBe(
      "observations/v1/source=http_backfill/year=2026/month=09/day=21/hour=14/0199a1b2-c3d4-7e5f-8a6b-7c8d9e0f1a2b.json.gz",
    );
  });

  it("rejects an invalid timestamp", () => {
    expect(() =>
      archiveKeyFor(
        "http_backfill",
        "not-a-date",
        "0199a1b2-c3d4-7e5f-8a6b-7c8d9e0f1a2b" as UuidV7,
      ),
    ).toThrow();
  });
});

describe("buildArchiveObject", () => {
  it("produces a self-describing gzip JSON object", async () => {
    const raw = new TextEncoder().encode(JSON.stringify(fixture.payload));
    const object = await buildArchiveObject(envelope, raw);

    expect(object.key).toBe(
      "observations/v1/source=http_backfill/year=2026/month=09/day=21/hour=10/0199a1b2-c3d4-7e5f-8a6b-7c8d9e0f1a2b.json.gz",
    );
    expect(object.http_metadata).toEqual({
      contentType: "application/json",
      contentEncoding: "gzip",
    });
    expect(object.custom_metadata).toEqual({
      format: "observation-envelope-json",
      compression: "gzip",
      envelope_version: "v1",
      observation_id: fixture.observation_id,
      source_kind: "http_backfill",
      payload_sha256: await sha256Hex(raw),
    });

    const decoded = JSON.parse(new TextDecoder().decode(await gunzipBytes(object.body)));
    expect(decoded).toEqual(fixture);
  });

  it("hashes the raw payload bytes, not the envelope", async () => {
    const a = await buildArchiveObject(envelope, new TextEncoder().encode("[1]"));
    const b = await buildArchiveObject(envelope, new TextEncoder().encode("[2]"));
    expect(a.custom_metadata.payload_sha256).not.toBe(b.custom_metadata.payload_sha256);
  });
});
