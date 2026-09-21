# ADR-0014: Go-first とし Cloudflare native adapter に TypeScript を使用する

* **ステータス**: 承認（Accepted）
* **決定日**: 2026-09-21
* **対象領域**: Implementation Architecture

## コンテキスト

OJIverse DWH は Cloudflare 上の Workers / Durable Objects と、Cloudflare runtime から独立して実行できる materializer / rebuild / tooling の双方を持つ。

全 component を単一言語へ統一すると、Cloudflare native runtime に Go / WebAssembly を強制するか、portable data processing を TypeScript に寄せる必要がある。

前者は Workers / Durable Objects の binding、Alarm、WebSocket lifecycle 等との統合を不必要に複雑化し、後者は standalone batch / CLI / Iceberg processing の portability と実装適性を下げる。

ADR-0013 により Iceberg materializer は iceberg-go を第一候補として検証する方針が既に確定している。

## 決定

本プロジェクトの既定実装言語を **Go** とする。

Cloudflare native runtime API に密接に依存する Workers / Durable Objects component に限って **TypeScript** を使用する。

### Go

* Canonical materializer / Iceberg
* replay / rebuild
* validation / semantic equivalence tooling
* CLI / maintenance tooling
* portable batch processing
* Cloudflare 外 Gateway collector

### TypeScript

* Gateway Session Durable Object
* Backfill Channel Durable Object
* HTTP Budget / Identify Coordinator Durable Object
* Backfill / External Ingest / Query API Worker
* Cloudflare binding / Alarm / WebSocket lifecycle adapter

Cloudflare component を言語統一だけを目的に Go / WebAssembly 化しない。

Python その他の第三言語は、Go / TypeScript で満たせない具体的な capability が確認された場合にだけ追加し、Production dependency なら新 ADR を要求する。

## Contract Boundary

Go と TypeScript は Domain semantics を別々に所有しない。

Observation Envelope、identity、Snowflake、Canonical schema 等の versioned persisted contract を境界とし、共通 fixture による compatibility test を要求する。

portable transformation / Canonical semantics は Go 側へ寄せ、TypeScript は Cloudflare-specific coordination / I/O adapter とする。

## 結果

主要な DWH implementation を Go に統一しながら、Cloudflare runtime では TypeScript の first-class integration を利用できる。

将来 Cloudflare 外へ component を移す場合も、platform-specific TypeScript adapter の交換範囲を局所化できる。
