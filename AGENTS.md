# AGENTS.md

Canonical instructions for agents working in this repository.

## Design Philosophy

Store raw data and retrieve intelligently.

The project is built around the finding that retrieval quality dominates write-time embellishment for long-term memory systems. Grasshopper keeps storage simple and spends computation on retrieval quality: hybrid search, reranking, and memory scoring. All models run locally through ONNX.

## Key Commands

```sh
cargo build                                  # Build
cargo test                                   # Run tests
cargo run -- --help                          # CLI help
cargo run -- index ./path                    # Index code
cargo run -- store "text"                    # Store a memory
cargo run -- search "query"                  # Hybrid search
cargo run -- search "query" --mode navigate  # Symbol definitions and references
cargo run -- search "query" --mode map       # Token-budgeted codebase overview
cargo run -- search "query" --mode impact    # Blast-radius analysis
cargo run -- status                          # Database health dashboard
cargo run -- get 42                          # Inspect a chunk by ID
cargo run -- memories                        # Browse stored memories
cargo run -- serve --port 8106               # HTTP MCP server
cargo run -- serve --stdio                   # Stdio MCP server
```

## Architecture

```text
src/
├── main.rs
├── lib.rs
├── store/
├── memory.rs
├── search.rs
├── index.rs
├── mcp/
├── rerank.rs
└── code/
```

Important modules:
- `store/` holds the SQLite schema, queries, and maintenance logic.
- `memory.rs` implements memory storage and cognitive scoring.
- `search.rs` runs hybrid retrieval, fusion, reranking, and relevance gating.
- `index.rs` scans, chunks, and embeds code.
- `mcp/` exposes `index`, `store`, and `search` over MCP.
- `code/` contains language-agnostic indexing primitives.

Retrieval pipeline:
- query expansion
- FTS5 keyword retrieval
- vector retrieval via HNSW
- RRF fusion
- cross-encoder reranking
- cognitive scoring for memories
- budget-aware truncation

## Schema Notes

A single `chunks` table stores both code and memory rows using a `kind` discriminator. Supporting tables include `codebases`, `indexed_files`, and `chunks_fts`.

Memory scoring uses:

```text
final_score = rrf_score x (0.5 + salience) x exp(-decay_rate x days_since_last_access)
```

Memory decay rates:
- `identity`: 0.0
- `knowledge`: 0.005
- `episode`: 0.023
- `procedure`: 0.01

## Conventions

- Default database path: `~/.grasshopper/brain.db`
- Model cache: `~/.cache/grasshopper/models/`
- Blocking DB and model work runs inside `spawn_blocking`
- Model initialization uses lazy `Arc<Mutex<Option<T>>>` patterns
- Ambiguous codebase references should error with suggestions
- Exit codes follow grep-style semantics: `0` success, `1` no results, `2` error

## Deployment

- Binary: `/usr/local/bin/grasshopper`
- Service: `grasshopper.service` on port 8106
- Auth proxy: `grasshopper-auth-proxy.service` on port 8107
- Local MCP: `http://127.0.0.1:8106/mcp`
- Remote MCP: Cloudflare Tunnel -> auth proxy -> Grasshopper
