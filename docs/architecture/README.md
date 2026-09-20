# Architecture Design

このディレクトリでは Domain Design を特定 platform の能力と制約の下でどう実現するかを扱います。

## 役割

Domain Design は Discord DWH としての意味や保証を定義します。

Architecture Design は、それらを実際に成立させるために application component をどう構成し、platform capability をどう利用するかを定義します。

Infrastructure resource の具体的な名前、binding、environment、権限、IaC などは Infrastructure Design に分離します。

## Platform

現在の主要 platform は Cloudflare です。

Cloudflare 上での Architecture Design は [cloudflare/README.md](cloudflare/README.md) 以下に配置します。

将来別 platform を採用する場合は、同じ階層に別 platform の directory を追加できます。

## Domain との関係

Infrastructure の制約が Domain requirement に影響する場合があります。

その影響自体を禁止しません。

ただし Architecture Design では、Domain requirement と platform-specific realization の対応関係を明確にします。
