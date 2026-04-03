# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## プロジェクト概要

排他制御（Race Condition対策）の学習用リポジトリ。ECサイトの商品購入を題材に、DB操作と外部API呼び出しが絡むシナリオでのレースコンディションと各種対策を比較する。

## 技術スタック

- Go 1.23（標準ライブラリ net/http）、MySQL 8.0
- Docker Compose でAPI・DBを起動（ローカル環境を汚さない）
- ローカルの Go バージョンが古いため、`go mod tidy` 等は Docker 内でのみ実行される

## コマンド

```bash
# 起動（初回・コード変更時は --build を付ける）
docker compose up -d --build

# 停止
docker compose down

# ログ確認
docker compose logs -f api

# 検証スクリプト実行
bash scripts/verify.sh
```

## シナリオ

商品購入フロー: 在庫確認(SELECT) → 決済API呼び出し(外部API) → 在庫更新(UPDATE) → 注文作成(INSERT)

| エンドポイント | 戦略 | 状態 |
|---|---|---|
| `POST /purchase/none` | ロックなし | 実装済 |
| `POST /purchase/pessimistic` | 悲観的ロック (SELECT FOR UPDATE) | 実装済 |
| `POST /purchase/optimistic` | 楽観的ロック (version) | 実装済 |
| `POST /purchase/lock` | ネームドロック (GET_LOCK) | 実装済 |

## アーキテクチャ

- `main.go` — ルーティングとDB接続
- `handler/` — 各ロック戦略のハンドラー。`handler.go` で共通の `Handler` 構造体（`*sql.DB` を保持）を定義
- `handler/payment.go` — モック決済API（100〜300ms のランダム遅延でレースコンディションの窓を広げる）
- `handler/common.go` — リセット・商品取得・注文一覧
- `db/init.sql` — スキーマ定義と初期データ
- `scripts/verify.sh` — 並行リクエストで整合性を検証

## 設計方針

- 各ロック戦略は `handler/` 内に1ファイル1戦略で配置
- 新しい戦略を追加するときは: ハンドラー実装 → `main.go` にルート追加 → `verify.sh` に `run_test` 行追加
- 学習目的のため、コードはシンプルに保ち、過度な抽象化をしない
