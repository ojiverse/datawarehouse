# Cloudflare インフラストラクチャ

Discord DWH を運用するために必要な Cloudflare resource と環境構成を扱います。

## サブシステム

- [environments/README.md](environments/README.md): development、beta、production
- [workers/README.md](workers/README.md): Worker resource と責務
- [durable-objects/README.md](durable-objects/README.md): Durable Object namespace と instance topology
- [queues/README.md](queues/README.md): Queue topology
- [r2/README.md](r2/README.md): R2 bucket と lifecycle
- [data-catalog/README.md](data-catalog/README.md): R2 Data Catalog
- [pipelines/README.md](pipelines/README.md): Cloudflare Pipelines
- [security/README.md](security/README.md): Secret、permission、credential
- [observability/README.md](observability/README.md): Log、metric、alert
- [deployment/README.md](deployment/README.md): Deployment と migration
- [cost/README.md](cost/README.md): Plan、quota、capacity、cost

## 現在の方針

開発とベータでは可能な範囲を Free tier で検証します。

Gateway を安定して継続運用する本番フェーズでは Workers Paid を利用する方針です。

具体的な resource 数、命名、binding、quota budget は詳細設計で確定します。
