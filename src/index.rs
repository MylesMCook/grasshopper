use anyhow::{Context, Result};
use rayon::prelude::*;
use std::collections::HashSet;
use std::path::Path;
use std::time::Instant;

use crate::store::{CodeChunkParams, FileChunks, Store};

const FILE_BATCH: usize = 200;
const EMBED_BATCH: usize = 32;

/// Result of an indexing operation.
#[derive(Debug)]
pub struct IndexResult {
    pub files_scanned: usize,
    pub files_changed: usize,
    pub files_skipped: usize,
    pub files_removed: usize,
    pub chunks_written: usize,
    pub edges_written: usize,
    pub errors: Vec<String>,
    pub duration_ms: u64,
    pub codebase_id: i64,
}

/// Index a directory: scan → chunk → graph → FTS.
/// Embedding is separate (call embed_codebase after this).
pub fn index_directory(store: &Store, dir: &Path) -> Result<IndexResult> {
    let start = Instant::now();

    // 1. Resolve and register codebase
    let root = dir.canonicalize().with_context(|| format!("resolving {}", dir.display()))?;
    let root_str = root.to_string_lossy();
    let dir_name = root
        .file_name()
        .map(|n| n.to_string_lossy().into_owned())
        .unwrap_or_default();
    let codebase_id = store.get_or_create_codebase(&root_str, &dir_name)?;

    // 2. Scan directory
    let scan_result = ferret::scan::scan_directory(&root)?;
    let mut errors = scan_result.errors;
    let total_files = scan_result.files.len();

    // 3. Diff against stored hashes
    let stored_hashes = store.get_all_file_hashes(codebase_id)?;
    let mut files_to_process = Vec::new();
    let mut skipped = 0usize;
    for file in &scan_result.files {
        if stored_hashes.get(&file.rel_path).map(String::as_str) == Some(&file.hash) {
            skipped += 1;
        } else {
            files_to_process.push(file);
        }
    }

    // 4. Chunk and extract graph tags in parallel batches
    let mut total_chunks = 0usize;
    let mut total_edges = 0usize;
    let mut changed_files = Vec::new();

    for batch in files_to_process.chunks(FILE_BATCH) {
        let results: Vec<_> = batch
            .par_iter()
            .map(|file| {
                let chunks = ferret::chunk::chunk_file(&file.rel_path, &file.content, &file.language);
                let tags = ferret::graph::get_tags_query(&file.language)
                    .and_then(|q| {
                        ferret::graph::get_language(&file.language).map(|lang| (lang, q))
                    })
                    .and_then(|(lang, q)| ferret::graph::extract_tags(&file.content, lang, q).ok())
                    .unwrap_or_default();
                (file, chunks, tags)
            })
            .collect();

        let mut file_chunks_batch = Vec::new();
        for (file, chunk_result, tags) in results {
            match chunk_result {
                Ok(parsed) => {
                    let params: Vec<CodeChunkParams> = parsed
                        .into_iter()
                        .map(|pc| parsed_to_params(pc, file))
                        .collect();
                    total_chunks += params.len();
                    changed_files.push(file.rel_path.clone());
                    file_chunks_batch.push(FileChunks {
                        file_path: file.rel_path.clone(),
                        file_hash: file.hash.clone(),
                        chunks: params,
                    });
                    if !tags.is_empty() {
                        total_edges += tags.len();
                        store.upsert_graph_edges_for_file(codebase_id, &file.rel_path, &tags)?;
                    }
                }
                Err(e) => errors.push(format!("{}: {e}", file.rel_path)),
            }
        }
        if !file_chunks_batch.is_empty() {
            store.batch_upsert_chunks(codebase_id, &file_chunks_batch)?;
        }
    }

    // 5. Remove stale files
    let active_files: HashSet<String> = scan_result.files.iter().map(|f| f.rel_path.clone()).collect();
    let removed = store.remove_stale_files(codebase_id, &active_files)?;

    // 6. FTS sync
    if !changed_files.is_empty() || removed > 0 {
        if skipped == 0 || changed_files.len() > total_files / 2 {
            store.rebuild_fts_for_codebase(codebase_id)?;
        } else {
            store.sync_fts_for_files(codebase_id, &changed_files)?;
        }
    }

    // 7. Finalize
    store.touch_codebase(codebase_id)?;
    store.optimize()?;

    Ok(IndexResult {
        files_scanned: total_files,
        files_changed: changed_files.len(),
        files_skipped: skipped,
        files_removed: removed,
        chunks_written: total_chunks,
        edges_written: total_edges,
        errors,
        duration_ms: start.elapsed().as_millis() as u64,
        codebase_id,
    })
}

