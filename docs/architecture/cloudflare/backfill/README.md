# Cloudflare Backfill アーキテクチャ

本ディレクトリでは、Discord HTTP API を利用して過去ログや欠損区間を取得する Backfill を Cloudflare 上で中断・再開可能に実行するアーキテクチャを定義する。

## 実行モデル

first-MVP の Backfill coordination には SQLite-backed Durable Objects を採用する。

stateless Worker は認証、入力検証、run 開始、status 取得等の API boundary を担当する。

各 Channel の stateful execution は Channel 単位の Durable Object が所有し、同一 Channel の active run を直列化する。

Durable progress は Durable Object SQLite storage に保存し、Queue message や process memory を progress authority にしない。

詳細は [execution.md](execution.md) に定義する。

## Continuation

長時間 run の self-continuation には Durable Object Alarm を使用する。

Alarm は at-least-once execution であるため、各 execution step は再実行可能に設計する。

1 invocation で run 全体を完走させず、bounded な page 処理単位で progress を commit して次の Alarm を設定する。

## Archive Write

Backfill が取得した HTTP response page は Observation Envelope v1 として R2 Observation Archive へ直接保存する。

Cloudflare Queues は first-MVP の Archive write path に使用しない。

R2 commit が完了した後にのみ Backfill cursor を前進させる。

R2 write 後、progress 更新前に runtime が停止した場合は同じ scope を再取得し、新しい Observation として追記する。

## Rate Limit

Discord route limit は response header と Retry-After に従い、固定 pacing だけに依存しない。

Channel-local route bucket state は Backfill Channel Durable Object が保持する。

application 全体の global request ceiling と invalid-request budget は application 単位の Discord HTTP Budget Durable Object が協調する。

401 は credential failure、403 は対象 scope の terminal access failure、429 は Retry-After に従う一時停止として扱う。

## Reconciliation

Reconciliation は既存の Backfill execution path を再利用する。

直近 scope を HTTP で再観測し、Archive や Canonical と事前比較せず新しい Observation として追記する。

重複は Processing の deterministic projection で収束させる。

## 詳細設計

* [execution.md](execution.md): Durable Object ownership、progress、Alarm、page commit order、rate-limit coordination

## 今後分割する詳細設計

* **Gap Recovery**: Gateway session loss から Backfill range を決める規則
* **Reconciliation Scheduling**: 定期実行 interval と target policy
