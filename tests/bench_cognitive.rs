// =============================================================================
// Grasshopper Cognitive Scoring Ablation Benchmark
// =============================================================================
//
// Tests each cognitive scoring component independently against a baseline
// (FTS + RRF only) to measure which components earn their complexity.
//
// Run: cargo test bench_cognitive -- --nocapture
//
// Components under test:
//   1. Recency boost:   0.2 * (1 - days/30) for entries < 30 days
//   2. Frequency boost:  0.15 * log2(1 + access_count)
//   3. Salience factor:  0.5 + salience => maps [0,1] -> [0.5,1.5]
//   4. Decay factor:     e^(-λ * days) where λ varies by memory_type
//   5. Learned decay:    EWA of access intervals -> derived λ
//   6. Entity augment:   regex NER -> entity edges -> inject into search
//   7. Full pipeline:    all components combined
// =============================================================================

use grasshopper::memory::cognitive_score;
use grasshopper::store::{SearchHit, Store};
use tempfile::TempDir;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/// Build a SearchHit with full control over cognitive fields.
#[allow(clippy::too_many_arguments)]
fn make_hit(
    id: i64,
    title: &str,
    rrf_score: f64,
    memory_type: &str,
    created_days_ago: f64,
    last_accessed_days_ago: Option<f64>,
    access_count: i64,
    salience: f64,
) -> SearchHit {
    let created_at = iso_days_ago(created_days_ago);
    let last_accessed = last_accessed_days_ago.map(iso_days_ago);

    SearchHit {
        id,
        kind: "memory".to_string(),
        file_path: None,
        symbol_name: None,
        symbol_kind: None,
        signature: None,
        title: title.to_string(),
        snippet: String::new(),
        start_line: None,
        end_line: None,
        memory_type: Some(memory_type.to_string()),
        score: rrf_score,
        reranker_score: None,
        access_count,
        last_accessed,
        salience,
        created_at,
        archived: false,
        descriptors: String::new(),
    }
}

fn iso_days_ago(days: f64) -> String {
    let dt = chrono::Utc::now() - chrono::Duration::seconds((days * 86400.0) as i64);
    dt.to_rfc3339()
}

/// Score a hit using only baseline (RRF score passthrough, no cognitive adjustments).
fn score_baseline(hit: &SearchHit) -> f64 {
    hit.score
}

/// Score with only recency boost applied.
fn score_recency_only(hit: &SearchHit) -> f64 {
    let days = days_since(&hit.created_at);
    let recency = if days >= 30.0 {
        0.0
    } else {
        0.2 * (1.0 - days / 30.0)
    };
    hit.score * (1.0 + recency)
}

/// Score with only frequency boost applied.
fn score_frequency_only(hit: &SearchHit) -> f64 {
    let freq = 0.15 * (1.0 + hit.access_count as f64).log2();
    hit.score * (1.0 + freq)
}

/// Score with only salience factor applied.
fn score_salience_only(hit: &SearchHit) -> f64 {
    let sal = 0.5 + hit.salience;
    hit.score * sal
}

/// Score with only decay factor applied (static λ).
fn score_decay_only(hit: &SearchHit) -> f64 {
    let mtype = hit.memory_type.as_deref().unwrap_or("knowledge");
    let lambda = match mtype {
        "identity" => 0.0,
        "knowledge" => 0.005,
        "episode" => 0.023,
        "procedure" => 0.01,
        _ => 0.015,
    };
    let days_la = hit
        .last_accessed
        .as_deref()
        .map(days_since)
        .unwrap_or_else(|| days_since(&hit.created_at));
    let decay = (-lambda * days_la).exp();
    hit.score * decay
}

/// Full cognitive score (delegates to the real function).
fn score_full(hit: &SearchHit) -> f64 {
    cognitive_score(hit)
}

fn days_since(iso: &str) -> f64 {
    let Ok(dt) = chrono::DateTime::parse_from_rfc3339(iso) else {
        return 0.0;
    };
    let dur = chrono::Utc::now().signed_duration_since(dt);
    (dur.num_seconds() as f64 / 86400.0).max(0.0)
}

// ---------------------------------------------------------------------------
// Test corpus: 40 memory entries with varied characteristics
// ---------------------------------------------------------------------------

struct TestEntry {
    id: i64,
    title: &'static str,
    content: &'static str,
    memory_type: &'static str,
    created_days_ago: f64,
    last_accessed_days_ago: Option<f64>,
    access_count: i64,
    salience: f64,
    descriptors: &'static str,
}

