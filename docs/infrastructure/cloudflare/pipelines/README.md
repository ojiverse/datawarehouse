# Cloudflare Pipelines

Observation から Canonical Store への processing で Cloudflare Pipelines を利用する場合の resource 設計を扱います。

## 現在の位置付け

Pipelines は Canonical materialization の候補です。

Observation Archive の durability 自体を Pipelines に依存させない方針です。

Pipelines を採用しなくても Domain Design が成立する状態を維持します。

## 設計時に確定する事項

- Pipeline topology
- Source
- Transform
- Sink
- Environment 分離
- Failure handling
- Quota
- Cost

採用可否は processing の詳細 Architecture Design と合わせて判断します。
