# Cloudflare 環境

開発から安定稼働までの Cloudflare environment 分離を扱います。

## 想定フェーズ

### 開発

HTTP Backfill を中心に、Observation Archive と Canonical Store への保存が正しく動作することを検証します。

Gateway の常時接続は必須としません。

### ベータ

Gateway を短期間、断続的に接続します。

Gateway からの保存、切断、再接続、Resume、Backfill による回復を検証します。

### 本番

Workers Paid を利用し、Gateway を継続運用します。

年内の安定稼働を目標とします。

## 未決定事項

- Environment ごとの resource 分離単位
- Bucket や Queue を共有するか
- Secret の分離方法
- Beta から Production への data migration
