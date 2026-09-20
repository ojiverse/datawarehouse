# Cloudflare セキュリティ設計

本ディレクトリでは、Discord DWH のインフラストラクチャおよびアプリケーションが扱う認証情報（Credential）、暗号化シークレット、アクセス権限（IAM）、およびパブリックリポジトリ運用におけるセキュリティ原則を扱います。

## セキュリティ基本原則

1. **パブリックリポジトリ前提のゼロコミット原則**: 本リポジトリがオープンソース/公開状態であっても安全性が保たれるよう、API キー、トークン、秘密鍵、エンドポイント内部シークレットをコードや設計文書に絶対にコミットしません。
2. **最小特権の原則（Least Privilege）**: 各 Worker やコンポーネントには、その責務に必要な最小限のバインディング（読み取り専用権限、特定のキューへの投入権限など）のみを付与します。
3. **環境ごとの認証情報分離**: Development、Beta、Production の各環境で個別の Discord Bot Token および Cloudflare API クレデンシャルを発行し、漏洩時の爆発半径（Blast Radius）を最小化します。

## 管理対象のクレデンシャルと保管方法

| 秘匿情報 | 用途 | 保管・注入方法 | アクセスするコンポーネント |
| :--- | :--- | :--- | :--- |
| **`DISCORD_BOT_TOKEN`** | Gateway 接続時の Identify 認証、および HTTP API クロール | Cloudflare Secrets（Wrangler secret） | Ingestion DO, Backfill Crawler |
| **Cloudflare API Token** | CI/CD パイプラインからのデプロイ実行、リソース作成 | GitHub Actions Secrets | デプロイパイプライン |
| **R2 Access Key（必要時）** | 外部クエリエンジンからの R2 / Catalog 直接読み取り | Cloudflare Access / Secrets | 外部分析ワーカー（原則内部バインディング優先） |

## インフラ設計時に確定すべき事項

* **Wrangler Secret のローテーション運用**: Discord トークン失効時のダウンタイムなきローテーション手順。
* **Cloudflare API トークンのスコープ制限**: デプロイ用トークンに付与する Cloudflare アカウント内の権限制限（Workers Scripts: Edit, R2: Edit 等）。
* **バインディングレベルのアクセス制御**: Processing Worker に R2 書き込み権限を与えつつ、Query API Worker には R2 読み取り専用権限のみをバインドする設定。
