# ADR-0008: OJIverse DWH の Product Policy を固定する

* **ステータス**: 承認（Accepted）
* **決定日**: 2026-09-20
* **対象領域**: Product Policy / Domain Design

## コンテキスト

Observation Archive、Canonical Store、Gateway、Backfill の技術設計を進める中で、既存仕様やベストプラクティスだけでは決められない Product Owner 判断が残っていた。

特に、DWH の最終目的、利用主体、Retention / Deletion、Collection scope、Current / Historical State の意味は、実装者が技術的な都合で決めるべき事項ではない。

これらを先に固定し、今後の設計および実装 Agent が同じ Product Policy を前提として判断できる状態にする必要がある。

## 決定事項

### 1. OJIverse の長期記憶基盤とする

DWH は、OJIverse コミュニティ内で観測された過去の発言・活動を保持し、現在および将来の Bot / AI Agent が検索・参照・集計して機能提供するための共通データ基盤とする。

個人を指定した過去発言の retrieval も core functionality に含む。

Vectorize、全文検索 index、Agent memory 等は Observation Archive / Canonical Store から再構築可能な Derived Data とする。

### 2. Community-wide corpus とする

DWH に保存する Channel / Thread scoped data は、任意の OJIverse メンバーへ公開してよい DWH-public scope に限定する。

現在の OJIverse メンバーと、そのメンバーに紐づく Bot / Application は同じ corpus を利用できる。

Discord の Role / Channel permission を DWH 内で再実装せず、record-level authorization を持たない。

一度 DWH-public として取り込んだデータは、元 scope が後に private 化されても corpus に残す。

### 3. 原則無期限保持とする

取得した API Data は、stated functionality に必要で削除義務がない限り期限を設けず保持する。

通常の Discord Message 削除は Archive の物理削除 trigger としない。

一方、ユーザー本人、Discord、法的要請等によって削除義務が発生した場合は、そのユーザーに紐づく全 API Data と Derived Data を削除対象とする。

### 4. Durable activity は原則保存する

DWH-public scope で観測可能な durable community activity は、現在の機能が利用しなくても原則 Archive する。

Presence、Typing、Voice State のような transient behavioral telemetry は deliberate absence として収集対象外とする。

### 5. Best-known state を提供する

Current / Historical State は Discord の完全な真実とは定義しない。

Observation Archive に存在する evidence から deterministic に導出可能な best-known state とする。

未観測の event や中間状態を推測して補完してはならない。

## 結果

この決定により、実装者は Product Policy を再判断せず、技術仕様と不変条件から Architecture / Infrastructure を設計できる。

Authorization、Retention、Collection、State projection に関する技術選択は、[Product Policy](../domain/product-policy/README.md) を満たす範囲で行わなければならない。

Product Policy 自体を変更する場合は、新しい Product Owner 判断と ADR によって本 ADR を置換する。
