# Observation Envelope v1

本書では Observation Archive に保存する Observation Envelope v1 の論理契約を定義する。

物理的な JSON encoding、compression、R2 key layout は Architecture / Infrastructure で定義する。

Go / TypeScript 間で共有する physical JSON field contract は `contracts/observation-envelope/v1/schema.json` を Source of Truth とする。Domain 文書は field の意味論を定義し、JSON Schema は field name、required / nullable、shape を定義する。

## 共通 Envelope

すべての Observation は、少なくとも以下の意味を保持する。

* **Envelope Version**: 当該 Observation が従う契約 version。初期 version は v1
* **Observation ID**: 一回の観測を識別する UUIDv7
* **Source Kind**: gateway、http_backfill、http_reconciliation のいずれか
* **Observed At**: producer が source payload 全体を受信し、Observation として扱える状態になった時刻
* **Payload**: Discord から得た source payload の全 field
* **Provenance**: Source Kind 固有の取得経路情報

Accepted time、Archive commit time、processing time 等の運用時刻を追加してよいが、Observed At と混同してはならない。

Observed At は producer の clock に基づく provenance であり、複数 producer 間の authoritative event order には使用しない。

## HTTP Observation

HTTP Backfill / Reconciliation では **1 HTTP response page を1 Observation** とする。

Payload には response body 全体を保存する。Message entity 単位へ分解するのは Canonical processing の責務である。

HTTP provenance には少なくとも以下を保持する。

* Backfill / Reconciliation の run identity
* Discord API version
* request 対象の Guild / Channel 等の resource identity
* endpoint / operation の種類
* pagination parameter と値
* request limit
* request 開始時刻
* response を受信完了した時刻
* HTTP status
* Message Content 等、取得結果の completeness に影響する application capability state

Authorization header、Bot Token、Cookie、Cloudflare credential 等の secret を provenance に保存してはならない。

Rate-limit header は運用診断のため保持してよいが、HTTP Observation の identity や completeness の根拠にはしない。

## Gateway Observation

Gateway では **1 Discord Dispatch frame を1 Observation** とする。

Payload は data field だけではなく、Discord から受信した Dispatch frame 全体を保持する。少なくとも opcode、event name、sequence、data を失ってはならない。

Gateway provenance には少なくとも以下を保持する。

* Gateway Instance identity
* Discord Gateway Session identity
* shard assignment
* sequence
* 当該 Session の Gateway Intents / capability state

Discord protocol の control frame は Dispatch Observation と同一視しない。保存対象とする場合は source semantics を明示して別種の evidence として扱う。

## Completeness

Envelope v1 は「Discord の完全な状態」を表現しない。

Gateway と HTTP で観測可能な情報が異なること、および Observation が欠損し得ることを前提とする。

後続処理は provenance を無視して Gateway event と HTTP snapshot を同一の event history として解釈してはならない。

## Versioning

Envelope Version は各 Observation 自体から判別可能でなければならない。

新 field の追加だけで既存 semantics を壊さない場合は v1 の後方互換拡張として扱える。

既存 field の意味、identity、payload boundary、provenance semantics を変更する場合は新しい envelope version とする。

HTTP 専用の構造変更を理由に、将来 Gateway を追加しただけで version を上げる必要がないよう、共通 envelope と source-specific provenance の境界を維持する。
