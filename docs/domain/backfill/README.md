# バックフィルドメイン（Backfill）

本ディレクトリでは、Discord HTTP API を利用して過去データおよび欠損区間の Observation を取得し、Gateway のリアルタイム受信を補完するドメイン設計を定義する。

## Backfill の機能的位置づけ

Backfill エンジンは、単一目的に特化させるのではなく、以下のユースケースにおいて共通のコアロジックとして機能するよう設計する。

* **初期データロード（Cold Start）**: DWH 初回セットアップ時における、サーバー（ギルド）内過去メッセージ履歴の一括取得
* **Gateway 切断区間の回復（Gap Fill）**: Gateway Session の Resume が成立しなかった区間の補完
* **インシデント復旧（Repair）**: 障害等によって欠損した特定データ区間の再取得
* **定常的な整合性維持（Reconciliation）**: 直近履歴を定期スキャンし、リアルタイム受信で見逃された潜在的欠損を自己修復

## 基本原則とデータ完全性の限界

### Gateway 履歴と HTTP スナップショットの違い
HTTP Backfill は、過去にリアルタイムで発生した「すべてのイベント履歴」を復元するものではない。

HTTP API から取得可能なデータは、リクエスト時点で Discord 上に残存する状態に限定される。

例えば、Gateway 停止中に投稿後すぐ削除されたメッセージや、複数回編集されたメッセージの中間状態を HTTP API から取得することはできない。

したがって、Gateway 由来データと Backfill 由来データで保証可能な完全性の差異を前提とし、Backfill データを架空のイベント履歴として扱わない。

## Durable Progress

Backfill は数時間から数日にわたって中断と再開を繰り返す可能性があるため、一時的な execution trigger や process memory だけを進行状態の唯一の根拠としてはならない。

各 Backfill run は、少なくとも対象範囲、現在の取得位置、完了状態を復元可能な durable progress を持たなければならない。

実行単位や実行基盤が停止しても、durable progress から未完了 run を再開できることを不変条件とする。

進行状態をどの storage mechanism へ保存するかは Architecture Design で決定する。

## アンチエントロピー機構（Anti-Entropy）

HTTP API は Gateway の単なる非常用バックアップではなく、データの完全性を能動的に維持するアンチエントロピー機構として位置づける。

定期的に直近のチャンネル履歴を HTTP 経由で再観測し、取得結果を新しい Observation として無条件に追記する。取り込み前に Observation Archive や Canonical Store と比較して保存要否を決めない。

重複は後続の deterministic projection で収束させ、Reconciliation の correctness を Canonical Store の正しさに依存させない。

定期照合は長期間の Cold Start や過去データ取得の durable progress の代替にはならない。

## 分割予定の詳細設計

* **Message History Crawling**: ページネーションを用いたチャンネル巡回アルゴリズム
* **Thread Discovery**: アーカイブ済みスレッドおよび新規スレッドの探索・検出ロジック
* **Gap Recovery**: Gateway の欠損から Backfill 対象範囲を特定する規則
* **Periodic Reconciliation**: 直近履歴を定期スキャンして整合性を確認するモデル
* **Backfill Run Model**: run identity、対象範囲、完了状態の意味論
* **Pagination & Durable Progress**: クロール進行カーソルと中断・再開の表現
* **Discord Rate Limit Semantics**: Discord HTTP API の rate limit に追従する協調モデル
