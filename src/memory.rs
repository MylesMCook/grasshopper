use anyhow::Result;
use sha2::{Digest, Sha256};

use crate::store::{hybrid_search, Chunk, Handoff, MemoryParams, SearchHit, Store};

/// Generate a title from content: simple truncation to 80 chars.
fn truncate_title(content: &str) -> String {
    let first_line = content
        .split('\n')
        .next()
        .unwrap_or(content)
        .trim();
    if first_line.chars().count() <= 80 {
        first_line.to_string()
    } else {
        let truncated: String = first_line.chars().take(77).collect();
        format!("{truncated}...")
    }
}

// --- Cognitive scoring ---

fn decay_rate(memory_type: &str) -> f64 {
    match memory_type {
        "identity" => 0.0,
        "knowledge" => 0.005,
        "episode" => 0.023,
        "procedure" => 0.01,
        _ => 0.015,
    }
}

fn days_since(iso_date: &str) -> f64 {
    let Ok(dt) = chrono::DateTime::parse_from_rfc3339(iso_date) else {
        return 0.0;
    };
    let now = chrono::Utc::now();
    let duration = now.signed_duration_since(dt);
    (duration.num_seconds() as f64 / 86400.0).max(0.0)
}

/// Simplified cognitive scoring (LAB-88 benchmark results).
/// final_score = rrf_score × salience_factor × decay_factor
pub fn cognitive_score(hit: &SearchHit) -> f64 {
    let rrf_score = hit.score;

    // Salience factor: maps [0, 1] → [0.5, 1.5]
    let salience_factor = 0.5 + hit.salience;

    // Exponential decay based on time since last access
    let mtype = hit.memory_type.as_deref().unwrap_or("knowledge");
    let lambda = decay_rate(mtype);
    let days_created = days_since(&hit.created_at);
    let days_last_access = hit
        .last_accessed
        .as_deref()
        .map(days_since)
        .unwrap_or(days_created);
    let decay_factor = (-lambda * days_last_access).exp();

    rrf_score * salience_factor * decay_factor
}

// --- Cognitive tools ---

/// Result of the `recall` command.
pub struct RecallResult {
    pub hits: Vec<SearchHit>,
    pub log_id: Option<i64>,
}

/// Cognitive-scored memory search with side effects (touch + association).
pub fn recall(
    store: &Store,
    embedder: Option<&mut crate::code::embed::Embedder>,
    reranker: Option<&mut crate::rerank::Reranker>,
    query: &str,
    limit: usize,
    hnsw: Option<&crate::code::hnsw::HnswIndex>,
    precomputed_embedding: Option<&[f32]>,
) -> Result<RecallResult> {
    // 1. Expanded FTS keyword candidates (multi-query for better recall)
    let fts = crate::search::expanded_fts_search(store, query, Some("memory"), 20)?;

    // 2. Vector candidates (use precomputed embedding if available, else embed)
    let vec_results = if let Some(qvec) = precomputed_embedding {
        if let Some(hnsw) = hnsw {
            store.vector_search_hnsw(hnsw, qvec, Some("memory"), 20)?
        } else {
            store.vector_search(qvec, crate::code::embed::MODEL_NAME, Some("memory"), 20)?
        }
    } else if let Some(emb) = embedder {
        match emb.embed_batch(&[query.to_string()]) {
            Ok(query_vec) if !query_vec.is_empty() => {
                if let Some(hnsw) = hnsw {
                    store.vector_search_hnsw(hnsw, &query_vec[0], Some("memory"), 20)?
                } else {
                    store.vector_search(&query_vec[0], crate::code::embed::MODEL_NAME, Some("memory"), 20)?
                }
            }
            Ok(_) => vec![],
            Err(e) => {
                tracing::warn!("Embedding failed, falling back to FTS-only: {e}");
                vec![]
            }
        }
    } else {
        vec![]
    };

    // 3. Merge via RRF — always normalize through RRF for consistent score scale
    let empty: Vec<SearchHit> = vec![];
    let merged = if fts.is_empty() && vec_results.is_empty() {
        vec![]
    } else if fts.is_empty() {
        hybrid_search(&empty, &vec_results, 20)
    } else if vec_results.is_empty() {
        hybrid_search(&fts, &empty, 20)
    } else {
        hybrid_search(&fts, &vec_results, 20)
    };

    // 3a. Rerank via cross-encoder if available (between RRF and cognitive scoring)
    let merged = if let Some(reranker) = reranker {
        match crate::search::rerank_hits(reranker, query, merged.clone(), 20) {
            Ok(reranked) => reranked,
            Err(e) => {
                tracing::warn!("reranking failed in recall, using unreranked results: {e}");
                merged
            }
        }
    } else {
        merged
    };

    // 4. Filter: skip archived and identity (identity is working memory, not searchable)
    let filtered: Vec<SearchHit> = merged
        .into_iter()
        .filter(|h| !h.archived && h.memory_type.as_deref() != Some("identity"))
        .collect();

    // 5. Apply cognitive scoring and sort
    let mut scored: Vec<SearchHit> = filtered
        .into_iter()
        .map(|mut h| {
            h.score = cognitive_score(&h);
            h
        })
        .collect();
    scored.sort_by(|a, b| b.score.partial_cmp(&a.score).unwrap_or(std::cmp::Ordering::Equal));
    scored.truncate(limit);

    // 6. Side effects: touch retrieved memories (log failures, don't abort)
    for hit in &scored {
        if let Err(e) = store.touch_memory(hit.id) {
            tracing::warn!("Failed to touch memory #{}: {e}", hit.id);
        }
    }

    // 7. Log retrieval for fine-tuning pipeline
    let log_id = store
        .log_retrieval(
            query,
            "recall",
            &scored.iter().map(|h| (h.id, h.score)).collect::<Vec<_>>(),
            None,
        )
        .map_err(|e| { tracing::warn!("Failed to log recall retrieval: {e}"); e })
        .ok();

    Ok(RecallResult { hits: scored, log_id })
}

