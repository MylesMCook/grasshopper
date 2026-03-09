# Changelog

## 1.0.0

Initial public release.

- **Hybrid retrieval pipeline**: FTS5 keyword search + HNSW vector search, fused via reciprocal rank fusion (k=60), rescored by cross-encoder reranking (BAAI/bge-reranker-base)
- **Cognitive memory**: Typed entries (identity, knowledge, episode, procedure) with salience scoring, time-based decay, and automatic deduplication
- **Code intelligence**: Language-agnostic structural chunking, symbol navigation, codebase mapping, and impact analysis
- **3 MCP tools**: `search`, `index`, `store` -- usable from Claude Code and any MCP client
- **4 CLI commands**: `search`, `index`, `store`, `serve`
- **All local**: Jina Code V2 embeddings + BGE Reranker Base via ONNX Runtime. Zero external API calls.
- **Single binary, single SQLite database**
