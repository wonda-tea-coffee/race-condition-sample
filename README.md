# Race Condition Sample

ECサイトの商品購入を題材に、排他制御（レースコンディション対策）を学ぶハンズオンリポジトリです。

## 購入フロー

```
1. 在庫を確認する (SELECT)
2. 決済APIを呼ぶ (外部API)  ← この間に他のリクエストも在庫を読める
3. 在庫を減らす (UPDATE)
4. 注文を作成する (INSERT)
```

ロックなしの場合、ステップ2の決済API呼び出し中に複数のリクエストが同じ在庫数を読み取り、全員が「在庫あり」と判断してしまいます。結果として在庫がマイナスになったり、実際の在庫以上の注文が作られます。

## セットアップ

Docker と Docker Compose が必要です。

```bash
docker compose up -d --build
```

API（Go）が `localhost:8080`、MySQL が `localhost:3306` で起動します。

## 検証してみる

```bash
bash scripts/verify.sh
```

在庫5個の商品に対して20件の同時購入リクエストを送り、結果の整合性をチェックします。

**ロックなしの場合の出力例:**

```
=== ロックなし (レースコンディション発生) ===
初期在庫: 5
20 件の並行購入リクエストを送信中...

--- 結果 ---
成功レスポンス (200): 20
在庫切れ (409):       0
エラー:               0
最終在庫:             -15 (期待: 0)
注文数:               20

判定: NG — 不整合あり
       在庫がマイナスになっています！
```

在庫5個なのに20件全て購入が成功し、在庫が -15 になっています。

## 手動で試す

```bash
# 在庫を5にリセット
curl -X POST localhost:8080/reset -H 'Content-Type: application/json' -d '{"stock": 5}'

# 購入（ロックなし）
curl -X POST localhost:8080/purchase/none -H 'Content-Type: application/json' -d '{"product_id": 1, "quantity": 1}'

# 現在の商品情報を確認
curl localhost:8080/products/1

# 注文一覧を確認
curl localhost:8080/orders
```

## ロック戦略の比較

| エンドポイント | 戦略 | 状態 |
|---|---|---|
| `POST /purchase/none` | ロックなし | 実装済 |
| `POST /purchase/pessimistic` | 悲観的ロック (`SELECT ... FOR UPDATE`) | 実装済 |
| `POST /purchase/optimistic` | 楽観的ロック (version カラム) | 実装済 |
| `POST /purchase/lock` | ネームドロック (`GET_LOCK()`) | 実装済 |

## 停止

```bash
docker compose down
```
