package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type product struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Price   int    `json:"price"`
	Stock   int    `json:"stock"`
	Version int    `json:"version"`
}

type order struct {
	ID        int64  `json:"id"`
	ProductID int64  `json:"product_id"`
	Quantity  int    `json:"quantity"`
	PaymentID string `json:"payment_id"`
	CreatedAt string `json:"created_at"`
}

func (h *Handler) GetProduct(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var p product
	err := h.db.QueryRow("SELECT id, name, price, stock, version FROM products WHERE id = ?", id).
		Scan(&p.ID, &p.Name, &p.Price, &p.Stock, &p.Version)
	if err != nil {
		http.Error(w, "product not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

func (h *Handler) ListOrders(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query("SELECT id, product_id, quantity, payment_id, created_at FROM orders ORDER BY id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var orders []order
	for rows.Next() {
		var o order
		if err := rows.Scan(&o.ID, &o.ProductID, &o.Quantity, &o.PaymentID, &o.CreatedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		orders = append(orders, o)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orders)
}

type resetRequest struct {
	Stock int `json:"stock"`
}

func (h *Handler) Reset(w http.ResponseWriter, r *http.Request) {
	var req resetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Stock <= 0 {
		req.Stock = 10
	}

	_, err := h.db.Exec("UPDATE products SET stock = ?, version = 1 WHERE id = 1", req.Stock)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_, err = h.db.Exec("DELETE FROM orders")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ResetPayments()

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"reset","stock":%d}`, req.Stock)
}
