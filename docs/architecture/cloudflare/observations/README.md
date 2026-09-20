# Cloudflare Observation アーキテクチャ

ドメイン上の Observation を Cloudflare 上で durable に保存する write path を扱います。

## 対象

- Observation の受理から durable storage までの境界
- Buffering と batching
- Retry
- Duplicate
- Storage failure
- Gateway と Backfill の共通 ingestion path

## 現時点の方向性

R2 を Observation Archive の主要 storage として利用する方針です。

Observation Archive は Canonical Store より先に durable になることを重視します。

大量の小さな object を無条件に生成せず、運用コストと failure semantics を踏まえた batching を検討します。

## 対象外

Bucket 名、lifecycle rule、binding、region policy などの resource configuration はインフラストラクチャ設計で扱います。

## 今後分割する詳細設計

- Archive Write Path
- Observation Batching
- Retry and Duplicate Handling
- Archive Failure Semantics
