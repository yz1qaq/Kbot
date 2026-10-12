package main

import (
	"encoding/json"
	"errors"
	"log"
	"my_kbot-crossborder/internal/httpapi"
	"my_kbot-crossborder/internal/service"
	"net/http"
	"os"
	"time"
)

func main() {
	addr := os.Getenv("CROSSBORDER_ADDR")
	if addr == "" {
		addr = ":8091"
	}
	svc := service.NewSeeded()
	router := httpapi.New(svc)

	server := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("crossborder simulator listening on %s", addr)

	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

// writeJSON 将数据编码为 JSON 并写入 HTTP 响应。
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
