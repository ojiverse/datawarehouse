# Backfill ドメイン設計

Discord HTTP API を利用して Observation を取得し、Gateway だけでは得られない既存データや欠損を補う仕組みを扱います。

## 用途

Backfill の実装は、可能な限り次の用途で共通化します。

- 初回導入時の既存ログ取得
- Gateway Resume が成立しなかった区間の回復
- Gateway や collector の不具合による欠損修復
- 定期的な reconciliation
- 手動 repair

## 基本原則

Backfill は Gateway event history を復元するものではありません。

HTTP API から取得できるのは、原則として取得時点で Discord 上に残っている state です。

Gateway 障害中に作成され、その後削除された Message や、複数回編集された Message の中間状態などは完全には復元できません。

そのため、Gateway ingestion と HTTP Backfill では completeness guarantee を区別します。

## Anti-entropy

HTTP API は Gateway の単なる非常用経路ではなく、Canonical completeness を継続的に確認する anti-entropy mechanism として扱います。

定期的に最近の一定範囲を再取得し、見逃していた欠損を eventual に発見できる設計を目指します。

## 今後分割する詳細設計

- Message History Crawling
- Thread Discovery
- Gap Recovery
- Periodic Reconciliation
- Pagination and Progress Semantics
- Discord Rate Limit Semantics
