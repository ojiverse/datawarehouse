# Cloudflare 運用アーキテクチャ（Operations）

本ディレクトリでは Discord DWH の correctness と継続運用を確認するための signal と Production Readiness を定義する。

## 主要 Operational Signal

| 対象 | 異常 | 対処 |
| :--- | :--- | :--- |
| **Gateway Liveness** | Heartbeat ACK 欠落、unexpected disconnect | connection close / Resume |
| **Gateway Durability Gap** | Received Sequence と Accepted Sequence の gap が継続 | R2 write failure を調査し必要なら Accepted Sequence から Resume |
| **Resume Failure** | Invalid Session | new Identify + HTTP Backfill / Reconciliation |
| **Archive Write** | R2 put error、conditional collision mismatch | source progress を止め、invariant violation を alert |
| **Backfill Progress** | active run の cursor / alarm が一定時間進まない | Channel DO state と Discord rate-limit state を調査 |
| **Discord HTTP Budget** | 429 増加、invalid-request budget 消費増大 | route pacing / global coordination を抑制 |
| **Canonical Freshness** | Archive cutoff と Canonical commit の差が拡大 | incremental trigger ではなく replay / materializer state を確認 |
| **Rebuild Progress** | manifest chunk が進まない | R2 control checkpoint と PyIceberg commit を調査 |

## Completeness Signal

Observation Archive と Discord の完全一致を正常性指標にしない。

Product Policy 上、DWH は best-known state を提供する。HTTP Reconciliation による再観測、Gateway Session continuity、Backfill completion、provenance coverage を使って evidence の quality を評価する。

未観測 history を補完したように見せる metric を作らない。

## Development Gate

first-MVP では以下を確認する。

* HTTP Backfill が restart を跨いで完走する
* R2 Archive が retry / refetch で破損しない
* PyIceberg で Archive-only rebuild が成立する
* Discord credential がない環境でも rebuild できる
* semantic equivalence test が deterministic に pass する

## Gateway Beta Gate

最初に #18 の outbound WebSocket lifecycle を実測する。

その後、Accepted Sequence からの Resume、Archive outage、Invalid Session、Backfill repair、Identify budget、複数 Gateway Instance を fault injection で確認する。

## Production Readiness

Production へ移行するには、通常運用だけでなく Archive write failure、runtime restart、Session loss、Canonical full rebuild の回復手順が再現可能でなければならない。

具体 alert threshold と通知先は observability / runbook で管理する。
