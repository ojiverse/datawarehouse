import { env } from "cloudflare:test";
import { beforeEach, describe, expect, it } from "vitest";
import { R2ObservationArchive } from "../../src/adapters/r2-archive";
import type { ArchiveObject } from "../../src/observation/archive-object";
import { ArchiveInvariantViolation } from "../../src/ports";
import { purgeObservations } from "../helpers/r2";

function object(overrides: Partial<ArchiveObject> = {}): ArchiveObject {
  return {
    key: "observations/v1/source=http_backfill/year=2026/month=09/day=21/hour=10/0199a1b2-c3d4-7e5f-8a6b-7c8d9e0f1a2b.json.gz",
    observation_id: "0199a1b2-c3d4-7e5f-8a6b-7c8d9e0f1a2b" as ArchiveObject["observation_id"],
    body: new TextEncoder().encode("original-body"),
    custom_metadata: {
      format: "observation-envelope-json",
      compression: "gzip",
      envelope_version: "v1",
      observation_id: "0199a1b2-c3d4-7e5f-8a6b-7c8d9e0f1a2b",
      source_kind: "http_backfill",
      payload_sha256: "aaaa",
    },
    http_metadata: { contentType: "application/json", contentEncoding: "gzip" },
    ...overrides,
  };
}

describe("R2ObservationArchive", () => {
  let archive: R2ObservationArchive;

  beforeEach(async () => {
    await purgeObservations();
    archive = new R2ObservationArchive(env.OBSERVATIONS);
  });

  it("creates the object with self-describing metadata", async () => {
    const result = await archive.commit(object());
    expect(result.outcome).toBe("created");

    const head = await env.OBSERVATIONS.head(result.key);
    expect(head?.customMetadata).toMatchObject({
      format: "observation-envelope-json",
      compression: "gzip",
      envelope_version: "v1",
      observation_id: "0199a1b2-c3d4-7e5f-8a6b-7c8d9e0f1a2b",
      source_kind: "http_backfill",
      payload_sha256: "aaaa",
    });
    expect(head?.httpMetadata?.contentType).toBe("application/json");
    expect(head?.httpMetadata?.contentEncoding).toBe("gzip");
  });

  it("treats a same-id same-hash retry as idempotent success without overwriting", async () => {
    await archive.commit(object());
    const before = await env.OBSERVATIONS.head(object().key);

    const retry = await archive.commit(
      object({ body: new TextEncoder().encode("retry-body-differs-in-encoding") }),
    );
    expect(retry.outcome).toBe("already_present");

    const after = await env.OBSERVATIONS.get(object().key);
    expect(after?.etag).toBe(before?.etag);
    expect(await after?.text()).toBe("original-body");
  });

  it("fails a same-key different-hash retry as an invariant violation and keeps the original", async () => {
    await archive.commit(object());
    await expect(
      archive.commit(
        object({ custom_metadata: { ...object().custom_metadata, payload_sha256: "bbbb" } }),
      ),
    ).rejects.toBeInstanceOf(ArchiveInvariantViolation);
    const kept = await env.OBSERVATIONS.get(object().key);
    expect(await kept?.text()).toBe("original-body");
  });

  it("fails a same-key different-id retry as an invariant violation", async () => {
    await archive.commit(object());
    await expect(
      archive.commit(
        object({
          custom_metadata: {
            ...object().custom_metadata,
            observation_id: "0199a1b2-c3d4-7e5f-8a6b-7c8d9e0f1a2c",
          },
        }),
      ),
    ).rejects.toBeInstanceOf(ArchiveInvariantViolation);
  });
});
