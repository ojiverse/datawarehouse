# Infrastructure Design

このディレクトリでは、Architecture Design を実際の infrastructure resource として構成する方法を扱います。

## Scope

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

## Platform

現在の主要 infrastructure は Cloudflare です。

Cloudflare 固有の Infrastructure Design は [cloudflare/README.md](cloudflare/README.md) 以下に配置します。

Infrastructure Design は platform-specific であることを許容します。

将来 platform を変更する場合、Domain Design を維持したまま Architecture Design と Infrastructure Design を置き換えられる構造を目指します。
