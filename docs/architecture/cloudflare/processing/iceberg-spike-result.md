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

## Archive 入力の contract 適合（ADR-0015）

Archive 入力の authoritative physical contract は `contracts/observation-envelope/v1/schema.json`（JSON Schema Draft 2020-12）であり、`http-backfill-page.json` が compatibility fixture である（`docs/architecture/data-contracts.md`）。Go の Envelope 型は schema から導出した implementation artifact として、top-level `provenance`、`provenance.endpoint`、required かつ全 member が nullable な `provenance.rate_limit`、null を許す `pagination.before / after` を持つ。R2 object の custom metadata は `format` / `compression` / `envelope_version` / `observation_id` / `source_kind` / `payload_sha256` とした。

compatibility proof は `internal/observation` の test で次を検証する。

* shared fixture が JSON Schema に適合する（format assertion を有効にした validator を使用）
* Go decoder が fixture を decode できる
* decode → encode 後の JSON が semantic に等価である（key sort と空白除去後の canonical JSON が一致）。gzip round-trip でも Envelope が変化しない
* decode → encode 後の JSON が再び schema に適合する
* spike の fixture generator の出力が schema に適合し、shared fixture と同じ key 構成を持つ
* validator が壊れた document（rate_limit 欠落）を拒否する

時刻は producer と同じミリ秒固定精度で出力する型を使う。契約上は semantic equivalence で足りるが、Go 標準の time 型が末尾の 0 を落として `.250Z` を `.25Z` にする挙動は文字列比較で差分になるため、固定精度に揃えた。

閉ループは生成 fixture 4 page に shared fixture 1 page を加えた 5 object を Archive 入力とする。

## 検証環境の二段構成

Failure Rule 第 2 項（Iceberg REST / Parquet boundary 自体の問題か iceberg-go 固有の問題かの切り分け）に備え、同一コードを二つの catalog に対して実行する。

1. **Local reference**: MinIO + apache/iceberg-rest-fixture（`spike/local/docker-compose.yml`）。R2 に依存しない標準 REST Catalog での基準結果を得る
2. **R2 real environment**: R2 Observation Archive bucket、R2 Data Catalog、R2 SQL

R2 でのみ失敗する項目は R2 固有、両方で失敗する項目は iceberg-go / protocol 側の問題として分類する。

## Local Reference の結果（2026-09-22 再実測、shared fixture 込み）

apache/iceberg-rest-fixture + MinIO（image は digest で pin）に対して閉ループは全項目 PASS した（R2 SQL 項目は環境上 skip）。

| Success Criterion | 結果 | 根拠 |
| :--- | :--- | :--- |
| Envelope v1 fixture を Archive へ保存 | PASS | 生成 4 page + shared fixture 1 page の 5 object を create-only put で保存。同一 object の再 put は Observation ID と payload hash の一致により idempotent success |
| fixture から Canonical Message Parquet を生成 | PASS | 5 page（overlap あり）から 87 row を projection。全 column chunk の codec が ZSTD であることを Parquet metadata から確認 |
| iceberg-go から REST Catalog に接続 | PASS | rest.Catalog で接続 |
| namespace / table を作成 | PASS | `dwh_spike.message` を固定 schema で作成 |
| Parquet data file を Iceberg table へ commit | PASS | table FileIO 経由で staging file を書き、AddFiles で snapshot を作成 |
| commit retry で同じ data file を二重登録しない | PASS | catalog 再読込後の retry は already_referenced として snapshot を増やさない。さらに iceberg-go の AddFiles 自体が既参照 file を拒否することを確認 |
| R2 SQL から query | SKIP | local には R2 SQL が存在しない。代替として iceberg-go scan が projection と一致することを確認 |
| Canonical table を削除 | PASS | purge 後に CheckTableExists = false |
| Discord API へアクセスせず再生成 | PASS | Discord credential 環境変数が未設定であることを assert した上で Archive listing のみから再 materialize |
| 再生成前後の domain data が一致 | PASS | 正規化 serialization の SHA-256 digest が一致（87 row）。chunk identity も一致 |
| runtime constraint の記録 | PASS | 後述 |
| beta 制約の記録 | N/A | R2 real environment で実測する（後節） |

Local 実測の resource は次の通り（macOS arm64、Go 1.26.3、初回 2026-09-21 の 85 row 実行時。再実測でも同水準）。

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

## R2 Real Environment の準備状況（2026-09-21）

OJIverse account（`8df65b32589ad7acc6d3d257d5dd2d04`）に dev 用 resource を Cloudflare 公式 MCP 経由で作成した。本番 data を持つ bucket には触れていない。

* Observation Archive: bucket `ojiverse-dwh-dev-observations`（APAC、default jurisdiction）
* Canonical Store: bucket `ojiverse-dwh-dev-canonical`（APAC、default jurisdiction）。R2 Data Catalog を有効化済み
* Warehouse: `8df65b32589ad7acc6d3d257d5dd2d04_ojiverse-dwh-dev-canonical`
* Catalog URI: `https://catalog.cloudflarestorage.com/8df65b32589ad7acc6d3d257d5dd2d04/ojiverse-dwh-dev-canonical`（未認証 GET に対して 401 "Missing Authorization header" を返すことを確認）
* Catalog の既定 maintenance: compaction enabled（target 128 MB、interval 1h）、snapshot expiration disabled。catalog 側の service credential は未登録（`credential_status: absent`）。spike の commit 検証には不要だが、managed compaction を実際に動かすには dashboard から登録が必要

### 実行手順

