# Cloudflare 処理アーキテクチャ（Processing）

本ディレクトリでは、Observation Archive から Canonical Store を生成・再生成する processing architecture を定義する。

## Failure Isolation

正規化・Canonical materialization の障害が Observation Archive への ingestion を阻害してはならない。

Observation Archive が唯一の Source of Evidence であり、Canonical Store はいつでも rebuild 可能な Derived Data とする。

## first-MVP Materializer

first-MVP の materializer は Iceberg REST Catalog を interoperability boundary とし、**iceberg-go を第一候補として #36 で実証する**。

#36 が成功した場合は Go materializer を採用する。失敗した場合は別 implementation へ自動的に切り替えず、blocking reason を記録して再設計する。

materializer は Workers runtime の CPU budget に依存させず、local、CI、専用 batch runtime 等から R2 Data Catalog の Iceberg REST Catalog へ接続できる standalone process とする。

詳細は [materialization.md](materialization.md) に定義する。

## Physical Representation

Canonical table は Apache Iceberg で管理し、data file は Parquet、compression は Zstandard を baseline とする。

Discord Snowflake は decimal string のまま保持し、分析用 timestamp を別 column として導出する。

## Resumable Rebuild

Full Rebuild 開始時に入力 Archive object set を immutable manifest として固定する。

manifest、checkpoint、chunk state は専用 R2 control bucket に置き、Observation Archive と分離する。

manifest chunk ごとに deterministic な staging Parquet file を生成し、completed chunk を durable に再利用できるようにする。

Iceberg commit retry では既に登録済み data file を二重登録しない。

## Incremental Trigger

R2 Event Notifications、Cloudflare Queues、Pipelines 等は Canonical freshness を改善する optional trigger として利用できる。

trigger の delivery guarantee を Canonical correctness の前提にしない。

trigger が欠損しても Archive listing と replay / rebuild により収束可能でなければならない。

## Cloudflare Pipelines

Pipelines は将来の incremental materialization 候補とするが、first-MVP の authoritative rebuild mechanism には採用しない。

Pipelines を停止・廃止しても standalone Iceberg materializer で Canonical を再構築できる状態を維持する。

## 詳細設計

* [materialization.md](materialization.md): iceberg-go technical gate、rebuild manifest、checkpoint、Iceberg commit

## 今後分割する詳細設計

* **Incremental Materialization**: freshness optimization と replay convergence
* **Canonical Promotion**: rebuild table の validation と切り替え
