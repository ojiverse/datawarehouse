# Cloudflare Pipelines インフラ設計

本ディレクトリでは Cloudflare Pipelines を将来の incremental Canonical materialization に利用する場合の境界を定義する。

## first-MVP では採用しない

first-MVP の authoritative Canonical materializer / Rebuild には PyIceberg を使用する。

Pipelines は open beta であり、Observation Archive からの完全 rebuild capability を置換しない。

## Source Constraint

現在の Pipelines は HTTP endpoint、Worker binding、Logpush 等の対応 source から stream へ ingest する。

Cloudflare Queues や R2 Event Notifications を Pipelines の direct source として記述してはならない。

Queue / R2 notification を起点に Pipelines へ送る場合は、対応 source へ bridge する Worker 等を明示する必要がある。

## Archive Independence

Observation Archive write を Pipelines の成功に依存させない。

Pipelines へ dual-write する場合も R2 Observation Archive commit を先に成立させ、Pipelines failure が ingestion を阻害しないようにする。

## Iceberg Sink

R2 Data Catalog sink を利用する場合、data file format は Parquet となる。

compression は Cloudflare の default と一致する Zstandard を baseline とする。

Pipelines で生成した Canonical data も、同じ Observation set と projection semantics から PyIceberg rebuild で再生成可能でなければならない。

## 採用条件

Pipelines を production incremental path に採用する前に、beta status、pricing、source limitation、failure isolation、replayability を再評価する。

Pipelines 固有 transform にしか存在しない business semantics を作らない。
