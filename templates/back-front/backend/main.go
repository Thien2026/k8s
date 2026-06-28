package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

func main() {
	prefix := strings.TrimSuffix(strings.TrimSpace(os.Getenv("API_ROUTE_PREFIX")), "/")
	if prefix == "" {
		prefix = "/api"
	}
	mux := http.NewServeMux()
	mux.HandleFunc(prefix+"/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc(prefix+"/hello", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "hello from api"})
	})
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	_ = http.ListenAndServe(":"+port, mux)
}
