"""
Fine-tune CodeBERT on the Devign vulnerability detection dataset.

This trains a binary classifier (vulnerable vs. clean) on top of
microsoft/codebert-base using the preprocessed Devign dataset.

Usage:
    python train.py [--epochs 5] [--batch-size 16] [--lr 2e-5]
"""

import argparse
import json
import os
import time
from pathlib import Path

import torch
from datasets import load_from_disk
from sklearn.metrics import (
    accuracy_score,
    f1_score,
    precision_score,
    recall_score,
)
from torch.utils.data import DataLoader
from transformers import (
    AutoModelForSequenceClassification,
    AutoTokenizer,
    get_linear_schedule_with_warmup,
)
from tqdm import tqdm


MODEL_NAME = "microsoft/codebert-base"
DATA_DIR = "data/processed"
CHECKPOINT_DIR = "checkpoints"


def get_device() -> torch.device:
    """Select best available device: MPS (Apple Silicon) > CUDA > CPU."""
    if torch.backends.mps.is_available():
        return torch.device("mps")
    elif torch.cuda.is_available():
        return torch.device("cuda")
    return torch.device("cpu")


def compute_metrics(preds, labels):
    """Compute classification metrics."""
    return {
        "accuracy": accuracy_score(labels, preds),
        "f1": f1_score(labels, preds, zero_division=0),
        "precision": precision_score(labels, preds, zero_division=0),
        "recall": recall_score(labels, preds, zero_division=0),
    }


def collate_fn(batch):
    """Custom collate to handle torch dataset format."""
    return {
        "input_ids": torch.stack([item["input_ids"] for item in batch]),
        "attention_mask": torch.stack([item["attention_mask"] for item in batch]),
        "labels": torch.tensor([item["labels"] for item in batch], dtype=torch.long),
    }


