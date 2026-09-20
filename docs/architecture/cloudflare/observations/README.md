# Cloudflare Observation アーキテクチャ

本ディレクトリでは、Gateway や HTTP Backfill から届いた生の観測事実（Observation）を、Cloudflare R2 上の Observation Archive へ高耐久かつコスト効率よく永続化する書き込みパス（Durable Write Path）のアーキテクチャを定義する。

## R2 永続化とバッチ集約のアーキテクチャ

Observation Archive の物理基盤として **Cloudflare R2** を採用する。

### Class A 操作コストの抑制とバッチング
Discord 上では 1 秒間に数十〜数百件のイベントが発生し得る。各イベントを個別の R2 オブジェクトとして都度保存（PUT）した場合、R2 の Class A 操作回数が急増し、コストおよびレイテンシ面で破綻する。

この問題を解決するため、以下のバッチングパイプラインを構成する。

```mermaid
flowchart LR
    Ingest[DO / Backfill Worker] -->|個別の観測イベント| Queue[Cloudflare Queues]
    Queue -->|バッチ集約<br>最大100件 または 数秒待機| Consumer[Queue Consumer Worker]
    Consumer -->|1つのバッチオブジェクト<br>JSON Lines / Parquet| R2[(R2: Observation Archive)]
```

* **Cloudflare Queues による自動集約**: Queue のバッチコンシューマ機能を利用し、「一定件数（例: 最大100件）」または「一定時間（例: 5〜10秒）」単位でメッセージを集約して引き渡す。
* **圧縮と複合フォーマット**: 集約バッチを 1 つのオブジェクト（NDJSON または Parquet 形式、Gzip/Zstd 圧縮）として R2 へ一括書き込みする。これにより Class A 操作回数を数百分の一に削減する。

## 書き込みパスの耐久性保証（Durability First）

* **Canonical 変換からの完全な独立**: Observation の R2 保存処理は、後段の Canonical Store への正規化処理の成否に一切依存させない。Observation が R2 に永続化された時点で、取り込み完了としてコミットする。
* **リトライと重複の許容**: R2 への書き込みに一時的なネットワークエラー等で失敗した場合は、Queue の標準リトライ機能によって自動再試行する。リトライによって同一バッチが重複書き込みされた場合であっても、ドメイン原則に従い重複を許容し、後段の処理層で排除する。
* **共通書き込みパイプラインの採用**: リアルタイムイベント（DO）と HTTP 取得データ（Backfill Worker）は、同一の Queue エンドポイントを経由することで、R2 への保存フォーマットとバッチングロジックを完全に共通化する。

## 分割予定の詳細設計

* **Archive Write Path**: Queue Consumer による R2 オブジェクト生成とコミットシーケンス
* **Observation Batching**: 最適なバッチサイズ、待機時間、および圧縮アルゴリズムの選定
* **Object Layout**: R2 内のプレフィックス階層設計（日付・ギルド・時間帯パーティション）
* **Retry & Duplicate Handling**: Queue リトライと DLQ（Dead Letter Queue）の退避ポリシー
