# Cloudflare セキュリティ設計

本ディレクトリでは、Discord DWH のインフラストラクチャおよびアプリケーションが扱う認証情報（Credential）、暗号化シークレット、アクセス権限（IAM）、およびパブリックリポジトリ運用におけるセキュリティ原則を定義する。

## セキュリティ基本原則

1. **パブリックリポジトリ前提のゼロコミット原則**: 本リポジトリが公開状態であっても安全性が維持されるよう、API キー、トークン、秘密鍵、内部シークレットのコードおよび設計文書へのコミットを固く禁じる。
2. **最小特権の原則（Least Privilege）**: 各 Worker およびコンポーネントには、その責務遂行に不可欠な最小限のバインディング（読み取り専用権限、特定キューへの投入権限等）のみを付与する。
3. **環境ごとの認証情報分離**: Development、Beta、Production の各環境で個別の Discord Bot Token および Cloudflare API クレデンシャルを発行し、漏洩時の影響半径（Blast Radius）を局所化する。

## DWH 利用者の認可境界

認可の Product Policy は [domain/product-policy/](../../../domain/product-policy/README.md) を基準とする。

DWH は community-wide corpus であり、Discord の Role、Channel Permission、Permission Override を record ごとに再現しない。

Query / API のアクセス境界では「現在の OJIverse メンバー、またはそのメンバーに紐づく Bot / Application として DWH 全体を利用できる主体か」を判定する。

一度 admission された主体に対し、Message / Channel 単位で異なる corpus を返す設計は採用しない。人間と Bot / Application でも取得可能な corpus を区別しない。

具体的な identity provider、Bot / Application とユーザーの紐付け方式、membership 検証方式は Architecture で決定する。

## 管理対象のクレデンシャルと保管方法

| 秘匿情報 | 用途 | 保管・注入方法 | アクセスするコンポーネント |
| :--- | :--- | :--- | :--- |
| **`DISCORD_BOT_TOKEN`** | Gateway 接続時の Identify 認証、および HTTP API クロール | Cloudflare Secrets（Wrangler secret） | Ingestion DO, Backfill Crawler |
| **Cloudflare API Token** | CI/CD パイプラインからのデプロイ実行、リソース作成 | GitHub Actions Secrets | デプロイパイプライン |
| **R2 Access Key（必要時）** | 外部クエリエンジンからの R2 / Catalog 直接読み取り | Cloudflare Access / Secrets | 外部分析ワーカー（原則内部バインディング優先） |

## インフラ設計における確定事項

* **Wrangler Secret のローテーション運用**: Discord トークン失効時の無停止ローテーション手順の確立。
* **Cloudflare API トークンのスコープ制限**: デプロイ用トークンに付与する権限（Workers Scripts: Edit, R2: Edit 等）の最小化。
* **バインディングレベルのアクセス制御**: Processing Worker に R2 書き込み権限を付与しつつ、Query API Worker には R2 読み取り専用権限のみをバインドする設定の徹底。
