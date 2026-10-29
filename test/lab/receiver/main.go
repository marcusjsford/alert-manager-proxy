// Command receiver retains a bounded set of synthetic lab webhook notifications.
package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

func main() {
	var mu sync.Mutex
	notifications := make([]json.RawMessage, 0)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /-/ready", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("OK\n"))
	})
	mux.HandleFunc("POST /alerts", func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil || !json.Valid(raw) {
			http.Error(w, "invalid notification", http.StatusBadRequest)
			return
		}
		mu.Lock()
		if len(notifications) >= 100 {
			notifications = notifications[1:]
		}
		notifications = append(notifications, json.RawMessage(raw))
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /notifications", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		copied := append([]json.RawMessage{}, notifications...)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(copied)
	})
	srv := &http.Server{
		Addr: ":8080", Handler: mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}
