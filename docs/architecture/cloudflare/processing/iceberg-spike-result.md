# Iceberg Materializer Technical Spike (#36) — Result Record

本書は Issue #36「Archive → Iceberg → R2 SQL の技術成立性を検証する」の実測記録である。

Engineering Baseline（Go、iceberg-go 第一候補、R2 Data Catalog = Iceberg REST Catalog、Parquet + Zstandard、R2 SQL）は変更していない。設計契約の変更は本 spike では不要と判断した。

## 検証対象と実装

検証コードは Go module `github.com/ojiverse/datawarehouse` に置き、`cmd/dwh-spike` が閉ループ（fixture → Archive → materialize → commit → query → drop → rebuild → 等価性検証）を一括実行して JSON report を出力する。

主要な依存は iceberg-go v0.6.0、arrow-go v18.6.0、aws-sdk-go-v2 の S3 client である。

Spike 用 package の責務は次の通り。

* `internal/observation`: Envelope v1 の JSON + gzip encoding、UUIDv7 Observation ID、R2 key layout、SHA-256 payload hash、overlap を含む fixture 生成
* `internal/projection`: docs/domain/processing/projection.md の best-known snapshot 選択規則
* `internal/canonical`: Canonical Message の Iceberg schema、Zstandard Parquet writer、deterministic chunk identity
* `internal/archive`: create-only conditional put、listing、取得
* `internal/icebergcat`: iceberg-go REST Catalog 接続、namespace / table 作成、staging file 書き込み、duplicate protection 付き commit、drop、scan
* `internal/r2sql`: R2 SQL HTTP API client
* `internal/equivalence`: rebuild 前後の semantic equivalence 定義（primary key 順の正規化 serialization、snapshot id / file path / row order / materialization time を除外）

## 検証環境の二段構成

Failure Rule 第 2 項（Iceberg REST / Parquet boundary 自体の問題か iceberg-go 固有の問題かの切り分け）に備え、同一コードを二つの catalog に対して実行する。

1. **Local reference**: MinIO + apache/iceberg-rest-fixture（`spike/local/docker-compose.yml`）。R2 に依存しない標準 REST Catalog での基準結果を得る
2. **R2 real environment**: R2 Observation Archive bucket、R2 Data Catalog、R2 SQL

R2 でのみ失敗する項目は R2 固有、両方で失敗する項目は iceberg-go / protocol 側の問題として分類する。

## Local Reference の結果（2026-09-21 実測）

apache/iceberg-rest-fixture + MinIO に対して閉ループは全項目 PASS した（R2 SQL 項目は環境上 skip）。

| Success Criterion | 結果 | 根拠 |
| :--- | :--- | :--- |
| Envelope v1 fixture を Archive へ保存 | PASS | 4 object を create-only put で保存。同一 object の再 put は Observation ID と payload hash の一致により idempotent success |
| fixture から Canonical Message Parquet を生成 | PASS | 4 page（overlap あり）から 85 row を projection。全 column chunk の codec が ZSTD であることを Parquet metadata から確認 |
| iceberg-go から REST Catalog に接続 | PASS | rest.Catalog で接続 |
| namespace / table を作成 | PASS | `dwh_spike.message` を固定 schema で作成 |
| Parquet data file を Iceberg table へ commit | PASS | table FileIO 経由で staging file を書き、AddFiles で snapshot を作成 |
| commit retry で同じ data file を二重登録しない | PASS | catalog 再読込後の retry は already_referenced として snapshot を増やさない。さらに iceberg-go の AddFiles 自体が既参照 file を拒否することを確認 |
| R2 SQL から query | SKIP | local には R2 SQL が存在しない。代替として iceberg-go scan が projection と一致することを確認 |
| Canonical table を削除 | PASS | purge 後に CheckTableExists = false |
| Discord API へアクセスせず再生成 | PASS | Discord credential 環境変数が未設定であることを assert した上で Archive listing のみから再 materialize |
| 再生成前後の domain data が一致 | PASS | 正規化 serialization の SHA-256 digest が一致（85 row）。chunk identity も一致 |
| runtime constraint の記録 | PASS | 後述 |
| beta 制約の記録 | PENDING | R2 real environment で実測する |

Local 実測の resource は次の通り（macOS arm64、Go 1.26.3、fixture 85 row）。

