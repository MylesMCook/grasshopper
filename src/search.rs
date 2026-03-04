use anyhow::Result;

use crate::rerank::Reranker;
use crate::store::{hybrid_search, SearchHit, Store};

/// Max candidates to pass to cross-encoder (latency control).
const RERANK_CANDIDATES: usize = 20;

/// Unified search: runs FTS (always), vector (if embedder provided),
/// merges via RRF, then optionally reranks via cross-encoder.
pub fn search(
    store: &Store,
    query: &str,
    kind_filter: Option<&str>,
    limit: usize,
    embedder: Option<&mut ferret::embed::Embedder>,
    reranker: Option<&mut Reranker>,
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

    let merged = if fts_results.is_empty() {
        vec_results
    } else if vec_results.is_empty() {
        fts_results
    } else {
        hybrid_search(&fts_results, &vec_results, limit)
    };

    // Rerank via cross-encoder if available
    if let Some(reranker) = reranker {
        rerank_hits(reranker, query, merged, limit)
    } else {
        Ok(merged)
    }
}

/// Apply cross-encoder reranking to search hits.
/// Takes top RERANK_CANDIDATES, scores them, returns top `limit`.
pub fn rerank_hits(
    reranker: &mut Reranker,
    query: &str,
    hits: Vec<SearchHit>,
    limit: usize,
) -> Result<Vec<SearchHit>> {
    if hits.is_empty() {
        return Ok(hits);
    }

    let n = RERANK_CANDIDATES.min(hits.len());
    let passages: Vec<String> = hits[..n]
        .iter()
        .map(|h| h.snippet.clone())
        .collect();

    let ranked = reranker.rerank(query, &passages, limit)?;
    Ok(ranked.into_iter().map(|(idx, _score)| hits[idx].clone()).collect())
}
