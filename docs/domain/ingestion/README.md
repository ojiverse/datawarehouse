# リアルタイム取り込みドメイン（Ingestion）

本ディレクトリでは、Discord Gateway（WebSocket）からリアルタイムにイベントを受信し、Observation として安全に受理するためのドメインロジック、セッション状態モデル、および配送保証を定義する。

## 責務と境界

### 扱う対象
* **Gateway Instance**: 本システム側で Gateway 接続を実行する観測主体。Cloudflare、Raspberry Pi、その他の実行環境に複数存在できる
* **Gateway Session**: Discord が発行する論理セッション。各 Session は独立した `session_id` と `sequence` 系列を持つ
* **Shard Assignment**: 各 Gateway Session が担当する Discord shard。移行や検証のため、同一 shard を複数 Session が並行して観測することを許容する
* **ハンドシェイクと復旧**: 初回認証（`Identify`）と切断後の同一セッション再開（`Resume`）
* **順序と追跡性**: Discord が発行する `sequence` を、各 Gateway Session 内の順序と Resume cursor として追跡する
* **活性監視**: ハートビート（`Heartbeat`）の送受信による死活監視（Liveness）
* **イベント受理と配送**: 受信した Dispatch イベントを Observation Archive へ確実に引き渡す配送セマンティクス

### 扱わない対象（アーキテクチャ・インフラ層へ委譲）
* Gateway Instance をどの実行基盤へ配置するか
* WebSocket 接続、タイマー、永続状態をどの具体リソースで実装するか
* キューやストレージの物理構成
* HTTP Backfill の crawler 実装

## 複数 Gateway を前提とする不変条件

同一の Discord traffic coverage に対し、複数の Gateway Instance と Gateway Session が同時に存在することを正常状態として扱う。

これは Cloudflare から Raspberry Pi への移行、ローリング更新、検証用の並行稼働、将来の冗長化を特別な migration mode なしで扱うための前提である。

各 Gateway Session は独立した Discord event stream であり、Session 間で `session_id` や `sequence` を共有しない。

同じ Discord 上の出来事が複数 Session から重複して観測されることを許容し、Observation Archive ではそれぞれ独立した観測証跡として保持する。

`sequence` は Discord が Gateway Session ごとに発行する順序情報であり、複数 Session 間で共通のイベント識別子または全体順序として扱わない。

## 収集対象の Product Policy

Gateway で観測可能だからという理由だけで、すべての transient telemetry を収集することはしない。

DWH-public scope に属する durable community activity は原則として Observation Archive へ保存し、現在の Canonical model や Bot / AI Agent が利用しないことだけを理由に drop しない。

Presence、Typing、Voice State は transient behavioral telemetry として Deliberate Absence に指定し、収集対象から除外する。

具体的な Gateway Intent、event filter、DWH-public の指定方法は、この Product Policy を満たす範囲で Architecture が決定する。

## Gateway 取り込みの基本原則

### 1. 切断と接続断の通常事象化
公衆網を介した長時間の WebSocket 接続において、ネットワーク瞬断、Discord 側のクラスタ再起動、および実行基盤側の再起動は不可避である。
システムは接続断を致命的な障害ではなく「定常的に発生する通常事象」として扱い、切断検知から自動再接続に至る状態遷移を標準動作として組み込む。

### 2. Resume と新規 Session の役割分離
同一 Gateway Session の接続断から回復する場合は、保持する `session_id` と `sequence` を用いて `Resume` を試行する。

実行環境の移行や別 Gateway Instance の追加では、既存 Session の状態を別 Instance へ移植することを前提とせず、新しい独立 Session を確立して並行観測期間を設ける。

Session が無効化され Resume に失敗した場合は、新規 Session を確立した上で、欠損区間を [HTTP Backfill](../backfill/README.md) による回復へ委譲する。

### 3. イベント履歴とスナップショットの非等価性
Gateway 経由で受信可能なデータは「発生したイベントの時系列」である。一方、HTTP API で取得可能なデータは「リクエスト時点で Discord 上に残存する状態」である。

Gateway の取りこぼしを HTTP Backfill で補完する場合であっても、両者が提供する完全性保証の差異をドメインとして認識し、架空のイベント履歴の合成を禁じる。

### 4. Durable Acceptance と At-least-once 配送
Gateway で Dispatch を受信しただけでは、Observation Archive への保存を保証したとはみなさない。

プロセス停止や実行環境の喪失後も配送を再試行できる状態へ到達した時点を **Durable Acceptance** と定義する。

Durable Acceptance された Observation は、Observation Archive へ **At-least-once** で配送されなければならない。配送の再試行によって同じ Observation が複数回 Archive へ到達することを正常系として許容する。

Durable Acceptance より前に失われた Gateway event については At-least-once を保証しない。切断時は Resume により event stream の回復を試み、Resume できない場合は HTTP Backfill により Discord 上に残存する状態を回復する。

したがって、At-least-once は Discord から Observation Archive までの end-to-end 保証ではなく、**本システムが Observation を durable に受理した後の配送保証**である。

## 分割予定の詳細設計

* **Gateway Instance Model**: 観測主体の安定した識別性とライフサイクル
* **Gateway Session Lifecycle**: 接続確立、認証、切断、再接続の状態遷移モデル
* **Shard Assignment**: Instance、Session、Discord shard の関係
* **Multi-session Observation**: 同一 traffic を複数 Session が観測する場合の意味論
* **Heartbeat & Liveness**: ハートビート送信間隔、ACK タイムアウト判定、ゾンビ接続の検知
* **Resume & Recovery**: Resume 試行、Session 破棄判定、Backfill へのハンドオフ条件
* **Gateway Event Semantics**: 受理すべき Dispatch イベント種別とペイロードの扱い
* **Event Delivery Semantics**: Durable Acceptance の境界、Acceptance 後の At-least-once 配送保証、および Acceptance 前の Gateway recovery semantics
* **Backpressure & Failure Semantics**: 後続ストレージ遅延・障害時におけるバッファリングと流量制御
