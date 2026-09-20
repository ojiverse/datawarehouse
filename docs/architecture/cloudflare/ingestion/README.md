# Cloudflare 取り込みアーキテクチャ（Ingestion）

本ディレクトリでは、Discord Gateway との WebSocket 接続を Cloudflare 上で維持し、Dispatch を Observation Archive へ安全に永続化するアーキテクチャを定義する。

## Gateway Session Owner

Cloudflare 上の1 Gateway Instance が所有する1 Discord Gateway Session の stateful owner に Durable Object を使用する。

同じ shard assignment を別 Gateway Instance が並行して観測することを許容し、DO を shard 全体の唯一 owner とはみなさない。

各 Session は独立した session ID と sequence stream を持つ。

## Received / Accepted Sequence

Session state は Received Sequence と Accepted Sequence を分離する。

Received Sequence は最後に受信した Dispatch sequence であり、接続中の Discord Heartbeat に使用する。

Accepted Sequence は R2 Observation Archive への commit が完了した Dispatch の highest contiguous sequence であり、Durable Object SQLite storage に保存する。

Resume cursor には Accepted Sequence を使用する。

詳細な semantics は [domain/ingestion/delivery-semantics.md](../../../domain/ingestion/delivery-semantics.md) に従う。

## Archive Write

Gateway Dispatch を Cloudflare Queues へ handoff して Durable Acceptance とする構成は採用しない。

Gateway Durable Object は Observation Envelope を生成し、R2 Observation Archive へ direct write する。

R2 commit 成功後にのみ Accepted Sequence を前進させる。

同じ Session の Dispatch processing は sequence 順に commit し、accepted watermark に gap を作らない。

Archive write が継続的に失敗して安全な backlog 保持が困難な場合は connection を閉じ、Accepted Sequence から Resume して replay させる。

## Restart / Resume

Session ID、resume gateway URL、Accepted Sequence 等の Resume state は通常処理中に durable storage へ更新する。

deployment / shutdown 時の graceful flush に correctness を依存させない。

R2 commit 後、Accepted Sequence 保存前に crash した場合は Resume replay により duplicate Observation が生じ得る。これは loss より duplicate を選ぶ意図的な failure semantics である。

Resume 不能な Session loss では新しい Session を Identify し、surviving state の回復を HTTP Backfill / Reconciliation へ委譲する。

## Runtime Verification

Durable Object の outbound WebSocket は hibernation できず、outbound connection が eviction を防ぐ効果にも時間上限がある。

Gateway Beta に入る前に #18 を最優先で実測し、Discord heartbeat と DO lifecycle の組み合わせで長時間 Session を維持できるか確認する。

検証結果により runtime implementation を変更しても、Received / Accepted Sequence の semantics は維持する。

## 外部 Gateway Instance

Cloudflare 外の Gateway Instance は authenticated ingest Worker へ Observation を送信する。

ingest Worker は Envelope validation と R2 commit を完了してから成功応答を返す。外部 producer はこの成功応答を Durable Acceptance とみなす。

## 今後分割する詳細設計

* **Gateway Runtime Verification**: #18 の長時間 connection test
* **Identify Budget Coordination**: application-wide Session Start Limit
* **External Ingest Authentication**: non-Cloudflare producer の認証