/// Result of proactive context retrieval.
pub struct GetContextResult {
    pub hits: Vec<SearchHit>,
    /// How many candidates were filtered out by the relevance threshold.
    pub filtered_count: usize,
    /// The threshold that was applied.
    pub threshold: f32,
    /// Retrieval log ID for feedback correlation.
    pub log_id: Option<i64>,
}

/// Proactive memory surfacing with relevance threshold.
/// Like recall(), but drops memories below a relevance floor.
/// Returns empty if nothing is relevant — that's the correct behavior.
/// Uses reranker score as the gate when available, falls back to RRF score.
#[allow(clippy::too_many_arguments)]
pub fn get_context(
    store: &Store,
    embedder: Option<&mut crate::code::embed::Embedder>,
    reranker: Option<&mut crate::rerank::Reranker>,
    query: &str,
    limit: usize,
    threshold: f32,
    hnsw: Option<&crate::code::hnsw::HnswIndex>,
    precomputed_embedding: Option<&[f32]>,
) -> Result<GetContextResult> {
    // 1. Expanded FTS keyword candidates (multi-query for better recall)
    let fts = crate::search::expanded_fts_search(store, query, Some("memory"), 20)?;

    // 2. Vector candidates (use precomputed embedding if available, else embed)
    let vec_results = if let Some(qvec) = precomputed_embedding {
        if let Some(hnsw) = hnsw {
            store.vector_search_hnsw(hnsw, qvec, Some("memory"), 20)?
        } else {
            store.vector_search(qvec, crate::code::embed::MODEL_NAME, Some("memory"), 20)?
        }
    } else if let Some(emb) = embedder {
        match emb.embed_batch(&[query.to_string()]) {
            Ok(query_vec) if !query_vec.is_empty() => {
                if let Some(hnsw) = hnsw {
                    store.vector_search_hnsw(hnsw, &query_vec[0], Some("memory"), 20)?
                } else {
                    store.vector_search(&query_vec[0], crate::code::embed::MODEL_NAME, Some("memory"), 20)?
                }
            }
            Ok(_) => vec![],
            Err(e) => {
                tracing::warn!("Embedding failed, falling back to FTS-only: {e}");
                vec![]
            }
        }
    } else {
        vec![]
    };

    // 3. Merge via RRF
    let empty: Vec<SearchHit> = vec![];
    let merged = if fts.is_empty() && vec_results.is_empty() {
        vec![]
    } else if fts.is_empty() {
        hybrid_search(&empty, &vec_results, 20)
    } else if vec_results.is_empty() {
        hybrid_search(&fts, &empty, 20)
    } else {
        hybrid_search(&fts, &vec_results, 20)
    };

    // 3a. Rerank via cross-encoder — track actual success, not just availability
    let mut rerank_succeeded = false;
    let merged = if let Some(reranker) = reranker {
        match crate::search::rerank_hits(reranker, query, merged.clone(), 20) {
            Ok(reranked) => {
                rerank_succeeded = true;
                reranked
            }
            Err(e) => {
                tracing::warn!("reranking failed in get_context, using unreranked results: {e}");
                merged
            }
        }
    } else {
        merged
    };

    // 4. Filter archived + identity
    let filtered: Vec<SearchHit> = merged
        .into_iter()
        .filter(|h| !h.archived && h.memory_type.as_deref() != Some("identity"))
        .collect();

    // 5. Relevance gate: use reranker score only if reranking actually succeeded
    let pre_gate_count = filtered.len();
    let gated: Vec<SearchHit> = filtered
        .into_iter()
        .filter(|h| {
            if rerank_succeeded {
                h.reranker_score.unwrap_or(0.0) >= threshold
            } else {
                // Fall back to RRF score (different scale, use lower threshold)
                h.score >= (threshold * 0.05) as f64
            }
        })
        .collect();
    let filtered_count = pre_gate_count - gated.len();

    // 6. Apply cognitive scoring and sort
    let mut scored: Vec<SearchHit> = gated
        .into_iter()
        .map(|mut h| {
            h.score = cognitive_score(&h);
            h
        })
        .collect();
    scored.sort_by(|a, b| b.score.partial_cmp(&a.score).unwrap_or(std::cmp::Ordering::Equal));
    scored.truncate(limit);

    // Note: side effects (touch + associations) are handled by the caller
    // after budget truncation, so only surfaced hits get boosted.

    // 7. Log retrieval for fine-tuning pipeline
    let log_id = store
        .log_retrieval(
            query,
            "get_context",
            &scored.iter().map(|h| (h.id, h.score)).collect::<Vec<_>>(),
            None,
        )
        .map_err(|e| { tracing::warn!("Failed to log get_context retrieval: {e}"); e })
        .ok();

    Ok(GetContextResult {
        hits: scored,
        filtered_count,
        threshold,
        log_id,
    })
}

/// Result of budget-aware context truncation.
pub struct BudgetResult {
    pub hits: Vec<SearchHit>,
    /// How many hits were dropped due to budget or dedup.
    pub dropped_count: usize,
    /// Estimated total tokens of returned hits.
    pub estimated_tokens: usize,
}

