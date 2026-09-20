# Cloudflare オブザーバビリティ

Cloudflare 上で稼働する Discord DWH の log、metric、alert の infrastructure を扱います。

## 観測対象

- Gateway connection
- Heartbeat
- Resume
- Observation ingestion
- R2 write
- Queue
- Backfill
- Canonical processing
- Canonical lag
- Error rate
- Resource quota

何を failure と判断するかはアーキテクチャ設計で定義し、ここではそれを観測する具体的なインフラストラクチャを設計します。

## 設計時に確定する事項

- Log destination
- Metric collection
- Dashboard
- Alert routing
- Retention
- Environment 分離
- Cost
