# Cloudflare Operations Architecture

Cloudflare 上で Discord DWH を安定運用するために application architecture が備えるべき運用 capability を扱います。

## Scope

- Gateway liveness の検知
- Resume failure の検知
- Observation ingestion failure の検知
- Backfill failure の検知
- Canonical lag の検知
- Fault injection
- Production readiness の検証

具体的な dashboard、alert destination、log sink などは Infrastructure Design に分離します。

## 検証方針

開発段階では HTTP Backfill を中心に Observation と Canonical Data の保存を検証します。

ベータ段階では Gateway を断続的に接続し、Gateway ingestion、切断、Resume、Backfill recovery を検証します。

安定稼働へ移行する前に、長時間接続と意図的な failure を含む verification を実施します。

## 今後分割する詳細設計

- Failure Detection
- Gateway Verification
- Backfill Verification
- Data Completeness Verification
- Production Readiness
