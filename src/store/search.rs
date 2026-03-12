use anyhow::Result;
use rusqlite::params;

use super::schema::Store;
use super::types::*;

/// Safety limit for brute-force vector search fallback.
/// Above this threshold, skip brute-force to avoid loading ~300MB+ of embeddings into memory.
/// The HNSW path handles large datasets correctly — this guards only the fallback.
const MAX_BRUTE_FORCE_CHUNKS: usize = 10_000;

/// Minimum cosine similarity for vector search results.
/// Defaults to 0.3, but code search uses a stricter floor to suppress
/// vector-only false positives on out-of-domain queries.
const MIN_VECTOR_SIMILARITY_DEFAULT: f64 = 0.3;
const MIN_VECTOR_SIMILARITY_CODE: f64 = 0.48;

#[inline]
fn min_vector_similarity(kind_filter: Option<&str>) -> f64 {
    if kind_filter == Some("code") {
        MIN_VECTOR_SIMILARITY_CODE
    } else {
        MIN_VECTOR_SIMILARITY_DEFAULT
    }
}

impl Store {
    /// Full-text search across code and/or memory chunks.
    pub fn fts_search(
        &self,
        query: &str,
        kind_filter: Option<&str>,
        limit: usize,
    ) -> Result<Vec<SearchHit>> {
        let prepared = crate::code::tokenizer::prepare_fts_query(query);
        if prepared.is_empty() {
            return Ok(vec![]);
        }

        let kind_clause = match kind_filter {
            Some("code") => "AND c.kind = 'code'",
            Some("memory") => "AND c.kind = 'memory'",
            _ => "",
        };

        let sql = format!(
            "SELECT c.id, c.kind, c.file_path, c.symbol_name, c.symbol_kind, c.signature,
                    CASE WHEN c.kind = 'memory' THEN COALESCE(NULLIF(c.snippet, ''), c.content, '')
                         ELSE COALESCE(c.snippet, '') END AS snippet,
                    c.start_line, c.end_line, c.title, c.memory_type,
                    bm25(chunks_fts, 5.0, 1.0, 1.0, 5.0, 2.0) AS score,
                    c.access_count, c.last_accessed, c.salience, c.created_at, c.archived,
                    c.descriptors
             FROM chunks_fts f
             JOIN chunks c ON c.id = f.rowid
             WHERE chunks_fts MATCH ?1 {kind_clause}
             ORDER BY score ASC
             LIMIT ?2"
        );

        let mut stmt = self.conn.prepare(&sql)?;
        let hits = stmt
            .query_map(params![prepared, limit as i64], |row| {
                Ok(SearchHit {
                    id: row.get(0)?,
                    kind: row.get(1)?,
                    file_path: row.get(2)?,
                    symbol_name: row.get(3)?,
                    symbol_kind: row.get(4)?,
                    signature: row.get(5)?,
                    snippet: row.get(6)?,
                    start_line: row.get(7)?,
                    end_line: row.get(8)?,
                    title: row.get(9)?,
                    memory_type: row.get(10)?,
                    score: {
                        let raw: f64 = row.get(11)?;
                        -raw
                    },
                    reranker_score: None,
                    access_count: row.get(12)?,
                    last_accessed: row.get(13)?,
                    salience: row.get(14)?,
                    created_at: row.get(15)?,
                    archived: row.get(16)?,
                    descriptors: row.get::<_, Option<String>>(17)?.unwrap_or_default(),
                })
            })?
            .filter_map(log_and_skip("fts_search"))
            .collect();
        Ok(hits)
    }

