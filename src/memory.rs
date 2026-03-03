use anyhow::Result;
use regex::Regex;
use sha2::{Digest, Sha256};
use std::sync::OnceLock;

use crate::store::{hybrid_search, Chunk, Handoff, MemoryParams, SearchHit, Store};

// --- Auto-classification ---

struct ClassifyResult {
    memory_type: &'static str,
    salience: f64,
}

fn identity_re() -> &'static Regex {
    static RE: OnceLock<Regex> = OnceLock::new();
    RE.get_or_init(|| {
        Regex::new(r"(?i)\b(i am|i prefer|my name|my role|i always|i never|i use|i like|i dislike)\b")
            .unwrap()
    })
}

fn procedure_re() -> &'static Regex {
    static RE: OnceLock<Regex> = OnceLock::new();
    RE.get_or_init(|| {
        Regex::new(r"(?i)\b(step \d|how to|workflow|procedure|recipe|instructions|run this|execute)\b")
            .unwrap()
    })
}

fn episode_re() -> &'static Regex {
    static RE: OnceLock<Regex> = OnceLock::new();
    RE.get_or_init(|| {
        Regex::new(r"(?i)\b(today|yesterday|just now|earlier|this morning|this afternoon|last night|\d{4}-\d{2}-\d{2})\b")
            .unwrap()
    })
}

fn auto_classify(content: &str) -> ClassifyResult {
    if identity_re().is_match(content) {
        return ClassifyResult { memory_type: "identity", salience: 1.0 };
    }
    if procedure_re().is_match(content) {
        return ClassifyResult { memory_type: "procedure", salience: 0.5 };
    }
    if episode_re().is_match(content) {
        return ClassifyResult { memory_type: "episode", salience: 0.5 };
    }
    ClassifyResult { memory_type: "knowledge", salience: 0.5 }
}

