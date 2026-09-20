# ADR-0012: Gateway Durable Acceptance を R2 commit とし Accepted Sequence を Resume cursor とする

* **ステータス**: 承認（Accepted）
* **決定日**: 2026-09-20
* **対象領域**: Gateway Ingestion
* **一部置換**: ADR-0006 の session persistence / zero-loss migration に関する前提

## コンテキスト

Discord Gateway は connection 内で last received sequence を Heartbeat と Resume に利用する。

しかし Observation Archive へ未保存の Dispatch まで persisted Resume cursor を前進させると、runtime loss 後にその event を replay できず Observation loss が発生する。

また Queue を Durable Acceptance boundary にすると Queue lifecycle と payload size が Archive durability を制約する。

## 決定

Gateway Session は Received Sequence と Accepted Sequence を分離する。

* Received Sequence は接続中に最後に受信した Dispatch sequence とし Heartbeat に使用する
* Accepted Sequence は R2 Archive commit 済み Dispatch の highest contiguous sequence とする
* Accepted Sequence を Durable Object SQLite storage へ保存する
* restart / reconnect 時の Resume cursor には Accepted Sequence を使用する
* Gateway Observation は Queue を介さず R2 へ direct write する
* R2 commit 後にのみ Accepted Sequence を進める
* R2 commit 後、Accepted Sequence 保存前の crash は replay duplicate を許容する
* Archive failure で backlog を安全に保持できない場合は connection を閉じ Accepted Sequence から Resume する

## Heartbeat と Resume の区別

Discord Heartbeat が要求する last received sequence と、durability を守る persisted Resume watermark は同じ責務ではない。

接続中は Received Sequence を protocol heartbeat に返し、process recovery では Accepted Sequence まで巻き戻して duplicate replay を許容する。

## Runtime の未確定事項

Durable Object outbound WebSocket の lifecycle には platform-specific constraint がある。

#18 の verification は Gateway Beta 前の blocker とし、結果によって Gateway runtime implementation を変更できる。

ただし Received / Accepted Sequence と Archive-before-progress の semantics は runtime に依存しない。

## 結果

Gateway correctness は graceful shutdown や Queue retention に依存せず、loss より duplicate を選ぶ failure semantics に統一される。
