// Package kafka provides a Kafka producer for publishing code snippets
// to the message bus for downstream ML inference processing.
package kafka

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	kgo "github.com/twmb/franz-go/pkg/kgo"
)

// Producer wraps a Kafka client for publishing code snippet messages.
type Producer struct {
	client *kgo.Client
	topic  string
	mu     sync.Mutex
	count  int64
}

// NewProducer creates a new Kafka producer connected to the given broker.
func NewProducer(broker string, topic string) (*Producer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(broker),
		kgo.DefaultProduceTopic(topic),
		kgo.ProducerBatchCompression(kgo.SnappyCompression()),
	)
	if err != nil {
		return nil, fmt.Errorf("create kafka client: %w", err)
	}

	// Ping broker to verify connectivity
	if err := client.Ping(context.Background()); err != nil {
		client.Close()
		return nil, fmt.Errorf("ping kafka broker %s: %w", broker, err)
	}

	slog.Info("Kafka producer connected", "broker", broker, "topic", topic)
	return &Producer{client: client, topic: topic}, nil
}

// Publish sends a message to Kafka asynchronously.
func (p *Producer) Publish(ctx context.Context, value []byte) error {
	record := &kgo.Record{
		Value: value,
	}

	// Use synchronous produce for reliability
	results := p.client.ProduceSync(ctx, record)
	if err := results.FirstErr(); err != nil {
		return fmt.Errorf("produce message: %w", err)
	}

	p.mu.Lock()
	p.count++
	p.mu.Unlock()

	return nil
}

// Count returns the total number of messages published.
func (p *Producer) Count() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.count
}

// Close shuts down the Kafka producer.
func (p *Producer) Close() {
	p.client.Close()
	slog.Info("Kafka producer closed", "total_messages", p.count)
}
