use anyhow::Result;

use crate::store::{hybrid_search, SearchHit, Store};

/// Unified search: runs FTS (always), vector (if embedder provided),
/// then merges via RRF if both sources return results.
pub fn search(
    store: &Store,
    query: &str,
    kind_filter: Option<&str>,
    limit: usize,
    embedder: Option<&mut ferret::embed::Embedder>,
) -> Result<Vec<SearchHit>> {
    let fts_results = store.fts_search(query, kind_filter, limit)?;

    let vec_results = if let Some(emb) = embedder {
        let query_vec = emb.embed_batch(&[query.to_string()])?;
        if query_vec.is_empty() {
            vec![]
        } else {
            store.vector_search(&query_vec[0], ferret::embed::MODEL_NAME, kind_filter, limit)?
        }
    } else {
        vec![]
    };

    if fts_results.is_empty() {
        Ok(vec_results)
    } else if vec_results.is_empty() {
        Ok(fts_results)
    } else {
        Ok(hybrid_search(&fts_results, &vec_results, limit))
    }
}
