# Cloudflare 運用アーキテクチャ（Operations）

本ディレクトリでは、Cloudflare 上で稼働する Discord DWH が長期間にわたり安定して動作し続けるために、アプリケーションアーキテクチャが備えるべき監視・障害検知機能、整合性検証能力、および本番稼働判定（Production Readiness）を定義する。

## 監視・検知すべき主要な運用シグナル

具体的なアラート通知先やログ設定はインフラ層へ委託し、本節では「何を以て異常と判定するか」の検知ロジックを定義する。

| 観測対象 | 異常と判定する条件 | アーキテクチャ上の対処 |
| :--- | :--- | :--- |
| **Gateway 活性度** | ハートビートの ACK 欠落連続、または WebSocket の予期せぬ切断 | DO 内で切断を検知し、直ちに再接続・Resume を試行 |
| **Resume 失敗率** | 切断後の Resume 試行に対し、Invalid Session（Opcode 9）を受信 | 新規セッション（Identify）へ切り替え、欠損区間の補完タスクを Backfill へ発行 |
| **取り込み滞留（Lag）** | Queues 内の未処理メッセージ数が閾値を超過、または R2 書き込みレイテンシが増大 | バッファリング制限を発動し、Consumer ワーカーのスケールアウトを促進 |
| **Canonical 処理遅延** | Observation Archive の最新タイムスタンプと、Canonical Store 最新コミット時刻の乖離が増大 | Processing ワーカーのエラーログ・DLQ を確認し、自動リトライを監視 |
| **Discord レート制限** | HTTP 429 の頻発、または `Retry-After` が長時間の状態 | クローラーの遅延秒数を自動調整し、Backfill キューの消費を一時減速 |

## 段階的検証ロードマップ

本番運用の安定性を段階的に確立するため、以下のフェーズに沿って検証を推進する。

1. **開発フェーズ（HTTP Backfill の検証）**:
   * HTTP Backfill を中心に稼働させ、Queues によるバッチ集約、R2 への Observation 保存、および Iceberg へのマテリアライズの正常性を検証する。
2. **ベータフェーズ（Gateway 連携と回復の検証）**:
   * Gateway 接続を断続的に稼働させ、リアルタイム取り込み、切断時の Resume 成功率、および Resume 失敗時の Backfill 自動補完連携を実測・検証する。
3. **本番移行前フェーズ（カオステストと負荷検証）**:
   * 意図的な切断（Fault Injection）、大量メッセージ発生時のバーストテスト、および長時間連続稼働（長時間のセッション維持）を実施し、Production Readiness を評価する。

## 分割予定の詳細設計

* **Failure Detection**: 各種エラーコード、タイムアウト閾値、およびゾンビ接続の検知アルゴリズム
* **Gateway Verification**: リアルタイム接続時のスループットとメモリ使用量の検証基準
* **Backfill Verification**: ページネーションの網羅性と API レート制限追従の検証手順
* **Data Completeness Verification**: Observation Archive と Discord 実データの突合検証ツール
* **Production Readiness Checklist**: Workers Paid 移行および本番運用開始の判定基準