fn test_corpus() -> Vec<TestEntry> {
    vec![
        // --- Recent, high-salience knowledge ---
        TestEntry {
            id: 1,
            title: "Rust async runtime",
            content: "Tokio is the primary async runtime for Rust. Use spawn_blocking for CPU-bound work.",
            memory_type: "knowledge",
            created_days_ago: 1.0,
            last_accessed_days_ago: Some(0.5),
            access_count: 20,
            salience: 0.9,
            descriptors: "rust,async,tokio",
        },
        TestEntry {
            id: 2,
            title: "SQLite WAL mode",
            content: "SQLite WAL mode allows concurrent reads during writes. Set PRAGMA journal_mode=WAL.",
            memory_type: "knowledge",
            created_days_ago: 2.0,
            last_accessed_days_ago: Some(1.0),
            access_count: 15,
            salience: 0.8,
            descriptors: "sqlite,database,wal",
        },
        // --- Old, rarely accessed knowledge ---
        TestEntry {
            id: 3,
            title: "Python virtualenv",
            content: "Use python -m venv to create virtual environments. Activate with source bin/activate.",
            memory_type: "knowledge",
            created_days_ago: 90.0,
            last_accessed_days_ago: Some(60.0),
            access_count: 1,
            salience: 0.3,
            descriptors: "python,virtualenv",
        },
        TestEntry {
            id: 4,
            title: "Git rebase workflow",
            content: "Prefer rebase over merge for linear history. Use git rebase -i for interactive cleanup.",
            memory_type: "knowledge",
            created_days_ago: 45.0,
            last_accessed_days_ago: Some(30.0),
            access_count: 5,
            salience: 0.5,
            descriptors: "git,rebase,workflow",
        },
        // --- Recent episodes ---
        TestEntry {
            id: 5,
            title: "Deployed grasshopper v0.8",
            content: "Today deployed grasshopper v0.8 with entity extraction. Fixed the NLI model loading.",
            memory_type: "episode",
            created_days_ago: 0.5,
            last_accessed_days_ago: Some(0.1),
            access_count: 3,
            salience: 0.7,
            descriptors: "deploy,grasshopper",
        },
        TestEntry {
            id: 6,
            title: "Debugged memory leak in embedder",
            content: "Yesterday found a memory leak in the ONNX embedder. The model cache was not bounded.",
            memory_type: "episode",
            created_days_ago: 1.5,
            last_accessed_days_ago: Some(1.0),
            access_count: 2,
            salience: 0.6,
            descriptors: "debug,embedder,memory-leak",
        },
        // --- Old episodes (should decay fast) ---
        TestEntry {
            id: 7,
            title: "Set up CI pipeline",
            content: "Last month set up GitHub Actions CI with cargo test and clippy checks.",
            memory_type: "episode",
            created_days_ago: 35.0,
            last_accessed_days_ago: Some(30.0),
            access_count: 1,
            salience: 0.4,
            descriptors: "ci,github-actions",
        },
        TestEntry {
            id: 8,
            title: "Fixed DNS resolution",
            content: "2025-01-15 fixed DNS resolution in the Docker container by adding custom resolv.conf.",
            memory_type: "episode",
            created_days_ago: 60.0,
            last_accessed_days_ago: None,
            access_count: 0,
            salience: 0.3,
            descriptors: "dns,docker",
        },
        // --- Procedures ---
        TestEntry {
            id: 9,
            title: "Deploy grasshopper procedure",
            content: "Step 1: cargo build --release. Step 2: sudo cp target/release/grasshopper /usr/local/bin/. Step 3: sudo systemctl restart grasshopper.",
            memory_type: "procedure",
            created_days_ago: 10.0,
            last_accessed_days_ago: Some(2.0),
            access_count: 8,
            salience: 0.7,
            descriptors: "deploy,grasshopper,procedure",
        },
        TestEntry {
            id: 10,
            title: "Backup restore procedure",
            content: "How to restore from restic backup: Step 1: source /etc/restic/r2.env. Step 2: restic restore latest --target /tmp/restore.",
            memory_type: "procedure",
            created_days_ago: 20.0,
            last_accessed_days_ago: Some(15.0),
            access_count: 3,
            salience: 0.5,
            descriptors: "backup,restore,restic",
        },
        // --- Identity memories (never decay) ---
        TestEntry {
            id: 11,
            title: "I prefer simple solutions",
            content: "I always prefer the simplest solution that works. 80/20 rule. No unnecessary abstractions.",
            memory_type: "identity",
            created_days_ago: 30.0,
            last_accessed_days_ago: Some(5.0),
            access_count: 50,
            salience: 1.0,
            descriptors: "preferences,simplicity",
        },
        TestEntry {
            id: 12,
            title: "I use Rust for systems work",
            content: "I use Rust for all systems-level projects. Prefer compiled languages.",
            memory_type: "identity",
            created_days_ago: 60.0,
            last_accessed_days_ago: Some(1.0),
            access_count: 100,
            salience: 1.0,
            descriptors: "preferences,rust",
        },
        // --- High frequency, low salience ---
        TestEntry {
            id: 13,
            title: "Common git commands",
            content: "git status, git add, git commit, git push. Basic git workflow for daily use.",
            memory_type: "knowledge",
            created_days_ago: 50.0,
            last_accessed_days_ago: Some(0.5),
            access_count: 100,
            salience: 0.2,
            descriptors: "git,commands",
        },
        // --- Low frequency, high salience ---
        TestEntry {
            id: 14,
            title: "Security incident response",
            content: "If a security incident occurs: isolate the system, check logs, rotate credentials, notify team.",
            memory_type: "procedure",
            created_days_ago: 40.0,
            last_accessed_days_ago: Some(40.0),
            access_count: 0,
            salience: 0.9,
            descriptors: "security,incident,response",
        },
        // --- Entity-rich entries ---
        TestEntry {
            id: 15,
            title: "Traefik configuration",
            content: "Traefik reverse proxy at ~/services/traefik/. Config in /etc/traefik/traefik.yml. Dashboard at https://traefik.example.com",
            memory_type: "knowledge",
            created_days_ago: 15.0,
            last_accessed_days_ago: Some(3.0),
            access_count: 10,
            salience: 0.7,
            descriptors: "traefik,proxy,config",
        },
        TestEntry {
            id: 16,
            title: "PocketBase corpus setup",
            content: "PocketBase runs at ~/projects/pocketbase-corpus/. Docker compose on port 8090. Accessible via reverse proxy.",
            memory_type: "knowledge",
            created_days_ago: 20.0,
            last_accessed_days_ago: Some(5.0),
            access_count: 12,
            salience: 0.6,
            descriptors: "pocketbase,corpus,docker",
        },
        // --- Near-duplicates (for dedup / differentiation testing) ---
        TestEntry {
            id: 17,
            title: "ONNX model loading",
            content: "ONNX models are loaded via ort crate. Model path: ~/.cache/grasshopper/models/. Use SessionBuilder for configuration.",
            memory_type: "knowledge",
            created_days_ago: 7.0,
            last_accessed_days_ago: Some(2.0),
            access_count: 6,
            salience: 0.6,
            descriptors: "onnx,models,loading",
        },
        TestEntry {
            id: 18,
            title: "ONNX runtime configuration",
            content: "ONNX Runtime uses ort 2.0.0-rc.11. Configure with OrtEnvironment. Supports CPU and GPU execution providers.",
            memory_type: "knowledge",
            created_days_ago: 8.0,
            last_accessed_days_ago: Some(3.0),
            access_count: 4,
            salience: 0.5,
            descriptors: "onnx,runtime,configuration",
        },
        // --- Varied time + access patterns ---
        TestEntry {
            id: 19,
            title: "FTS5 tokenizer setup",
            content: "FTS5 uses porter unicode61 tokenizer. Custom code_expand function for camelCase splitting.",
            memory_type: "knowledge",
            created_days_ago: 12.0,
            last_accessed_days_ago: Some(1.0),
            access_count: 8,
            salience: 0.6,
            descriptors: "fts5,tokenizer,search",
        },
        TestEntry {
            id: 20,
            title: "HNSW index parameters",
            content: "HNSW approximate nearest neighbor search. instant-distance crate. ef_construction=200, M=32 for good recall.",
            memory_type: "knowledge",
            created_days_ago: 14.0,
            last_accessed_days_ago: Some(7.0),
            access_count: 3,
            salience: 0.5,
            descriptors: "hnsw,vector,search",
        },
        // --- More recent mixed entries ---
        TestEntry {
            id: 21,
            title: "Cloudflare tunnel setup",
            content: "Reverse tunnel using cloudflared daemon. Config at ~/.cloudflared/config.yml. Exposes internal services securely.",
            memory_type: "knowledge",
            created_days_ago: 5.0,
            last_accessed_days_ago: Some(1.0),
            access_count: 7,
            salience: 0.7,
            descriptors: "cloudflare,tunnel,networking",
        },
        TestEntry {
            id: 22,
            title: "Systemd service creation",
            content: "Create systemd service files in /etc/systemd/system/. Use ExecStart, Restart=always. Enable with systemctl enable.",
            memory_type: "procedure",
            created_days_ago: 25.0,
            last_accessed_days_ago: Some(10.0),
            access_count: 6,
            salience: 0.5,
            descriptors: "systemd,service,linux",
        },
        TestEntry {
            id: 23,
            title: "Cross-encoder reranking",
            content: "BAAI/bge-reranker-base for cross-encoder reranking. fastembed crate. Scores query-passage pairs.",
            memory_type: "knowledge",
            created_days_ago: 6.0,
            last_accessed_days_ago: Some(2.0),
            access_count: 5,
            salience: 0.7,
            descriptors: "reranking,cross-encoder,search",
        },
        TestEntry {
            id: 24,
            title: "Memory consolidation strategy",
            content: "Consolidate similar memories when count exceeds threshold. Merge content, update salience, archive originals.",
            memory_type: "knowledge",
            created_days_ago: 9.0,
            last_accessed_days_ago: Some(4.0),
            access_count: 4,
            salience: 0.6,
            descriptors: "consolidation,memory,strategy",
        },
        // --- Zero-access entries (new, never retrieved) ---
        TestEntry {
            id: 25,
            title: "Docker compose networking",
            content: "Docker compose creates default network. Use networks key for custom. Expose vs ports for internal only.",
            memory_type: "knowledge",
            created_days_ago: 3.0,
            last_accessed_days_ago: None,
            access_count: 0,
            salience: 0.5,
            descriptors: "docker,compose,networking",
        },
        TestEntry {
            id: 26,
            title: "Rust error handling patterns",
            content: "Use anyhow for applications, thiserror for libraries. ? operator for propagation. Context trait for adding context.",
            memory_type: "knowledge",
            created_days_ago: 4.0,
            last_accessed_days_ago: None,
            access_count: 0,
            salience: 0.5,
            descriptors: "rust,errors,anyhow",
        },
        // --- Very old, very high access (tests frequency vs decay tradeoff) ---
        TestEntry {
            id: 27,
            title: "SSH key management",
            content: "Generate ed25519 keys with ssh-keygen -t ed25519. Add to ssh-agent. Store in ~/.ssh/.",
            memory_type: "knowledge",
            created_days_ago: 120.0,
            last_accessed_days_ago: Some(2.0),
            access_count: 50,
            salience: 0.6,
            descriptors: "ssh,keys,security",
        },
        // --- Entries with extreme salience values ---
        TestEntry {
            id: 28,
            title: "Temporary debug note",
            content: "Debug: the vector search returns wrong results when embedding is all zeros. Check normalization.",
            memory_type: "episode",
            created_days_ago: 0.1,
            last_accessed_days_ago: Some(0.05),
            access_count: 1,
            salience: 0.0,
            descriptors: "debug,vector,search",
        },
        TestEntry {
            id: 29,
            title: "Critical: backup encryption key",
            content: "The restic backup encryption passphrase is stored in a password manager. Without it, backups are unrecoverable.",
            memory_type: "knowledge",
            created_days_ago: 30.0,
            last_accessed_days_ago: Some(10.0),
            access_count: 5,
            salience: 1.0,
            descriptors: "backup,encryption,critical",
        },
        // --- More procedures at different ages ---
        TestEntry {
            id: 30,
            title: "MCP server deployment",
            content: "Step 1: npm run build in mcp dir. Step 2: sudo systemctl restart mcp-service. Step 3: smoke test with curl.",
            memory_type: "procedure",
            created_days_ago: 7.0,
            last_accessed_days_ago: Some(1.0),
            access_count: 10,
            salience: 0.7,
            descriptors: "mcp,deploy,procedure",
        },
        // --- Entity test targets ---
        TestEntry {
            id: 31,
            title: "Jina embeddings model",
            content: "Jina Code V2 for embeddings. 768 dimensions. Model at ~/.cache/grasshopper/models/jina-code-v2/.",
            memory_type: "knowledge",
            created_days_ago: 11.0,
            last_accessed_days_ago: Some(3.0),
            access_count: 7,
            salience: 0.6,
            descriptors: "jina,embeddings,model",
        },
        TestEntry {
            id: 32,
            title: "RRF fusion parameters",
            content: "Reciprocal Rank Fusion with k=60. FTS weight 0.4, vector weight 0.6. Merges keyword and semantic results.",
            memory_type: "knowledge",
            created_days_ago: 13.0,
            last_accessed_days_ago: Some(5.0),
            access_count: 5,
            salience: 0.6,
            descriptors: "rrf,fusion,search",
        },
        // --- More episodes ---
        TestEntry {
            id: 33,
            title: "Migrated from stdio to HTTP",
            content: "Today migrated all MCP servers from stdio to Streamable HTTP transport. Much cleaner.",
            memory_type: "episode",
            created_days_ago: 3.0,
            last_accessed_days_ago: Some(2.0),
            access_count: 2,
            salience: 0.5,
            descriptors: "mcp,migration,http",
        },
        TestEntry {
            id: 34,
            title: "Added NLI contradiction check",
            content: "Yesterday added NLI contradiction detection to the retrieval pipeline. Uses cross-encoder/nli-MiniLM2.",
            memory_type: "episode",
            created_days_ago: 2.0,
            last_accessed_days_ago: Some(1.0),
            access_count: 3,
            salience: 0.6,
            descriptors: "nli,contradiction,pipeline",
        },
        // --- Knowledge at the 30-day boundary ---
        TestEntry {
            id: 35,
            title: "Tailscale VPN configuration",
            content: "Tailscale VPN for secure remote access. MagicDNS enabled. Subnet routes for LAN access.",
            memory_type: "knowledge",
            created_days_ago: 30.0,
            last_accessed_days_ago: Some(5.0),
            access_count: 8,
            salience: 0.6,
            descriptors: "tailscale,vpn,networking",
        },
        // --- Filler with specific search targets ---
        TestEntry {
            id: 36,
            title: "Tree-sitter code parsing",
            content: "Tree-sitter parses 13 languages. Incremental parsing. TAGS_QUERY for symbol extraction.",
            memory_type: "knowledge",
            created_days_ago: 16.0,
            last_accessed_days_ago: Some(8.0),
            access_count: 4,
            salience: 0.5,
            descriptors: "tree-sitter,parsing,code",
        },
        TestEntry {
            id: 37,
            title: "Query expansion strategy",
            content: "Expand queries with stop-word removal, 2-token subsets, and quoted adjacent pairs. Max 6 variants.",
            memory_type: "knowledge",
            created_days_ago: 8.0,
            last_accessed_days_ago: Some(3.0),
            access_count: 3,
            salience: 0.5,
            descriptors: "query,expansion,search",
        },
        TestEntry {
            id: 38,
            title: "Restic backup schedule",
            content: "Restic backups run nightly at 3am via systemd timer. R2 bucket. 7 daily + 4 weekly retention.",
            memory_type: "knowledge",
            created_days_ago: 25.0,
            last_accessed_days_ago: Some(10.0),
            access_count: 4,
            salience: 0.6,
            descriptors: "restic,backup,schedule",
        },
        TestEntry {
            id: 39,
            title: "OAuth 2.1 for MCP servers",
            content: "All tunneled MCP servers use OAuth 2.1 with PKCE. DCR for dynamic client registration. Shared mcp-oauth module.",
            memory_type: "knowledge",
            created_days_ago: 10.0,
            last_accessed_days_ago: Some(2.0),
            access_count: 6,
            salience: 0.7,
            descriptors: "oauth,mcp,security",
        },
        TestEntry {
            id: 40,
            title: "Hebbian association learning",
            content: "Co-retrieved memories get linked via Hebbian associations in the graph table. Strength increases with co-retrieval.",
            memory_type: "knowledge",
            created_days_ago: 9.0,
            last_accessed_days_ago: Some(4.0),
            access_count: 3,
            salience: 0.6,
            descriptors: "hebbian,associations,learning",
        },
    ]
}

