# アーキテクチャ意思決定記録

このディレクトリには、重要な設計判断とその理由を ADR として保存します。

## 役割

通常の設計文書は「現在どのような設計か」を説明します。

ADR は「どの選択肢から、なぜその判断をしたか」を保存します。

一度採用した ADR を後から書き換えて歴史を消すのではなく、判断が変わった場合は新しい ADR で supersede します。

## 初期 ADR 候補

今後、少なくとも次の判断を ADR として切り出すことを検討します。

- Observation Archive と Canonical Store の2層構造
- Observation Archive を source of evidence とする判断
- Core DWH で D1 を必須にしない判断
- HTTP Backfill を anti-entropy mechanism とする判断
- Cloudflare を primary platform とする判断
- Discord Gateway runtime に Durable Objects を利用する判断

ADR 自体も 200 行以内とします。