| 指標 | 値 |
| :--- | :--- |
| user CPU | 約 130 ms |
| system CPU | 約 80 ms |
| max RSS | 約 100 MB |
| Go heap sys | 約 57 MB |
| 各 step の wall time | 10〜80 ms（commit 約 30 ms、drop 約 80 ms） |

この規模では Workers の CPU limit とは比較にならないほど小さいが、materializer は standalone Go process として設計しており、Workers 内実行は対象外である。

## iceberg-go の capability 確認（source 読解 + local 実測）

* REST Catalog client は `X-Iceberg-Access-Delegation: vended-credentials` を送り、catalog が返した storage credential で FileIO を構成する。R2 では API token 1 つで catalog と data file の双方にアクセスできる見込み
* vended credential が返らない catalog では `s3.endpoint` / `s3.region` / `s3.access-key-id` / `s3.secret-access-key` を catalog property として渡す（local reference はこの経路）
* `Transaction.AddFiles(ignoreDuplicates=false)` は現 snapshot が参照する file path との重複を検出して拒否する
* `PurgeTable` / `DropTable`、`CreateNamespace`、`CheckTableExists` を提供する
* `Table.Scan().ToArrowTable` により engine 非依存の読み出しが可能

## 実装上の制約と判断（Why）

* **s3:// FileIO の side-effect import が必須**: `github.com/apache/iceberg-go/io/gocloud` を import しないと metadata / data file の s3 path を解決できず、「io scheme not registered」で失敗する。初回 local 実行で実際に遭遇した
* **avro / arrow の version pin**: iceberg-go v0.6.0 は twmb/avro v1.7.2 と arrow-go v18.6.0 を前提とする。avro v1.8.0 に上げると iceberg-go 内部が compile error になるため、go.mod で v1.7.2 / v18.6.0 に固定した
* **staging file の配置**: vended credential は catalog bucket に scope されるため、spike の staging file は Canonical bucket の table location 配下 `data/staging/run=<run-id>/chunk=<chunk-id>.parquet` に置く。control bucket の設計契約は変更していない
* **chunk identity**: 入力 Observation ID の sorted list の SHA-256。listing 順や retry 回数に依存しない
* **duplicate protection の二重化**: iceberg-go の検出に加え、commit 前に現 snapshot の参照 file を照合し、全 file 既参照なら新 snapshot を作らず成功扱いにする。materialization.md の「commit と checkpoint の間の crash では Catalog を再読込し既参照 file を成功済みとする」規則の直接実装である
* **R2 SQL response の parse**: R2 SQL HTTP API の response body 形式は公式 doc に記載がないため、複数の形状を防御的に受理し、生 response の先頭を report に残す
* **R2 SQL の比較範囲**: timestamp の JSON 表現が未文書化のため、R2 SQL 経由の比較は message_id / observation_id / content の identity set と COUNT に限定し、timestamp を含む完全一致は iceberg-go scan 側で検証する

## R2 Real Environment の結果

未実施。実行には次の入力が必要である。

* Cloudflare account ID
* R2 API token（Admin Read & Write、R2 Data Catalog と R2 SQL の permission を含む）と、同 token の S3 Access Key ID / Secret Access Key
* Observation Archive 用 dev bucket と、catalog を有効化した Canonical 用 dev bucket の名前、および catalog URI / warehouse

実行手順と環境変数は `cmd/dwh-spike/main.go` の package comment と `scripts/spike.sh` に記載する。実行後、本節を Success Criterion ごとの PASS / FAIL、R2 SQL の生 response 形状、vended credential の実際の挙動、観測した beta 制約で更新する。

## 事前に把握している R2 側の制約（公式 doc 由来、未実測）

* R2 Data Catalog / R2 SQL は public / open beta
* R2 SQL の query 結果は既定で 500 row に制限される（fixture は 85 row で収まる）
* R2 SQL 用 token は R2 SQL Read、R2 Data Catalog、R2 Storage の permission を要する
* R2 Data Catalog は非 default jurisdiction の bucket を未サポート
* managed compaction は Parquet のみ対象、target size 64〜512 MB

## 判定

Local reference により、iceberg-go + 標準 Iceberg REST Catalog + Parquet/Zstandard の閉ループは技術的に成立した。

#36 の Close 判定は R2 real environment で同じ loop が PASS した時点とする。R2 でのみ失敗する項目が出た場合は、本書に最小再現と分類を記録し、Failure Rule に従って #32 へ戻す。