    /// Brute-force vector search across all chunks with embeddings.
    /// Guarded by MAX_BRUTE_FORCE_CHUNKS to prevent OOM on large datasets.
    pub fn vector_search(
        &self,
        query_embedding: &[f32],
        model_name: &str,
        kind_filter: Option<&str>,
        limit: usize,
    ) -> Result<Vec<SearchHit>> {
        let min_similarity = min_vector_similarity(kind_filter);
        let total: i64 = self.count_embedded_filtered(kind_filter)?;
        if total as usize > MAX_BRUTE_FORCE_CHUNKS {
            tracing::warn!(
                "brute-force vector search skipped: {total} embedded chunks (kind={}) exceeds \
                 limit of {MAX_BRUTE_FORCE_CHUNKS}. Rebuild HNSW index to enable vector search.",
                kind_filter.unwrap_or("all")
            );
            return Ok(vec![]);
        }

        let kind_clause = match kind_filter {
            Some("code") => "AND kind = 'code'",
            Some("memory") => "AND kind = 'memory'",
            _ => "",
        };

        let sql = format!(
            "SELECT id, kind, file_path, symbol_name, symbol_kind, signature,
                    CASE WHEN kind = 'memory' THEN COALESCE(NULLIF(snippet, ''), content, '')
                         ELSE COALESCE(snippet, '') END AS snippet,
                    start_line, end_line, title, memory_type, embedding,
                    access_count, last_accessed, salience, created_at, archived, descriptors
             FROM chunks
             WHERE embedding IS NOT NULL AND embedding_model = ?1 {kind_clause}"
        );

        let mut stmt = self.conn.prepare(&sql)?;
        let mut scored: Vec<SearchHit> = Vec::new();
        let mut rows = stmt.query(params![model_name])?;
        while let Some(row) = rows.next()? {
            let blob: Vec<u8> = row.get(11)?;
            let Some(emb) = blob_to_embedding(&blob) else {
                continue;
            };
            let score = dot_product(query_embedding, emb) as f64;
            if score < min_similarity {
                continue;
            }
            scored.push(SearchHit {
                id: row.get(0)?,
                kind: row.get(1)?,
                file_path: row.get(2)?,
                symbol_name: row.get(3)?,
                symbol_kind: row.get(4)?,
                signature: row.get(5)?,
                snippet: row.get(6)?,
                start_line: row.get(7)?,
                end_line: row.get(8)?,
                title: row.get(9)?,
                memory_type: row.get(10)?,
                score,
                reranker_score: None,
                access_count: row.get(12)?,
                last_accessed: row.get(13)?,
                salience: row.get(14)?,
                created_at: row.get(15)?,
                archived: row.get(16)?,
                descriptors: row.get::<_, Option<String>>(17)?.unwrap_or_default(),
            });
        }

        scored.sort_by(|a, b| {
            b.score
                .partial_cmp(&a.score)
                .unwrap_or(std::cmp::Ordering::Equal)
        });
        scored.truncate(limit);
        Ok(scored)
    }

    /// Get all embeddings as (chunk_id, embedding_vector) pairs for HNSW index building.
    pub fn get_all_embeddings(&self) -> Result<Vec<(i64, Vec<f32>)>> {
        let mut stmt = self
            .conn
            .prepare("SELECT id, embedding FROM chunks WHERE embedding IS NOT NULL")?;
        let mut results = Vec::new();
        let mut dropped = 0u64;
        let raw_rows = stmt.query_map([], |row| {
            let id: i64 = row.get(0)?;
            let blob: Vec<u8> = row.get(1)?;
            Ok((id, blob))
        })?;
        for row in raw_rows {
            let Ok((id, blob)) = row else {
                dropped += 1;
                continue;
            };
            let Some(emb) = blob_to_embedding(&blob) else {
                dropped += 1;
                continue;
            };
            results.push((id, emb.to_vec()));
        }
        if dropped > 0 {
            tracing::warn!("get_all_embeddings: dropped {dropped} rows with decode errors");
        }
        Ok(results)
    }

