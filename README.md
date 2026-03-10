<p align="center">
  <img src="assets/logo.svg" alt="Grasshopper" width="128" height="128">
</p>

<h1 align="center">Grasshopper</h1>

<p align="center">
  Persistent hybrid search for code and memory.<br>
  One binary. One SQLite database. All inference runs locally.
</p>

<p align="center">
  <a href="https://github.com/MylesMCook/grasshopper/actions/workflows/ci.yml"><img src="https://github.com/MylesMCook/grasshopper/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License: MIT"></a>
  <a href="https://github.com/MylesMCook/grasshopper/releases/latest"><img src="https://img.shields.io/github/v/release/MylesMCook/grasshopper" alt="Latest Release"></a>
</p>

---

## What It Does

Grasshopper gives AI agents a brain that persists between sessions. It indexes codebases, stores memories, and retrieves both through a single hybrid search pipeline — keyword matching, vector similarity, and cross-encoder reranking, all in one query.

No API keys. No cloud dependencies. No Docker. Just a binary and a database file.

**7 CLI commands. 3 MCP tools. 158 tests.**

## Use Cases

### Give your AI agent persistent memory

Claude Code forgets everything when a session ends. Grasshopper remembers. Store architectural decisions, debugging insights, and user preferences. They surface automatically in future searches.

```sh
grasshopper store "This project uses bun, not npm"
grasshopper store "Auth bug: the refresh token was expiring because clock skew exceeded 30s"
grasshopper search "why does auth keep breaking"
# → returns the refresh token insight, ranked by relevance
```

### Search a codebase by meaning, not just keywords

`grep` finds exact strings. Grasshopper finds what you mean. It combines keyword matching (FTS5) with semantic search (768-dim embeddings) and reranks results with a cross-encoder.

```sh
grasshopper index ./my-project --embed
grasshopper search "how does authentication work"
# → finds auth middleware, token validation, session handling
# even if none of them contain the word "authentication"
```

### Understand unfamiliar code fast

Dropped into a new repo? Map the structure, find symbol definitions, and trace what breaks if you change something.

```sh
grasshopper search _ --mode map --budget 4000      # project overview
grasshopper search Config --mode navigate           # where is Config defined? who uses it?
grasshopper search DatabasePool --mode impact       # what breaks if I change this?
```

### Drop-in MCP server for any AI tool

Grasshopper speaks MCP natively. Claude Code, Cursor, Windsurf, or any MCP client can use it as a search backend with zero glue code.

```sh
grasshopper serve              # HTTP on port 8106
grasshopper serve --stdio      # stdio transport for Claude Code
```

## Install

