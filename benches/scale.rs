//! Scale testing benchmark.
//!
//! Progressive insertion of synthetic chunks. At each checkpoint
//! (1K, 2K, 5K, 10K), measures search latency, HNSW rebuild time,
//! DB/HNSW size, and VmRSS.
//!
//! Run: cargo bench --bench scale
//! Full: GRASSHOPPER_SCALE_FULL=1 cargo bench --bench scale  (adds 50K)
//!
//! Note: 10K takes ~40 min to embed on Beelink. 50K takes ~3.5 hours.

mod common;

use common::{BenchReport, PercentileStats, read_vmrss};
use grasshopper::code::embed::{Embedder, default_cache_dir};
use grasshopper::code::hnsw::HnswIndex;
use grasshopper::search::{SearchContext, unified_search};
use grasshopper::store::{MemoryParams, Store};
use std::time::{Duration, Instant};
use tempfile::TempDir;

const STANDARD_QUERIES: &[&str] = &[
    "search pipeline",
    "how does indexing work",
    "database schema",
    "error handling patterns",
    "embedding model configuration",
    "concurrent access safety",
    "MCP server tools",
    "reranking results",
    "memory decay and scoring",
    "code chunking strategy",
];

fn checkpoints() -> Vec<usize> {
    let mut points = vec![1_000, 2_000, 5_000, 10_000];
    if std::env::var("GRASSHOPPER_SCALE_FULL").is_ok() {
        points.push(50_000);
    }
    points
}

// Diverse vocabulary for generating semantically distinct chunks.
const DOMAINS: &[&str] = &[
    "authentication",
    "caching",
    "compression",
    "cryptography",
    "database",
    "debugging",
    "deployment",
    "encoding",
    "filesystem",
    "garbage_collection",
    "hashing",
    "indexing",
    "logging",
    "marshalling",
    "networking",
    "optimization",
    "parsing",
    "profiling",
    "queuing",
    "replication",
    "scheduling",
    "serialization",
    "threading",
    "validation",
    "websocket",
];
const PATTERNS: &[&str] = &[
    "builder",
    "factory",
    "observer",
    "strategy",
    "adapter",
    "decorator",
    "proxy",
    "iterator",
    "visitor",
    "mediator",
];
const STRUCTURES: &[&str] = &[
    "HashMap",
    "BTreeMap",
    "Vec",
    "LinkedList",
    "HashSet",
    "BinaryHeap",
    "VecDeque",
    "Arc<Mutex>",
    "RwLock",
    "Channel",
];
const ACTIONS: &[&str] = &[
    "initialize",
    "validate",
    "transform",
    "aggregate",
    "partition",
    "merge",
    "filter",
    "sort",
    "compress",
    "encrypt",
    "decode",
    "normalize",
    "interpolate",
    "reconcile",
    "propagate",
];

/// Generate semantically diverse synthetic content.
/// Uses combinatorial expansion across domains × patterns × structures × actions
/// to produce content that is unique enough to avoid dedup (0.75 cosine threshold).
fn synthetic_content(i: usize) -> (String, String) {
    let domain = DOMAINS[i % DOMAINS.len()];
    let pattern = PATTERNS[(i / DOMAINS.len()) % PATTERNS.len()];
    let structure = STRUCTURES[(i / (DOMAINS.len() * PATTERNS.len())) % STRUCTURES.len()];
    let action = ACTIONS[(i / (DOMAINS.len() * PATTERNS.len() * STRUCTURES.len())) % ACTIONS.len()];

    let title = format!("{domain} {pattern}: {action} with {structure} (variant {i})");
    let content = format!(
        "/// {domain} subsystem using the {pattern} pattern.\n\
         /// Manages {structure} instances to {action} data at scale.\n\
         pub struct {domain}_{pattern}_{i} {{\n\
         \x20   store: {structure},\n\
         \x20   config: {domain}::Config,\n\
         }}\n\n\
         impl {domain}_{pattern}_{i} {{\n\
         \x20   pub fn {action}(&mut self, input: &[u8]) -> Result<Vec<u8>> {{\n\
         \x20       let key = self.config.{domain}_key()?;\n\
         \x20       let processed = {domain}::{action}(input, &key)?;\n\
         \x20       self.store.insert(processed.id, processed.clone());\n\
         \x20       tracing::info!(\"{domain}/{pattern}/{i}: {action} complete\");\n\
         \x20       Ok(processed.into_bytes())\n\
         \x20   }}\n\
         \x20   pub fn rollback_{i}(&mut self) -> Result<()> {{\n\
         \x20       self.store.clear();\n\
         \x20       {domain}::cleanup(&self.config)?;\n\
         \x20       Ok(())\n\
         \x20   }}\n\
         }}",
    );
    (title, content)
}

