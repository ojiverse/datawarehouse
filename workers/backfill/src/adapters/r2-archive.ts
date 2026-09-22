import { type ArchiveObject, toArrayBuffer } from "../observation/archive-object";
import {
  type ArchiveCommitResult,
  ArchiveInvariantViolation,
  type ObservationArchive,
} from "../ports";

/**
 * Create-only R2 writer (archive-write-path.md "No Overwrite").
 *
 * `onlyIf: { etagDoesNotMatch: "*" }` is the binding form of `If-None-Match: *`; R2 returns
 * `null` instead of storing when the key already exists.
 */
export class R2ObservationArchive implements ObservationArchive {
  constructor(private readonly bucket: R2Bucket) {}

  async commit(object: ArchiveObject): Promise<ArchiveCommitResult> {
    const stored = await this.bucket.put(object.key, toArrayBuffer(object.body), {
      onlyIf: { etagDoesNotMatch: "*" },
      httpMetadata: object.http_metadata,
      customMetadata: object.custom_metadata,
    });
    if (stored !== null) {
      return { key: object.key, outcome: "created" };
    }

    const existing = await this.bucket.head(object.key);
    if (existing === null) {
      // The precondition failed yet the object is gone: a concurrent erasure or an R2 hiccup.
      // Report as a transient failure so the caller retries without advancing progress.
      throw new Error(`conditional put rejected but ${object.key} is absent`);
    }
    const meta = existing.customMetadata ?? {};
    const sameId = meta.observation_id === object.custom_metadata.observation_id;
    const sameHash = meta.payload_sha256 === object.custom_metadata.payload_sha256;
    if (sameId && sameHash) {
      return { key: object.key, outcome: "already_present" };
    }
    throw new ArchiveInvariantViolation(
      `existing object ${object.key} differs from retry (id match=${sameId}, hash match=${sameHash})`,
    );
  }
}
