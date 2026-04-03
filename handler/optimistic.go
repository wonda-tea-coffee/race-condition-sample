package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// PurchaseOptimistic は楽観的ロック（version）による購入処理。
//
// ロックを取らずに在庫と version を読み取り、決済API呼び出し後に
// UPDATE の WHERE で version が変わっていないことを確認する。
// 競合時は決済を取り消してエラーを返す。
func (h *Handler) PurchaseOptimistic(w http.ResponseWriter, r *http.Request) {
	var req purchaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 1. ロックなしで在庫と version を取得
	var stock, price, version int
	err := h.db.QueryRow("SELECT stock, price, version FROM products WHERE id = ?", req.ProductID).
		Scan(&stock, &price, &version)
	if err != nil {
		http.Error(w, "product not found", http.StatusNotFound)
		return
	}

	if stock < req.Quantity {
		http.Error(w, "out of stock", http.StatusConflict)
		return
	}

	// 2. 決済API呼び出し（ロックを取っていないので他リクエストも並行で通る）
	paymentID, err := callOptimisticPaymentAPI(price * req.Quantity)
	if err != nil {
		http.Error(w, "payment failed", http.StatusInternalServerError)
		return
	}

	// 3. version を条件に在庫更新（楽観的ロックの本体）
	tx, err := h.db.Begin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		"UPDATE products SET stock = stock - ?, version = version + 1 WHERE id = ? AND version = ? AND stock >= ?",
		req.Quantity, req.ProductID, version, req.Quantity)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	affected, err := result.RowsAffected()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if affected == 0 {
		// 競合検知 — 決済済みだが注文できないので取り消しが必要
		// （本来はここで決済の取り消しAPIを呼ぶ）
		http.Error(w, "conflict: version changed (payment should be refunded)", http.StatusConflict)
		return
	}

	// 4. 注文作成
	_, err = tx.Exec("INSERT INTO orders (product_id, quantity, payment_id) VALUES (?, ?, ?)",
		req.ProductID, req.Quantity, paymentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok","payment_id":"%s"}`, paymentID)
}

func callOptimisticPaymentAPI(amount int) (string, error) {
	body, _ := json.Marshal(map[string]int{"amount": amount})
	resp, err := http.Post("http://localhost:8080/mock/payment", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	var result paymentResponse
	if err := json.Unmarshal(b, &result); err != nil {
		return "", err
	}
	return result.PaymentID, nil
}
