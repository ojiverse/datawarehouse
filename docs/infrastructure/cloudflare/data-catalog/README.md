# R2 Data Catalog インフラ設計

本ディレクトリでは Canonical Store の Apache Iceberg catalog と R2 SQL integration を定義する。

## Role

R2 Data Catalog を Canonical Store の Iceberg REST Catalog として使用する。

Observation Archive の Source of Evidence ownership は持たない。

Catalog / Canonical table を失っても Observation Archive から再作成可能でなければならない。

## first-MVP Writer

first-MVP の table creation / commit / rebuild には PyIceberg を使用する。

PyIceberg は R2 Data Catalog の REST Catalog へ接続し、R2 上の Parquet data file を Iceberg table として commit する。

Cloudflare Workers 内で独自の Iceberg metadata writer を実装しない。

## Physical Format

Canonical data file は Parquet、compression は Zstandard とする。

staging Parquet file を既存 file として追加する場合、PyIceberg の duplicate file check を有効にする。

Discord Snowflake column は decimal string として保持する。

## Initial Table Scope

first-MVP の必須 table は Message Canonical dataset とする。

Channel、Thread、Reaction 等の table は Product / Canonical requirements が実装対象になった時点で追加する。

## Query

R2 SQL を first-MVP の analytical query engine とする。

R2 SQL / Data Catalog の beta status を前提に、Query semantics を R2 SQL 固有構文へ閉じ込めない。

Catalog が Iceberg REST interface を提供することを exit path とし、別 Iceberg engine からも table を扱える状態を維持する。

## Maintenance

Data Catalog が提供する managed compaction と snapshot expiration を優先して利用する。

同等機能の custom maintenance job を二重に所有しない。

managed feature で扱えない orphan file や rebuild staging file の cleanup だけを独自運用の対象とする。

## Access Control

materializer だけに Catalog / Canonical write 権限を与える。

Query component には read-only capability を付与し、Observation Archive への write 権限を与えない。
