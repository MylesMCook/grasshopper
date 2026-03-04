# Grasshopper

Unified agent brain — code intelligence (Ferret) + cognitive memory (Corpus) in one Rust binary.

## Design Philosophy: "Store Raw, Retrieve Smart"

Based on Yuan et al. (March 2026, arXiv:2603.02473v1):
- Retrieval method = 20pt accuracy swing. Write strategy = only 3-8pt.
- Raw chunks + hybrid search + reranking = best config (81.1% accuracy on LoCoMo)
- Invest compute in retrieval (reranker, query expansion), not write-time processing
- All models run locally via ONNX — zero external API calls

## Key Commands

```sh
cargo build                    # Build
cargo test                     # 53 tests across all modules
cargo run -- --help            # CLI help
cargo run -- remember "text"   # Store a memory
cargo run -- recall "query"    # Cognitive-scored memory search
cargo run -- stats             # Show counts
cargo run -- search "query"    # Hybrid search (code + memory)
cargo run -- index ./path      # Index code
cargo run -- serve --port 8106 # HTTP MCP server
```

## Architecture

```
src/
├── main.rs    — CLI with 11 subcommands
├── lib.rs     — Module exports
├── store.rs   — Unified SQLite schema, all queries (2144 lines)
├── memory.rs  — Cognitive layer: remember, recall, classify, score, reflect, consolidate (797 lines)
├── search.rs  — Hybrid search: FTS5 + vector → RRF fusion (→ cross-encoder rerank, planned)
├── index.rs   — Code indexing: scan → chunk → graph → FTS → embed (286 lines)
├── mcp.rs     — MCP server: 12 tools, HTTP + stdio transport (1092 lines)
└── rerank.rs  — Cross-encoder reranking via fastembed/ONNX (planned, LAB-32)
```

- Imports Ferret as library: chunk, embed, scan, graph, hnsw, tokenizer modules
- Does NOT use Ferret's `store` (hardcoded schema) or `mcp` (Ferret-specific tools)

### Retrieval Pipeline

```
FTS5 (BM25) → Vector (cosine) → RRF fusion (k=60) → [Rerank (planned)] → Cognitive scoring → Return
```

### Schema

Single `chunks` table with `kind` discriminator ('code' | 'memory'). Code chunks have file_path, language, symbol_name. Memory entries have memory_type, salience, access_count. Both share embeddings, content_hash, and graph edges.

Supporting tables: `codebases`, `indexed_files`, `graph` (code refs + Hebbian associations), `chunks_fts` (FTS5), `handoffs`.

## Key Dependencies

- `ferret` (local `../ferret`, `semantic` feature) — code intelligence, ONNX embeddings (Jina Code V2, 768-dim)
- `rusqlite` 0.32 — SQLite with FTS5 + bundled
- `rmcp` 0.16 — MCP server (stdio + streamable HTTP)
- `fastembed` — cross-encoder reranking via ONNX Runtime (planned, LAB-32)

## Conventions

- Lazy model init: `Arc<Mutex<Option<T>>>` pattern in mcp.rs for embedder (and future reranker)
- Database default: `~/.grasshopper/brain.db`
- Clippy lints: `perf = deny`, `redundant_clone = deny`
- All blocking operations (DB, embedder) run inside `spawn_blocking`
- Poisoned mutexes are recovered with a warning log, not panicked
- Ambiguous codebase references error with suggestions instead of silently picking one
- Design doc: Craft > Projects > Grasshopper: Unified Agent Brain

## Deployment

- Binary: `/usr/local/bin/grasshopper`
- Service: `grasshopper.service` on port 8106
- Auth proxy: `grasshopper-auth-proxy.service` on port 8107 (OAuth 2.1 for remote)
- Local MCP: `http://127.0.0.1:8106/mcp` (no auth, loopback bypass)
- Remote: `Internet → Cloudflare Tunnel → auth-proxy (:8107) → grasshopper (:8106)`

## Linear

- Project: **Grasshopper Next-Gen** (LAB-31+)
- Completed: LAB-26 (scaffold), LAB-27 (code intelligence), LAB-28 (cognitive memory), LAB-29 (MCP server), LAB-30 (hook-driven consolidation)
- Phase 1 — Retrieval Excellence: LAB-45 → LAB-32 → LAB-33, LAB-34 (parallel), LAB-35 (eval)
- Phase 2 — Proactive Intelligence: LAB-36 (parent), LAB-38 (relevance gate), LAB-39 (query expander), LAB-40 (contradiction detector), LAB-46 (working memory)
- Phase 3 — Learning & Evolution: LAB-37 (parent), LAB-41 (decay scorer), LAB-42 (consolidation), LAB-43 (entity graph), LAB-44 (fine-tuning pipeline)