/// Embed any code chunks that don't have embeddings yet.
/// Call after index_directory(). Opt-in via --embed flag.
pub fn embed_codebase(
    store: &Store,
    embedder: &mut ferret::embed::Embedder,
    codebase_id: i64,
) -> Result<usize> {
    let stale = store.get_stale_embeddings(codebase_id, ferret::embed::MODEL_NAME)?;
    if stale.is_empty() {
        return Ok(0);
    }

    let total = stale.len();
    let mut embedded = 0usize;

    for batch in stale.chunks(EMBED_BATCH) {
        let texts: Vec<String> = batch
            .iter()
            .map(|s| {
                ferret::embed::build_embed_text(
                    &s.file_path,
                    &s.language,
                    &s.symbol_kind,
                    &s.symbol_name,
                    &s.signature,
                    &s.snippet,
                )
            })
            .collect();

        let vectors = embedder.embed_batch(&texts)?;
        let items: Vec<(i64, &[f32], &str)> = batch
            .iter()
            .zip(vectors.iter())
            .map(|(s, v)| (s.id, v.as_slice(), ferret::embed::MODEL_NAME))
            .collect();
        store.batch_upsert_embeddings(&items)?;
        embedded += batch.len();
        eprintln!("  Embedded {embedded}/{total}");
    }

    Ok(embedded)
}

fn parsed_to_params(
    pc: ferret::chunk::ParsedChunk,
    file: &ferret::scan::ScannedFile,
) -> CodeChunkParams {
    CodeChunkParams {
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
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    #[test]
    fn test_index_empty_directory() {
        let dir = TempDir::new().unwrap();
        let db_path = dir.path().join("test.db");
        let store = Store::open(&db_path).unwrap();

        let src_dir = dir.path().join("empty_project");
        std::fs::create_dir_all(&src_dir).unwrap();

        let result = index_directory(&store, &src_dir).unwrap();
        assert_eq!(result.files_scanned, 0);
        assert_eq!(result.chunks_written, 0);
    }

    #[test]
    fn test_index_single_rust_file() {
        let dir = TempDir::new().unwrap();
        let db_path = dir.path().join("test.db");
        let store = Store::open(&db_path).unwrap();

        let src_dir = dir.path().join("project");
        std::fs::create_dir_all(&src_dir).unwrap();
        std::fs::write(
            src_dir.join("main.rs"),
            "fn hello() {\n    println!(\"hi\");\n}\n\nfn world() {\n    println!(\"world\");\n}\n",
        )
        .unwrap();

        let result = index_directory(&store, &src_dir).unwrap();
        assert_eq!(result.files_scanned, 1);
        assert!(result.chunks_written >= 2, "expected >=2 chunks, got {}", result.chunks_written);
        assert!(result.edges_written > 0, "expected graph edges");

        // Verify FTS works
        let hits = store.fts_search("hello", None, 10).unwrap();
        assert!(!hits.is_empty(), "FTS should find 'hello'");
    }

    #[test]
    fn test_incremental_skip_unchanged() {
        let dir = TempDir::new().unwrap();
        let db_path = dir.path().join("test.db");
        let store = Store::open(&db_path).unwrap();

        let src_dir = dir.path().join("project");
        std::fs::create_dir_all(&src_dir).unwrap();
        std::fs::write(src_dir.join("lib.rs"), "fn stable() {}\n").unwrap();

        // First index
        let r1 = index_directory(&store, &src_dir).unwrap();
        assert_eq!(r1.files_changed, 1);

        // Second index — nothing changed
        let r2 = index_directory(&store, &src_dir).unwrap();
        assert_eq!(r2.files_changed, 0);
        assert_eq!(r2.files_skipped, 1);
    }

    #[test]
    fn test_stale_file_removal() {
        let dir = TempDir::new().unwrap();
        let db_path = dir.path().join("test.db");
        let store = Store::open(&db_path).unwrap();

        let src_dir = dir.path().join("project");
        std::fs::create_dir_all(&src_dir).unwrap();
        std::fs::write(src_dir.join("a.rs"), "fn a() {}\n").unwrap();
        std::fs::write(src_dir.join("b.rs"), "fn b() {}\n").unwrap();

        index_directory(&store, &src_dir).unwrap();
        let (code, _) = store.count_by_kind().unwrap();
        assert!(code >= 2);

        // Delete b.rs and re-index
        std::fs::remove_file(src_dir.join("b.rs")).unwrap();
        let r2 = index_directory(&store, &src_dir).unwrap();
        assert_eq!(r2.files_removed, 1);
    }
}
