# システム設計と不変条件（System Design）

本ディレクトリでは、システム全体のアーキテクチャやドメインモデルを策定する際に、長期間にわたって破綻を防ぐための**構造設計と不変条件に関する基本原則**を扱います。

## 原則一覧

* [現在の確実性のレベルで設計する (Design at the Current Level of Certainty)](design-at-current-certainty.md)  
  すでに確定している既知の事実には厳格であり、未知の事柄には寛容であること。早すぎる解釈より情報の保存（来歴）を優先し、未知の将来要件のための余白を意図して残す。
* [要件と不変条件 (Requirements and Invariants)](requirements-and-invariants.md)  
  機能要件・非機能要件を、システムが常に保護すべき「不変条件」へと翻訳して設計を開始する。
* [唯一の事実源 (Single Source of Truth)](single-source-of-truth.md)  
  すべての宣言的事実と解釈に唯一の所有者を割り当て、並行した第二の権威や重複したコピーを作らない。
* [あるべき姿を宣言し、自律的に差分を収束させる (Declarative and Reconciliation)](declarative-and-reconciliation.md)  
  一回限りの命令的手順書を排し、目標状態をデータとして定義してシステムに継続的な差分解消を行わせる。
* [疎結合と高凝集 (Loose Coupling and High Cohesion)](loose-coupling-and-high-cohesion.md)  
  密接に関連する責務をひとまとめにし、外部コンポーネントが依存すべき公開契約を最小限に絞る。
* [削除と意図的な不在 (Deletion and Deliberate Absence)](deletion-and-deliberate-absence.md)  
  重複した仕組みを削除して責任の連鎖を明確にし、あえて作らなかった理由（意図的な不在）を記録する。
