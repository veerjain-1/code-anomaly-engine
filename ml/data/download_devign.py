"""
Download and preprocess the Devign vulnerability detection dataset.

Devign (Zhou et al., NeurIPS 2019) contains ~27K C/C++ functions
labeled as vulnerable (1) or clean (0). We tokenize with CodeBERT's
tokenizer and create train/val/test splits.

Usage:
    python data/download_devign.py [--output-dir ./data/processed]
"""

import argparse
import json
import os
from pathlib import Path

from datasets import load_dataset
from transformers import AutoTokenizer


MODEL_NAME = "microsoft/codebert-base"
MAX_LENGTH = 512


def download_and_preprocess(output_dir: str) -> None:
    """Download Devign dataset, tokenize, and save splits."""
    output_path = Path(output_dir)
    output_path.mkdir(parents=True, exist_ok=True)

    print(f"[1/4] Loading Devign dataset from HuggingFace Hub...")
    dataset = load_dataset("google/code_x_glue_cc_defect_detection")

    print(f"[2/4] Loading CodeBERT tokenizer ({MODEL_NAME})...")
    tokenizer = AutoTokenizer.from_pretrained(MODEL_NAME)

    # Save tokenizer for inference engine
    tokenizer_dir = output_path / "tokenizer"
    tokenizer.save_pretrained(str(tokenizer_dir))
    print(f"  → Tokenizer saved to {tokenizer_dir}")

    def tokenize_function(examples):
        """Tokenize code functions with truncation and padding."""
        return tokenizer(
            examples["func"],
            truncation=True,
            padding="max_length",
            max_length=MAX_LENGTH,
            return_tensors="pt",
        )

    print(f"[3/4] Tokenizing dataset (max_length={MAX_LENGTH})...")
    tokenized = dataset.map(
        tokenize_function,
        batched=True,
        remove_columns=["func", "id", "project", "commit_id"],
        desc="Tokenizing",
    )

    # Rename 'target' to 'labels' for HuggingFace Trainer compatibility
    tokenized = tokenized.rename_column("target", "labels")
    tokenized.set_format("torch")

    # Create val split from train (Devign only has train/test)
    if "validation" not in tokenized:
        split = tokenized["train"].train_test_split(test_size=0.1, seed=42)
        tokenized["train"] = split["train"]
        tokenized["validation"] = split["test"]

    print(f"[4/4] Saving processed splits...")
    for split_name in ["train", "validation", "test"]:
        split_path = output_path / split_name
        tokenized[split_name].save_to_disk(str(split_path))
        n = len(tokenized[split_name])
        # Count label distribution
        labels = tokenized[split_name]["labels"]
        n_pos = sum(1 for l in labels if l == 1)
        n_neg = n - n_pos
        print(f"  → {split_name}: {n} samples ({n_pos} vulnerable, {n_neg} clean)")

    # Save dataset stats
    stats = {
        "model_name": MODEL_NAME,
        "max_length": MAX_LENGTH,
        "splits": {
            split: len(tokenized[split])
            for split in ["train", "validation", "test"]
        },
    }
    stats_path = output_path / "stats.json"
    with open(stats_path, "w") as f:
        json.dump(stats, f, indent=2)
    print(f"\n✅ Dataset ready at {output_path}")
    print(f"   Stats: {json.dumps(stats['splits'])}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Download and preprocess Devign dataset")
    parser.add_argument(
        "--output-dir",
        default="data/processed",
        help="Directory to save processed dataset (default: data/processed)",
    )
    args = parser.parse_args()
    download_and_preprocess(args.output_dir)
