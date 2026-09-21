# Cloudflare Workers インフラ設計

本ディレクトリでは Discord DWH を構成する Cloudflare Worker / Durable Object host の責務、実装言語、および binding boundary を定義する。

Cloudflare native runtime component は [Implementation Language Policy](../../../architecture/implementation-language.md) に従い **TypeScript** で実装する。

## Service Baseline

| サービス | 言語 | Entry point | 主な責務 | 主な binding |
| :--- | :--- | :--- | :--- | :--- |
| **Gateway Worker / Session DO** | TypeScript | HTTP / DO routing | Gateway Session ownership、WebSocket、Resume state | Durable Objects, R2, Secrets |
| **Backfill API Worker / Channel DO** | TypeScript | HTTP / DO routing | Backfill run、pagination、Alarm、durable progress | Durable Objects, R2, Secrets |
| **HTTP Budget / Identify Coordinator DO** | TypeScript | DO RPC | Discord application-wide coordination | Durable Objects |
| **External Ingest Worker** | TypeScript | HTTP | 外部 Gateway の認証、Envelope validation、R2 commit | R2, auth secrets |
| **Query API Worker** | TypeScript | HTTP | membership admission、R2 SQL query、response | R2 SQL / Data Catalog, auth |

Canonical materializer / replay / rebuild は Worker service とせず、Go の standalone process として実装する。

first-MVP では Observation Queue Consumer と Canonical Processing Worker を必須 resource としない。

## Worker / Durable Object Boundary

Worker は stateless authentication、validation、routing、response formatting を担当する。

strong consistency、serialized ownership、durable progress、scheduled continuation が必要な state は Durable Object が所有する。

Backfill progress を Worker memory や Queue message に保存しない。

## Binding Principle

各 Worker / Durable Object には責務遂行に必要な最小 binding のみを付与する。

* Backfill Channel DO は Observation bucket の create-only write を持つ
* Gateway Session DO は Observation bucket の create-only write を持つ
* External Ingest Worker は Observation bucket の create-only write を持つ
* Query API Worker は Canonical query capability を持ち Observation write を持たない

## Go/Wasm を強制しない

Workers / Durable Objects を言語統一だけを目的に Go / WebAssembly へ移植しない。

Cloudflare native API への密結合部分を TypeScript adapter として隔離し、portable logic を Go へ置くことで portability を確保する。

## Future Incremental Processing

Canonical freshness のために Queue consumer / Pipelines bridge 等を追加する場合も TypeScript の Cloudflare adapter として独立させ、Observation Archive ingestion と failure domain を共有しない。
