# Grasshopper

Unified agent brain — code intelligence (Ferret) + cognitive memory (Corpus) in one Rust binary.

## Key Commands

```sh
cargo build                    # Build
cargo test                     # Run tests
cargo run -- --help            # CLI help
cargo run -- remember "text"   # Store a memory
cargo run -- stats             # Show counts
cargo run -- search "query"    # Search (Phase 1+)
cargo run -- index ./path      # Index code (Phase 1+)
cargo run -- serve --port 8101 # MCP server (Phase 3+)
```

## Architecture

- `store.rs` — Unified SQLite schema (code chunks + memory entries in one `chunks` table, graph edges, handoffs)
- Imports Ferret as library: chunk, embed, scan, graph, hnsw, tokenizer modules
- Does NOT use Ferret's `store` (hardcoded schema) or `mcp` (Ferret-specific tools)

## Schema

Single `chunks` table with `kind` discriminator ('code' | 'memory'). Code chunks have file_path, language, symbol_name, etc. Memory entries have memory_type, salience, access_count, etc. Both share embeddings, content_hash, and graph edges.

## Conventions

- Ferret dependency: `ferret = { path = "../ferret", features = ["semantic"] }`
- Database default: `~/.grasshopper/brain.db`
- Design doc: Craft > Projects > Grasshopper: Unified Agent Brain
- Linear: LAB-26 through LAB-29 (Phases 0-3)
