# Grasshopper

Persistent retrieval engine for AI agents — code intelligence + cognitive memory in one self-contained Rust binary.

## Design Philosophy: "Store Raw, Retrieve Smart"

Based on Yuan et al. (March 2026, arXiv:2603.02473v1) and Maharana et al. (2024, arXiv:2402.17753v1):
- Retrieval method = 20pt accuracy swing. Write strategy = only 3-8pt.
- Raw chunks + hybrid search + reranking = best config (81.1% accuracy on LoCoMo)
- Invest compute in retrieval (reranker, query expansion), not write-time processing
- All models run locally via ONNX — zero external API calls
- Research papers in `/home/myles/misc/` (2402.17753v1.pdf = LoCoMo benchmark, 2603.02473v1.pdf = retrieval vs write)

## Key Commands

```sh
cargo build                                  # Build
cargo test                                   # ~157 tests
cargo run -- --help                          # CLI help
cargo run -- index ./path                    # Index code
cargo run -- store "text"                    # Store a memory
cargo run -- search "query"                  # Hybrid search (code + memory)
cargo run -- search "query" --mode navigate  # Symbol definitions + references
cargo run -- search "query" --mode map       # Token-budgeted codebase overview
cargo run -- search "query" --mode impact    # Blast radius analysis
cargo run -- serve --port 8106               # HTTP MCP server
cargo run -- serve --stdio                   # stdio MCP (for Claude Code)
```

## Architecture

```
src/
├── main.rs     — CLI with 4 subcommands (index, store, search, serve)
├── lib.rs      — Module exports
├── store.rs    — Unified SQLite schema, all queries, auto-maintenance
├── memory.rs   — Cognitive layer: store, recall, cognitive_score (salience + decay)
├── search.rs   — Hybrid search: FTS5 + vector → RRF fusion → query expansion → cross-encoder rerank → unified_search
├── index.rs    — Code indexing: scan → chunk → graph → FTS → embed
├── mcp.rs      — MCP server: 3 tools (index, store, search), HTTP + stdio transport, embedding cache
├── rerank.rs   — Cross-encoder reranking via fastembed/ONNX (BAAI/bge-reranker-base)
└── code/       — Code intelligence primitives (self-contained)
    ├── chunk.rs      — Tree-sitter code chunking (13 languages)
    ├── embed.rs      — ONNX embeddings (Jina Code V2, 768-dim)
    ├── scan.rs       — Directory walking + SHA-256 hashing
    ├── tokenizer.rs  — CamelCase splitting for FTS5
    ├── graph.rs      — Tag extraction via tree-sitter TAGS_QUERY
    └── hnsw.rs       — HNSW approximate nearest neighbor (instant-distance)
```

### Retrieval Pipeline

```
Query Expansion → FTS5 (BM25) → Vector (cosine, HNSW) → RRF fusion (k=60) → Cross-encoder rerank → Relevance gate → Cognitive scoring (salience × decay) → Budget truncation → Return
```

### Schema

Single `chunks` table with `kind` discriminator ('code' | 'memory'). Code chunks have file_path, language, symbol_name. Memory entries have memory_type, salience, access_count. Both share embeddings, content_hash, and graph edges.

Supporting tables: `codebases`, `indexed_files`, `graph` (code refs), `chunks_fts` (FTS5), `handoffs`, `access_log`, `retrieval_log` (query logging), `feedback` (user signal on retrieval quality).

## Key Dependencies

- `tree-sitter` 0.25 + 13 language grammars — code parsing and tag extraction
- `ort` 2.0.0-rc.11 + `tokenizers` 0.22 — ONNX embeddings (Jina Code V2, 768-dim)
- `instant-distance` 0.6 — HNSW approximate nearest neighbor search
- `rusqlite` 0.32 — SQLite with FTS5 + bundled
- `rmcp` 0.16 — MCP server (stdio + streamable HTTP)
- `fastembed` — cross-encoder reranking via ONNX Runtime (BAAI/bge-reranker-base)

## Conventions

- Lazy model init: `Arc<Mutex<Option<T>>>` pattern in mcp.rs for embedder (and future reranker)
- Database default: `~/.grasshopper/brain.db`
- Model cache: `~/.cache/grasshopper/models/`
- Clippy lints: `perf = deny`, `redundant_clone = deny`
- All blocking operations (DB, embedder) run inside `spawn_blocking`
- Poisoned mutexes are recovered with a warning log, not panicked
- Ambiguous codebase references error with suggestions instead of silently picking one
- Design doc: Craft > Projects > Grasshopper: Persistent Retrieval Engine

## Deployment

- Binary: `/usr/local/bin/grasshopper`
- Service: `grasshopper.service` on port 8106
- Auth proxy: `grasshopper-auth-proxy.service` on port 8107 (OAuth 2.1 for remote)
- Local MCP: `http://127.0.0.1:8106/mcp` (no auth, loopback bypass)
- Remote: `Internet → Cloudflare Tunnel → auth-proxy (:8107) → grasshopper (:8106)`

## Linear

- Project: **Grasshopper** (Lab team)
- v1 phases complete: Retrieval Excellence, Proactive Intelligence, Learning & Evolution, Production Hardening (LAB-26 through LAB-81)
- v2 redesign: LAB-82 (parent) — tool collapse (18→3), CLI collapse (14→4), simplified cognitive scoring, auto maintenance
