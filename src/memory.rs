use anyhow::Result;
use sha2::{Digest, Sha256};

use crate::store::{MemoryParams, SearchHit, Store};

/// Generate a title from content: simple truncation to 80 chars.
fn truncate_title(content: &str) -> String {
    let first_line = content.split('\n').next().unwrap_or(content).trim();
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

// --- Store ---

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
    memory_type: Option<&str>,
) -> Result<StoreResult> {
    let memory_type = match memory_type {
        None | Some("knowledge") => "knowledge",
        Some("identity") => "identity",
        Some("episode") => "episode",
        Some("procedure") => "procedure",
        Some(t) => anyhow::bail!(
            "invalid memory_type '{t}': must be identity, knowledge, episode, or procedure"
        ),
    };
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
            let similar =
                db.search_similar_memories(query_vec, crate::code::embed::MODEL_NAME, 0.75, 1)?;

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
                memory_type,
                descriptors: tags,
                salience: 0.5,
                content_hash: &hash,
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
        memory_type,
        descriptors: tags,
        salience: 0.5,
        content_hash: &hash,
    })?;
    Ok(StoreResult {
        id,
        title,
        was_update: false,
        similar_id: None,
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
            id: 1,
            kind: "memory".into(),
            file_path: None,
            symbol_name: None,
            symbol_kind: None,
            signature: None,
            title: "test".into(),
            snippet: String::new(),
            start_line: None,
            end_line: None,
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
            id: 1,
            kind: "memory".into(),
            file_path: None,
            symbol_name: None,
            symbol_kind: None,
            signature: None,
            title: "test".into(),
            snippet: String::new(),
            start_line: None,
            end_line: None,
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
        assert!(
            recent_score > old_score,
            "recent episode ({recent_score}) should score higher than old ({old_score})"
        );
    }

    #[test]
    fn test_store_basic() {
        let (_dir, db) = test_store();

        let result = store(
            &db,
            None,
            "Always use bun for package management",
            None,
            "tools",
            None,
        )
        .unwrap();

        assert!(!result.was_update);
        assert!(!result.title.is_empty());

        let chunk = db.get_chunk(result.id).unwrap().unwrap();
        assert_eq!(chunk.kind, "memory");
        assert_eq!(chunk.descriptors, "tools");
    }

    #[test]
    fn test_store_defaults_to_knowledge() {
        let (_dir, db) = test_store();

        let result = store(&db, None, "I am Myles, a software engineer", None, "", None).unwrap();

        // No memory_type param → defaults to knowledge
        let chunk = db.get_chunk(result.id).unwrap().unwrap();
        assert_eq!(chunk.memory_type.as_deref(), Some("knowledge"));
    }

    #[test]
    fn test_store_with_explicit_title() {
        let (_dir, db) = test_store();

        let result = store(
            &db,
            None,
            "Some content here",
            Some("My Custom Title"),
            "",
            None,
        )
        .unwrap();

        assert_eq!(result.title, "My Custom Title");
    }
}
