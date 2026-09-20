# Observations Domain

Observation Archive に保存する「Discord から実際に観測した事実」の意味を定義します。

## Observation の役割

Observation は Canonical Data の前段にある source of evidence です。

Gateway event と HTTP API response のどちらも Observation になり得ます。

HTTP Backfill で取得した Message を、観測していない Gateway の MESSAGE_CREATE として扱ってはいけません。

取得経路の違いを provenance として保持します。

## 主な概念

Observation には少なくとも次の意味情報が必要です。

- 一意に識別できること
- 何を取得元としたか
- いつ観測したか
- Discord 上のどの対象に関係するか
- 元の Discord payload を保持できること
- Gateway 由来の場合に session と sequence を追跡できること
- Backfill 由来の場合に backfill run を追跡できること

具体的な保存形式や object layout は Domain Design では決めません。

## 不変条件

Observation Archive は通常運用では append-oriented に扱います。

同じ情報が複数回観測されることを許容します。

Retry、Resume replay、pagination overlap などによる duplicate が発生しても、観測を欠損させるより duplicate を許容する方を優先します。

Canonical Data は Observation の provenance を追跡できる必要があります。

## 今後分割する詳細設計

- Observation Model
- Provenance
- Observation Identity
- Duplicate Semantics
- Completeness Model
