# Cloudflare コストとキャパシティ

Cloudflare の plan、quota、capacity、cost を扱います。

## 現在の方針

開発とベータは可能な範囲で Free tier を利用します。

本番では Workers Paid を利用し、Gateway を継続運用する方針です。

Gateway 用 Durable Object 1 instance の常時稼働について、Workers Paid の included duration 内で運用できる可能性を確認しています。

ただし最終判断はベータの実測値を用いて行います。

## 継続的に確認する対象

- Workers request と CPU
- Durable Objects duration
- Durable Objects storage operation
- Queue operation
- R2 storage
- R2 Class A と Class B operation
- R2 SQL scan
- Pipelines usage

## 原則

Cost 最適化のために durability や recoverability を暗黙に弱めません。

設計上の保証を変更する場合はアーキテクチャ設計と ADR に反映します。
