# Cloudflare Workers インフラ設計

本ディレクトリでは、Discord DWH を構成する各コンポーネントのエントリポイントとなる Cloudflare Worker リソースの分割方針、配置、およびバインディング設計を定義する。

## Worker サービスの責務分割

障害の局所化（Failure Isolation）および独立したスケーリング・デプロイを実現するため、以下の関心事ごとに Worker サービスを分割配置する。

| Worker サービス候補 | トリガー / エントリポイント | 主な責務 | 主なバインディング |
| :--- | :--- | :--- | :--- |
| **Ingestion Worker** | Gateway WebSocket / DO | Gateway セッション管理（Durable Object ホスト）とキューへのイベント投入 | Durable Objects, Queues |
| **Observation Queue Consumer** | Cloudflare Queues | キューからバッチ集約されたイベントを受け取り、R2 へ NDJSON/Parquet として書き込み | R2 (Observation Bucket) |
| **Backfill Crawler Worker** | HTTP リクエスト / Queues / Cron | Discord HTTP API を巡回し、取得した生メッセージを Ingestion Queue へ投入 | Queues, Secrets (Bot Token) |
| **Canonical Processing Worker** | R2 Event / Queues | 新着 Observation を読み込み、Iceberg テーブルへ正規化・書き込み | R2 (Observation & Canonical), Data Catalog |
| **Query API Worker** | HTTP エンドポイント | 外部ダッシュボードや分析ツール向けに R2 SQL クエリを実行・返却 | R2 SQL, Data Catalog |

## インフラ設計における確定事項

* **サービス分割粒度**: 各責務を完全に独立した Worker プロジェクトとするか、単一リポジトリ内の複数モジュールとして段階的に切り出すかの決定。
* **リソース制限と Quota**: 
  * Ingestion Worker: WebSocket 接続を維持するための CPU 時間およびメモリフットプリントの最適化。
  * Crawler Worker: レート制限待機時の不要なコンピュート消費を回避するための非同期キュー駆動の徹底。
* **バインディング（Bindings）の最小権限**: 各 Worker に必要なリソース（R2 バケット、キュー、シークレット等）のみをバインドし、意図しない権限共有を防止。
