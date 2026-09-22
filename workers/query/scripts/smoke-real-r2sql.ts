// Reproducible smoke test against the REAL R2 SQL HTTP API and a Canonical
// Message table produced by the real `cmd/dwh-materializer` (not a
// hand-crafted fixture), exercising the exact same query-building and
// row-mapping code the Worker uses (src/domain, src/adapters).
//
// It never touches the Observation Archive's raw object layout: the only
// input is the Iceberg table `<DWH_R2SQL_NAMESPACE>.<DWH_R2SQL_TABLE>` via
// R2 SQL, proving the Query API's dependency boundary documented in
// docs/architecture/cloudflare/canonical-store/README.md.
//
// Usage (dev-environment credentials from 1Password, ojilab account):
//   op run --account ojilab --env-file ../../spike/r2-dev.op.env -- \
//     pnpm run smoke:real-r2sql
//
// Required env vars: DWH_R2SQL_ACCOUNT_ID, DWH_R2SQL_BUCKET, DWH_R2SQL_TOKEN,
// DWH_CATALOG_NAMESPACE (defaults to "dwh"), DWH_R2SQL_TABLE (defaults to
// "message"). The table must already contain rows materialized by
// cmd/dwh-materializer against the same warehouse.
import { createR2SqlClient } from "../src/adapters/r2sql-client.ts";
import { mapRowToCanonicalMessage, mapRowToChannelMessageCount } from "../src/domain/message.ts";
import {
  buildChannelMessageCountQuery,
  buildGetMessageByIdQuery,
  buildListMessagesQuery,
  parseSnowflake,
  parseTimestamp,
} from "../src/domain/query.ts";

function requireEnv(name: string): string {
  const value = process.env[name];
  if (!value) throw new Error(`${name} is required (see script header)`);
  return value;
}

async function main(): Promise<void> {
  const client = createR2SqlClient({
    accountId: requireEnv("DWH_R2SQL_ACCOUNT_ID"),
    bucket: requireEnv("DWH_R2SQL_BUCKET"),
    token: requireEnv("DWH_R2SQL_TOKEN"),
  });
  const table = {
    namespace: process.env.DWH_CATALOG_NAMESPACE ?? "dwh",
    table: process.env.DWH_R2SQL_TABLE ?? "message",
  };

  // Query 5 first: aggregate Message count per Channel, and pick a real
  // Channel/Message to drive Queries 1-4 against, so this script never
  // depends on hand-picked fixture IDs.
  const counts = (await client.query(buildChannelMessageCountQuery(table, 500))).map(
    mapRowToChannelMessageCount,
  );
  if (counts.length === 0) throw new Error("table has no rows; materialize data first");
  console.log(`Query 5 (aggregate per Channel): ${counts.length} channel(s)`, counts.slice(0, 5));

  const firstChannel = counts[0];
  if (!firstChannel) throw new Error("unreachable: counts is non-empty");
  const channelId = parseSnowflake(firstChannel.channel_id);
  if (!channelId)
    throw new Error(`channel_id from R2 SQL is not a Snowflake: ${firstChannel.channel_id}`);

  const byChannel = (
    await client.query(
      buildListMessagesQuery(table, "channel_id", channelId, undefined, { limit: 5 }),
    )
  ).map(mapRowToCanonicalMessage);
  if (byChannel.length === 0) throw new Error("channel listing returned no rows");
  console.log(`Query 2 (list by Channel ${channelId}): ${byChannel.length} row(s)`);

  const sample = byChannel[0];
  if (!sample) throw new Error("unreachable: byChannel is non-empty");

  const sampleMessageId = parseSnowflake(sample.message_id);
  if (!sampleMessageId)
    throw new Error(`message_id from R2 SQL is not a Snowflake: ${sample.message_id}`);
  const byId = (await client.query(buildGetMessageByIdQuery(table, sampleMessageId))).map(
    mapRowToCanonicalMessage,
  );
  if (byId.length !== 1 || byId[0]?.message_id !== sample.message_id) {
    throw new Error("Query 1 (get by ID) did not return exactly the sampled Message");
  }
  console.log("Query 1 (get by Message ID): OK", byId[0]);

  const authorId = parseSnowflake(sample.author_id);
  if (!authorId) throw new Error(`author_id from R2 SQL is not a Snowflake: ${sample.author_id}`);
  const byAuthor = (
    await client.query(
      buildListMessagesQuery(table, "author_id", authorId, undefined, { limit: 5 }),
    )
  ).map(mapRowToCanonicalMessage);
  if (!byAuthor.some((m) => m.message_id === sample.message_id)) {
    throw new Error("Query 3 (list by Author) did not include the sampled Message");
  }
  console.log(`Query 3 (list by Author ${authorId}): ${byAuthor.length} row(s)`);

  // Query 4: a created_at range that must include the sampled Message and a
  // range that must exclude it, proving the filter is a real predicate.
  const createdAt = parseTimestamp(sample.created_at);
  if (!createdAt) throw new Error(`created_at from R2 SQL did not parse: ${sample.created_at}`);
  const including = (
    await client.query(
      buildListMessagesQuery(
        table,
        "channel_id",
        channelId,
        {
          after: new Date(createdAt.getTime() - 1000),
          before: new Date(createdAt.getTime() + 1000),
        },
        { limit: 500 },
      ),
    )
  ).map(mapRowToCanonicalMessage);
  if (!including.some((m) => m.message_id === sample.message_id)) {
    throw new Error("Query 4 (created-at range) excluded the sampled Message unexpectedly");
  }
  const excluding = (
    await client.query(
      buildListMessagesQuery(
        table,
        "channel_id",
        channelId,
        { after: new Date(createdAt.getTime() + 1000) },
        { limit: 500 },
      ),
    )
  ).map(mapRowToCanonicalMessage);
  if (excluding.some((m) => m.message_id === sample.message_id)) {
    throw new Error(
      "Query 4 (created-at range) included the sampled Message when it should not have",
    );
  }
  console.log("Query 4 (Discord-side created-at range filter): OK (includes/excludes verified)");

  console.log("SMOKE OK: all 5 required queries succeeded against real R2 SQL data.");
}

main().catch((err: unknown) => {
  console.error(err);
  process.exitCode = 1;
});