// ---------------------------------------------------------------------------
// Query test set with gold answers
// ---------------------------------------------------------------------------

struct QueryTest {
    query: &'static str,
    /// Entry IDs that should appear in top-5 for a good ranking.
    gold_ids: Vec<i64>,
    /// Brief description of what this tests.
    #[allow(dead_code)]
    description: &'static str,
}

fn query_tests() -> Vec<QueryTest> {
    vec![
        QueryTest {
            query: "Rust async runtime tokio",
            gold_ids: vec![1, 12],
            description: "Direct keyword match + identity preference",
        },
        QueryTest {
            query: "SQLite database configuration",
            gold_ids: vec![2, 19],
            description: "Knowledge about SQLite and FTS5",
        },
        QueryTest {
            query: "deploy grasshopper",
            gold_ids: vec![5, 9, 30],
            description: "Recent episode + deploy procedure",
        },
        QueryTest {
            query: "backup restore",
            gold_ids: vec![10, 29, 38],
            description: "Procedures + critical knowledge about backups",
        },
        QueryTest {
            query: "ONNX model",
            gold_ids: vec![17, 18],
            description: "Near-duplicates should both appear",
        },
        QueryTest {
            query: "security credentials",
            gold_ids: vec![14, 29, 27],
            description: "High-salience security entries despite age",
        },
        QueryTest {
            query: "search ranking reranking",
            gold_ids: vec![23, 32, 37],
            description: "Search pipeline components",
        },
        QueryTest {
            query: "docker networking",
            gold_ids: vec![25, 21],
            description: "Recent zero-access entry should surface",
        },
        QueryTest {
            query: "MCP server HTTP",
            gold_ids: vec![30, 33, 39],
            description: "MCP deployment + migration episode",
        },
        QueryTest {
            query: "Cloudflare tunnel proxy",
            gold_ids: vec![21, 15],
            description: "Entity-rich entries about infrastructure",
        },
        QueryTest {
            query: "memory consolidation decay",
            gold_ids: vec![24, 40],
            description: "Memory system internals",
        },
        QueryTest {
            query: "git workflow commands",
            gold_ids: vec![4, 13],
            description: "High-frequency vs old knowledge tradeoff",
        },
        QueryTest {
            query: "vector embeddings model",
            gold_ids: vec![31, 20, 28],
            description: "Embedding-related knowledge + debug note",
        },
        QueryTest {
            query: "systemd service deploy",
            gold_ids: vec![22, 9, 30],
            description: "Procedures about service deployment",
        },
    ]
}

