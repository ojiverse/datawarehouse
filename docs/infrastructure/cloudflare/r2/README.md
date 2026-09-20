# Cloudflare R2 インフラ設計

本ディレクトリでは、Observation Archive（生ログ証跡）および Canonical Store（Apache Iceberg 分析モデル）の物理ストレージ層として機能する Cloudflare R2 のバケット構成、オブジェクトレイアウト、およびライフサイクルルールを定義する。

## バケットトポロジー

責務とアクセスパターンの異なる2つのデータ層を、独立した R2 バケットとして分離配置する。

| バケット（論理名） | 格納データ | データ形式 | アクセスパターン |
| :--- | :--- | :--- | :--- |
| **`ojiverse-dwh-observations`** | 観測事実の生ログ証跡（Source of Evidence） | NDJSON (gzip圧縮) または Parquet | 追記専用（Write-Heavy）、リプレイ時のバルクスキャン |
| **`ojiverse-dwh-canonical`** | Apache Iceberg テーブル（正規化データモデル） | Parquet データファイル + Iceberg メタデータ JSON/AVRO | 分析クエリ（Read-Heavy）、マテリアライズ時のスナップショットコミット |

## オブジェクトレイアウト設計（プレフィックス階層）

### Observation Archive バケット
クエリおよびリプレイ時のスキャン範囲を効率的に絞り込むため、以下の階層構造を採用する。

* 構造: `observations/v1/source={gateway|backfill}/guild_id={guild_id}/year={YYYY}/month={MM}/day={DD}/{batch_id}.ndjson.gz`
* 特徴: 日付と取得元（Source）によるパーティショニングにより、特定期間のリプレイや特定ギルドの監査を高速化する。

### Canonical Store バケット
Apache Iceberg の標準仕様に準拠したテーブルレイアウトを採用する。

* 構造: `tables/{table_name}/data/...` および `tables/{table_name}/metadata/...`
* 特徴: R2 Data Catalog から参照されるメタデータツリーと実データファイルを同居させる。

## コスト最適化とライフサイクル管理

* **Class A 操作（PUT）の抑制**: Ingestion Queue のバッチ集約により、個々のオブジェクトサイズを 1MB〜10MB 程度にまとめ、不要な書き込み操作回数を削減する。
* **ライフサイクルルール**: 
  * Observation Archive は長期保存（5〜10年）を前提とし、自動削除（Expiry）は適用しない。
  * 古い Iceberg スナップショットのメタデータや孤立ファイル（Orphan Files）は、定期的なメンテナンスジョブによって削除し、容量肥大化を防止する。
