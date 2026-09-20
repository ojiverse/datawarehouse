# リアルタイム取り込みドメイン（Ingestion）

本ディレクトリでは、Discord Gateway（WebSocket）からリアルタイムにイベントを継続受信し、Observation として安全に受理するためのドメインロジック、セッション状態モデル、および配送保証を扱います。

## 本ドキュメントの責務と境界

### 扱う対象
* **接続とセッションのライフサイクル**: 物理的な WebSocket 接続と、Discord 上の論理セッション（Gateway Session）の対応関係
* **ハンドシェイクと復旧**: 初回認証（`Identify`）と切断後のセッション再開（`Resume`）のプロトコル
* **順序と追跡性**: シーケンス番号（`sequence`）の進捗管理と欠損検知
* **活性監視**: ハートビート（`Heartbeat`）の送受信による死活監視（Liveness）
* **イベント受理と配送**: 受信した Dispatch イベントをダウンストリーム（Observation Archive）へ確実に引き渡す配送セマンティクス

### 扱わない対象（アーキテクチャ・インフラ層へ委譲）
* Cloudflare 上で WebSocket 接続をどのリソース（Durable Objects 等）が所有するか
* タイマーのスケジューリングや永続化ストレージの実装手段
* キュー（Cloudflare Queues）やストレージ（R2）の物理構成

## Gateway 取り込みの基本原則

### 1. 切断と接続断は「通常事象」として扱う
インターネットを介した長時間の WebSocket 接続において、ネットワークの瞬断、Discord 側のクラスタ再起動、およびクラウド基盤側のコンテナ移行は不可避です。
本システムでは、接続断を「致命的な障害」ではなく「日常的に発生する通常事象」として設計し、切断検知から自動再接続までのステートマシンを標準動作として組み込みます。

### 2. Resume による継続と HTTP Backfill への委譲
切断が発生した際、Discord Gateway のセッションキャッシュが有効な期間内であれば、保持している `session_id` と `sequence` を用いて `Resume` を試行し、切断中に滞留していたイベントの再送を受けます。
もしセッションが無効化（Invalid Session）されて Resume に失敗した場合は、無理に Gateway だけで過去を復元しようとせず、速やかに新規セッションを確立した上で、欠損した時間区間を [HTTP Backfill](../backfill/README.md) による回復へと委ねます。

### 3. イベント履歴とスナップショットの非等価性
Gateway 経由で受信できるのは「発生したイベントの生の時系列」です。一方で、後から HTTP API で取得できるのは「その時点で Discord 上に残っている最新状態」に過ぎません。
Gateway で取りこぼしたデータを HTTP Backfill で補完する場合であっても、両者が提供する情報の一貫性や保証レベルの違いをドメインとして認識し、架空のイベント履歴を合成しない規律を守ります。

## 今後分割する詳細設計

* **Gateway Session Lifecycle**: 接続確立、認証、切断、再接続の状態遷移モデル
* **Heartbeat & Liveness**: ハートビート送信間隔、ACK タイムアウト判定、ゾンビ接続の検知
* **Resume & Recovery**: Resume 試行、セッション破棄判定、Backfill へのハンドオフ条件
* **Gateway Event Semantics**: 受理すべき Dispatch イベント種別とペイロードの扱い
* **Event Delivery Semantics**: 受信からストレージ永続化までの少なくとも1回（At-least-once）配送保証
* **Backpressure & Failure Semantics**: 後続ストレージの遅延・障害時におけるバッファリングと流量制御
