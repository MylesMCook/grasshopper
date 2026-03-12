//! Memory retrieval accuracy benchmark.
//!
//! Stores realistic memories with varied types, ages, and salience,
//! then runs gold-standard queries measuring Recall, P@K, MRR, NDCG@10,
//! plus cognitive-specific metrics (recency bias, decay correctness,
//! salience accumulation, identity exclusion).
//!
//! Gold sets are built INDEPENDENTLY of search results — by scanning ALL
//! memories in the DB before any search runs.
//!
//! Runs each query in TWO configs: FTS-only and Hybrid (FTS + embeddings + HNSW).
//! Reports which queries improve with semantic search and which degrade.
//!
//! Run: cargo bench --bench accuracy_memory

mod common;

use common::{BenchReport, mrr, ndcg_at_k, precision_at_k};
use grasshopper::code::embed::{Embedder, default_cache_dir};
use grasshopper::code::hnsw::HnswIndex;
use grasshopper::search::{SearchContext, unified_search};
use grasshopper::store::{MemoryParams, Store};
use std::time::Instant;
use tempfile::TempDir;

// ---------------------------------------------------------------------------
// Memory corpus (~120 entries)
// ---------------------------------------------------------------------------

struct MemEntry {
    title: &'static str,
    content: &'static str,
    memory_type: &'static str,
    days_ago: f64,
    last_access_days: Option<f64>,
    salience: f64,
    descriptors: &'static str,
}

