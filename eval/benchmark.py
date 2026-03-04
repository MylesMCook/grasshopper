#!/usr/bin/env python3
"""Benchmark Grasshopper retrieval quality using LoCoMo QA pairs.

Two modes:
  fts    — Direct SQLite FTS5 search (fast, baseline)
  hybrid — Calls `grasshopper recall` CLI (tests full pipeline: FTS + vector + RRF + rerank)

For each question, checks if evidence dia_ids appear in retrieved content.

Usage:
    python eval/benchmark.py [--mode fts|hybrid] [--db eval/data/eval.db] [--qa eval/data/qa_map.json]
"""

import argparse
import json
import sqlite3
import subprocess
import sys
from collections import defaultdict
from pathlib import Path


CATEGORY_NAMES = {1: "single-hop", 2: "multi-hop", 3: "temporal", 4: "commonsense", 5: "adversarial"}


def fts_search(conn: sqlite3.Connection, query: str, limit: int = 10) -> list[dict]:
    """FTS5 search returning chunk id, title, content, score."""
    # Prepare query for FTS5 — simple tokenized approach
    tokens = query.split()
    # Use OR between tokens for recall
    fts_query = " OR ".join(f'"{t}"' for t in tokens if len(t) > 2)
    if not fts_query:
        fts_query = " OR ".join(f'"{t}"' for t in tokens)
    if not fts_query:
        return []

    try:
        rows = conn.execute(
            """SELECT c.id, c.title, c.content,
                      bm25(chunks_fts, 5.0, 1.0, 1.0, 5.0, 2.0) AS score
               FROM chunks_fts f
               JOIN chunks c ON c.id = f.rowid
               WHERE chunks_fts MATCH ?
               AND c.kind = 'memory'
               ORDER BY score ASC
               LIMIT ?""",
            (fts_query, limit),
        ).fetchall()
    except sqlite3.OperationalError:
        return []

    return [{"id": r[0], "title": r[1], "content": r[2], "score": -r[3]} for r in rows]


def hybrid_search(db_path: str, query: str, limit: int = 10, binary: str = "grasshopper") -> list[dict]:
    """Call `grasshopper recall` CLI and parse output, then load full content from DB."""
    try:
        result = subprocess.run(
            [binary, "--db", db_path, "recall", query, "--limit", str(limit)],
            capture_output=True, text=True, timeout=30,
        )
    except (subprocess.TimeoutExpired, FileNotFoundError):
        return []

    if result.returncode != 0:
        return []

    # Parse recall output to extract memory IDs from the numbered list
    # Format: "1. [0.1234] [episode] title (accessed: N, salience: 0.50)"
    hit_titles = []
    for line in result.stdout.strip().split("\n"):
        line = line.strip()
        if not line or line.startswith("No memories"):
            continue
        # Extract title between type bracket and " (accessed:"
        if "] " in line and " (accessed:" in line:
            # Find the second "] " (after score and type)
            parts = line.split("] ", 2)
            if len(parts) >= 3:
                title = parts[2].split(" (accessed:")[0]
                hit_titles.append(title)

    if not hit_titles:
        return []

    # Load full content from DB by title match
    conn = sqlite3.connect(db_path)
    results = []
    for title in hit_titles:
        row = conn.execute(
            "SELECT id, title, content FROM chunks WHERE kind='memory' AND title=? LIMIT 1",
            (title,),
        ).fetchone()
        if row:
            results.append({"id": row[0], "title": row[1], "content": row[2]})
    conn.close()
    return results


def evidence_in_results(evidence: list[str], results: list[dict]) -> dict:
    """Check which evidence dia_ids appear in retrieved content."""
    if not evidence:
        return {"found": 0, "total": 0, "recall": 1.0}

    found = 0
    for eid in evidence:
        for r in results:
            content = r.get("content", "")
            if f"[{eid}]" in content:
                found += 1
                break

    total = len(evidence)
    return {
        "found": found,
        "total": total,
        "recall": found / total if total > 0 else 0.0,
    }


