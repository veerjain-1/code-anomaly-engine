// Package main provides the API gateway for the code anomaly detection engine.
// It consumes code snippets from Kafka, sends them to the Rust inference engine
// via gRPC, publishes results, and serves a real-time WebSocket feed + REST API.
package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/veerjain-1/code-anomaly-engine/services/gateway/internal/api"
	"github.com/veerjain-1/code-anomaly-engine/services/gateway/internal/kafka"
	"github.com/veerjain-1/code-anomaly-engine/services/gateway/internal/ws"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	port := getEnv("PORT", "8080")
	inferenceAddr := getEnv("INFERENCE_ADDR", "localhost:50051")
	kafkaBroker := getEnv("KAFKA_BROKER", "localhost:9092")

	// WebSocket hub for real-time anomaly feed
	hub := ws.NewHub()
	go hub.Run()

	// Anomaly store (in-memory for demo)
	store := api.NewAnomalyStore()

	// Kafka -> gRPC bridge
	topic := getEnv("KAFKA_TOPIC", "code-snippets")
	bridge, err := kafka.NewBridge(kafkaBroker, topic, inferenceAddr, func(result kafka.AnomalyResult) {
		// Save to store
		store.Add(api.Anomaly{
			ID:          result.ID,
			Repo:        result.Repo,
			CommitSHA:   result.CommitSHA,
			FilePath:    result.FilePath,
			StartLine:   result.StartLine,
			EndLine:     result.EndLine,
			Code:        result.Code,
			IsAnomalous: result.IsAnomalous,
			Confidence:  result.Confidence,
			Label:       result.Label,
			LatencyMs:   result.LatencyMs,
			Timestamp:   result.Timestamp,
		})

		// Broadcast to WebSocket clients
		b, _ := json.Marshal(result)
		hub.Broadcast(b)
	})
	if err != nil {
		slog.Error("Failed to initialize Kafka bridge", "error", err)
		os.Exit(1)
	}

	bridgeCtx, bridgeCancel := context.WithCancel(context.Background())
	bridge.Start(bridgeCtx)

	// HTTP server
	mux := http.NewServeMux()

	// REST endpoints
	mux.HandleFunc("GET /api/anomalies", store.HandleList)
	mux.HandleFunc("GET /api/anomalies/{id}", store.HandleGet)
	mux.HandleFunc("GET /api/stats", store.HandleStats)

	// WebSocket endpoint
	mux.HandleFunc("GET /ws/feed", hub.HandleWebSocket)

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":         "ok",
			"inference_addr": inferenceAddr,
			"kafka_broker":   kafkaBroker,
		})
	})

	// Prometheus metrics
	mux.HandleFunc("GET /metrics", api.HandleMetrics)

	// CORS middleware for dashboard
	handler := corsMiddleware(mux)

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup

	// Start HTTP server
	wg.Add(1)
	go func() {
		defer wg.Done()
		slog.Info("Gateway starting",
			"port", port,
			"inference_addr", inferenceAddr,
			"kafka_broker", kafkaBroker,
		)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("Shutting down gateway...")

	bridgeCancel()
	bridge.Stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)
	hub.Close()
	wg.Wait()
	slog.Info("Gateway stopped")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
