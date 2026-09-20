# Data Lifecycle Domain

Discord DWH を5〜10年運用する前提で、データの保持、削除、再構築などの lifecycle を扱います。

## Scope

- Observation の retention
- Canonical Data の retention
- Discord 上の削除との関係
- 削除要求への対応
- Canonical rebuild
- Schema migration 時の lifecycle

## 基本原則

「append-oriented」と「永久に削除しない」は同義ではありません。

通常の ingestion では既存 Observation を更新しないことを基本としつつ、削除要件や規約上の要求がある場合に対象データを追跡して削除できる必要があります。

Canonical Data が削除対象となった場合、対応する Observation まで provenance を辿れる状態を維持します。

## 長期運用

すべての runtime や infrastructure が5〜10年間同じ形で存在することは前提にしません。

長寿命であるべきものは、Discord DWH としての意味と、再構築に必要な source evidence です。

## 今後分割する詳細設計

- Retention
- Deletion
- Long-term Migration
- Rebuild Lifecycle
