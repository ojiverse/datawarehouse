# Cloudflare インフラストラクチャ

本ディレクトリでは、Discord DWH を Cloudflare 上で安全かつ経済的に稼働させるために必要な、具体的なリソース構成、環境設計、セキュリティ、およびコスト管理を扱います。

## サブシステム構成

関心事に応じて、以下のサブシステムに分割してインフラストラクチャを設計しています。

| カテゴリ | サブシステム | 主な設計対象 |
| :--- | :--- | :--- |
| **環境と配置** | [environments/](environments/README.md) | 開発（dev）、検証（beta）、本番（prod）の環境分離ポリシー |
| | [deployment/](deployment/README.md) | Wrangler / CI/CD によるデプロイ、マイグレーション、ロールバック |
| **コンピュート** | [workers/](workers/README.md) | Worker の責務分割、CPU/メモリ制限、およびルーティング設定 |
| | [durable-objects/](durable-objects/README.md) | Gateway セッションを常時維持する Durable Objects の名前空間と設定 |
| **データ基盤** | [queues/](queues/README.md) | イベント中継・バッチ集約・Backfill 進行を行うキューのトポロジー |
| | [r2/](r2/README.md) | 生ログ（Observation）および分析テーブルを保持する R2 バケット設計 |
| | [data-catalog/](data-catalog/README.md) | Apache Iceberg テーブルを管理する R2 Data Catalog の設定 |
| | [pipelines/](pipelines/README.md) | ストリーム変換・ロードを行う Cloudflare Pipelines の構成 |
| **運用と統制** | [security/](security/README.md) | Discord Bot Token、バインディング権限、およびアクセス制御 |
| | [observability/](observability/README.md) | Workers ログ、メトリクス収集、トレース、およびアラート設定 |
| | [cost/](cost/README.md) | プラン選定（Free / Workers Paid）、クォータ制限、およびコスト試算 |

## プラン選定と移行方針

* **開発・ベータ環境**: リソース消費が限定的であるため、可能な限り Cloudflare の Free プラン枠内で検証を進めます。
* **本番環境**: Gateway の WebSocket 常時接続およびセッション維持を安定して実現するため、**Workers Paid プラン** を採用します。常時稼働する Durable Object 1 インスタンスの継続時間（Duration）が、Workers Paid に含まれる利用枠（Included Usage）内に収まるかをベータ期間の実測値で確認した上で本番へ移行します。