def train(
    epochs: int = 5,
    batch_size: int = 16,
    lr: float = 2e-5,
    warmup_ratio: float = 0.1,
    max_grad_norm: float = 1.0,
) -> None:
    """Fine-tune CodeBERT on Devign."""
    device = get_device()
    print(f"🖥  Device: {device}")
    print(f"📊 Config: epochs={epochs}, batch_size={batch_size}, lr={lr}")

    # Load preprocessed data
    print(f"\n[1/5] Loading preprocessed dataset from {DATA_DIR}...")
    train_dataset = load_from_disk(os.path.join(DATA_DIR, "train"))
    val_dataset = load_from_disk(os.path.join(DATA_DIR, "validation"))

    train_dataset.set_format("torch", columns=["input_ids", "attention_mask", "labels"])
    val_dataset.set_format("torch", columns=["input_ids", "attention_mask", "labels"])

    train_loader = DataLoader(
        train_dataset, batch_size=batch_size, shuffle=True, collate_fn=collate_fn,
        num_workers=0, pin_memory=(device.type != "cpu"),
    )
    val_loader = DataLoader(
        val_dataset, batch_size=batch_size, shuffle=False, collate_fn=collate_fn,
        num_workers=0, pin_memory=(device.type != "cpu"),
    )

    print(f"  → Train: {len(train_dataset)} samples ({len(train_loader)} batches)")
    print(f"  → Val:   {len(val_dataset)} samples ({len(val_loader)} batches)")

    # Load model
    print(f"\n[2/5] Loading {MODEL_NAME}...")
    model = AutoModelForSequenceClassification.from_pretrained(
        MODEL_NAME,
        num_labels=2,
        problem_type="single_label_classification",
    )
    model.to(device)

    # Optimizer + scheduler
    print(f"\n[3/5] Setting up optimizer...")
    optimizer = torch.optim.AdamW(model.parameters(), lr=lr, weight_decay=0.01)
    total_steps = len(train_loader) * epochs
    warmup_steps = int(total_steps * warmup_ratio)
    scheduler = get_linear_schedule_with_warmup(
        optimizer, num_warmup_steps=warmup_steps, num_training_steps=total_steps,
    )

    # Training loop
    print(f"\n[4/5] Training for {epochs} epochs...")
    best_f1 = 0.0
    history = []
    checkpoint_path = Path(CHECKPOINT_DIR)
    checkpoint_path.mkdir(parents=True, exist_ok=True)

    for epoch in range(epochs):
        # --- Train ---
        model.train()
        total_loss = 0.0
        train_preds, train_labels = [], []

        pbar = tqdm(train_loader, desc=f"Epoch {epoch+1}/{epochs} [Train]")
        for batch in pbar:
            input_ids = batch["input_ids"].to(device)
            attention_mask = batch["attention_mask"].to(device)
            labels = batch["labels"].to(device)

            outputs = model(
                input_ids=input_ids,
                attention_mask=attention_mask,
                labels=labels,
            )
            loss = outputs.loss

            loss.backward()
            torch.nn.utils.clip_grad_norm_(model.parameters(), max_grad_norm)
            optimizer.step()
            scheduler.step()
            optimizer.zero_grad()

            total_loss += loss.item()
            preds = torch.argmax(outputs.logits, dim=-1)
            train_preds.extend(preds.cpu().tolist())
            train_labels.extend(labels.cpu().tolist())

            pbar.set_postfix({"loss": f"{loss.item():.4f}"})

        avg_train_loss = total_loss / len(train_loader)
        train_metrics = compute_metrics(train_preds, train_labels)

        # --- Validate ---
        model.eval()
        val_preds, val_labels = [], []
        val_loss = 0.0

        with torch.no_grad():
            for batch in tqdm(val_loader, desc=f"Epoch {epoch+1}/{epochs} [Val]"):
                input_ids = batch["input_ids"].to(device)
                attention_mask = batch["attention_mask"].to(device)
                labels = batch["labels"].to(device)

                outputs = model(
                    input_ids=input_ids,
                    attention_mask=attention_mask,
                    labels=labels,
                )
                val_loss += outputs.loss.item()
                preds = torch.argmax(outputs.logits, dim=-1)
                val_preds.extend(preds.cpu().tolist())
                val_labels.extend(labels.cpu().tolist())

        avg_val_loss = val_loss / len(val_loader)
        val_metrics = compute_metrics(val_preds, val_labels)

        epoch_result = {
            "epoch": epoch + 1,
            "train_loss": avg_train_loss,
            "val_loss": avg_val_loss,
            "train": train_metrics,
            "val": val_metrics,
        }
        history.append(epoch_result)

        print(f"\n  Epoch {epoch+1}/{epochs}:")
        print(f"    Train — loss: {avg_train_loss:.4f}, F1: {train_metrics['f1']:.4f}, acc: {train_metrics['accuracy']:.4f}")
        print(f"    Val   — loss: {avg_val_loss:.4f}, F1: {val_metrics['f1']:.4f}, acc: {val_metrics['accuracy']:.4f}")

        # Save best model
        if val_metrics["f1"] > best_f1:
            best_f1 = val_metrics["f1"]
            best_path = checkpoint_path / "best"
            model.save_pretrained(str(best_path))
            print(f"    ✅ New best model saved (F1={best_f1:.4f}) → {best_path}")

    # Save training history
    print(f"\n[5/5] Saving training history...")
    history_path = checkpoint_path / "training_history.json"
    with open(history_path, "w") as f:
        json.dump(history, f, indent=2)

    print(f"\n{'='*60}")
    print(f"🏁 Training complete!")
    print(f"   Best Val F1: {best_f1:.4f}")
    print(f"   Checkpoint:  {checkpoint_path / 'best'}")
    print(f"   History:     {history_path}")
    print(f"{'='*60}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Fine-tune CodeBERT on Devign")
    parser.add_argument("--epochs", type=int, default=5)
    parser.add_argument("--batch-size", type=int, default=16)
    parser.add_argument("--lr", type=float, default=2e-5)
    args = parser.parse_args()
    train(epochs=args.epochs, batch_size=args.batch_size, lr=args.lr)
