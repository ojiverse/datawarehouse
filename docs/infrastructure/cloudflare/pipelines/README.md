# Cloudflare Pipelines インフラ設計

本ディレクトリでは、Observation Archive（R2）に到達した生データを Canonical Store（Iceberg）へストリーミング変換・ロードする手段として Cloudflare Pipelines を採用する場合の構成、評価基準、およびフォールバック設計を定義する。

## Pipelines の位置づけと評価方針

Cloudflare Pipelines は、サーバーレスなストリーム取り込みとデータウェアハウスへのデータ配送を統合するマネージドサービスである。

* **候補技術としての評価**: 生の Observation ストリームを Iceberg テーブルへ直接 sink（書き込み）可能か、また変換処理（JavaScript/SQL Transform）の自由度が十分かを評価する。
* **非依存の原則（疎結合の維持）**: 
  * Observation Archive への高耐久な生ログ書き込み自体は、Pipelines の稼働状況に一切依存させない。
  * システム全体として、Pipelines を利用せず「Worker + Queue」の組み合わせだけでも Canonical Store のマテリアライズが完全に成立する構造を担保する。

## インフラ設計における確定事項

* **パイプライン構成（Source / Transform / Sink）**:
  * **Source**: Cloudflare Queues または R2 Event Notifications
  * **Transform**: JSON ペイロードの抽出、Discord Snowflake ID からのタイムスタンプ変換、正規化
  * **Sink**: R2 Data Catalog 経由の Iceberg テーブル
* **エラーハンドリング**: パースエラーや型不整合メッセージが発生した際の Dead Letter Sink（隔離ストレージ）への退避ルートの確立。
* **コストとクォータ**: データ転送量および変換処理量に基づく利用料金と、Workers ベースで自作した場合のランニングコストの比較検証。
