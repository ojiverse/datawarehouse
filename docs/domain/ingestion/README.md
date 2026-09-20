# Ingestion Domain

Discord Gateway から継続的に Observation を取得するための Domain Design を扱います。

## Scope

- Gateway connection と logical session の関係
- Identify と Resume
- Sequence の意味
- Heartbeat と liveness
- Dispatch event の受理
- Connection loss と session recovery
- Observation へ引き渡すまでの delivery semantics

## Non-scope

- Cloudflare 上で Gateway connection をどの resource が所有するか
- Timer や永続状態をどう実装するか
- Queue や R2 の具体的な利用方法
- HTTP Backfill の crawler 設計

HTTP Backfill は [../backfill/README.md](../backfill/README.md) で扱います。

## 基本原則

Gateway connection の永続性は保証として扱いません。

Connection loss は例外ではなく、通常運用で発生し得る事象として扱います。

可能な場合は Discord Gateway の Resume により logical session を継続し、Resume できない場合は HTTP Backfill による回復へ委ねます。

Gateway 経由で取得できる event history と、HTTP API から後から取得できる state snapshot は異なるものとして扱います。

## 今後分割する詳細設計

- Gateway Session Lifecycle
- Heartbeat and Liveness
- Resume and Recovery
- Gateway Event Semantics
- Event Delivery Semantics
- Backpressure and Failure Semantics

各文書が 200 行以内になるよう concern ごとに分割します。
