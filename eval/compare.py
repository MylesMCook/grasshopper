#!/usr/bin/env python3
"""A/B comparison: FTS-only baseline vs full hybrid pipeline.

Runs benchmark.py twice — once with --mode fts, once with --mode hybrid —
then prints a side-by-side comparison table.

Usage:
    python eval/compare.py [--db eval/data/eval.db] [--qa eval/data/qa_map.json]
"""

import argparse
import json
import subprocess
import sys
from pathlib import Path


def run_benchmark(db: str, qa: str, limit: int, mode: str, output: str, binary: str, label: str):
    """Run benchmark.py and return the results."""
    cmd = [
        sys.executable, "eval/benchmark.py",
        "--db", db, "--qa", qa, "--limit", str(limit),
        "--mode", mode, "--output", output, "--binary", binary,
    ]
    print(f"\n{'='*60}")
    print(f"Running: {label}")
    print(f"{'='*60}")
    result = subprocess.run(cmd, capture_output=False, text=True)
    if result.returncode != 0:
        print(f"Benchmark failed for {label}", file=sys.stderr)
        return None

    with open(output) as f:
        return json.load(f)


def main():
    parser = argparse.ArgumentParser(description="A/B comparison of retrieval configs")
    parser.add_argument("--db", default="eval/data/eval.db", help="Database path")
    parser.add_argument("--qa", default="eval/data/qa_map.json", help="QA map file")
    parser.add_argument("--limit", type=int, default=10, help="Search result limit")
    parser.add_argument("--binary", default="grasshopper", help="Path to grasshopper binary")
    args = parser.parse_args()

    # Run FTS-only baseline
    baseline = run_benchmark(
        args.db, args.qa, args.limit, "fts",
        "eval/results/baseline-fts.json", args.binary, "FTS-only baseline (direct SQLite)"
    )

    # Run hybrid pipeline
    hybrid = run_benchmark(
        args.db, args.qa, args.limit, "hybrid",
        "eval/results/baseline-hybrid.json", args.binary, "Hybrid pipeline (grasshopper recall CLI)"
    )

    if not baseline:
        print("FTS baseline failed — cannot compare.", file=sys.stderr)
        sys.exit(1)

    # Print comparison table
    print(f"\n{'='*60}")
    print(f"COMPARISON (limit={args.limit})")
    print(f"{'='*60}")

    b = baseline["aggregate"]
    print(f"{'Metric':<30} {'FTS-only':>10}", end="")
    if hybrid:
        h = hybrid["aggregate"]
        print(f" {'Hybrid':>10} {'Delta':>10}")
    else:
        print(f" {'Hybrid':>10}")
        h = None

    print(f"{'-'*30} {'-'*10}", end="")
    if h:
        print(f" {'-'*10} {'-'*10}")
    else:
        print(f" {'-'*10}")

    metrics = [
        ("Evidence Recall", "evidence_recall"),
        ("Perfect Recall Rate", "perfect_recall_rate"),
        ("Retrieval Failure Rate", "retrieval_failure_rate"),
    ]

    for label, key in metrics:
        bv = b[key]
        print(f"{label:<30} {bv:>10.4f}", end="")
        if h:
            hv = h[key]
            delta = hv - bv
            sign = "+" if delta > 0 else ""
            print(f" {hv:>10.4f} {sign}{delta:>9.4f}")
        else:
            print(f" {'(failed)':>10}")

    print(f"\nBy Category:")
    cats = sorted(set(list(baseline.get("by_category", {}).keys()) + (list(hybrid.get("by_category", {}).keys()) if hybrid else [])))
    for cat in cats:
        b_cat = baseline.get("by_category", {}).get(cat, {})
        b_recall = b_cat.get("avg_recall", 0)
        n = b_cat.get("n_questions", 0)
        line = f"  {cat:<15} ({n:3}q): fts={b_recall:.4f}"
        if hybrid:
            h_cat = hybrid.get("by_category", {}).get(cat, {})
            h_recall = h_cat.get("avg_recall", 0)
            delta = h_recall - b_recall
            sign = "+" if delta > 0 else ""
            line += f"  hybrid={h_recall:.4f}  ({sign}{delta:.4f})"
        print(line)


if __name__ == "__main__":
    main()
