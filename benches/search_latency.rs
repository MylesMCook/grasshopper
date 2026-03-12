//! Search latency benchmark using Criterion.
//!
//! Pre-populates a temp DB with Grasshopper's own source (indexed + embedded),
//! then measures latency of each pipeline component.
//!
//! Run: cargo bench --bench search_latency

mod common;

use criterion::{Criterion, criterion_group, criterion_main};
use grasshopper::code::embed::Embedder;
use grasshopper::code::hnsw::HnswIndex;
use grasshopper::rerank::Reranker;
use grasshopper::search::{SearchContext, expanded_fts_search, rerank_hits, unified_search};
use grasshopper::store::Store;
use std::sync::Mutex;
use tempfile::TempDir;

struct BenchState {
    _dir: TempDir,
    store: Store,
    embedder: Mutex<Embedder>,
    reranker: Mutex<Reranker>,
    hnsw: HnswIndex,
}

fn build_memory_embedding_text(title: &str, content: &str, descriptors: &str) -> String {
    let tags = descriptors
        .split(',')
        .map(str::trim)
        .filter(|tag| !tag.is_empty())
        .collect::<Vec<_>>();

    if tags.is_empty() {
        format!("{title}\n{content}")
    } else {
        format!("{title}\n{content}\nDescriptors: {}", tags.join(", "))
    }
}

fn seed_memory_embeddings(store: &Store, embedder: &mut Embedder) {
    common::fixtures::create_memories(store, 64).unwrap();

    let mut stmt = store
        .conn()
        .prepare(
            "SELECT id, COALESCE(title,''), COALESCE(content,''), COALESCE(descriptors,'') \
             FROM chunks WHERE kind = 'memory'",
        )
        .unwrap();
    let rows: Vec<(i64, String, String, String)> = stmt
        .query_map([], |row| Ok((row.get(0)?, row.get(1)?, row.get(2)?, row.get(3)?)))
        .unwrap()
        .filter_map(|row| row.ok())
        .collect();

    for batch in rows.chunks(32) {
        let texts: Vec<String> = batch
            .iter()
            .map(|(_, title, content, descriptors)| {
                build_memory_embedding_text(title, content, descriptors)
            })
            .collect();
        let vectors = embedder.embed_batch(&texts).unwrap();
        let items: Vec<(i64, &[f32], &str)> = batch
            .iter()
            .zip(vectors.iter())
            .map(|((id, _, _, _), vector)| {
                (*id, vector.as_slice(), grasshopper::code::embed::MODEL_NAME)
            })
            .collect();
        store.batch_upsert_embeddings(&items).unwrap();
    }
}

fn setup() -> BenchState {
    let dir = TempDir::new().unwrap();
    let db_path = dir.path().join("bench.db");
    let store = Store::open(&db_path).unwrap();

    let (_, mut embedder) = common::fixtures::index_self_with_embeddings(&store).unwrap();
    seed_memory_embeddings(&store, &mut embedder);

    let embeddings = store.get_all_embeddings().unwrap();
    let hnsw = HnswIndex::from_embeddings(&embeddings).unwrap();

    let reranker = Reranker::new().unwrap();

    BenchState {
        _dir: dir,
        store,
        embedder: Mutex::new(embedder),
        reranker: Mutex::new(reranker),
        hnsw,
    }
}

fn bench_fts_search(c: &mut Criterion) {
    let state = setup();

    let mut group = c.benchmark_group("fts_search");
    group.sample_size(20);

    group.bench_function("single_word", |b| {
        b.iter(|| {
            expanded_fts_search(&state.store, "search", Some("code"), 10).unwrap();
        });
    });

    group.bench_function("multi_word", |b| {
        b.iter(|| {
            expanded_fts_search(&state.store, "hybrid search results", Some("code"), 10).unwrap();
        });
    });

    group.bench_function("expanded_query", |b| {
        b.iter(|| {
            expanded_fts_search(
                &state.store,
                "how does the search pipeline handle reranking",
                Some("code"),
                10,
            )
            .unwrap();
        });
    });

    group.finish();
}

