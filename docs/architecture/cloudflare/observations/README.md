# Cloudflare Observation アーキテクチャ

本ディレクトリでは、Gateway、HTTP Backfill、Reconciliation 等の producer が Observation を Cloudflare R2 の Observation Archive へ永続化する write path を定義する。

## Direct-to-R2 を標準とする

Observation Archive の write path では Cloudflare Queues を durability boundary にしない。

Cloudflare 内の producer は Observation を R2 へ直接 commit する。

Cloudflare 外の producer は authenticated ingest Worker を経由し、Worker が R2 commit を完了してから成功応答を返す。

これにより Queue retention、message size、retry exhaustion、DLQ retention を Source of Evidence の correctness から排除する。

詳細は [archive-write-path.md](archive-write-path.md) に定義する。

## Durable Acceptance

Cloudflare architecture では **R2 Archive commit の成功そのものを Durable Acceptance** とする。

source progress、Backfill cursor、Gateway accepted sequence は R2 commit 成功前に前進させない。

commit 結果が不明な場合は retry / replay により duplicate 側へ倒す。

## Object Granularity

現行 baseline は **1 Observation = 1 R2 object** とする。

小 object の operation cost を理由に、初期設計から batching を correctness path へ導入しない。

将来 batching / compaction が必要になっても Observation identity は物理 object から独立しているため、domain contract は変更しない。

## Multi-producer Provenance

同じ Discord 上の出来事を複数 producer が観測することを正常状態とする。

write path では semantic deduplication を行わず、各 Observation の provenance を保持して保存する。

Canonical reconciliation は Processing の責務とする。

## Failure Isolation

Canonical Store、R2 Data Catalog、Pipelines、Query 等の障害は Observation Archive write を阻害してはならない。

Observation Archive が durable commit された後の downstream processing はいつでも replay / rebuild 可能とする。

## 詳細設計

* [archive-write-path.md](archive-write-path.md): direct R2、conditional put、payload size、外部 producer

## 今後分割する詳細設計

* **Archive Compaction**: 実測で必要になった場合の immutable segment 化
* **External Ingest Authentication**: Cloudflare 外 producer の credential / admission
