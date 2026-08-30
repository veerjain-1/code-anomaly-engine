// Package kafka provides a Kafka consumer that reads code snippets,
// forwards them to the Rust inference engine via gRPC, and publishes
// anomaly results back for the WebSocket feed.
package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	kgo "github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/veerjain-1/code-anomaly-engine/services/gateway/internal/proto"
)

// CodeSnippet mirrors the ingestion service's snippet format.
type CodeSnippet struct {
	Code      string `json:"code"`
	Repo      string `json:"repo"`
	CommitSHA string `json:"commit_sha"`
	FilePath  string `json:"file_path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Timestamp int64  `json:"timestamp"`
}

// AnomalyResult is the enriched result after inference.
type AnomalyResult struct {
	ID          string  `json:"id"`
	Repo        string  `json:"repo"`
	CommitSHA   string  `json:"commit_sha"`
	FilePath    string  `json:"file_path"`
	StartLine   int     `json:"start_line"`
	EndLine     int     `json:"end_line"`
	Code        string  `json:"code"`
	IsAnomalous bool    `json:"is_anomalous"`
	Confidence  float32 `json:"confidence"`
	Label       string  `json:"label"`
	LatencyMs   float32 `json:"latency_ms"`
	Timestamp   int64   `json:"timestamp"`
}

// ResultHandler is called when an inference result is ready.
type ResultHandler func(result AnomalyResult)

// Bridge consumes from Kafka, calls the inference engine, and emits results.
type Bridge struct {
	client        *kgo.Client
	grpcConn      *grpc.ClientConn
	inferClient   pb.InferenceServiceClient
	onResult      ResultHandler
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	resultCounter int64
	mu            sync.Mutex
}

// NewBridge creates a new Kafka→gRPC bridge.
func NewBridge(kafkaBroker, topic, inferenceAddr string, handler ResultHandler) (*Bridge, error) {
	// Kafka consumer
	client, err := kgo.NewClient(
		kgo.SeedBrokers(kafkaBroker),
		kgo.ConsumeTopics(topic),
		kgo.ConsumerGroup("gateway-inference"),
	)
	if err != nil {
		return nil, fmt.Errorf("create kafka consumer: %w", err)
	}

	// gRPC connection to inference engine
	conn, err := grpc.NewClient(
		inferenceAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("connect to inference engine at %s: %w", inferenceAddr, err)
	}

	inferClient := pb.NewInferenceServiceClient(conn)

	slog.Info("Bridge initialized",
		"kafka_broker", kafkaBroker,
		"topic", topic,
		"inference_addr", inferenceAddr,
	)

	return &Bridge{
		client:      client,
		grpcConn:    conn,
		inferClient: inferClient,
		onResult:    handler,
	}, nil
}

// Start begins consuming from Kafka and processing snippets.
func (b *Bridge) Start(ctx context.Context) {
	ctx, b.cancel = context.WithCancel(ctx)

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		b.consumeLoop(ctx)
	}()

	slog.Info("Bridge consumer started")
}

// Stop gracefully shuts down the bridge.
func (b *Bridge) Stop() {
	if b.cancel != nil {
		b.cancel()
	}
	b.wg.Wait()
	b.grpcConn.Close()
	b.client.Close()
	slog.Info("Bridge stopped", "total_processed", b.resultCounter)
}

func (b *Bridge) consumeLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		fetches := b.client.PollFetches(ctx)
		if errs := fetches.Errors(); len(errs) > 0 {
			for _, err := range errs {
				slog.Error("Kafka fetch error", "topic", err.Topic, "partition", err.Partition, "error", err.Err)
			}
			continue
		}

		fetches.EachRecord(func(record *kgo.Record) {
			b.processRecord(ctx, record)
		})
	}
}

func (b *Bridge) processRecord(ctx context.Context, record *kgo.Record) {
	var snippet CodeSnippet
	if err := json.Unmarshal(record.Value, &snippet); err != nil {
		slog.Error("Failed to unmarshal snippet", "error", err, "offset", record.Offset)
		return
	}

	// Skip empty code (will be enriched later via GitHub API)
	if snippet.Code == "" {
		slog.Debug("Skipping snippet with empty code", "file", snippet.FilePath)
		return
	}

	// Call inference engine
	resp, err := b.inferClient.Predict(ctx, &pb.PredictRequest{
		Code:      snippet.Code,
		Repo:      snippet.Repo,
		CommitSha: snippet.CommitSHA,
		FilePath:  snippet.FilePath,
		StartLine: int32(snippet.StartLine),
		EndLine:   int32(snippet.EndLine),
	})
	if err != nil {
		slog.Error("Inference failed", "error", err, "file", snippet.FilePath)
		return
	}

	b.mu.Lock()
	b.resultCounter++
	id := fmt.Sprintf("anomaly-%d", b.resultCounter)
	b.mu.Unlock()

	result := AnomalyResult{
		ID:          id,
		Repo:        snippet.Repo,
		CommitSHA:   snippet.CommitSHA,
		FilePath:    snippet.FilePath,
		StartLine:   snippet.StartLine,
		EndLine:     snippet.EndLine,
		Code:        snippet.Code,
		IsAnomalous: resp.IsAnomalous,
		Confidence:  resp.Confidence,
		Label:       resp.Label,
		LatencyMs:   resp.LatencyMs,
		Timestamp:   time.Now().UnixMilli(),
	}

	if b.onResult != nil {
		b.onResult(result)
	}

	slog.Info("Inference complete",
		"id", id,
		"file", snippet.FilePath,
		"label", resp.Label,
		"confidence", fmt.Sprintf("%.3f", resp.Confidence),
		"latency_ms", fmt.Sprintf("%.1f", resp.LatencyMs),
	)
}
