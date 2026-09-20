# Cloudflare Queues

Discord DWH で利用する Queue resource の topology と設定を扱います。

## 想定用途

- Gateway から Observation ingestion への handoff
- Backfill continuation
- Canonical processing
- Replay や rebuild の job

すべてを同じ Queue に載せることは前提にしません。

## 設計原則

Queue 自体を source of truth にしません。

Queue message の loss や expiry が長期データの loss に直結しない topology を目指します。

## 設計時に確定する事項

- Queue の分割単位
- Retention
- Retry
- Dead letter の扱い
- Producer と consumer
- Environment 分離
- Operation quota
