# Cloudflare Queues インフラ設計

本ディレクトリでは Cloudflare Queues を **再生成可能な非同期 trigger / buffer** として利用する場合の invariant を定義する。

## Core Data Path からの除外

first-MVP の HTTP Backfill、Observation Archive write、Backfill continuation には Cloudflare Queues を使用しない。

Gateway Observation の Durable Acceptance にも Queue を使用せず、R2 Archive commit を durability boundary とする。

Queue message を Observation、Backfill progress、Canonical correctness の唯一の根拠にしてはならない。

## Canonical Trigger Queue

将来 low-latency incremental materialization が必要になった場合、R2 Event Notification 等から Canonical processing を起動する trigger queue を追加できる。

queue message には Archive object identity / key 等の再取得可能な pointer を載せ、Source payload の唯一の copy を載せない。

trigger が loss / expiry しても Archive listing と replay / rebuild で Canonical が収束できなければならない。

## Delivery Semantics

Cloudflare Queues は at-least-once delivery であり duplicate delivery を正常系として扱う。

consumer は idempotent でなければならない。

Queue send の成功を Source-of-Evidence の Durable Acceptance と定義しない。

## Dead Letter Queue

push consumer を持つ Queue には DLQ を必須とする。

retry exhaustion を silent deletion として扱わず、DLQ arrival を alert / investigation 対象とする。

DLQ 自体も長期 durability store ではない。DLQ retention が切れても Source of Evidence を失わない topology を維持する。

必要な failure record は R2 control / operational log へ退避できるが、Observation Archive の代替にしない。

## Retention

Paid plan で Queue を使用する場合は、correctness を損なわない範囲で最大 retention を優先する。

Free plan の短い retention や DLQ retention を前提に、Source data を Queue にしか存在させる設計は禁止する。

## 現在の Resource Baseline

first-MVP に必須の Queue resource は存在しない。

canonical trigger queue と DLQ は incremental materialization を導入する段階で作成する。