fn memory_corpus() -> Vec<MemEntry> {
    vec![
        // === Architectural decisions (knowledge, varied ages) ===
        MemEntry {
            title: "Use SQLite for all storage",
            content: "We chose SQLite with WAL mode for the main database. Single-file deployment, zero config, handles concurrent reads well.",
            memory_type: "knowledge",
            days_ago: 60.0,
            last_access_days: Some(5.0),
            salience: 0.8,
            descriptors: "architecture,database,sqlite",
        },
        MemEntry {
            title: "ONNX for local inference",
            content: "All models run locally via ONNX Runtime. No external API calls. Jina Code V2 for embeddings, BGE reranker for cross-encoder.",
            memory_type: "knowledge",
            days_ago: 45.0,
            last_access_days: Some(3.0),
            salience: 0.7,
            descriptors: "architecture,models,onnx",
        },
        MemEntry {
            title: "Hybrid search over pure vector",
            content: "Hybrid FTS+vector with RRF fusion outperforms pure vector search. FTS catches exact keyword matches that embeddings miss.",
            memory_type: "knowledge",
            days_ago: 30.0,
            last_access_days: Some(2.0),
            salience: 0.9,
            descriptors: "architecture,search,hybrid",
        },
        MemEntry {
            title: "Single binary deployment",
            content: "Grasshopper ships as a single binary. No Docker, no sidecar processes. SQLite file + HNSW sidecar is the entire state.",
            memory_type: "knowledge",
            days_ago: 50.0,
            last_access_days: Some(10.0),
            salience: 0.6,
            descriptors: "architecture,deployment",
        },
        MemEntry {
            title: "Store raw retrieve smart",
            content: "Yuan et al. showed retrieval method gives 20pt accuracy swing vs 3-8pt for write strategy. We invest compute in retrieval, not write-time processing.",
            memory_type: "knowledge",
            days_ago: 40.0,
            last_access_days: Some(1.0),
            salience: 0.9,
            descriptors: "architecture,philosophy,research",
        },
        MemEntry {
            title: "WAL mode for concurrency",
            content: "PRAGMA journal_mode=WAL enables concurrent readers during writes. Critical for search during index operations.",
            memory_type: "knowledge",
            days_ago: 55.0,
            last_access_days: Some(15.0),
            salience: 0.5,
            descriptors: "database,concurrency,wal",
        },
        MemEntry {
            title: "FTS5 with custom tokenizer",
            content: "FTS5 uses porter unicode61 tokenizer with custom code_expand function for camelCase/PascalCase splitting.",
            memory_type: "knowledge",
            days_ago: 35.0,
            last_access_days: Some(4.0),
            salience: 0.7,
            descriptors: "search,fts5,tokenizer",
        },
        MemEntry {
            title: "Cross-encoder reranking improves precision",
            content: "Adding BGE reranker base after RRF fusion consistently improves top-3 precision. Worth the ~50ms latency cost.",
            memory_type: "knowledge",
            days_ago: 20.0,
            last_access_days: Some(2.0),
            salience: 0.8,
            descriptors: "search,reranking,precision",
        },
        MemEntry {
            title: "Language-agnostic chunking",
            content: "Chunk code by universal structural patterns (indentation, blank lines) rather than language-specific parsers. One path for everything.",
            memory_type: "knowledge",
            days_ago: 42.0,
            last_access_days: Some(7.0),
            salience: 0.6,
            descriptors: "indexing,chunking,language-agnostic",
        },
        MemEntry {
            title: "HNSW for O(log N) vector search",
            content: "instant-distance crate for HNSW. ef_construction=100. Serialized to sidecar file. Falls back to brute-force under 10K chunks.",
            memory_type: "knowledge",
            days_ago: 25.0,
            last_access_days: Some(3.0),
            salience: 0.7,
            descriptors: "search,hnsw,vector",
        },
        MemEntry {
            title: "Embedding dimension is 768",
            content: "Jina Code V2 produces 768-dimensional embeddings. L2-normalized. Stored as little-endian f32 blobs in SQLite.",
            memory_type: "knowledge",
            days_ago: 38.0,
            last_access_days: Some(5.0),
            salience: 0.5,
            descriptors: "embeddings,dimensions",
        },
        MemEntry {
            title: "RRF fusion with k=60",
            content: "Reciprocal Rank Fusion with k=60. FTS weight 0.4, vector weight 0.6. Merges keyword and semantic results.",
            memory_type: "knowledge",
            days_ago: 28.0,
            last_access_days: Some(2.0),
            salience: 0.6,
            descriptors: "search,rrf,fusion",
        },
        MemEntry {
            title: "Query expansion for recall",
            content: "Expand queries with stop-word removal, quoted adjacent pairs, and 2-token subsets. Max 6 variants. Merged pairwise via RRF.",
            memory_type: "knowledge",
            days_ago: 15.0,
            last_access_days: Some(1.0),
            salience: 0.7,
            descriptors: "search,query-expansion",
        },
        MemEntry {
            title: "Systemd for service management",
            content: "All services run as systemd units. grasshopper.service on port 8106. Auth proxy on 8107.",
            memory_type: "knowledge",
            days_ago: 50.0,
            last_access_days: Some(8.0),
            salience: 0.5,
            descriptors: "deployment,systemd",
        },
        MemEntry {
            title: "Cloudflare tunnel for external access",
            content: "External traffic flows through Cloudflare tunnel. Auth proxy adds OAuth 2.1. Local bypass for loopback requests.",
            memory_type: "knowledge",
            days_ago: 48.0,
            last_access_days: Some(6.0),
            salience: 0.6,
            descriptors: "networking,cloudflare,tunnel",
        },
        MemEntry {
            title: "Restic for nightly backups",
            content: "Nightly restic backups to Cloudflare R2. 7 daily + 4 weekly retention. Includes DB, env files, configs.",
            memory_type: "knowledge",
            days_ago: 55.0,
            last_access_days: Some(12.0),
            salience: 0.5,
            descriptors: "backup,restic,r2",
        },
        MemEntry {
            title: "Cognitive scoring formula",
            content: "final_score = rrf_score * (0.5 + salience) * exp(-decay_rate * days_since_last_access). Salience bumped +0.05 per retrieval.",
            memory_type: "knowledge",
            days_ago: 10.0,
            last_access_days: Some(1.0),
            salience: 0.8,
            descriptors: "scoring,cognitive,formula",
        },
        MemEntry {
            title: "MCP server with 3 tools",
            content: "Grasshopper exposes 3 MCP tools: index, store, search. Supports stdio and Streamable HTTP transport.",
            memory_type: "knowledge",
            days_ago: 18.0,
            last_access_days: Some(2.0),
            salience: 0.7,
            descriptors: "mcp,tools,server",
        },
        MemEntry {
            title: "Symbol navigation is language-agnostic",
            content: "Definitions derived from chunks.symbol_name. References found via FTS search on content. Map ranks files by definition count.",
            memory_type: "knowledge",
            days_ago: 22.0,
            last_access_days: Some(3.0),
            salience: 0.6,
            descriptors: "navigation,symbols,definitions",
        },
        MemEntry {
            title: "Content hash for exact dedup",
            content: "SHA-256 of title+content for exact-match dedup before embedding similarity check. 0.75 cosine threshold for near-dups.",
            memory_type: "knowledge",
            days_ago: 16.0,
            last_access_days: Some(2.0),
            salience: 0.6,
            descriptors: "dedup,hash,content",
        },
        MemEntry {
            title: "Rust edition 2024",
            content: "Project uses Rust edition 2024 with rust-version 1.85 minimum. Clippy perf=deny, redundant_clone=deny.",
            memory_type: "knowledge",
            days_ago: 35.0,
            last_access_days: Some(5.0),
            salience: 0.5,
            descriptors: "rust,edition,clippy",
        },
        MemEntry {
            title: "Lazy model initialization",
            content: "Embedder and reranker use Arc<Mutex<Option<T>>> for lazy init in MCP server. Models loaded on first use.",
            memory_type: "knowledge",
            days_ago: 32.0,
            last_access_days: Some(4.0),
            salience: 0.6,
            descriptors: "models,lazy-init,mcp",
        },
        MemEntry {
            title: "BM25 weights tuned for code",
            content: "FTS5 BM25 weights: title=5.0, content=1.0, snippet=1.0, symbol_name=5.0, descriptors=2.0. Title and symbol matches are strong signals.",
            memory_type: "knowledge",
            days_ago: 24.0,
            last_access_days: Some(3.0),
            salience: 0.7,
            descriptors: "search,bm25,weights",
        },
        MemEntry {
            title: "Exit codes follow grep convention",
            content: "Exit 0 = success with results, 1 = no results found, 2 = error. Follows grep convention for scripting.",
            memory_type: "knowledge",
            days_ago: 40.0,
            last_access_days: Some(8.0),
            salience: 0.4,
            descriptors: "cli,exit-codes",
        },
        MemEntry {
            title: "Traefik reverse proxy setup",
            content: "Traefik handles HTTPS termination and routing. Docker labels for service discovery. Middleware for security headers.",
            memory_type: "knowledge",
            days_ago: 60.0,
            last_access_days: Some(10.0),
            salience: 0.5,
            descriptors: "traefik,proxy,networking",
        },
        MemEntry {
            title: "OAuth 2.1 with PKCE for remote access",
            content: "All tunneled MCP servers use OAuth 2.1 with PKCE S256. Shared mcp-oauth module. DCR for dynamic client registration.",
            memory_type: "knowledge",
            days_ago: 14.0,
            last_access_days: Some(2.0),
            salience: 0.7,
            descriptors: "oauth,security,mcp",
        },
        MemEntry {
            title: "Brute force guard at 10K chunks",
            content: "Vector search falls back to brute-force when HNSW unavailable. MAX_BRUTE_FORCE_CHUNKS=10000 prevents OOM on large datasets.",
            memory_type: "knowledge",
            days_ago: 20.0,
            last_access_days: Some(4.0),
            salience: 0.5,
            descriptors: "search,vector,guard",
        },
        MemEntry {
            title: "Poisoned mutex recovery",
            content: "MCP server recovers from poisoned mutexes with a warning log instead of panicking. Defensive pattern for long-running services.",
            memory_type: "knowledge",
            days_ago: 28.0,
            last_access_days: Some(6.0),
            salience: 0.5,
            descriptors: "error-handling,mutex,recovery",
        },
        MemEntry {
            title: "Docker log rotation limits",
            content: "Docker daemon configured with 10MB max-size and 3 max-files for log rotation. Prevents disk fill.",
            memory_type: "knowledge",
            days_ago: 55.0,
            last_access_days: Some(20.0),
            salience: 0.4,
            descriptors: "docker,logging,limits",
        },
        MemEntry {
            title: "Tailscale VPN for remote access",
            content: "Primary remote access via Tailscale. Static IP 100.125.35.63. MagicDNS enabled for beelink.taila43aae.ts.net.",
            memory_type: "knowledge",
            days_ago: 65.0,
            last_access_days: Some(1.0),
            salience: 0.6,
            descriptors: "tailscale,vpn,remote",
        },
        // === NEW: Additional knowledge for topical overlap ===
        MemEntry {
            title: "SQLite connection pooling strategy",
            content: "Single connection with WAL mode. No pool needed — SQLite handles concurrent reads natively. Write serialization via Mutex.",
            memory_type: "knowledge",
            days_ago: 50.0,
            last_access_days: Some(8.0),
            salience: 0.5,
            descriptors: "database,sqlite,connections",
        },
        MemEntry {
            title: "Cloudflare Access email OTP policy",
            content: "Sensitive endpoints gated by Cloudflare Access with email OTP. Applied to firecrawl and SSH browser tunnels.",
            memory_type: "knowledge",
            days_ago: 40.0,
            last_access_days: Some(5.0),
            salience: 0.6,
            descriptors: "security,cloudflare,access",
        },
        MemEntry {
            title: "SSH hardening configuration",
            content: "SSH config: X11Forwarding no, PermitRootLogin no, MaxAuthTries 3, ClientAliveInterval 30. Ed25519 keys only.",
            memory_type: "knowledge",
            days_ago: 45.0,
            last_access_days: Some(10.0),
            salience: 0.5,
            descriptors: "security,ssh,hardening",
        },
        MemEntry {
            title: "Env file permission model",
            content: "All .env files chmod 600 root-owned. Systemd services use EnvironmentFile= directive. Never readable by unprivileged users.",
            memory_type: "knowledge",
            days_ago: 42.0,
            last_access_days: Some(8.0),
            salience: 0.6,
            descriptors: "security,env,permissions",
        },
        MemEntry {
            title: "Kernel sysctl hardening",
            content: "IPv6 redirects disabled. Strict reverse path filtering. Config at /etc/sysctl.d/99-beelink.conf.",
            memory_type: "knowledge",
            days_ago: 50.0,
            last_access_days: Some(15.0),
            salience: 0.4,
            descriptors: "security,kernel,sysctl",
        },
        MemEntry {
            title: "Container security constraints",
            content: "All containers run with no-new-privileges:true and resource limits (memory + CPU). Docker daemon hardened.",
            memory_type: "knowledge",
            days_ago: 48.0,
            last_access_days: Some(12.0),
            salience: 0.5,
            descriptors: "security,docker,containers",
        },
        MemEntry {
            title: "Backup encryption with restic",
            content: "Restic uses repository password for encryption. Passphrase stored in Bitwarden. R2 credentials in /etc/restic/r2.env.",
            memory_type: "knowledge",
            days_ago: 52.0,
            last_access_days: Some(10.0),
            salience: 0.5,
            descriptors: "backup,encryption,restic",
        },
        MemEntry {
            title: "Backup coverage and exclusions",
            content: "Backs up PocketBase data, all .env files, cloudflared creds, systemd units, Claude memory. Excludes node_modules and build artifacts.",
            memory_type: "knowledge",
            days_ago: 50.0,
            last_access_days: Some(8.0),
            salience: 0.5,
            descriptors: "backup,coverage,scope",
        },
        MemEntry {
            title: "FTS5 query preparation pipeline",
            content: "prepare_fts_query: strip special chars, handle quotes, escape operators. Applied before every FTS MATCH clause.",
            memory_type: "knowledge",
            days_ago: 20.0,
            last_access_days: Some(3.0),
            salience: 0.6,
            descriptors: "search,fts5,query-prep",
        },
        MemEntry {
            title: "Relevance threshold gating",
            content: "Configurable threshold filters low-quality results. Presets: strict=0.5, balanced=0.3, exploratory=0.1. Default: no threshold.",
            memory_type: "knowledge",
            days_ago: 18.0,
            last_access_days: Some(2.0),
            salience: 0.6,
            descriptors: "search,threshold,presets",
        },
        MemEntry {
            title: "Systemd hardening for cloudflared",
            content: "ProtectSystem=strict, ProtectHome=true, PrivateTmp=true. Minimizes blast radius of tunnel process.",
            memory_type: "knowledge",
            days_ago: 44.0,
            last_access_days: Some(12.0),
            salience: 0.4,
            descriptors: "deployment,systemd,hardening",
        },
        MemEntry {
            title: "Cloudflare tunnel ingress routing",
            content: "config.yml maps hostnames to local ports. Catch-all returns 404. Each service gets a subdomain of funnydomainname.com.",
            memory_type: "knowledge",
            days_ago: 46.0,
            last_access_days: Some(8.0),
            salience: 0.5,
            descriptors: "networking,cloudflare,routing",
        },
        MemEntry {
            title: "MCP OAuth shared module design",
            content: "Shared mcp-oauth module handles DCR, PKCE S256 auth code, token exchange. Used by corpus, grasshopper-auth-proxy, firecrawl.",
            memory_type: "knowledge",
            days_ago: 16.0,
            last_access_days: Some(3.0),
            salience: 0.6,
            descriptors: "mcp,oauth,module",
        },
        MemEntry {
            title: "Local MCP loopback bypass",
            content: "Middleware skips OAuth when cf-connecting-ip header absent. Only tunneled requests carry it, so local Claude Code gets free access.",
            memory_type: "knowledge",
            days_ago: 14.0,
            last_access_days: Some(2.0),
            salience: 0.6,
            descriptors: "mcp,auth,loopback",
        },
        MemEntry {
            title: "Traefik middleware gotcha with @file",
            content: "Docker labels must use secure-headers@file,rate-limit@file suffix. Without @file, Traefik returns 404.",
            memory_type: "knowledge",
            days_ago: 30.0,
            last_access_days: Some(8.0),
            salience: 0.5,
            descriptors: "traefik,middleware,gotcha",
        },
        MemEntry {
            title: "Index file hash for incremental updates",
            content: "SHA-256 hash of each file stored in indexed_files table. Only re-chunks files whose hash changed since last index run.",
            memory_type: "knowledge",
            days_ago: 26.0,
            last_access_days: Some(4.0),
            salience: 0.6,
            descriptors: "indexing,incremental,hash",
        },
        MemEntry {
            title: "Code structure graph for navigation",
            content: "Symbol graph built from chunk definitions and FTS-based reference lookup. Used by navigate and impact search modes.",
            memory_type: "knowledge",
            days_ago: 20.0,
            last_access_days: Some(3.0),
            salience: 0.6,
            descriptors: "navigation,graph,symbols",
        },
        MemEntry {
            title: "Near-duplicate detection via embeddings",
            content: "After exact hash dedup, check cosine similarity of embeddings against existing entries. 0.75 threshold for near-duplicates.",
            memory_type: "knowledge",
            days_ago: 16.0,
            last_access_days: Some(2.0),
            salience: 0.5,
            descriptors: "dedup,embeddings,similarity",
        },
        MemEntry {
            title: "SSH authorized keys management",
            content: "Ed25519 keys in ~/.ssh/authorized_keys. Multiple keys for different clients (VS Code, Echo app, work laptop). Remove revoked keys promptly.",
            memory_type: "knowledge",
            days_ago: 40.0,
            last_access_days: Some(8.0),
            salience: 0.5,
            descriptors: "ssh,keys,management",
        },
        MemEntry {
            title: "CI test pipeline configuration",
            content: "GitHub Actions runs cargo test, cargo clippy, cargo fmt --check on every push. MSRV tested against rust-version in Cargo.toml.",
            memory_type: "knowledge",
            days_ago: 30.0,
            last_access_days: Some(5.0),
            salience: 0.4,
            descriptors: "ci,testing,github-actions",
        },
        // === Debugging episodes (25 entries, recent, high salience) ===
        MemEntry {
            title: "Fixed FTS query escaping bug",
            content: "FTS5 MATCH was failing on queries with double quotes. Fixed by escaping quotes in prepare_fts_query.",
            memory_type: "episode",
            days_ago: 2.0,
            last_access_days: Some(1.0),
            salience: 0.7,
            descriptors: "debug,fts,escaping",
        },
        MemEntry {
            title: "Debugged HNSW deserialization OOM",
            content: "Corrupt HNSW file with u64::MAX point count caused OOM. Fixed with bincode with_limit guard.",
            memory_type: "episode",
            days_ago: 5.0,
            last_access_days: Some(2.0),
            salience: 0.8,
            descriptors: "debug,hnsw,oom",
        },
        MemEntry {
            title: "Fixed embedding model download",
            content: "Model download was failing silently on network errors. Added atomic temp file + rename pattern.",
            memory_type: "episode",
            days_ago: 8.0,
            last_access_days: Some(3.0),
            salience: 0.6,
            descriptors: "debug,download,embedder",
        },
        MemEntry {
            title: "Resolved concurrent index lock",
            content: "Two index operations on same directory caused database lock. Added per-directory lockfile with stale detection.",
            memory_type: "episode",
            days_ago: 3.0,
            last_access_days: Some(1.0),
            salience: 0.7,
            descriptors: "debug,locking,concurrent",
        },
        MemEntry {
            title: "Fixed reranker score normalization",
            content: "Reranker scores were in [0,1] but RRF scores were ~0.003-0.016. Added RRF_THRESHOLD_SCALE constant.",
            memory_type: "episode",
            days_ago: 6.0,
            last_access_days: Some(2.0),
            salience: 0.6,
            descriptors: "debug,reranker,normalization",
        },
        MemEntry {
            title: "Debugged camelCase tokenizer",
            content: "FTS5 wasn't finding camelCase symbols. Added code_expand SQL function for token splitting.",
            memory_type: "episode",
            days_ago: 10.0,
            last_access_days: Some(4.0),
            salience: 0.5,
            descriptors: "debug,tokenizer,camelcase",
        },
        MemEntry {
            title: "Fixed memory salience overflow",
            content: "Salience was exceeding 1.0 after many retrievals. Added cap at 1.0 in touch_memory.",
            memory_type: "episode",
            days_ago: 4.0,
            last_access_days: Some(1.0),
            salience: 0.6,
            descriptors: "debug,salience,overflow",
        },
        MemEntry {
            title: "Resolved Brotli compression issue",
            content: "Cloudflare edge was Brotli-compressing MCP responses. Fixed by adding Cache-Control: no-transform to all servers.",
            memory_type: "episode",
            days_ago: 7.0,
            last_access_days: Some(2.0),
            salience: 0.7,
            descriptors: "debug,cloudflare,brotli",
        },
        MemEntry {
            title: "Fixed stale file detection",
            content: "Removed files weren't being cleaned from FTS index. Added remove_stale_files after scanning.",
            memory_type: "episode",
            days_ago: 12.0,
            last_access_days: Some(5.0),
            salience: 0.5,
            descriptors: "debug,indexing,stale",
        },
        MemEntry {
            title: "Debugged vector search empty results",
            content: "Vector search returning empty when embeddings existed. Root cause: model name mismatch in query.",
            memory_type: "episode",
            days_ago: 9.0,
            last_access_days: Some(3.0),
            salience: 0.6,
            descriptors: "debug,vector,empty",
        },
        MemEntry {
            title: "Fixed MCP tool schema validation",
            content: "MCP tools were rejecting valid requests. Fixed z.coerce.number() instead of z.number() for string params.",
            memory_type: "episode",
            days_ago: 1.0,
            last_access_days: Some(0.5),
            salience: 0.7,
            descriptors: "debug,mcp,schema",
        },
        MemEntry {
            title: "Resolved token_type_ids crash",
            content: "ONNX model crash on inference. ALiBi-based Jina model doesn't expect token_type_ids. Added runtime detection.",
            memory_type: "episode",
            days_ago: 15.0,
            last_access_days: Some(6.0),
            salience: 0.5,
            descriptors: "debug,onnx,crash",
        },
        MemEntry {
            title: "Fixed batch embedding OOM",
            content: "Large batch of 1000 texts caused OOM in embed_batch. Added EMBED_BATCH=32 constant for chunked processing.",
            memory_type: "episode",
            days_ago: 11.0,
            last_access_days: Some(4.0),
            salience: 0.6,
            descriptors: "debug,embedding,oom",
        },
        MemEntry {
            title: "Debugged FTS rebuild timing",
            content: "Full FTS rebuild after small change was slow. Added conditional: full rebuild only when >50% files changed.",
            memory_type: "episode",
            days_ago: 14.0,
            last_access_days: Some(5.0),
            salience: 0.5,
            descriptors: "debug,fts,rebuild",
        },
        MemEntry {
            title: "Fixed identity memories appearing in search",
            content: "Identity memories were showing up in regular search results. Added filter in unified_search cognitive scoring block.",
            memory_type: "episode",
            days_ago: 3.0,
            last_access_days: Some(1.0),
            salience: 0.7,
            descriptors: "debug,identity,search",
        },
        MemEntry {
            title: "Resolved HNSW count mismatch",
            content: "HNSW header count didn't match actual graph after tampering. Added validation with warning on mismatch.",
            memory_type: "episode",
            days_ago: 7.0,
            last_access_days: Some(2.0),
            salience: 0.5,
            descriptors: "debug,hnsw,validation",
        },
        MemEntry {
            title: "Fixed hybrid search dedup",
            content: "Same chunk appearing in both FTS and vector results got double-counted in RRF. Fixed by keying on chunk ID.",
            memory_type: "episode",
            days_ago: 13.0,
            last_access_days: Some(5.0),
            salience: 0.5,
            descriptors: "debug,hybrid,dedup",
        },
        MemEntry {
            title: "Debugged Unicode in FTS queries",
            content: "FTS5 was choking on Unicode queries. prepare_fts_query now handles non-ASCII characters correctly.",
            memory_type: "episode",
            days_ago: 8.0,
            last_access_days: Some(3.0),
            salience: 0.5,
            descriptors: "debug,unicode,fts",
        },
        MemEntry {
            title: "Fixed archive flag not persisting",
            content: "Archived memories still appeared after restart. The archived column wasn't included in the update query.",
            memory_type: "episode",
            days_ago: 6.0,
            last_access_days: Some(2.0),
            salience: 0.5,
            descriptors: "debug,archive,persistence",
        },
        MemEntry {
            title: "Resolved search timeout on large DB",
            content: "Search was timing out with 50K+ chunks. Root cause: brute-force vector search loading all embeddings. Added HNSW fallback.",
            memory_type: "episode",
            days_ago: 4.0,
            last_access_days: Some(1.0),
            salience: 0.8,
            descriptors: "debug,performance,timeout",
        },
        // NEW: More episodes for topical overlap
        MemEntry {
            title: "Fixed FTS ranking inconsistency",
            content: "BM25 scores were varying across identical queries due to stale FTS statistics. Forced FTS optimize after bulk insert.",
            memory_type: "episode",
            days_ago: 3.0,
            last_access_days: Some(1.0),
            salience: 0.6,
            descriptors: "debug,fts,ranking",
        },
        MemEntry {
            title: "Debugged memory type filter regression",
            content: "After schema change, memory_type column was NULL for migrated rows. Added COALESCE fallback in queries.",
            memory_type: "episode",
            days_ago: 6.0,
            last_access_days: Some(2.0),
            salience: 0.5,
            descriptors: "debug,schema,migration",
        },
        MemEntry {
            title: "Fixed OAuth token refresh loop",
            content: "Auth proxy was stuck refreshing expired tokens in a loop. Root cause: clock skew between tunnel and server.",
            memory_type: "episode",
            days_ago: 5.0,
            last_access_days: Some(2.0),
            salience: 0.6,
            descriptors: "debug,oauth,token",
        },
        MemEntry {
            title: "Resolved deploy script race condition",
            content: "Deploy script restarted service before npm build finished. Added wait-for-build step with exit code check.",
            memory_type: "episode",
            days_ago: 8.0,
            last_access_days: Some(3.0),
            salience: 0.5,
            descriptors: "debug,deploy,race",
        },
        MemEntry {
            title: "Fixed backup verification failure",
            content: "Restic check was reporting corrupt pack. Root cause: incomplete upload during network interruption. Re-pruned orphan packs.",
            memory_type: "episode",
            days_ago: 10.0,
            last_access_days: Some(4.0),
            salience: 0.5,
            descriptors: "debug,backup,corruption",
        },
        // === Deployment procedures (25 entries, medium age) ===
        MemEntry {
            title: "Deploy grasshopper binary",
            content: "Step 1: cargo build --release. Step 2: sudo cp target/release/grasshopper /usr/local/bin/. Step 3: sudo systemctl restart grasshopper.",
            memory_type: "procedure",
            days_ago: 10.0,
            last_access_days: Some(2.0),
            salience: 0.7,
            descriptors: "deploy,grasshopper,procedure",
        },
        MemEntry {
            title: "Backup restore procedure",
            content: "Step 1: source /etc/restic/r2.env. Step 2: restic snapshots (find target). Step 3: restic restore latest --target /tmp/restore.",
            memory_type: "procedure",
            days_ago: 20.0,
            last_access_days: Some(8.0),
            salience: 0.6,
            descriptors: "backup,restore,procedure",
        },
        MemEntry {
            title: "MCP server deployment",
            content: "Step 1: cd /opt/mcp-servers/server-name. Step 2: npm run build. Step 3: sudo systemctl restart mcp-service. Step 4: smoke test with curl.",
            memory_type: "procedure",
            days_ago: 12.0,
            last_access_days: Some(3.0),
            salience: 0.7,
            descriptors: "mcp,deploy,procedure",
        },
        MemEntry {
            title: "HNSW index rebuild",
            content: "HNSW rebuilds automatically on startup. Manual: delete .hnsw sidecar file, restart grasshopper service.",
            memory_type: "procedure",
            days_ago: 15.0,
            last_access_days: Some(5.0),
            salience: 0.5,
            descriptors: "hnsw,rebuild,procedure",
        },
        MemEntry {
            title: "Add new MCP tool",
            content: "Step 1: Add tool handler in mcp/mod.rs. Step 2: Add to ServerHandler trait impl. Step 3: Test via stdio mode. Step 4: Deploy.",
            memory_type: "procedure",
            days_ago: 18.0,
            last_access_days: Some(4.0),
            salience: 0.6,
            descriptors: "mcp,tool,procedure",
        },
        MemEntry {
            title: "Database migration process",
            content: "Add migration SQL to init_schema in store/schema.rs. Use IF NOT EXISTS guards. Down+up for dev. Ship with binary.",
            memory_type: "procedure",
            days_ago: 25.0,
            last_access_days: Some(6.0),
            salience: 0.6,
            descriptors: "database,migration,procedure",
        },
        MemEntry {
            title: "Run full test suite",
            content: "cargo test runs ~158 tests. Use --nocapture for bench_cognitive output. Model tests download on first run (~200MB).",
            memory_type: "procedure",
            days_ago: 8.0,
            last_access_days: Some(1.0),
            salience: 0.6,
            descriptors: "testing,procedure",
        },
        MemEntry {
            title: "Cloudflare tunnel reconfiguration",
            content: "Edit ~/.cloudflared/config.yml. Add new ingress rule. Restart cloudflared service. Test via external URL.",
            memory_type: "procedure",
            days_ago: 30.0,
            last_access_days: Some(10.0),
            salience: 0.5,
            descriptors: "cloudflare,tunnel,procedure",
        },
        MemEntry {
            title: "Create systemd service",
            content: "Create .service file in /etc/systemd/system/. Use ExecStart, Restart=always, After=network.target. Enable with systemctl enable.",
            memory_type: "procedure",
            days_ago: 35.0,
            last_access_days: Some(12.0),
            salience: 0.5,
            descriptors: "systemd,service,procedure",
        },
        MemEntry {
            title: "Monitor service health",
            content: "Check: systemctl status grasshopper. Logs: journalctl -u grasshopper -f. Health: curl http://127.0.0.1:8106/mcp.",
            memory_type: "procedure",
            days_ago: 14.0,
            last_access_days: Some(2.0),
            salience: 0.6,
            descriptors: "monitoring,health,procedure",
        },
        MemEntry {
            title: "Update Rust toolchain",
            content: "rustup update stable. Check MSRV in Cargo.toml. Run cargo test. Update CI workflow if needed.",
            memory_type: "procedure",
            days_ago: 22.0,
            last_access_days: Some(7.0),
            salience: 0.4,
            descriptors: "rust,toolchain,procedure",
        },
        MemEntry {
            title: "Rotate secrets",
            content: "Step 1: Generate new secret. Step 2: Update .env file (chmod 600). Step 3: Restart affected service. Step 4: Test.",
            memory_type: "procedure",
            days_ago: 40.0,
            last_access_days: Some(15.0),
            salience: 0.5,
            descriptors: "security,secrets,procedure",
        },
        MemEntry {
            title: "Debug production issue",
            content: "Step 1: journalctl -u service -n 100. Step 2: Check /proc/PID/status for memory. Step 3: strace if needed. Step 4: Fix and deploy.",
            memory_type: "procedure",
            days_ago: 16.0,
            last_access_days: Some(3.0),
            salience: 0.6,
            descriptors: "debugging,production,procedure",
        },
        MemEntry {
            title: "Benchmark grasshopper search",
            content: "cargo bench --bench search_latency. Check bench/results/. Compare P50/P95 against baseline.",
            memory_type: "procedure",
            days_ago: 5.0,
            last_access_days: Some(1.0),
            salience: 0.6,
            descriptors: "benchmark,procedure",
        },
        MemEntry {
            title: "Set up new project",
            content: "cargo init. Add to Linear. Create systemd service. Add to backup script. Add Cloudflare tunnel route if external.",
            memory_type: "procedure",
            days_ago: 30.0,
            last_access_days: Some(10.0),
            salience: 0.5,
            descriptors: "project,setup,procedure",
        },
        MemEntry {
            title: "Review and merge PR",
            content: "Check CI green. Review diff. Test locally if complex. Merge. Deploy if service. Close Linear issue.",
            memory_type: "procedure",
            days_ago: 20.0,
            last_access_days: Some(5.0),
            salience: 0.4,
            descriptors: "pr,review,procedure",
        },
        MemEntry {
            title: "SSH key rotation",
            content: "Generate new ed25519 key. Add to authorized_keys. Test connection. Remove old key. Update Bitwarden.",
            memory_type: "procedure",
            days_ago: 45.0,
            last_access_days: Some(20.0),
            salience: 0.5,
            descriptors: "ssh,keys,procedure",
        },
        MemEntry {
            title: "Docker compose update",
            content: "Edit docker-compose.yml. docker compose pull. docker compose up -d. Verify with docker compose ps.",
            memory_type: "procedure",
            days_ago: 25.0,
            last_access_days: Some(6.0),
            salience: 0.5,
            descriptors: "docker,compose,procedure",
        },
        MemEntry {
            title: "Add Cloudflare Access policy",
            content: "Cloudflare Zero Trust dashboard. Add application. Configure email OTP. Test with incognito browser.",
            memory_type: "procedure",
            days_ago: 35.0,
            last_access_days: Some(12.0),
            salience: 0.4,
            descriptors: "cloudflare,access,procedure",
        },
        MemEntry {
            title: "Onboard new MCP server",
            content: "Create server in /opt/mcp-servers/. Add to registry.json. Create systemd service. Add tunnel route. Test with smoke_test.py.",
            memory_type: "procedure",
            days_ago: 12.0,
            last_access_days: Some(3.0),
            salience: 0.6,
            descriptors: "mcp,onboard,procedure",
        },
        // NEW: More procedures for topical overlap
        MemEntry {
            title: "Deploy MCP auth proxy update",
            content: "Step 1: cd /opt/mcp-servers/grasshopper-auth-proxy. Step 2: npm run build. Step 3: sudo systemctl restart grasshopper-auth-proxy. Step 4: Test OAuth flow.",
            memory_type: "procedure",
            days_ago: 10.0,
            last_access_days: Some(3.0),
            salience: 0.6,
            descriptors: "deploy,mcp,auth-proxy",
        },
        MemEntry {
            title: "Run backup verification check",
            content: "sudo bash -c 'source /etc/restic/r2.env && restic check'. Validates pack integrity. Run after any restore or prune.",
            memory_type: "procedure",
            days_ago: 18.0,
            last_access_days: Some(6.0),
            salience: 0.5,
            descriptors: "backup,verify,procedure",
        },
        MemEntry {
            title: "Configure Cloudflare firewall rule",
            content: "CF dashboard > WAF > Custom rules. Block by country or IP range. Rate limit per path. Test with curl from external.",
            memory_type: "procedure",
            days_ago: 28.0,
            last_access_days: Some(10.0),
            salience: 0.4,
            descriptors: "cloudflare,firewall,procedure",
        },
        MemEntry {
            title: "Migrate database schema manually",
            content: "sqlite3 brain.db < migration.sql. Backup DB first. Verify with .schema. Rebuild FTS after column changes.",
            memory_type: "procedure",
            days_ago: 22.0,
            last_access_days: Some(8.0),
            salience: 0.5,
            descriptors: "database,schema,manual-migration",
        },
        MemEntry {
            title: "Update deployed service environment",
            content: "Edit /opt/service/.env. sudo systemctl restart service. Verify with journalctl. Check no secrets in logs.",
            memory_type: "procedure",
            days_ago: 15.0,
            last_access_days: Some(4.0),
            salience: 0.5,
            descriptors: "deploy,env,procedure",
        },
        // === User preferences (identity, zero decay) ===
        MemEntry {
            title: "I prefer simple solutions",
            content: "Always prefer the simplest solution that works. 80/20 rule. No unnecessary abstractions.",
            memory_type: "identity",
            days_ago: 90.0,
            last_access_days: Some(1.0),
            salience: 1.0,
            descriptors: "preferences,simplicity",
        },
        MemEntry {
            title: "I use Rust for systems work",
            content: "Rust for all systems-level projects. Prefer compiled languages for performance-critical code.",
            memory_type: "identity",
            days_ago: 90.0,
            last_access_days: Some(1.0),
            salience: 1.0,
            descriptors: "preferences,rust,language",
        },
        MemEntry {
            title: "I use kebab-case for projects",
            content: "Project names use kebab-case. File fields use snake_case. Container names use kebab-case.",
            memory_type: "identity",
            days_ago: 60.0,
            last_access_days: Some(5.0),
            salience: 0.8,
            descriptors: "conventions,naming",
        },
        MemEntry {
            title: "I prefer passkey authentication",
            content: "WebAuthn/passkeys preferred for auth. better-auth library for new projects. PocketBase uses its own auth.",
            memory_type: "identity",
            days_ago: 45.0,
            last_access_days: Some(3.0),
            salience: 0.8,
            descriptors: "preferences,auth,passkey",
        },
        MemEntry {
            title: "My name is Myles",
            content: "Owner of this beelink server. Email: mylesmcook@gmail.com. Software engineer.",
            memory_type: "identity",
            days_ago: 120.0,
            last_access_days: Some(1.0),
            salience: 1.0,
            descriptors: "identity,owner",
        },
        MemEntry {
            title: "I use Linear for task tracking",
            content: "Linear is the primary task hub. Lab team for personal projects. Work team for professional.",
            memory_type: "identity",
            days_ago: 60.0,
            last_access_days: Some(2.0),
            salience: 0.8,
            descriptors: "preferences,linear,tasks",
        },
        MemEntry {
            title: "I use Craft for knowledge",
            content: "Craft Docs replaced Fabric.so as the primary knowledge store. Research, specs, and decisions go there.",
            memory_type: "identity",
            days_ago: 30.0,
            last_access_days: Some(1.0),
            salience: 0.9,
            descriptors: "preferences,craft,knowledge",
        },
        MemEntry {
            title: "Secrets in env files only",
            content: "All secrets in .env files with chmod 600. Never committed to git. Never in code.",
            memory_type: "identity",
            days_ago: 90.0,
            last_access_days: Some(2.0),
            salience: 1.0,
            descriptors: "security,secrets,convention",
        },
        MemEntry {
            title: "No over-engineering",
            content: "Don't add features beyond what's asked. Three similar lines are better than a premature abstraction.",
            memory_type: "identity",
            days_ago: 60.0,
            last_access_days: Some(1.0),
            salience: 0.9,
            descriptors: "philosophy,engineering",
        },
        MemEntry {
            title: "I value thorough testing",
            content: "Test after every deploy. curl against production tunnel. Don't ship without verification.",
            memory_type: "identity",
            days_ago: 45.0,
            last_access_days: Some(2.0),
            salience: 0.8,
            descriptors: "preferences,testing",
        },
        MemEntry {
            title: "Modern information science naming",
            content: "Use corpus, entries, artifacts, descriptors, indexed, abstract, source for digital naming.",
            memory_type: "identity",
            days_ago: 50.0,
            last_access_days: Some(3.0),
            salience: 0.7,
            descriptors: "conventions,naming,terminology",
        },
        MemEntry {
            title: "Bun for package management",
            content: "Use bun instead of npm/yarn for package management and running scripts.",
            memory_type: "identity",
            days_ago: 40.0,
            last_access_days: Some(2.0),
            salience: 0.7,
            descriptors: "preferences,bun,tools",
        },
        MemEntry {
            title: "Loopback-only bindings",
            content: "All services bind to 127.0.0.1 by default. External access only through Cloudflare tunnel.",
            memory_type: "identity",
            days_ago: 60.0,
            last_access_days: Some(5.0),
            salience: 0.8,
            descriptors: "security,networking,convention",
        },
        MemEntry {
            title: "I use VS Code Remote Tunnels",
            content: "Primary workspace: VS Code tunnel to beelink. Machine name: beelink. Full system access.",
            memory_type: "identity",
            days_ago: 90.0,
            last_access_days: Some(1.0),
            salience: 0.8,
            descriptors: "preferences,vscode,remote",
        },
        MemEntry {
            title: "No emojis unless asked",
            content: "Don't use emojis in communication unless explicitly requested by the user.",
            memory_type: "identity",
            days_ago: 30.0,
            last_access_days: Some(1.0),
            salience: 0.7,
            descriptors: "preferences,communication",
        },
        // === Stale memories (old, low salience, high decay) ===
        MemEntry {
            title: "Old migration strategy v1",
            content: "Original migration used manual SQL files. Replaced by inline init_schema migrations.",
            memory_type: "knowledge",
            days_ago: 100.0,
            last_access_days: Some(90.0),
            salience: 0.2,
            descriptors: "migration,old,deprecated",
        },
        MemEntry {
            title: "Tree-sitter was considered",
            content: "Considered tree-sitter for parsing but decided on universal chunking. Too many language grammars to maintain.",
            memory_type: "knowledge",
            days_ago: 95.0,
            last_access_days: Some(85.0),
            salience: 0.2,
            descriptors: "parsing,tree-sitter,rejected",
        },
        MemEntry {
            title: "Initial Hebbian association design",
            content: "Co-retrieved memories were linked via Hebbian associations. Feature was removed in v2 for simplicity.",
            memory_type: "knowledge",
            days_ago: 110.0,
            last_access_days: Some(95.0),
            salience: 0.1,
            descriptors: "hebbian,removed,v1",
        },
        MemEntry {
            title: "Entity extraction experiment",
            content: "Regex-based NER for entity extraction at write time. Removed in v2 — retrieval-time expansion works better.",
            memory_type: "knowledge",
            days_ago: 105.0,
            last_access_days: Some(90.0),
            salience: 0.15,
            descriptors: "entity,ner,removed",
        },
        MemEntry {
            title: "NLI contradiction detection",
            content: "Cross-encoder NLI model for contradiction detection. Removed — too slow and rarely triggered.",
            memory_type: "knowledge",
            days_ago: 100.0,
            last_access_days: Some(88.0),
            salience: 0.1,
            descriptors: "nli,contradiction,removed",
        },
        MemEntry {
            title: "Old 18-tool MCP design",
            content: "v1 had 18 MCP tools with separate endpoints for each operation. Collapsed to 3 tools in v2.",
            memory_type: "knowledge",
            days_ago: 95.0,
            last_access_days: Some(80.0),
            salience: 0.15,
            descriptors: "mcp,old-design,v1",
        },
        MemEntry {
            title: "Frequency boost was useful",
            content: "Original scoring included 0.15 * log2(1 + access_count) frequency boost. Removed for simplicity, salience captures it.",
            memory_type: "knowledge",
            days_ago: 90.0,
            last_access_days: Some(85.0),
            salience: 0.2,
            descriptors: "scoring,frequency,removed",
        },
        MemEntry {
            title: "Jotty app notes",
            content: "Jotty was a note-taking app. Fully decommissioned. Content migrated to Craft Docs.",
            memory_type: "knowledge",
            days_ago: 120.0,
            last_access_days: Some(100.0),
            salience: 0.1,
            descriptors: "jotty,decommissioned",
        },
        MemEntry {
            title: "Yak v2 chat app design",
            content: "Yak v2 was a chat frontend. Decommissioned because MCP tools provide all needed value without a custom UI.",
            memory_type: "knowledge",
            days_ago: 115.0,
            last_access_days: Some(95.0),
            salience: 0.1,
            descriptors: "yak,decommissioned,chat",
        },
        MemEntry {
            title: "Old CI with Travis",
            content: "Originally used Travis CI. Migrated to GitHub Actions for better Rust support.",
            memory_type: "knowledge",
            days_ago: 120.0,
            last_access_days: Some(110.0),
            salience: 0.1,
            descriptors: "ci,travis,old",
        },
        MemEntry {
            title: "Python-based MCP prototype",
            content: "First MCP server was Python/FastMCP. Replaced with TypeScript for consistency with other servers.",
            memory_type: "knowledge",
            days_ago: 100.0,
            last_access_days: Some(90.0),
            salience: 0.15,
            descriptors: "mcp,python,prototype",
        },
        MemEntry {
            title: "Redis cache experiment",
            content: "Briefly tried Redis for search result caching. Removed — SQLite WAL mode fast enough, simpler stack.",
            memory_type: "knowledge",
            days_ago: 110.0,
            last_access_days: Some(100.0),
            salience: 0.1,
            descriptors: "redis,cache,removed",
        },
        MemEntry {
            title: "gRPC transport attempt",
            content: "Tried gRPC for MCP transport. Switched to Streamable HTTP — simpler, works with web clients.",
            memory_type: "knowledge",
            days_ago: 105.0,
            last_access_days: Some(95.0),
            salience: 0.1,
            descriptors: "grpc,transport,rejected",
        },
        MemEntry {
            title: "Old Fabric.so integration",
            content: "Fabric.so was the knowledge store before Craft Docs. API was unreliable. Fully migrated.",
            memory_type: "knowledge",
            days_ago: 100.0,
            last_access_days: Some(90.0),
            salience: 0.1,
            descriptors: "fabric,old,migrated",
        },
        MemEntry {
            title: "Manual HNSW rebuild command",
            content: "v1 had explicit 'grasshopper hnsw rebuild' command. Removed in v2 — auto-rebuild on startup.",
            memory_type: "knowledge",
            days_ago: 95.0,
            last_access_days: Some(85.0),
            salience: 0.15,
            descriptors: "hnsw,command,removed",
        },
    ]
}

