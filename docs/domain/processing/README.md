# 処理ドメイン

Observation Archive の Observation を Canonical Data へ変換する処理の意味論を扱います。

## 対象

- Observation の解釈
- Canonical entity への normalization
- Duplicate の扱い
- Event ordering の扱い
- Gateway event と HTTP snapshot の統合
- Replay
- Canonical Store の rebuild

## 基本原則

Processing の実装が失敗しても Observation を失わないことを前提とします。

Canonicalizer の不具合は、Observation Archive から修正版の処理を再実行することで回復可能であることを目指します。

HTTP snapshot を架空の Gateway event に変換しません。

取得経路と観測時点の情報を Canonical Data から追跡できるようにします。

## 再処理

5〜10年の運用では Canonical schema や transformation rule の変更が発生することを前提とします。

そのため再処理は例外的な migration ではなく、通常の system capability として設計します。

## 今後分割する詳細設計

- Observation Normalization
- Deduplication
- Ordering
- Gateway and Snapshot Reconciliation
- Replay
- Canonical Rebuild
