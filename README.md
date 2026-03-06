# Grasshopper

Persistent hybrid search for code and memory. One binary, one SQLite database. All inference runs locally — no external API calls at query time.

Grasshopper indexes codebases and stores memories, then retrieves them with a pipeline that combines full-text search, semantic embeddings, reciprocal rank fusion, and cross-encoder reranking — all running locally via ONNX.

**3 MCP tools. 4 CLI commands. Built for AI agents.**

## Install

**Pre-built binaries** (Linux x64/arm64, macOS x64/arm64, Windows x64):

```sh
# macOS / Linux
curl --proto '=https' --tlsv1.2 -LsSf https://github.com/MylesMCook/grasshopper/releases/latest/download/grasshopper-installer.sh | sh

# Windows (PowerShell)
powershell -ExecutionPolicy ByPass -c "irm https://github.com/MylesMCook/grasshopper/releases/latest/download/grasshopper-installer.ps1 | iex"
```

**From source**:

```sh
cargo install --git https://github.com/MylesMCook/grasshopper
```

**Docker** (runs MCP server — for indexing/searching, use the CLI binary):

```sh
docker run -v grasshopper-data:/data -p 8106:8106 ghcr.io/mylesmcook/grasshopper
```

## Quick Start

```sh
# Index a codebase (first run downloads ~270MB of models)
grasshopper index ./my-project --embed

# Search across code and memory
grasshopper search "error handling"

# Store a memory
grasshopper store "Always use .clamp() over .min().max() for bounded values"

# Start the MCP server
grasshopper serve --port 8106
```

## What This Is

- A local retrieval engine that AI agents query via MCP or CLI
- Hybrid search combining keywords (FTS5) and semantics (vector) with reranking
- Cognitive memory with typed entries, salience scoring, and time decay
- Language-agnostic code intelligence — works on any text file

## What This Isn't

- Not a vector database (it's SQLite with an HNSW index)
- Not a cloud service (everything runs on your machine)
- Not language-specific (no tree-sitter, no AST parsing — universal heuristics only)

## CLI Reference

| Command | Description |
|---------|-------------|
| `index <dir>` | Index a source directory. Add `--embed` for semantic search. |
| `search <query>` | Hybrid search across code and memory. |
| `store <content>` | Store a memory with auto-dedup. |
| `serve` | Start the MCP server (HTTP or `--stdio`). |

Global option: `--db /path/to/brain.db` overrides the default `~/.grasshopper/brain.db`.

## Search Modes

The `search` command (and MCP tool) supports four modes via `--mode`:

| Mode | Description | Example |
|------|-------------|---------|
| `search` *(default)* | Hybrid retrieval: FTS5 + vector + RRF fusion + reranking | `grasshopper search "auth middleware"` |
| `navigate` | Find symbol definitions and references | `grasshopper search "Config" --mode navigate` |
| `map` | Token-budgeted codebase overview | `grasshopper search "" --mode map` |
| `impact` | BFS blast radius — what breaks if you change a symbol | `grasshopper search "Store" --mode impact` |

Additional flags: `--kind code|memory`, `--limit N`, `--json`, `--direction defs|refs|both`.

## MCP Integration

**Claude Code** (`~/.claude.json`):

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

Or use stdio transport directly:

```json
{
  "mcpServers": {
    "grasshopper": {
      "command": "grasshopper",
      "args": ["serve", "--stdio"]
    }
  }
}
```

The MCP server exposes three tools: `index`, `store`, `search`. The `search` tool accepts a `mode` parameter matching the CLI modes above.

Verify the server is running: `curl http://127.0.0.1:8106/healthz` should return `ok`.

## How It Works

### Retrieval Pipeline

```
Query → Expansion → FTS5 (BM25) ─┐
                                  ├─ RRF Fusion → Cross-encoder Rerank → Relevance Gate → Cognitive Scoring → Return
Query → Embedder → HNSW (cosine) ┘
```

1. **Query expansion**: CamelCase splitting, synonym generation
2. **Dual retrieval**: FTS5 keyword search + HNSW vector search run in parallel
3. **RRF fusion** (k=60): merges ranked lists from both retrievers
4. **Cross-encoder rerank**: BAAI/bge-reranker-base rescores the top candidates
5. **Cognitive scoring**: `score × salience × decay` weights memories by importance and recency
6. **Budget truncation**: trims output to fit the caller's token budget

### Cognitive Memory

Memories are typed, scored, and decay over time:

| Type | Decay Rate | Purpose |
|------|-----------|---------|
| `identity` | None | Preferences, never fades |
| `knowledge` | Slow | Facts, decisions, architecture |
| `episode` | Fast | Events, sessions, temporal context |
| `procedure` | Medium | How-to, workflows, recipes |

Salience increases (+0.05) each retrieval, capped at 1.0. New memories are deduplicated by cosine similarity (threshold: 0.75).

### Models

All models run locally via ONNX Runtime. Downloaded automatically on first use to `~/.cache/grasshopper/models/`.

| Model | Purpose | Size |
|-------|---------|------|
| Jina Code V2 | Embeddings (768-dim) | ~142 MB |
| BGE Reranker Base | Cross-encoder reranking | ~130 MB |

## Architecture

```
src/
├── main.rs        CLI (4 subcommands)
├── lib.rs         Module exports
├── store/         SQLite schema, queries, maintenance
│   ├── schema.rs  Store struct, migrations
│   ├── types.rs   Chunk, SearchHit, GraphEdge, row mappers
│   ├── memory.rs  Memory CRUD
│   ├── code.rs    Codebase indexing queries
│   ├── search.rs  FTS + vector search
│   ├── navigate.rs  Definitions, references, impact BFS
│   └── maintenance.rs  Stats, logging, optimize
├── memory.rs      Cognitive layer (store, recall, scoring)
├── search.rs      Unified search orchestrator
├── index.rs       Directory scanning, chunking, embedding
├── mcp.rs         MCP server (3 tools, HTTP + stdio)
├── rerank.rs      Cross-encoder reranking
└── code/
    ├── chunk.rs   Structural chunking (language-agnostic)
    ├── embed.rs   ONNX embeddings
    ├── scan.rs    Directory walking + hashing
    ├── tokenizer.rs  CamelCase splitting for FTS5
    └── hnsw.rs    HNSW nearest neighbor index
```

## Development

```sh
cargo build                    # Build
cargo test                     # 107 tests
cargo clippy --all-targets     # Lint
cargo run -- --help            # CLI help
RUST_LOG=debug cargo run -- serve --port 8106  # Verbose server
```

## License

[MIT](LICENSE)
