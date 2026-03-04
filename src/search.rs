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

    // Rerank via cross-encoder if available (graceful fallback on error)
    if let Some(reranker) = reranker {
        match rerank_hits(reranker, query, merged.clone(), limit) {
            Ok(reranked) => Ok(reranked),
            Err(e) => {
                tracing::warn!("reranking failed, returning unreranked results: {e}");
                Ok(merged)
            }
        }
    } else {
        Ok(merged)
    }
}

/// Build a meaningful passage for cross-encoder reranking.
/// Code entries use snippet (source text); memory entries use title + content
/// since their snippet column is empty by design.
fn rerank_passage(hit: &SearchHit) -> String {
    if hit.kind == "memory" || hit.snippet.is_empty() {
        if hit.title.is_empty() {
            hit.snippet.clone()
        } else {
            format!("{}: {}", hit.title, hit.snippet)
        }
    } else {
        hit.snippet.clone()
    }
}

/// Apply cross-encoder reranking to search hits.
/// Takes top RERANK_CANDIDATES, scores them with the cross-encoder,
/// writes reranker scores into hit.score, then appends any remaining
/// un-reranked hits to preserve the caller's `limit` contract.
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
        .map(rerank_passage)
        .collect();

    let ranked = reranker.rerank(query, &passages, limit.min(n))?;

    // Build result: reranked hits with cross-encoder scores written in
    let mut result: Vec<SearchHit> = ranked
        .into_iter()
        .map(|(idx, score)| {
            let mut hit = hits[idx].clone();
            hit.score = score as f64;
            hit
        })
        .collect();

    // Append remaining un-reranked hits to preserve limit contract
    if result.len() < limit && n < hits.len() {
        let reranked_ids: std::collections::HashSet<i64> =
            result.iter().map(|h| h.id).collect();
        for hit in &hits[n..] {
            if result.len() >= limit {
                break;
            }
            if !reranked_ids.contains(&hit.id) {
                result.push(hit.clone());
            }
        }
    }

    result.truncate(limit);
    Ok(result)
}
