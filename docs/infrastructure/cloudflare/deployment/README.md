# Cloudflare デプロイ・マイグレーション設計

本ディレクトリでは、Cloudflare 上に配置される各 Worker、Durable Objects、キュー、およびストレージリソースを安全かつ再現可能にデプロイ・移行（マイグレーション）・ロールバックするためのインフラ設計を扱います。

## CI/CD と構成管理（IaC）方針

すべてのリソース構成およびコードのデプロイは、手動操作を排し、Git リポジトリを唯一の正（Source of Truth）とした自動化パイプラインによって実行します。

* **デプロイツール**: Cloudflare 公式の `wrangler` CLI を採用し、環境（`dev` / `beta` / `prod`）ごとの設定を `wrangler.toml`（または環境別設定ファイル）で管理します。
* **CI/CD パイプライン**: GitHub Actions を利用し、テスト実行、環境別ブランチへのマージ、および自動デプロイを統制します。

## デプロイ時のデータ欠損防止原則（Zero Data Loss）

特に常時接続を維持する Ingestion Worker（Durable Objects）のデプロイにおいては、以下の耐障害機構と連携してデータ欠損を防止します。

```mermaid
flowchart TD
    Deploy[新バージョンのデプロイ実行] --> Evict[旧 DO インスタンスの終了処理]
    Evict --> SaveState[セッション情報・直近シーケンスを DO ストレージへフラッシュ]
    SaveState --> CloseWS[WebSocket の Graceful Close]
    CloseWS --> SpawnNew[新 DO インスタンスの起動]
    SpawnNew --> Resume[保存情報を用いた Discord への Resume 試行]
    
    Resume -->|成功| Normal[通常取り込みへ復帰（欠損ゼロ）]
    Resume -->|失敗（セッション破棄時）| Backfill[新規セッション確立 + 欠損区間の Backfill 自動発行]
```

* **安全な終了処理**: 新コード反映に伴い DO インスタンスが再生成される際、最新の `session_id` と `sequence` を確実にストレージへ永続化してから接続を切断します。
* **Resume と Backfill の多重防御**: デプロイ直後に新インスタンスが Discord へ `Resume` を要求し、未達イベントを回収します。仮にセッションが失効した場合でも、Backfill パイプラインが直ちに起動して未取得区間を補完します。

## インフラ設計時に確定すべき事項

* **ロールバック手順**: アプリケーションのデプロイに起因する致命的な不具合発生時に、直前の安定バージョンへ安全に切り戻す自動/手動手順。
* **Durable Object のマイグレーションタグ**: クラス定義や内部ストレージスキーマを変更する際の Wrangler migration 設定。
* **Data Catalog / Iceberg スキーマのバージョン管理**: テーブル変更を適用する際のマイグレーションスクリプトの配置と実行パイプライン。
