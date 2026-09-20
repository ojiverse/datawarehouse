# Cloudflare Canonical Store アーキテクチャ

Canonical Data を Cloudflare Data Platform 上で materialize し、分析可能にする方法を扱います。

## 現時点の方向性

R2 上の Apache Iceberg table を Canonical Store の主要候補とします。

R2 Data Catalog と R2 SQL を利用する構成を検討します。

Observation Archive を source of evidence とし、Canonical Store は再構築可能な analytical representation として扱います。

## 対象

- Canonical table の materialization path
- Iceberg を利用した長期的な schema evolution
- Query path
- Rebuild 時の扱い
- Observation への provenance

## 対象外

具体的な catalog 名、bucket、Pipeline 名、binding、quota はインフラストラクチャ設計で扱います。

## 今後分割する詳細設計

- Canonical Materialization
- Iceberg Mapping
- Query Runtime
- Rebuild Architecture
