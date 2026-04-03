package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// PurchaseLock はネームドロック（GET_LOCK）による購入処理。
//
// アドバイザリーロックで商品単位のロックを取得し、
// 在庫確認・決済・更新の全体を保護する。
// トランザクションとロックが独立しているため、トランザクションは短く保てる。
func (h *Handler) PurchaseLock(w http.ResponseWriter, r *http.Request) {
	var req purchaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// GET_LOCK / RELEASE_LOCK はセッション（コネクション）に紐づくため、
	// 同一コネクションで操作する必要がある
	conn, err := h.db.Conn(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	// 1. ネームドロック取得（タイムアウト10秒）
	lockName := fmt.Sprintf("purchase_product_%d", req.ProductID)
	var lockResult int
	err = conn.QueryRowContext(r.Context(), "SELECT GET_LOCK(?, 10)", lockName).Scan(&lockResult)
	if err != nil || lockResult != 1 {
		http.Error(w, "could not acquire lock", http.StatusServiceUnavailable)
		return
	}
	defer conn.ExecContext(r.Context(), "SELECT RELEASE_LOCK(?)", lockName)

	// 2. 在庫確認（ロック保持中なので他リクエストは待機中）
	var stock, price int
	err = conn.QueryRowContext(r.Context(), "SELECT stock, price FROM products WHERE id = ?", req.ProductID).
		Scan(&stock, &price)
	if err != nil {
		http.Error(w, "product not found", http.StatusNotFound)
		return
	}

	if stock < req.Quantity {
		http.Error(w, "out of stock", http.StatusConflict)
		return
	}

	// 3. 決済API呼び出し（ロック保持中だがトランザクションは開いていない）
	paymentID, err := callLockPaymentAPI(price * req.Quantity)
	if err != nil {
		http.Error(w, "payment failed", http.StatusInternalServerError)
		return
	}

	// 4. トランザクションはDB更新だけに絞る
	tx, err := conn.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	_, err = tx.Exec("UPDATE products SET stock = stock - ? WHERE id = ?", req.Quantity, req.ProductID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

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

func callLockPaymentAPI(amount int) (string, error) {
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
