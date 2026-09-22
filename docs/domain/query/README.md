# クエリドメイン（Query）

本ディレクトリでは、Canonical Store に格納されたデータを対象に、Discord の最新状態（Current State）および過去の変更履歴（Historical State）を問い合わせ・集計する際の意味論とクエリモデルを定義する。

## クエリ設計の中核概念

本設計では特定のクエリエンジン（DuckDB、ClickHouse、R2 SQL 等）や SQL 方言の具体構文には立ち入らず、データウェアハウスとして提供すべき問い合わせの論理モデルを定義する。

### プロジェクション（Projection）としての最新状態
一般的な Web アプリケーションでは「現在の状態」が 1 つの更新可能行として保存される。
しかし本 DWH では、最新状態を静的な物理レコードとして固定化することを前提としない。

Observation Archive に保存された evidence から、**対象時点（または現時点）における best-known state を deterministic に導出する「プロジェクション（Projection）」**として状態を定義する。

### 過去時点の best-known state
Historical State at T は「Discord が実際に T 時点で持っていた完全な状態」とは定義しない。

対象時点までに利用可能な Observation と provenance から導出できる best-known state とする。Gateway で観測できなかった event や、HTTP Backfill では復元できない中間状態を推測して補完してはならない。

## クエリおよび分析のユースケース

本 DWH が対象とする主要な問い合わせ要件は以下の通りである。

* **メッセージ履歴検索**: 特定チャンネルやスレッドにおける、編集・削除履歴を含めたメッセージの時系列追跡。
* **サーバー活動状況の集計**: チャンネル別・時間帯別のアクティビティ推移、投稿数、発言ユーザー数の集計。
* **エンティティ関係性の追跡**: スレッドと親チャンネル、メッセージと添付ファイル・リアクションの関連性分析。
* **生データへのドリルダウン**: 集計結果から根拠となった生の Observation まで辿れるトレーサビリティの提供。

## first-MVP の Query Semantics（Issue #41）

first-MVP の Canonical Message は、materializer が Observation Archive から best-known state を Message ID ごとに 1 件へ deterministic に導出済みである（`docs/domain/processing/projection.md`）。したがって first-MVP の Query API は Canonical Store を単純な等価・範囲・集計で問い合わせるのみで best-known state の投影を担わず、Current State / Historical State の投影規則そのものは引き続き将来の詳細設計課題とする。

時刻基準は 2 種類あり、区別して扱う。

* **Discord 作成時刻**（Canonical の `created_at`）: Discord が返す Message の作成時刻。first-MVP の作成時刻 range filter はこの列を対象とする。
* **観測時刻**（Canonical の `observed_at`）: best-known state 選定にのみ用いる内部時刻で、first-MVP の Query API では filter 対象にしない。

Snowflake ID（`message_id` / `channel_id` / `author_id` / `guild_id`）は decimal string のまま扱い、集計・フィルタの等価比較も文字列として行う。

## 分割予定の詳細設計

* **Current State Projection**: 編集・削除イベントを畳み込んで最新状態を導出する計算規則
* **Historical Projection**: 指定したタイムスタンプ（Point-in-Time）時点の状態を再現するクエリセマンティクス
