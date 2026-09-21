# ADR-0013: Iceberg materializer は Go-first で検証し言語を protocol boundary から分離する

* **ステータス**: 承認（Accepted）
* **決定日**: 2026-09-21
* **対象領域**: Canonical Store / Processing
* **置換対象**: ADR-0011

## コンテキスト

ADR-0011 では first-MVP の Canonical materializer を PyIceberg + PyArrow に固定した。

しかし R2 Data Catalog との本質的な interoperability boundary は Python implementation ではなく、Apache Iceberg REST Catalog と Parquet data file である。

Apache Iceberg には公式の Go implementation が存在し、REST Catalog、Arrow、Parquet、table transaction、append / overwrite 等の first-MVP に必要な capability を提供する。

したがって Python を architecture requirement とする根拠はなく、言語選択を protocol boundary より上位の不変条件へ昇格させるべきではない。

## 決定

* Canonical Store の固定 boundary は Apache Iceberg REST Catalog と Parquet とする
* R2 Data Catalog を Iceberg REST Catalog として利用する
* Canonical data file は Parquet、compression は Zstandard とする
* first-MVP の materializer implementation は **iceberg-go を第一候補**とする
* #36 technical spike で iceberg-go から R2 Data Catalog への table create / commit / query / rebuild を実証する
* #36 が成功した場合、first-MVP materializer は Go 実装で進める
* #36 が失敗した場合、PyIceberg 等へ silently fallback せず、blocking reason を記録して設計へ戻す
* Cloudflare Workers 内で独自の Iceberg metadata writer を実装しない

## Rebuild Contract

materializer implementation language に関係なく、以下は固定する。

* Observation Archive だけから full rebuild できる
* rebuild input object set は run 開始時に immutable manifest として固定する
* rebuild manifest / checkpoint は専用 R2 control bucket に保持する
* staging file は deterministic chunk identity で識別する
* retry で同じ data file を二重登録しない
* Discord credential のない環境でも rebuild できる

## Pipelines の位置づけ

Cloudflare Pipelines は将来の incremental acceleration 候補であり、authoritative rebuild mechanism にはしない。

Pipelines を停止・廃止しても Iceberg REST Catalog を利用する standalone materializer で Canonical Store を再構築できなければならない。

## ADR-0011 との関係

ADR-0011 の Parquet、Zstandard、R2 Data Catalog、R2 SQL、resumable rebuild という方向性は維持する。

PyIceberg + PyArrow を first-MVP の固定 implementation とした部分だけを本 ADR で置換する。

## 結果

Canonical architecture を Python runtime から切り離し、標準 Iceberg interface を長期的な portability boundary とできる。

first-MVP では iceberg-go を最優先で検証することで、実装言語を増やさずに closed loop を成立させられる可能性を先に確認する。