// ---------------------------------------------------------------------------
// Ablation configurations
// ---------------------------------------------------------------------------

#[derive(Clone)]
struct AblationConfig {
    name: &'static str,
    scorer: fn(&SearchHit) -> f64,
}

fn ablation_configs() -> Vec<AblationConfig> {
    vec![
        AblationConfig {
            name: "Baseline (RRF only)",
            scorer: score_baseline,
        },
        AblationConfig {
            name: "+Recency",
            scorer: score_recency_only,
        },
        AblationConfig {
            name: "+Frequency",
            scorer: score_frequency_only,
        },
        AblationConfig {
            name: "+Salience",
            scorer: score_salience_only,
        },
        AblationConfig {
            name: "+Decay",
            scorer: score_decay_only,
        },
        AblationConfig {
            name: "+All (full cognitive)",
            scorer: score_full,
        },
    ]
}

// ---------------------------------------------------------------------------
// Metrics
// ---------------------------------------------------------------------------

/// Precision@k: fraction of top-k results that are in the gold set.
fn precision_at_k(ranked_ids: &[i64], gold_ids: &[i64], k: usize) -> f64 {
    let top_k: Vec<i64> = ranked_ids.iter().take(k).copied().collect();
    let hits = top_k.iter().filter(|id| gold_ids.contains(id)).count();
    hits as f64 / k.min(gold_ids.len()).max(1) as f64
}

