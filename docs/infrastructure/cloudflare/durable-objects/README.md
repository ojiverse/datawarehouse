# Cloudflare Durable Objects インフラ設計

本ディレクトリでは stateful coordination に使用する Durable Objects の class と ownership boundary を定義する。

すべての新規 class は SQLite-backed Durable Objects を使用する。

## Gateway Session Durable Object

Gateway Session Durable Object は、Cloudflare 上の1 Gateway Instance が所有する1 Discord Gateway Session の stateful owner とする。

同じ shard assignment に別 Gateway Instance が存在することを許容し、shard ID だけで global singleton を作らない。

durable state には少なくとも Gateway Instance identity、Session ID、resume gateway URL、shard assignment、Accepted Sequence を保持する。

Received Sequence は connection-local state として管理し、Accepted Sequence と分離する。

R2 Archive commit 後にのみ Accepted Sequence を前進させる。

## Backfill Channel Durable Object

Backfill は Discord Channel を coordination atom とする。

environment と Channel ID から安定して同じ Durable Object を解決し、同一 Channel に複数 active run を許可しない。

SQLite storage に Run ID、target range、pagination position、run state、last archived page、next execution time、terminal error を保持する。

page continuation には Alarm を使用する。

Alarm の at-least-once execution を前提に handler を idempotent にする。

## Discord HTTP Budget Durable Object

Discord application / Bot 単位で共有される HTTP global rate limit と invalid-request budget を協調するため、application 単位の Budget Durable Object を使用する。

各 Backfill Channel Durable Object は request 実行前後に Budget owner と協調し、global ceiling と invalid-request budget を超えないようにする。

route-specific bucket state は Channel Durable Object 側で response-driven に管理する。

## Gateway Identify Coordinator Durable Object

Discord application ごとに1つの Identify Coordinator Durable Object を配置する。

Get Gateway Bot 由来の session_start_limit、max_concurrency bucket、Identify lease を durable に管理し、Cloudflare 内外の全 Gateway Instance の新規 Identify を直列化・制限する。

外部 Gateway Instance は authenticated Worker endpoint を経由して lease を取得する。

## Worker と Durable Object の境界

stateless Worker は authentication、validation、routing、response formatting を担当する。

strong consistency、per-entity serialization、durable progress、scheduled continuation が必要な責務だけを Durable Object に置く。

Durable Object を単なる stateless request handler として利用しない。

## Storage

Durable Object storage は control / coordination state のために使用する。

Observation Archive や Canonical Data の primary data store として使用しない。

SQLite storage の transaction と Point-in-Time Recovery は運用上利用できるが、Observation Archive の代替にはしない。

## Gateway Runtime Verification

outbound WebSocket は hibernation 対象外であり、outbound connection が eviction を防ぐ効果には時間上限がある。

#18 の technical verification が完了するまで、Durable Object が Discord Gateway Session を無期限に維持できることを確定事実として扱わない。

検証結果によって Gateway runtime を変更しても、Session state と Accepted Sequence の domain semantics は維持する。
