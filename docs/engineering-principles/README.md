# エンジニアリング原則

本ディレクトリでは、特定の技術選定や一時的な実装手法を超えて、長期にわたり有効に機能するエンジニアリング原則を定義します。

これらの文書は、システム各部へ責任（Ownership）をどう割り当て、何を意味のある検証とし、5〜10年に及ぶ進化の中でリポジトリの可読性と保守性をどう維持するかを示す意思決定の指針です。スクリプトやルールチェックを無目的に増やすための規則集ではなく、設計判断の拠り所として機能します。

## 原則の体系

本原則群は、「開発における姿勢と優先順位の付け方」と「システム構造と設計の不変条件」の2つの側面から構成されています。

### 1. 開発姿勢と問題解決の原則
* [成否を決定づける支配的要因を見極める (Find the Dominant)](find-the-dominant.md)  
  努力量ではなく構造が価値を決める。課題の成否を握る唯一の急所にリソースを集中させ、表面的な症状ではなく構造を直す。
* [正しさを判定する評価基準を最初に築く (Optimize Function First)](optimize-function-first.md)  
  実装に飛びつく前に、成果の良し悪しを即座に客観判定できる評価基準（オラクルや検証データセット）を先行して整える。
* [人間は舵取りに徹し、実装は機械に委ねる (Human Steers, AI Implements)](human-steers-ai-implements.md)  
  境界、契約、不変条件、安全網を厳格に制御し、個別のコード記述や文体への執着を手放して全体最適を掴む。

### 2. 構造と設計の不変原則
* [要件と不変条件 (Requirements and Invariants)](requirements-and-invariants.md)  
  機能要件・非機能要件から、システムが維持すべき「不変条件」を導出する。
* [唯一の事実源 (Single Source of Truth)](single-source-of-truth.md)  
  すべての事実に唯一の所有者を割り当て、重複した権威の発生を防ぐ。
* [あるべき姿を宣言し、自律的に差分を収束させる (Declarative and Reconciliation)](declarative-and-reconciliation.md)  
  一回限りの手順書を排し、目標状態を定義してシステムに継続的な差分解消を行わせる。
* [疎結合と高凝集 (Loose Coupling and High Cohesion)](loose-coupling-and-high-cohesion.md)  
  関連する責務をひとまとめにし、外部コンポーネントが知るべき知識を最小限に絞る。
* [設計としての型システム (Types as Design)](types-as-design.md)  
  要件と不変条件を静的型として表現し、設計に反する実装がコンパイルを通らないようにする。
* [デフォルトでの不変性 (Immutability by Default)](immutability-by-default.md)  
  状態遷移を明示的な値の変換として扱い、ミュータブルなデータは正当な理由がある例外に限定する。
* [コメントは「Why」を記録する (Comments Explain Why)](comments-explain-why.md)  
  型やコード構造で表現できない、設計上の重要な意図・制約・トレードオフのみをコメントに残す。
* [削除と意図的な不在 (Deletion and Deliberate Absence)](deletion-and-deliberate-absence.md)  
  競合する仕組みを削除して責任の連鎖を明確にし、あえて存在させない理由を記録する。

これらの原則をデータソース追加や機能拡張に適用した具体例については、[設計ポリシー (Design Policies)](../design-policy/README.md) を参照してください。

## 保証の短いパス

すべての原則は、以下の短い検証パスを確立することを共通の目標としています。

```text
要件 (Requirement)
  -> 不変条件 (Invariant)
  -> 信頼できる情報源 (Authoritative Source)
  -> 所有するメカニズム (Owning Mechanism)
  -> 観測可能な結果・証拠 (Observable Outcome / Evidence)
```

目的はチェックの数を最大化することではありません。**どこに責任があり、何がその証拠であるかを曖昧さなく確定させること**です。
