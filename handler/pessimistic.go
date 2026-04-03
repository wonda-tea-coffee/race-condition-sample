package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// PurchasePessimistic は悲観的ロック（SELECT FOR UPDATE）による購入処理。
//
// トランザクション開始時に行ロックを取得し、決済API呼び出しを含む
// 全処理が完了するまでロックを保持する。
// 他のリクエストは同じ行のロック解放を待つため、直列化される。
func (h *Handler) PurchasePessimistic(w http.ResponseWriter, r *http.Request) {
	var req purchaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// 1. SELECT FOR UPDATE で行ロックを取得
	var stock, price int
	err = tx.QueryRow("SELECT stock, price FROM products WHERE id = ? FOR UPDATE", req.ProductID).
		Scan(&stock, &price)
	if err != nil {
		http.Error(w, "product not found", http.StatusNotFound)
		return
	}

	if stock < req.Quantity {
		http.Error(w, "out of stock", http.StatusConflict)
		return
	}

	// 2. 決済API呼び出し（ロックを握ったまま）
	paymentID, err := callPessimisticPaymentAPI(price * req.Quantity)
	if err != nil {
		http.Error(w, "payment failed", http.StatusInternalServerError)
		return
	}

	// 3. 在庫を減らす
	_, err = tx.Exec("UPDATE products SET stock = stock - ? WHERE id = ?", req.Quantity, req.ProductID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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

func callPessimisticPaymentAPI(amount int) (string, error) {
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
