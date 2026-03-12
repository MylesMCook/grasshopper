//! Code search accuracy benchmark.
//!
//! Indexes Grasshopper's own src/ with embeddings, then runs gold-standard
//! queries in 3 configs: FTS-only, hybrid (no rerank), full pipeline.
//! Measures Precision@K, MRR, NDCG@10, and reranker lift.
//!
//! Gold sets are built INDEPENDENTLY of search results — by scanning ALL
//! chunks in the DB before any search runs. This prevents the search engine
//! from grading its own exam.
//!
//! Run: cargo bench --bench accuracy_code

mod common;

use common::{BenchReport, mrr, ndcg_at_k, precision_at_k};
use grasshopper::code::hnsw::HnswIndex;
use grasshopper::rerank::Reranker;
use grasshopper::search::{SearchContext, unified_search};
use grasshopper::store::Store;
use std::time::Instant;
use tempfile::TempDir;

// ---------------------------------------------------------------------------
// Gold-standard queries
// ---------------------------------------------------------------------------

struct GoldQuery {
    query: &'static str,
    /// Expected file paths or symbol names that should appear in relevant chunks.
    /// Matched as case-insensitive substrings against all chunks in the DB.
    expected: Vec<&'static str>,
    /// Type: "semantic", "keyword", "negative"
    query_type: &'static str,
}

fn gold_queries() -> Vec<GoldQuery> {
    vec![
        GoldQuery {
            query: "how does error handling work",
            // "anyhow" is specific; removed "Result" and "context" (match nearly every Rust chunk)
            expected: vec!["anyhow", "bail!", "with_context"],
            query_type: "semantic",
        },
        GoldQuery {
            query: "where is the main entry point",
            expected: vec!["main.rs", "fn run()"],
            query_type: "keyword+semantic",
        },
        GoldQuery {
            query: "what happens when a search comes in",
            expected: vec!["unified_search", "SearchContext"],
            query_type: "semantic",
        },
        GoldQuery {
            query: "how are embeddings stored",
            expected: vec!["embedding_to_blob", "batch_upsert_embeddings"],
            query_type: "semantic",
        },
        GoldQuery {
            query: "database schema migrations",
            expected: vec!["schema.rs", "init_schema"],
            query_type: "keyword",
        },
        GoldQuery {
            query: "concurrent access patterns",
            expected: vec!["journal_mode=WAL", "Mutex"],
            query_type: "semantic",
        },
        GoldQuery {
            query: "text tokenization for search",
            expected: vec!["tokenizer", "prepare_fts_query"],
            query_type: "keyword+semantic",
        },
        GoldQuery {
            query: "blockchain consensus mechanism",
            expected: vec![],
            query_type: "negative",
        },
        GoldQuery {
            query: "GPU CUDA kernel launch",
            expected: vec![],
            query_type: "negative",
        },
        GoldQuery {
            query: "how does incremental indexing work",
            expected: vec!["index.rs", "file_hash", "stored_hashes"],
            query_type: "semantic",
        },
        GoldQuery {
            query: "cross encoder reranking",
            expected: vec!["rerank", "fastembed", "bge-reranker"],
            query_type: "keyword+semantic",
        },
        GoldQuery {
            query: "HNSW approximate nearest neighbor",
            expected: vec!["hnsw.rs", "instant_distance", "HnswIndex"],
            query_type: "keyword",
        },
        GoldQuery {
            query: "how does memory decay work",
            expected: vec!["cognitive_score", "decay_rate"],
            query_type: "semantic",
        },
        GoldQuery {
            query: "MCP server tools",
            expected: vec!["ServerHandler", "ListToolsRequest"],
            query_type: "keyword+semantic",
        },
        GoldQuery {
            query: "directory walking and file scanning",
            expected: vec!["scan.rs", "scan_directory", "WalkBuilder"],
            query_type: "semantic",
        },
        GoldQuery {
            query: "reciprocal rank fusion merging",
            expected: vec!["hybrid_search", "RRF_K"],
            query_type: "keyword+semantic",
        },
        GoldQuery {
            query: "code chunk parsing",
            expected: vec!["chunk.rs", "chunk_content", "ParsedChunk"],
            query_type: "keyword+semantic",
        },
        GoldQuery {
            query: "embedding model download",
            expected: vec!["download_model", "HF_REPO", "model.onnx"],
            query_type: "semantic",
        },
        GoldQuery {
            query: "symbol navigation and definitions",
            expected: vec!["find_definitions", "find_references", "GraphEdge"],
            query_type: "semantic",
        },
        GoldQuery {
            query: "query expansion stop words",
            expected: vec!["expand_query", "STOP_WORDS"],
            query_type: "keyword+semantic",
        },
    ]
}

