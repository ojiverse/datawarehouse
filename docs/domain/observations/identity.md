# Observation Identity

本書では Observation Archive で利用する identity の責務と具体方式を定義する。

## Observation ID

Observation ID には RFC 9562 の **UUIDv7** を採用する。

UUIDv7 は各 producer が中央協調なしで生成し、Observation を生成した時点から Archive まで一貫して同じ値を使用する。

Observation ID は Durable Acceptance より前に生成する。

同じ Observation を write retry する場合は同じ Observation ID を再利用する。一方、Discord HTTP API への再取得や Gateway replay によって新たに観測し直した場合は、新しい Observation ID を生成する。

UUIDv7 に含まれる timestamp は identity の生成時刻にすぎず、Discord event の発生時刻、observed time、複数 producer 間の全体順序を表すものとして利用してはならない。

UUIDv7 の追加 monotonic counter をシステム全体で協調させない。分散 producer 間の order は Observation ID の責務ではない。

## Discord Entity Identity

Guild、Channel、Thread、Message、User 等の Discord entity は Discord Snowflake で識別する。

Snowflake は Discord API が提供する decimal string を無損失に保持し、Archive と Canonical の双方で文字列を canonical representation とする。

Snowflake を signed 64-bit integer のみで表現してはならない。

Snowflake から導出可能な作成時刻は別 field として materialize してよいが、元 Snowflake を置換しない。

## Source Delivery Identity

Observation ID と source delivery identity は別物である。

Gateway では Discord Gateway Session の session identity と sequence の組を source-local delivery の追跡に使用する。同一 Session 内の replay を識別できるが、Session を跨いだ global event identity には使用しない。

HTTP では Backfill run identity と request scope / pagination position により取得経路を追跡する。同じ scope を再取得した場合も新しい Observation とする。

## Run Identity

Backfill、Reconciliation、Rebuild 等の実行単位を識別する run identity にも UUIDv7 を使用する。

Run ID は実行の grouping と traceability のための identity であり、Observation identity や Discord entity identity を兼務しない。

## 物理配置との分離

Observation ID は R2 object key、batch identity、Parquet file path 等の物理配置から独立する。

現在の R2 layout が Observation ID を object key の一部として利用することは許容するが、object key から Observation ID を定義してはならない。

将来 object の移動、再圧縮、compaction、削除対応による rewrite が発生しても、Observation 自体の identity は変化しない。

## 参照仕様

* RFC 9562: UUID Version 7
* Discord Developer Documentation: Snowflake Reference
