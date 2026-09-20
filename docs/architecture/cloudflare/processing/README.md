# Cloudflare Processing Architecture

Observation から Canonical Data を生成する processing を Cloudflare 上でどう実行するかを扱います。

## Scope

- Normalization の execution model
- Batch と streaming の境界
- Replay
- Rebuild
- Failure isolation
- Canonical Store への write

## 現時点の方向性

Observation Archive への保存を processing の成功に依存させません。

Canonical processing が停止しても新しい Observation を蓄積し続けられ、後から replay できる構成を目指します。

Cloudflare Pipelines は候補ですが、Observation Archive 自体の durability を Pipelines 固有機能に依存させません。

## 今後分割する詳細設計

- Normalization Pipeline
- Replay
- Rebuild
- Processing Failure Isolation