// ---------------------------------------------------------------------------
// Gold queries — harder, with larger gold sets and ambiguous phrasing
// ---------------------------------------------------------------------------

struct GoldQuery {
    query: &'static str,
    /// Expected memory titles (partial match OK).
    expected_titles: Vec<&'static str>,
    test_type: &'static str,
}

fn gold_queries() -> Vec<GoldQuery> {
    vec![
        // --- basic-retrieval: Queries with clear expected results ---
        GoldQuery {
            query: "what database do we use",
            expected_titles: vec![
                "SQLite for all storage",
                "WAL mode",
                "SQLite connection pooling",
            ],
            test_type: "basic-retrieval",
        },
        GoldQuery {
            query: "how does reranking work",
            expected_titles: vec![
                "Cross-encoder reranking",
                "reranker score normalization",
                "BM25 weights",
            ],
            test_type: "basic-retrieval",
        },
        GoldQuery {
            query: "embedding model details",
            expected_titles: vec![
                "Embedding dimension is 768",
                "ONNX for local inference",
                "Lazy model initialization",
            ],
            test_type: "basic-retrieval",
        },
        GoldQuery {
            query: "cloudflare tunnel setup",
            expected_titles: vec![
                "Cloudflare tunnel for external access",
                "Cloudflare tunnel reconfiguration",
                "Cloudflare tunnel ingress routing",
            ],
            test_type: "basic-retrieval",
        },
        GoldQuery {
            query: "how is search scoring calculated",
            expected_titles: vec![
                "Cognitive scoring formula",
                "RRF fusion with k=60",
                "BM25 weights",
                "Relevance threshold gating",
            ],
            test_type: "basic-retrieval",
        },
        GoldQuery {
            query: "symbol definitions and navigation",
            expected_titles: vec![
                "Symbol navigation is language-agnostic",
                "Language-agnostic chunking",
                "BM25 weights",
                "Code structure graph for navigation",
            ],
            test_type: "basic-retrieval",
        },
        GoldQuery {
            query: "content deduplication",
            expected_titles: vec![
                "Content hash for exact dedup",
                "Embedding dimension",
                "Near-duplicate detection via embeddings",
            ],
            test_type: "basic-retrieval",
        },
        GoldQuery {
            query: "HNSW index performance",
            expected_titles: vec![
                "HNSW for O(log N)",
                "HNSW index rebuild",
                "Brute force guard",
            ],
            test_type: "basic-retrieval",
        },
        // --- ambiguous: Multiple valid interpretations, ranking matters ---
        GoldQuery {
            query: "how do we handle security",
            expected_titles: vec![
                "OAuth 2.1 with PKCE",
                "Cloudflare Access email OTP",
                "SSH hardening",
                "Env file permission model",
                "Kernel sysctl hardening",
                "Container security constraints",
                "Rotate secrets",
            ],
            test_type: "ambiguous",
        },
        GoldQuery {
            query: "text search bugs",
            expected_titles: vec![
                "FTS query escaping bug",
                "Unicode in FTS",
                "camelCase tokenizer",
                "FTS ranking inconsistency",
            ],
            test_type: "ambiguous",
        },
        GoldQuery {
            query: "how do migrations work",
            expected_titles: vec![
                "Old migration strategy v1",
                "Database migration process",
                "Migrate database schema manually",
            ],
            test_type: "ambiguous",
        },
        GoldQuery {
            query: "memory and OOM issues",
            expected_titles: vec![
                "HNSW deserialization OOM",
                "batch embedding OOM",
                "Brute force guard",
                "search timeout on large DB",
            ],
            test_type: "ambiguous",
        },
        GoldQuery {
            query: "concurrency problems",
            expected_titles: vec![
                "WAL mode for concurrency",
                "concurrent index lock",
                "Poisoned mutex",
                "hybrid search dedup",
            ],
            test_type: "ambiguous",
        },
        // --- cross-type: Answer spans knowledge + procedure + episode ---
        GoldQuery {
            query: "everything about deployment",
            expected_titles: vec![
                "Deploy grasshopper binary",
                "Single binary deployment",
                "Systemd for service management",
                "MCP server deployment",
                "Cloudflare tunnel for external access",
                "deploy script race condition",
                "Deploy MCP auth proxy",
                "Update deployed service",
            ],
            test_type: "cross-type",
        },
        GoldQuery {
            query: "backup and disaster recovery",
            expected_titles: vec![
                "Restic for nightly backups",
                "Backup restore procedure",
                "Backup encryption with restic",
                "Backup coverage and exclusions",
                "backup verification",
                "backup verification failure",
            ],
            test_type: "cross-type",
        },
        GoldQuery {
            query: "all MCP related operations",
            expected_titles: vec![
                "MCP server with 3 tools",
                "MCP server deployment",
                "Add new MCP tool",
                "Onboard new MCP server",
                "MCP OAuth shared module",
                "Local MCP loopback bypass",
                "MCP tool schema validation",
                "Deploy MCP auth proxy",
                "Old 18-tool MCP design",
            ],
            test_type: "cross-type",
        },
        // --- paraphrased: No keyword overlap with stored titles, needs semantic ---
        GoldQuery {
            query: "making things faster",
            expected_titles: vec![
                "Cognitive scoring formula",
                "HNSW for O(log N)",
                "Query expansion for recall",
                "Brute force guard",
                "Relevance threshold gating",
            ],
            test_type: "paraphrased",
        },
        GoldQuery {
            query: "keeping data safe",
            expected_titles: vec![
                "Restic for nightly backups",
                "Cloudflare Access email OTP",
                "Env file permission model",
                "Backup encryption",
            ],
            test_type: "paraphrased",
        },
        GoldQuery {
            query: "connecting from outside the network",
            expected_titles: vec![
                "Tailscale VPN",
                "Cloudflare tunnel for external access",
                "OAuth 2.1 with PKCE",
                "SSH hardening",
            ],
            test_type: "paraphrased",
        },
        // --- procedure-recall ---
        GoldQuery {
            query: "deploy grasshopper",
            expected_titles: vec![
                "Deploy grasshopper binary",
                "MCP server deployment",
                "Deploy MCP auth proxy",
            ],
            test_type: "procedure-recall",
        },
        GoldQuery {
            query: "backup restore steps",
            expected_titles: vec![
                "Backup restore procedure",
                "Restic for nightly backups",
                "Run backup verification",
            ],
            test_type: "procedure-recall",
        },
        GoldQuery {
            query: "how to add a new MCP tool",
            expected_titles: vec![
                "Add new MCP tool",
                "MCP server with 3 tools",
                "Onboard new MCP server",
            ],
            test_type: "procedure-recall",
        },
        GoldQuery {
            query: "debugging production issues",
            expected_titles: vec![
                "Debug production issue",
                "search timeout on large DB",
                "Monitor service health",
            ],
            test_type: "procedure-recall",
        },
        GoldQuery {
            query: "Docker container management",
            expected_titles: vec![
                "Docker compose update",
                "Docker log rotation",
                "Container security constraints",
            ],
            test_type: "procedure-recall",
        },
        GoldQuery {
            query: "SSH key management",
            expected_titles: vec![
                "SSH key rotation",
                "SSH hardening",
                "SSH authorized keys management",
                "Cloudflare Access email OTP",
            ],
            test_type: "procedure-recall",
        },
        GoldQuery {
            query: "monitoring and health checks",
            expected_titles: vec![
                "Monitor service health",
                "Benchmark grasshopper search",
                "Debug production issue",
                "Systemd for service management",
            ],
            test_type: "procedure-recall",
        },
        GoldQuery {
            query: "testing workflow",
            expected_titles: vec![
                "Run full test suite",
                "Benchmark grasshopper search",
                "Deploy grasshopper binary",
                "CI test pipeline",
            ],
            test_type: "procedure-recall",
        },
        // --- recency+episode: Recent bugs should rank high ---
        GoldQuery {
            query: "Brotli compression fix",
            expected_titles: vec![
                "Brotli compression issue",
                "Cloudflare tunnel for external access",
                "MCP OAuth shared module",
            ],
            test_type: "recency+episode",
        },
        // --- ranking-validation: Asserts P@1 or P@3, not just recall ---
        GoldQuery {
            query: "recent bugs I fixed",
            // days_ago 1-4 should outrank days_ago 10-15
            expected_titles: vec![
                "MCP tool schema validation",  // days_ago=1
                "FTS query escaping bug",      // days_ago=2
                "concurrent index lock",       // days_ago=3
                "FTS ranking inconsistency",   // days_ago=3
                "identity memories appearing", // days_ago=3
            ],
            test_type: "ranking-validation",
        },
        // --- decay-test: Old memories should decay, stale content should rank low ---
        GoldQuery {
            query: "old migration strategy",
            expected_titles: vec![
                "Old migration strategy",
                "Database migration process",
                "Migrate database schema",
            ],
            test_type: "decay-test",
        },
        GoldQuery {
            query: "removed features from v1",
            expected_titles: vec![
                "Hebbian association",
                "Entity extraction",
                "NLI contradiction",
                "Old 18-tool MCP",
                "Frequency boost",
                "Manual HNSW rebuild",
            ],
            test_type: "decay-test",
        },
        GoldQuery {
            query: "decommissioned projects",
            expected_titles: vec!["Jotty app", "Yak v2 chat app", "Old Fabric.so"],
            test_type: "decay-test",
        },
        // --- identity-exclusion: Identity memories should not appear in results ---
        GoldQuery {
            query: "naming conventions",
            expected_titles: vec!["kebab-case for projects", "information science naming"],
            test_type: "identity-exclusion",
        },
        // --- true-negative: Topics completely absent from corpus ---
        GoldQuery {
            query: "blockchain consensus mechanism",
            expected_titles: vec![],
            test_type: "true-negative",
        },
        GoldQuery {
            query: "GPU CUDA kernel launch",
            expected_titles: vec![],
            test_type: "true-negative",
        },
        GoldQuery {
            query: "Kubernetes pods and services",
            expected_titles: vec![],
            test_type: "true-negative",
        },
        GoldQuery {
            query: "machine learning training pipeline",
            expected_titles: vec![],
            test_type: "true-negative",
        },
    ]
}

