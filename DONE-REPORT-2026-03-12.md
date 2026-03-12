# Grasshopper Done Report — 2026-03-12 (UTC)

## Final status
- Implemented all planned retrieval refinements and related runtime fixes.
- Verified compile/test state locally.
- Re-ran benchmark suites and captured latest metrics.
- Deployed release binary and confirmed service health.

## Deployment status
- Service: `grasshopper.service` is active/running.
- Start time: 2026-03-12 01:10:07 UTC.
- Exec: `/usr/local/bin/grasshopper serve --port 8106`.
- Binary in use: `/usr/local/bin/grasshopper` (release build deployed).

## Code changes completed

### 1) Vector false-positive suppression
- File: `src/store/search.rs`
- Added:
  - `MIN_VECTOR_SIMILARITY_DEFAULT = 0.3`
  - `MIN_VECTOR_SIMILARITY_CODE = 0.48`
  - `min_vector_similarity(kind_filter)` helper
- Applied floor filtering in:
  - `vector_search(...)`
  - `vector_search_hnsw(...)`
- Effect: low-similarity code-vector matches are dropped before fusion.

### 2) Reranker behavior + code path optimization
- File: `src/search.rs`
- Set `RERANK_CANDIDATES` from `20` to `10`.
- Set `RERANK_MIN_SCORE` to `f32::NEG_INFINITY` (true no-op until calibrated).
- Added `RERANK_RANK_K = 60.0`.
- In `rerank_hits(...)`:
  - preserve raw cross-encoder logit in `hit.reranker_score`
  - set `hit.score` using rank-based positive RRF-scale value:
    - `1.0 / (RERANK_RANK_K + rank)`
- In `unified_search(...)`:
  - skip reranking when `kind_filter == Some("code")`.

### 3) Reranker model and cache-path hardening
- File: `src/rerank.rs`
- Switched model:
  - `BGERerankerBase` -> `JINARerankerV1TurboEn`
- Forced absolute cache dir:
  - `~/.fastembed_cache` via `with_cache_dir(...)`
- Purpose: prevent CWD-dependent reranker availability.

### 4) Memory decay regression fix
- File: `src/memory.rs`
- In `cognitive_score(...)`, applied decay floor:
  - if `reranker_score >= 0.3`: floor `0.5`
  - else: floor `0.15`
- Purpose: prevent old but relevant memories from collapsing to near-zero.

### 5) Embedder warm-up on MCP startup
- Files: `src/mcp/transport.rs`, `src/mcp/mod.rs`
- Added fire-and-forget `spawn_blocking(...)` warm-up in:
  - `run_stdio()`
  - `run_http()`
- Exposed for module access:
  - `pub(crate) embedder` field
  - `pub(crate) init_embedder_blocking(...)`

### 6) Reranker init observability
- File: `src/main.rs`
- `try_reranker()` now logs a warning on init failure instead of silently degrading.

### 7) Tests updated for similarity-floor semantics
- File: `src/store/mod.rs`
- Updated vector-search unit assertions from `len == 2` to `len == 1` where low-similarity test vectors should now be filtered.

## Runtime/cache correction performed
- Synced Jina reranker cache to home absolute cache path so all run modes share it:
  - source: `/home/myles/projects/grasshopper/.fastembed_cache/models--jinaai--jina-reranker-v1-turbo-en`
  - target: `/home/myles/.fastembed_cache/models--jinaai--jina-reranker-v1-turbo-en`
- Verified cache presence under `/home/myles/.fastembed_cache`.

## Verification executed
- `cargo check` -> pass
- `cargo test -q` -> pass (148 unit + 2 + 8 integration, no failures)
- CLI smoke:
  - code negative query returns no hits (`[]`, exit code 1 for no results)
  - memory query returns populated `reranker_score` values

## Benchmark results (latest artifacts)
- Artifacts directory: `bench/results/`
- Timestamps:
  - `accuracy-code.json`: 2026-03-11T23:18:46Z
  - `accuracy-memory.json`: 2026-03-11T23:19:52Z
  - `resources.json`: 2026-03-11T23:21:09Z

### accuracy-code
- Hybrid (no rerank):
  - MRR: `0.6469`
  - P@1: `0.5000`
  - NDCG@10: `0.5133`
  - Negative false positives: `0/2`
- Full pipeline:
  - same as Hybrid (code rerank is intentionally skipped)
- FTS-only MRR: `0.5687`

### accuracy-memory
- Hybrid:
  - Recall: `0.7376`
  - MRR: `0.7830`
  - P@1: `0.6563`
  - NDCG@10: `0.6610`
  - Identity leaks: `0`
  - Decay-test recall: `0.6667` (n=3)
- FTS-only recall: `0.5848`

### resources
- Baseline VmRSS: `11.80 MB`
- After embedder load: `230.91 MB`
- After embedding all chunks: `1364.48 MB`
- After HNSW build: `1376.30 MB`
- After loading reranker: `1560.08 MB`
- After 10 searches: `1560.35 MB`
- Reranker load time: `379 ms`

### search_latency (criterion medians)
- `full_pipeline/hybrid`: `17.048 ms`
- `full_pipeline/full_with_rerank`: `16.511 ms` (legacy benchmark label. The harness passed a reranker object into a code-only search, but `unified_search(...)` still skipped code reranking. This was not a representative production code-search path.)
- `full_pipeline/fts_only`: `1.944 ms`
- `reranking/rerank_10`: `608.382 ms`

## Success criteria check
- Zero false positives on negative code queries: **PASS** (`0/2`)
- Memory hybrid recall >= 64%: **PASS** (`73.76%`)
- Code MRR >= 0.598: **PASS** (`0.6469`)
- Decay-test recall >= 45%: **PASS** (`66.67%`)
- Search latency target (<3s memory, <100ms code): **PASS**
- Zero identity leaks: **PASS**

## Notes / caveats
- Working tree is currently dirty with additional pre-existing and in-flight changes; this report only covers completed retrieval work and verified benchmarks.
- Benchmark JSON structure uses `sections[*].metrics`; values above were extracted from those aggregates.
- The `search_latency` harness has since been corrected to benchmark truthful paths: `fts_only_code`, `hybrid_code`, and `memory_with_rerank`. This report preserves the historical artifact label above because the benchmark was not rerun as part of the follow-up cleanup.