fn bench_vector_search(c: &mut Criterion) {
    let state = setup();

    let mut group = c.benchmark_group("vector_search");
    group.sample_size(10);

    group.bench_function("hnsw_lookup", |b| {
        let mut emb = state.embedder.lock().unwrap();
        let query_vec = emb.embed_batch(&["search pipeline".to_string()]).unwrap();
        let qv = &query_vec[0];
        b.iter(|| {
            state
                .store
                .vector_search_hnsw(&state.hnsw, qv, Some("code"), 10)
                .unwrap();
        });
    });

    group.finish();
}

fn bench_rrf_fusion(c: &mut Criterion) {
    let state = setup();

    let mut group = c.benchmark_group("rrf_fusion");
    group.sample_size(30);

    let fts_20 = expanded_fts_search(&state.store, "search", Some("code"), 20).unwrap();
    let fts_100 = expanded_fts_search(&state.store, "search", Some("code"), 100).unwrap();

    group.bench_function("merge_20", |b| {
        let half = fts_20.len() / 2;
        let (a, bb) = fts_20.split_at(half.max(1));
        b.iter(|| {
            grasshopper::store::hybrid_search(a, bb, 10);
        });
    });

    if fts_100.len() >= 20 {
        group.bench_function("merge_100", |b| {
            let half = fts_100.len() / 2;
            let (a, bb) = fts_100.split_at(half);
            b.iter(|| {
                grasshopper::store::hybrid_search(a, bb, 10);
            });
        });
    }

    group.finish();
}

fn bench_reranking(c: &mut Criterion) {
    let state = setup();

    let mut group = c.benchmark_group("reranking");
    group.sample_size(10);

    let hits =
        expanded_fts_search(&state.store, "hybrid search pipeline", Some("code"), 20).unwrap();

    for &n in &[5, 10, 20] {
        if hits.len() >= n {
            let subset = &hits[..n];
            group.bench_function(format!("rerank_{n}"), |b| {
                b.iter(|| {
                    let mut reranker = state.reranker.lock().unwrap();
                    rerank_hits(&mut reranker, "hybrid search pipeline", subset, n).unwrap();
                });
            });
        }
    }

    group.finish();
}

fn bench_full_pipeline(c: &mut Criterion) {
    let state = setup();

    let mut group = c.benchmark_group("full_pipeline");
    group.sample_size(10);

    group.bench_function("fts_only_code", |b| {
        b.iter(|| {
            let ctx = SearchContext {
                store: &state.store,
                query: "how does search work",
                kind_filter: Some("code"),
                limit: 10,
                threshold: None,
                embedder: None,
                reranker: None,
                hnsw: None,
            };
            unified_search(ctx).unwrap();
        });
    });

    group.bench_function("hybrid_code", |b| {
        b.iter(|| {
            let mut emb = state.embedder.lock().unwrap();
            let ctx = SearchContext {
                store: &state.store,
                query: "how does search work",
                kind_filter: Some("code"),
                limit: 10,
                threshold: None,
                embedder: Some(&mut *emb),
                reranker: None,
                hnsw: Some(&state.hnsw),
            };
            unified_search(ctx).unwrap();
        });
    });

    group.bench_function("memory_with_rerank", |b| {
        b.iter(|| {
            let mut emb = state.embedder.lock().unwrap();
            let mut reranker = state.reranker.lock().unwrap();
            let ctx = SearchContext {
                store: &state.store,
                query: "procedure topic-3 retrieve patterns",
                kind_filter: Some("memory"),
                limit: 10,
                threshold: None,
                embedder: Some(&mut *emb),
                reranker: Some(&mut *reranker),
                hnsw: Some(&state.hnsw),
            };
            unified_search(ctx).unwrap();
        });
    });

    group.finish();
}

criterion_group!(
    benches,
    bench_fts_search,
    bench_vector_search,
    bench_rrf_fusion,
    bench_reranking,
    bench_full_pipeline,
);
criterion_main!(benches);
