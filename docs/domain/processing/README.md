# データ処理ドメイン（Processing）

本ディレクトリでは、Observation Archive に蓄積された生の観測事実（Observation）を解釈・正規化し、分析用の Canonical Data へと変換・マテリアライズする処理パイプラインの意味論を定義する。

## 責務と基本原則

### 1. 取り込みと処理の障害隔離（Durability First）
正規化パイプラインの不具合や一時的なダウンストリーム障害が発生した場合であっても、**前段の Observation Archive への生データ保存が決して阻害されてはならない**。
Observation の耐久性（Durability）を最優先とし、処理層の不具合は「バグ修正後の生データ再読み込み（Replay）により完全復旧可能」なアーキテクチャを前提とする。

### 2. データ来歴の保持と非改変
正規化処理において、HTTP スナップショットを架空の Gateway イベントへ変換（捏造）することを禁じる。
Canonical Data への変換後であっても、元となった Observation の取得経路（Gateway または HTTP）、受信時刻、およびセッション情報が完全に追跡（Provenance）可能な状態を維持する。

### 3. 重複排除と状態投影（Deduplication & Projection）
同じ Discord entity に関する複数の Observation が Archive に存在することを正常状態とする。

処理層は Observation ID による source-local duplicate と、再取得・複数 Gateway 等による独立 Observation を区別する。

複数 Gateway Session を跨ぐ global total order は存在すると仮定しない。HTTP-only の Message projection は [projection.md](projection.md) に定義した deterministic rule に従い、best-known state を導出する。

## 定常機能としての再処理（Replay & Rebuild）

5〜10年にわたる長期運用において、分析要件の追加、スキーマ定義の変更、および正規化ロジックの改善に伴う再処理は、例外対応ではなく「定常機能（System Capability）」として設計されなければならない。

* **Replay**: 特定時間範囲の Observation をパイプラインへ再投入し、差分や誤変換を修復する。
* **Rebuild**: 新規 Canonical スキーマに基づき、蓄積されたすべての Observation をゼロベースで再変換し、新規テーブル表現を構築する。

## 詳細設計

* [projection.md](projection.md): HTTP Message snapshot から best-known state を決定する deterministic projection

## 今後分割する詳細設計

* **Observation Normalization**: Discord の多層 JSON からフラットな Canonical 表現へのマッピング規則
* **Deduplication**: Observation ID、source delivery identity、semantic duplicate の分離
* **Gateway Ordering**: Session-local sequence と cross-session reconciliation の規則
* **Gateway & Snapshot Reconciliation**: リアルタイムイベントと HTTP スナップショットの競合解決
* **Replay & Rebuild**: 過去 Observation の再読み込み手順と Canonical Store の無停止切り替え
