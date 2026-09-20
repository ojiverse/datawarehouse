# Cloudflare コストとキャパシティ設計

本ディレクトリでは Discord DWH の Cloudflare resource consumption を評価する基準と、コスト最適化が correctness を侵食しないための原則を定義する。

## 基準構成

Production では Workers Paid を基準構成とする。

ただし Gateway runtime の最終構成は #18 の Beta verification 後に確定し、Cloudflare 外 Gateway Instance を併用できる。

## 主な Cost Driver

* Observation Archive の R2 PUT 数と保存容量
* Backfill Channel Durable Object の request / SQLite operation
* Gateway Instance ごとの Durable Object duration / request
* Canonical Parquet / Iceberg の R2 storage
* R2 SQL の query / scan usage
* Reconciliation の HTTP / R2 write 頻度

Archive は1 Observation = 1 object を correctness baseline とするため、R2 PUT 数を継続的に実測する。

operation cost が支配的要因になった場合のみ、domain identity を保った immutable compaction / segment 化を再設計する。

コスト削減だけを理由に source payload を drop、truncate、sample、overwrite してはならない。

## Durable Objects

Gateway cost は shard 数ではなく Cloudflare 上で active な Gateway Instance / Session owner 数を基準に評価する。

Backfill cost は同時に active な Channel DO と crawl frequency を基準にする。

Raspberry Pi 等の Cloudflare 外 Gateway Instance 自体は Durable Object duration を消費しない。

## Queue

first-MVP の core path に Queue cost は存在しない。

将来 incremental Canonical trigger に Queue を導入した場合のみ operation / retention / DLQ cost を追加評価する。

## Canonical

PyIceberg materializer を外部 runtime で実行する場合、その compute cost を Cloudflare cost と分離して記録する。

R2 Data Catalog の managed compaction / snapshot expiration を優先し、独自 maintenance compute の重複を避ける。

## 不変条件

* Observation Archive の durability をコスト最適化より優先する
* Product Policy の collection scope をコストだけで縮小しない
* コスト変更で保証レベルを変える場合は ADR を残す
* 単価 / included quota の具体値は Cloudflare pricing の変更に追従して更新する