/// Mean Reciprocal Rank: 1/rank of first gold result.
fn mrr(ranked_ids: &[i64], gold_ids: &[i64]) -> f64 {
    for (i, id) in ranked_ids.iter().enumerate() {
        if gold_ids.contains(id) {
            return 1.0 / (i as f64 + 1.0);
        }
    }
    0.0
}

// ---------------------------------------------------------------------------
// Database setup: insert test corpus with controlled timestamps
// ---------------------------------------------------------------------------

fn setup_test_db() -> (TempDir, Store) {
    let dir = TempDir::new().unwrap();
    let db_path = dir.path().join("bench.db");
    let store = Store::open(&db_path).unwrap();

    let corpus = test_corpus();
    for entry in &corpus {
        // Insert with controlled timestamps via raw SQL
        let created_at = iso_days_ago(entry.created_days_ago);
        let last_accessed = entry.last_accessed_days_ago.map(iso_days_ago);
        let la_sql = last_accessed.as_deref().unwrap_or("NULL");

        // Insert into chunks
        let sql = if last_accessed.is_some() {
            format!(
                "INSERT INTO chunks (kind, title, content, memory_type, descriptors, salience, \
                 content_hash, agent_id, access_count, last_accessed, created_at, updated_at) \
                 VALUES ('memory', '{}', '{}', '{}', '{}', {}, '', 'bench', {}, '{}', '{}', '{}')",
                entry.title.replace('\'', "''"),
                entry.content.replace('\'', "''"),
                entry.memory_type,
                entry.descriptors,
                entry.salience,
                entry.access_count,
                la_sql,
                created_at,
                created_at,
            )
        } else {
            format!(
                "INSERT INTO chunks (kind, title, content, memory_type, descriptors, salience, \
                 content_hash, agent_id, access_count, last_accessed, created_at, updated_at) \
                 VALUES ('memory', '{}', '{}', '{}', '{}', {}, '', 'bench', {}, NULL, '{}', '{}')",
                entry.title.replace('\'', "''"),
                entry.content.replace('\'', "''"),
                entry.memory_type,
                entry.descriptors,
                entry.salience,
                entry.access_count,
                created_at,
                created_at,
            )
        };
        store.execute_batch(&sql).unwrap();

        // Get the rowid (should match insertion order)
        let id_sql = format!(
            "INSERT INTO chunks_fts (rowid, title, content, snippet, symbol_name, descriptors) \
             VALUES ({}, '{}', '{}', '', '', '{}')",
            entry.id,
            entry.title.replace('\'', "''"),
            entry.content.replace('\'', "''"),
            entry.descriptors,
        );
        store.execute_batch(&id_sql).unwrap();
    }

    (dir, store)
}

