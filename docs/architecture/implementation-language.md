# Implementation Language Policy

本書では OJIverse Data Warehouse の実装言語境界を定義する。

## Go-first

本プロジェクトの既定実装言語は **Go** とする。

Cloudflare 固有 runtime に依存せず実装できる DWH logic、batch processing、CLI、tooling、外部 collector は原則として Go で実装する。

新しい component に別言語を採用する場合は、Go では責務を適切に実現できない具体的な platform / ecosystem constraint が必要である。

単なる library preference や実装者の慣れを理由に runtime language を増やさない。

## Go が所有する領域

少なくとも以下は Go を第一選択とする。

* Canonical materializer / Apache Iceberg integration
* Archive replay / full rebuild
* semantic equivalence / validation tooling
* DWH CLI と migration / maintenance tooling
* portable batch processing
* Cloudflare 外で動作する Gateway collector
* platform-independent な data transformation

Iceberg integration は ADR-0013 に従い iceberg-go を第一候補として #36 で実証する。

## TypeScript を使用する領域

Cloudflare Workers / Durable Objects の runtime API と密接に結合する component は **TypeScript** で実装する。

対象には以下を含む。

* Gateway Session Durable Object
* Backfill Channel Durable Object
* Discord HTTP Budget Durable Object
* Gateway Identify Coordinator Durable Object
* Backfill API Worker
* External Ingest Worker
* Query API Worker
* Cloudflare bindings / Alarm / WebSocket lifecycle を直接扱う adapter

これらを言語統一だけを目的に Go / WebAssembly 化しない。

Cloudflare native API への adapter を TypeScript に限定することで、platform-specific concern を portability boundary の内側へ閉じ込める。

## Go と TypeScript の責務境界

TypeScript component は Cloudflare resource ownership、coordination、authentication、routing、I/O を担当する。

portable に表現できる transformation、rebuild、validation、Canonical semantics は Go 側へ置く。

同じ domain rule を Go と TypeScript に別々の権威として実装しない。

Domain semantics の Source of Truth は docs/domain 配下の設計文書であり、両言語の実装は同じ contract に従う。

## Cross-language Contract

言語間の境界は implementation-specific object や in-memory type ではなく、versioned contract で接続する。

主な境界は以下とする。

* Observation Envelope v1
* R2 Observation Archive object
* Discord Snowflake の decimal string representation
* UUIDv7 Observation / Run identity
* Canonical Iceberg schema
* HTTP / RPC boundary が必要な場合の versioned request / response contract

Go と TypeScript の双方で扱う永続 contract は [Cross-language Data Contract](data-contracts.md) に従う。

Observation Envelope v1 の物理 wire contract は `contracts/observation-envelope/v1/schema.json` を Source of Truth とし、Go / TypeScript の struct / type はその implementation artifact とする。

versioned fixture を両言語の compatibility test で共有する。

Observation Archive の schema 共有だけを目的に Protobuf / ProtoJSON を導入しない。将来明示的な RPC boundary が必要になった場合は Protobuf / Connect 等を別責務として検討する。

一方の implementation detail を他方が暗黙に知る設計を避ける。

## Python その他の言語

Python を first-MVP の必須 runtime としない。

Go または TypeScript で合理的に実現できない capability が確認された場合に限り、別言語を追加できる。

新しい runtime language を Production dependency として追加する場合は ADR を要求する。

## 判断原則

言語統一そのものを目的にしない。

portable DWH core を Go に寄せ、Cloudflare native adapter を TypeScript に限定することで、実装言語数を抑えながら platform-native capability と portability の双方を維持する。
