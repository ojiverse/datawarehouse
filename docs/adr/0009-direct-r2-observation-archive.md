# ADR-0009: Observation Archive は direct-to-R2 とする

* **ステータス**: 承認（Accepted）
* **決定日**: 2026-09-20
* **対象領域**: Observation Archive / Cloudflare Architecture

## コンテキスト

初期設計では Cloudflare Queues で Observation を batch 化してから R2 へ保存する構成を想定していた。

しかし Queue には message size、retention、retry exhaustion、DLQ retention の制約があり、Queue write を Durable Acceptance とすると Source of Evidence の durability が Queue lifecycle に依存する。

また Product Policy では将来利用するか未定の durable community activity も原則 Archive するため、source payload を size limit に合わせて欠損させることは許容できない。

## 決定

Observation Archive の標準 write path は direct-to-R2 とする。

* Cloudflare 内 producer は R2 へ直接 commit する
* Cloudflare 外 producer は authenticated ingest Worker が R2 commit した後に成功応答を返す
* R2 commit 成功を Cloudflare architecture 上の Durable Acceptance とする
* 1 Observation を1 R2 object とする
* Archive object は UTF-8 JSON を gzip 圧縮する
* 通常 ingestion の Archive write は conditional create-only とし overwrite しない
* Product Policy に基づく explicit erasure だけは compliance rewrite / delete の例外とする
* Observation ID は UUIDv7 とし object key から独立させる
* R2 key は Source Kind と Observed At の UTC 時間軸で partition する

## Queue の位置づけ

Cloudflare Queues は Source-of-Evidence の delivery path から外す。

将来 Canonical freshness のために Queue を利用する場合も、Archive object pointer を運ぶ再生成可能な trigger としてのみ扱う。

## 結果

Queue message size や retention が Observation loss の原因にならず、HTTP / Gateway / external producer で同じ Archive durability boundary を共有できる。

一方で R2 PUT 数は増加する。

現時点では correctness と単純性を優先し、Archive batching は実測で operation cost / rebuild throughput が支配的要因になった場合にだけ再検討する。