// ---------------------------------------------------------------------------
// Benchmark runner
// ---------------------------------------------------------------------------

fn run_ablation(store: &Store) -> Vec<(String, f64, f64, Vec<f64>)> {
    let queries = query_tests();
    let configs = ablation_configs();
    let mut results: Vec<(String, f64, f64, Vec<f64>)> = Vec::new();

    for config in &configs {
        let mut total_p5 = 0.0;
        let mut total_mrr = 0.0;
        let mut query_scores = Vec::new();

        for qt in &queries {
            // Get FTS results from the test database
            let fts_hits = store
                .fts_search(qt.query, Some("memory"), 20)
                .unwrap_or_default();

            // Apply the ablation scorer
            let mut scored: Vec<SearchHit> = fts_hits
                .into_iter()
                .filter(|h| !h.archived && h.memory_type.as_deref() != Some("identity"))
                .map(|mut h| {
                    h.score = (config.scorer)(&h);
                    h
                })
                .collect();
            scored.sort_by(|a, b| {
                b.score
                    .partial_cmp(&a.score)
                    .unwrap_or(std::cmp::Ordering::Equal)
            });

            let ranked_ids: Vec<i64> = scored.iter().map(|h| h.id).collect();
            let p5 = precision_at_k(&ranked_ids, &qt.gold_ids, 5);
            let m = mrr(&ranked_ids, &qt.gold_ids);

            total_p5 += p5;
            total_mrr += m;
            query_scores.push(p5);
        }

        let n = queries.len() as f64;
        results.push((
            config.name.to_string(),
            total_p5 / n,
            total_mrr / n,
            query_scores,
        ));
    }

    results
}