```sh
# From source
cargo install --git https://github.com/MylesMCook/grasshopper

# Pre-built binary (Linux x64, macOS ARM)
curl --proto '=https' --tlsv1.2 -LsSf https://github.com/MylesMCook/grasshopper/releases/latest/download/grasshopper-installer.sh | sh
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

# Check what's indexed
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

Scans files, chunks them at function/class boundaries, and indexes for search. Incremental — only re-processes changed files. Respects `.gitignore`.

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
| `search` | Hybrid FTS + vector + reranking | General queries |
| `navigate` | Symbol definitions & references | "Where is X defined? Who calls it?" |
| `map` | Token-budgeted codebase overview | "Show me the project structure" |
| `impact` | BFS blast radius analysis | "What breaks if I change X?" |

**Options:**

| Flag | Description |
|------|-------------|
| `--kind code\|memory\|all` | Filter by entry type |
| `--limit N` | Max results (1–100, default 10) |
| `--threshold 0.0–1.0` | Minimum relevance for memories |
| `--preset strict\|balanced\|exploratory` | Named threshold presets |
| `--json` | Machine-readable output |
| `--dir <path>` | Scope to a specific codebase |
| `--direction defs\|refs\|both` | Navigate mode edge direction |
| `--budget N` | Token budget for map mode |
| `--depth N` | BFS depth for impact mode (1–5) |

**Threshold presets:**
- `strict` — High-confidence results only (0.5)
- `balanced` — Good precision/recall tradeoff (0.3)
- `exploratory` — Cast a wide net (0.1)

### `store` — Save a memory

```sh
grasshopper store "Always use bun for scripts"
grasshopper store "I prefer dark themes" --memory-type identity
grasshopper store "Deploy: build, test, push" --tags ops,deploy --title "Deploy Steps"
```

Memories are deduplicated by content hash and embedding similarity (cosine threshold 0.75). If a similar memory exists, Grasshopper updates it instead of creating a duplicate.

**Memory types:**

| Type | Decay | Use for |
|------|-------|---------|
| `knowledge` | Slow | Facts, decisions, architecture (default) |
| `identity` | None | Preferences, config — never fades |
| `episode` | Fast | Session events, temporal context |
| `procedure` | Medium | Workflows, how-tos, recipes |

Identity memories are stored but excluded from hybrid search results. Access them with the `memories` command.

### `status` — Database overview

```sh
grasshopper status        # human-readable
grasshopper status --json # machine-readable
```

Shows database size, indexed codebases with embedding coverage, memory stats by type, HNSW index state, and model cache status.

### `get` — Inspect a chunk

```sh
grasshopper get 42        # full content + metadata
grasshopper get 42 --json # structured output
```

### `memories` — Browse stored memories

```sh
grasshopper memories                           # all active memories
grasshopper memories --type identity           # filter by type
grasshopper memories --type episode --archived # include archived
grasshopper memories --limit 50 --json         # paginate
```

### `serve` — MCP server

```sh
grasshopper serve                # HTTP on port 8106
grasshopper serve --port 9000   # custom port
grasshopper serve --stdio       # stdio transport
```

**Global option:** `--db /path/to/brain.db` overrides the default `~/.grasshopper/brain.db`.

## MCP Integration

### Claude Code (HTTP)

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
2. **Dual retrieval** — FTS5 keyword search and HNSW vector search run independently
3. **RRF fusion** (k=60) — Merges ranked lists with 40% FTS weight, 60% vector weight
4. **Cross-encoder rerank** — BAAI/bge-reranker-base rescores the top 20 candidates
5. **Cognitive scoring** — For memories: `score × salience × decay` weights by importance and recency
6. **Side effects** — Retrieved memories gain a salience boost (+0.05, capped at 1.0)

### Cognitive Memory

Memories decay exponentially based on type:

```
final_score = rrf_score × (0.5 + salience) × exp(-decay_rate × days_since_last_access)
```

| Type | Decay Rate | Half-life |
|------|-----------|-----------|
| `identity` | 0.0 | Never |
| `knowledge` | 0.005 | ~139 days |
| `episode` | 0.023 | ~30 days |
| `procedure` | 0.01 | ~69 days |

Salience starts at 0.5 and increases by 0.05 each retrieval (capped at 1.0). Frequently-accessed memories rise; stale ones fade.

### Code Intelligence

Language-agnostic — no tree-sitter, no AST parsing. Works on any text file:

- **Chunking** — Splits at blank lines and declaration boundaries. Merges small blocks, splits oversized ones.
- **Definitions** — Chunks with a detected symbol name (via regex on `fn`, `class`, `struct`, etc.)
- **References** — FTS search for symbol name across all non-definition chunks
- **Impact** — BFS traversal from definitions through references

### Models

All models run locally via ONNX Runtime. Downloaded on first use.

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

Single `chunks` table discriminated by `kind` (`'code'` or `'memory'`). Code chunks carry file paths, symbols, and line numbers. Memory entries carry types, salience, and decay rates. Both share embeddings and content hashes.

Supporting tables: `codebases` (indexed directories), `indexed_files` (SHA-256 hashes for incremental indexing), `chunks_fts` (FTS5 virtual table).

Default location: `~/.grasshopper/brain.db`.

## Development

```sh
cargo build                                  # build
cargo test                                   # 158 tests
cargo clippy --all-targets                   # lint
cargo run -- --help                          # CLI help
cargo run -- status                          # check database state
RUST_LOG=debug cargo run -- serve            # verbose server
```

## Design Principles

- **Store Raw, Retrieve Smart** — Heavy compute happens at query time (expansion, fusion, reranking, scoring), not at write time. Based on Yuan et al. (2026): retrieval method matters 20x more than write strategy.
- **Language-agnostic** — One code path for all languages. No tree-sitter, no language-specific parsers.
- **Graceful degradation** — Embedder fails? FTS-only search. Reranker fails? Unreranked results. HNSW missing? Brute-force vectors.
- **Unix philosophy** — Exit code 1 = no results (grep convention). Exit code 2 = error. `--json` for piping. Color auto-detection.

## License

[MIT](LICENSE)