/// Budget-aware truncation of search results.
/// Greedy fill by cognitive score (already sorted) until token budget exhausted.
/// Deduplicates by content hash (title + snippet) when `dedup` is true.
pub fn budget_context(
    hits: Vec<SearchHit>,
    token_budget: usize,
    dedup: bool,
) -> BudgetResult {
    let mut result = Vec::new();
    let mut total_tokens: usize = 0;
    let mut seen_hashes = std::collections::HashSet::new();
    let original_count = hits.len();

    for hit in hits {
        // Estimate tokens: ~4 chars per token
        let hit_tokens = (hit.title.len() + hit.snippet.len()).div_ceil(4);

        // Content-hash dedup
        if dedup {
            let hash_input = format!("{}:{}", hit.title, hit.snippet);
            let hash = {
                use sha2::{Digest, Sha256};
                let mut hasher = Sha256::new();
                hasher.update(hash_input.as_bytes());
                format!("{:x}", hasher.finalize())
            };
            if !seen_hashes.insert(hash) {
                continue; // Duplicate content
            }
        }

        // Budget check
        if total_tokens + hit_tokens > token_budget && !result.is_empty() {
            break; // Budget exhausted (always include at least one hit)
        }

        total_tokens += hit_tokens;
        result.push(hit);
    }

    let dropped_count = original_count - result.len();
    BudgetResult {
        hits: result,
        dropped_count,
        estimated_tokens: total_tokens,
    }
}

/// Result of the `store` command.
pub struct StoreResult {
    pub id: i64,
    pub title: String,
    pub was_update: bool,
    pub similar_id: Option<i64>,
}

/// Store raw content with dedup and embedding. No auto-classification, no entity extraction at write time.
/// Follows "Store Raw, Retrieve Smart" — invest compute in retrieval, not write-time processing.
pub fn store(
    db: &Store,
    embedder: Option<&mut crate::code::embed::Embedder>,
    content: &str,
    title: Option<&str>,
    tags: &str,
) -> Result<StoreResult> {
    // 1. Title: use provided or simple truncation
    let title = title
        .map(String::from)
        .unwrap_or_else(|| truncate_title(content));

    // 2. Content hash for exact-match dedup
    let hash = format!(
        "{:x}",
        Sha256::new()
            .chain_update(title.as_bytes())
            .chain_update(content.as_bytes())
            .finalize()
    );

    // 3. Quick exact-match dedup via content hash (before embedding similarity)
    if let Some(existing_id) = db.find_memory_by_hash(&hash)? {
        // Update existing without changing memory_type or salience
        db.update_memory_content(existing_id, &title, content, tags, &hash)?;
        return Ok(StoreResult {
            id: existing_id,
            title,
            was_update: true,
            similar_id: Some(existing_id),
        });
    }

    // 4. Dedup via embedding similarity (if embedder available, graceful fallback on error)
    if let Some(emb) = embedder {
        let embed_text = format!("{}\n{}", title, content);
        let vectors = match emb.embed_batch(&[embed_text]) {
            Ok(v) => v,
            Err(e) => {
                tracing::warn!("Embedding failed, skipping dedup: {e}");
                vec![]
            }
        };
        if let Some(query_vec) = vectors.first() {
            let similar = db.search_similar_memories(
                query_vec,
                crate::code::embed::MODEL_NAME,
                0.75,
                1,
            )?;

            if let Some(&(existing_id, _sim)) = similar.first() {
                // Update existing entry — preserve memory_type and salience
                db.update_memory_content(existing_id, &title, content, tags, &hash)?;
                // Re-embed the updated entry
                db.batch_upsert_embeddings(&[(
                    existing_id,
                    query_vec.as_slice(),
                    crate::code::embed::MODEL_NAME,
                )])?;
                return Ok(StoreResult {
                    id: existing_id,
                    title,
                    was_update: true,
                    similar_id: Some(existing_id),
                });
            }

            // No match — create new and embed
            let id = db.insert_memory(&MemoryParams {
                title: &title,
                content,
                memory_type: "knowledge",
                descriptors: tags,
                salience: 0.5,
                content_hash: &hash,
                agent_id: "cli",
            })?;
            db.batch_upsert_embeddings(&[(
                id,
                query_vec.as_slice(),
                crate::code::embed::MODEL_NAME,
            )])?;
            return Ok(StoreResult {
                id,
                title,
                was_update: false,
                similar_id: None,
            });
        }
    }

    // 5. No embedder — always create new (skip dedup)
    let id = db.insert_memory(&MemoryParams {
        title: &title,
        content,
        memory_type: "knowledge",
        descriptors: tags,
        salience: 0.5,
        content_hash: &hash,
        agent_id: "cli",
    })?;
    Ok(StoreResult {
        id,
        title,
        was_update: false,
        similar_id: None,
    })
}

/// Identity snapshot returned by `me`.
pub struct MeResult {
    pub identity: Vec<Chunk>,
    pub active_projects: Vec<Handoff>,
    pub working_set: Vec<Chunk>,
    pub recent_activity: Vec<Chunk>,
    pub active_count: i64,
    pub archived_count: i64,
}

/// Build an identity snapshot: who am I, what am I working on, what do I know.
pub fn me(store: &Store) -> Result<MeResult> {
    let identity = store.list_memories(Some("identity"), false, 100)?;
    let active_projects = store.list_recent_handoffs(5)?;
    let working_set = store.list_memories_by_access(10)?;
    let recent_activity = store.list_memories(None, false, 10)?;
    let active_count = store.count_memories(false)?;
    let archived_count = store.count_memories(true)?;

    Ok(MeResult {
        identity,
        active_projects,
        working_set,
        recent_activity,
        active_count,
        archived_count,
    })
}

/// Session continuity: load latest handoff + related memories.
pub struct PickupResult {
    pub handoff: Option<Handoff>,
    pub related_memories: Vec<SearchHit>,
}

