# Gateway Delivery and Resume Semantics

本書では Discord Gateway Session における sequence、Durable Acceptance、Resume の関係を定義する。

## 二つの Sequence Watermark

Gateway Session は少なくとも二つの sequence watermark を区別する。

* **Received Sequence**: 当該 Session で最後に受信した Dispatch sequence
* **Accepted Sequence**: Observation Archive への Durable Acceptance が完了した Dispatch のうち、欠番なく連続している最大 sequence

Received Sequence は接続中の protocol state であり、Discord Heartbeat が要求する last received sequence に使用する。

Accepted Sequence は data durability state であり、durable storage に保存する。

Received Sequence と Accepted Sequence を同一変数で管理してはならない。

## Accepted Sequence の前進

Accepted Sequence は Observation の Durable Acceptance が完了した後にのみ前進できる。

sequence N+1 の Archive commit が sequence N より先に完了しても、N が未 accepted である限り accepted watermark を N+1 へ進めてはならない。

Accepted Sequence は **highest contiguous accepted sequence** である。

Cloudflare Gateway Instance では、Observation の R2 commit 成功後に Durable Object storage の Accepted Sequence を更新する。

shutdown 時の flush に correctness を依存させない。

## Resume Cursor

process restart、runtime eviction、connection failure 等から Resume する際の cursor には Accepted Sequence を使用する。

Received Sequence が Accepted Sequence より進んでいた場合、その区間は replay により再受信することを許容する。

この replay による duplicate は正常系であり、Observation / Canonical の duplicate tolerance で処理する。

Accepted Sequence より新しい値を Resume cursor として送ってはならない。

## Archive Commit と Session State

Observation Archive への commit と Durable Object の Accepted Sequence 更新は同一 transaction にはできない。

R2 commit 後、Accepted Sequence 更新前に crash した場合は、Resume により当該 Dispatch が replay され、新しい Observation が追加され得る。

これは loss より duplicate を選ぶ意図的な failure semantics である。

## Archive Failure

R2 write が失敗した Dispatch は accepted とみなさない。

Archive failure が継続し、in-memory backlog を安全に保持できない場合は Gateway connection を意図的に閉じ、Accepted Sequence から Resume して再取得する。

## Session Loss

Discord が Session を無効化し Resume できない場合は新規 Session を Identify し、HTTP Backfill / Reconciliation で surviving state の回復を試みる。

HTTP では復元不能な transient history が存在するため、Session loss 後の完全な event history を保証しない。

## Cloudflare Runtime Constraint

Cloudflare Durable Object の outbound WebSocket は hibernation 対象ではなく、outbound connection が eviction を防ぐ効果には時間上限がある。

Gateway Beta に入る前に、Discord heartbeat と Durable Object lifecycle の組み合わせで長時間 Session が実運用上維持されることを #18 の technical verification で確認する。

この検証結果によって runtime implementation は変更し得るが、Received / Accepted Sequence と Resume cursor の domain semantics は変更しない。
