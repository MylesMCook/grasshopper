use anyhow::Result;
use grasshopper::store::{MemoryParams, Store};

/// Create N memories with varied characteristics for benchmarking.
pub fn create_memories(store: &Store, count: usize) -> Result<Vec<i64>> {
    let types = ["knowledge", "episode", "procedure", "identity"];
    let mut ids = Vec::with_capacity(count);

    for i in 0..count {
        let mtype = types[i % types.len()];
        let days_ago = (i as f64 * 1.5) % 120.0;
        let salience = 0.3 + (i as f64 % 7.0) * 0.1;
        let created = chrono::Utc::now() - chrono::Duration::seconds((days_ago * 86400.0) as i64);
        let accessed =
            chrono::Utc::now() - chrono::Duration::seconds(((days_ago * 0.5) * 86400.0) as i64);

        let content = format!(
            "Memory entry {i} about topic-{} with details about feature-{} and component-{}. \
             This relates to {} operations and {} patterns used in the system.",
            i % 10,
            i % 5,
            i % 8,
            mtype,
            ["search", "index", "store", "retrieve"][i % 4]
        );
        let title = format!("Memory {i}: {mtype} about topic-{}", i % 10);
        let hash = format!("bench-hash-{i}");

        let id = store.insert_memory(&MemoryParams {
            title: &title,
            content: &content,
            memory_type: mtype,
            descriptors: &format!("bench,topic-{}", i % 10),
            salience,
            content_hash: &hash,
        })?;

        // Set controlled timestamps via raw SQL
        store.execute_batch(&format!(
            "UPDATE chunks SET created_at = '{}', last_accessed = '{}' WHERE id = {}",
            created.to_rfc3339(),
            accessed.to_rfc3339(),
            id,
        ))?;

        ids.push(id);
    }

    // Rebuild FTS for the new memories
    store.execute_batch(
        "DELETE FROM chunks_fts; \
         INSERT INTO chunks_fts (rowid, title, content, snippet, symbol_name, descriptors) \
         SELECT id, code_expand(COALESCE(title,'')), code_expand(COALESCE(content,'')), \
                code_expand(COALESCE(snippet,'')), code_expand(COALESCE(symbol_name,'')), \
                COALESCE(descriptors,'') \
         FROM chunks",
    )?;

    Ok(ids)
}

/// Index grasshopper's own src/ directory into a temp store.
/// Returns (store, codebase_id).
pub fn index_self(store: &Store) -> Result<i64> {
    let src_dir = std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("src");
    let result = grasshopper::index::index_directory(store, &src_dir)?;
    Ok(result.codebase_id)
}

/// Index grasshopper's own src/ with embeddings.
/// Returns (codebase_id, embedder).
pub fn index_self_with_embeddings(
    store: &Store,
) -> Result<(i64, grasshopper::code::embed::Embedder)> {
    let codebase_id = index_self(store)?;
    let cache_dir = grasshopper::code::embed::default_cache_dir();
    let mut embedder = grasshopper::code::embed::Embedder::new(&cache_dir)?;
    grasshopper::index::embed_codebase(store, &mut embedder, codebase_id)?;
    Ok((codebase_id, embedder))
}
