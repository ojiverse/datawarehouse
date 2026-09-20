# Cloudflare デプロイ・マイグレーション設計

本ディレクトリでは Cloudflare 上の Worker、Durable Objects、R2、Data Catalog 等を安全かつ再現可能にデプロイ・移行・ロールバックする原則を定義する。

## CI/CD と構成管理

リソース構成とコードのデプロイは Git repository を構成の事実源として自動化する。

* Cloudflare resource / Worker deployment には Wrangler を使用する
* dev / beta / prod の configuration と secrets を分離する
* GitHub Actions 等から test、migration、deployment を再現可能に実行する

## Gateway Deployment

Gateway correctness を graceful shutdown に依存させない。

Gateway Session の Accepted Sequence は通常の ingestion path で継続的に durable storage へ保存され、R2 Observation Archive へ commit 済みの highest contiguous sequence を表す。

deployment により WebSocket が予告なく切断されても、再起動後は保存済み Session state と Accepted Sequence から Resume を試行する。

Received Sequence が Accepted Sequence より進んでいた場合は replay duplicate を許容する。

Resume 不能な場合は新規 Session を確立し、HTTP Backfill / Reconciliation により surviving state を回復する。

したがって deployment は「イベント欠損ゼロ」を保証しない。Archive commit 前に失われた event や Session loss 後に HTTP で復元不能な transient history が存在し得る。

## Durable Object Migration

Durable Object class や SQLite schema を変更する際は Cloudflare の migration mechanism を使用する。

migration 前後で Gateway Accepted Sequence、Backfill progress、HTTP budget state の意味を変えない。

control state の schema migration に失敗した場合に Production data を失わない rollback path を用意する。

## Canonical Deployment

Canonical materializer の version と projection version を明示的に関連付ける。

大規模 schema / projection 変更では既存 table を in-place に破壊せず、新しい Canonical table を Observation Archive から rebuild して validation 後に切り替える。

R2 Data Catalog / Iceberg metadata は rebuildable であり、Observation Archive より高い永続性を要求しない。

## Rollback

application code の rollback が Observation Archive の既存 object を変更してはならない。

Envelope / Product Policy / Domain semantics の backward-incompatible change は単純な code rollback では解決せず、versioned migration または新 ADR を要求する。