// ---------------------------------------------------------------------------
// Gold set construction (INDEPENDENT of search results)
// ---------------------------------------------------------------------------

/// Build gold sets by scanning ALL chunks in the database, not search results.
/// Returns one Vec<String> per query, containing chunk IDs of relevant chunks.
/// Gold sets are constant across all search configurations.
fn build_gold_sets(store: &Store, queries: &[GoldQuery]) -> Vec<Vec<String>> {
    let mut stmt = store
        .conn()
        .prepare(
            "SELECT id, COALESCE(file_path,''), COALESCE(symbol_name,''), \
                COALESCE(snippet,''), COALESCE(title,''), COALESCE(signature,'') \
         FROM chunks WHERE kind = 'code'",
        )
        .unwrap();

    let all_chunks: Vec<(i64, String)> = stmt
        .query_map([], |row| {
            let id: i64 = row.get(0)?;
            let searchable = format!(
                "{} {} {} {} {}",
                row.get::<_, String>(1)?,
                row.get::<_, String>(2)?,
                row.get::<_, String>(3)?,
                row.get::<_, String>(4)?,
                row.get::<_, String>(5)?,
            )
            .to_lowercase();
            Ok((id, searchable))
        })
        .unwrap()
        .filter_map(|r| r.ok())
        .collect();

    eprintln!(
        "Building gold sets from {} total chunks...",
        all_chunks.len()
    );

    queries
        .iter()
        .enumerate()
        .map(|(qi, gq)| {
            if gq.query_type == "negative" || gq.expected.is_empty() {
                return vec![];
            }
            let gold: Vec<String> = all_chunks
                .iter()
                .filter(|(_, searchable)| {
                    gq.expected
                        .iter()
                        .any(|exp| searchable.contains(&exp.to_lowercase()))
                })
                .map(|(id, _)| id.to_string())
                .collect();
            assert!(
                !gold.is_empty(),
                "Q{qi} '{}' has no gold chunks — expected terms {:?} matched nothing in DB",
                gq.query,
                gq.expected
            );
            eprintln!(
                "  Q{qi}: {:50} {} gold chunks (expected: {:?})",
                gq.query,
                gold.len(),
                gq.expected
            );
            gold
        })
        .collect()
}

/// Check which expected terms were found in any result (for hit_rate reporting).
fn find_matches(results: &[grasshopper::store::SearchHit], expected: &[&str]) -> Vec<String> {
    let mut found = Vec::new();
    for hit in results {
        let searchable = format!(
            "{} {} {} {} {}",
            hit.file_path.as_deref().unwrap_or(""),
            hit.symbol_name.as_deref().unwrap_or(""),
            hit.snippet,
            hit.title,
            hit.signature.as_deref().unwrap_or(""),
        )
        .to_lowercase();

        for &exp in expected {
            let exp_lower = exp.to_lowercase();
            if searchable.contains(&exp_lower) && !found.contains(&exp_lower) {
                found.push(exp_lower);
            }
        }
    }
    found
}

// ---------------------------------------------------------------------------
// Search configs
// ---------------------------------------------------------------------------

enum SearchConfig {
    FtsOnly,
    Hybrid,
    Full,
}