    /// Vector search using HNSW index for O(log N) approximate nearest neighbors.
    /// Falls back to brute-force if HNSW returns no results.
    pub fn vector_search_hnsw(
        &self,
        hnsw: &crate::code::hnsw::HnswIndex,
        query_embedding: &[f32],
        kind_filter: Option<&str>,
        limit: usize,
    ) -> Result<Vec<SearchHit>> {
        let min_similarity = min_vector_similarity(kind_filter);
        if hnsw.is_empty() {
            return self.vector_search(
                query_embedding,
                crate::code::embed::MODEL_NAME,
                kind_filter,
                limit,
            );
        }

        // Retrieve 2x candidates for re-scoring (HNSW is approximate)
        let candidates = hnsw.search(query_embedding, limit * 2);
        if candidates.is_empty() {
            return self.vector_search(
                query_embedding,
                crate::code::embed::MODEL_NAME,
                kind_filter,
                limit,
            );
        }

        let kind_clause = match kind_filter {
            Some("code") => "AND kind = 'code'",
            Some("memory") => "AND kind = 'memory'",
            _ => "",
        };

        // Load full SearchHit data for HNSW candidates and rescore with exact embeddings
        let placeholders = vec!["?"; candidates.len()].join(",");
        let sql = format!(
            "SELECT id, kind, file_path, symbol_name, symbol_kind, signature,
                    CASE WHEN kind = 'memory' THEN COALESCE(NULLIF(snippet, ''), content, '')
                         ELSE COALESCE(snippet, '') END AS snippet,
                    start_line, end_line, title, memory_type, embedding,
                    access_count, last_accessed, salience, created_at, archived, descriptors
             FROM chunks
             WHERE id IN ({placeholders}) {kind_clause}"
        );

        let mut stmt = self.conn.prepare(&sql)?;
        let params: Vec<Box<dyn rusqlite::types::ToSql>> = candidates
            .iter()
            .map(|(id, _)| Box::new(*id) as Box<dyn rusqlite::types::ToSql>)
            .collect();
        let param_refs: Vec<&dyn rusqlite::types::ToSql> =
            params.iter().map(|p| p.as_ref()).collect();

        let mut scored: Vec<SearchHit> = Vec::new();
        let mut rows = stmt.query(param_refs.as_slice())?;
        while let Some(row) = rows.next()? {
            let blob: Vec<u8> = row.get(11)?;
            let Some(emb) = blob_to_embedding(&blob) else {
                continue;
            };
            let score = dot_product(query_embedding, emb) as f64;
            if score < min_similarity {
                continue;
            }
            scored.push(SearchHit {
                id: row.get(0)?,
                kind: row.get(1)?,
                file_path: row.get(2)?,
                symbol_name: row.get(3)?,
                symbol_kind: row.get(4)?,
                signature: row.get(5)?,
                snippet: row.get(6)?,
                start_line: row.get(7)?,
                end_line: row.get(8)?,
                title: row.get(9)?,
                memory_type: row.get(10)?,
                score,
                reranker_score: None,
                access_count: row.get(12)?,
                last_accessed: row.get(13)?,
                salience: row.get(14)?,
                created_at: row.get(15)?,
                archived: row.get(16)?,
                descriptors: row.get::<_, Option<String>>(17)?.unwrap_or_default(),
            });
        }

        scored.sort_by(|a, b| {
            b.score
                .partial_cmp(&a.score)
                .unwrap_or(std::cmp::Ordering::Equal)
        });
        scored.truncate(limit);
        Ok(scored)
    }

    /// Search for similar memories by embedding cosine similarity.
    /// Returns Vec<(chunk_id, similarity_score)> above threshold.
    pub fn search_similar_memories(
        &self,
        query_embedding: &[f32],
        model_name: &str,
        threshold: f64,
        limit: usize,
    ) -> Result<Vec<(i64, f64)>> {
        let mut stmt = self.conn.prepare(
            "SELECT id, embedding FROM chunks
             WHERE kind = 'memory' AND archived = 0
               AND embedding IS NOT NULL AND embedding_model = ?1",
        )?;
        let mut scored: Vec<(i64, f64)> = Vec::new();
        let mut rows = stmt.query(params![model_name])?;
        while let Some(row) = rows.next()? {
            let id: i64 = row.get(0)?;
            let blob: Vec<u8> = row.get(1)?;
            let Some(emb) = blob_to_embedding(&blob) else {
                continue;
            };
            let sim = dot_product(query_embedding, emb) as f64;
            if sim >= threshold {
                scored.push((id, sim));
            }
        }
        scored.sort_by(|a, b| b.1.partial_cmp(&a.1).unwrap_or(std::cmp::Ordering::Equal));
        scored.truncate(limit);
        Ok(scored)
    }
}
