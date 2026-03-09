# Grasshopper

[![CI](https://github.com/MylesMCook/grasshopper/actions/workflows/ci.yml/badge.svg)](https://github.com/MylesMCook/grasshopper/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Persistent hybrid search for code and memory. One binary, one SQLite database. All inference runs locally via ONNX — no external API calls at query time.

**7 CLI commands. 3 MCP tools. Built for AI agents.**

## Install

```sh
cargo install --git https://github.com/MylesMCook/grasshopper
```

First run downloads ~270 MB of models (Jina Code V2 embeddings + BGE reranker). Cached in `~/.cache/grasshopper/models/`.

## Quick Start

```sh
# Index a codebase
grasshopper index ./my-project --embed

# Search across code and memory
grasshopper search "error handling"

# Store a memory
grasshopper store "Always use .clamp() over .min().max() for bounded values"

# Check what's in the database
grasshopper status

# Start the MCP server
grasshopper serve
```

## CLI Reference

### `index` — Index source code

```sh
grasshopper index <dir>          # FTS only (fast)
grasshopper index <dir> --embed  # FTS + semantic embeddings (slow, ~4/sec CPU)
```

Scans files, chunks them at function/class boundaries, and indexes for search. Incremental — only re-processes changed files on subsequent runs. Respects `.gitignore`.

### `search` — Find code and memories

```sh
grasshopper search "auth middleware"              # hybrid search (default)
grasshopper search Config --mode navigate         # find definitions & references
grasshopper search _ --mode map --budget 8000     # codebase overview (query ignored)
grasshopper search Store --mode impact --depth 3  # what breaks if you change this?
```

**Modes:**

| Mode | What it does | When to use |
|------|-------------|-------------|
| `search` | Hybrid FTS + vector + reranking | General queries, finding code/memories |
| `navigate` | Symbol definitions & references | "Where is X defined? Who calls it?" |
| `map` | Token-budgeted codebase overview | "Show me the project structure" |
| `impact` | BFS blast radius analysis | "What breaks if I change X?" |

**Options:**

| Flag | Description |
|------|-------------|
| `--kind code\|memory\|all` | Filter by entry type |
| `--limit N` | Max results (1-100, default 10) |
| `--threshold 0.0-1.0` | Minimum relevance for memories |
| `--preset strict\|balanced\|exploratory` | Named threshold presets |
| `--json` | Machine-readable output |
| `--dir <path>` | Scope to a specific codebase |
| `--direction defs\|refs\|both` | Navigate mode edge direction |
| `--budget N` | Token budget for map mode |
| `--depth N` | BFS depth for impact mode (1-5) |

**Threshold presets** (alternative to raw `--threshold` values):
- `strict` — Only high-confidence results (0.5)
- `balanced` — Good precision/recall tradeoff (0.3)
- `exploratory` — Cast a wide net (0.1)

### `store` — Save a memory

```sh
grasshopper store "Always use bun for scripts"
grasshopper store "I prefer dark themes" --memory-type identity
grasshopper store "Deploy: build, test, push" --tags ops,deploy --title "Deploy Steps"
```

Memories are deduplicated by content hash (always) and embedding similarity when the embedder is available (cosine threshold 0.75). If a similar memory exists, it's updated instead of duplicated.

**Memory types:**

| Type | Decay | Use for |
|------|-------|---------|
| `knowledge` | Slow | Facts, decisions, architecture (default) |
| `identity` | None | Preferences, config — never fades (stored only; excluded from hybrid search, accessible via `memories` command) |
| `episode` | Fast | Session events, temporal context |
| `procedure` | Medium | Workflows, how-tos, recipes |

### `status` — Database health dashboard

```sh
grasshopper status        # human-readable overview
grasshopper status --json # machine-readable for scripts
```

Shows: database size, indexed codebases (with embedding coverage), memory stats by type, HNSW index state, and model cache status.

### `get` — Inspect any chunk

```sh
grasshopper get 42        # full content + metadata
grasshopper get 42 --json # structured output
```

Shows the complete content and metadata for any chunk (code or memory) by ID. Use it to debug search results.

### `memories` — Browse stored memories

```sh
grasshopper memories                        # all active memories
grasshopper memories --type identity        # filter by type
grasshopper memories --type episode --archived  # include archived
grasshopper memories --limit 50 --json      # paginate, machine output
```

### `serve` — MCP server

```sh
grasshopper serve                # HTTP on port 8106
grasshopper serve --port 9000   # custom port
grasshopper serve --stdio       # stdio transport (for Claude Code)
```

**Global option:** `--db /path/to/brain.db` overrides the default `~/.grasshopper/brain.db`.

## MCP Integration

### Claude Code (HTTP)

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

### Claude Code (stdio)

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

### MCP Tools

| Tool | Parameters | Description |
|------|-----------|-------------|
| `search` | `query`, `mode`, `kind`, `limit`, `threshold`, `budget`, `depth`, `dir`, `direction` | All search modes |
| `index` | `directory`, `embed` | Index a codebase |
| `store` | `content`, `title`, `tags`, `memory_type` | Store a memory |

Health check: `curl http://127.0.0.1:8106/healthz` returns `ok`.

## How It Works

### Retrieval Pipeline

```
Query → Expansion → FTS5 (BM25) ─┐
                                  ├─ RRF Fusion → Cross-encoder Rerank → Cognitive Scoring → Return
Query → Embedder → HNSW (cosine) ┘
```

1. **Query expansion** — CamelCase splitting, stop-word removal, phrase pairs, 2-token subsets (up to 6 variants)
2. **Dual retrieval** — FTS5 keyword search + HNSW vector search run independently
3. **RRF fusion** (k=60) — Merges ranked lists (40% FTS weight, 60% vector weight)
4. **Cross-encoder rerank** — BAAI/bge-reranker-base rescores top 20 candidates
5. **Cognitive scoring** — For memories: `score x salience x decay` weights by importance and recency
6. **Side effects** — Retrieved memories get a salience boost (+0.05, capped at 1.0)

### Cognitive Memory

Memories decay exponentially based on type:

```
final_score = rrf_score x (0.5 + salience) x exp(-decay_rate x days_since_last_access)
```

| Type | Decay Rate | Half-life |
|------|-----------|-----------|
| `identity` | 0.0 | Never |
| `knowledge` | 0.005 | ~139 days |
| `episode` | 0.023 | ~30 days |
| `procedure` | 0.01 | ~69 days |

Salience starts at 0.5 and increases by 0.05 each time a memory is retrieved (capped at 1.0). Frequently-accessed memories rise to the top.

### Code Intelligence

Language-agnostic — no tree-sitter, no AST parsing. Works on any text file:

- **Chunking** — Splits at blank lines and declaration boundaries. Merges small blocks, splits oversized ones.
- **Definitions** — Any chunk with a detected symbol name (via regex on `fn`, `class`, `struct`, etc.)
- **References** — FTS search for symbol name across all non-definition chunks
- **Impact** — BFS traversal from definitions through references

### Models

All models run locally via ONNX Runtime. Downloaded automatically on first use.

| Model | Purpose | Size |
|-------|---------|------|
| Jina Code V2 | Embeddings (768-dim) | ~142 MB |
| BGE Reranker Base | Cross-encoder reranking | ~130 MB |

## Architecture

```
src/
├── main.rs        CLI (7 subcommands)
├── lib.rs         Module exports
├── store/         SQLite schema, queries, maintenance
│   ├── mod.rs     Module exports, integration tests
│   ├── schema.rs  Store struct, migrations, PRAGMA config
│   ├── types.rs   Chunk, SearchHit, GraphEdge, RRF fusion
│   ├── memory.rs  Memory CRUD, touch, archive
│   ├── code.rs    Codebase indexing queries
│   ├── search.rs  FTS + vector search, HNSW search
│   ├── navigate.rs  Definitions, references, impact BFS
│   └── maintenance.rs  Stats, diagnostics, health checks
├── memory.rs      Cognitive layer (store, recall, scoring)
├── search.rs      Unified search orchestrator
├── index.rs       Directory scanning, chunking, embedding
├── mcp/           MCP server (3 tools, HTTP + stdio)
│   ├── mod.rs     Tool implementations, ServerHandler
│   ├── transport.rs  HTTP + stdio transports
│   └── format.rs  Result formatting
├── rerank.rs      Cross-encoder reranking (ONNX)
└── code/          Code intelligence primitives
    ├── mod.rs     Module exports, language hint tables
    ├── chunk.rs   Universal structural chunking
    ├── embed.rs   ONNX embeddings (Jina Code V2)
    ├── scan.rs    Directory walking, .gitignore, binary detection
    ├── tokenizer.rs  CamelCase splitting for FTS5
    └── hnsw.rs    HNSW approximate nearest neighbor
```

### Database

Single `chunks` table discriminated by `kind` column (`'code'` or `'memory'`). Code chunks have file paths, symbols, line numbers. Memory entries have types, salience, decay. Both share embeddings and content hashes.

Supporting tables: `codebases` (indexed directories), `indexed_files` (SHA-256 hashes for incremental indexing), `chunks_fts` (FTS5 virtual table).

Default location: `~/.grasshopper/brain.db`.

## Development

```sh
cargo build                                  # build
cargo test                                   # 155 tests
cargo clippy --all-targets                   # lint
cargo run -- --help                          # CLI help
cargo run -- status                          # check database state
RUST_LOG=debug cargo run -- serve            # verbose server
```

## Design Principles

- **Store Raw, Retrieve Smart** — All heavy compute happens at query time (expansion, fusion, reranking, scoring), not at write time. Based on Yuan et al. (2026) showing retrieval method matters 20x more than write strategy.
- **Language-agnostic** — One universal code path for all languages. No tree-sitter, no language-specific parsers.
- **Graceful degradation** — Embedder fails? FTS-only search. Reranker fails? Unreranked results. HNSW missing? Brute-force vectors.
- **Unix philosophy** — Exit code 1 = no results (grep convention). Exit code 2 = error. Supports `--json` for piping. Color auto-detection.

## License

[MIT](LICENSE)