/// Insert chunks up to target count, bypassing memory::store dedup.
/// Inserts directly via insert_memory + batch_upsert_embeddings for exact
/// corpus size control. Embeds in batches of 32 for efficiency.
fn fill_to(store: &Store, embedder: &mut Embedder, current: usize, target: usize) -> usize {
    let batch_size = 32;
    let mut inserted = 0;
    let mut batch_texts: Vec<String> = Vec::with_capacity(batch_size);
    let mut batch_ids: Vec<i64> = Vec::with_capacity(batch_size);

    for i in current..target {
        let (title, content) = synthetic_content(i);
        let hash = format!("scale-{i}");

        let id = store
            .insert_memory(&MemoryParams {
                title: &title,
                content: &content,
                memory_type: "knowledge",
                descriptors: &format!("scale,{}", DOMAINS[i % DOMAINS.len()]),
                salience: 0.5,
                content_hash: &hash,
            })
            .unwrap();

        batch_texts.push(format!("{}\n{}", title, content));
        batch_ids.push(id);
        inserted += 1;

        // Embed in batches
        if batch_texts.len() >= batch_size {
            let vectors = embedder.embed_batch(&batch_texts).unwrap();
            let items: Vec<(i64, &[f32], &str)> = batch_ids
                .iter()
                .zip(vectors.iter())
                .map(|(&id, v)| (id, v.as_slice(), grasshopper::code::embed::MODEL_NAME))
                .collect();
            store.batch_upsert_embeddings(&items).unwrap();
            batch_texts.clear();
            batch_ids.clear();
        }

        if inserted % 500 == 0 {
            eprintln!("    Stored {inserted}/{}", target - current);
        }
    }

    // Flush remaining batch
    if !batch_texts.is_empty() {
        let vectors = embedder.embed_batch(&batch_texts).unwrap();
        let items: Vec<(i64, &[f32], &str)> = batch_ids
            .iter()
            .zip(vectors.iter())
            .map(|(&id, v)| (id, v.as_slice(), grasshopper::code::embed::MODEL_NAME))
            .collect();
        store.batch_upsert_embeddings(&items).unwrap();
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

    inserted
}

/// Run standardized queries and measure latencies.
/// Returns (fts_latencies, hybrid_latencies).
/// Note: unified_search always runs FTS when a query is provided, so there is
/// no true "vector-only" path through the API. The two configs are:
///   FTS-only:  no embedder, no HNSW → pure BM25
///   Hybrid:    embedder + HNSW → FTS + vector + RRF fusion
fn run_queries(
    store: &Store,
    embedder: &mut Embedder,
    hnsw: &Option<HnswIndex>,
) -> (Vec<Duration>, Vec<Duration>) {
    let mut fts_latencies = Vec::new();
    let mut hybrid_latencies = Vec::new();

    for &query in STANDARD_QUERIES {
        // FTS-only (no embedder, no HNSW)
        let t = Instant::now();
        let ctx = SearchContext {
            store,
            query,
            kind_filter: Some("memory"),
            limit: 10,
            threshold: None,
            embedder: None,
            reranker: None,
            hnsw: None,
        };
        unified_search(ctx).expect("FTS search must not fail in benchmark");
        fts_latencies.push(t.elapsed());

        // Hybrid (FTS + embedder + HNSW, no reranker for speed)
        let t = Instant::now();
        let ctx = SearchContext {
            store,
            query,
            kind_filter: Some("memory"),
            limit: 10,
            threshold: None,
            embedder: Some(embedder),
            reranker: None,
            hnsw: hnsw.as_ref(),
        };
        unified_search(ctx).expect("hybrid search must not fail in benchmark");
        hybrid_latencies.push(t.elapsed());
    }

    (fts_latencies, hybrid_latencies)
}

fn main() {
    let start = Instant::now();
    eprintln!("=== Scale Testing Benchmark ===\n");

    let points = checkpoints();
    eprintln!("Checkpoints: {:?}", points);
    if std::env::var("GRASSHOPPER_SCALE_FULL").is_ok() {
        eprintln!("FULL mode enabled (including 50K — this will take hours)\n");
    }

    let dir = TempDir::new().unwrap();
    let db_path = dir.path().join("bench.db");
    let hnsw_path = dir.path().join("bench.hnsw");
    let store = Store::open(&db_path).unwrap();

    let cache_dir = default_cache_dir();
    let mut embedder = Embedder::new(&cache_dir).unwrap();

    let mut report = BenchReport::new("scale");
    let mut current_count = 0;
    let mut checkpoint_results = Vec::new();

    for &target in &points {
        eprintln!("--- Checkpoint: {target} chunks ---");

        // 1. Insert + embed chunks
        let insert_start = Instant::now();
        let inserted = fill_to(&store, &mut embedder, current_count, target);
        let insert_ms = insert_start.elapsed().as_millis();
        eprintln!("  Inserted+embedded {inserted} chunks ({}ms)", insert_ms);

        // 3. Build HNSW
        let hnsw_start = Instant::now();
        let embeddings = store.get_all_embeddings().unwrap();
        let hnsw = if !embeddings.is_empty() {
            Some(HnswIndex::from_embeddings(&embeddings).unwrap())
        } else {
            None
        };
        let hnsw_build_ms = hnsw_start.elapsed().as_millis();
        eprintln!(
            "  HNSW built ({}ms, {} points)",
            hnsw_build_ms,
            embeddings.len()
        );

        // 4. Verify actual chunk count (dedup may reduce it)
        let (_, actual_count) = store.count_by_kind().unwrap();
        if (actual_count as usize) < target {
            eprintln!(
                "  WARNING: expected {target} chunks but only {actual_count} (dedup removed {})",
                target as i64 - actual_count
            );
        }

        // 5. Run queries
        let (fts_lats, hybrid_lats) = run_queries(&store, &mut embedder, &hnsw);
        let fts_stats = PercentileStats::from_durations(&fts_lats);
        let hybrid_stats = PercentileStats::from_durations(&hybrid_lats);
        eprintln!(
            "  FTS    P50={:.1}ms P95={:.1}ms",
            fts_stats.p50_ms, fts_stats.p95_ms
        );
        eprintln!(
            "  Hybrid P50={:.1}ms P95={:.1}ms",
            hybrid_stats.p50_ms, hybrid_stats.p95_ms
        );

        // 5. Measure sizes
        let db_size = store.db_size_bytes().unwrap();
        let hnsw_size = if let Some(ref h) = hnsw {
            h.save(&hnsw_path).unwrap();
            std::fs::metadata(&hnsw_path).map(|m| m.len()).unwrap_or(0)
        } else {
            0
        };
        let vmrss = read_vmrss();
        eprintln!(
            "  DB={:.2}MB HNSW={:.2}MB VmRSS={:.1}MB\n",
            db_size as f64 / (1024.0 * 1024.0),
            hnsw_size as f64 / (1024.0 * 1024.0),
            vmrss as f64 / (1024.0 * 1024.0)
        );

        checkpoint_results.push(serde_json::json!({
            "target_chunks": target,
            "insert_embed_ms": insert_ms,
            "hnsw_build_ms": hnsw_build_ms,
            "hnsw_points": embeddings.len(),
            "actual_chunks": actual_count,
            "fts_latency": fts_stats,
            "hybrid_latency": hybrid_stats,
            "db_bytes": db_size,
            "db_mb": db_size as f64 / (1024.0 * 1024.0),
            "hnsw_bytes": hnsw_size,
            "hnsw_mb": hnsw_size as f64 / (1024.0 * 1024.0),
            "vmrss_bytes": vmrss,
            "vmrss_mb": vmrss as f64 / (1024.0 * 1024.0),
        }));

        current_count = target;
    }

    report.add_section("checkpoints", serde_json::json!(checkpoint_results));
    report.add_section(
        "timing",
        serde_json::json!({
            "total_seconds": start.elapsed().as_secs_f64(),
        }),
    );

    report.save("bench/results/scale.json").unwrap();
    eprintln!("Done in {:.1}s", start.elapsed().as_secs_f64());
}
