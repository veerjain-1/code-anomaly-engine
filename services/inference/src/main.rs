//! gRPC server entry point for the code anomaly inference engine.
//!
//! Serves the InferenceService gRPC API backed by an ONNX Runtime session.
//! Designed for sub-50ms p99 latency on single predictions.

mod engine;

use anyhow::Result;
use clap::Parser;
use std::net::SocketAddr;
use std::path::PathBuf;
use std::sync::Arc;
use tonic::{transport::Server, Request, Response, Status};
use tracing::{info, error};

// Generated gRPC code from proto/inference.proto
pub mod proto {
    tonic::include_proto!("inference");
}

use proto::inference_service_server::{InferenceService, InferenceServiceServer};
use proto::{
    HealthCheckRequest, HealthCheckResponse, PredictBatchRequest, PredictBatchResponse,
    PredictRequest, PredictResponse,
    health_check_response::Status as HealthStatus,
};

/// Command-line arguments for the inference server.
#[derive(Parser, Debug)]
#[command(name = "inference-server", about = "Code anomaly detection inference engine")]
struct Args {
    /// Path to the ONNX model file
    #[arg(long, default_value = "models/codebert_defect.onnx")]
    model: PathBuf,

    /// Path to the tokenizer directory
    #[arg(long, default_value = "models/tokenizer")]
    tokenizer: PathBuf,

    /// Maximum sequence length for tokenization
    #[arg(long, default_value_t = 512)]
    max_length: usize,

    /// gRPC server port
    #[arg(long, default_value_t = 50051)]
    port: u16,
}

/// gRPC service implementation backed by the ONNX engine.
struct InferenceServiceImpl {
    engine: Arc<engine::Engine>,
}

#[tonic::async_trait]
impl InferenceService for InferenceServiceImpl {
    async fn predict(
        &self,
        request: Request<PredictRequest>,
    ) -> Result<Response<PredictResponse>, Status> {
        let req = request.into_inner();

        if req.code.is_empty() {
            return Err(Status::invalid_argument("code field cannot be empty"));
        }

        let engine = self.engine.clone();
        let code = req.code.clone();

        // Run inference in a blocking task to avoid blocking the async runtime
        let result = tokio::task::spawn_blocking(move || engine.predict(&code))
            .await
            .map_err(|e| {
                error!("Inference task panicked: {}", e);
                Status::internal("Inference task failed")
            })?
            .map_err(|e| {
                error!("Inference error: {}", e);
                Status::internal(format!("Inference error: {}", e))
            })?;

        Ok(Response::new(PredictResponse {
            is_anomalous: result.is_anomalous,
            confidence: result.confidence,
            label: result.label,
            latency_ms: result.latency_ms,
            repo: req.repo,
            commit_sha: req.commit_sha,
            file_path: req.file_path,
            start_line: req.start_line,
            end_line: req.end_line,
        }))
    }

    async fn predict_batch(
        &self,
        request: Request<PredictBatchRequest>,
    ) -> Result<Response<PredictBatchResponse>, Status> {
        let req = request.into_inner();
        let start = std::time::Instant::now();

        let engine = self.engine.clone();
        let snippets = req.snippets;

        let results = tokio::task::spawn_blocking(move || {
            let codes: Vec<&str> = snippets.iter().map(|s| s.code.as_str()).collect();
            let predictions = engine.predict_batch(&codes)?;

            let responses: Vec<PredictResponse> = predictions
                .into_iter()
                .zip(snippets.iter())
                .map(|(pred, snippet)| PredictResponse {
                    is_anomalous: pred.is_anomalous,
                    confidence: pred.confidence,
                    label: pred.label,
                    latency_ms: pred.latency_ms,
                    repo: snippet.repo.clone(),
                    commit_sha: snippet.commit_sha.clone(),
                    file_path: snippet.file_path.clone(),
                    start_line: snippet.start_line,
                    end_line: snippet.end_line,
                })
                .collect();

            Ok::<Vec<PredictResponse>, anyhow::Error>(responses)
        })
        .await
        .map_err(|e| {
            error!("Batch inference task panicked: {}", e);
            Status::internal("Batch inference task failed")
        })?
        .map_err(|e| {
            error!("Batch inference error: {}", e);
            Status::internal(format!("Batch inference error: {}", e))
        })?;

        let total_latency_ms = start.elapsed().as_secs_f32() * 1000.0;

        Ok(Response::new(PredictBatchResponse {
            results,
            total_latency_ms,
        }))
    }

    async fn health_check(
        &self,
        _request: Request<HealthCheckRequest>,
    ) -> Result<Response<HealthCheckResponse>, Status> {
        let stats = self.engine.stats();

        Ok(Response::new(HealthCheckResponse {
            status: HealthStatus::Serving.into(),
            model_name: "codebert-defect-detection".to_string(),
            model_load_time_ms: stats.model_load_time_ms as i64,
            total_predictions: stats.total_predictions as i64,
            avg_latency_ms: stats.avg_latency_ms as f32,
        }))
    }
}

#[tokio::main]
async fn main() -> Result<()> {
    // Initialize tracing
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| "info".into()),
        )
        .json()
        .init();

    let args = Args::parse();

    info!(
        model = %args.model.display(),
        tokenizer = %args.tokenizer.display(),
        max_length = args.max_length,
        port = args.port,
        "Starting inference engine"
    );

    // Load model
    let engine = Arc::new(
        engine::Engine::new(&args.model, &args.tokenizer, args.max_length)
            .expect("Failed to load inference engine"),
    );

    let service = InferenceServiceImpl { engine };

    let addr: SocketAddr = format!("0.0.0.0:{}", args.port).parse()?;
    info!(%addr, "gRPC server listening");

    Server::builder()
        .add_service(InferenceServiceServer::new(service))
        .serve(addr)
        .await?;

    Ok(())
}
