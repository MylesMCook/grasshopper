//! Resource profiling benchmark.
//!
//! Measures memory (VmRSS) and disk usage at various stages:
//! baseline, after FTS indexing, after embedding, after HNSW build,
//! after loading reranker, after searches, and DB/HNSW file sizes.
//!
//! Not Criterion — runs once and produces a JSON report.
//!
//! Run: cargo bench --bench resources

mod common;

use common::{BenchReport, read_vmrss};
use grasshopper::code::embed::{Embedder, default_cache_dir};
use grasshopper::code::hnsw::HnswIndex;
use grasshopper::rerank::Reranker;
use grasshopper::search::{SearchContext, unified_search};
use grasshopper::store::Store;
use std::time::Instant;
use tempfile::TempDir;

fn main() {
    let start = Instant::now();
    eprintln!("=== Resource Profiling Benchmark ===\n");

    let mut report = BenchReport::new("resources");
    let mut measurements = Vec::new();

    // 1. Baseline — just open store
    let dir = TempDir::new().unwrap();
    let db_path = dir.path().join("bench.db");
    let store = Store::open(&db_path).unwrap();
    let vmrss_baseline = read_vmrss();
    measurements.push(serde_json::json!({
        "stage": "baseline (Store::open)",
        "vmrss_bytes": vmrss_baseline,
        "vmrss_mb": vmrss_baseline as f64 / (1024.0 * 1024.0),
    }));
    eprintln!(
        "  Baseline VmRSS: {:.1} MB",
        vmrss_baseline as f64 / (1024.0 * 1024.0)
    );

    // 2. After FTS indexing
    let src_dir = std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("src");
    let index_result = grasshopper::index::index_directory(&store, &src_dir).unwrap();
    let codebase_id = index_result.codebase_id;
    let vmrss_fts = read_vmrss();
    measurements.push(serde_json::json!({
        "stage": "after FTS indexing",
        "vmrss_bytes": vmrss_fts,
        "vmrss_mb": vmrss_fts as f64 / (1024.0 * 1024.0),
        "delta_mb": (vmrss_fts as f64 - vmrss_baseline as f64) / (1024.0 * 1024.0),
        "chunks_written": index_result.chunks_written,
        "files_scanned": index_result.files_scanned,
    }));
    eprintln!(
        "  After FTS index: {:.1} MB (+{:.1} MB, {} chunks)",
        vmrss_fts as f64 / (1024.0 * 1024.0),
        (vmrss_fts as f64 - vmrss_baseline as f64) / (1024.0 * 1024.0),
        index_result.chunks_written
    );

    // 3. Model cold start
    let cache_dir = default_cache_dir();
    let cold_start = Instant::now();
    let mut embedder = Embedder::new(&cache_dir).unwrap();
    let cold_start_ms = cold_start.elapsed().as_millis();
    let vmrss_model_loaded = read_vmrss();
    measurements.push(serde_json::json!({
        "stage": "embedder loaded (cold start)",
        "vmrss_bytes": vmrss_model_loaded,
        "vmrss_mb": vmrss_model_loaded as f64 / (1024.0 * 1024.0),
        "delta_mb": (vmrss_model_loaded as f64 - vmrss_fts as f64) / (1024.0 * 1024.0),
        "cold_start_ms": cold_start_ms,
    }));
    eprintln!(
        "  Embedder loaded: {:.1} MB (+{:.1} MB, cold start: {}ms)",
        vmrss_model_loaded as f64 / (1024.0 * 1024.0),
        (vmrss_model_loaded as f64 - vmrss_fts as f64) / (1024.0 * 1024.0),
        cold_start_ms
    );

    // 4. First embed_batch (warm up model)
    let warm_start = Instant::now();
    let _ = embedder
        .embed_batch(&["warm up the model".to_string()])
        .unwrap();
    let warm_start_ms = warm_start.elapsed().as_millis();
    measurements.push(serde_json::json!({
        "stage": "first embed_batch (model warm)",
        "warm_start_ms": warm_start_ms,
    }));
    eprintln!("  First embed: {}ms", warm_start_ms);

    // 5. After embedding all chunks
    grasshopper::index::embed_codebase(&store, &mut embedder, codebase_id).unwrap();
    let vmrss_embedded = read_vmrss();
    measurements.push(serde_json::json!({
        "stage": "after embedding all chunks",
        "vmrss_bytes": vmrss_embedded,
        "vmrss_mb": vmrss_embedded as f64 / (1024.0 * 1024.0),
        "delta_mb": (vmrss_embedded as f64 - vmrss_model_loaded as f64) / (1024.0 * 1024.0),
    }));
    eprintln!(
        "  After embedding: {:.1} MB (+{:.1} MB)",
        vmrss_embedded as f64 / (1024.0 * 1024.0),
        (vmrss_embedded as f64 - vmrss_model_loaded as f64) / (1024.0 * 1024.0)
    );

    // 6. After HNSW build
    let embeddings = store.get_all_embeddings().unwrap();
    let hnsw_build_start = Instant::now();
    let hnsw = HnswIndex::from_embeddings(&embeddings).unwrap();
    let hnsw_build_ms = hnsw_build_start.elapsed().as_millis();
    let vmrss_hnsw = read_vmrss();
    measurements.push(serde_json::json!({
        "stage": "after HNSW build",
        "vmrss_bytes": vmrss_hnsw,
        "vmrss_mb": vmrss_hnsw as f64 / (1024.0 * 1024.0),
        "delta_mb": (vmrss_hnsw as f64 - vmrss_embedded as f64) / (1024.0 * 1024.0),
        "hnsw_build_ms": hnsw_build_ms,
        "hnsw_points": hnsw.len(),
    }));
    eprintln!(
        "  After HNSW: {:.1} MB (+{:.1} MB, {} points, {}ms)",
        vmrss_hnsw as f64 / (1024.0 * 1024.0),
        (vmrss_hnsw as f64 - vmrss_embedded as f64) / (1024.0 * 1024.0),
        hnsw.len(),
        hnsw_build_ms
    );

    // 7. After loading reranker
    let reranker_start = Instant::now();
    let mut reranker = Reranker::new().unwrap();
    let reranker_ms = reranker_start.elapsed().as_millis();
    let vmrss_reranker = read_vmrss();
    measurements.push(serde_json::json!({
        "stage": "after loading reranker",
        "vmrss_bytes": vmrss_reranker,
        "vmrss_mb": vmrss_reranker as f64 / (1024.0 * 1024.0),
        "delta_mb": (vmrss_reranker as f64 - vmrss_hnsw as f64) / (1024.0 * 1024.0),
        "load_ms": reranker_ms,
    }));
    eprintln!(
        "  After reranker: {:.1} MB (+{:.1} MB, {}ms)",
        vmrss_reranker as f64 / (1024.0 * 1024.0),
        (vmrss_reranker as f64 - vmrss_hnsw as f64) / (1024.0 * 1024.0),
        reranker_ms
    );

    // 8. After 10 searches
    let queries = [
        "search pipeline",
        "how does indexing work",
        "embedding model",
        "database schema",
        "error handling",
        "concurrent access",
        "MCP server tools",
        "reranking results",
        "memory decay",
        "code chunking",
    ];
    for q in &queries {
        let ctx = SearchContext {
            store: &store,
            query: q,
            kind_filter: Some("code"),
            limit: 10,
            threshold: None,
            embedder: Some(&mut embedder),
            reranker: Some(&mut reranker),
            hnsw: Some(&hnsw),
        };
        let _ = unified_search(ctx);
    }
    let vmrss_after_search = read_vmrss();
    measurements.push(serde_json::json!({
        "stage": "after 10 searches",
        "vmrss_bytes": vmrss_after_search,
        "vmrss_mb": vmrss_after_search as f64 / (1024.0 * 1024.0),
        "delta_mb": (vmrss_after_search as f64 - vmrss_reranker as f64) / (1024.0 * 1024.0),
    }));
    eprintln!(
        "  After 10 searches: {:.1} MB (+{:.1} MB)",
        vmrss_after_search as f64 / (1024.0 * 1024.0),
        (vmrss_after_search as f64 - vmrss_reranker as f64) / (1024.0 * 1024.0)
    );

    // 9. DB file size
    let db_size = store.db_size_bytes().unwrap();
    measurements.push(serde_json::json!({
        "stage": "db_file_size",
        "db_bytes": db_size,
        "db_mb": db_size as f64 / (1024.0 * 1024.0),
    }));
    eprintln!(
        "\n  DB file size: {:.2} MB",
        db_size as f64 / (1024.0 * 1024.0)
    );

    // 10. HNSW file size
    let hnsw_path = dir.path().join("bench.hnsw");
    hnsw.save(&hnsw_path).unwrap();
    let hnsw_size = std::fs::metadata(&hnsw_path).map(|m| m.len()).unwrap_or(0);
    measurements.push(serde_json::json!({
        "stage": "hnsw_file_size",
        "hnsw_bytes": hnsw_size,
        "hnsw_mb": hnsw_size as f64 / (1024.0 * 1024.0),
    }));
    eprintln!(
        "  HNSW file size: {:.2} MB",
        hnsw_size as f64 / (1024.0 * 1024.0)
    );

    // 11. Model warm start (second embed_batch)
    let warm2_start = Instant::now();
    let _ = embedder
        .embed_batch(&["second warm start".to_string()])
        .unwrap();
    let warm2_ms = warm2_start.elapsed().as_millis();
    measurements.push(serde_json::json!({
        "stage": "model warm start (second embed)",
        "warm_ms": warm2_ms,
    }));
    eprintln!("  Model warm embed: {}ms", warm2_ms);

    report.add_section("measurements", serde_json::json!(measurements));
    report.add_section(
        "summary",
        serde_json::json!({
            "total_vmrss_mb": vmrss_after_search as f64 / (1024.0 * 1024.0),
            "baseline_vmrss_mb": vmrss_baseline as f64 / (1024.0 * 1024.0),
            "overhead_mb": (vmrss_after_search as f64 - vmrss_baseline as f64) / (1024.0 * 1024.0),
            "db_size_mb": db_size as f64 / (1024.0 * 1024.0),
            "hnsw_size_mb": hnsw_size as f64 / (1024.0 * 1024.0),
            "cold_start_ms": cold_start_ms,
            "reranker_load_ms": reranker_ms,
        }),
    );

    report.add_section(
        "timing",
        serde_json::json!({
            "total_seconds": start.elapsed().as_secs_f64(),
        }),
    );

    report.save("bench/results/resources.json").unwrap();
    eprintln!("\nDone in {:.1}s", start.elapsed().as_secs_f64());
}