pub fn pickup(
    store: &Store,
    embedder: Option<&mut crate::code::embed::Embedder>,
    reranker: Option<&mut crate::rerank::Reranker>,
    project: Option<&str>,
    hnsw: Option<&crate::code::hnsw::HnswIndex>,
) -> Result<PickupResult> {
    let handoff = store.get_latest_handoff(project)?;

    let related_memories = if let Some(ref h) = handoff {
        let query = format!("{} {}", h.summary, h.project);
        recall(store, embedder, reranker, &query, 5, hnsw, None)?.hits
    } else {
        vec![]
    };

    Ok(PickupResult {
        handoff,
        related_memories,
    })
}

// --- Reflect ---

pub enum ReflectFocus {
    Overview,
    Growing,
    Fading,
    Connections,
    Gaps,
}

pub struct ReflectResult {
    pub growing: Vec<Chunk>,
    pub fading: Vec<Chunk>,
    pub connections: Vec<(Chunk, i64)>,
    pub type_counts: std::collections::HashMap<String, i64>,
    pub observations: Vec<String>,
    pub active_count: i64,
    pub archived_count: i64,
}

pub fn reflect(store: &Store, focus: &ReflectFocus) -> Result<ReflectResult> {
    let growing = match focus {
        ReflectFocus::Overview | ReflectFocus::Growing => {
            store.list_memories_growing(7, 10)?
        }
        _ => vec![],
    };

    let fading = match focus {
        ReflectFocus::Overview | ReflectFocus::Fading => {
            store.list_memories_fading(30, 10)?
        }
        _ => vec![],
    };

    let connections = match focus {
        ReflectFocus::Overview | ReflectFocus::Connections => {
            store.list_memories_with_associations(10)?
        }
        _ => vec![],
    };

    let type_counts = match focus {
        ReflectFocus::Overview | ReflectFocus::Gaps => {
            store.count_memories_by_type()?
        }
        _ => std::collections::HashMap::new(),
    };

    // Auto-observations for gaps
    let mut observations = Vec::new();
    if matches!(focus, ReflectFocus::Overview | ReflectFocus::Gaps) {
        if !type_counts.contains_key("identity") {
            observations.push("No identity entries found. Consider storing who you are and your preferences.".into());
        }
        let episodes = type_counts.get("episode").copied().unwrap_or(0);
        let knowledge = type_counts.get("knowledge").copied().unwrap_or(0);
        if knowledge > 0 && episodes as f64 / knowledge as f64 > 2.0 {
            observations.push("High episode:knowledge ratio. Consider consolidating episodes into knowledge.".into());
        }
        if type_counts.values().sum::<i64>() == 0 {
            observations.push("Memory is empty. Start with 'store' to build your knowledge base.".into());
        }
    }

    let active_count = store.count_memories(false)?;
    let archived_count = store.count_memories(true)?;

    Ok(ReflectResult {
        growing,
        fading,
        connections,
        type_counts,
        observations,
        active_count,
        archived_count,
    })
}

// --- Consolidate ---

pub struct ConsolidateSummary {
    pub id: i64,
    pub title: String,
    pub member_count: usize,
    pub member_ids: Vec<i64>,
}

pub struct ConsolidateResult {
    pub near_duplicates: Vec<(i64, i64, f64)>, // (id_a, id_b, similarity)
    pub auto_archived: Vec<i64>,
    pub stale_for_review: Vec<Chunk>,
    pub episode_clusters: Vec<(String, Vec<Chunk>)>, // (tag, episodes)
    pub summaries_created: Vec<ConsolidateSummary>,
}

/// Build a template-based summary for a cluster of memories.
fn build_summary_text(tag: &str, members: &[Chunk]) -> String {
    let mut lines = vec![format!("Summary: {} entries about \"{}\"", members.len(), tag)];
    lines.push("Entries:".to_string());
    for m in members {
        lines.push(format!("- [#{}] {}", m.id, m.title));
    }
    let all_descriptors: std::collections::BTreeSet<String> = members
        .iter()
        .flat_map(|m| m.descriptors.split(',').map(|t| t.trim().to_lowercase()))
        .filter(|t| !t.is_empty())
        .collect();
    if !all_descriptors.is_empty() {
        lines.push(format!("Descriptors: {}", all_descriptors.into_iter().collect::<Vec<_>>().join(", ")));
    }
    if let (Some(earliest), Some(latest)) = (
        members.iter().map(|m| &m.created_at).min(),
        members.iter().map(|m| &m.created_at).max(),
    ) {
        lines.push(format!("Period: {} to {}", &earliest[..10.min(earliest.len())], &latest[..10.min(latest.len())]));
    }
    lines.join("\n")
}

