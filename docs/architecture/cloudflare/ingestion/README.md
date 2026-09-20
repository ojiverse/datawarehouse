# Cloudflare 取り込みアーキテクチャ（Ingestion）

本ディレクトリでは、Discord Gateway との WebSocket 接続を Cloudflare 上で維持し、受信したイベントを Observation として後続へ安全に引き渡すアーキテクチャを定義する。

## Durable Objects によるセッションの単一所有

Discord Gateway のプロトコル仕様では、1 つのシャードに対して同時に複数のクライアントが接続することは許されず、単一のセッションが順序づけられたシーケンス番号（`sequence`）を厳密に管理しなければならない。

このドメイン要件を充足するため、Cloudflare 上での接続管理には **Durable Objects（DO）** を採用する。

* **排他的な単一所有権**: ギルド/シャードごとに 1 つの DO インスタンスを割り当て、WebSocket 接続の排他性を保証する。
* **メモリ内でのセッション維持**: メモリ上で WebSocket コネクション、`session_id`、および最新の `sequence` を保持し、低レイテンシでハートビート（Heartbeat）の送受信とシーケンス更新を実行する。
* **Alarm API による自律的死活監視**: DO の Alarm 機構を利用し、指定間隔（Heartbeat Interval）ごとの定期ハートビート送信と、ACK 未達時のタイムアウト・強制再接続判定を自律的に実行する。

## 再起動と Resume のアーキテクチャ

Durable Objects は、デプロイや内部メンテナンスに伴い再起動（Eviction / Migration）が発生する場合がある。

* **永続化境界**: セッション再開に必要な最小限の情報（`session_id`、`resume_gateway_url`、直近確定シーケンス番号）を DO のトランザクションストレージに随時永続化する。
* **再起動後の Resume 試行**: DO インスタンスが再生成された際、保存されたセッション情報を読み込んで Discord への `Resume` を試行し、切断中のイベントを再受信する。
* **無効セッション時のフォールバック**: セッション破棄（Invalid Session）を受信した場合は、速やかに新規セッションを確立（`Identify`）し、未取得となった区間の修復要求を Backfill コンポーネントへ発行する。

## ダウンストリームへの配送とバックプレッシャー

DO が受信した Dispatch イベントは、直接 R2 に書き込まず、**Cloudflare Queues** を介して非同期に Observation Archive へ配送する。

* **障害隔離**: ストレージ（R2）の書き込み遅延や一時的な障害が発生した場合であっても、WebSocket 受信ループのブロックを阻止する。
* **流量制御**: キュー滞留時におけるメモリバッファリング上限を規定し、メモリ超過によるクラッシュを防ぐバックプレッシャー戦略を適用する。

## 分割予定の詳細設計

* **Gateway Runtime Ownership**: DO インスタンスのライフサイクルとシャードマッピング
* **Session Persistence**: DO ストレージへの状態書き込みタイミングとコスト最適化
* **Heartbeat Runtime**: Alarm API を用いた正確なタイマー実装とゾンビ検知
* **Resume Runtime**: 切断検知から再接続・Resume 実行までのステートマシン
* **Event Handoff & Queues**: DO から Cloudflare Queues へのバッチ投入ロジック
* **Backpressure Strategy**: ダウンストリーム遅延時のメモリ枯渇防止ルール
