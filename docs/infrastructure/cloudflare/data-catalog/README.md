# R2 Data Catalog

Canonical Store を Apache Iceberg table として管理するための R2 Data Catalog resource を扱います。

## 現在の方向性

Canonical Store は Observation Archive から再構築可能な analytical representation とします。

R2 Data Catalog を利用して Iceberg table を管理し、R2 SQL から問い合わせる構成を主要候補とします。

## 設計時に確定する事項

- Catalog topology
- Environment 分離
- Table namespace
- Access control
- Schema migration
- Operational limits

Canonical Data Model そのものは Domain Design で扱います。
