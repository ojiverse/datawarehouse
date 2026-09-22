import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { buildArchiveObject } from "../src/observation/archive-object.ts";
import type { HttpBackfillEnvelopeV1 } from "../src/observation/envelope.ts";

/**
 * Regenerates the cross-language Archive object fixture consumed by the Go
 * replay reader tests (internal/archive/archive_cross_language_test.go, #38).
 *
 * This script runs the real TypeScript producer path (buildArchiveObject,
 * including CompressionStream gzip and Web Crypto SHA-256) against the shared
 * Envelope v1 contract fixture, so the Go side decodes bytes the TypeScript
 * writer actually produces, not a Go-authored stand-in.
 *
 * The gzip output embeds a wall-clock MTIME header field, so re-running this
 * script changes the committed bytes without changing decoded content; that
 * is expected and does not need to be treated as a diff to review carefully -
 * only the decoded semantics matter (docs/architecture/data-contracts.md).
 *
 * Usage: node --experimental-strip-types scripts/generate-cross-language-archive-fixture.ts
 */

const here = dirname(fileURLToPath(import.meta.url));
const contractFixturePath = join(
  here,
  "../../../contracts/observation-envelope/v1/http-backfill-page.json",
);
const outDir = join(here, "../../../internal/archive/testdata/ts_producer");

async function main(): Promise<void> {
  const fixtureText = readFileSync(contractFixturePath, "utf8");
  const envelope = JSON.parse(fixtureText) as HttpBackfillEnvelopeV1;
  const rawPayload = new TextEncoder().encode(JSON.stringify(envelope.payload));

  const object = await buildArchiveObject(envelope, rawPayload);

  mkdirSync(outDir, { recursive: true });
  writeFileSync(join(outDir, "archive-object.json.gz"), object.body);
  writeFileSync(
    join(outDir, "archive-object.metadata.json"),
    `${JSON.stringify(
      {
        key: object.key,
        observation_id: object.observation_id,
        custom_metadata: object.custom_metadata,
        http_metadata: object.http_metadata,
      },
      null,
      2,
    )}\n`,
  );
  console.log(`wrote ${outDir}/archive-object.json.gz and archive-object.metadata.json`);
}

await main();