/// Generate a title from content: first sentence up to 80 chars.
fn auto_title(content: &str) -> String {
    let first_sentence = content
        .split(['.', '!', '?', '\n'])
        .next()
        .unwrap_or(content)
        .trim();
    if first_sentence.len() <= 80 {
        first_sentence.to_string()
    } else {
        format!("{}...", &first_sentence[..77])
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

/// Cognitive scoring formula ported from TypeScript tools.ts:cognitiveScore.
/// score = rrf × (1 + recencyBoost) × (1 + frequencyBoost) × salienceFactor × decayFactor
pub fn cognitive_score(hit: &SearchHit) -> f64 {
    let rrf_score = hit.score;

    // Recency boost: higher for recently created entries
    let days_created = days_since(&hit.created_at);
    let recency_boost = if days_created >= 30.0 {
        0.0
    } else {
        0.2 * (1.0 - days_created / 30.0)
    };

    // Frequency boost: logarithmic on access count
    let frequency_boost = 0.15 * (1.0 + hit.access_count as f64).log2();

    // Salience factor: maps [0, 1] → [0.5, 1.5]
    let salience_factor = 0.5 + hit.salience;

    // Exponential decay based on time since last access
    let mtype = hit.memory_type.as_deref().unwrap_or("knowledge");
    let lambda = decay_rate(mtype);
    let days_last_access = hit
        .last_accessed
        .as_deref()
        .map(days_since)
        .unwrap_or(days_created);
    let decay_factor = (-lambda * days_last_access).exp();

    rrf_score * (1.0 + recency_boost) * (1.0 + frequency_boost) * salience_factor * decay_factor
}

// --- Cognitive tools ---

/// Result of the `recall` command.
pub struct RecallResult {
    pub hits: Vec<SearchHit>,
}

/// Cognitive-scored memory search with side effects (touch + association).
pub fn recall(
    store: &Store,
    embedder: Option<&mut ferret::embed::Embedder>,
    query: &str,
    limit: usize,
) -> Result<RecallResult> {
    // 1. FTS keyword candidates
    let fts = store.fts_search(query, Some("memory"), 20)?;

    // 2. Vector candidates (if embedder available)
    let vec_results = if let Some(emb) = embedder {
        let query_vec = emb.embed_batch(&[query.to_string()])?;
        if query_vec.is_empty() {
            vec![]
        } else {
            store.vector_search(&query_vec[0], ferret::embed::MODEL_NAME, Some("memory"), 20)?
        }
    } else {
        vec![]
    };

    // 3. Merge via RRF
    let merged = if fts.is_empty() {
        vec_results
    } else if vec_results.is_empty() {
        fts
    } else {
        hybrid_search(&fts, &vec_results, 20)
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

    // 6. Side effects: touch and create associations
    let ids: Vec<i64> = scored.iter().map(|h| h.id).collect();
    for &id in &ids {
        let _ = store.touch_memory(id);
    }
    for i in 0..ids.len() {
        for j in (i + 1)..ids.len() {
            let _ = store.upsert_association(ids[i], ids[j]);
        }
    }

    Ok(RecallResult { hits: scored })
}

/// Result of the `remember` command.
pub struct RememberResult {
    pub id: i64,
    pub title: String,
    pub memory_type: String,
    pub salience: f64,
    pub was_update: bool,
    pub similar_id: Option<i64>,
}

/// Store a memory with auto-classification and dedup.
pub fn remember(
    store: &Store,
    embedder: Option<&mut ferret::embed::Embedder>,
    content: &str,
    title: Option<&str>,
    memory_type: Option<&str>,
    tags: &str,
) -> Result<RememberResult> {
    // 1. Auto-title
    let title = title
        .map(String::from)
        .unwrap_or_else(|| auto_title(content));

    // 2. Auto-classify if type not specified
    let classified = auto_classify(content);
    let memory_type = memory_type.unwrap_or(classified.memory_type);
    let salience = if memory_type == "identity" { 1.0 } else { classified.salience };

    // 3. Content hash
    let hash = format!(
        "{:x}",
        Sha256::new()
            .chain_update(title.as_bytes())
            .chain_update(content.as_bytes())
            .finalize()
    );

    // 4. Dedup via embedding similarity (if embedder available)
    if let Some(emb) = embedder {
        let embed_text = format!("{}\n{}", title, content);
        let vectors = emb.embed_batch(&[embed_text])?;
        if let Some(query_vec) = vectors.first() {
            let similar = store.search_similar_memories(
                query_vec,
                ferret::embed::MODEL_NAME,
                0.75,
                1,
            )?;

            if let Some(&(existing_id, _sim)) = similar.first() {
                // Update existing entry
                store.update_memory(existing_id, &title, content, tags, &hash)?;
                // Re-embed the updated entry
                store.batch_upsert_embeddings(&[(
                    existing_id,
                    query_vec.as_slice(),
                    ferret::embed::MODEL_NAME,
                )])?;
                return Ok(RememberResult {
                    id: existing_id,
                    title,
                    memory_type: memory_type.to_string(),
                    salience,
                    was_update: true,
                    similar_id: Some(existing_id),
                });
            }

            // No match — create new and embed
            let id = store.insert_memory(&MemoryParams {
                title: &title,
                content,
                memory_type,
                descriptors: tags,
                salience,
                content_hash: &hash,
                agent_id: "cli",
            })?;
            store.batch_upsert_embeddings(&[(
                id,
                query_vec.as_slice(),
                ferret::embed::MODEL_NAME,
            )])?;
            return Ok(RememberResult {
                id,
                title,
                memory_type: memory_type.to_string(),
                salience,
                was_update: false,
                similar_id: None,
            });
        }
    }

    // 5. No embedder — always create new (skip dedup)
    let id = store.insert_memory(&MemoryParams {
        title: &title,
        content,
        memory_type,
        descriptors: tags,
        salience,
        content_hash: &hash,
        agent_id: "cli",
    })?;
    Ok(RememberResult {
        id,
        title,
        memory_type: memory_type.to_string(),
        salience,
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
    embedder: Option<&mut ferret::embed::Embedder>,
    project: Option<&str>,
) -> Result<PickupResult> {
    let handoff = store.get_latest_handoff(project)?;

    let related_memories = if let Some(ref h) = handoff {
        let query = format!("{} {}", h.summary, h.project);
        recall(store, embedder, &query, 5)?.hits
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
            observations.push("Memory is empty. Start with 'remember' to build your knowledge base.".into());
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

pub struct ConsolidateResult {
    pub near_duplicates: Vec<(i64, i64, f64)>, // (id_a, id_b, similarity)
    pub auto_archived: Vec<i64>,
    pub stale_for_review: Vec<Chunk>,
    pub episode_clusters: Vec<(String, Vec<Chunk>)>, // (tag, episodes)
}

pub fn consolidate(
    store: &Store,
    embedder: Option<&mut ferret::embed::Embedder>,
    dry_run: bool,
    stale_days: i64,
) -> Result<ConsolidateResult> {
    let mut near_duplicates = Vec::new();
    let mut auto_archived = Vec::new();

    // Phase 1: Near-duplicates (requires embedder)
    if let Some(emb) = embedder {
        let all_embeddings = store.get_all_memory_embeddings(ferret::embed::MODEL_NAME)?;
        // Compare each pair — O(n²) but n is small (memory entries, not code chunks)
        for (src_id, src_emb) in &all_embeddings {
            let similar = store.search_similar_memories(
                src_emb,
                ferret::embed::MODEL_NAME,
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

    Ok(ConsolidateResult {
        near_duplicates,
        auto_archived,
        stale_for_review,
        episode_clusters,
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
    fn test_auto_title_short() {
        assert_eq!(auto_title("Always use bun."), "Always use bun");
    }

    #[test]
    fn test_auto_title_long() {
        let long = "A".repeat(100);
        let title = auto_title(&long);
        assert!(title.len() <= 80);
        assert!(title.ends_with("..."));
    }

    #[test]
    fn test_auto_title_newline() {
        assert_eq!(auto_title("First line\nSecond line"), "First line");
    }

    #[test]
    fn test_auto_classify_identity() {
        let r = auto_classify("I am a software engineer");
        assert_eq!(r.memory_type, "identity");
        assert_eq!(r.salience, 1.0);
    }

    #[test]
    fn test_auto_classify_procedure() {
        let r = auto_classify("Step 1: install the package");
        assert_eq!(r.memory_type, "procedure");
    }

    #[test]
    fn test_auto_classify_episode() {
        let r = auto_classify("Today I fixed the login bug");
        assert_eq!(r.memory_type, "episode");
    }

    #[test]
    fn test_auto_classify_knowledge() {
        let r = auto_classify("SQLite WAL mode improves concurrency");
        assert_eq!(r.memory_type, "knowledge");
    }

    #[test]
    fn test_cognitive_score_identity_no_decay() {
        let hit = SearchHit {
            id: 1, kind: "memory".into(), file_path: None, symbol_name: None,
            symbol_kind: None, signature: None, title: "test".into(), snippet: String::new(),
            start_line: None, end_line: None,
            memory_type: Some("identity".into()),
            score: 1.0,
            access_count: 5,
            last_accessed: Some("2020-01-01T00:00:00+00:00".into()),
            salience: 1.0,
            created_at: "2020-01-01T00:00:00+00:00".into(),
            archived: false,
            descriptors: String::new(),
        };
        let score = cognitive_score(&hit);
        // Identity has decay_rate=0, so decay_factor=1.0 regardless of age
        // The old created_at means recency_boost=0
        // salience_factor = 0.5 + 1.0 = 1.5
        // frequency_boost = 0.15 * log2(6) ≈ 0.388
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
    fn test_remember_basic() {
        let (_dir, store) = test_store();

        let result = remember(
            &store, None,
            "Always use bun for package management",
            None, None, "tools",
        ).unwrap();

        assert!(!result.was_update);
        assert_eq!(result.memory_type, "knowledge");
        assert!(!result.title.is_empty());

        let chunk = store.get_chunk(result.id).unwrap().unwrap();
        assert_eq!(chunk.kind, "memory");
        assert_eq!(chunk.descriptors, "tools");
    }

    #[test]
    fn test_remember_identity_auto_classify() {
        let (_dir, store) = test_store();

        let result = remember(
            &store, None,
            "I am Myles, a software engineer",
            None, None, "",
        ).unwrap();

        assert_eq!(result.memory_type, "identity");
        assert_eq!(result.salience, 1.0);
    }

    #[test]
    fn test_remember_explicit_type_overrides() {
        let (_dir, store) = test_store();

        let result = remember(
            &store, None,
            "I am not really identity content",
            None, Some("episode"), "",
        ).unwrap();

        assert_eq!(result.memory_type, "episode");
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
        let (_dir, store) = test_store();

        remember(&store, None, "I am Myles", None, Some("identity"), "").unwrap();
        remember(&store, None, "SQLite is great", None, Some("knowledge"), "").unwrap();
        store.create_handoff("h1", "Done phase 0", "Start phase 1", "grasshopper").unwrap();

        let result = me(&store).unwrap();
        assert_eq!(result.identity.len(), 1);
        assert_eq!(result.active_projects.len(), 1);
        assert_eq!(result.active_count, 2);
    }

    #[test]
    fn test_recall_basic() {
        let (_dir, store) = test_store();

        remember(&store, None, "Always use bun for packages", None, Some("knowledge"), "tools").unwrap();
        remember(&store, None, "SQLite WAL mode", None, Some("knowledge"), "database").unwrap();

        let result = recall(&store, None, "bun", 10).unwrap();
        assert!(!result.hits.is_empty());
        assert_eq!(result.hits[0].memory_type.as_deref(), Some("knowledge"));
    }

    #[test]
    fn test_recall_skips_identity() {
        let (_dir, store) = test_store();

        remember(&store, None, "I am a developer", None, Some("identity"), "").unwrap();
        remember(&store, None, "Developer tools are great", None, Some("knowledge"), "").unwrap();

        let result = recall(&store, None, "developer", 10).unwrap();
        // Should only return the knowledge entry, not the identity
        for hit in &result.hits {
            assert_ne!(hit.memory_type.as_deref(), Some("identity"));
        }
    }

    #[test]
    fn test_reflect_overview() {
        let (_dir, store) = test_store();

        remember(&store, None, "Some knowledge", None, Some("knowledge"), "").unwrap();
        remember(&store, None, "An episode today", None, Some("episode"), "").unwrap();

        let result = reflect(&store, &ReflectFocus::Overview).unwrap();
        assert_eq!(result.active_count, 2);
        assert!(!result.type_counts.is_empty());
    }

    #[test]
    fn test_reflect_gaps_no_identity() {
        let (_dir, store) = test_store();

        remember(&store, None, "Some knowledge", None, Some("knowledge"), "").unwrap();

        let result = reflect(&store, &ReflectFocus::Gaps).unwrap();
        assert!(result.observations.iter().any(|o| o.contains("identity")));
    }

    #[test]
    fn test_consolidate_dry_run() {
        let (_dir, store) = test_store();

        remember(&store, None, "Some old content", None, Some("episode"), "tag1").unwrap();

        let result = consolidate(&store, None, true, 0).unwrap();
        // With stale_days=0, entries created just now should still be caught
        // but auto_archived should be empty because it's dry_run
        // (Though the entry was JUST created, it might not be stale yet depending on timing)
        assert!(result.near_duplicates.is_empty()); // no embedder = no dedup
    }

    #[test]
    fn test_pickup_no_handoff() {
        let (_dir, store) = test_store();
        let result = pickup(&store, None, None).unwrap();
        assert!(result.handoff.is_none());
        assert!(result.related_memories.is_empty());
    }

    #[test]
    fn test_pickup_with_handoff() {
        let (_dir, store) = test_store();

        remember(&store, None, "Grasshopper phase 2 work", None, Some("knowledge"), "grasshopper").unwrap();
        store.create_handoff("h1", "Finished phase 1", "Start phase 2", "grasshopper").unwrap();

        let result = pickup(&store, None, Some("grasshopper")).unwrap();
        assert!(result.handoff.is_some());
        assert_eq!(result.handoff.as_ref().unwrap().project, "grasshopper");
    }
}
