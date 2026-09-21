# エージェント行動規範（AGENTS.md）

本リポジトリで作業するすべての AI エージェントおよび開発者が遵守すべき行動規範を定義する。
実装および設計は推測によらず、本規範および所定の設計文書を起点として進めるものとする。

## 1. プロジェクト概要の確認
作業着手前に、必ずリポジトリルートの [README.md](README.md) を確認すること。
本データウェアハウスの目的、2層データフロー（生ログである Observation Archive と分析用 Canonical Store の分離）、および開発フェーズの全体像を把握した上で作業を開始しなければならない。

## 2. Product Policy の遵守
設計および実装に着手する前に、必ず [docs/domain/product-policy/](docs/domain/product-policy/README.md) を確認すること。
利用目的、community-wide corpus、Retention / Deletion、Collection Policy、best-known state は Product Owner が確定した不変条件であり、AI エージェントや実装者が独自判断で変更してはならない。

## 3. 設計思想と原則の遵守
設計および実装上の判断を下す際は、必ず [docs/philosophy/](docs/philosophy/README.md) 配下の設計思想とエンジニアリング原則を確認し、その規律を厳格に遵守すること。

* **[approach/](docs/philosophy/approach/README.md)（開発姿勢と問題解決）**: 課題の成否を握る支配的要因へリソースを集中させ、実装コード作成前に正しさを判定する評価基準を先行して確立し、人間が定義した責務境界と不変条件を制約として作業を進めること。
* **[system-design/](docs/philosophy/system-design/README.md)（システム設計と不変条件）**: 現在の確実性のレベルで設計し、未知の将来要件のための余白を残し、唯一の事実源と宣言的な差分収束を徹底すること。
* **[practices/](docs/philosophy/practices/README.md)（実装とデータ運用の規律）**: 設計としての型システム、デフォルトでの不変性、理由（Why）を記録するコメント、およびデータソース設計指針に従うこと。

## 4. 実装言語ポリシーの遵守
実装に着手する前に [docs/architecture/implementation-language.md](docs/architecture/implementation-language.md) を確認すること。

既定実装言語は Go とし、Cloudflare Workers / Durable Objects の native runtime adapter に限って TypeScript を使用する。言語選択を実装者の好みで変更してはならない。Production dependency として別言語を追加する場合は ADR を要求する。

## 5. 設計文書体系の参照
具体的なドメイン仕様、Cloudflare 上でのアーキテクチャ、インフラ設定、採択済み ADR、運用手順については、すべて [docs/README.md](docs/README.md) を起点として段階的に参照すること。設計文書の規約（200行制限、自然言語と図のみの記述）を常に維持しなければならない。
