# 取り込みドメイン

Discord Gateway から継続的に Observation を取得するためのドメイン設計を扱います。

## 対象

- Gateway connection と logical session の関係
- Identify と Resume
- Sequence の意味
- Heartbeat と liveness
- Dispatch event の受理
- Connection loss と session recovery
- Observation へ引き渡すまでの delivery semantics

## 対象外

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

各文書が 200 行以内になるよう関心事ごとに分割します。
