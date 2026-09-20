# Observation Archive Write Path

本書では Observation を Cloudflare R2 の Observation Archive へ永続化する write path を定義する。

## Direct-to-R2

Observation Archive への durable write path に Cloudflare Queues を使用しない。

HTTP Backfill、Cloudflare Gateway、Reconciliation は、各 producer から Observation を R2 へ直接 commit する。

Cloudflare 外の Gateway Instance は authenticated ingest Worker を経由し、その Worker が R2 commit を完了した後にのみ成功を返す。

これにより Queue の message size、retention、retry exhaustion、DLQ retention を Source of Evidence の durability から切り離す。

## Durable Acceptance

Cloudflare architecture における Observation の Durable Acceptance は **Observation Archive への R2 commit 成功時点**とする。

producer は R2 put の成功を確認する前に source progress、Backfill cursor、Gateway accepted sequence を進めてはならない。

R2 commit の成否が不明な場合は loss を仮定せず retry または source replay を許容し、duplicate 側へ倒す。

## One Observation per Object

現行 baseline では1 Observation を1 R2 object として保存する。

R2 operation 数を減らすための Archive batching は correctness の前提にせず、実測で必要性が確認された場合だけ将来設計する。

Observation identity は object placement から独立しているため、将来 compaction / segment 化しても domain contract を変更しない。

## No Overwrite

Archive write は create-only とする。

R2 conditional put を利用して、既存 key を上書きしない。

同じ Observation ID の retry で既存 object が存在した場合は、保存済み object の Observation ID と source payload hash を確認する。

一致する場合のみ idempotent success と扱う。不一致の場合は UUID collision または実装 bug による invariant violation として失敗させ、既存 object を変更しない。

Source payload hash には SHA-256 を使用する。

## Payload Size

Observation payload を Cloudflare Queues の message size に合わせて切り詰めてはならない。

producer が受信できた Discord payload は Envelope contract に従って全体を R2 へ保存する。

Worker / Durable Object の memory limit に近い payload では streaming API を優先し、不必要な複製 buffer を作らない。

## Failure Handling

Archive write failure は source ingestion failure として扱い、Canonical processing へ進めない。

HTTP Backfill は durable progress を前進させず再試行する。

Gateway は accepted sequence を前進させず、必要に応じて connection を切断して accepted sequence から Resume する。

Canonical materialization failure は Archive write failure へ波及させない。

## 外部 Producer

Cloudflare 外の producer に R2 bucket の広範な credential を直接配布することを必須としない。

標準構成では authenticated ingest Worker が Envelope validation、DWH-public admission、conditional R2 put を実行し、commit 完了後に応答する。

外部 producer は成功応答を Durable Acceptance とみなす。
