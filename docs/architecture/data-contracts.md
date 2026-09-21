# Cross-language Data Contract

本書では Go と TypeScript を跨ぐ永続データ契約の Source of Truth を定義する。

## Persisted Contract

Observation Archive の永続形式には **JSON Schema** を採用する。

repository root の `contracts/` 配下に versioned schema と fixture を置き、Go / TypeScript の双方が同じ contract を参照する。

Observation Envelope v1 では `contracts/observation-envelope/v1/schema.json` を物理 wire contract の唯一の定義とする。

Domain 文書は field の意味と不変条件を定義し、JSON Schema は field name、required / nullable、primitive shape 等の wire representation を定義する。

両者が矛盾した場合は実装者が独自判断で補正せず、設計へ戻す。

## Generated / Hand-written Types

Go struct、TypeScript type、validator は JSON Schema から導出される implementation artifact とする。

型生成を利用してもよいが、生成物自体を contract の Source of Truth にしない。

Go と TypeScript がそれぞれ独立して field name や nullability を決めてはならない。

## Compatibility Fixtures

各 version は少なくとも1つ以上の representative fixture を持つ。

fixture は JSON Schema に適合し、Go / TypeScript 双方の compatibility test で読み込む。

producer が生成した JSON を consumer が decode できること、consumer が decode / encode しても domain semantics が変わらないことを CI で検証する。

JSON object の byte-for-byte equality は要求しない。field order、insignificant whitespace 等を除外した semantic JSON equivalence を比較する。

## Evolution

既存 consumer が意味を失わず受理できる additive change は同じ Envelope Version 内で schema を拡張できる。

required field の削除、既存 field の意味変更、型変更、payload boundary の変更等の backward-incompatible change では Envelope Version を上げる。

forward compatibility のため、schema は unknown additive field を原則拒否しない。ただし required field と source-specific minimum shape は厳格に検証する。

## Protobuf の位置づけ

Observation Archive の永続 format として Protobuf / ProtoJSON は採用しない。

Discord の source payload が JSON であり、Archive では unknown source field の保持と人間が直接検査可能な evidence を優先するためである。

将来 Go service と TypeScript Worker 間に明示的な RPC boundary が必要になった場合は、Protobuf / Connect 等を別 ADR で採用できる。

RPC schema と Observation Archive schema を同じ責務として扱わない。

## Canonical Store

Canonical Store の persisted schema authority は Apache Iceberg schema とする。

Observation JSON Schema を Canonical Iceberg schema の代替として使用しない。
