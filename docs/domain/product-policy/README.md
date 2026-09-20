# Product Policy

本ディレクトリでは、OJIverse Data Warehouse が何のために存在し、誰が利用でき、何を収集・保持し、どのような状態を提供するかという Product Owner の決定を定義する。

これらは実装方式ではなく、Domain、Architecture、Infrastructure のすべてを拘束するプロダクト上の不変条件である。

## 1. Product Purpose

OJIverse Data Warehouse は、OJIverse コミュニティ内で観測された過去の発言・活動を長期的な記憶として保持し、現在および将来の Bot / AI Agent がそれらを検索・参照・集計して、過去のコミュニティ活動を文脈として利用する機能を提供するための共通データ基盤とする。

個人を指定した過去発言の retrieval も core functionality に含む。

Observation Archive と Canonical Store が長期的な基盤を担い、全文検索 index、Vectorize、Agent 固有 memory、その他の retrieval index は、それらから再構築可能な Derived Data として扱う。

この DWH は Discord API Data を AI / ML model の学習 corpus とすることを目的としない。

## 2. Community-wide Corpus と Authorization

DWH は **community-wide corpus** とする。

DWH に保存する Channel / Thread scoped data は、OJIverse の任意のメンバーへ公開してよいと明示された scope に限定する。この scope を **DWH-public** と呼ぶ。

DWH を利用できる主体は、現在の OJIverse メンバー、およびそのメンバーに紐づいて利用される Bot / Application とする。

人間と Bot / Application で取得可能なデータ範囲を区別しない。同一の認可主体として利用が許可されている限り、同じ問い合わせから得られる corpus は等しい。

DWH 内では Discord の Role、Channel Permission、Permission Override を再構築せず、record-level authorization を行わない。

一度 DWH-public として取り込まれたデータは、元 Channel / Thread が後から private 化されても community-wide corpus に残る。

ただし、明示的なデータ削除要求、Discord からの削除要求、法的義務その他の deletion requirement はこの規則に優先する。

DWH-public をどのような設定や管理手段で指定するかは Architecture の責務とする。

## 3. Retention と Deletion

DWH-public として取得した API Data は、stated functionality のために必要であり、削除義務が発生していない限り、期限を設けず保持する。

Discord 上で通常の Message 削除が行われても、それだけを理由に Observation Archive の historical evidence を物理削除しない。

Message 削除を観測できた場合は「過去に存在し、その後削除された」という事実として保持し、Canonical の Current State に反映する。

対象ユーザー本人、Discord、法的要請その他の理由で API Data の削除義務が発生した場合は、対象ユーザーに紐づく **全 API Data** を削除対象とする。

削除対象には Observation Archive、Canonical Store、および Vectorize、全文検索 index、cache、Agent 固有 index 等の Derived Data を含む。

具体的な削除方式、subject index、object rewrite、暗号化消去等は Architecture / Infrastructure の責務とする。

## 4. Collection Policy

DWH-public scope で観測可能な **durable community activity** は、現在の利用機能で必要かどうかにかかわらず、原則として Observation Archive に保存する。

Message、編集、削除、Reaction、Thread / Channel の作成や変更等は、将来の再解釈や Bot / AI Agent からの利用可能性を残すための evidence として扱う。

保存しないデータは暗黙に drop せず、Deliberate Absence として対象と理由を文書化する。

以下の transient behavioral telemetry は収集対象から明示的に除外する。

* Presence
* Typing
* Voice State

Canonical Store や Derived Data が構造化・index 化する範囲は、Observation Archive の収集範囲より狭くてよい。

## 5. State Semantics

DWH が提供する Current State / Historical State は、Discord の完全な真実そのものではない。

Observation Archive に保存された evidence から deterministic に導出できる **best-known state** とする。

* **Current State**: 現時点までに観測できた evidence から導出した最新の best-known state
* **Historical State at T**: 対象時点までに利用可能な観測 evidence から導出可能な best-known state

観測できなかった event、中間編集、短時間で作成・削除された entity 等を推測して補完してはならない。

Gateway event history と HTTP snapshot では完全性が異なるため、取得経路と provenance を保持し、結果の根拠と限界を説明可能にする。

## 設計への帰結

本 Policy から以下を導く。

* Observation Archive は将来の機能追加に対して最も広い evidence を保持する
* Canonical Store は Archive から再構築可能な解釈済み表現である
* Vectorize 等の検索 index は Source of Truth ではなく再構築可能な Derived Data である
* Authorization は record ごとではなく、DWH corpus を利用できる主体かどうかの admission として扱う
* Deletion requirement は Append-only の通常動作より優先する
* State projection は完全性を捏造せず、best-known state として提供する
