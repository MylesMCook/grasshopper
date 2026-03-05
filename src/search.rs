use std::collections::HashSet;

use anyhow::Result;

use crate::rerank::Reranker;
use crate::store::{hybrid_search, SearchHit, Store};

/// Max candidates to pass to cross-encoder (latency control).
const RERANK_CANDIDATES: usize = 20;

/// Max expanded query variants (original + expansions).
const MAX_EXPANSIONS: usize = 6;

/// Common stop words to strip for a tighter query variant.
const STOP_WORDS: &[&str] = &[
    "a", "an", "the", "is", "are", "was", "were", "be", "been", "being",
    "have", "has", "had", "do", "does", "did", "will", "would", "could",
    "should", "may", "might", "can", "shall", "to", "of", "in", "for",
    "on", "with", "at", "by", "from", "as", "into", "about", "it", "its",
    "this", "that", "and", "or", "but", "not", "so", "if", "then",
];

/// Tokenize a query into lowercase alphanumeric words.
fn tokenize(query: &str) -> Vec<String> {
    query
        .split(|c: char| !c.is_alphanumeric() && c != '_' && c != '-')
        .filter(|w| !w.is_empty())
        .map(|w| w.to_lowercase())
        .collect()
}

/// Generate expanded query variants for better FTS recall.
/// Returns the original query first, then deterministic expansions:
/// stop-word-stripped variant, 2-token subsets, and quoted adjacent pairs.
/// Capped at MAX_EXPANSIONS total.
pub fn expand_query(query: &str) -> Vec<String> {
    let tokens = tokenize(query);
    let mut variants: Vec<String> = vec![query.to_string()];

    if tokens.len() <= 1 {
        return variants;
    }

    let stop_set: HashSet<&str> = STOP_WORDS.iter().copied().collect();

    // Stop-word-stripped variant
    let content_tokens: Vec<&str> = tokens
        .iter()
        .map(|t| t.as_str())
        .filter(|t| !stop_set.contains(t))
        .collect();
    if content_tokens.len() >= 2 && content_tokens.len() < tokens.len() {
        variants.push(content_tokens.join(" "));
    }

    // Quoted adjacent pairs (phrase matching)
    for pair in tokens.windows(2) {
        if variants.len() >= MAX_EXPANSIONS {
            break;
        }
        let phrase = format!("\"{}\"", pair.join(" "));
        variants.push(phrase);
    }

    // 2-token subsets from content tokens (skip if too many)
    if content_tokens.len() >= 3 && content_tokens.len() <= 8 {
        for i in 0..content_tokens.len() {
            for j in (i + 1)..content_tokens.len() {
                if variants.len() >= MAX_EXPANSIONS {
                    break;
                }
                variants.push(format!("{} {}", content_tokens[i], content_tokens[j]));
            }
        }
    }

    variants.truncate(MAX_EXPANSIONS);
    variants
}

/// Run expanded FTS queries and merge results via RRF.
/// Vector search still runs once on the original query (embeddings capture semantics).
pub fn expanded_fts_search(
    store: &Store,
    query: &str,
    kind_filter: Option<&str>,
    limit: usize,
) -> Result<Vec<SearchHit>> {
    let variants = expand_query(query);
    if variants.len() <= 1 {
        return store.fts_search(query, kind_filter, limit);
    }

    let mut all_results: Vec<Vec<SearchHit>> = Vec::new();
    for (i, variant) in variants.iter().enumerate() {
        match store.fts_search(variant, kind_filter, limit) {
            Ok(hits) if !hits.is_empty() => all_results.push(hits),
            Ok(_) => {}
            Err(e) => {
                if i == 0 {
                    // Original query failure is a real error — propagate it
                    return Err(e);
                }
                tracing::debug!("Expanded FTS query failed for '{}': {e}", variant);
            }
        }
    }

    if all_results.is_empty() {
        return Ok(vec![]);
    }
    if all_results.len() == 1 {
        return Ok(all_results.into_iter().next().unwrap());
    }

    // Merge all result lists pairwise via RRF
    let mut merged = all_results.remove(0);
    for results in all_results {
        merged = hybrid_search(&merged, &results, limit);
    }

    Ok(merged)
}

/// Unified search: runs FTS (always), vector (if embedder provided),
/// merges via RRF, then optionally reranks via cross-encoder.
/// When an HNSW index is provided, vector search uses O(log N) ANN instead of O(N) brute-force.
pub fn search(
    store: &Store,
    query: &str,
    kind_filter: Option<&str>,
    limit: usize,
    embedder: Option<&mut crate::code::embed::Embedder>,
    reranker: Option<&mut Reranker>,
    hnsw: Option<&crate::code::hnsw::HnswIndex>,
) -> Result<Vec<SearchHit>> {
    let fts_results = store.fts_search(query, kind_filter, limit)?;

    let vec_results = if let Some(emb) = embedder {
        let query_vec = emb.embed_batch(&[query.to_string()])?;
        if query_vec.is_empty() {
            vec![]
        } else if let Some(hnsw) = hnsw {
            store.vector_search_hnsw(hnsw, &query_vec[0], kind_filter, limit)?
        } else {
            store.vector_search(&query_vec[0], crate::code::embed::MODEL_NAME, kind_filter, limit)?
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
            hit.reranker_score = Some(score);
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_expand_query_single_word_passthrough() {
        let variants = expand_query("SQLite");
        assert_eq!(variants.len(), 1);
        assert_eq!(variants[0], "SQLite");
    }

    #[test]
    fn test_expand_query_basic() {
        let variants = expand_query("SQLite WAL concurrency");
        assert!(variants.len() >= 2, "should produce expansions: {variants:?}");
        assert_eq!(variants[0], "SQLite WAL concurrency"); // original always first
    }

    #[test]
    fn test_expand_query_stop_word_removal() {
        let variants = expand_query("the best way to configure SQLite");
        // Should have a variant without stop words (lowercased tokens)
        assert!(
            variants.iter().any(|v| !v.contains("the") && v.contains("configure") && v.contains("sqlite")),
            "should have stop-word-stripped variant: {variants:?}"
        );
    }

    #[test]
    fn test_expand_query_cap_at_max() {
        let long_query = "one two three four five six seven eight nine ten";
        let variants = expand_query(long_query);
        assert!(variants.len() <= MAX_EXPANSIONS, "should cap at {MAX_EXPANSIONS}: got {}", variants.len());
    }
}
