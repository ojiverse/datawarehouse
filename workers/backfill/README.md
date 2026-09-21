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
  (key layout, gzip, SHA-256, metadata). The shared contract fixture lives in
  `contracts/observation-envelope/v1`.
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
and `pnpm test`. Tests execute inside workerd through the Cloudflare Vitest plugin with an
isolated R2 bucket and Durable Object storage per test. The fault-injection suite in
`test/do/backfill-channel.test.ts` is the reproducible proof required by the issue's Definition
of Done: it terminates execution after the HTTP fetch, after the R2 write, and after the
progress update, duplicates Alarm execution, injects 429 / 401 / 403 / 5xx responses and
evicts the Durable Object, and asserts that every page ends up in the Archive exactly as the
design predicts.

Deploy with `wrangler deploy --env dev` after creating the observation bucket declared in
`wrangler.jsonc` and setting both secrets.
