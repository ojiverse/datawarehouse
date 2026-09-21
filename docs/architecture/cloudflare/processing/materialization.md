# Canonical Materialization Architecture

本書では first-MVP における Observation Archive から Apache Iceberg Canonical Store への materialization と rebuild の実行方式を定義する。

## Implementation Language

Materializer、Archive replay、full rebuild、semantic validation は **Go** で実装する。

Cloudflare runtime へ依存しない standalone process とし、言語境界は Apache Iceberg REST Catalog、Parquet、Observation Envelope 等の標準・versioned contract に置く。

## Materializer Runtime

first-MVP の materializer は **Apache Iceberg REST Catalog を標準境界とし、iceberg-go を第一候補として実装する**。

#36 technical spike では iceberg-go から R2 Data Catalog への接続、table create / commit、Parquet data file の登録、R2 SQL query、delete / rebuild を実証する。

#36 が成功した場合は Go materializer を採用する。失敗した場合は PyIceberg 等へ自動的に fallback せず、blocking reason を記録して設計へ戻す。

Materializer は Workers の request CPU budget に依存させず、local batch、CI、専用 batch runtime 等の standalone process として実行可能にする。

R2 Data Catalog の Iceberg REST Catalog と R2 の S3-compatible data access を利用し、Cloudflare 固有 runtime や特定言語 implementation を Canonical semantics の boundary にしない。

Cloudflare Pipelines は将来の incremental acceleration 候補であり、first-MVP の rebuild correctness の前提にしない。

## Canonical Physical Format

Canonical Store の data file format は Apache Parquet とする。

Parquet compression は Zstandard を baseline とする。

Discord Snowflake は Canonical schema でも decimal string として保持し、分析用 timestamp を別 column として materialize する。

## Rebuild Input Snapshot

Rebuild は開始時点で入力となる Archive object set を固定する。

R2 object listing と object uploaded time を利用して、rebuild start cutoff 以前に commit 済みの Archive object key を immutable input manifest として記録する。

Rebuild 実行中に追加された Observation はその run の入力へ混入させない。

Input manifest は Source of Evidence ではなく rebuild control metadata である。

## Control Storage

Rebuild manifest、checkpoint、chunk state は専用 R2 control bucket に保存する。

Control bucket の情報は Canonical の correctness を補助する execution state であり、Observation Archive を代替しない。

各 rebuild run は UUIDv7 Run ID を持つ。

Manifest は immutable chunk に分割し、progress はどの chunk が materialize 済みかを durable に復元できるようにする。

## Resumable Chunk Processing

各 manifest chunk から生成する staging Parquet file は rebuild run と chunk identity から deterministic に識別できるようにする。

同じ chunk の再実行で異なる logical row set を生成してはならない。

既存 staging file が存在する場合は content / metadata consistency を確認し、同じ結果なら再利用する。

Crash 後は完成済み chunk を再生成せず、未完了 chunk から再開できる。

## Iceberg Commit

完成した staging Parquet file を Iceberg REST Catalog 経由で Iceberg table へ commit する。

iceberg-go を採用する場合も、commit retry によって同じ data file を二重登録しないことを invariant とする。

Crash が Iceberg commit と control checkpoint の間で発生した場合は Catalog を再読込し、既に参照済みの data file を成功済みとして扱う。

## Full Rebuild

Full Rebuild は既存 Canonical Store を input に使用しない。

Observation Archive、確定した projection version、schema definition のみから新しい Canonical table を生成する。

Acceptance Test では Discord credential を持たない環境で rebuild を実行し、外部 API への暗黙依存がないことを証明する。

## Incremental Processing

incremental trigger は latency optimization であり correctness source ではない。

R2 Event Notification、Queue、Pipelines 等の trigger が欠損しても、Archive listing と rebuild / replay により Canonical を収束できなければならない。
