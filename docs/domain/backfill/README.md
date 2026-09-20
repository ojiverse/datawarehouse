# バックフィルドメイン（Backfill）

本ディレクトリでは、Discord HTTP API を利用して過去データおよび欠損区間の Observation を取得し、Gateway のリアルタイム受信を補完するドメイン設計を定義する。

## Backfill の機能的位置づけ

Backfill エンジンは、単一目的に特化させるのではなく、以下のユースケースにおいて共通のコアロジックとして機能するよう設計する。

* **初期データロード（Cold Start）**: DWH 初回セットアップ時における、サーバー（ギルド）内過去メッセージ履歴の一括取得。
* **Gateway 切断区間の回復（Gap Fill）**: Gateway セッション無効化（Invalid Session）等に伴う未取得区間の補完。
* **インシデント復旧（Repair）**: 障害等によって欠損した特定データ区間の再取得。
* **定常的な整合性維持（Reconciliation）**: 直近履歴を定期スキャンし、リアルタイム受信で見逃された潜在的欠損を自己修復。

## 基本原則とデータ完全性の限界

### Gateway 履歴と HTTP スナップショットの違い
HTTP Backfill は、過去にリアルタイムで発生した「すべてのイベント履歴」を復元するものではない。
HTTP API から取得可能なデータは、**リクエスト時点で Discord 上に残存する最新の状態（State）**に限定される。

例えば、Gateway 停止中に「投稿後に即座に削除されたメッセージ」や「複数回編集されたメッセージの中間状態」を HTTP API から取得することは不可能である。
したがって、Gateway 由来データと Backfill 由来データで保証可能な完全性（Completeness Guarantee）の差異を前提とし、Backfill データを架空のイベント履歴として扱わない原則を徹底する。

## アンチエントロピー機構（Anti-Entropy）

分散データ基盤において、リアルタイムストリーム取り込み（Gateway）のみに依存した場合、潜在的なパケットドロップや一時的処理落ちによるデータ欠損（エントロピーの増大）が不可避である。

本システムでは、HTTP API を単なる非常用バックアップではなく、データの完全性を能動的に保証する**アンチエントロピー機構**として位置づける。
定期的に直近（例: 過去数時間〜数日）のチャンネル履歴を HTTP 経由でバックグラウンド巡回し、Observation Archive と照合することで、見逃されたイベントを結果整合（Eventual Consistency）により確実に回収する。

## 分割予定の詳細設計

* **Message History Crawling**: ページネーション（`before` / `after`）を用いたチャンネル巡回アルゴリズム
* **Thread Discovery**: アーカイブ済みスレッドおよび新規スレッドの探索・検出ロジック
* **Gap Recovery**: Gateway シーケンス欠落からの Backfill 対象時間範囲（Gap）特定規則
* **Periodic Reconciliation**: 直近履歴を定期スキャンして整合性を確認するスケジューリングモデル
* **Pagination & Progress Semantics**: クロール進行カーソルと中断・再開の表現
* **Discord Rate Limit Semantics**: Discord HTTP API レート制限（429）への追従と協調モデル
