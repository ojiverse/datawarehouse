# Cloudflare 取り込みアーキテクチャ

Discord Gateway のドメイン要件を Cloudflare runtime 上で実現する方法を扱います。

## 対象

- Gateway connection の所有単位
- Gateway session state の保持
- Heartbeat の runtime 上の維持
- Runtime restart 後の Resume
- Dispatch event の downstream への引き渡し
- Downstream failure 時の振る舞い
- Backpressure

## 現時点の方向性

Gateway session は単一の active owner を持ち、runtime restart を跨いで Resume に必要な state を回復できる構成を検討します。

Cloudflare Durable Objects はこの capability の主要候補です。

ただし、具体的な state machine、durable boundary、sequence の確定タイミングなどは今後の詳細設計で決定します。

## 対象外

- Durable Object namespace の具体名
- Worker binding
- environment ごとの instance 構成
- cost budget

これらはインフラストラクチャ設計で扱います。

## 今後分割する詳細設計

- Gateway Runtime Ownership
- Session Persistence
- Heartbeat Runtime
- Resume Runtime
- Event Handoff
- Downstream Failure
- Backpressure
