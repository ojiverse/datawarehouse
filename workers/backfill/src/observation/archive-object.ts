import type { UuidV7 } from "../domain/ids";
import type { HttpBackfillEnvelopeV1 } from "./envelope";

/**
 * Physical R2 representation of one Observation (docs/infrastructure/cloudflare/r2/README.md).
 */

export const ARCHIVE_FORMAT = "observation-envelope-json" as const;
export const ARCHIVE_COMPRESSION = "gzip" as const;

/** R2 custom metadata keys; values are strings because R2 metadata is string-only. */
export type ArchiveCustomMetadata = {
  readonly format: typeof ARCHIVE_FORMAT;
  readonly compression: typeof ARCHIVE_COMPRESSION;
  readonly envelope_version: string;
  readonly observation_id: string;
  readonly source_kind: string;
  /** Hex SHA-256 of the raw source payload bytes as received from Discord. */
  readonly payload_sha256: string;
};

export type ArchiveObject = {
  readonly key: string;
  readonly observation_id: UuidV7;
  readonly body: Uint8Array;
  readonly custom_metadata: ArchiveCustomMetadata;
  readonly http_metadata: { readonly contentType: string; readonly contentEncoding: string };
};

function pad2(n: number): string {
  return n.toString().padStart(2, "0");
}

/** Partition prefix derives from Observed At (UTC), never from Discord entity time. */
export function archiveKeyFor(
  sourceKind: string,
  observedAtIso: string,
  observationId: UuidV7,
): string {
  const t = new Date(observedAtIso);
  if (Number.isNaN(t.getTime())) {
    throw new Error(`observed_at is not a valid timestamp: ${observedAtIso}`);
  }
  const year = t.getUTCFullYear();
  const month = pad2(t.getUTCMonth() + 1);
  const day = pad2(t.getUTCDate());
  const hour = pad2(t.getUTCHours());
  return `observations/v1/source=${sourceKind}/year=${year}/month=${month}/day=${day}/hour=${hour}/${observationId}.json.gz`;
}

/**
 * Copies a view into a standalone ArrayBuffer. Web APIs (digest, Response, R2 put) reject a
 * `Uint8Array<ArrayBufferLike>` at the type level because it could be backed by a
 * SharedArrayBuffer; a copy of one page is cheap and keeps the callers free of casts.
 */
export function toArrayBuffer(bytes: Uint8Array): ArrayBuffer {
  const out = new ArrayBuffer(bytes.byteLength);
  new Uint8Array(out).set(bytes);
  return out;
}

export async function sha256Hex(bytes: Uint8Array): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", toArrayBuffer(bytes));
  return Array.from(new Uint8Array(digest), (b) => b.toString(16).padStart(2, "0")).join("");
}

export async function gzipBytes(bytes: Uint8Array): Promise<Uint8Array> {
  const stream = new Response(toArrayBuffer(bytes)).body?.pipeThrough(
    new CompressionStream("gzip"),
  );
  return new Uint8Array(await new Response(stream ?? null).arrayBuffer());
}

export async function gunzipBytes(bytes: Uint8Array): Promise<Uint8Array> {
  const stream = new Response(toArrayBuffer(bytes)).body?.pipeThrough(
    new DecompressionStream("gzip"),
  );
  return new Uint8Array(await new Response(stream ?? null).arrayBuffer());
}

export async function buildArchiveObject(
  envelope: HttpBackfillEnvelopeV1,
  rawPayloadBytes: Uint8Array,
): Promise<ArchiveObject> {
  const json = new TextEncoder().encode(JSON.stringify(envelope));
  const [body, payloadSha256] = await Promise.all([gzipBytes(json), sha256Hex(rawPayloadBytes)]);
  return {
    key: archiveKeyFor(envelope.source_kind, envelope.observed_at, envelope.observation_id),
    observation_id: envelope.observation_id,
    body,
    custom_metadata: {
      format: ARCHIVE_FORMAT,
      compression: ARCHIVE_COMPRESSION,
      envelope_version: envelope.envelope_version,
      observation_id: envelope.observation_id,
      source_kind: envelope.source_kind,
      payload_sha256: payloadSha256,
    },
    http_metadata: { contentType: "application/json", contentEncoding: "gzip" },
  };
}
