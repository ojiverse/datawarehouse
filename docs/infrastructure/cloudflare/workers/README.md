# Cloudflare Workers

Discord DWH で利用する Worker resource の構成を扱います。

## 想定責務

Workers は HTTP endpoint、Backfill execution、Observation ingestion、processing など複数の application entry point で利用する可能性があります。

責務ごとに Worker を分割するか、単一 Worker 内の entry point として管理するかは未決定です。

## 設計時に確定する事項

- Worker の責務境界
- Environment ごとの配置
- Binding
- CPU と request quota
- Deployment 単位
- Failure isolation

Gateway session の所有は Durable Objects を主要候補として別途検討します。