fn run_search(
    store: &Store,
    query: &str,
    config: &SearchConfig,
    embedder: &mut Option<grasshopper::code::embed::Embedder>,
    reranker: &mut Option<Reranker>,
    hnsw: &Option<HnswIndex>,
) -> Vec<grasshopper::store::SearchHit> {
    let limit = 10;
    match config {
        SearchConfig::FtsOnly => {
            let ctx = SearchContext {
                store,
                query,
                kind_filter: Some("code"),
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
                kind_filter: Some("code"),
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
        SearchConfig::Full => {
            let ctx = SearchContext {
                store,
                query,
                kind_filter: Some("code"),
                limit,
                threshold: None,
                embedder: embedder.as_mut(),
                reranker: reranker.as_mut(),
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
    eprintln!("=== Code Search Accuracy Benchmark ===\n");

    // Setup: index grasshopper src/ with embeddings
    let dir = TempDir::new().unwrap();
    let db_path = dir.path().join("bench.db");
    let store = Store::open(&db_path).unwrap();

    eprintln!("Indexing Grasshopper src/ with embeddings...");
    let (codebase_id, embedder) = common::fixtures::index_self_with_embeddings(&store).unwrap();
    let (code_count, _) = store.count_by_kind().unwrap();
    eprintln!("Indexed {code_count} code chunks\n");

    // Build HNSW
    let embeddings = store.get_all_embeddings().unwrap();
    let hnsw = if !embeddings.is_empty() {
        Some(HnswIndex::from_embeddings(&embeddings).unwrap())
    } else {
        None
    };

    let mut embedder = Some(embedder);
    let mut reranker = Some(Reranker::new().unwrap());

    let queries = gold_queries();

    // Build gold sets ONCE from all chunks — independent of search results
    let gold_sets = build_gold_sets(&store, &queries);
    eprintln!();

    let configs = [
        ("FTS-only", SearchConfig::FtsOnly),
        ("Hybrid (no rerank)", SearchConfig::Hybrid),
        ("Full pipeline", SearchConfig::Full),
    ];

    let mut report = BenchReport::new("accuracy-code");
    report.add_section(
        "setup",
        serde_json::json!({
            "code_chunks": code_count,
            "codebase_id": codebase_id,
            "hnsw_points": hnsw.as_ref().map(|h| h.len()).unwrap_or(0),
            "queries": queries.len(),
            "gold_set_sizes": gold_sets.iter().map(|g| g.len()).collect::<Vec<_>>(),
        }),
    );

    for (config_name, config) in &configs {
        eprintln!("--- {config_name} ---");
        let mut query_results = Vec::new();
        let mut total_p1 = 0.0;
        let mut total_p3 = 0.0;
        let mut total_p5 = 0.0;
        let mut total_mrr = 0.0;
        let mut total_ndcg = 0.0;
        let non_negative: Vec<usize> = queries
            .iter()
            .enumerate()
            .filter(|(_, q)| q.query_type != "negative")
            .map(|(i, _)| i)
            .collect();

        for (qi, gq) in queries.iter().enumerate() {
            let hits = run_search(
                &store,
                gq.query,
                config,
                &mut embedder,
                &mut reranker,
                &hnsw,
            );

            if gq.query_type == "negative" {
                // Negative queries should return no results. Any hit is a false positive.
                let false_positive = !hits.is_empty();
                query_results.push(serde_json::json!({
                    "query": gq.query,
                    "type": gq.query_type,
                    "hits": hits.len(),
                    "false_positive": false_positive,
                }));
                continue;
            }

            // Ranked IDs: chunk IDs in the order returned by search (guaranteed unique)
            let ranked_ids: Vec<String> = hits.iter().map(|h| h.id.to_string()).collect();
            // Gold IDs: pre-computed from ALL chunks, constant across configs
            let gold_ids = &gold_sets[qi];

            let p1 = precision_at_k(&ranked_ids, gold_ids, 1);
            let p3 = precision_at_k(&ranked_ids, gold_ids, 3);
            let p5 = precision_at_k(&ranked_ids, gold_ids, 5);
            let m = mrr(&ranked_ids, gold_ids);
            let ndcg = ndcg_at_k(&ranked_ids, gold_ids, 10);

            // Hit rate: fraction of expected terms found anywhere in results
            let matches = find_matches(&hits, &gq.expected);
            let hit_rate = if gq.expected.is_empty() {
                1.0
            } else {
                matches.len() as f64 / gq.expected.len() as f64
            };

            total_p1 += p1;
            total_p3 += p3;
            total_p5 += p5;
            total_mrr += m;
            total_ndcg += ndcg;

            eprintln!(
                "  {:50} P@1={:.2} MRR={:.2} hit_rate={:.0}% ({}/{}) gold={}",
                gq.query,
                p1,
                m,
                hit_rate * 100.0,
                matches.len(),
                gq.expected.len(),
                gold_ids.len(),
            );

            query_results.push(serde_json::json!({
                "query": gq.query,
                "type": gq.query_type,
                "hits": hits.len(),
                "expected": gq.expected,
                "found": matches,
                "gold_set_size": gold_ids.len(),
                "hit_rate": hit_rate,
                "p@1": p1,
                "p@3": p3,
                "p@5": p5,
                "mrr": m,
                "ndcg@10": ndcg,
            }));
        }

        let n = non_negative.len() as f64;
        let avg_metrics = serde_json::json!({
            "avg_p@1": total_p1 / n,
            "avg_p@3": total_p3 / n,
            "avg_p@5": total_p5 / n,
            "avg_mrr": total_mrr / n,
            "avg_ndcg@10": total_ndcg / n,
        });

        eprintln!(
            "  Avg P@1={:.3} P@3={:.3} P@5={:.3} MRR={:.3} NDCG@10={:.3}\n",
            total_p1 / n,
            total_p3 / n,
            total_p5 / n,
            total_mrr / n,
            total_ndcg / n,
        );

        report.add_section(
            config_name,
            serde_json::json!({
                "aggregate": avg_metrics,
                "queries": query_results,
            }),
        );
    }

    report.add_section(
        "timing",
        serde_json::json!({
            "total_seconds": start.elapsed().as_secs_f64(),
        }),
    );

    report.save("bench/results/accuracy-code.json").unwrap();
    eprintln!("Done in {:.1}s", start.elapsed().as_secs_f64());
}
