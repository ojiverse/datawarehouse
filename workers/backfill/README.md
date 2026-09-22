# Backfill Worker

Cloudflare Workers / Durable Objects implementation of the first-MVP HTTP Backfill
(Issue #37). It realises `docs/architecture/cloudflare/backfill/execution.md` and ADR-0010 in
TypeScript, as required by the Implementation Language Policy.

## Components

* **Backfill API Worker** (`src/worker/api.ts`): stateless bearer-token authentication, input
  validation and routing. It never owns progress.
* **Backfill Channel Durable Object** (`src/do/backfill-channel.ts`): one SQLite-backed object per
  environment and Discord Channel. It stores the run record, pagination cursor, route bucket
  state and terminal errors, executes bounded page batches on Alarm, and re-arms itself from
  storage after a restart. Only one active run per Channel is allowed.
* **Discord HTTP Budget Durable Object** (`src/do/discord-http-budget.ts`): one object per
  environment / Discord application. It coordinates the global request ceiling, the
  invalid-request budget, global 429 pauses and the credential-failure halt.
* **Domain modules** (`src/domain`): pure pagination, rate-limit, status classification and the
  page commit order, independent of Cloudflare APIs.
* **Observation modules** (`src/observation`): Envelope v1 construction and the physical R2 object
  (key layout, gzip, SHA-256, metadata). The authoritative wire contract is
  `contracts/observation-envelope/v1/schema.json` (ADR-0015); the TypeScript envelope type is an
  implementation artifact of it, and `test/observation/schema.test.ts` validates every producer
  output against the schema and the shared fixture.
* **Adapters** (`src/adapters`): Discord HTTP client, create-only R2 writer, Budget RPC client.

## Page commit order

Each page is processed strictly as: read cursor from SQLite, reserve a budget slot, fetch the
page from Discord, build the Envelope, conditional create-only put to R2, then advance the
cursor in one SQLite transaction. Progress is never advanced before the R2 commit succeeds. A
crash between the R2 commit and the progress update leads to a re-fetch of the same scope,
which is archived as a new Observation with a new Observation ID.

## HTTP API

All endpoints except `/healthz` require `Authorization: Bearer <BACKFILL_API_TOKEN>`.

* `POST /v1/backfill/runs` starts a run for a Guild / Channel with an optional Snowflake range
  (`range.after`, `range.before`) and page limit. Returns 202 with the run record, or 409 when
  the Channel already has an active run.
* `GET /v1/backfill/channels/{channel_id}/runs/{run_id}` returns one run record.
* `GET /v1/backfill/channels/{channel_id}` returns the active run, or the most recent one.
* `POST /v1/backfill/channels/{channel_id}/resume` re-arms the Alarm of an active run.

Run states are `running`, `waiting` (rate-limit or budget pause), `completed`, `failed`
(403 / 404 / invalid request / archive invariant violation) and `halted` (401 credential
failure). A halt also flips the Budget object so every other Channel stops issuing requests
until an operator clears the halt after rotating the token.

## Budget coordination is fail-closed

Reports of 401, 403 and 429 responses feed application-wide state in the Budget object (the
invalid-request budget, the global pause and the credential halt). When such a report cannot be
delivered, the Channel does not act on the response locally and does not issue another Discord
request. Instead it stores the report and the outcome it would have applied as a pending
coordination in SQLite, moves to `waiting`, and on each following Alarm retries the report first.
Only once the Budget object has recorded the report is the deferred outcome applied (halt, fail
or wait); clearing the pending record and storing that outcome happen in one SQLite
transaction, so a crash after the report succeeded can only replay the report, never lose the
outcome. Pending coordination survives restarts because it lives in the run record. Reports of
2xx and 5xx responses remain advisory: a failed delivery is logged and progress continues.

## Configuration

Variables are declared in `wrangler.jsonc`; secrets are injected with `wrangler secret put` and
never committed.

* `DISCORD_BOT_TOKEN` (secret): bot token used for Get Channel Messages.
* `BACKFILL_API_TOKEN` (secret): bearer token expected by the API Worker.
* `ENVIRONMENT`: prefix for Durable Object names, isolating dev / beta / prod state.
* `DISCORD_API_BASE_URL`, `DISCORD_API_VERSION`: Discord REST endpoint and version recorded in
  provenance.
* `DISCORD_MESSAGE_CONTENT_ENABLED`: whether the application has the Message Content intent,
  recorded in provenance as a completeness capability.
* `BACKFILL_PAGES_PER_ALARM`, `BACKFILL_PAGE_LIMIT`: bounded work per Alarm and default page size.
* `DISCORD_GLOBAL_REQUESTS_PER_SECOND`, `DISCORD_INVALID_REQUEST_BUDGET`,
  `DISCORD_INVALID_REQUEST_WINDOW_SECONDS`: safety ceilings for the Budget object. They are not
  route limits; route limits always come from response headers.

## Development

Install dependencies with pnpm from the repository root, then run `pnpm lint`, `pnpm typecheck`
and `pnpm test`. Tests execute inside workerd through the Cloudflare Vitest plugin; each test
uses its own Durable Object name and purges the miniflare R2 bucket before asserting on object
counts. The fault-injection suite in
`test/do/backfill-channel.test.ts` is the reproducible proof required by the issue's Definition
of Done: it terminates execution after the HTTP fetch, after the R2 write, and after the
progress update, duplicates Alarm execution, injects 429 / 401 / 403 / 5xx responses and
evicts the Durable Object, and asserts that every page ends up in the Archive exactly as the
design predicts.

## Real Discord smoke procedure (dev)

Automated tests run against fakes and miniflare only. A one-off smoke against real Discord
needs the following inputs, none of which are stored in the repository:

* an R2 bucket named `ojiverse-dwh-observations-dev` in the target Cloudflare account;
* the secret `DISCORD_BOT_TOKEN` (bot with `VIEW_CHANNEL` and `READ_MESSAGE_HISTORY` on the
  target Channel) set with `wrangler secret put DISCORD_BOT_TOKEN --env dev`;
* the secret `BACKFILL_API_TOKEN` set with `wrangler secret put BACKFILL_API_TOKEN --env dev`;
* a DWH-public test Guild / Channel pair to crawl.

Then deploy with `wrangler deploy --env dev`, start a run through `POST /v1/backfill/runs`
with a small `page_limit`, poll `GET /v1/backfill/channels/{channel_id}` until the state is
`completed`, and confirm the objects under `observations/v1/source=http_backfill/` in the bucket
and the route bucket values recorded in the run's provenance.