// ---------------------------------------------------------------------------
// Cognitive score component isolation tests
// ---------------------------------------------------------------------------

#[test]
fn bench_cognitive_component_isolation() {
    println!("\n=== Component Isolation Tests ===\n");
    println!("Testing how each component modifies a base RRF score of 0.01\n");

    let base_score = 0.01;

    // Test recency: new vs old entry
    let new_hit = make_hit(1, "New", base_score, "knowledge", 1.0, Some(0.5), 5, 0.5);
    let old_hit = make_hit(2, "Old", base_score, "knowledge", 60.0, Some(30.0), 5, 0.5);

    println!("--- Recency Boost ---");
    println!(
        "  New (1 day):    {:.6} -> {:.6} ({:+.1}%)",
        base_score,
        score_recency_only(&new_hit),
        (score_recency_only(&new_hit) / base_score - 1.0) * 100.0
    );
    println!(
        "  Old (60 days):  {:.6} -> {:.6} ({:+.1}%)",
        base_score,
        score_recency_only(&old_hit),
        (score_recency_only(&old_hit) / base_score - 1.0) * 100.0
    );

    // Test frequency: high vs low access
    let high_freq = make_hit(
        3,
        "FreqHi",
        base_score,
        "knowledge",
        10.0,
        Some(1.0),
        100,
        0.5,
    );
    let low_freq = make_hit(
        4,
        "FreqLo",
        base_score,
        "knowledge",
        10.0,
        Some(1.0),
        0,
        0.5,
    );

    println!("\n--- Frequency Boost ---");
    println!(
        "  100 accesses:   {:.6} -> {:.6} ({:+.1}%)",
        base_score,
        score_frequency_only(&high_freq),
        (score_frequency_only(&high_freq) / base_score - 1.0) * 100.0
    );
    println!(
        "  0 accesses:     {:.6} -> {:.6} ({:+.1}%)",
        base_score,
        score_frequency_only(&low_freq),
        (score_frequency_only(&low_freq) / base_score - 1.0) * 100.0
    );

    // Test salience: high vs low
    let high_sal = make_hit(5, "SalHi", base_score, "knowledge", 10.0, Some(1.0), 5, 1.0);
    let low_sal = make_hit(6, "SalLo", base_score, "knowledge", 10.0, Some(1.0), 5, 0.0);

    println!("\n--- Salience Factor ---");
    println!(
        "  Salience 1.0:   {:.6} -> {:.6} ({:+.1}%)",
        base_score,
        score_salience_only(&high_sal),
        (score_salience_only(&high_sal) / base_score - 1.0) * 100.0
    );
    println!(
        "  Salience 0.0:   {:.6} -> {:.6} ({:+.1}%)",
        base_score,
        score_salience_only(&low_sal),
        (score_salience_only(&low_sal) / base_score - 1.0) * 100.0
    );

    // Test decay: episode vs identity
    let episode = make_hit(
        7,
        "Episode",
        base_score,
        "episode",
        30.0,
        Some(20.0),
        5,
        0.5,
    );
    let identity = make_hit(
        8,
        "Identity",
        base_score,
        "identity",
        30.0,
        Some(20.0),
        5,
        0.5,
    );
    let knowledge = make_hit(
        9,
        "Knowledge",
        base_score,
        "knowledge",
        30.0,
        Some(20.0),
        5,
        0.5,
    );

    println!("\n--- Decay Factor (20 days since last access) ---");
    println!(
        "  Episode  (lambda=0.023): {:.6} -> {:.6} ({:+.1}%)",
        base_score,
        score_decay_only(&episode),
        (score_decay_only(&episode) / base_score - 1.0) * 100.0
    );
    println!(
        "  Knowledge (lambda=0.005): {:.6} -> {:.6} ({:+.1}%)",
        base_score,
        score_decay_only(&knowledge),
        (score_decay_only(&knowledge) / base_score - 1.0) * 100.0
    );
    println!(
        "  Identity (lambda=0.0):   {:.6} -> {:.6} ({:+.1}%)",
        base_score,
        score_decay_only(&identity),
        (score_decay_only(&identity) / base_score - 1.0) * 100.0
    );

    // Test full cognitive score
    let best_case = make_hit(10, "Best", base_score, "identity", 0.5, Some(0.1), 100, 1.0);
    let worst_case = make_hit(11, "Worst", base_score, "episode", 90.0, None, 0, 0.0);

    println!("\n--- Full Cognitive Score (all components) ---");
    println!(
        "  Best case:  {:.6} -> {:.6} ({:+.1}%)",
        base_score,
        score_full(&best_case),
        (score_full(&best_case) / base_score - 1.0) * 100.0
    );
    println!(
        "  Worst case: {:.6} -> {:.6} ({:+.1}%)",
        base_score,
        score_full(&worst_case),
        (score_full(&worst_case) / base_score - 1.0) * 100.0
    );
    let spread = score_full(&best_case) / score_full(&worst_case);
    println!("  Spread (best/worst): {:.1}x", spread);

    assert!(
        score_full(&best_case) > score_full(&worst_case),
        "Best case should score higher than worst case"
    );
    assert!(spread > 5.0, "Score spread should be significant (>5x)");

    println!("\nAll component isolation assertions passed.");
}