// ---------------------------------------------------------------------------
// Gold set construction (INDEPENDENT of search results)
// ---------------------------------------------------------------------------

/// Build gold sets by scanning ALL memories in the database, not search results.
/// Returns one Vec<String> per query, containing chunk IDs of relevant memories.
/// Gold sets are constant — they don't change based on what the search returns.
///
/// Note: identity memories are excluded from gold sets because unified_search
/// filters them out. Including them would create false negatives in the metrics.
fn build_memory_gold_sets(store: &Store, queries: &[GoldQuery]) -> Vec<Vec<String>> {
    let mut stmt = store
        .conn()
        .prepare(
            "SELECT id, COALESCE(title,''), COALESCE(content,''), memory_type \
         FROM chunks WHERE kind = 'memory' AND archived = 0",
        )
        .unwrap();

    let all_memories: Vec<(i64, String, String)> = stmt
        .query_map([], |row| {
            let id: i64 = row.get(0)?;
            let title: String = row.get(1)?;
            let mtype: String = row.get::<_, Option<String>>(3)?.unwrap_or_default();
            Ok((id, title, mtype))
        })
        .unwrap()
        .filter_map(|r| r.ok())
        .collect();

    eprintln!(
        "Building gold sets from {} total memories...",
        all_memories.len()
    );

    queries
        .iter()
        .enumerate()
        .map(|(qi, gq)| {
            // Skip gold set construction for queries that don't measure retrieval quality
            if gq.test_type == "true-negative"
                || gq.test_type == "identity-exclusion"
                || gq.expected_titles.is_empty()
            {
                return vec![];
            }
            let gold: Vec<String> = all_memories
                .iter()
                .filter(|(_, title, mtype)| {
                    // Exclude identity memories from gold set (search filters them)
                    if mtype == "identity" {
                        return false;
                    }
                    gq.expected_titles
                        .iter()
                        .any(|exp| title.to_lowercase().contains(&exp.to_lowercase()))
                })
                .map(|(id, _, _)| id.to_string())
                .collect();
            assert!(
                !gold.is_empty(),
                "Q{qi} '{}' has no gold memories — expected titles {:?} matched nothing in DB",
                gq.query,
                gq.expected_titles
            );
            eprintln!("  Q{qi}: {:50} {} gold memories", gq.query, gold.len());
            gold
        })
        .collect()
}

