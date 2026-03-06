use std::collections::HashMap;

/// Parameters for inserting a new memory entry.
pub struct MemoryParams<'a> {
    pub title: &'a str,
    pub content: &'a str,
    pub memory_type: &'a str,
    pub descriptors: &'a str,
    pub salience: f64,
    pub content_hash: &'a str,
    pub agent_id: &'a str,
}

/// Parameters for a single code chunk, converted from crate::code::chunk::ParsedChunk.
pub struct CodeChunkParams {
    pub chunk_key: String,
    pub file_path: String,
    pub language: String,
    pub symbol_kind: String,
    pub symbol_name: String,
    pub signature: String,
    pub snippet: String,
    pub start_line: i64,
    pub end_line: i64,
    pub file_hash: String,
}

/// Batch of chunks for a single file, used during indexing.
pub struct FileChunks {
    pub file_path: String,
    pub file_hash: String,
    pub chunks: Vec<CodeChunkParams>,
}

/// A chunk that needs (re-)embedding.
pub struct StaleChunk {
    pub id: i64,
    pub file_path: String,
    pub language: String,
    pub symbol_kind: String,
    pub symbol_name: String,
    pub signature: String,
    pub snippet: String,
}

/// Search result from FTS, vector, or hybrid search.
#[derive(Debug, Clone, serde::Serialize)]
pub struct SearchHit {
    pub id: i64,
    pub kind: String,
    pub file_path: Option<String>,
    pub symbol_name: Option<String>,
    pub symbol_kind: Option<String>,
    pub signature: Option<String>,
    pub title: String,
    pub snippet: String,
    pub start_line: Option<i64>,
    pub end_line: Option<i64>,
    pub memory_type: Option<String>,
    pub score: f64,
    /// Raw cross-encoder reranker score (set by rerank_hits, None if not reranked)
    pub reranker_score: Option<f32>,
    // Cognitive fields (populated from chunks table)
    pub access_count: i64,
    pub last_accessed: Option<String>,
    pub salience: f64,
    pub created_at: String,
    pub archived: bool,
    pub descriptors: String,
}

/// A code graph edge.
#[derive(Debug, serde::Serialize)]
pub struct GraphEdge {
    pub file_path: String,
    pub symbol: String,
    pub role: String,
    pub kind: String,
    pub line: i64,
}

/// Result of database maintenance operation.
#[derive(Debug, serde::Serialize)]
pub struct MaintenanceReport {
    pub retrieval_logs_pruned: usize,
    pub wal_pages_before: i64,
    pub wal_pages_after: i64,
}

#[derive(Debug, Clone, serde::Serialize)]
pub struct Chunk {
    pub id: i64,
    pub kind: String,
    pub title: String,
    pub content: String,
    pub snippet: String,
    pub symbol_name: Option<String>,
    pub symbol_kind: Option<String>,
    pub signature: Option<String>,
    pub file_path: Option<String>,
    pub language: Option<String>,
    pub start_line: Option<i64>,
    pub end_line: Option<i64>,
    pub memory_type: Option<String>,
    pub descriptors: String,
    pub source: String,
    pub access_count: i64,
    pub last_accessed: Option<String>,
    pub salience: f64,
    pub archived: bool,
    pub content_hash: String,
    pub agent_id: String,
    pub created_at: String,
    pub updated_at: String,
    pub codebase_id: Option<i64>,
}

#[derive(Debug, Clone, serde::Serialize)]
pub struct ImpactHit {
    pub file_path: String,
    pub via_symbol: String,
    pub depth: usize,
}

// --- Row mappers ---

pub(crate) fn row_to_chunk(row: &rusqlite::Row) -> rusqlite::Result<Chunk> {
    Ok(Chunk {
        id: row.get(0)?,
        kind: row.get(1)?,
        title: row.get(2)?,
        content: row.get(3)?,
        snippet: row.get(4)?,
        symbol_name: row.get(5)?,
        symbol_kind: row.get(6)?,
        signature: row.get(7)?,
        file_path: row.get(8)?,
        language: row.get(9)?,
        start_line: row.get(10)?,
        end_line: row.get(11)?,
        memory_type: row.get(12)?,
        descriptors: row.get(13)?,
        source: row.get(14)?,
        access_count: row.get(15)?,
        last_accessed: row.get(16)?,
        salience: row.get(17)?,
        archived: row.get(18)?,
        content_hash: row.get(19)?,
        agent_id: row.get(20)?,
        created_at: row.get(21)?,
        updated_at: row.get(22)?,
        codebase_id: row.get(23)?,
    })
}

pub(crate) fn row_to_graph_edge(row: &rusqlite::Row) -> rusqlite::Result<GraphEdge> {
    Ok(GraphEdge {
        file_path: row.get(0)?,
        symbol: row.get(1)?,
        role: row.get(2)?,
        kind: row.get(3)?,
        line: row.get(4)?,
    })
}

// --- Utility functions ---

pub(crate) fn now_millis() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as i64
}

pub(crate) fn embedding_to_blob(embedding: &[f32]) -> Vec<u8> {
    embedding.iter().flat_map(|f| f.to_le_bytes()).collect()
}

pub(crate) fn blob_to_embedding(blob: &[u8]) -> Option<&[f32]> {
    bytemuck::try_cast_slice(blob).ok()
}

pub(crate) fn dot_product(a: &[f32], b: &[f32]) -> f32 {
    a.iter().zip(b).map(|(x, y)| x * y).sum()
}

/// Hybrid merge of FTS and vector results via Reciprocal Rank Fusion.
pub fn hybrid_search(fts: &[SearchHit], vec: &[SearchHit], limit: usize) -> Vec<SearchHit> {
    const K: f64 = 60.0;
    const FTS_WEIGHT: f64 = 0.4;
    const VEC_WEIGHT: f64 = 0.6;

    let mut scores: HashMap<i64, (f64, SearchHit)> = HashMap::new();

    for (rank, hit) in fts.iter().enumerate() {
        let rrf = FTS_WEIGHT / (K + rank as f64 + 1.0);
        scores
            .entry(hit.id)
            .and_modify(|(s, _)| *s += rrf)
            .or_insert_with(|| (rrf, hit.clone()));
    }

    for (rank, hit) in vec.iter().enumerate() {
        let rrf = VEC_WEIGHT / (K + rank as f64 + 1.0);
        scores
            .entry(hit.id)
            .and_modify(|(s, _)| *s += rrf)
            .or_insert_with(|| (rrf, hit.clone()));
    }

    let mut results: Vec<SearchHit> = scores
        .into_values()
        .map(|(score, mut hit)| {
            hit.score = score;
            hit
        })
        .collect();

    results.sort_by(|a, b| b.score.partial_cmp(&a.score).unwrap_or(std::cmp::Ordering::Equal));
    results.truncate(limit);
    results
}
