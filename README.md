# 🔍 Code Anomaly Detection Engine

> Real-time code vulnerability detection powered by a custom-trained CodeBERT model, served at sub-50ms latency through a Rust inference engine.

[![Go](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go)](https://go.dev)
[![Rust](https://img.shields.io/badge/Rust-1.78-DEA584?logo=rust)](https://www.rust-lang.org)
[![Python](https://img.shields.io/badge/Python-3.11-3776AB?logo=python)](https://python.org)
[![React](https://img.shields.io/badge/React-18-61DAFB?logo=react)](https://react.dev)

## Architecture

```
GitHub Webhook ──▶ Go Ingestion ──▶ Kafka ──▶ Go Gateway ──▶ Rust Inference Engine
                   (parse diffs)              (orchestrate)    (ONNX Runtime, <50ms)
                                                   │
                                                   ▼
                                            React Dashboard
                                          (WebSocket real-time)
```

### Tech Stack

| Component | Language | Purpose |
|---|---|---|
| **Ingestion Service** | Go | Webhook receiver, diff parser, Kafka producer |
| **API Gateway** | Go | REST API, WebSocket hub, Kafka consumer, gRPC client |
| **Inference Engine** | Rust | ONNX Runtime inference, gRPC server, <50ms p99 |
| **ML Training** | Python | Fine-tune CodeBERT on Devign vulnerability dataset |
| **Dashboard** | React/TS | Real-time anomaly feed, diff viewer, metrics charts |
| **Event Bus** | Kafka | Async message passing between services |
| **Observability** | Prometheus/Grafana | Latency histograms, throughput, error rates |

## How It Works

1. **Webhook** → GitHub sends push/PR events to the Go ingestion service
2. **Parse** → Diffs are parsed into function-level code snippets
3. **Queue** → Snippets are published to Kafka for reliable async processing
4. **Infer** → The Go gateway consumes from Kafka, calls the Rust inference engine via gRPC
5. **Predict** → The Rust engine tokenizes with CodeBERT tokenizer, runs ONNX inference, returns probability
6. **Stream** → Results are broadcast via WebSocket to the React dashboard in real-time

## ML Model

- **Base model**: [microsoft/codebert-base](https://huggingface.co/microsoft/codebert-base) (125M params)
- **Dataset**: [Devign](https://huggingface.co/datasets/google/code_x_glue_cc_defect_detection) — 27K C/C++ functions labeled vulnerable/clean
- **Task**: Binary classification (vulnerability detection)
- **Export**: ONNX format with INT8 quantization for optimized inference
- **Inference**: ONNX Runtime via Rust `ort` crate, <50ms p99 latency

## Quick Start

```bash
# 1. Train the model
make setup
make train
make export

# 2. Start all services
make docker-up

# 3. Open the dashboard
open http://localhost:3000
```

## Development

```bash
# Run Go tests
make test-go

# Run Rust tests
make test-rust

# Benchmark inference latency
make bench-rust

# Evaluate model metrics
make evaluate
```

## Project Structure

```
code-anomaly-engine/
├── services/
│   ├── ingestion/       # Go — webhook + diff parser
│   ├── gateway/         # Go — REST API + WebSocket + Kafka
│   └── inference/       # Rust — ONNX inference engine
├── ml/                  # Python — CodeBERT training pipeline
├── dashboard/           # React — real-time anomaly dashboard
├── proto/               # Shared gRPC protobuf definitions
├── infra/               # Prometheus, Grafana configs
├── docker-compose.yml   # Full-stack orchestration
└── Makefile             # Build automation
```

## License

MIT
