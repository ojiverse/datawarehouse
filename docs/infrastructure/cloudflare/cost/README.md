# Cloudflare コストとキャパシティ設計

本ディレクトリでは、Discord DWH を Cloudflare 上で運用する際に発生するインフラストラクチャ費用、利用枠、およびコスト最適化とデータ耐久性のトレードオフ原則を定義する。

## プラン選定と基準構成

本番環境では、WebSocket 常時接続とバックグラウンドタスクの安定実行のため、Workers Paid プランを基本契約として採用する。

コスト試算の基準構成は、**Cloudflare 上で常時稼働する Gateway Instance が1つ**の状態とする。

Discord shard 数と Gateway Instance 数は同一概念ではない。同じ shard assignment を複数 Gateway Instance が並行して観測できるため、Durable Objects の capacity は shard 数だけでなく Cloudflare 上の Gateway Instance 数を基準に評価する。

## 複数 Gateway とコスト

Raspberry Pi 等の Cloudflare 外 Gateway Instance を追加しても、その Instance 自体は Cloudflare Durable Objects Duration を消費しない。

Cloudflare 上で新旧 Gateway Instance を一時的に並行稼働させる場合は、その overlap 期間だけ Durable Objects の消費が増加する。

Cloudflare 内で複数 Gateway Instance を常時 active にする場合は、Workers Paid の included usage に収まることを前提にせず、Instance 数と実測 duration に基づいて追加コストを評価する。

## コストと耐久性に関する基本不変条件

* **耐久性の犠牲によるコスト削減の禁止**: コスト削減のみを理由として Observation Archive の保存や回復経路を省略しない
* **複数 Gateway の許容**: コストを理由に Gateway Instance の並行稼働という domain capability 自体を禁止しない
* **トレードオフの明文化**: 保証レベルや常時冗長化方針を変更する場合は、アーキテクチャ設計および ADR に根拠を記録する

## ベータ期間での実測検証項目

* Gateway Instance 1つあたりの Durable Objects Duration
* 新旧 Cloudflare Gateway Instance を overlap させた場合の追加消費
* Queue の batch size に応じた R2 operation 数
* R2 SQL のデータ走査量
* Backfill と reconciliation の実行頻度による Workers / Queues 使用量

具体的な単価と included quota は Cloudflare の料金体系変更に追従して更新する。
