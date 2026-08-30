//! ONNX Runtime inference engine for code anomaly detection.
//!
//! Manages the ONNX session lifecycle, handles tokenization via HuggingFace
//! tokenizers, and serves predictions with sub-50ms p99 latency.

use anyhow::{Context, Result};
use ndarray::Array2;
use ort::{session::Session, value::Value};
use std::path::Path;
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::Instant;
use tracing::{info, warn};

/// Result of a single code anomaly prediction.
#[derive(Debug, Clone)]
pub struct PredictionResult {
    /// Whether the code is predicted to be anomalous (vulnerable/defective).
    pub is_anomalous: bool,
    /// Confidence score [0.0, 1.0] — probability of being anomalous.
    pub confidence: f32,
    /// Human-readable label.
    pub label: String,
    /// Inference latency in milliseconds.
    pub latency_ms: f32,
}

/// ONNX inference engine that loads a CodeBERT model and serves predictions.
pub struct Engine {
    session: Session,
    tokenizer: tokenizers::Tokenizer,
    max_length: usize,
    total_predictions: AtomicU64,
    total_latency_us: AtomicU64,
    model_load_time_ms: u64,
}

impl Engine {
    /// Load the ONNX model and tokenizer from the given directory.
    ///
    /// # Arguments
    /// * `model_path` - Path to the `.onnx` model file
    /// * `tokenizer_path` - Path to the tokenizer directory (containing tokenizer.json)
    /// * `max_length` - Maximum sequence length for tokenization (default: 512)
    pub fn new(model_path: &Path, tokenizer_path: &Path, max_length: usize) -> Result<Self> {
        let start = Instant::now();

        info!("Loading ONNX model from {:?}", model_path);
        let session = Session::builder()
            .context("Failed to create ONNX session builder")?
            .with_intra_threads(4)
            .context("Failed to set intra-op threads")?
            .commit_from_file(model_path)
            .context("Failed to load ONNX model")?;

        info!("Loading tokenizer from {:?}", tokenizer_path);
        let tokenizer_file = tokenizer_path.join("tokenizer.json");
        let tokenizer = tokenizers::Tokenizer::from_file(&tokenizer_file)
            .map_err(|e| anyhow::anyhow!("Failed to load tokenizer: {}", e))?;

        let model_load_time_ms = start.elapsed().as_millis() as u64;
        info!(
            "Engine initialized in {}ms (model: {:?})",
            model_load_time_ms, model_path
        );

        Ok(Self {
            session,
            tokenizer,
            max_length,
            total_predictions: AtomicU64::new(0),
            total_latency_us: AtomicU64::new(0),
            model_load_time_ms,
        })
    }

    /// Run inference on a single code snippet.
    pub fn predict(&self, code: &str) -> Result<PredictionResult> {
        let start = Instant::now();

        // Tokenize
        let encoding = self
            .tokenizer
            .encode(code, true)
            .map_err(|e| anyhow::anyhow!("Tokenization failed: {}", e))?;

        let mut input_ids: Vec<i64> = encoding.get_ids().iter().map(|&id| id as i64).collect();
        let mut attention_mask: Vec<i64> =
            encoding.get_attention_mask().iter().map(|&m| m as i64).collect();

        // Truncate or pad to max_length
        input_ids.truncate(self.max_length);
        attention_mask.truncate(self.max_length);

        while input_ids.len() < self.max_length {
            input_ids.push(0); // PAD token
            attention_mask.push(0);
        }

        // Create ONNX input tensors
        let input_ids_array =
            Array2::from_shape_vec((1, self.max_length), input_ids)
                .context("Failed to create input_ids tensor")?;
        let attention_mask_array =
            Array2::from_shape_vec((1, self.max_length), attention_mask)
                .context("Failed to create attention_mask tensor")?;

        let input_ids_value = Value::from_array(input_ids_array)
            .context("Failed to create input_ids ONNX value")?;
        let attention_mask_value = Value::from_array(attention_mask_array)
            .context("Failed to create attention_mask ONNX value")?;

        // Run inference
        let outputs = self
            .session
            .run(ort::inputs![input_ids_value, attention_mask_value]?)
            .context("ONNX inference failed")?;

        // Extract logits [batch_size=1, num_labels=2]
        let logits_tensor = outputs[0]
            .try_extract_tensor::<f32>()
            .context("Failed to extract logits tensor")?;
        let logits: Vec<f32> = logits_tensor.iter().copied().collect();

        if logits.len() < 2 {
            warn!("Unexpected logits shape: {:?}", logits);
            anyhow::bail!("Expected 2 logits, got {}", logits.len());
        }

        // Softmax
        let max_logit = logits[0].max(logits[1]);
        let exp0 = (logits[0] - max_logit).exp();
        let exp1 = (logits[1] - max_logit).exp();
        let sum = exp0 + exp1;
        let prob_vulnerable = exp1 / sum;

        let latency = start.elapsed();
        let latency_ms = latency.as_secs_f32() * 1000.0;

        // Update stats
        self.total_predictions.fetch_add(1, Ordering::Relaxed);
        self.total_latency_us
            .fetch_add(latency.as_micros() as u64, Ordering::Relaxed);

        Ok(PredictionResult {
            is_anomalous: prob_vulnerable > 0.5,
            confidence: prob_vulnerable,
            label: if prob_vulnerable > 0.5 {
                "vulnerable".to_string()
            } else {
                "clean".to_string()
            },
            latency_ms,
        })
    }

    /// Run batch inference on multiple code snippets.
    pub fn predict_batch(&self, codes: &[&str]) -> Result<Vec<PredictionResult>> {
        // For now, run sequentially. Future optimization: batch tokenize + batch inference.
        codes.iter().map(|code| self.predict(code)).collect()
    }

    /// Get engine statistics.
    pub fn stats(&self) -> EngineStats {
        let total = self.total_predictions.load(Ordering::Relaxed);
        let total_latency = self.total_latency_us.load(Ordering::Relaxed);
        EngineStats {
            model_load_time_ms: self.model_load_time_ms,
            total_predictions: total,
            avg_latency_ms: if total > 0 {
                (total_latency as f64 / total as f64) / 1000.0
            } else {
                0.0
            },
        }
    }
}

/// Engine performance statistics.
#[derive(Debug)]
pub struct EngineStats {
    pub model_load_time_ms: u64,
    pub total_predictions: u64,
    pub avg_latency_ms: f64,
}
