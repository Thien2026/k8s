package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

func main() {
	prefix := strings.TrimSuffix(strings.TrimSpace(os.Getenv("API_ROUTE_PREFIX")), "/")
	if prefix == "" {
		prefix = "/api"
	}
	rdb := newRedisClient()
	mux := http.NewServeMux()
	mux.HandleFunc(prefix+"/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc(prefix+"/hello", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "hello from api"})
	})
	mux.HandleFunc(prefix+"/redis/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if rdb == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "REDIS_URL chưa có — bật Redis addon trên Console",
			})
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := rdb.Ping(ctx).Err(); err != nil {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"result": "PONG"})
	})
	mux.HandleFunc(prefix+"/redis/demo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if rdb == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "REDIS_URL chưa có"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		key := "console:hello"
		ttl := defaultKeyTTL()
		val := "world"
		if err := rdb.Set(ctx, key, val, ttl).Err(); err != nil {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		got, err := rdb.Get(ctx, key).Result()
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"key": key, "value": got, "ttl_sec": int64(ttl.Seconds()),
		})
	})
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	_ = http.ListenAndServe(":"+port, mux)
}

func newRedisClient() *redis.Client {
	u := strings.TrimSpace(os.Getenv("REDIS_URL"))
	if u == "" {
		return nil
	}
	opt, err := redis.ParseURL(u)
	if err != nil {
		return nil
	}
	return redis.NewClient(opt)
}

func defaultKeyTTL() time.Duration {
	raw := strings.TrimSpace(os.Getenv("REDIS_KEY_TTL_SECONDS"))
	if raw == "" {
		return time.Hour
	}
	sec, err := strconv.Atoi(raw)
	if err != nil || sec <= 0 {
		return time.Hour
	}
	return time.Duration(sec) * time.Second
}
