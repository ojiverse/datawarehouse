# リアルタイム取り込みドメイン（Ingestion）

本ディレクトリでは、Discord Gateway（WebSocket）からリアルタイムにイベントを受信し、Observation として安全に受理するためのドメインロジック、セッション状態モデル、および配送保証を定義する。

## 責務と境界

### 扱う対象
* **接続とセッションのライフサイクル**: 物理的な WebSocket 接続と、Discord 上の論理セッション（Gateway Session）の対応関係
* **ハンドシェイクと復旧**: 初回認証（`Identify`）と切断後のセッション再開（`Resume`）のプロトコル
* **順序と追跡性**: シーケンス番号（`sequence`）の進捗管理と欠損検知
* **活性監視**: ハートビート（`Heartbeat`）の送受信による死活監視（Liveness）
* **イベント受理と配送**: 受信した Dispatch イベントをダウンストリーム（Observation Archive）へ確実に引き渡す配送セマンティクス

### 扱わない対象（アーキテクチャ・インフラ層へ委譲）
* Cloudflare 上で WebSocket 接続を所有する具体リソース（Durable Objects 等）
* タイマーのスケジューリングや永続化ストレージの実装手段
* キュー（Cloudflare Queues）やストレージ（R2）の物理構成

## Gateway 取り込みの基本原則

### 1. 切断と接続断の通常事象化
公衆網を介した長時間の WebSocket 接続において、ネットワーク瞬断、Discord 側のクラスタ再起動、およびクラウド基盤側の移行は不可避である。
システムは接続断を致命的な障害ではなく「定常的に発生する通常事象」として扱い、切断検知から自動再接続に至るステートマシンを標準動作として組み込む。

### 2. Resume による継続と HTTP Backfill への委譲
切断発生時、Discord Gateway のセッションキャッシュが有効な期間内であれば、保持する `session_id` と `sequence` を用いて `Resume` を試行し、切断中の滞留イベントを受信する。
セッションが無効化（Invalid Session）されて Resume に失敗した場合は、Gateway 単独での過去復元を試みず、速やかに新規セッションを確立した上で、欠損区間を [HTTP Backfill](../backfill/README.md) による回復へ委譲する。

### 3. イベント履歴とスナップショットの非等価性
Gateway 経由で受信可能なデータは「発生したイベントの生の時系列」である。一方、HTTP API で取得可能なデータは「リクエスト時点で Discord 上に残存する最新状態」である。
Gateway の取りこぼしを HTTP Backfill で補完する場合であっても、両者が提供する完全性保証の差異をドメインとして認識し、架空のイベント履歴の合成を禁じる。

## 分割予定の詳細設計

* **Gateway Session Lifecycle**: 接続確立、認証、切断、再接続の状態遷移モデル
* **Heartbeat & Liveness**: ハートビート送信間隔、ACK タイムアウト判定、ゾンビ接続の検知
* **Resume & Recovery**: Resume 試行、セッション破棄判定、Backfill へのハンドオフ条件
* **Gateway Event Semantics**: 受理すべき Dispatch イベント種別とペイロードの扱い
* **Event Delivery Semantics**: 受信からストレージ永続化までの少なくとも1回（At-least-once）配送保証
* **Backpressure & Failure Semantics**: 後続ストレージ遅延・障害時におけるバッファリングと流量制御