def main():
    parser = argparse.ArgumentParser(description="Benchmark Grasshopper retrieval")
    parser.add_argument("--db", default="eval/data/eval.db", help="Database path")
    parser.add_argument("--qa", default="eval/data/qa_map.json", help="QA map file")
    parser.add_argument("--limit", type=int, default=10, help="Search result limit")
    parser.add_argument("--output", default=None, help="Output file (auto-named by mode if omitted)")
    parser.add_argument("--mode", choices=["fts", "hybrid"], default="fts",
                        help="fts = direct SQLite FTS (fast), hybrid = grasshopper recall CLI (full pipeline)")
    parser.add_argument("--binary", default="grasshopper", help="Path to grasshopper binary")
    args = parser.parse_args()

    if args.output is None:
        args.output = f"eval/results/benchmark-{args.mode}.json"

    qa_path = Path(args.qa)
    db_path = Path(args.db)

    if not qa_path.exists():
        print(f"Error: {qa_path} not found. Run ingest.py first.", file=sys.stderr)
        sys.exit(1)
    if not db_path.exists():
        print(f"Error: {db_path} not found. Run ingest.py first.", file=sys.stderr)
        sys.exit(1)

    conn = sqlite3.connect(str(db_path))
    total_memories = conn.execute("SELECT COUNT(*) FROM chunks WHERE kind='memory'").fetchone()[0]

    with open(qa_path) as f:
        qa_items = json.load(f)

    mode_label = "FTS-only (direct SQLite)" if args.mode == "fts" else "Hybrid (grasshopper recall CLI)"
    print(f"Benchmarking {len(qa_items)} questions against {db_path} ({total_memories} memories)")
    print(f"Mode: {mode_label}, limit: {args.limit}")

    results = []
    by_category = defaultdict(list)

    for i, qa in enumerate(qa_items):
        question = qa["question"]
        evidence = qa["evidence"]
        category = qa["category"]
        sample_id = qa["sample_id"]

        if args.mode == "fts":
            hits = fts_search(conn, question, args.limit)
        else:
            hits = hybrid_search(str(db_path), question, args.limit, args.binary)

        ev = evidence_in_results(evidence, hits)

        result = {
            "sample_id": sample_id,
            "question": question,
            "category": category,
            "category_name": CATEGORY_NAMES.get(category, "unknown"),
            "n_hits": len(hits),
            "evidence_found": ev["found"],
            "evidence_total": ev["total"],
            "evidence_recall": ev["recall"],
        }
        results.append(result)
        by_category[category].append(result)

        if (i + 1) % 100 == 0:
            print(f"  {i + 1}/{len(qa_items)} questions evaluated")

    if args.mode == "fts":
        conn.close()

    # Compute aggregates
    total = len(results)
    non_adversarial = [r for r in results if r["category"] != 5]
    avg_recall = sum(r["evidence_recall"] for r in non_adversarial) / len(non_adversarial) if non_adversarial else 0

    perfect_recall = sum(1 for r in non_adversarial if r["evidence_recall"] == 1.0)
    partial_recall = sum(1 for r in non_adversarial if 0 < r["evidence_recall"] < 1.0)
    retrieval_failures = sum(1 for r in non_adversarial if r["evidence_recall"] == 0.0)
    avg_hits = sum(r["n_hits"] for r in results) / total if total else 0

    print(f"\n{'='*60}")
    print(f"RESULTS — {mode_label} ({total} questions, limit={args.limit})")
    print(f"{'='*60}")
    print(f"Avg hits per query:                {avg_hits:.1f}")
    print(f"Evidence Recall (non-adversarial):  {avg_recall:.4f}")
    print(f"Perfect Recall (all evidence):      {perfect_recall}/{len(non_adversarial)} ({perfect_recall/len(non_adversarial)*100:.1f}%)")
    print(f"Partial Recall (some evidence):     {partial_recall}/{len(non_adversarial)} ({partial_recall/len(non_adversarial)*100:.1f}%)")
    print(f"Retrieval Failures (none found):    {retrieval_failures}/{len(non_adversarial)} ({retrieval_failures/len(non_adversarial)*100:.1f}%)")

    print(f"\nBy Category:")
    for cat in sorted(by_category.keys()):
        items = by_category[cat]
        cat_non_adv = [r for r in items if r["category"] != 5]
        if cat_non_adv:
            cat_recall = sum(r["evidence_recall"] for r in cat_non_adv) / len(cat_non_adv)
            cat_perfect = sum(1 for r in cat_non_adv if r["evidence_recall"] == 1.0)
            print(f"  {CATEGORY_NAMES.get(cat, '?'):15} ({len(items):3}q): recall={cat_recall:.4f}, perfect={cat_perfect}/{len(cat_non_adv)}")
        else:
            print(f"  {CATEGORY_NAMES.get(cat, '?'):15} ({len(items):3}q): adversarial (no evidence)")

    # Save results
    output_path = Path(args.output)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    summary = {
        "config": {
            "mode": args.mode,
            "db": str(db_path),
            "limit": args.limit,
            "n_questions": total,
            "n_memories": total_memories,
        },
        "aggregate": {
            "evidence_recall": avg_recall,
            "perfect_recall_rate": perfect_recall / len(non_adversarial) if non_adversarial else 0,
            "partial_recall_rate": partial_recall / len(non_adversarial) if non_adversarial else 0,
            "retrieval_failure_rate": retrieval_failures / len(non_adversarial) if non_adversarial else 0,
            "avg_hits_per_query": avg_hits,
        },
        "by_category": {
            CATEGORY_NAMES.get(cat, str(cat)): {
                "n_questions": len(items),
                "avg_recall": sum(r["evidence_recall"] for r in items if r["category"] != 5) / max(1, len([r for r in items if r["category"] != 5])),
            }
            for cat, items in sorted(by_category.items())
        },
        "details": results,
    }
    with open(output_path, "w") as f:
        json.dump(summary, f, indent=2)
    print(f"\nResults saved to {output_path}")


if __name__ == "__main__":
    main()
