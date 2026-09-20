# Cloudflare Backfill アーキテクチャ

Discord HTTP API を利用した Backfill Domain を Cloudflare 上で実行する方法を扱います。

## 対象

- Backfill run の起動
- Pagination を跨いだ継続実行
- Discord rate limit への追従
- Retry
- Progress の表現
- Observation Archive への保存
- Periodic reconciliation の scheduling

## 基本原則

Backfill の progress state を特定の mutable database に依存させることは現時点では必須としません。

処理単位と continuation state を durable message や Observation と組み合わせて表現できるかを検討します。

D1 は必要性が明確になった場合に導入を再検討します。

## 対象外

Queue 名、retention、environment ごとの schedule などはインフラストラクチャ設計で扱います。

## 今後分割する詳細設計

- Backfill Execution
- Pagination Continuation
- Rate Limit Handling
- Reconciliation Scheduling
- Backfill Failure Recovery