// ---------------------------------------------------------------------------
// Search configs
// ---------------------------------------------------------------------------

enum SearchConfig {
    FtsOnly,
    Hybrid,
}

fn run_memory_search(
    store: &Store,
    query: &str,
    config: &SearchConfig,
    embedder: &mut Option<Embedder>,
    hnsw: &Option<HnswIndex>,
) -> Vec<grasshopper::store::SearchHit> {
    let limit = 10;
    match config {
        SearchConfig::FtsOnly => {
            let ctx = SearchContext {
                store,
                query,
                kind_filter: Some("memory"),
                limit,
                threshold: None,
                embedder: None,
                reranker: None,
                hnsw: None,
            };
            unified_search(ctx)
                .expect("search must not fail in benchmark")
                .hits
        }
        SearchConfig::Hybrid => {
            let ctx = SearchContext {
                store,
                query,
                kind_filter: Some("memory"),
                limit,
                threshold: None,
                embedder: embedder.as_mut(),
                reranker: None,
                hnsw: hnsw.as_ref(),
            };
            unified_search(ctx)
                .expect("search must not fail in benchmark")
                .hits
        }
    }
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

fn main() {
    let start = Instant::now();
    eprintln!("=== Memory Retrieval Accuracy Benchmark ===\n");

    // Setup: create store with memories
    let dir = TempDir::new().unwrap();
    let db_path = dir.path().join("bench.db");
    let store = Store::open(&db_path).unwrap();

    let corpus = memory_corpus();
    eprintln!("Storing {} memories...", corpus.len());
    for (i, entry) in corpus.iter().enumerate() {
        let created =
            chrono::Utc::now() - chrono::Duration::seconds((entry.days_ago * 86400.0) as i64);
        let last_accessed = entry.last_access_days.map(|d| {
            (chrono::Utc::now() - chrono::Duration::seconds((d * 86400.0) as i64)).to_rfc3339()
        });
        let hash = format!("bench-{i}");

        let id = store
            .insert_memory(&MemoryParams {
                title: entry.title,
                content: entry.content,
                memory_type: entry.memory_type,
                descriptors: entry.descriptors,
                salience: entry.salience,
                content_hash: &hash,
            })
            .unwrap();

        // Set controlled timestamps
        let la_clause = match &last_accessed {
            Some(la) => format!("last_accessed = '{la}',"),
            None => String::new(),
        };
        store
            .execute_batch(&format!(
                "UPDATE chunks SET created_at = '{}', {la_clause} salience = {} WHERE id = {}",
                created.to_rfc3339(),
                entry.salience,
                id,
            ))
            .unwrap();
    }

    // Rebuild FTS
    store
        .execute_batch(
            "DELETE FROM chunks_fts; \
         INSERT INTO chunks_fts (rowid, title, content, snippet, symbol_name, descriptors) \
         SELECT id, code_expand(COALESCE(title,'')), code_expand(COALESCE(content,'')), \
                code_expand(COALESCE(snippet,'')), code_expand(COALESCE(symbol_name,'')), \
                COALESCE(descriptors,'') \
         FROM chunks",
        )
        .unwrap();

    let (_, mem_count) = store.count_by_kind().unwrap();
    eprintln!("Stored {mem_count} memories\n");

    // Setup embedder and HNSW for hybrid config
    eprintln!("Loading embedder for hybrid search...");
    let cache_dir = default_cache_dir();
    let mut embedder = Embedder::new(&cache_dir).unwrap();

    // Embed all memories
    eprintln!("Embedding {} memories...", mem_count);
    let mut stmt = store
        .conn()
        .prepare(
            "SELECT id, COALESCE(title,''), COALESCE(content,'') FROM chunks WHERE kind = 'memory'",
        )
        .unwrap();
    let mem_rows: Vec<(i64, String, String)> = stmt
        .query_map([], |row| Ok((row.get(0)?, row.get(1)?, row.get(2)?)))
        .unwrap()
        .filter_map(|r| r.ok())
        .collect();
    drop(stmt);

    // Embed in batches of 32
    let batch_size = 32;
    for chunk in mem_rows.chunks(batch_size) {
        let texts: Vec<String> = chunk
            .iter()
            .map(|(_, title, content)| format!("{}\n{}", title, content))
            .collect();
        let vectors = embedder.embed_batch(&texts).unwrap();
        let items: Vec<(i64, &[f32], &str)> = chunk
            .iter()
            .zip(vectors.iter())
            .map(|((id, _, _), v)| (*id, v.as_slice(), grasshopper::code::embed::MODEL_NAME))
            .collect();
        store.batch_upsert_embeddings(&items).unwrap();
    }
    eprintln!("Embeddings stored.");

    // Build HNSW
    let embeddings = store.get_all_embeddings().unwrap();
    let hnsw = if !embeddings.is_empty() {
        Some(HnswIndex::from_embeddings(&embeddings).unwrap())
    } else {
        None
    };
    eprintln!("HNSW built with {} points\n", embeddings.len());

    let mut embedder_opt = Some(embedder);

    let queries = gold_queries();

    // Build gold sets ONCE from all memories — independent of search results
    let gold_sets = build_memory_gold_sets(&store, &queries);
    eprintln!();

    let mut report = BenchReport::new("accuracy-memory");
    report.add_section(
        "setup",
        serde_json::json!({
            "memory_count": mem_count,
            "queries": queries.len(),
            "search_modes": ["FTS-only", "Hybrid (FTS + embeddings + HNSW)"],
            "types": {
                "knowledge": corpus.iter().filter(|m| m.memory_type == "knowledge").count(),
                "episode": corpus.iter().filter(|m| m.memory_type == "episode").count(),
                "procedure": corpus.iter().filter(|m| m.memory_type == "procedure").count(),
                "identity": corpus.iter().filter(|m| m.memory_type == "identity").count(),
            },
            "gold_set_sizes": gold_sets.iter().map(|g| g.len()).collect::<Vec<_>>(),
        }),
    );

    let configs = [
        ("FTS-only", SearchConfig::FtsOnly),
        ("Hybrid", SearchConfig::Hybrid),
    ];

    // Collect per-query metrics for each config to compute deltas afterward.
    // Each entry: (query_name, recall, mrr)
    let mut per_config_metrics: Vec<Vec<(String, f64, f64)>> = Vec::new();

    for (config_name, config) in &configs {
        eprintln!("--- {config_name} ---");
        let mut query_results = Vec::new();
        let mut config_query_metrics: Vec<(String, f64, f64)> = Vec::new();
        let mut total_recall = 0.0;
        let mut total_p1 = 0.0;
        let mut total_p3 = 0.0;
        let mut total_p5 = 0.0;
        let mut total_mrr_val = 0.0;
        let mut total_ndcg = 0.0;
        let mut identity_leaks = 0;
        let mut non_negative_count = 0;
        let mut by_type: std::collections::HashMap<&str, Vec<f64>> =
            std::collections::HashMap::new();

        for (qi, gq) in queries.iter().enumerate() {
            // Use a savepoint to prevent search side effects (salience/last_accessed updates)
            // from leaking between queries — ensures order-independent metrics.
            store.execute_batch("SAVEPOINT bench_query").unwrap();

            let results = run_memory_search(&store, gq.query, config, &mut embedder_opt, &hnsw);

            // Roll back salience/last_accessed mutations before processing results
            store.execute_batch("ROLLBACK TO bench_query").unwrap();
            store.execute_batch("RELEASE bench_query").unwrap();

            // Check for identity memory leaks
            let identity_in_results = results
                .iter()
                .any(|h| h.memory_type.as_deref() == Some("identity"));
            if identity_in_results {
                identity_leaks += 1;
            }

            if gq.test_type == "true-negative" {
                // Any hit is a false positive — the topic shouldn't exist in our corpus
                let false_positive = !results.is_empty();
                query_results.push(serde_json::json!({
                    "query": gq.query,
                    "type": gq.test_type,
                    "false_positive": false_positive,
                    "hits": results.len(),
                    "identity_leak": identity_in_results,
                }));
                continue;
            }

            if gq.test_type == "identity-exclusion" {
                // Identity memories should be filtered out by search. Check if any leaked.
                let identity_leaked = results
                    .iter()
                    .any(|h| h.memory_type.as_deref() == Some("identity"));
                query_results.push(serde_json::json!({
                    "query": gq.query,
                    "type": gq.test_type,
                    "identity_leaked": identity_leaked,
                    "hits": results.len(),
                }));
                continue;
            }

            non_negative_count += 1;

            // Ranked IDs: chunk IDs in the order returned (guaranteed unique)
            let ranked_ids: Vec<String> = results.iter().map(|h| h.id.to_string()).collect();
            // Gold IDs: pre-computed from ALL memories, constant
            let gold_ids = &gold_sets[qi];

            // IR metrics using independent gold set
            let p1 = precision_at_k(&ranked_ids, gold_ids, 1);
            let p3 = precision_at_k(&ranked_ids, gold_ids, 3);
            let p5 = precision_at_k(&ranked_ids, gold_ids, 5);
            let m = mrr(&ranked_ids, gold_ids);
            let ndcg = ndcg_at_k(&ranked_ids, gold_ids, 10);

            // Recall: fraction of expected titles found anywhere in results
            let result_titles: Vec<String> = results.iter().map(|h| h.title.clone()).collect();
            let found_titles: Vec<String> = gq
                .expected_titles
                .iter()
                .filter(|&&exp| {
                    result_titles
                        .iter()
                        .any(|rt| rt.to_lowercase().contains(&exp.to_lowercase()))
                })
                .map(|s| s.to_string())
                .collect();
            let recall = if gq.expected_titles.is_empty() {
                0.0
            } else {
                found_titles.len() as f64 / gq.expected_titles.len() as f64
            };

            total_recall += recall;
            total_p1 += p1;
            total_p3 += p3;
            total_p5 += p5;
            total_mrr_val += m;
            total_ndcg += ndcg;

            by_type.entry(gq.test_type).or_default().push(recall);
            config_query_metrics.push((gq.query.to_string(), recall, m));

            eprintln!(
                "  {:50} P@1={:.2} MRR={:.2} recall={:.0}% ({}/{}) gold={}  type={}",
                gq.query,
                p1,
                m,
                recall * 100.0,
                found_titles.len(),
                gq.expected_titles.len(),
                gold_ids.len(),
                gq.test_type,
            );

            query_results.push(serde_json::json!({
                "query": gq.query,
                "type": gq.test_type,
                "hits": results.len(),
                "expected": gq.expected_titles,
                "found": found_titles,
                "gold_set_size": gold_ids.len(),
                "recall": recall,
                "p@1": p1,
                "p@3": p3,
                "p@5": p5,
                "mrr": m,
                "ndcg@10": ndcg,
                "identity_leak": identity_in_results,
                "result_types": results.iter().map(|h| h.memory_type.as_deref().unwrap_or("?")).collect::<Vec<_>>(),
            }));
        }

        let n = non_negative_count as f64;
        eprintln!("\n  --- {config_name} Aggregate ---");
        eprintln!("  Avg recall:   {:.1}%", total_recall / n * 100.0);
        eprintln!("  Avg P@1:      {:.3}", total_p1 / n);
        eprintln!("  Avg P@3:      {:.3}", total_p3 / n);
        eprintln!("  Avg P@5:      {:.3}", total_p5 / n);
        eprintln!("  Avg MRR:      {:.3}", total_mrr_val / n);
        eprintln!("  Avg NDCG@10:  {:.3}", total_ndcg / n);
        eprintln!("  Identity leaks: {identity_leaks}");

        // Per-type breakdown
        eprintln!("\n  --- By Test Type ---");
        for (ttype, scores) in &by_type {
            let avg = scores.iter().sum::<f64>() / scores.len() as f64;
            eprintln!(
                "  {ttype:25} avg_recall={:.1}% (n={})",
                avg * 100.0,
                scores.len()
            );
        }
        eprintln!();

        report.add_section(
            config_name,
            serde_json::json!({
                "aggregate": {
                    "avg_recall": total_recall / n,
                    "avg_p@1": total_p1 / n,
                    "avg_p@3": total_p3 / n,
                    "avg_p@5": total_p5 / n,
                    "avg_mrr": total_mrr_val / n,
                    "avg_ndcg@10": total_ndcg / n,
                    "identity_leaks": identity_leaks,
                },
                "by_type": by_type.iter().map(|(k, v)| {
                    (k.to_string(), serde_json::json!({
                        "avg_recall": v.iter().sum::<f64>() / v.len() as f64,
                        "count": v.len(),
                    }))
                }).collect::<serde_json::Map<String, serde_json::Value>>(),
                "queries": query_results,
            }),
        );

        per_config_metrics.push(config_query_metrics);
    }

    // --- FTS-vs-Hybrid delta comparison ---
    if per_config_metrics.len() == 2 {
        let fts_metrics = &per_config_metrics[0];
        let hybrid_metrics = &per_config_metrics[1];

        let mut deltas = Vec::new();
        let mut improved = 0;
        let mut degraded = 0;
        let mut unchanged = 0;

        eprintln!("--- FTS-vs-Hybrid Delta ---");
        for (fts, hybrid) in fts_metrics.iter().zip(hybrid_metrics.iter()) {
            let recall_delta = hybrid.1 - fts.1;
            let mrr_delta = hybrid.2 - fts.2;

            if recall_delta > 0.01 || mrr_delta > 0.01 {
                improved += 1;
            } else if recall_delta < -0.01 || mrr_delta < -0.01 {
                degraded += 1;
            } else {
                unchanged += 1;
            }

            eprintln!(
                "  {:50} recall: {:.0}%→{:.0}% ({:+.0}%)  MRR: {:.2}→{:.2} ({:+.2})",
                fts.0,
                fts.1 * 100.0,
                hybrid.1 * 100.0,
                recall_delta * 100.0,
                fts.2,
                hybrid.2,
                mrr_delta,
            );

            deltas.push(serde_json::json!({
                "query": fts.0,
                "fts_recall": fts.1,
                "hybrid_recall": hybrid.1,
                "recall_delta": recall_delta,
                "fts_mrr": fts.2,
                "hybrid_mrr": hybrid.2,
                "mrr_delta": mrr_delta,
            }));
        }

        eprintln!(
            "\n  Hybrid improved {} queries, degraded {}, unchanged {}\n",
            improved, degraded, unchanged
        );

        report.add_section(
            "delta",
            serde_json::json!({
                "improved": improved,
                "degraded": degraded,
                "unchanged": unchanged,
                "queries": deltas,
            }),
        );
    }

    report.add_section(
        "timing",
        serde_json::json!({
            "total_seconds": start.elapsed().as_secs_f64(),
        }),
    );

    report.save("bench/results/accuracy-memory.json").unwrap();
    eprintln!("Done in {:.1}s", start.elapsed().as_secs_f64());
}
