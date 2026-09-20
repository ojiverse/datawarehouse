# Cloudflare Queues インフラ設計

本ディレクトリでは、システム内の各コンポーネント間で非同期通信、バッファリング、およびバッチ集約を行うための Cloudflare Queues リソースのトポロジー、保持期間（Retention）、およびリトライ設定を定義する。

## キュートポロジー設計

異なる流量特性や処理優先度を持つメッセージの相互干渉を抑止するため、用途ごとに以下のキューを分離して定義する。

| キュー名（論理名） | 主な役割 | プロデューサ | コンシューマ | 主な設定要件 |
| :--- | :--- | :--- | :--- | :--- |
| **`observation-ingest-queue`** | Gateway イベントおよび Backfill 取得データを R2 保存用にバッチ集約 | Ingestion DO / Backfill Worker | R2 Writer Worker | 最大バッチサイズ: 100件<br>最大待機時間: 5〜10秒 |
| **`backfill-task-queue`** | Backfill のページネーション（次ページ取得タスク）の連鎖 | Backfill Worker | Backfill Worker | 遅延配信（Delay Seconds）対応<br>レート制限（429）時のバックオフ |
| **`canonical-process-queue`** | R2 保存完了通知を受け、Iceberg テーブルへの正規化をトリガー | R2 Event / Writer | Processing Worker | DLQ（Dead Letter Queue）併設<br>リトライ上限設定 |

## キューの基本不変条件（Non-Source-of-Truth）

* **非永続化導管としての位置づけ**: キューはコンポーネント間を一時的につなぐ配送導管（Transit）であり、データの事実源（Source of Truth）として扱ってはならない。
* **欠損への回復耐性**: 基盤障害によるメッセージ欠損や TTL 切れが発生した場合であっても、「Gateway セッションの Resume 失敗（Backfill 回復）」または「Observation Archive からの Replay」により完全再取得・再計算可能なトポロジーを構築する。

## インフラ設計における確定事項

* **Dead Letter Queue（DLQ）ポリシー**: リトライ回数超過メッセージの隔離設計および異常検知アラートとの連携。
* **メッセージ保持期間（Retention Period）**: 各キューにおけるメッセージの最大滞留許容時間（例: 最大4日間）の設定。
* **操作クォータとコスト**: Queues メッセージ操作数（Write, Read, Delete）の月間試算とコスト最適化。
