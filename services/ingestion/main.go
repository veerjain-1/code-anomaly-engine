// Package main provides the entry point for the code anomaly detection
// ingestion service. It receives GitHub webhook events, parses code diffs,
// extracts function-level snippets, and publishes them to Kafka for
// downstream ML inference.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/veerjain-1/code-anomaly-engine/services/ingestion/internal/kafka"
	"github.com/veerjain-1/code-anomaly-engine/services/ingestion/internal/webhook"
)

func main() {
	// Structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Configuration from environment
	port := getEnv("PORT", "8081")
	kafkaBroker := getEnv("KAFKA_BROKER", "localhost:9092")
	kafkaTopic := getEnv("KAFKA_TOPIC", "code-snippets")
	webhookSecret := getEnv("GITHUB_WEBHOOK_SECRET", "")

	// Initialize Kafka producer
	producer, err := kafka.NewProducer(kafkaBroker, kafkaTopic)
	if err != nil {
		slog.Error("Failed to create Kafka producer", "error", err)
		os.Exit(1)
	}
	defer producer.Close()

	// Initialize webhook handler
	handler := webhook.NewHandler(producer, webhookSecret)

	// HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhook", handler.HandleWebhook)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("Ingestion service starting", "port", port, "kafka_broker", kafkaBroker, "topic", kafkaTopic)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("Shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("Shutdown error", "error", err)
	}
	slog.Info("Server stopped")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
