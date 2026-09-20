# Cloudflare 環境設計（Environments）

本ディレクトリでは、開発から本番の安定運用に至るまでのフェーズ定義と、Cloudflare 上における環境分離（Development, Beta, Production）のインフラ設計を扱います。

## 想定フェーズと環境の定義

システムの成熟度と検証目的に応じて、以下の 3 つの環境を定義します。

```mermaid
flowchart LR
    Dev[Development 環境<br>HTTP Backfill の検証] --> Beta[Beta 環境<br>断続 Gateway & 回復検証]
    Beta --> Prod[Production 環境<br>Workers Paid 常時運用]
```

### 1. 開発環境（Development）
* **主目的**: Discord HTTP API 経由での Backfill 実行、Cloudflare Queues によるバッチ集約、R2 への Observation 保存、および Iceberg へのマテリアライズパイプラインの正常性確認。
* **運用形態**: Gateway の常時接続は行わず、手動トリガーまたは限定的なクロールテストを実行。Free プラン枠内で運用。

### 2. 検証環境（Beta）
* **主目的**: 短期間・断続的な Gateway 接続テスト。WebSocket 接続の安定性、切断時の Resume 動作、セッション切れ時の Backfill 自動補完連携を実測・評価。
* **運用形態**: 実際のテストサーバー（ギルド）を接続し、負荷変動や障害注入（意図的な切断）を行いながら、Durable Objects のリソース消費量（GB-s 等）を測定。

### 3. 本番環境（Production）
* **主目的**: 本番 Discord サーバーの全イベントの常時収集、長期間にわたる高耐久な蓄積、および分析クエリの提供。
* **運用形態**: Workers Paid プランを契約し、年内の安定稼働・24時間365日の連続接続を確立。

## 環境分離における設計論点

本番移行に向けて、以下のインフラ分離境界を決定します。

* **リソースの完全分離**: 本番（prod）環境と開発・検証（dev/beta）環境の間で、R2 バケット、Queues、Durable Objects 名前空間を完全に分離し、開発時の誤操作やテストデータ混入が本番データに影響しない構成とする。
* **認証情報の分離**: Discord Bot Token および Cloudflare API トークンを環境ごとに発行し、Wrangler Environments または Cloudflare Secrets で厳格に個別管理する。
* **移行ポリシー**: Beta で収集したデータを Production へ直接引き継ぐことはせず、本番公開時に初期 Backfill を改めて実行するクリーンスタート方針を基本とする。
