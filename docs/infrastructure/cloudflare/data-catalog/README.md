# R2 Data Catalog インフラ設計

本ディレクトリでは、Canonical Store に格納される Apache Iceberg テーブルのメタデータ管理、スキーマ追跡、および R2 SQL との連携を担う R2 Data Catalog の構成と運用設計を定義する。

## R2 Data Catalog の役割と位置づけ

R2 Data Catalog は、オープンなテーブルフォーマットである Apache Iceberg の **REST カタログ** として機能する。

* **テーブルメタデータの一元管理**: R2 上のデータファイル（Parquet）に対する最新スナップショットのコミット、スキーマ定義、およびパーティション仕様をカタログ上で一元管理する。
* **ACID トランザクションの保証**: 複数のワーカーやクエリエンジンが並行してデータにアクセスする際、楽観的並行性制御（OCC）によって安全なスナップショットコミットを実現する。
* **クエリエンジン連携**: R2 SQL などのクエリエンジンがカタログを参照し、最新テーブルスキーマと走査対象ファイルを即座に特定可能とする。

## テーブル名前空間（Namespace）と初期カタログ構成

環境（`dev`, `beta`, `prod`）ごとに独立したカタログインスタンスを作成し、以下のテーブル名前空間を管理する。

| テーブル名 | 名前空間 | 主なパーティションキー |
| :--- | :--- | :--- |
| **`messages`** | `ojiverse_dwh` | `guild_id`, `created_date` (日単位) |
| **`channels`** | `ojiverse_dwh` | `guild_id` |
| **`threads`** | `ojiverse_dwh` | `guild_id`, `parent_channel_id` |

## インフラ設計における確定事項

* **アクセス制御（IAM）**: Processing Worker にのみカタログの書き込み（Commit）権限を付与し、Query API Worker には読み取り専用権限を付与する最小権限ポリシーを適用する。
* **スキーマ進化（Schema Evolution）の適用フロー**: 列追加や型変更をカタログ経由で安全に適用する手順を確立する。
* **メンテナンス運用**: 定期的な Iceberg のテーブル最適化（Compaction: 細かい Parquet ファイルのマージ）と、古いスナップショットの整理（Expire Snapshots）の実行体制を整備する。
