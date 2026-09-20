# Query Domain

Canonical Data を利用して Discord の状態や履歴を問い合わせる際の意味論を扱います。

## Scope

- Current State projection
- Historical State
- Message history
- Channel や Thread 単位の分析
- Aggregate query に必要な意味定義

具体的な query engine や SQL dialect は Domain Design では扱いません。

## Current State

Current State は mutable database 上の唯一の行として保存されていることを前提としません。

複数の Canonical Observation から、対象時点で最も妥当な状態を導出する projection として定義します。

## Provenance

分析結果から、必要に応じて元の Canonical Data と Observation まで追跡できることを重視します。

## 今後分割する詳細設計

- Current State Projection
- Historical Projection
- Query Semantics
