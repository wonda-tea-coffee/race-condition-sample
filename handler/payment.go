package handler

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type paymentRequest struct {
	Amount int `json:"amount"`
}

type paymentResponse struct {
	PaymentID string `json:"payment_id"`
}

var (
	paymentCount atomic.Int64
	paymentMu    sync.Mutex
	paymentIDs   []string
)

// MockPayment は決済APIのモック。意図的に遅延を入れてレースコンディションの窓を広げる。
func (h *Handler) MockPayment(w http.ResponseWriter, r *http.Request) {
	var req paymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 100〜300ms のランダム遅延
	time.Sleep(time.Duration(100+rand.IntN(200)) * time.Millisecond)

	id := fmt.Sprintf("pay_%d", rand.IntN(1_000_000))

	paymentCount.Add(1)
	paymentMu.Lock()
	paymentIDs = append(paymentIDs, id)
	paymentMu.Unlock()

	resp := paymentResponse{PaymentID: id}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// GetPayments は決済成功数と決済ID一覧を返す。
func (h *Handler) GetPayments(w http.ResponseWriter, r *http.Request) {
	paymentMu.Lock()
	ids := make([]string, len(paymentIDs))
	copy(ids, paymentIDs)
	paymentMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"count": paymentCount.Load(),
		"ids":   ids,
	})
}

// ResetPayments は決済記録をリセットする。
func ResetPayments() {
	paymentCount.Store(0)
	paymentMu.Lock()
	paymentIDs = nil
	paymentMu.Unlock()
}
