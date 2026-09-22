# Query Worker

Cloudflare Workers implementation of the first-MVP Query API (Issue #41). It exposes the
minimal set of queries needed to prove the Canonical Store is queryable, using **R2 SQL** as
the query engine (a decision made in advance and not revisited here), in TypeScript as required
by the Implementation Language Policy.

## Components

* **Query API Worker** (`src/worker/api.ts`, wired by `src/index.ts`): stateless bearer-token
  authentication, routing and request/response shaping. It owns no state and reads nothing but
  the Canonical Store.
* **Domain modules** (`src/domain`): SQL building (`query.ts`) and R2 SQL row → Canonical
  Message mapping (`message.ts`), independent of Cloudflare APIs. This is where "query semantics
  follow the Canonical Message domain model" is enforced: field names in every response mirror
  `internal/canonical/schema.go`'s Iceberg column names exactly.
* **Adapter** (`src/adapters/r2sql-client.ts`): the R2 SQL HTTP API client (the TypeScript
  counterpart of `internal/r2sql`), the only I/O boundary to the Canonical Store. It never reads
  Observation Archive raw objects.

## HTTP API

All endpoints except `/healthz` require `Authorization: Bearer <QUERY_API_TOKEN>`, the same
bearer-token convention `workers/backfill` uses (`docs/infrastructure/cloudflare/security/README.md`
does not prescribe a different membership-admission mechanism for first-MVP, so none is invented
here). Every route is read-only (`GET`); non-`GET` requests get `405`.

| Required query | Endpoint |
| :--- | :--- |
| Get a Message by Message ID | `GET /v1/messages/{message_id}` |
| List Messages by Channel | `GET /v1/channels/{channel_id}/messages` |
| List Messages by Author | `GET /v1/authors/{author_id}/messages` |
| Filter by Discord-side created-at range | add `created_after` / `created_before` (ISO 8601) to either list endpoint above |
| Aggregate Message count per Channel | `GET /v1/channels/messages/count` |

List endpoints also accept `limit` (bounded by `QUERY_MAX_PAGE_LIMIT`, default
`QUERY_DEFAULT_PAGE_LIMIT`) and `after` (a Message ID cursor for simple keyset pagination ordered
by `message_id`). `GET /v1/messages/{message_id}` returns `404` when no row matches; list
endpoints return an empty array. A malformed or oversized R2 SQL response is surfaced as `502`,
never as a silently empty result.

## Configuration

Variables are declared in `wrangler.jsonc`; secrets are injected with `wrangler secret put` and
never committed.

* `R2_SQL_ACCOUNT_ID`, `R2_SQL_BUCKET`: the Cloudflare account and R2 bucket (warehouse) R2 SQL
  queries against. Not secret (also recorded in
  `docs/architecture/cloudflare/processing/iceberg-spike-result.md`).
* `R2_SQL_NAMESPACE`, `R2_SQL_TABLE`: the Iceberg REST Catalog namespace and table name
  (`dwh.message` in production, matching `cmd/dwh-materializer`'s defaults).
* `R2_SQL_TOKEN` (secret): Cloudflare API token scoped to R2 SQL.
* `QUERY_API_TOKEN` (secret): bearer token this Worker's own API requires from its caller.
* `QUERY_DEFAULT_PAGE_LIMIT`, `QUERY_MAX_PAGE_LIMIT`: list endpoint page size bounds.

## Development

Install dependencies with pnpm from the repository root, then run `pnpm lint`, `pnpm typecheck`
and `pnpm test`. Tests run inside workerd through the Cloudflare Vitest plugin:

* `test/domain/*.test.ts` unit-test SQL building, input validation (Snowflake / timestamp
  parsing, so nothing attacker-controlled reaches R2 SQL unvalidated) and R2 SQL row → Canonical
  Message mapping, including the exact row shape real R2 SQL returned in the real-environment
  smoke test below.
* `test/adapters/r2sql-client.test.ts` unit-tests the HTTP client against a stubbed `fetch`.
* `test/worker/api.test.ts` exercises full routing and auth for all 5 required queries against a
  fake `R2SqlPort` (dependency-injected into `route()`), so it needs no network I/O.

## Real R2 SQL proof (Issue #41 Definition of Done)

`scripts/smoke-real-r2sql.ts` (`pnpm run smoke:real-r2sql`) runs the exact query-building and
row-mapping code above against a **real** Canonical Message table on Cloudflare R2, produced by
the real `cmd/dwh-materializer` from realistic Archive objects (not a hand-crafted fixture). It
was run against the OJIverse dev environment (`docs/architecture/cloudflare/processing/iceberg-spike-result.md`'s
R2 real environment) and confirmed all 5 required queries, including that the created-at range
filter genuinely includes/excludes rows rather than trivially matching everything. R2 SQL beta
constraints observed while building this Worker (timestamp literal syntax, per-column JSON
typing, the 500-row result default, cold-query latency) are recorded in
`docs/architecture/cloudflare/canonical-store/README.md`.

Reproduce with dev-environment credentials from 1Password (ojilab account,
`ojiverse-datawarehouse-dev` vault):

```
op run --account ojilab --env-file ../../spike/r2-dev.op.env -- pnpm run smoke:real-r2sql
```

Set `DWH_CATALOG_NAMESPACE` and `DWH_R2SQL_TABLE` to point at a table
`cmd/dwh-materializer` has already populated (see its own README / package comment for how to run
a rebuild against the same warehouse).

## Real Discord-derived smoke procedure (dev)

Automated tests run against fakes and miniflare only, following the same convention
`workers/backfill`'s README documents. A one-off smoke of the deployed Worker against Backfill-produced
data needs:

* a Canonical Store table materialized by `cmd/dwh-materializer` from real Backfill Archive
  objects;
* the secrets `R2_SQL_TOKEN` and `QUERY_API_TOKEN` set with `wrangler secret put --env dev`.

Then deploy with `wrangler deploy --env dev` and call the endpoints above with
`Authorization: Bearer <QUERY_API_TOKEN>`.