// ---------------------------------------------------------------------------
// Main ablation benchmark (FTS-backed)
// ---------------------------------------------------------------------------

#[test]
fn bench_cognitive_ablation() {
    println!("\n{}", "=".repeat(70));
    println!("=== Cognitive Scoring Ablation Benchmark ===");
    println!("{}\n", "=".repeat(70));

    let (_dir, store) = setup_test_db();

    // Run main ablation
    let results = run_ablation(&store);

    // Print results table
    println!(
        "{:<28} {:>10} {:>10} {:>12}",
        "Configuration", "Avg P@5", "Avg MRR", "vs Baseline"
    );
    println!("{}", "-".repeat(64));

    let baseline_p5 = results[0].1;
    for (name, avg_p5, avg_mrr, _) in &results {
        let delta = if *avg_p5 > 0.0 && baseline_p5 > 0.0 {
            format!("{:+.1}%", (avg_p5 / baseline_p5 - 1.0) * 100.0)
        } else if baseline_p5 == 0.0 {
            "N/A".to_string()
        } else {
            "-100.0%".to_string()
        };
        println!(
            "{:<28} {:>10.3} {:>10.3} {:>12}",
            name, avg_p5, avg_mrr, delta
        );
    }

    // Per-query breakdown for full cognitive
    println!("\n{:<28} {:>10}", "Query", "P@5 (Full)");
    println!("{}", "-".repeat(40));
    let queries = query_tests();
    let full_scores = &results.last().unwrap().3;
    for (qt, score) in queries.iter().zip(full_scores.iter()) {
        let truncated: String = qt.query.chars().take(26).collect();
        println!("{:<28} {:>10.3}", truncated, score);
    }

    // Score distribution analysis
    println!("\n=== Score Distribution Analysis ===\n");
    println!("How much does each component change the score (on a typical entry)?");

    let typical = make_hit(99, "Typical", 0.01, "knowledge", 7.0, Some(3.0), 5, 0.5);
    let base = score_baseline(&typical);

    let components = vec![
        ("Recency", score_recency_only(&typical)),
        ("Frequency", score_frequency_only(&typical)),
        ("Salience", score_salience_only(&typical)),
        ("Decay", score_decay_only(&typical)),
        ("Full", score_full(&typical)),
    ];

    println!("{:<16} {:>10} {:>12}", "Component", "Score", "Multiplier");
    println!("{}", "-".repeat(40));
    println!("{:<16} {:>10.6} {:>12}", "Baseline", base, "1.00x");
    for (name, score) in &components {
        println!("{:<16} {:>10.6} {:>12.2}x", name, score, score / base);
    }

    println!("\n=== Benchmark complete ===");
}
