# HTTP Message Deterministic Projection

本書では HTTP Observation から Canonical Message の best-known state を deterministic に導出する規則を定義する。

Gateway を含む cross-session ordering は対象外とする。

## 基本モデル

HTTP response page に含まれる各 Message は、その Observation 時点で Discord 上に残存していた Message snapshot として解釈する。

同じ Message ID が複数 Observation に含まれることを正常状態とする。

HTTP page に Message が存在しないことだけから、その Message が削除されたとは推論しない。

## Source-local Duplicate

同じ Observation ID が複数回 processing された場合は、同一 evidence の再処理として扱い、Canonical 上で二重化しない。

Observation ID が異なる HTTP 再取得は、それぞれ独立した snapshot evidence として保持する。

同一 payload であっても Observation ID が異なる場合、Archive 上の evidence を消去または統合しない。

## Message Snapshot Identity

Canonical の履歴表現では、Message ID と根拠 Observation ID の組により snapshot を一意に追跡できることを要求する。

Current State projection は Message ID ごとに1つの best-known snapshot を選択する。

## Best-known Snapshot の選択

同一 Message の複数 snapshot から Current State を選ぶ際は、以下の優先順位を使用する。

1. Discord が提供する Message の edited_timestamp が新しい snapshot
2. edited_timestamp が同値または双方未編集の場合は Observed At が新しい snapshot
3. それでも同値の場合は Observation ID の UUID 値による deterministic な順序

未編集 Message の creation time は Snowflake から導出できるが、snapshot の新旧判定で Observed At を置き換えない。

Observed At は HTTP snapshot 間の best-known recency にのみ使用し、Discord event の authoritative order として一般化しない。

## Mutable Fields

Reaction、pin、flags、その他 edited_timestamp の更新を伴わず変化し得る Message field があるため、edited_timestamp が同じ snapshot 間では新しい Observed At を優先する。

Canonical Current State の field を複数 Observation から field-by-field に合成することは first-MVP では行わない。選択された1つの Message snapshot を current representation の根拠とする。

これにより Canonical record の provenance は first-MVP では **採用された1 Observation ID** とする。

## 削除

HTTP Backfill だけでは、Message が取得結果から消えた理由を確定できない。

したがって HTTP Observation の不在だけから Message deletion を生成しない。

Gateway の MESSAGE_DELETE 等を導入した後に、削除 event を含む projection rule を拡張する。

## Determinism の要件

Projection result は以下に依存してはならない。

* Archive object の列挙順
* R2 key の物理配置
* Parquet row order
* materializer の並列度
* rebuild の実行回数

同一 Observation set と同一 projection version からは、常に同じ domain result を得なければならない。
