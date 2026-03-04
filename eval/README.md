# Grasshopper Evaluation Harness

Measures retrieval quality using the [LoCoMo benchmark](https://github.com/snap-research/locomo) (10 long-term conversations, ~2,000 QA pairs).

## Setup

```sh
pip install -r eval/requirements.txt
```

## Run

### 1. Ingest LoCoMo conversations

Downloads `locomo10.json` and ingests raw 3-turn windows into a fresh Grasshopper DB:

```sh
python eval/ingest.py --download
```

This creates:
- `eval/data/eval.db` — Grasshopper database with all conversation windows
- `eval/data/qa_map.json` — Extracted QA pairs with evidence references

### 2. Run retrieval benchmark

```sh
python eval/benchmark.py
```

Evaluates each QA pair's evidence recall — whether the gold evidence `dia_id`s appear in retrieved results. No LLM judge needed.

### 3. Compare configurations

```sh
python eval/compare.py
```

Runs the benchmark with different configurations and outputs a comparison table.

## Metrics

- **Evidence Recall**: Fraction of gold evidence dia_ids found in top-k results
- **Perfect Recall Rate**: Questions where ALL evidence was found
- **Retrieval Failure Rate**: Questions where NO evidence was found

## Question Categories

| Cat | Name | Description |
|-----|------|-------------|
| 1 | Single-hop | One dialogue turn needed |
| 2 | Multi-hop | Multiple turns needed |
| 3 | Temporal | Time/sequence reasoning |
| 4 | Commonsense | External knowledge needed |
| 5 | Adversarial | Answer is "Not mentioned" |

## Target Metrics (Yuan et al.)

| Config | Accuracy |
|--------|----------|
| Basic RAG + Hybrid | ~73-77% |
| Basic RAG + Hybrid + Rerank | 81.1% |

Grasshopper's cognitive scoring is an additional signal not tested in the paper.
