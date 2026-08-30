"""
Export trained CodeBERT model to ONNX format for the Rust inference engine.

Exports the best checkpoint to ONNX with:
  - Dynamic sequence length axis
  - Optional INT8 quantization for faster inference
  - Validation against the original PyTorch model

Usage:
    python export_onnx.py [--checkpoint checkpoints/best] [--quantize]
"""

import argparse
import time
from pathlib import Path

import numpy as np
import onnx
import onnxruntime as ort
import torch
from transformers import AutoModelForSequenceClassification, AutoTokenizer


MODEL_NAME = "microsoft/codebert-base"
OUTPUT_DIR = "../services/inference/models"


def export_to_onnx(checkpoint_dir: str, quantize: bool = False) -> None:
    """Export trained model to ONNX format."""
    checkpoint_path = Path(checkpoint_dir)
    output_path = Path(OUTPUT_DIR)
    output_path.mkdir(parents=True, exist_ok=True)

    if checkpoint_path.exists():
        print(f"[1/5] Loading trained model from {checkpoint_path}...")
        model = AutoModelForSequenceClassification.from_pretrained(
            str(checkpoint_path), num_labels=2,
        )
    else:
        print(f"[1/5] Checkpoint {checkpoint_path} not found. Loading base model {MODEL_NAME}...")
        model = AutoModelForSequenceClassification.from_pretrained(
            MODEL_NAME, num_labels=2,
        )
    model.eval()

    tokenizer = AutoTokenizer.from_pretrained(MODEL_NAME)

    # Create dummy input for tracing
    print(f"[2/5] Creating dummy input for ONNX tracing...")
    dummy_code = "int main() { char buf[10]; gets(buf); return 0; }"
    inputs = tokenizer(
        dummy_code,
        return_tensors="pt",
        padding="max_length",
        truncation=True,
        max_length=512,
    )

    # Export to ONNX
    onnx_path = output_path / "codebert_defect.onnx"
    print(f"[3/5] Exporting to ONNX → {onnx_path}...")

    torch.onnx.export(
        model,
        (inputs["input_ids"], inputs["attention_mask"]),
        str(onnx_path),
        input_names=["input_ids", "attention_mask"],
        output_names=["logits"],
        dynamic_axes={
            "input_ids": {0: "batch_size", 1: "sequence_length"},
            "attention_mask": {0: "batch_size", 1: "sequence_length"},
            "logits": {0: "batch_size"},
        },
        opset_version=17,
        do_constant_folding=True,
    )

    # Validate ONNX model
    print(f"[4/5] Validating ONNX model...")
    onnx_model = onnx.load(str(onnx_path))
    onnx.checker.check_model(onnx_model)

    # Compare PyTorch vs ONNX output
    session = ort.InferenceSession(str(onnx_path))
    ort_inputs = {
        "input_ids": inputs["input_ids"].numpy(),
        "attention_mask": inputs["attention_mask"].numpy(),
    }

    with torch.no_grad():
        pt_logits = model(**inputs).logits.numpy()
    ort_logits = session.run(["logits"], ort_inputs)[0]

    max_diff = np.max(np.abs(pt_logits - ort_logits))
    print(f"  → Max output difference (PT vs ONNX): {max_diff:.6f}")
    assert max_diff < 1e-4, f"Output mismatch too large: {max_diff}"
    print(f"  ✅ Validation passed!")

    # Optional quantization
    if quantize:
        from onnxruntime.quantization import quantize_dynamic, QuantType
        quantized_path = output_path / "codebert_defect_quantized.onnx"
        print(f"  → Quantizing to INT8 → {quantized_path}...")
        quantize_dynamic(
            str(onnx_path),
            str(quantized_path),
            weight_type=QuantType.QInt8,
        )
        orig_size = onnx_path.stat().st_size / 1024 / 1024
        quant_size = quantized_path.stat().st_size / 1024 / 1024
        print(f"  → Original: {orig_size:.1f}MB, Quantized: {quant_size:.1f}MB ({quant_size/orig_size*100:.0f}%)")

    # Benchmark inference latency
    print(f"\n[5/5] Benchmarking inference latency...")
    latencies = []
    for _ in range(100):
        start = time.perf_counter()
        session.run(["logits"], ort_inputs)
        latencies.append((time.perf_counter() - start) * 1000)

    latencies.sort()
    p50 = latencies[49]
    p95 = latencies[94]
    p99 = latencies[98]
    print(f"  → p50: {p50:.2f}ms, p95: {p95:.2f}ms, p99: {p99:.2f}ms")

    model_size = onnx_path.stat().st_size / 1024 / 1024
    print(f"\n{'='*60}")
    print(f"🏁 Export complete!")
    print(f"   Model: {onnx_path} ({model_size:.1f}MB)")
    print(f"   Latency: p50={p50:.2f}ms, p95={p95:.2f}ms, p99={p99:.2f}ms")
    print(f"{'='*60}")

    # Also copy tokenizer to inference models dir for the Rust engine
    tokenizer.save_pretrained(str(output_path / "tokenizer"))
    print(f"   Tokenizer saved to {output_path / 'tokenizer'}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Export CodeBERT to ONNX")
    parser.add_argument("--checkpoint", default="checkpoints/best")
    parser.add_argument("--quantize", action="store_true")
    args = parser.parse_args()
    export_to_onnx(args.checkpoint, args.quantize)
