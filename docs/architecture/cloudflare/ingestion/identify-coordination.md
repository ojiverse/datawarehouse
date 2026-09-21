# Gateway Identify Coordination

本書では複数 Gateway Instance が共有する Discord application-wide Identify budget の coordination を定義する。

Gateway Identify Coordinator Durable Object と external lease endpoint は **TypeScript** で実装する。

## Single Budget Owner

Discord application ごとに1つの **Gateway Identify Coordinator Durable Object** を設ける。

Cloudflare 上の Gateway Session DO だけでなく、Raspberry Pi 等の外部 Gateway Instance も Identify 前に Coordinator から lease を取得しなければならない。

Cloudflare 内は Service Binding、外部 Instance は authenticated Worker endpoint を通じて同じ Coordinator へ到達する。

Resume は Identify budget を消費しないため lease 対象外とする。Resume 不能で新 Session を開始する場合だけ Identify lease を要求する。

## Discord Authority

Coordinator は Get Gateway Bot の session start limit を Discord 側の authority とする。

少なくとも total、remaining、reset_after、max_concurrency を durable state として保持し、起動時、reset 後、budget が少ない場合、Discord から Invalid Session / limit error を受けた場合に再取得する。

固定値だけを長期 contract としない。

## Concurrency Bucket

Identify lease は shard ID と Discord が返す max_concurrency から rate-limit bucket を計算する。

同一 bucket に属する Identify を5秒 window 内で同時に許可しない。

同一 shard assignment の複数 Session も同じ application budget を共有するため、Gateway Instance が異なっても coordinator を迂回してはならない。

## Daily Budget

Coordinator は Discord が返す remaining / reset_after を基準に新しい Identify を許可する。

remaining が0の間は新規 Identify を禁止し、reset 後まで待機する。

local lease count と Discord response が矛盾した場合は安全側へ倒し、新規 Identify を停止して Get Gateway Bot を再取得する。

budget exhaustion を reconnect loop で悪化させてはならない。

## Lease Semantics

lease request は Gateway Instance identity、shard assignment、request identity を含む。

同じ request identity の retry は新しい budget consumption として数えない。

lease は短い expiration を持ち、Identify を送信しなかった lease は expiration 後に再評価できる。

Gateway Instance は lease を得る前に Identify payload を Discord へ送ってはならない。

## Observability

少なくとも remaining、reset time、max_concurrency、bucket wait、Identify attempt / success / failure を観測可能にする。

budget が枯渇する前に alert できるようにし、remaining=0 を通常の auto-retry condition としない。
