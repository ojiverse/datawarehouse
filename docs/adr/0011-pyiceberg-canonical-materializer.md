# ADR-0011: first-MVP の Canonical materializer に PyIceberg を採用する

* **ステータス**: 置換済み（Superseded by ADR-0013）
* **決定日**: 2026-09-20
* **対象領域**: Canonical Store / Processing

> **置換注記**: PyIceberg + PyArrow を固定 implementation とする決定は ADR-0013 により置換された。本文は当時の判断履歴として保持する。

## コンテキスト

Canonical Store は R2 Data Catalog 上の Apache Iceberg table とし、R2 SQL から query する。

Workers 上で独自の Iceberg metadata writer を実装すると複雑性と vendor/runtime coupling が増える。

R2 Data Catalog は標準 Iceberg REST Catalog を公開し、PyIceberg を公式の接続手段として案内している。

Cloudflare Pipelines も Iceberg sink を提供するが open beta であり、source model と runtime dependency を first-MVP rebuild correctness に持ち込む必要はない。

## 決定

* first-MVP の authoritative materializer / rebuild engine に PyIceberg + PyArrow を使用する
* materializer は local / CI / batch runtime から実行可能な外部 process とする
* Canonical data file は Parquet とする
* compression は Zstandard とする
* R2 Data Catalog を Iceberg REST Catalog として利用する
* R2 SQL を first-MVP query engine とする
* rebuild manifest / checkpoint は専用 R2 control bucket に保存する
* rebuild input object set は run 開始時に immutable manifest として固定する
* staging Parquet file を deterministic chunk identity で生成し、retry で再利用する
* PyIceberg の duplicate file detection を有効にする

## Beta Product Risk

R2 Data Catalog と R2 SQL は 2026-09-20 時点で beta product である。

Canonical semantics を Cloudflare 固有 SQL や proprietary metadata に閉じ込めず、Iceberg REST Catalog と Parquet を exit path とする。

Pipelines は incremental acceleration の候補に留め、PyIceberg rebuild capability を維持する。

## 結果

first-MVP で Archive → Canonical → Query → Rebuild を実証しつつ、Cloudflare runtime の CPU limit と Pipelines availability から materializer correctness を分離できる。
