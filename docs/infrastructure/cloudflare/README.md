# Cloudflare インフラストラクチャ

本ディレクトリでは、Discord DWH を Cloudflare 上で運用するために必要なリソース構成、環境設計、セキュリティ、およびコスト管理を定義する。

## サブシステム構成

関心事に応じて、以下のサブシステムに分割してインフラストラクチャを設計する。

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

* **開発・ベータ環境**: リソース消費が限定的であるため、Cloudflare の Free プラン枠内で検証を実施する。
* **本番環境**: Gateway の WebSocket 常時接続およびセッション維持を担保するため、**Workers Paid プラン** を採用する。常時稼働する Durable Object 1 インスタンスの継続時間（Duration）が Workers Paid に含まれる利用枠（400,000 GB-s）内に収まることをベータ期間の実測値で検証した上で本番運用へ移行する。
