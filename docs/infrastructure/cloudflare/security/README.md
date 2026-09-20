# Cloudflare Security

Discord DWH の Cloudflare resource に関する credential、permission、secret の設計を扱います。

## 対象

- Discord Bot token
- Cloudflare binding
- Service credential
- R2 access
- Data Catalog access
- Deployment credential
- Environment separation

## 原則

Secret を設計文書へ記載しません。

Application に不要な resource permission を付与しない最小権限を基本とします。

Public repository であることを前提に、repository 内の情報だけで credential や秘密情報を復元できない状態を維持します。

## 今後の設計

実際の resource topology が確定した段階で permission matrix と secret lifecycle を分割して設計します。