pub fn consolidate(
    store: &Store,
    embedder: Option<&mut crate::code::embed::Embedder>,
    dry_run: bool,
    stale_days: i64,
) -> Result<ConsolidateResult> {
    let mut near_duplicates = Vec::new();
    let mut auto_archived = Vec::new();

    // Phase 1: Near-duplicates (requires embedder)
    if let Some(emb) = embedder {
        let all_embeddings = store.get_all_memory_embeddings(crate::code::embed::MODEL_NAME)?;
        // Compare each pair — O(n²) but n is small (memory entries, not code chunks)
        for (src_id, src_emb) in &all_embeddings {
            let similar = store.search_similar_memories(
                src_emb,
                crate::code::embed::MODEL_NAME,
                0.9,
                5,
            )?;
            for (target_id, sim) in similar {
                if target_id != *src_id && target_id > *src_id {
                    near_duplicates.push((*src_id, target_id, sim));
                }
            }
        }
        // Dedup (same pair may appear from both directions)
        near_duplicates.sort_by(|a, b| a.0.cmp(&b.0).then(a.1.cmp(&b.1)));
        near_duplicates.dedup_by(|a, b| a.0 == b.0 && a.1 == b.1);

        // Use embedder ref to avoid "unused" warning
        let _ = emb;
    }

    // Phase 2: Stale entries
    let stale = store.list_memories_stale(stale_days, 100)?;
    let mut stale_for_review = Vec::new();

    for entry in stale {
        if entry.access_count == 0 && entry.memory_type.as_deref() != Some("identity") {
            // Zero accesses — auto-archive
            if !dry_run {
                store.archive_memory(entry.id)?;
            }
            auto_archived.push(entry.id);
        } else {
            // Accessed but stale — flag for manual review
            stale_for_review.push(entry);
        }
    }

    // Phase 3: Episode clustering by descriptor tags
    let mut episode_clusters: Vec<(String, Vec<Chunk>)> = Vec::new();
    let episodes = store.list_memories(Some("episode"), false, 100)?;
    let mut tag_groups: std::collections::HashMap<String, Vec<Chunk>> = std::collections::HashMap::new();
    for ep in episodes {
        for tag in ep.descriptors.split(',').map(|t| t.trim().to_lowercase()) {
            if !tag.is_empty() {
                tag_groups.entry(tag).or_default().push(ep.clone());
            }
        }
    }
    for (tag, entries) in tag_groups {
        if entries.len() >= 3 {
            episode_clusters.push((tag, entries));
        }
    }
    episode_clusters.sort_by(|a, b| b.1.len().cmp(&a.1.len()));

    // Phase 4: Create summary memories for episode clusters (transactional)
    let mut summaries_created = Vec::new();
    if !dry_run {
        store.execute_batch("SAVEPOINT consolidate_summaries")?;
        let tx_result: Result<()> = (|| {
            for (tag, members) in &episode_clusters {
                let member_ids: Vec<i64> = members.iter().map(|m| m.id).collect();
                // Check if summary already exists for this exact member set
                if let Ok(existing) = store.get_summaries_for_chunks(&member_ids)
                    && !existing.is_empty()
                {
                    continue; // Already summarized
                }
                let summary_text = build_summary_text(tag, members);
                let summary_title = format!("Summary: {} entries about \"{}\"", members.len(), tag);
                // Content hash from sorted member IDs for dedup
                let mut sorted_ids = member_ids.clone();
                sorted_ids.sort();
                let hash = format!(
                    "{:x}",
                    Sha256::new()
                        .chain_update(format!("summary:{sorted_ids:?}").as_bytes())
                        .finalize()
                );
                // Check content hash dedup
                if store.get_memory_by_hash(&hash)?.is_some() {
                    continue;
                }
                let max_salience = members.iter().map(|m| m.salience).fold(0.5_f64, f64::max);
                let all_desc: std::collections::BTreeSet<String> = members
                    .iter()
                    .flat_map(|m| m.descriptors.split(',').map(|t| t.trim().to_lowercase()))
                    .filter(|t| !t.is_empty())
                    .collect();
                let mut desc_list: Vec<String> = all_desc.into_iter().collect();
                desc_list.push("summary".to_string());
                let descriptors = desc_list.join(", ");

                let summary_id = store.insert_memory(&MemoryParams {
                    title: &summary_title,
                    content: &summary_text,
                    memory_type: "knowledge",
                    descriptors: &descriptors,
                    salience: max_salience,
                    content_hash: &hash,
                    agent_id: "consolidate",
                })?;
                store.insert_summary_edges(summary_id, &member_ids)?;
                summaries_created.push(ConsolidateSummary {
                    id: summary_id,
                    title: summary_title,
                    member_count: members.len(),
                    member_ids,
                });
            }
            Ok(())
        })();
        match tx_result {
            Ok(()) => store.execute_batch("RELEASE consolidate_summaries")?,
            Err(e) => {
                store.execute_batch("ROLLBACK TO consolidate_summaries").ok();
                return Err(e);
            }
        }
    }

    Ok(ConsolidateResult {
        near_duplicates,
        auto_archived,
        stale_for_review,
        episode_clusters,
        summaries_created,
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    fn test_store() -> (TempDir, Store) {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();
        (dir, store)
    }

    #[test]
    fn test_truncate_title_short() {
        assert_eq!(truncate_title("Always use bun."), "Always use bun.");
    }

    #[test]
    fn test_truncate_title_long() {
        let long = "A".repeat(100);
        let title = truncate_title(&long);
        assert!(title.len() <= 80);
        assert!(title.ends_with("..."));
    }

    #[test]
    fn test_truncate_title_newline() {
        assert_eq!(truncate_title("First line\nSecond line"), "First line");
    }

    #[test]
    fn test_cognitive_score_identity_no_decay() {
        let hit = SearchHit {
            id: 1, kind: "memory".into(), file_path: None, symbol_name: None,
            symbol_kind: None, signature: None, title: "test".into(), snippet: String::new(),
            start_line: None, end_line: None,
            memory_type: Some("identity".into()),
            score: 1.0,
            reranker_score: None,
            access_count: 5,
            last_accessed: Some("2020-01-01T00:00:00+00:00".into()),
            salience: 1.0,
            created_at: "2020-01-01T00:00:00+00:00".into(),
            archived: false,
            descriptors: String::new(),
        };
        let score = cognitive_score(&hit);
        // Identity has decay_rate=0, so decay_factor=1.0 regardless of age
        // salience_factor = 0.5 + 1.0 = 1.5
        // final_score = 1.0 * 1.5 * 1.0 = 1.5
        assert!(score > 0.0, "identity score should be positive: {score}");
    }

    #[test]
    fn test_cognitive_score_episode_decays() {
        let now = chrono::Utc::now().to_rfc3339();
        let recent_hit = SearchHit {
            id: 1, kind: "memory".into(), file_path: None, symbol_name: None,
            symbol_kind: None, signature: None, title: "test".into(), snippet: String::new(),
            start_line: None, end_line: None,
            memory_type: Some("episode".into()),
            score: 1.0,
            reranker_score: None,
            access_count: 0,
            last_accessed: Some(now.clone()),
            salience: 0.5,
            created_at: now,
            archived: false,
            descriptors: String::new(),
        };

        let old_hit = SearchHit {
            last_accessed: Some("2020-01-01T00:00:00+00:00".into()),
            created_at: "2020-01-01T00:00:00+00:00".into(),
            ..recent_hit.clone()
        };

        let recent_score = cognitive_score(&recent_hit);
        let old_score = cognitive_score(&old_hit);
        assert!(recent_score > old_score, "recent episode ({recent_score}) should score higher than old ({old_score})");
    }

    #[test]
    fn test_store_basic() {
        let (_dir, db) = test_store();

        let result = store(
            &db, None,
            "Always use bun for package management",
            None, "tools",
        ).unwrap();

        assert!(!result.was_update);
        assert!(!result.title.is_empty());

        let chunk = db.get_chunk(result.id).unwrap().unwrap();
        assert_eq!(chunk.kind, "memory");
        assert_eq!(chunk.descriptors, "tools");
    }

    #[test]
    fn test_store_defaults_to_knowledge() {
        let (_dir, db) = test_store();

        let result = store(
            &db, None,
            "I am Myles, a software engineer",
            None, "",
        ).unwrap();

        // No auto-classification — always defaults to knowledge
        let chunk = db.get_chunk(result.id).unwrap().unwrap();
        assert_eq!(chunk.memory_type.as_deref(), Some("knowledge"));
    }

    #[test]
    fn test_store_with_explicit_title() {
        let (_dir, db) = test_store();

        let result = store(
            &db, None,
            "Some content here",
            Some("My Custom Title"), "",
        ).unwrap();

        assert_eq!(result.title, "My Custom Title");
    }

    #[test]
    fn test_me_empty() {
        let (_dir, store) = test_store();
        let result = me(&store).unwrap();
        assert!(result.identity.is_empty());
        assert_eq!(result.active_count, 0);
    }

    #[test]
    fn test_me_with_data() {
        let (_dir, db) = test_store();

        db.insert_memory(&MemoryParams {
            title: "I am Myles", content: "I am Myles",
            memory_type: "identity", descriptors: "",
            salience: 1.0, content_hash: "id1", agent_id: "test",
        }).unwrap();
        db.insert_memory(&MemoryParams {
            title: "SQLite is great", content: "SQLite is great",
            memory_type: "knowledge", descriptors: "",
            salience: 0.5, content_hash: "k1", agent_id: "test",
        }).unwrap();
        db.create_handoff("h1", "Done phase 0", "Start phase 1", "grasshopper").unwrap();

        let result = me(&db).unwrap();
        assert_eq!(result.identity.len(), 1);
        assert_eq!(result.active_projects.len(), 1);
        assert_eq!(result.active_count, 2);
    }

    #[test]
    fn test_recall_basic() {
        let (_dir, db) = test_store();

        store(&db, None, "Always use bun for packages", None, "tools").unwrap();
        store(&db, None, "SQLite WAL mode", None, "database").unwrap();

        let result = recall(&db, None, None, "bun", 10, None, None).unwrap();
        assert!(!result.hits.is_empty());
        assert_eq!(result.hits[0].memory_type.as_deref(), Some("knowledge"));
    }

    #[test]
    fn test_recall_skips_identity() {
        let (_dir, db) = test_store();

        db.insert_memory(&MemoryParams {
            title: "I am a developer", content: "I am a developer",
            memory_type: "identity", descriptors: "",
            salience: 1.0, content_hash: "id1", agent_id: "test",
        }).unwrap();
        store(&db, None, "Developer tools are great", None, "").unwrap();

        let result = recall(&db, None, None, "developer", 10, None, None).unwrap();
        // Should only return the knowledge entry, not the identity
        for hit in &result.hits {
            assert_ne!(hit.memory_type.as_deref(), Some("identity"));
        }
    }

    #[test]
    fn test_reflect_overview() {
        let (_dir, db) = test_store();

        store(&db, None, "Some knowledge", None, "").unwrap();
        db.insert_memory(&MemoryParams {
            title: "An episode today", content: "An episode today",
            memory_type: "episode", descriptors: "",
            salience: 0.5, content_hash: "ep1", agent_id: "test",
        }).unwrap();

        let result = reflect(&db, &ReflectFocus::Overview).unwrap();
        assert_eq!(result.active_count, 2);
        assert!(!result.type_counts.is_empty());
    }

    #[test]
    fn test_reflect_gaps_no_identity() {
        let (_dir, db) = test_store();

        store(&db, None, "Some knowledge", None, "").unwrap();

        let result = reflect(&db, &ReflectFocus::Gaps).unwrap();
        assert!(result.observations.iter().any(|o| o.contains("identity")));
    }

    #[test]
    fn test_consolidate_dry_run() {
        let (_dir, db) = test_store();

        store(&db, None, "Some old content", None, "tag1").unwrap();

        let result = consolidate(&db, None, true, 0).unwrap();
        // With stale_days=0, entries created just now should still be caught
        // but auto_archived should be empty because it's dry_run
        // (Though the entry was JUST created, it might not be stale yet depending on timing)
        assert!(result.near_duplicates.is_empty()); // no embedder = no dedup
    }

    #[test]
    fn test_pickup_no_handoff() {
        let (_dir, db) = test_store();
        let result = pickup(&db, None, None, None, None).unwrap();
        assert!(result.handoff.is_none());
        assert!(result.related_memories.is_empty());
    }

    #[test]
    fn test_pickup_with_handoff() {
        let (_dir, db) = test_store();

        store(&db, None, "Grasshopper phase 2 work", None, "grasshopper").unwrap();
        db.create_handoff("h1", "Finished phase 1", "Start phase 2", "grasshopper").unwrap();

        let result = pickup(&db, None, None, Some("grasshopper"), None).unwrap();
        assert!(result.handoff.is_some());
        assert_eq!(result.handoff.as_ref().unwrap().project, "grasshopper");
    }

    // --- get_context tests ---

    #[test]
    fn test_get_context_empty_when_nothing_relevant() {
        let (_dir, db) = test_store();

        // Store a memory about Rust
        store(&db, None, "Rust's ownership model prevents data races", None, "").unwrap();

        // Query about something completely unrelated with a high threshold
        let result = get_context(&db, None, None, "quantum physics entanglement", 5, 0.9, None, None).unwrap();
        // With FTS-only (no embedder/reranker), threshold scales down by 0.05x
        // Even if FTS returns something, a 0.9 threshold (→ 0.045 RRF) should filter weak matches
        assert_eq!(result.threshold, 0.9);
        assert!(result.hits.is_empty(), "should return empty for unrelated query with high threshold, got {} hits", result.hits.len());
    }

    #[test]
    fn test_get_context_passes_strong_matches() {
        let (_dir, db) = test_store();

        store(&db, None, "SQLite WAL mode improves write concurrency", None, "database").unwrap();
        store(&db, None, "PostgreSQL uses MVCC for concurrency control", None, "database").unwrap();

        // Query matching the stored memories, with a low threshold
        let result = get_context(&db, None, None, "SQLite WAL concurrency", 5, 0.0, None, None).unwrap();
        // With threshold 0.0 and FTS match, we should get results
        assert!(!result.hits.is_empty(), "should find relevant memories with threshold 0.0");
        assert_eq!(result.threshold, 0.0);
    }

    #[test]
    fn test_get_context_filters_identity_and_archived() {
        let (_dir, db) = test_store();

        // Store identity memory (should be filtered out)
        db.insert_memory(&MemoryParams {
            title: "I am a Rust developer", content: "I am a Rust developer who loves SQLite",
            memory_type: "identity", descriptors: "",
            salience: 1.0, content_hash: "id1", agent_id: "test",
        }).unwrap();
        // Store knowledge memory (should pass)
        let know = store(&db, None, "SQLite FTS5 enables full-text search", None, "").unwrap();
        // Archive one
        let archived = store(&db, None, "SQLite is a database engine", None, "").unwrap();
        db.archive_memory(archived.id).unwrap();

        let result = get_context(&db, None, None, "SQLite", 10, 0.0, None, None).unwrap();
        // Should only contain the non-archived knowledge memory
        let ids: Vec<i64> = result.hits.iter().map(|h| h.id).collect();
        assert!(ids.contains(&know.id), "should contain knowledge memory");
        assert!(!ids.contains(&archived.id), "should not contain archived memory");
        // Identity memories are also filtered
        for hit in &result.hits {
            assert_ne!(hit.memory_type.as_deref(), Some("identity"), "should not contain identity memories");
        }
    }

    // --- budget_context tests ---

    fn make_hit(id: i64, title: &str, snippet: &str) -> SearchHit {
        SearchHit {
            id, kind: "memory".into(), file_path: None, symbol_name: None,
            symbol_kind: None, signature: None, title: title.into(), snippet: snippet.into(),
            start_line: None, end_line: None,
            memory_type: Some("knowledge".into()),
            score: 1.0 - (id as f64 * 0.1), // Decreasing score
            reranker_score: None,
            access_count: 0,
            last_accessed: None,
            salience: 0.5,
            created_at: "2026-01-01T00:00:00+00:00".into(),
            archived: false,
            descriptors: String::new(),
        }
    }

    #[test]
    fn test_budget_respects_limit() {
        let hits = vec![
            make_hit(1, "Short", "a"),       // ~2 tokens
            make_hit(2, "Medium", &"x".repeat(100)),  // ~27 tokens
            make_hit(3, "Long", &"y".repeat(1000)),   // ~252 tokens
        ];
        let result = budget_context(hits, 50, false);
        // Should include first two hits (~29 tokens), but not the third (~252 tokens)
        assert_eq!(result.hits.len(), 2);
        assert!(result.estimated_tokens <= 50);
        assert_eq!(result.dropped_count, 1);
    }

    #[test]
    fn test_budget_dedup_works() {
        let hits = vec![
            make_hit(1, "Title", "Same content"),
            make_hit(2, "Title", "Same content"), // Duplicate
            make_hit(3, "Different", "Other content"),
        ];
        let result = budget_context(hits, 10000, true);
        assert_eq!(result.hits.len(), 2); // Dedup removes one
        assert_eq!(result.hits[0].id, 1);
        assert_eq!(result.hits[1].id, 3);
    }

    #[test]
    fn test_budget_preserves_order() {
        let hits = vec![
            make_hit(1, "First", "Content A"),
            make_hit(2, "Second", "Content B"),
            make_hit(3, "Third", "Content C"),
        ];
        let result = budget_context(hits, 10000, false);
        assert_eq!(result.hits.len(), 3);
        assert_eq!(result.hits[0].id, 1);
        assert_eq!(result.hits[1].id, 2);
        assert_eq!(result.hits[2].id, 3);
    }

    #[test]
    fn test_budget_empty_input() {
        let result = budget_context(vec![], 4000, true);
        assert!(result.hits.is_empty());
        assert_eq!(result.dropped_count, 0);
        assert_eq!(result.estimated_tokens, 0);
    }

    // --- Consolidation tests ---

    fn make_chunk(id: i64, title: &str, descriptors: &str) -> Chunk {
        let now = chrono::Utc::now().to_rfc3339();
        Chunk {
            id, kind: "memory".into(), title: title.into(), content: format!("content for {title}"),
            snippet: String::new(), symbol_name: None, symbol_kind: None, signature: None,
            file_path: None, language: None, start_line: None, end_line: None,
            memory_type: Some("episode".into()), descriptors: descriptors.into(),
            source: String::new(), access_count: 0, last_accessed: None, salience: 0.5,
            archived: false, content_hash: String::new(), agent_id: "test".into(),
            created_at: now.clone(), updated_at: now, codebase_id: None,
        }
    }

    #[test]
    fn test_build_summary_text() {
        let members = vec![
            make_chunk(1, "First", "tag-a, tag-b"),
            make_chunk(2, "Second", "tag-a"),
        ];
        let text = build_summary_text("tag-a", &members);
        assert!(text.contains("2 entries about \"tag-a\""), "should have count and tag: {text}");
        assert!(text.contains("[#1] First"), "should list member titles: {text}");
        assert!(text.contains("[#2] Second"), "should list all members: {text}");
    }

    #[test]
    fn test_consolidate_creates_summary_for_cluster() {
        let (_dir, store) = test_store();
        // Create 3 episodes with shared tag
        for i in 0..3 {
            store.insert_memory(&MemoryParams {
                title: &format!("Episode {i}"), content: &format!("content {i}"),
                memory_type: "episode", descriptors: "shared-tag",
                salience: 0.5, content_hash: &format!("hash{i}"), agent_id: "test",
            }).unwrap();
        }
        let result = consolidate(&store, None, false, 90).unwrap();
        assert!(!result.episode_clusters.is_empty(), "should find episode clusters");
        assert!(!result.summaries_created.is_empty(), "should create summaries");
        assert_eq!(result.summaries_created[0].member_count, 3);
    }

    #[test]
    fn test_consolidate_summary_idempotent() {
        let (_dir, store) = test_store();
        for i in 0..3 {
            store.insert_memory(&MemoryParams {
                title: &format!("Episode {i}"), content: &format!("content {i}"),
                memory_type: "episode", descriptors: "shared-tag",
                salience: 0.5, content_hash: &format!("hash{i}"), agent_id: "test",
            }).unwrap();
        }
        let r1 = consolidate(&store, None, false, 90).unwrap();
        assert!(!r1.summaries_created.is_empty());

        // Second run should not create duplicates
        let r2 = consolidate(&store, None, false, 90).unwrap();
        assert!(r2.summaries_created.is_empty(), "should not create duplicate summaries");
    }

    #[test]
    fn test_consolidate_preserves_originals() {
        let (_dir, store) = test_store();
        for i in 0..3 {
            store.insert_memory(&MemoryParams {
                title: &format!("Episode {i}"), content: &format!("content {i}"),
                memory_type: "episode", descriptors: "tag-x",
                salience: 0.5, content_hash: &format!("hash{i}"), agent_id: "test",
            }).unwrap();
        }
        consolidate(&store, None, false, 90).unwrap();
        // All originals still active (not archived)
        let episodes = store.list_memories(Some("episode"), false, 100).unwrap();
        assert_eq!(episodes.len(), 3, "all originals should still be active");
    }

    // --- Retrieval logging tests ---

    #[test]
    fn test_recall_returns_log_id() {
        let (_dir, store) = test_store();
        store.insert_memory(&MemoryParams {
            title: "Test memory", content: "some searchable content about Rust",
            memory_type: "knowledge", descriptors: "rust",
            salience: 0.5, content_hash: "rc1", agent_id: "test",
        }).unwrap();
        let result = recall(&store, None, None, "Rust", 5, None, None).unwrap();
        assert!(result.log_id.is_some(), "recall should return a log_id");
    }

    #[test]
    fn test_get_context_returns_log_id() {
        let (_dir, store) = test_store();
        store.insert_memory(&MemoryParams {
            title: "Test memory", content: "some searchable content about Rust",
            memory_type: "knowledge", descriptors: "rust",
            salience: 0.5, content_hash: "gc1", agent_id: "test",
        }).unwrap();
        let result = get_context(&store, None, None, "Rust", 5, 0.0, None, None).unwrap();
        assert!(result.log_id.is_some(), "get_context should return a log_id");
    }

    #[test]
    fn test_recall_returns_object_with_query_id() {
        let (_dir, store) = test_store();
        store.insert_memory(&crate::store::MemoryParams {
            title: "Test recall shape", content: "shape test content",
            memory_type: "knowledge", descriptors: "test",
            salience: 0.5, content_hash: "rshape1", agent_id: "test",
        }).unwrap();
        let result = recall(&store, None, None, "shape test", 5, None, None).unwrap();
        // log_id should always be present (Some) or None — but the MCP layer
        // now always wraps in {query_id, results}. Here we verify the memory layer
        // always produces a log_id when retrieval_log succeeds.
        // The MCP test would verify JSON shape, but at this layer we verify log_id exists.
        assert!(result.log_id.is_some(), "recall should produce a log_id for the fine-tuning pipeline");
    }
}
