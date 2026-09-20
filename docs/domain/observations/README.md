# 観測ドメイン（Observation）

本ディレクトリでは、Observation Archive に保存される「Discord から観測された事実」の定義、セマンティクス、およびデータ不変条件を規定する。

## Observation の役割と位置づけ

Observation は、分析用モデル（Canonical Data）へ変換される前段に位置する**唯一無二の事実証跡（Source of Evidence）**である。
リアルタイムに受信した Gateway イベントおよび HTTP API クロールにより取得したレスポンスの双方が、本 Observation として記録される。

### 取得経路（Provenance）の厳格な分離

HTTP Backfill によって取得されたメッセージを、未受信の Gateway イベント（`MESSAGE_CREATE` 等）として偽装・変換することを禁じる。

リアルタイム Gateway においては投稿直後の状態、中間編集履歴、削除イベントといった時系列変化を観測可能であるが、HTTP API においてはリクエスト時点で Discord 上に残存する最新スナップショットのみが取得される。

取得経路に応じた完全性保証レベルの本質的差異を保護するため、システムは取得経路および観測時刻を来歴（Provenance）として保持し、後続処理において厳格に区別可能とする。

## Observation に求められる中核要件

すべての Observation は、インフラの物理レイアウトに依存せず、論理的に以下の情報を保持しなければならない。

* **一意な識別性（Identity）**: システム全体で各観測を一意に特定可能な識別子を保持すること。
* **取得元と経路の明示（Source & Provenance）**: Gateway 由来、HTTP Backfill 由来、または定常照合（Reconciliation）由来を判別可能であること。
* **観測時刻（Observed Timestamp）**: Discord 側のタイムスタンプとは独立に、システムが当該データを受信・記録した時刻を保持すること。
* **対象エンティティの参照**: Guild ID、Channel ID、Message ID 等、Discord 上の対象リソースを即座に特定可能であること。
* **生ペイロードの完全保持（Raw Payload）**: 将来のスキーマ変更や再処理に備え、Discord から受信した生の JSON ペイロードを改変せずに保持すること。
* **セッション追跡性**: Gateway 由来の場合は `session_id` および `sequence` を、Backfill 由来の場合は一連の取得実行を表す `run_id` を追跡可能であること。

## 重複と整合性の不変条件

* **追記専用（Append-only）の徹底**: Observation Archive に対するインプレース更新および破壊的変更を禁じる。すべての観測は追記としてのみ永続化される。
* **欠損防止のための重複許容**: ネットワーク再接続（Resume replay）、ワーカーリトライ、HTTP ページネーション境界の重複等に伴い、同一データが複数回観測されることを許容する。分散環境において「観測を欠損させるリスク」を排し、「重複を許容して後段の正規化処理で排除する」方針を優先する。
* **追跡可能性の保証**: Canonical Data を参照した際、その状態がどの Observation 群に基づいて生成されたのか、元の生データまで完全に辿れるトレーサビリティを維持する。

## 分割予定の詳細設計

* **Observation Model**: ペイロードを内包する共通エンベロープ（Envelope）の論理定義
* **Provenance**: 取得経路、セッション、実行コンテキストの追跡モデル
* **Observation Identity**: 観測 ID の採番規則と時間ソート可能性
* **Duplicate Semantics**: 重複発生パターンと後段での重複排除ルール
* **Completeness Model**: Gateway 観測と HTTP 観測におけるデータ完全性の差異定義
