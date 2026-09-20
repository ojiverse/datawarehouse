# Data Erasure

本書では Product Policy に基づく明示的な user data deletion requirement の実現方式を定義する。

## 通常削除との分離

Discord 上の通常の Message delete は compliance erasure ではない。通常削除は historical evidence として Archive に残す。

本人、Discord、法的要請等による erasure requirement だけが物理消去 workflow を開始する。

## Request Boundary

現在の OJIverse メンバーは、DWH と同じ community identity で認証された self-service deletion request を発行できる。

本人による request は自分自身の Discord User を subject とする。他ユーザーの erasure を一般ユーザーが要求することはできない。

Discord からの要求、法的要請、運営上の compliance request は管理者経路から同じ Erasure Run へ正規化する。

request の受付時には Erasure Run ID を発行し、accepted / running / completed / failed の状態を request owner から確認可能にする。

Erasure audit record には request metadata と execution result だけを残し、削除対象 API Data の本文や復元可能な copy を保存しない。

## Authoritative Discovery

削除対象の発見は Observation Archive の全走査を最終 authority とする。

高速化用の subject index を構築してよいが、index だけを deletion completeness の根拠にしてはならない。schema evolution や過去の indexing bug により見逃し得るためである。

削除 run は対象 Discord User Snowflake と Erasure Run ID を受け取り、Envelope / payload を現在の Discord schema rules で走査して対象ユーザーに紐づく API Data を特定する。

## Archive Rewrite

1 HTTP page に複数ユーザーの data が含まれるため、対象 object 全体を無条件に削除しない。

対象ユーザーの API Data を含む Observation object は compliance rewrite する。

rewrite 後も Observation ID と元の観測 provenance は維持するが、Envelope に compliance erasure が適用済みであることと Erasure Run ID を記録する。

rewrite は対象 object を読み取った時点の ETag を条件に実行する。条件不一致の場合は最新 object を再評価し、古い copy で上書きしない。

削除対象 field / entity は payload から物理的に除去し、元値を tombstone、hash、暗号文として残さない。

対象ユーザーが主体である Message / Member / Reaction 等の entity は entity 単位で除去する。他ユーザーの entity 内に埋め込まれた対象ユーザーの structured reference も除去または非識別化する。

Product Policy に基づき compliance rewrite は通常の append-only invariant に優先する。

## Derived Data

Archive rewrite 後、Canonical Store、全文検索、Vectorize、cache、Agent index 等の Derived Data は対象ユーザーを含まない状態へ rebuild / purge する。

Canonical だけを消して Archive に元データを残した状態を deletion complete とみなさない。

## Re-ingestion Prevention

erasure 後に過去 HTTP Backfill や Gateway event から同じ user data を再導入してはならない。

compliance control state に suppression marker を保持し、Archive write 前に対象 subject の API Data を除去する。

raw Discord User ID 自体を長期 suppression record として残すことを避けるため、marker は secret-keyed HMAC-SHA-256 による subject token とする。

HMAC secret は DWH data と分離した secret store で管理し、rotation 時に既存 marker を継続評価できる migration を必要とする。

この suppression marker は削除済み API Data を復元する用途に使用してはならない。

## Completion

erasure run は少なくとも以下を満たして完了とする。

* Archive full scan で対象 API Data が検出されない
* Canonical / search / Vectorize / cache / Agent index から対象 API Data を取得できない
* future ingestion に対する suppression が有効
* R2 delete / rewrite が strong consistency 下で反映済み
* erasure audit record が API Data の内容を再保持していない

Observation Archive bucket に deletion を妨げる indefinite Bucket Lock を設定してはならない。
