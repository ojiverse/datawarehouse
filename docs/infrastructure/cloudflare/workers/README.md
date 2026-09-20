# Cloudflare Workers インフラ設計

本ディレクトリでは Discord DWH を構成する stateless Worker entry point と Durable Object host の責務分割を定義する。

## Service Baseline

| サービス | Entry point | 主な責務 | 主な binding |
| :--- | :--- | :--- | :--- |
| **Gateway Worker** | HTTP / DO routing | Gateway Session Durable Object の host と管理 endpoint | Durable Objects, R2, Secrets |
| **Backfill API Worker** | HTTP | Backfill run の認証、validation、Channel DO への routing、status | Durable Objects, Secrets |
| **External Ingest Worker** | HTTP | 非 Cloudflare Gateway の認証、Envelope validation、DWH-public admission、R2 commit | R2, auth secrets |
| **Query API Worker** | HTTP | OJIverse membership admission、R2 SQL query、response | R2 SQL / Data Catalog, auth |

first-MVP では Observation Queue Consumer と Canonical Processing Worker を必須 resource としない。

Observation は producer から R2 へ direct write し、Canonical materializer は PyIceberg external process として実行する。

## Worker / Durable Object Boundary

Worker は stateless authentication、validation、routing、response formatting を担当する。

strong consistency、serialized ownership、durable progress、scheduled continuation が必要な state は Durable Object が所有する。

Backfill progress を Worker memory や Queue message に保存しない。

## Binding Principle

各 Worker には責務遂行に必要な最小 binding のみを付与する。

* Backfill API Worker は Observation bucket へ直接書き込まず Channel DO を経由する
* Gateway Worker / Session DO は Observation bucket write を持つ
* External Ingest Worker は Observation bucket create-only write を持つ
* Query API Worker は Canonical read/query capability を持ち Observation write を持たない

## Future Incremental Processing

Canonical freshness のために Queue consumer / Pipelines bridge 等を追加する場合も独立 service とし、Observation Archive ingestion と failure domain を共有しない。
