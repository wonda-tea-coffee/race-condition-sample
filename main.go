package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"

	_ "github.com/go-sql-driver/mysql"
	"github.com/wonda-tea-coffee/race-condition-sample/handler"
)

func main() {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		dsn = "root:password@tcp(localhost:3306)/shop?parseTime=true&charset=utf8mb4"
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}

	h := handler.New(db)

	mux := http.NewServeMux()

	// 商品・注文
	mux.HandleFunc("GET /products/{id}", h.GetProduct)
	mux.HandleFunc("GET /orders", h.ListOrders)
	mux.HandleFunc("POST /reset", h.Reset)

	// モック決済API
	mux.HandleFunc("POST /mock/payment", h.MockPayment)
	mux.HandleFunc("GET /mock/payments", h.GetPayments)

	// 購入（各ロック戦略）
	mux.HandleFunc("POST /purchase/none", h.PurchaseNone)
	mux.HandleFunc("POST /purchase/pessimistic", h.PurchasePessimistic)
	mux.HandleFunc("POST /purchase/optimistic", h.PurchaseOptimistic)
	mux.HandleFunc("POST /purchase/lock", h.PurchaseLock)

	fmt.Println("Server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
