# インフラストラクチャ設計

このディレクトリでは、アーキテクチャ設計を実際のインフラストラクチャリソースとして構成する方法を扱います。

## 対象

- Resource topology
- Environment
- Binding
- Secret
- Permission
- Resource naming
- Deployment
- Observability configuration
- Quota
- Cost
- IaC

## プラットフォーム

現在の主要インフラストラクチャは Cloudflare です。

Cloudflare 固有のインフラストラクチャ設計は [cloudflare/README.md](cloudflare/README.md) 以下に配置します。

インフラストラクチャ設計はプラットフォーム固有であることを許容します。

将来プラットフォームを変更する場合、ドメイン設計を維持したままアーキテクチャ設計とインフラストラクチャ設計を置き換えられる構造を目指します。
