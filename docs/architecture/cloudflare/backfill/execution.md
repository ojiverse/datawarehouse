# Backfill Execution Architecture

本書では first-MVP の HTTP Backfill を Cloudflare 上で中断・再開可能に実行する具体構成を定義する。

Backfill API Worker、Backfill Channel Durable Object、Discord HTTP Budget Durable Object は **TypeScript** で実装する。

## Component Ownership

Backfill の stateful coordination には SQLite-backed Durable Objects を使用する。

HTTP entry Worker は認証、入力検証、start / status 等の stateless API boundary のみを担当し、Backfill progress を所有しない。

各 Channel の Backfill は、その Channel を coordination atom とする Backfill Channel Durable Object が所有する。

Durable Object identity は environment と Discord Channel ID から安定的に解決し、同一 Channel に対する active run を1つに直列化する。

Durable progress は Durable Object の SQLite storage に保存する。D1、Queue message、process memory を progress authority にしない。

## Durable Progress

各 active run は少なくとも以下を durable に復元できなければならない。

* Run ID
* target Guild / Channel
* requested range
* current pagination position
* run state
* last successfully archived page
* next eligible execution time
* terminal error state

Run ID には UUIDv7 を使用する。

## Continuation

ページ処理の継続には Durable Object Alarm を使用する。

Alarm は at-least-once で実行され得るため、Alarm handler 自体を idempotent にする。

1回の Alarm で無制限に crawl せず、bounded なページ数を処理した後、未完了なら次の Alarm を設定する。

これにより Worker / Durable Object の CPU、wall-clock、runtime restart を Backfill correctness から切り離す。

## Page Commit Order

各 page は必ず以下の順序で処理する。

1. durable progress から現在 cursor を読む
2. Discord HTTP API から page を取得する
3. Observation Envelope を生成する
4. Observation Archive へ direct R2 write する
5. R2 commit 成功後にのみ durable progress を次 cursor へ進める
6. 未完了なら次の Alarm を設定する

R2 commit 後、progress 更新前に停止した場合は同じ HTTP scope を再取得してよい。この再取得は新しい Observation ID を持つ新しい evidence とする。

## Discord Rate Limit

Discord の route limit は固定値をハードコードせず、response の rate-limit metadata を利用する。

Channel Durable Object は、自身が利用する route / top-level resource の bucket state を保持し、Remaining、Reset-After、Bucket、Retry-After に従って次回実行時刻を決める。

Bot application 全体で共有される global rate limit と invalid-request budget は、application 単位の Discord HTTP Budget Durable Object で協調する。

HTTP 401 を受けた場合は credential failure として全 Backfill を停止方向へ遷移させる。

HTTP 403 は当該 scope を inaccessible として terminal failure にし、無制限 retry しない。

HTTP 429 は Retry-After に従って再開を遅延させる。shared scope を除く 401 / 403 / 429 は invalid-request budget の観測対象とする。

Discord が公開する global request ceiling は safety ceiling として扱うが、route-specific limit の代替にはしない。

## First-MVP Scope

first-MVP では Thread discovery を行わず、明示指定された Channel の Message history のみを対象とする。

同一 Channel の concurrent run は許可しない。別 Channel の run は独立 Durable Object として並行実行できる。

## 参照仕様

* Cloudflare Durable Objects: SQLite storage / Alarms
* Cloudflare Durable Objects Best Practices: coordination atom
* Discord HTTP API Rate Limits
