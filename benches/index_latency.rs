//! Index latency benchmark using Criterion.
//!
//! Measures latency of indexing pipeline components:
//! scan, chunk, upsert, FTS rebuild, embed, HNSW build/serialize.
//!
//! Run: cargo bench --bench index_latency

mod common;

use criterion::{Criterion, criterion_group, criterion_main};
use grasshopper::code::embed::{Embedder, default_cache_dir};
use grasshopper::code::hnsw::HnswIndex;
use grasshopper::store::Store;
use std::path::Path;
use tempfile::TempDir;

fn bench_scan(c: &mut Criterion) {
    let src_dir = Path::new(env!("CARGO_MANIFEST_DIR")).join("src");

    let mut group = c.benchmark_group("scan");
    group.sample_size(20);

    group.bench_function("scan_src", |b| {
        b.iter(|| {
            grasshopper::code::scan::scan_directory(&src_dir).unwrap();
        });
    });

    group.finish();
}

fn bench_chunk(c: &mut Criterion) {
    let src_dir = Path::new(env!("CARGO_MANIFEST_DIR")).join("src");
    let scan = grasshopper::code::scan::scan_directory(&src_dir).unwrap();

    let mut group = c.benchmark_group("chunk");
    group.sample_size(20);

    // Single file
    if let Some(file) = scan.files.first() {
        let content = file.content.clone();
        let rel_path = file.rel_path.clone();
        let language = file.language.clone();
        group.bench_function("single_file", |b| {
            b.iter(|| {
                grasshopper::code::chunk::chunk_content(&rel_path, &content, &language);
            });
        });
    }

    // Batch of 10
    let batch: Vec<_> = scan.files.iter().take(10).collect();
    if batch.len() >= 5 {
        group.bench_function("batch_10", |b| {
            b.iter(|| {
                for file in &batch {
                    grasshopper::code::chunk::chunk_content(
                        &file.rel_path,
                        &file.content,
                        &file.language,
                    );
                }
            });
        });
    }

    group.finish();
}

fn bench_upsert(c: &mut Criterion) {
    let src_dir = Path::new(env!("CARGO_MANIFEST_DIR")).join("src");
    let scan = grasshopper::code::scan::scan_directory(&src_dir).unwrap();

    // Pre-chunk everything
    let file_chunks: Vec<grasshopper::store::FileChunks> = scan
        .files
        .iter()
        .map(|file| {
            let chunks = grasshopper::code::chunk::chunk_content(
                &file.rel_path,
                &file.content,
                &file.language,
            );
            grasshopper::store::FileChunks {
                file_path: file.rel_path.clone(),
                file_hash: file.hash.clone(),
                chunks: chunks
                    .into_iter()
                    .map(|pc| grasshopper::store::CodeChunkParams {
                        chunk_key: pc.chunk_key,
                        file_path: file.rel_path.clone(),
                        language: pc.language,
                        symbol_kind: pc.kind,
                        symbol_name: pc.name,
                        signature: pc.signature,
                        snippet: pc.snippet,
                        start_line: pc.start_line as i64,
                        end_line: pc.end_line as i64,
                        file_hash: file.hash.clone(),
                    })
                    .collect(),
            }
        })
        .collect();

    let mut group = c.benchmark_group("upsert");
    group.sample_size(10);

    group.bench_function("batch_upsert", |b| {
        b.iter(|| {
            let dir = TempDir::new().unwrap();
            let store = Store::open(&dir.path().join("bench.db")).unwrap();
            let cb = store.get_or_create_codebase("/tmp/bench", "bench").unwrap();
            store.batch_upsert_chunks(cb, &file_chunks).unwrap();
        });
    });

    group.finish();
}

fn bench_fts_rebuild(c: &mut Criterion) {
    let mut group = c.benchmark_group("fts_rebuild");
    group.sample_size(10);

    group.bench_function("rebuild_after_index", |b| {
        b.iter_with_setup(
            || {
                let dir = TempDir::new().unwrap();
                let db_path = dir.path().join("bench.db");
                let store = Store::open(&db_path).unwrap();
                let src_dir = Path::new(env!("CARGO_MANIFEST_DIR")).join("src");
                let result = grasshopper::index::index_directory(&store, &src_dir).unwrap();
                (dir, store, result.codebase_id)
            },
            |(_dir, store, codebase_id)| {
                store.rebuild_fts_for_codebase(codebase_id).unwrap();
            },
        );
    });

    group.finish();
}

fn bench_embed(c: &mut Criterion) {
    let mut group = c.benchmark_group("embed");
    group.sample_size(10);

    let cache_dir = default_cache_dir();
    let mut embedder = Embedder::new(&cache_dir).unwrap();

    let texts: Vec<String> = (0..32)
        .map(|i| format!("fn function_{i}() {{ println!(\"hello from function {i}\"); }}"))
        .collect();

    group.bench_function("embed_batch_32", |b| {
        b.iter(|| {
            embedder.embed_batch(&texts).unwrap();
        });
    });

    group.finish();
}

fn bench_hnsw(c: &mut Criterion) {
    // Generate random embeddings
    fn random_embeddings(n: usize, dim: usize) -> Vec<(i64, Vec<f32>)> {
        use std::collections::hash_map::DefaultHasher;
        use std::hash::{Hash, Hasher};

        (0..n)
            .map(|i| {
                let mut v = vec![0.0f32; dim];
                for (j, val) in v.iter_mut().enumerate() {
                    let mut h = DefaultHasher::new();
                    (i * dim + j).hash(&mut h);
                    *val = (h.finish() as f32 / u64::MAX as f32) * 2.0 - 1.0;
                }
                // L2 normalize
                let norm: f32 = v.iter().map(|x| x * x).sum::<f32>().sqrt();
                if norm > 0.0 {
                    for val in &mut v {
                        *val /= norm;
                    }
                }
                (i as i64, v)
            })
            .collect()
    }

    let mut group = c.benchmark_group("hnsw");
    group.sample_size(10);

    for &n in &[1000, 5000] {
        let embeddings = random_embeddings(n, 768);

        group.bench_function(format!("build_{n}"), |b| {
            b.iter(|| {
                HnswIndex::from_embeddings(&embeddings).unwrap();
            });
        });
    }

    // Serialize roundtrip
    let embeddings_1k = random_embeddings(1000, 768);
    let index = HnswIndex::from_embeddings(&embeddings_1k).unwrap();

    group.bench_function("serialize_roundtrip_1k", |b| {
        b.iter(|| {
            let dir = TempDir::new().unwrap();
            let path = dir.path().join("bench.hnsw");
            index.save(&path).unwrap();
            HnswIndex::load(&path).unwrap();
        });
    });

    group.finish();
}

criterion_group!(
    benches,
    bench_scan,
    bench_chunk,
    bench_upsert,
    bench_fts_rebuild,
    bench_embed,
    bench_hnsw,
);
criterion_main!(benches);
