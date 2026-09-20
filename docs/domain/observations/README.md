# 観測ドメイン（Observation）

本ディレクトリでは、Observation Archive に保存される「Discord から観測された事実」の定義、セマンティクス、およびデータ不変条件を規定する。

## Observation の役割と位置づけ

Observation は、分析用モデル（Canonical Data）へ変換される前段に位置する**唯一無二の事実証跡（Source of Evidence）**である。

リアルタイムに受信した Gateway イベントおよび HTTP API クロールにより取得したレスポンスの双方が Observation として記録される。

保存対象は [Product Policy](../product-policy/README.md) に従う。DWH-public scope の durable community activity は将来の利用有無にかかわらず原則保存し、Presence、Typing、Voice State のような deliberate absence は Observation Archive に取り込まない。

## 取得経路（Provenance）の厳格な分離

HTTP Backfill によって取得されたメッセージを、未受信の Gateway イベント（`MESSAGE_CREATE` 等）として偽装・変換することを禁じる。

リアルタイム Gateway においては投稿直後の状態、中間編集履歴、削除イベントといった時系列変化を観測可能であるが、HTTP API においてはリクエスト時点で Discord 上に残存する状態が取得される。

取得経路に応じた完全性保証の差異を保護するため、システムは取得経路および観測時刻を来歴として保持し、後続処理において区別可能とする。

Gateway 由来の Observation では、どの Gateway Instance、どの Discord Gateway Session、どの shard assignment から取得したかを追跡可能にする。

## Identity の責務分離

Observation の識別子は「本システムが行った一回の観測」を一意に識別する責務だけを持つ。

Observation ID に、Discord 上のエンティティ識別、Gateway Session 内の順序、複数 Gateway 間の重複判定、または全体の時系列順序を兼務させない。

Discord の Guild、Channel、Thread、Message 等の識別には Discord が発行する Snowflake を使用し、Observation ID と分離する。

Gateway 由来の `session_id` と `sequence` は、その Gateway Session 内における source delivery の追跡と Resume に利用する。複数 Session 間の共通イベント ID として扱わない。

複数 Gateway Session が同じ Discord 上の出来事を観測した場合、それぞれ異なる Observation として保存する。Canonical 側で必要な reconciliation を行う。

## Observation に求められる中核要件

すべての Observation は、インフラの物理レイアウトに依存せず、論理的に以下の情報を保持しなければならない。

* **Observation の一意な識別性**: システム全体で各観測を一意に特定できること
* **取得元と経路の明示**: Gateway、HTTP Backfill、定常照合などの取得経路を判別できること
* **観測時刻**: Discord 側の時刻とは独立に、システムが当該データを観測した時刻を保持すること
* **Discord エンティティの参照**: Guild ID、Channel ID、Message ID 等を Discord Snowflake の意味を失わず保持すること
* **生ペイロードの完全保持**: 将来のスキーマ変更や再処理に備え、Discord から受信した元のペイロードを保持すること
* **Gateway provenance**: Gateway 由来の場合、Gateway Instance、`session_id`、`sequence`、shard assignment を追跡できること
* **Backfill provenance**: HTTP 由来の場合、一連の取得実行を表す run identity と取得位置を追跡できること
* **Envelope version**: Observation Archive の契約変更を識別できる version を各 Observation から判別できること

## 重複と整合性の不変条件

* **通常取り込みでの追記優先**: 日常的な ingestion では既存 Observation の更新ではなく、新しい観測の追記を基本とする
* **欠損防止のための重複許容**: Durable Acceptance 後の At-least-once 配送では retry による重複が正常に発生し得る。加えて Resume replay、複数 Gateway による並行観測、HTTP pagination overlap 等に伴う重複も許容する
* **Source-local な重複と意味的な重複の分離**: At-least-once retry や同一 Session の再送による source-local duplicate と、異なる Session が同じ出来事を観測した semantic duplicate を同じ問題として扱わない
* **追跡可能性の保証**: Canonical Data から、その状態の根拠となった Observation 群と元の取得経路まで辿れること

## 詳細設計

* [identity.md](identity.md): UUIDv7 Observation ID、Discord Snowflake、Source Delivery Identity、Run Identity
* [envelope.md](envelope.md): Observation Envelope v1、HTTP page / Gateway frame、Provenance と observed time

## 今後分割する詳細設計

* **Observation Identity Evolution**: identity contract の将来変更と migration
* **Discord Entity Identity**: Snowflake の無損失な保持と Canonical への変換規則
* **Source Delivery Identity**: Gateway Session 内および HTTP 取得内の再送・重複追跡
* **Gateway Provenance**: Gateway Instance、Session、shard、sequence の関係
* **Duplicate Semantics**: source-local duplicate と cross-session observation の reconciliation
* **Completeness Model**: Gateway 観測と HTTP 観測におけるデータ完全性の差異定義
