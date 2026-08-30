.PHONY: all setup train export test build run clean

# ─── Top-level targets ───────────────────────────────────────

all: setup train export build

# ─── ML Pipeline ─────────────────────────────────────────────

setup:
	@echo "📦 Setting up Python environment..."
	cd ml && python3 -m venv .venv && \
		.venv/bin/pip install -e ".[dev]"

download-data:
	@echo "📊 Downloading Devign dataset..."
	cd ml && .venv/bin/python data/download_devign.py

train: download-data
	@echo "🧠 Training CodeBERT on Devign..."
	cd ml && .venv/bin/python train.py --epochs 1 --batch-size 16

export:
	@echo "📦 Exporting model to ONNX..."
	cd ml && .venv/bin/python export_onnx.py --quantize

evaluate:
	@echo "📊 Evaluating model..."
	cd ml && .venv/bin/python evaluate.py

# ─── Go Services ─────────────────────────────────────────────

build-ingestion:
	@echo "🔨 Building ingestion service..."
	cd services/ingestion && go build -o ../../bin/ingestion .

build-gateway:
	@echo "🔨 Building gateway service..."
	cd services/gateway && go build -o ../../bin/gateway .

test-go:
	@echo "🧪 Running Go tests..."
	cd services/ingestion && go test ./...
	cd services/gateway && go test ./...

# ─── Rust Inference Engine ───────────────────────────────────

build-inference:
	@echo "🦀 Building inference engine..."
	cd services/inference && cargo build --release

test-rust:
	@echo "🧪 Running Rust tests..."
	cd services/inference && cargo test

bench-rust:
	@echo "⚡ Benchmarking inference latency..."
	cd services/inference && cargo bench

# ─── Dashboard ───────────────────────────────────────────────

build-dashboard:
	@echo "⚛️  Building dashboard..."
	cd dashboard && npm install && npm run build

# ─── Docker ──────────────────────────────────────────────────

docker-up:
	@echo "🐳 Starting all services..."
	docker-compose up --build -d

docker-down:
	docker-compose down

docker-logs:
	docker-compose logs -f

# ─── Combined ────────────────────────────────────────────────

build: build-ingestion build-gateway build-inference build-dashboard

test: test-go test-rust
	@echo "✅ All tests passed!"

clean:
	rm -rf bin/ ml/.venv ml/data/processed ml/checkpoints
	cd services/inference && cargo clean
