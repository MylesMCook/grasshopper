# Changelog

## 2.0.1 (24 September 2026)

- Accept one explicitly configured private HTTPS proxy Host for MCP without relaxing the default Host check for other requests. Reject mismatched Origins.
- Add regression coverage for the proxy path and document the Mac mini private service and encrypted off-host recovery.

## 2.0.0 (24 September 2026): Shared memory

- One authenticated SQLite memory service with `context`, `store`, `search`, `get`, and `archive`.
- One stateless client for Codex, Cursor, and Claude Code on macOS, Windows, and Linux. AGENTS.md is the sole standing-instruction format.
- Scoped records, explicit corrections with revision conflicts, provenance, full history, idempotent retries, and copy-only migration/backup.
- Portable bundles tested on Mac, Work HP, and Beelink with synthetic data. Desktop app revalidation remains user-led.
- Retired the Rust CLI and local code-search feature. Entries below describe earlier releases, not active commands.

## 1.0.1

Security hardening and quality sprint.

- **FTS5 injection prevention**: All query tokens double-quoted to block operator injection (AND, NOT, NEAR)
- **FTS5 punctuation crash fix**: Queries like `*` or `!@#$` no longer cause MATCH errors
- **HNSW deserialization bounded**: `bincode::with_limit()` prevents OOM on corrupt indexes
- **HNSW v2 format**: Magic header (`GH02`) with count validation on load
- **Silent error drops replaced**: 14 `.filter_map(|r| r.ok())` calls now log warnings via `log_and_skip`
- **Brute-force guard kind-filtered**: Threshold checks code and memory chunks independently
- **DX improvements**: `status`, `get`, `memories` CLI commands; named search presets
- **158 tests**: 19 FTS5 fuzz tests, 10 HNSW corruption tests added

## 1.0.0

Initial public release.

- **Hybrid retrieval pipeline**: FTS5 keyword search + HNSW vector search, fused via reciprocal rank fusion (k=60), rescored by cross-encoder reranking (BAAI/bge-reranker-base)
- **Cognitive memory**: Typed entries (identity, knowledge, episode, procedure) with salience scoring, time-based decay, and automatic deduplication
- **Code intelligence**: Language-agnostic structural chunking, symbol navigation, codebase mapping, and impact analysis
- **3 MCP tools**: `search`, `index`, `store` — usable from Claude Code and any MCP client
- **7 CLI commands**: `search`, `index`, `store`, `status`, `get`, `memories`, `serve`
- **All local**: Jina Code V2 embeddings + BGE Reranker Base via ONNX Runtime. Zero external API calls.
- **Single binary, single SQLite database**
