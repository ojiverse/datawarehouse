# ADR-0007: 複数 Gateway Instance と独立 Session の並行稼働を許容する

* **ステータス**: 承認（Accepted）
* **決定日**: 2026-09-20
* **対象領域**: ドメイン設計 / アーキテクチャ設計
* **置換対象**: ADR-0006 の「1 shard につき1つの Gateway Session を排他的に所有する」という前提

## コンテキストと課題

当初は、1つの Discord shard に対して本システム側の Gateway Session も1つだけ存在する前提で設計していた。

しかし Discord Gateway では、同じ shard assignment を持つ独立した複数 Session を同時に確立できる。

また本システムでは、将来的に Cloudflare だけでなく Raspberry Pi 等の別実行環境へ Gateway collector を配置する可能性がある。

移行期間、ローリング更新、検証、将来の冗長化を個別の特別モードとして設計すると、Gateway の状態モデルと Observation provenance が実行環境ごとに複雑化する。

## 決定事項

Gateway を単一の取り込み endpoint としてではなく、複数存在可能な observer fleet として扱う。

本システム側の観測主体を **Gateway Instance** と呼び、各 Gateway Instance は1つ以上の独立した Discord Gateway Session を所有できる。

同じ shard assignment に対し、複数 Gateway Session が並行して存在することを正常状態として許容する。

各 Gateway Session は Discord が発行する独立した `session_id` と `sequence` 系列を持つ。

`sequence` は当該 Session 内の順序と Resume にのみ利用し、Session を跨いだ共通 event identity または全体順序として扱わない。

## Observation への影響

複数 Gateway Session が同じ Discord 上の出来事を観測した場合、それぞれを独立した Observation として保存する。

Observation Archive の write path では cross-session duplicate を無理に統合しない。

Canonical processing が Discord entity と event semantics に基づいて必要な reconciliation を行う。

Observation からは、少なくとも Gateway Instance、Discord Gateway Session、shard assignment、sequence を追跡可能にする。

## 移行と Resume の分離

一時的な connection failure から同じ Gateway Session を継続する場合は Resume を利用する。

Cloudflare から Raspberry Pi への移行や、新旧 deployment の切り替えでは、既存 Session の state を新 Instance へ移植することを前提としない。

新 Gateway Instance が独立 Session を開始し、新旧 Instance を一定期間並行稼働させた後に旧 Instance を停止する。

このため platform migration を特別な domain state として持たない。

## Durable Objects への影響

ADR-0006 で決定した、Cloudflare 上の Gateway Session を Durable Objects で所有する方針は維持する。

ただし Durable Object は shard 全体の唯一の owner ではなく、Cloudflare 上の特定 Gateway Instance が持つ特定 Session の owner として扱う。

DO instance identity は shard ID だけで決定せず、Gateway Instance と shard assignment を区別できる必要がある。

## 結果とトレードオフ

### 利点

* Cloudflare、Raspberry Pi、将来の別 runtime を同じ Gateway Instance model で扱える
* 移行時に新旧 Gateway を overlap させられる
* Gateway 側の障害や deployment と Observation ingestion の可用性を分離しやすい
* permanent dual-active、canary、移行 overlap を同じ domain capability の上に構築できる

### コスト

* 同一 Discord event に対応する複数 Observation が正常に発生する
* cross-session reconciliation が Canonical processing の責務として必要になる
* Gateway Instance と Gateway Session の identity を別々に管理する必要がある
* Cloudflare 内で複数 Gateway Instance を常時動かす場合は Durable Objects の利用量が増加する
