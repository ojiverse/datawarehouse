# Cloudflare Deployment

Cloudflare resource と application の deployment、upgrade、migration を扱います。

## Scope

- Environment ごとの deployment
- Worker deployment
- Durable Object migration
- Queue や R2 の変更
- Data Catalog migration
- Rollback
- Breaking change の展開

## 原則

Gateway の deployment が長時間のデータ欠損を生まないことを目指します。

Deployment 中に Gateway ingestion が途切れる可能性がある場合、Resume と HTTP Backfill によって回復できる設計と組み合わせます。

## 今後の設計

Resource topology と CI/CD 方針が確定した段階で詳細を分割します。
