# Canonical Store Domain

Discord から得た Observation を、長期的に分析可能な一貫したデータモデルへ変換した Canonical Data を扱います。

## 役割

Canonical Store は Observation Archive の代替ではありません。

Observation Archive が「何を観測したか」を保持するのに対し、Canonical Store は「観測結果を Discord DWH としてどう解釈するか」を表現します。

Canonical Store は Observation Archive から再構築可能であることを原則とします。

## 初期対象

初期段階では特に次の Discord entity を中心に設計します。

- Message
- Channel
- Thread

Reaction、Member、Role などは必要性が確認された時点で追加します。

## 履歴と Current State

Canonical Data は append-oriented な observation history を基本とします。

Current State を唯一の保存形式とはしません。

Message の編集や削除、HTTP snapshot など複数の観測から、必要な時点の状態を projection として求められる設計を目指します。

## Schema Evolution

5〜10年の運用を想定し、Discord API の変化と Canonical schema の変化を前提とします。

Canonical schema の変更で過去データを再処理する必要が生じた場合、Observation Archive から再構築できることを重視します。

## 今後分割する詳細設計

- Canonical Data Model
- Message Model
- Channel and Thread Model
- Historical State
- Schema Evolution
