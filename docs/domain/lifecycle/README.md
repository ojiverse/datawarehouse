# データライフサイクルドメイン（Lifecycle）

本ディレクトリでは、Discord DWH の長期稼働（5〜10年）を前提とした、データ保持期間（Retention）、利用規約に基づく削除要求への対応、およびデータ再構築のライフサイクルを定義する。

## 保持方針と削除の不変条件

保持・削除の Product Policy は [product-policy/](../product-policy/README.md) を基準とする。

### 原則無期限の保持
DWH-public として取得した API Data は、stated functionality のために必要であり削除義務が発生していない限り、期限を設けず保持する。

ここでの無期限保持は「いかなる理由でも永久に削除しない」という意味ではない。Product Policy、Discord の要求、ユーザー本人からの削除要求、法的義務等による deletion requirement は通常の Retention より優先する。

### 「追記専用（Append-only）」と削除義務の峻別
本システムにおいて「追記専用（Append-only）」であることは、「いかなるデータも物理削除しない」ことを意味しない。

通常の Discord Message 削除は Archive の物理削除 trigger としない。削除を観測できた場合は「過去に存在し、その後削除された」という historical evidence として Observation Archive に保持し、Canonical の best-known Current State に反映する。

一方、ユーザー本人、Discord、法的要請その他によって API Data の削除義務が発生した場合は、対象ユーザーに紐づく全 API Data を削除対象とする。

### 削除追跡性（Deletion Provenance）
Deletion requirement を満たすため、対象ユーザーに紐づく API Data を Observation Archive まで遡って特定できなければならない。

削除対象は Observation Archive と Canonical Store に限らず、Vectorize、全文検索 index、cache、Agent 固有 index 等の Derived Data を含む。

物理 object の書き換え、subject index、暗号化消去等の具体方式は Architecture / Infrastructure で決定する。

## 長期運用における不変の価値

クラウドベンダーの提供サービス、Worker ランタイム、ストレージ API は、長期運用の過程において仕様変更または廃止（Deprecation）される可能性がある。
したがって、特定の実行基盤やランタイムが不変の形で存続することを前提としてはならない。

長期的に維持すべき核心的資産は以下の2点に限定される。

1. **Discord DWH としての概念定義とドメインモデル**（データの意味論）
2. **再構築を可能にする Observation Archive**（生の事実証跡としてのデータそのもの）

これらがプラットフォーム固有の実装から独立して保たれている限り、背後の基盤インフラの進化や移行にかかわらず、DWH としての継続的運用が可能となる。

## 分割予定の詳細設計

* **Retention Policy**: Observation Archive および Canonical Store の保存期間とアーカイブ階層化
* **Deletion & Compliance**: Discord の削除イベント追跡、利用規約準拠のデータ抹消ワークフロー
* **Long-term Migration**: 世代交代に伴うストレージ移行やエンベロープ形式のバージョン管理
* **Rebuild Lifecycle**: 大規模再構築時における旧テーブルの縮退と新テーブルへの切り替え手順
