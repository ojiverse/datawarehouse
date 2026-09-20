# 実装とデータ運用の規律（Practices）

本ディレクトリでは、システム設計原則を実際の実装作業、データモデル策定、および外部データソース追加に適用する際の**実装規律および運用評価基準**を定義する。

## 規律一覧

* [設計としての型システム (Types as Design)](types-as-design.md)  
  型システムをアノテーションではなく設計の表現媒体とし、設計に反するコードをコンパイルエラーとして機械的に排除する。
* [デフォルトでの不変性 (Immutability by Default)](immutability-by-default.md)  
  データはデフォルトで不変（Immutable）とし、状態変更は新値への変換として表現する。可変性は厳格な例外として局所化する。
* [コメントは「Why」を記録する (Comments Explain Why)](comments-explain-why.md)  
  コードや型から復元不可能な外部制約、採用を見送った代替案、トレードオフの根拠（Why）のみをコメントに記録する。
* [データソースと機能の設計指針 (Datasource and Feature Design Policy)](datasource-and-feature.md)  
  情報セマンティクス（イベント、状態、知識）の厳格な分類、構造化データの維持、安定した識別性と来歴、インデックスの再構築可能性を担保する設計方針。
