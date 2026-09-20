# Cloudflare R2

Observation Archive と Canonical Store の storage 基盤として利用する R2 resource を扱います。

## 想定用途

Observation Archive の durable object storage を R2 に配置します。

Canonical Store についても R2 上の Iceberg table を利用する方向で検討します。

## 設計時に確定する事項

- Bucket topology
- Environment 分離
- Object layout
- Lifecycle rule
- Batching による object size
- Deletion workflow
- Binding
- Operation quota
- Storage capacity

Observation の意味や Canonical schema は Domain Design に置きます。
