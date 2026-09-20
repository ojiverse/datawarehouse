# 観測ドメイン（Observation）

本ディレクトリでは、Observation Archive に保存される「Discord から実際に観測された事実」の定義、セマンティクス、およびデータ不変条件を扱います。

## Observation の役割と位置づけ

Observation は、分析用モデル（Canonical Data）へ変換される前段に位置する**唯一無二の事実証跡（Source of Evidence）**です。
リアルタイムに流れてくる Gateway イベントと、HTTP API を巡回して取得したレスポンスの双方が、この Observation として記録されます。

### 取得経路（Provenance）の厳格な分離

HTTP Backfill で取得したメッセージを、実際に受信していない Gateway の `MESSAGE_CREATE` イベントとして扱ってはなりません。
リアルタイム Gateway では「投稿直後の状態、その後の編集履歴、削除イベント」といった時系列の変化を捉えられますが、HTTP API では「取得時点で Discord 上に残っている最新状態」しか得られません。

このように取得経路によってデータの完全性保証レベルが根本的に異なるため、システムは取得経路や観測時刻を来歴（Provenance）として保持し、後続処理で確実に区別できるようにします。

## Observation に求められる中核要件

すべての Observation は、インフラの物理レイアウトに依存せず、論理的に以下の情報を持たなければなりません。

* **一意な識別性（Identity）**: システム全体で各観測を一意に特定できる識別子を持つこと。
* **取得元と経路の明示（Source & Provenance）**: Gateway 由来か、HTTP Backfill 由来か、あるいは定常照合（Reconciliation）由来かを判別できること。
* **観測時刻（Observed Timestamp）**: Discord 側のタイムスタンプとは独立に、システムがそのデータをいつ受信・記録したかを保持すること。
* **対象エンティティの参照**: Guild ID、Channel ID、Message ID など、Discord 上のどのリソースに関する観測かを即座に特定できること。
* **生ペイロードの完全保持（Raw Payload）**: 将来のスキーマ変更やロジック見直しに備え、Discord から送られてきた生の JSON ペイロードを改変せずに保持すること。
* **セッション追跡性**: Gateway 由来の場合は `session_id` と `sequence` を、Backfill 由来の場合は一連の取得実行を表す `run_id` を追跡できること。

## 重複と整合性の不変条件

* **追記専用（Append-only）の徹底**: Observation Archive に対する日常的な更新やインプレース変更を禁じます。すべての観測は追記として保存されます。
* **欠損防止のための重複許容**: ネットワークの再接続（Resume replay）、ワーカーのリトライ、HTTP ページネーションの境界重複などにより、同一の Discord データが複数回観測されることを許容します。分散環境において「観測を欠損させるリスク」を冒すよりも、「重複を許容し、後段の正規化処理で排除する」方針を優先します。
* **追跡可能性の保証**: Canonical Data を参照した際、その状態がどの Observation 群に基づいて生成されたのか、元の生データまで完全に辿れるトレーサビリティを維持します。

## 今後分割する詳細設計

* **Observation Model**: ペイロードを包む共通エンベロープ（Envelope）の論理定義
* **Provenance**: 取得経路、セッション、実行コンテキストの追跡モデル
* **Observation Identity**: 観測 ID の採番規則と時間ソート可能性
* **Duplicate Semantics**: 意図的な重複発生パターンと後段での重複排除ルール
* **Completeness Model**: Gateway 観測と HTTP 観測におけるデータ完全性の差異定義
