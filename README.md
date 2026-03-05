# Grasshopper

A unified agent brain that gives AI coding agents both code intelligence and cognitive memory through a single self-contained Rust binary and one SQLite database.

## Why

AI agents need two capabilities to work well across sessions: understanding codebases and remembering context. These are traditionally separate systems — a code search tool and a knowledge store — each with their own database, embedding pipeline, search index, and MCP server. Grasshopper merges them into one.

**One binary, one database, one embedding pipeline, 18 MCP tools, one systemd service.**

The merged architecture isn't just simpler to operate. It enables cross-domain search — a single query can find both the function definition and the design decision that explains why it exists.

## Design Philosophy: "Store Raw, Retrieve Smart"

Based on peer-reviewed research (Yuan et al. 2026, Maharana et al. 2024):
- Retrieval method = 20pt accuracy swing. Write strategy = only 3-8pt.
- Raw chunks + hybrid search + reranking = best config (81.1% accuracy on LoCoMo benchmark)
- Invest compute in retrieval (reranker, query expansion), not write-time processing
- All models run locally via ONNX — zero external API calls

## What It Does

### Code Intelligence

Index source code directories and search them with keyword, semantic, or hybrid search. Navigate symbol definitions and references via a code graph extracted by tree-sitter. Get a token-budgeted overview of an entire codebase. Analyze the blast radius of changing a symbol.

- **13 languages** supported via tree-sitter: Rust, TypeScript, JavaScript, Python, Go, Java, C, C++, C#, Ruby, PHP, Scala, TSX
- **Hybrid search**: FTS5 keyword matching + Jina Code V2 semantic embeddings, merged via Reciprocal Rank Fusion (k=60)
- **Incremental indexing**: content-hash diffing skips unchanged files, parallel chunking via Rayon
- **Code graph**: tree-sitter extracts definitions and references, stored as edges for BFS traversal

### Cognitive Memory

A brain-inspired memory system where memories are typed, scored, and decay over time. Agents store decisions, preferences, and learnings during a session and retrieve them later with cognitive-weighted search.

**Memory types** with different decay rates:
| Type | Decay | Purpose |
|------|-------|---------|
| `identity` | None | Who I am, preferences, never fades |
| `knowledge` | Slow (0.005/day) | Facts, decisions, architecture |
| `episode` | Fast (0.023/day) | Events, sessions, temporal context |
| `procedure` | Medium (0.01/day) | How-to, workflows, recipes |

**Cognitive scoring formula:**

```
score = rrf_score * (1 + recency) * (1 + frequency) * salience * decay
```

- **Recency boost**: +0.2 for entries created in the last 30 days, linear falloff
- **Frequency boost**: logarithmic on access count (memories accessed more rank higher)
- **Salience**: dynamic, +0.05 per retrieval, capped at 1.0
- **Exponential decay**: type-specific lambda applied to days since last access
- **Hebbian associations**: memories retrieved together get linked, strengthening future co-retrieval

**Dedup**: new memories are embedded and compared against existing entries. Cosine similarity > 0.75 triggers an update instead of a duplicate insert.

### Session Continuity

Handoffs record what was accomplished and what comes next. Pickup loads the latest handoff and retrieves related memories via semantic search. This gives agents context continuity across sessions without relying on conversation history.

## Architecture

```
grasshopper/src/
├── main.rs     — CLI with 14 subcommands
├── lib.rs      — Module exports
├── store.rs    — Unified SQLite schema, all queries, maintenance
├── memory.rs   — Cognitive layer: remember, recall, classify, score, reflect, consolidate
├── search.rs   — Hybrid search: FTS5 + vector → RRF fusion → query expansion → cross-encoder rerank
├── index.rs    — Code indexing: scan → chunk → graph → FTS → embed
├── mcp.rs      — MCP server: 18 tools, HTTP + stdio transport, embedding cache
├── nli.rs      — NLI contradiction detection via ONNX Runtime
├── rerank.rs   — Cross-encoder reranking via fastembed/ONNX
└── code/       — Code intelligence primitives (self-contained)
    ├── chunk.rs      — Tree-sitter code chunking (13 languages)
    ├── embed.rs      — ONNX embeddings (Jina Code V2, 768-dim)
    ├── scan.rs       — Directory walking + SHA-256 hashing
    ├── tokenizer.rs  — CamelCase splitting for FTS5
    ├── graph.rs      — Tag extraction via tree-sitter TAGS_QUERY
    └── hnsw.rs       — HNSW approximate nearest neighbor (instant-distance)
```

Fully self-contained — no external path dependencies. All code intelligence primitives are in the `code/` module.

### Database

Single SQLite file at `~/.grasshopper/brain.db` with WAL mode. One `chunks` table stores both code and memory entries, discriminated by a `kind` column (`'code'` or `'memory'`). Code chunks have file paths, symbols, and line numbers. Memory entries have types, salience, and access counts. Both share embeddings and content hashes.

Supporting tables:
- `codebases` — registered source directories
- `indexed_files` — file hashes for incremental indexing
- `graph` — code definition/reference edges + Hebbian memory associations
- `chunks_fts` — FTS5 virtual table with Porter stemming and Unicode tokenization
- `handoffs` — session continuity records