接続情報と secret は 1Password（ojilab account、vault `ojiverse-datawarehouse-dev`、item `cloudflare-r2-dwh-spike`）で管理し、`spike/r2-dev.op.env` に `op://` 参照だけを置く。実行は次のコマンドで行い、secret を shell history や repository に残さない。

`op run --account ojilab --env-file spike/r2-dev.op.env -- scripts/spike.sh loop`

環境変数の意味は `cmd/dwh-spike/main.go` の package comment に記載する。後片付けは同じ形で `scripts/spike.sh cleanup` を実行する。

## R2 Real Environment の結果（2026-09-21 初回、2026-09-22 shared fixture 込みで再実測）

R2 API token（Admin Read & Write、TTL 1 週間）を 1Password 経由で注入し、上記コマンドで閉ループを完走した（両日とも exit 0、retry なし）。以下は shared contract fixture を含む 2026-09-22 の結果である。R2 でのみ失敗した項目はなく、Failure Rule の発動は不要である。

| Success Criterion | 結果 | 根拠 |
| :--- | :--- | :--- |
| Envelope v1 fixture を R2 Observation Archive へ保存 | PASS | S3 API の `If-None-Match: *` create-only put で 5 object（shared fixture 1 件を含む）を保存。再 put は 412 を受けて metadata 照合により idempotent success |
| fixture から Canonical Message Parquet を生成 | PASS | R2 listing のみを入力に 87 row、ZSTD。shared fixture の 2 Message（うち 1 件は edited）も projection に含まれる |
| iceberg-go から R2 Data Catalog に接続 | PASS | bearer token のみで REST Catalog に接続 |
| namespace / table を作成 | PASS | `dwh_spike.message`。table location は catalog 管理の `s3://<bucket>/__r2_data_catalog/<catalog uuid>/<table uuid>` |
| Parquet data file を Iceberg table へ commit | PASS | vended credential で staging file を書き、AddFiles で snapshot 作成 |
| commit retry で同じ data file を二重登録しない | PASS | 再読込後 retry は already_referenced、iceberg-go の AddFiles も拒否、snapshot 不変 |
| R2 SQL から query | PASS | 87 row が projection の identity set と一致。COUNT(*) = 87。commit 直後の初回 query で整合し propagation retry は不要だった（両日とも） |
| Canonical table を削除 | PASS | PurgeTable が受理され CheckTableExists = false |
| Discord API へアクセスせず再生成 | PASS | Discord 環境変数未設定を assert し Archive listing のみから再 materialize |
| 再生成前後の domain data が一致 | PASS | digest 一致（87 row）、chunk identity 一致。rebuild 後の R2 SQL も 87 row 一致 |
| runtime constraint の記録 | PASS | 後述 |
| beta 制約の記録 | PASS | 後述 |

### vended credential の実挙動

catalog property に S3 key を一切渡さず（`DWH_CATALOG_PROPS` 未設定）、bearer token だけで table の data file の書き込み・読み出しが成功した。iceberg-go が `X-Iceberg-Access-Delegation: vended-credentials` を送り、R2 Data Catalog が返す storage credential で FileIO を構成する経路が機能している。したがって materializer の credential は R2 API token 1 つで足り、S3 key pair は Observation Archive bucket へのアクセスにのみ必要である。

### R2 SQL の response 形状（実測）

Cloudflare v4 wrapper（success / errors / messages）の `result` 配下に `request_id`、`schema`（column ごとの name と型 descriptor、nullable）、`rows`（column 名を key とする object の配列）、`metrics`（r2_requests_count、files_scanned、bytes_scanned、cache_hits）を持つ。string / int64 は JSON の文字列 / 数値として返る。timestamp 型の表現は本 spike では比較対象にしていない。

### 実測 resource / latency（macOS arm64 → R2 APAC、2 回の実行の範囲）

| 指標 | 値 |
| :--- | :--- |
| user / system CPU | 約 300〜330 ms / 100〜130 ms |
| max RSS | 約 118 MB |
| Archive put（4〜5 object + retry） | 約 1.3〜1.5 s |
| catalog namespace + table 作成 | 約 3.9〜7.5 s |
| staging 書き込み + commit | 約 2.4〜5.6 s |
| iceberg-go scan | 約 0.6〜0.8 s |
| R2 SQL（SELECT + COUNT、1 criterion 分） | 約 6〜26 s。cold な初回 SELECT が 約 10〜20 s、warm な query は 約 2.5 s |
| drop（purge） | 約 0.6〜1.6 s |

閉ループ全体は約 60〜65 s で、そのうち R2 SQL の cold query が支配的である。実行ごとのばらつきは R2 SQL 側が大きく、Go process 側は安定している。

### 観測した beta 制約

* R2 SQL の cold query は約 10〜20 s かかり、以降は数秒に落ちる（cache_hits が増える）。table を作り直す rebuild 直後は再び cold になる。interactive 用途では warm-up を前提にする
* R2 SQL の metrics は data file 1 つの table でも files_scanned 5 を返す（metadata / manifest を含む）
* table location は catalog が `__r2_data_catalog/` 配下の UUID path で決めるため、application 側で物理 path を契約にしない設計（r2/README.md）は正しかった
* R2 SQL 結果の既定上限は 500 row。大きな検証は LIMIT / 集計で行う
* catalog の managed compaction は既定で enabled（128 MB、1h）だが service credential 未登録のため実行されない。有効化すると staging file が rewrite され、data file path による重複判定の前提が変わる点は #40 で扱う

## 判定

Local reference と R2 real environment の双方で、iceberg-go + Iceberg REST Catalog（R2 Data Catalog）+ Parquet/Zstandard + R2 SQL の閉ループが成立した。#36 の Definition of Done を満たしており、#40 は iceberg-go を first-MVP materializer implementation として進めてよい。
