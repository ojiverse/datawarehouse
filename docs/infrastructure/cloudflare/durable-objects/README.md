# Cloudflare Durable Objects

Discord Gateway session を Cloudflare 上で所有するための Durable Object infrastructure を扱います。

## 現在の方向性

1 Gateway session に対して単一の active owner を提供し、session state と Resume に必要な状態を保持する用途を主要候補とします。

1 shard で開始する前提では、常時 active な Gateway 用 Durable Object が Workers Paid の included usage に収まる可能性を確認しています。

最終的な cost と quota は実測を踏まえて確定します。

## 設計時に確定する事項

- Namespace
- Instance key
- Shard との対応
- Storage usage
- Alarm usage
- Migration
- Environment 分離
- Resource monitoring

Gateway protocol 上の state machine は Domain Design、Durable Object 上での動作は Architecture Design で扱います。
