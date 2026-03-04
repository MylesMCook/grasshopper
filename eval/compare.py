#!/usr/bin/env python3
"""A/B comparison: run benchmark with and without HNSW/reranker.

Usage:
    python eval/compare.py [--db eval/data/eval.db] [--qa eval/data/qa_map.json]
"""

import argparse
import json
import subprocess
import sys
from pathlib import Path


def run_benchmark(db: str, qa: str, limit: int, output: str, label: str):
    """Run benchmark.py and return the results."""
    cmd = [
        sys.executable, "eval/benchmark.py",
        "--db", db, "--qa", qa, "--limit", str(limit), "--output", output,
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
    args = parser.parse_args()

    # Run baseline
    baseline = run_benchmark(
        args.db, args.qa, args.limit,
        "eval/results/baseline.json", "Baseline (FTS + RRF)"
    )

    if not baseline:
        sys.exit(1)

    # Print comparison table
    print(f"\n{'='*60}")
    print(f"COMPARISON")
    print(f"{'='*60}")

    agg = baseline["aggregate"]
    print(f"{'Metric':<30} {'Baseline':>10}")
    print(f"{'-'*30} {'-'*10}")
    print(f"{'Evidence Recall':<30} {agg['evidence_recall']:>10.4f}")
    print(f"{'Perfect Recall Rate':<30} {agg['perfect_recall_rate']:>10.4f}")
    print(f"{'Retrieval Failure Rate':<30} {agg['retrieval_failure_rate']:>10.4f}")

    print(f"\nBy Category:")
    for cat, data in baseline.get("by_category", {}).items():
        print(f"  {cat:<15}: recall={data['avg_recall']:.4f} ({data['n_questions']} questions)")


if __name__ == "__main__":
    main()
