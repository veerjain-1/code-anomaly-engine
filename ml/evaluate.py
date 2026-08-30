"""
Evaluate the trained CodeBERT model on the test set.

Prints classification report, confusion matrix, and ROC-AUC.

Usage:
    python evaluate.py [--checkpoint checkpoints/best]
"""

import argparse
import os

import torch
import numpy as np
from datasets import load_from_disk
from sklearn.metrics import (
    classification_report,
    confusion_matrix,
    roc_auc_score,
)
from torch.utils.data import DataLoader
from transformers import AutoModelForSequenceClassification
from tqdm import tqdm


DATA_DIR = "data/processed"


def collate_fn(batch):
    return {
        "input_ids": torch.stack([item["input_ids"] for item in batch]),
        "attention_mask": torch.stack([item["attention_mask"] for item in batch]),
        "labels": torch.tensor([item["labels"] for item in batch], dtype=torch.long),
    }


def evaluate(checkpoint_dir: str) -> None:
    """Evaluate model on test set."""
    device = torch.device("mps" if torch.backends.mps.is_available()
                          else "cuda" if torch.cuda.is_available() else "cpu")
    print(f"🖥  Device: {device}")

    print(f"Loading model from {checkpoint_dir}...")
    model = AutoModelForSequenceClassification.from_pretrained(
        checkpoint_dir, num_labels=2,
    )
    model.to(device)
    model.eval()

    print(f"Loading test dataset from {DATA_DIR}/test...")
    test_dataset = load_from_disk(os.path.join(DATA_DIR, "test"))
    test_dataset.set_format("torch", columns=["input_ids", "attention_mask", "labels"])
    test_loader = DataLoader(
        test_dataset, batch_size=32, shuffle=False, collate_fn=collate_fn,
    )
    print(f"  → {len(test_dataset)} test samples")

    all_preds = []
    all_labels = []
    all_probs = []

    with torch.no_grad():
        for batch in tqdm(test_loader, desc="Evaluating"):
            input_ids = batch["input_ids"].to(device)
            attention_mask = batch["attention_mask"].to(device)
            labels = batch["labels"]

            outputs = model(input_ids=input_ids, attention_mask=attention_mask)
            probs = torch.softmax(outputs.logits, dim=-1)[:, 1].cpu().numpy()
            preds = torch.argmax(outputs.logits, dim=-1).cpu().numpy()

            all_preds.extend(preds.tolist())
            all_labels.extend(labels.tolist())
            all_probs.extend(probs.tolist())

    print(f"\n{'='*60}")
    print("Classification Report:")
    print("="*60)
    print(classification_report(
        all_labels, all_preds,
        target_names=["Clean", "Vulnerable"],
        digits=4,
    ))

    print("Confusion Matrix:")
    cm = confusion_matrix(all_labels, all_preds)
    print(f"  TN={cm[0][0]:5d}  FP={cm[0][1]:5d}")
    print(f"  FN={cm[1][0]:5d}  TP={cm[1][1]:5d}")

    roc_auc = roc_auc_score(all_labels, all_probs)
    print(f"\nROC-AUC: {roc_auc:.4f}")
    print(f"{'='*60}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Evaluate CodeBERT on test set")
    parser.add_argument("--checkpoint", default="checkpoints/best")
    args = parser.parse_args()
    evaluate(args.checkpoint)
