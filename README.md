# Grasshopper

A persistent retrieval engine for AI agents. Indexes codebases and stores memories in a single SQLite database. Agents search with one tool; the engine handles hybrid retrieval, reranking, and cognitive scoring internally.

**One binary, one database, 3 MCP tools, 4 CLI commands.**

## Design Philosophy: "Store Raw, Retrieve Smart"

Based on peer-reviewed research (Yuan et al. 2026, Maharana et al. 2024):
- Retrieval method = 20pt accuracy swing. Write strategy = only 3-8pt.
- Raw chunks + hybrid search + reranking = best config (81.1% accuracy on LoCoMo benchmark)
- Invest compute in retrieval (reranker, query expansion), not write-time processing
- All models run locally via ONNX -- zero external API calls

## Quick Start

```sh
# Build and install
cargo build --release
sudo cp target/release/grasshopper /usr/local/bin/grasshopper

# Index a codebase
grasshopper index ./my-project --embed

# Store a memory
grasshopper store "Prefer .clamp() over .min() for bounded values"

# Search across code and memory
grasshopper search "parameter validation"

# Start MCP server
grasshopper serve --port 8106
```

## MCP Tools

3 tools, covering all agent workflows:

| Tool | Description |
|------|-------------|
| `index` | Index a source directory (with optional embeddings). |
| `store` | Store a memory with auto-classification and dedup. |
| `search` | Hybrid search across code and memory. Supports `mode` parameter for specialized queries. |

The `search` tool's `mode` parameter:

| Mode | What it does |
|------|-------------|
| *(default)* | Hybrid search: FTS5 + vector + RRF fusion + reranking |
| `navigate` | Find a symbol's definition and all its references via the code graph |
| `map` | Token-budgeted overview of an entire codebase |
| `impact` | BFS traversal showing what breaks if you change a symbol |

## CLI

4 commands:

```sh
grasshopper index ./my-project              # Index source code
grasshopper index ./my-project --embed      # Index + generate embeddings
grasshopper store "design decision text"    # Store a memory
grasshopper search "hybrid search"          # Search code + memory
grasshopper search "query" --kind code      # Code only
grasshopper search "query" --mode navigate  # Symbol navigation
grasshopper search "query" --mode map       # Codebase overview
grasshopper search "query" --mode impact    # Blast radius analysis
grasshopper serve --port 8106              # HTTP MCP server
grasshopper serve --stdio                  # stdio MCP (for Claude Code)
```

Global option: `--db /path/to/brain.db` overrides the default database location.

## Architecture

```
grasshopper/src/
├── main.rs     -- CLI with 4 subcommands (index, store, search, serve)
├── lib.rs      -- Module exports
├── store.rs    -- Unified SQLite schema, all queries, auto-maintenance
├── memory.rs   -- Cognitive layer: store, recall, cognitive_score (salience + decay)
├── search.rs   -- Hybrid search: FTS5 + vector -> RRF fusion -> query expansion -> cross-encoder rerank -> unified_search
├── index.rs    -- Code indexing: scan -> chunk -> graph -> FTS -> embed
├── mcp.rs      -- MCP server: 3 tools, HTTP + stdio transport, embedding cache
├── rerank.rs   -- Cross-encoder reranking via fastembed/ONNX
└── code/       -- Code intelligence primitives (self-contained)
    ├── chunk.rs      -- Tree-sitter code chunking (13 languages)
    ├── embed.rs      -- ONNX embeddings (Jina Code V2, 768-dim)
    ├── scan.rs       -- Directory walking + SHA-256 hashing
    ├── tokenizer.rs  -- CamelCase splitting for FTS5
    ├── graph.rs      -- Tag extraction via tree-sitter TAGS_QUERY
    └── hnsw.rs       -- HNSW approximate nearest neighbor (instant-distance)
```

### Retrieval Pipeline

```
Query Expansion -> FTS5 (BM25) -> Vector (cosine, HNSW) -> RRF fusion (k=60) -> Cross-encoder rerank -> Relevance gate -> Cognitive scoring (salience * decay) -> Budget truncation -> Return
```

### Code Intelligence

- **13 languages** via tree-sitter: Rust, TypeScript, JavaScript, Python, Go, Java, C, C++, C#, Ruby, PHP, Scala, TSX
- **Hybrid search**: FTS5 keyword matching + Jina Code V2 semantic embeddings, merged via Reciprocal Rank Fusion
- **Incremental indexing**: content-hash diffing skips unchanged files, parallel chunking via Rayon
- **Code graph**: tree-sitter extracts definitions and references for navigate/impact queries

### Cognitive Memory

Memories are typed, scored, and decay over time:

| Type | Decay | Purpose |
|------|-------|---------|
| `identity` | None | Who I am, preferences, never fades |
| `knowledge` | Slow (0.005/day) | Facts, decisions, architecture |
| `episode` | Fast (0.023/day) | Events, sessions, temporal context |
| `procedure` | Medium (0.01/day) | How-to, workflows, recipes |

**Cognitive scoring**: `score = salience * decay`. Salience increases (+0.05) each time a memory is retrieved, capped at 1.0. Decay is exponential with type-specific lambda.

**Dedup**: new memories are embedded and compared against existing entries. Cosine similarity > 0.75 triggers an update instead of a duplicate insert.

### Database

Single SQLite file at `~/.grasshopper/brain.db` with WAL mode. One `chunks` table stores both code and memory entries, discriminated by a `kind` column. Auto-maintenance: HNSW rebuilds on startup, PRAGMA optimize on shutdown.

### Embedding

Jina Code V2 (`jina-embeddings-v2-base-code`) via ONNX Runtime, producing 768-dimensional vectors. Quantized to uint8 for storage. The embedder initializes lazily and is shared across all tool calls.

## Deployment

### Systemd (HTTP mode)

The server runs as a systemd service on port 8106, accessed remotely through an OAuth 2.1 auth proxy on port 8107 and a Cloudflare tunnel.

```
Internet -> Cloudflare Tunnel -> grasshopper-auth-proxy (:8107) -> grasshopper (:8106)
Local    -> http://127.0.0.1:8106/mcp (no auth, loopback only)
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

Or use stdio mode: `grasshopper serve --stdio`.

## Development

```sh
cargo build                    # Build
cargo test                     # ~157 tests across all modules
cargo run -- --help            # CLI help
RUST_LOG=debug cargo run -- serve --port 8106  # Verbose server
```

### Conventions

- Fully self-contained -- no external path dependencies
- Database default: `~/.grasshopper/brain.db`
- Embedding model cache: `~/.cache/grasshopper/models/`
- All blocking operations (DB, embedder) run inside `spawn_blocking`
- Poisoned mutexes are recovered with a warning log, not panicked
- Ambiguous codebase references error with suggestions instead of silently picking one
