# データ処理ドメイン（Processing）

本ディレクトリでは、Observation Archive に蓄積された生の観測事実（Observation）を解釈・正規化し、分析用の Canonical Data へと変換・マテリアライズする処理パイプラインの意味論を扱います。

## 本ドキュメントの責務と基本原則

### 1. 取り込みと処理の障害隔離（Durability First）
正規化パイプラインのコードにバグがあったり、一時的なダウンストリーム障害によって処理が失敗した場合であっても、**前段の Observation Archive への生データ保存が決して阻害されてはなりません**。
Observation の耐久性（Durability）を最優先とし、処理層の不具合は「バグを修正した上で、後から生データを再読み込み（Replay）してリカバリ可能」なアーキテクチャを前提とします。

### 2. データ来歴の保持と非改変
正規化処理において、HTTP スナップショットを架空の Gateway イベントへ変換（捏造）してはなりません。
Canonical Data に変換された後であっても、元になった Observation の取得経路（Gateway または HTTP）、受信時刻、およびセッション情報が完全に追跡（Provenance）できる状態を維持します。

### 3. 重複排除と順序解決（Deduplication & Ordering）
ネットワークのリトライや Resume replay によって、同一メッセージに関する複数の Observation がアーカイブへ届く場合があります。
処理層は、Discord の Snowflake ID（ミリ秒精度の作成時刻が埋め込まれている）やシーケンス番号、観測時刻を手がかりとして、意図的な重複を適切に集約・排除し、論理的に正しいイベント順序を確定します。

## ファーストクラスの機能としての「再処理（Replay & Rebuild）」

一般的なウェブアプリケーションにおいて、過去データの全件再処理（マイグレーション）は例外的な緊急対応であることが大半です。
しかし、5〜10年にわたって稼働する DWH においては、**分析要件の追加、スキーマ定義の変更、および正規化ロジックの改善に伴う再処理は「日常的な定常機能（System Capability）」**として設計されなければなりません。

* **Replay**: 特定の時間範囲の Observation を再度パイプラインに流し、差分や誤変換を修復する。
* **Rebuild**: 新しい Canonical スキーマに基づいて、最初からの全 Observation をゼロベースで再変換し、新しいテーブル表現を構築する。

## 今後分割する詳細設計

* **Observation Normalization**: Discord の多層 JSON からフラットな Canonical 表現へのマッピング規則
* **Deduplication**: メッセージ ID および観測ハッシュに基づく重複排除のアルゴリズム
* **Ordering**: Snowflake ID と観測時刻を用いたイベント順序の決定ロジック
* **Gateway & Snapshot Reconciliation**: リアルタイムイベントと HTTP スナップショットの競合解決
* **Replay & Rebuild**: 過去 Observation の再読み込み手順と Canonical Store の無停止切り替え
