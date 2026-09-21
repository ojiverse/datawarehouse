# ADR-0015: Observation の cross-language contract に JSON Schema を採用する

* **ステータス**: 承認（Accepted）
* **決定日**: 2026-09-21
* **対象領域**: Data Contract / Go-TypeScript Boundary

## コンテキスト

ADR-0014 により portable DWH core は Go、Cloudflare native adapter は TypeScript で実装することを決定した。

Observation Envelope は両言語が共有する永続 contract であるため、各言語の struct / type を独立に実装すると field name、nullability、source-specific provenance が乖離する。

実際に first-MVP の並列検証で TypeScript producer と Go reader が異なる physical Envelope shape を実装し、共通 contract の authority が必要であることが確認された。

## 検討した選択肢

### Protobuf / ProtoJSON

RPC では強力だが、Observation Archive は Discord の JSON payload を evidence として長期保存する責務を持つ。

binary Protobuf は確定済み JSON + gzip Archive format を変更し、ProtoJSON は Archive の raw JSON evidence と RPC schema の責務を混在させる。

### JSON Schema

確定済み JSON Archive format を維持しつつ、Go / TypeScript に依存しない wire contract と fixture validation を提供できる。

## 決定

* Observation Archive の wire schema の Source of Truth に JSON Schema を採用する
* `contracts/observation-envelope/v1/schema.json` を Envelope v1 の authoritative physical contract とする
* versioned fixture を compatibility oracle とする
* Go / TypeScript type は schema から導出される implementation artifact とする
* CI で schema validation と cross-language fixture compatibility を検証する
* Archive contract に Protobuf / ProtoJSON を使用しない
* 将来の service-to-service RPC では Protobuf / Connect を別責務として採用可能とする

## 結果

Go と TypeScript が独立した schema authority を持たず、Archive の persisted bytes と domain semantics の境界を明示できる。

RPC protocol を将来追加しても Observation Archive format を変更する必要がない。
