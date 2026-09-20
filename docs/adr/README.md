# アーキテクチャ意思決定記録（ADR）

本ディレクトリでは、OJIverse Data Warehouse における重要な技術的・設計上の意思決定と、その判断に至った背景やトレードオフを ADR（Architecture Decision Records）として記録・管理します。

## ADR の役割と運用方針

通常の設計文書が「現在のシステムがどう設計されているか」を説明するのに対し、ADR は「どのような代替案を比較検討し、なぜその決定に至ったのか」という決定プロセスを永続化する目的を持ちます。

* **歴史の改ざん禁止**: 一度合意された ADR は後から直接改変せず、過去の事実として保存します。
* **Supersede（置換）による更新**: 前提条件の変化や要件変更によって判断を改める場合は、新しい ADR を起票して古い ADR を `Superseded by ADR-xxx` として参照・更新します。
* **文書制約の継承**: 各 ADR もプロジェクト共通の原則に従い、自然言語を中心に論理パラグラフで記述し、200行以内を維持します。

## 採択済み ADR 一覧

本プロジェクトの基本アーキテクチャを決定づけた主要な意思決定の一覧です。

| 番号 | タイトル | 決定の要点 | ステータス |
| :--- | :--- | :--- | :--- |
| [0001](0001-two-layer-storage-architecture.md) | **2層ストレージアーキテクチャ** | 生ログ（Observation）と分析モデル（Canonical）を分離し、いつでも全再構築を可能にする | 承認（Accepted） |
| [0002](0002-observation-archive-as-source-of-evidence.md) | **生ログの唯一の事実証跡化** | HTTP 取得データを架空の Gateway イベントに偽装せず、厳格な来歴（Provenance）を保持する | 承認（Accepted） |
| [0003](0003-avoid-relational-db-in-core-dwh.md) | **コア DWH における RDB 排除** | 書き込み限界とコストを回避するため、D1 等のリレーショナル DB をデータパスから排除する | 承認（Accepted） |
| [0004](0004-http-backfill-as-anti-entropy.md) | **Backfill の定常アンチエントロピー化** | 単なる初期移行ツールではなく、リアルタイム欠損を定常修復する中核機構として位置づける | 承認（Accepted） |
| [0005](0005-cloudflare-as-primary-platform.md) | **主要基盤としての Cloudflare 採用** | 月額 $5〜 の超低コスト運用、R2 の転送量無料、Durable Objects の WebSocket 統合性を評価 | 承認（Accepted） |
| [0006](0006-durable-objects-for-gateway-session.md) | **Gateway 管理への Durable Objects 採用** | 単一シャードの常時接続と Resume 状態の排他的維持を、サーバーレス環境で最小コストで実現 | 承認（Accepted） |
