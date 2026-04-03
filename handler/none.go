package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type purchaseRequest struct {
	ProductID int64 `json:"product_id"`
	Quantity  int   `json:"quantity"`
}

// PurchaseNone はロックなしの購入処理。レースコンディションが発生する。
//
// 問題のあるフロー:
//  1. 在庫を読み取る（SELECT）
//  2. 在庫があれば決済APIを呼ぶ（外部API呼び出し中に他のリクエストも同じ在庫を読める）
//  3. 在庫を減らす（UPDATE）
//  4. 注文を作成（INSERT）
func (h *Handler) PurchaseNone(w http.ResponseWriter, r *http.Request) {
	var req purchaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 1. 在庫確認
	var stock, price int
	err := h.db.QueryRow("SELECT stock, price FROM products WHERE id = ?", req.ProductID).
		Scan(&stock, &price)
	if err != nil {
		http.Error(w, "product not found", http.StatusNotFound)
		return
	}

	if stock < req.Quantity {
		http.Error(w, "out of stock", http.StatusConflict)
		return
	}

	// 2. 決済API呼び出し（ここで時間がかかる間に他のリクエストが同じ在庫を読む）
	paymentID, err := callPaymentAPI(price * req.Quantity)
	if err != nil {
		http.Error(w, "payment failed", http.StatusInternalServerError)
		return
	}

	// 3. 在庫を減らす
	_, err = h.db.Exec("UPDATE products SET stock = stock - ? WHERE id = ?", req.Quantity, req.ProductID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 4. 注文作成
	_, err = h.db.Exec("INSERT INTO orders (product_id, quantity, payment_id) VALUES (?, ?, ?)",
		req.ProductID, req.Quantity, paymentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok","payment_id":"%s"}`, paymentID)
}

func callPaymentAPI(amount int) (string, error) {
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
