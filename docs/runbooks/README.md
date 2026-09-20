# Runbooks

このディレクトリには、運用中に実行する具体的な手順を保存します。

## 設計文書との違い

設計文書は system がどのように振る舞うべきかを説明します。

Runbook は、特定の状態になったときに運用者が何を確認し、何を実行するかを説明します。

## 初期 Runbook 候補

安定稼働へ向けて、必要になった時点で次の Runbook を追加します。

- Gateway disconnected
- Resume failed
- Observation ingestion stopped
- Manual Backfill
- Periodic Reconciliation failure
- Canonical processing lag
- Replay Observations
- Rebuild Canonical Store
- Discord API rate limit incident

Runbook も directory ごとに README.md を持ち、各文書は 200 行以内とします。

実際の操作コマンドや設定例が必要になる場合、設計文書の制約とは分離して管理方法を決定します。
