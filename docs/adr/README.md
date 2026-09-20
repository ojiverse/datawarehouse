# アーキテクチャ意思決定記録（ADR）

本ディレクトリでは、OJIverse Data Warehouse における重要な技術的・設計上の意思決定、ならびに判断に至った背景およびトレードオフを ADR（Architecture Decision Records）として記録・管理する。

## ADR の役割と運用方針

通常の設計文書がシステムの現状仕様を記述するのに対し、ADR は代替案の比較検討および意思決定に至った論理的根拠を永続化する目的を持つ。

* **歴史の改ざん禁止**: 一度合意された ADR は事後改変せず、意思決定時点の事実として保存する。
* **Supersede（置換）による更新**: 前提条件の変化や要件変更によって判断を改める場合は、新規 ADR を起票して旧 ADR を `Superseded by ADR-xxx` として参照・更新する。
* **文書制約の継承**: 各 ADR もプロジェクト共通規約に従い、自然言語を中心とした論理パラグラフで記述し、200行以内を遵守する。

## 採択済み ADR 一覧

本プロジェクトの基本アーキテクチャを決定づけた主要な意思決定の一覧を示す。

| 番号 | タイトル | 決定の要点 | ステータス |
| :--- | :--- | :--- | :--- |
| [0001](0001-two-layer-storage-architecture.md) | **2層ストレージアーキテクチャ** | 生ログ（Observation）と分析モデル（Canonical）を分離し、全再構築可能性を担保 | 承認（Accepted） |
| [0002](0002-observation-archive-as-source-of-evidence.md) | **生ログの唯一の事実証跡化** | HTTP 取得データを架空の Gateway イベントに偽装せず、厳格な来歴（Provenance）を保持 | 承認（Accepted） |
| [0003](0003-avoid-relational-db-in-core-dwh.md) | **コア DWH における RDB 排除** | 書き込み限界とコストを回避するため、D1 等のリレーショナル DB をデータパスから排除 | 承認（Accepted） |
| [0004](0004-http-backfill-as-anti-entropy.md) | **Backfill の定常アンチエントロピー化** | 単なる初期移行ツールではなく、リアルタイム欠損を定常修復する中核機構として位置づけ | 承認（Accepted） |
| [0005](0005-cloudflare-as-primary-platform.md) | **主要基盤としての Cloudflare 採用** | 低コスト運用、R2 の転送量無料、Durable Objects の WebSocket 統合性を評価 | 承認（Accepted） |
| [0006](0006-durable-objects-for-gateway-session.md) | **Gateway 管理への Durable Objects 採用** | Cloudflare 上の Gateway Session の所有と Resume 状態管理に Durable Objects を採用 | 一部置換（ADR-0007） |
| [0007](0007-multiple-gateway-instances.md) | **複数 Gateway Instance の並行稼働** | 同一 shard を複数の独立 Session が観測できる observer fleet として Gateway をモデル化 | 承認（Accepted） |
