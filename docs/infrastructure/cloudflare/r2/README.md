# Cloudflare R2 インフラ設計

本ディレクトリでは Observation Archive、Canonical Store、再構築 control state に利用する Cloudflare R2 の bucket topology と object contract を定義する。

## Bucket Topology

| 論理名 | 役割 | 形式 | 永続性 |
| :--- | :--- | :--- | :--- |
| **ojiverse-dwh-observations** | Source of Evidence | JSON + gzip | 原則無期限 |
| **ojiverse-dwh-canonical** | Apache Iceberg Canonical Store | Parquet + Iceberg metadata | Rebuildable |
| **ojiverse-dwh-control** | Rebuild manifest / checkpoint / staging control | JSON / manifest / staging Parquet | Disposable / Rebuildable |

環境ごとに bucket を分離し、Production data と Development / Beta data を同一 bucket に混在させない。

## Observation Archive Format

Observation Archive は **1 Observation = 1 R2 object** とする。

object body は Observation Envelope v1 の UTF-8 JSON を gzip 圧縮して保存する。

Archive に Parquet を採用しない。Raw evidence は schema evolution と source payload fidelity を優先し、分析最適化は Canonical Store の責務とする。

Workers runtime が標準で扱える gzip を使用し、独自 codec を必須にしない。

## Observation Object Key

Archive key は observed time の UTC 時間軸と source kind で prefix partition する。

基準 layout は `observations/v1/source=<source-kind>/year=<YYYY>/month=<MM>/day=<DD>/hour=<HH>/<observation-id>.json.gz` とする。

source kind は gateway、http_backfill、http_reconciliation のいずれかとする。

partition time には Discord entity time ではなく Observation の Observed At を使用する。

Guild ID を mandatory prefix にしない。READY、guild-less event、将来の非 Guild observation を同じ contract で保存できることを優先する。

Observation ID は object key から導出しない。key が ID を含むのは現在の物理 layout 上の選択にすぎない。

## Self-describing Metadata

各 Archive object は body 内の Envelope Version に加え、R2 metadata からも少なくとも format、compression、Envelope Version、Observation ID、Source Kind を識別できるようにする。

source payload の SHA-256 を metadata として保持し、同一 Observation ID の retry consistency check に利用する。

HTTP metadata では JSON content type と gzip content encoding を明示する。

## Write Semantics

Archive write は conditional create-only put とし、既存 key を overwrite しない。

同一 key が存在する場合は Observation ID と source payload hash を確認し、一致する retry だけを idempotent success と扱う。

不一致は invariant violation として失敗させる。

R2 の strong consistency を前提に、put 成功後は Archive commit 済みとして扱う。

## Canonical Store

Canonical bucket は R2 Data Catalog / Apache Iceberg が所有する。

data file は Parquet、compression は Zstandard とする。

Iceberg metadata / data file の物理 path を application contract にしない。Catalog と Iceberg engine が layout を管理する。

## Control Bucket

Control bucket は Rebuild Run ID、immutable input manifest、checkpoint、staging file 等を保持する。

Control data は Source of Evidence ではなく、削除しても Observation Archive から再作成可能でなければならない。

## Lifecycle

Observation Archive に automatic expiry を設定しない。

Product Policy に基づく deletion requirement は automatic retention rule ではなく明示的な deletion workflow で処理する。

Canonical / control data は rebuildability を維持した上で snapshot expiration、compaction、cleanup を適用できる。
