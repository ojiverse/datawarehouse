# ADR-0010: Backfill coordination に SQLite-backed Durable Objects を採用する

* **ステータス**: 承認（Accepted）
* **決定日**: 2026-09-20
* **対象領域**: Backfill Architecture
* **一部置換**: ADR-0003 の Queue message を Backfill progress authority とする決定

## コンテキスト

Backfill は長時間実行、中断、再開、rate-limit wait を通常状態として扱う。

Queue message のみへ cursor を保持すると message expiry や retry exhaustion で run state を失うため、durable progress invariant を満たさない。

Cloudflare Durable Objects は per-entity coordination、strongly consistent SQLite storage、Alarm を提供し、Channel 単位の Backfill ownership と一致する。

## 決定

* Discord Channel を Backfill coordination atom とする
* Channel ごとに SQLite-backed Durable Object を割り当てる
* 同一 Channel の active Backfill run は1つに直列化する
* run identity、target range、pagination cursor、state、next execution time を SQLite storage に保持する
* continuation には Durable Object Alarm を使用する
* stateless Worker は authentication / validation / routing に限定する
* HTTP page は Durable Object から R2 Observation Archive へ direct write する
* R2 commit 後にのみ progress を前進させる

Discord application 全体の global HTTP budget は application 単位の Budget Durable Object で協調する。

## ADR-0003 との関係

コア DWH data を D1 等の RDB へ保存しないという ADR-0003 の判断は維持する。

一方、Backfill cursor を Queue message のみに保持する部分は本 ADR で置換する。

Durable Object SQLite は DWH data store ではなく control-plane progress store として利用する。

## 結果

runtime restart や execution trigger loss から run を再開でき、Queue を Backfill correctness から除外できる。