A custom `code_expand` SQL function splits camelCase and PascalCase identifiers so `parseJSON` is searchable as `parse json`.

### Embedding

Jina Code V2 (`jina-embeddings-v2-base-code`) via ONNX Runtime, producing 768-dimensional vectors. Quantized to uint8 for storage. ~4 embeddings/sec on CPU (cold start ~600ms for model load, then <30ms per tool call warm).

Embeddings are optional for code indexing (`--embed` flag) but always used for memory dedup and cognitive recall. The embedder initializes lazily on first use and is shared across all tool calls via `Arc<Mutex<Option<Embedder>>>`.

## Tools

18 MCP tools, grouped by annotation:

### Read-only

| Tool | Description |
|------|-------------|
| `search` | Hybrid search across code and memory. Start here. |
| `navigate` | Find a symbol's definition and all its references via the code graph. |
| `map` | Token-budgeted overview of an entire codebase. |
| `impact` | BFS traversal showing what breaks if you change a symbol. |
| `me` | Identity snapshot: who am I, active projects, working set. |
| `pickup` | Resume a session: loads latest handoff + related memories. |
| `recall` | Cognitive-scored memory search with decay and salience. |
| `get_context` | Proactive context surfacing with relevance gate. |
| `reflect` | Meta-cognition: growing, fading, connections, gaps analysis. |
| `get` | Retrieve the full content of any entry by numeric ID. |
| `status` | System health: DB size, model status, counts. |

### Write

| Tool | Description |
|------|-------------|
| `index` | Index a source directory (with optional `--embed`). |
| `remember` | Store a memory with auto-classification and dedup. |
| `handoff` | Record session progress and next steps for a project. |
| `archive` | Soft-delete a memory by ID. |
| `feedback` | Rate retrieval quality for fine-tuning pipeline. |

### Destructive

| Tool | Description |
|------|-------------|
| `consolidate` | Memory hygiene: find duplicates, archive stale entries, cluster episodes. Defaults to dry-run. |
| `maintenance` | Database maintenance: VACUUM, integrity check, orphan cleanup. |

## CLI

```sh
grasshopper index ./my-project              # Index source code
grasshopper index ./my-project --embed      # Index + generate embeddings
grasshopper search "hybrid search"          # Search code + memory
grasshopper search "Store" --kind code      # Code only
grasshopper remember "Prefer .clamp() over .min() for bounded values"
grasshopper recall "parameter validation"   # Cognitive-scored memory search
grasshopper me                              # Identity snapshot
grasshopper pickup                          # Resume from last handoff
grasshopper pickup --project grasshopper    # Resume specific project
grasshopper reflect --focus fading          # Find neglected memories
grasshopper consolidate --dry-run           # Preview cleanup
grasshopper consolidate                     # Execute cleanup
grasshopper stats                           # Memory and code counts
grasshopper serve --port 8106              # HTTP MCP server
grasshopper mcp                             # stdio MCP (for Claude Code)
```

Global option: `--db /path/to/brain.db` overrides the default database location.

## Deployment

### Build

```sh
cargo build --release
```

Release profile uses LTO, symbol stripping, and single codegen unit for a small binary.

### Install

```sh
sudo cp target/release/grasshopper /usr/local/bin/grasshopper
```

### Systemd (HTTP mode)

The server runs as a systemd service on port 8106, accessed remotely through an OAuth 2.1 auth proxy on port 8107 and a Cloudflare tunnel.

```
Internet → Cloudflare Tunnel → grasshopper-auth-proxy (:8107) → grasshopper (:8106)
Local    → http://127.0.0.1:8106/mcp (no auth, loopback only)
```

### Claude Code integration

Add to `~/.claude.json`:
```json
{
  "mcpServers": {
    "grasshopper": {
      "type": "http",
      "url": "http://127.0.0.1:8106/mcp"
    }
  }
}
```

Or for stdio mode, use `grasshopper mcp` as the command.

### Agent instructions (one-liner)

Add this to any LLM system prompt, CLAUDE.md, or agent harness:

> Grasshopper is a unified agent brain combining code intelligence and cognitive memory. Start: call `me` then `pickup` to load identity and resume context. During: use `remember` to save decisions, learnings, and preferences. End: call `handoff` to record progress and next steps. Code intelligence (requires `index` first): `search`, `navigate`, `map`, `impact`. Memory: `remember`, `me`, `pickup`, `handoff`, `reflect`, `consolidate`. `get` retrieves full content by ID.

## Development

```sh
cargo build                    # Build
cargo test                     # 173 tests across all modules
cargo run -- --help            # CLI help
RUST_LOG=debug cargo run -- serve --port 8106  # Verbose server
```

### Conventions

- Fully self-contained — no external path dependencies
- Database default: `~/.grasshopper/brain.db`
- Embedding model cache: `~/.cache/grasshopper/models/`
- All blocking operations (DB, embedder) run inside `spawn_blocking` — never on the async runtime
- Poisoned mutexes are recovered with a warning log, not panicked
- Ambiguous codebase references error with suggestions instead of silently picking one
