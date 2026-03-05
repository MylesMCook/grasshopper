use anyhow::{Context, Result};
use rusqlite::functions::FunctionFlags;
use rusqlite::{params, Connection, OptionalExtension};
use std::collections::{HashMap, HashSet};
use std::path::Path;

/// Unified data store for code intelligence + cognitive memory.
/// Single SQLite database with WAL mode, FTS5 keyword search,
/// and shared schema for code chunks and memory entries.
pub struct Store {
    conn: Connection,
}

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

/// Parameters for a single code chunk, converted from ferret::chunk::ParsedChunk.
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
#[derive(Debug, Clone)]
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
#[derive(Debug)]
pub struct GraphEdge {
    pub file_path: String,
    pub symbol: String,
    pub role: String,
    pub kind: String,
    pub line: i64,
}

impl Store {
    pub fn open(path: &Path) -> Result<Self> {
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent)
                .with_context(|| format!("creating directory for {}", path.display()))?;
        }

        let conn = Connection::open(path)
            .with_context(|| format!("opening database at {}", path.display()))?;

        conn.execute_batch(
            "PRAGMA journal_mode = WAL;
             PRAGMA busy_timeout = 60000;
             PRAGMA synchronous = NORMAL;
             PRAGMA cache_size = -64000;
             PRAGMA auto_vacuum = INCREMENTAL;",
        )?;

        // Register code-aware token expansion as a SQL scalar function.
        // Used in FTS5 INSERT to make camelCase/PascalCase searchable.
        conn.create_scalar_function(
            "code_expand",
            1,
            FunctionFlags::SQLITE_UTF8 | FunctionFlags::SQLITE_DETERMINISTIC,
            |ctx| {
                let text: String = ctx.get(0)?;
                Ok(ferret::tokenizer::expand_code_tokens(&text))
            },
        )
        .context("registering code_expand SQL function")?;

        let store = Self { conn };
        store.init_schema()?;
        Ok(store)
    }

    /// Execute raw SQL batch (e.g. BEGIN/COMMIT/ROLLBACK).
    pub fn execute_batch(&self, sql: &str) -> Result<()> {
        self.conn.execute_batch(sql)?;
        Ok(())
    }

    fn init_schema(&self) -> Result<()> {
        self.conn.execute_batch(
            "-- Indexed codebases (directories of source code)
            CREATE TABLE IF NOT EXISTS codebases (
                id          INTEGER PRIMARY KEY AUTOINCREMENT,
                root_path   TEXT NOT NULL UNIQUE,
                name        TEXT NOT NULL DEFAULT '',
                indexed_at  INTEGER NOT NULL DEFAULT 0
            );

            -- Unified chunks: both code and memory entries
            CREATE TABLE IF NOT EXISTS chunks (
                id              INTEGER PRIMARY KEY AUTOINCREMENT,
                kind            TEXT NOT NULL,       -- 'code' | 'memory'

                -- Code fields (NULL for memories)
                codebase_id     INTEGER REFERENCES codebases(id),
                file_path       TEXT,
                chunk_key       TEXT,                  -- dedup key for code chunks
                language        TEXT,
                symbol_kind     TEXT,                 -- 'function', 'class', 'struct', etc.
                symbol_name     TEXT DEFAULT '',
                signature       TEXT DEFAULT '',
                start_line      INTEGER,
                end_line        INTEGER,
                file_hash       TEXT,

                -- Memory fields (NULL for code)
                memory_type     TEXT,                 -- 'identity' | 'knowledge' | 'episode' | 'procedure'

                -- Shared fields
                title           TEXT DEFAULT '',
                content         TEXT NOT NULL,
                snippet         TEXT DEFAULT '',
                descriptors     TEXT DEFAULT '',      -- comma-separated tags
                source          TEXT DEFAULT '',      -- origin URL or file path

                -- Cognitive fields (used by memory, trackable for code)
                access_count    INTEGER NOT NULL DEFAULT 0,
                last_accessed   TEXT,                 -- ISO 8601 datetime
                salience        REAL NOT NULL DEFAULT 0.5,
                archived        INTEGER NOT NULL DEFAULT 0,

                -- Embedding
                embedding       BLOB,
                embedding_model TEXT DEFAULT '',
                content_hash    TEXT DEFAULT '',

                -- Metadata
                agent_id        TEXT DEFAULT '',
                created_at      TEXT NOT NULL,
                updated_at      TEXT NOT NULL,
                indexed_at      INTEGER NOT NULL DEFAULT 0
            );

            CREATE INDEX IF NOT EXISTS idx_chunks_kind
                ON chunks(kind);
            CREATE INDEX IF NOT EXISTS idx_chunks_codebase
                ON chunks(codebase_id) WHERE codebase_id IS NOT NULL;
            CREATE INDEX IF NOT EXISTS idx_chunks_file
                ON chunks(codebase_id, file_path) WHERE file_path IS NOT NULL;
            CREATE INDEX IF NOT EXISTS idx_chunks_memory_type
                ON chunks(memory_type) WHERE memory_type IS NOT NULL;
            CREATE INDEX IF NOT EXISTS idx_chunks_archived
                ON chunks(archived) WHERE kind = 'memory';
            CREATE INDEX IF NOT EXISTS idx_chunks_recall_filter
                ON chunks(kind, archived, memory_type);
            CREATE INDEX IF NOT EXISTS idx_chunks_content_hash
                ON chunks(content_hash) WHERE content_hash != '';
            CREATE UNIQUE INDEX IF NOT EXISTS idx_chunks_chunk_key
                ON chunks(codebase_id, chunk_key);

            -- File tracking for incremental code indexing
            CREATE TABLE IF NOT EXISTS indexed_files (
                id          INTEGER PRIMARY KEY AUTOINCREMENT,
                codebase_id INTEGER NOT NULL REFERENCES codebases(id),
                file_path   TEXT NOT NULL,
                file_hash   TEXT NOT NULL,
                chunk_count INTEGER NOT NULL DEFAULT 0,
                indexed_at  INTEGER NOT NULL,
                UNIQUE(codebase_id, file_path)
            );

            -- Graph: code refs + Hebbian associations
            CREATE TABLE IF NOT EXISTS graph (
                id              INTEGER PRIMARY KEY AUTOINCREMENT,
                source_chunk    INTEGER REFERENCES chunks(id),
                target_chunk    INTEGER REFERENCES chunks(id),

                -- For code edges from tree-sitter
                codebase_id     INTEGER REFERENCES codebases(id),
                file_path       TEXT,
                symbol          TEXT,
                role            TEXT NOT NULL,        -- 'definition' | 'reference' | 'associates'
                kind            TEXT DEFAULT '',
                line            INTEGER,

                -- For Hebbian associations
                strength        REAL NOT NULL DEFAULT 1.0
            );

            -- Code edges: one entry per (codebase, file, symbol, role, kind, line)
            CREATE UNIQUE INDEX IF NOT EXISTS idx_graph_code_edge
                ON graph(codebase_id, file_path, symbol, role, kind, line)
                WHERE codebase_id IS NOT NULL;
            -- Hebbian associations: one edge per (source, target, role)
            CREATE UNIQUE INDEX IF NOT EXISTS idx_graph_association
                ON graph(source_chunk, target_chunk, role)
                WHERE role = 'associates';
            -- Entity edges: one edge per (chunk, entity_value, entity_kind)
            CREATE UNIQUE INDEX IF NOT EXISTS idx_graph_entity
                ON graph(source_chunk, symbol, role, kind)
                WHERE role = 'entity';

            CREATE INDEX IF NOT EXISTS idx_graph_symbol
                ON graph(codebase_id, symbol) WHERE symbol IS NOT NULL;
            CREATE INDEX IF NOT EXISTS idx_graph_source
                ON graph(source_chunk) WHERE source_chunk IS NOT NULL;
            CREATE INDEX IF NOT EXISTS idx_graph_target
                ON graph(target_chunk) WHERE target_chunk IS NOT NULL;

            -- FTS5 keyword search across all chunks (code + memory)
            CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
                title,
                content,
                snippet,
                symbol_name,
                descriptors,
                tokenize='porter unicode61'
            );

            -- Access log for learned decay rates
            CREATE TABLE IF NOT EXISTS access_log (
                id          INTEGER PRIMARY KEY AUTOINCREMENT,
                chunk_id    INTEGER NOT NULL REFERENCES chunks(id),
                accessed_at TEXT NOT NULL
            );
            CREATE INDEX IF NOT EXISTS idx_access_log_chunk
                ON access_log(chunk_id);

            -- Retrieval logging for fine-tuning pipeline
            CREATE TABLE IF NOT EXISTS retrieval_log (
                id              INTEGER PRIMARY KEY AUTOINCREMENT,
                query           TEXT NOT NULL,
                tool            TEXT NOT NULL,
                result_ids      TEXT NOT NULL,
                scores          TEXT NOT NULL,
                result_count    INTEGER NOT NULL,
                latency_ms      INTEGER,
                created_at      TEXT NOT NULL
            );
            CREATE INDEX IF NOT EXISTS idx_retrieval_log_created
                ON retrieval_log(created_at);

            -- Feedback on retrieval results
            CREATE TABLE IF NOT EXISTS feedback (
                id                  INTEGER PRIMARY KEY AUTOINCREMENT,
                retrieval_log_id    INTEGER REFERENCES retrieval_log(id),
                chunk_id            INTEGER NOT NULL REFERENCES chunks(id),
                signal              TEXT NOT NULL,
                created_at          TEXT NOT NULL
            );

            -- Session handoffs for continuity
            CREATE TABLE IF NOT EXISTS handoffs (
                id          TEXT PRIMARY KEY,
                summary     TEXT NOT NULL,
                next_steps  TEXT DEFAULT '',
                project     TEXT DEFAULT '',
                created_at  TEXT NOT NULL
            );",
        )?;
        Ok(())
    }

    // --- Memory operations ---

    /// Insert a memory entry. Returns the new row ID.
    pub fn insert_memory(&self, p: &MemoryParams) -> Result<i64> {
        let now = chrono::Utc::now().to_rfc3339();
        self.conn.execute_batch("SAVEPOINT insert_memory")?;
        let result = (|| -> Result<i64> {
            self.conn.execute(
                "INSERT INTO chunks (kind, title, content, memory_type, descriptors, salience,
                                     content_hash, agent_id, created_at, updated_at)
                 VALUES ('memory', ?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?8)",
                params![p.title, p.content, p.memory_type, p.descriptors, p.salience, p.content_hash, p.agent_id, now],
            )?;
            let id = self.conn.last_insert_rowid();
            self.conn.execute(
                "INSERT INTO chunks_fts (rowid, title, content, snippet, symbol_name, descriptors)
                 VALUES (?1, ?2, ?3, '', '', ?4)",
                params![id, p.title, p.content, p.descriptors],
            )?;
            Ok(id)
        })();
        match result {
            Ok(id) => { self.conn.execute_batch("RELEASE insert_memory")?; Ok(id) }
            Err(e) => { let _ = self.conn.execute_batch("ROLLBACK TO insert_memory"); Err(e) }
        }
    }

    /// Get a chunk by ID.
    pub fn get_chunk(&self, id: i64) -> Result<Option<Chunk>> {
        let mut stmt = self.conn.prepare(
            "SELECT id, kind, title, content, snippet, symbol_name, symbol_kind, signature,
                    file_path, language, start_line, end_line, memory_type,
                    descriptors, source, access_count, last_accessed, salience, archived,
                    content_hash, agent_id, created_at, updated_at, codebase_id
             FROM chunks WHERE id = ?1",
        )?;

        let chunk = stmt.query_row(params![id], |row| {
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
        });

        match chunk {
            Ok(c) => Ok(Some(c)),
            Err(rusqlite::Error::QueryReturnedNoRows) => Ok(None),
            Err(e) => Err(e.into()),
        }
    }

    /// List memory entries, optionally filtered by type. Excludes archived by default.
    pub fn list_memories(
        &self,
        memory_type: Option<&str>,
        include_archived: bool,
        limit: usize,
    ) -> Result<Vec<Chunk>> {
        let select = "SELECT id, kind, title, content, snippet, symbol_name, symbol_kind, signature,
                    file_path, language, start_line, end_line, memory_type,
                    descriptors, source, access_count, last_accessed, salience, archived,
                    content_hash, agent_id, created_at, updated_at, codebase_id
             FROM chunks WHERE kind = 'memory'";

        let archived_clause = if include_archived { "" } else { " AND archived = 0" };

        if let Some(mt) = memory_type {
            let sql = format!(
                "{select}{archived_clause} AND memory_type = ?1 ORDER BY created_at DESC LIMIT ?2"
            );
            let mut stmt = self.conn.prepare(&sql)?;
            let rows = stmt
                .query_map(params![mt, limit as i64], row_to_chunk)?
                .filter_map(|r| r.ok())
                .collect();
            Ok(rows)
        } else {
            let sql = format!(
                "{select}{archived_clause} ORDER BY created_at DESC LIMIT ?1"
            );
            let mut stmt = self.conn.prepare(&sql)?;
            let rows = stmt
                .query_map(params![limit as i64], row_to_chunk)?
                .filter_map(|r| r.ok())
                .collect();
            Ok(rows)
        }
    }

    /// Update a memory entry's content and metadata.
    pub fn update_memory(&self, id: i64, p: &MemoryParams) -> Result<bool> {
        let now = chrono::Utc::now().to_rfc3339();
        self.conn.execute_batch("SAVEPOINT update_memory")?;
        let result = (|| -> Result<bool> {
            let rows = self.conn.execute(
                "UPDATE chunks SET title = ?1, content = ?2, memory_type = ?3, descriptors = ?4,
                                   salience = ?5, content_hash = ?6, updated_at = ?7
                 WHERE id = ?8 AND kind = 'memory'",
                params![p.title, p.content, p.memory_type, p.descriptors, p.salience, p.content_hash, now, id],
            )?;
            if rows > 0 {
                self.conn.execute("DELETE FROM chunks_fts WHERE rowid = ?1", params![id])?;
                self.conn.execute(
                    "INSERT INTO chunks_fts (rowid, title, content, snippet, symbol_name, descriptors)
                     VALUES (?1, ?2, ?3, '', '', ?4)",
                    params![id, p.title, p.content, p.descriptors],
                )?;
            }
            Ok(rows > 0)
        })();
        match result {
            Ok(updated) => { self.conn.execute_batch("RELEASE update_memory")?; Ok(updated) }
            Err(e) => { let _ = self.conn.execute_batch("ROLLBACK TO update_memory"); Err(e) }
        }
    }

    /// Count chunks by kind.
    pub fn count_by_kind(&self) -> Result<(i64, i64)> {
        let code: i64 = self.conn.query_row(
            "SELECT COUNT(*) FROM chunks WHERE kind = 'code'",
            [],
            |row| row.get(0),
        )?;
        let memory: i64 = self.conn.query_row(
            "SELECT COUNT(*) FROM chunks WHERE kind = 'memory'",
            [],
            |row| row.get(0),
        )?;
        Ok((code, memory))
    }

    /// Update access tracking for a memory (increment count, update timestamp, bump salience).
    pub fn touch_memory(&self, id: i64) -> Result<()> {
        let now = chrono::Utc::now().to_rfc3339();
        self.conn.execute(
            "UPDATE chunks SET
                access_count = access_count + 1,
                last_accessed = ?1,
                salience = MIN(1.0, salience + 0.05),
                updated_at = ?1
             WHERE id = ?2 AND kind = 'memory'",
            params![now, id],
        )?;
        // Record access event for learned decay rates
        self.conn.execute(
            "INSERT INTO access_log (chunk_id, accessed_at) VALUES (?1, ?2)",
            params![id, now],
        )?;
        Ok(())
    }

    /// Get intervals (in days) between successive accesses for a memory.
    /// Returns an empty vec if fewer than 2 access events exist.
    pub fn get_access_intervals(&self, chunk_id: i64) -> Result<Vec<f64>> {
        let mut stmt = self.conn.prepare(
            "SELECT accessed_at FROM access_log WHERE chunk_id = ?1 ORDER BY accessed_at ASC",
        )?;
        let timestamps: Vec<String> = stmt
            .query_map(params![chunk_id], |row| row.get(0))?
            .filter_map(|r| r.ok())
            .collect();

        if timestamps.len() < 2 {
            return Ok(vec![]);
        }

        let mut intervals = Vec::with_capacity(timestamps.len() - 1);
        for pair in timestamps.windows(2) {
            let t0 = chrono::DateTime::parse_from_rfc3339(&pair[0])
                .map(|dt| dt.with_timezone(&chrono::Utc));
            let t1 = chrono::DateTime::parse_from_rfc3339(&pair[1])
                .map(|dt| dt.with_timezone(&chrono::Utc));
            if let (Ok(t0), Ok(t1)) = (t0, t1) {
                let days = (t1 - t0).num_seconds() as f64 / 86400.0;
                intervals.push(days.max(0.001)); // Floor at ~86s to avoid div-by-zero
            }
        }
        Ok(intervals)
    }

    /// Log a retrieval event. Returns the log entry ID.
    pub fn log_retrieval(
        &self,
        query: &str,
        tool: &str,
        results: &[(i64, f64)],
        latency_ms: Option<i64>,
    ) -> Result<i64> {
        let now = chrono::Utc::now().to_rfc3339();
        let ids: Vec<i64> = results.iter().map(|(id, _)| *id).collect();
        let scores: Vec<f64> = results.iter().map(|(_, s)| *s).collect();
        self.conn.execute(
            "INSERT INTO retrieval_log (query, tool, result_ids, scores, result_count, latency_ms, created_at)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)",
            params![
                query,
                tool,
                serde_json::to_string(&ids)?,
                serde_json::to_string(&scores)?,
                results.len() as i64,
                latency_ms,
                now,
            ],
        )?;
        Ok(self.conn.last_insert_rowid())
    }

    /// Validate that a chunk_id was in the results for a given retrieval log entry.
    pub fn validate_feedback(&self, log_id: i64, chunk_id: i64) -> Result<bool> {
        let result_ids_json: String = self.conn.query_row(
            "SELECT result_ids FROM retrieval_log WHERE id = ?1",
            params![log_id],
            |row| row.get(0),
        )?;
        let result_ids: Vec<i64> = serde_json::from_str(&result_ids_json).unwrap_or_default();
        Ok(result_ids.contains(&chunk_id))
    }

    /// Record feedback on a retrieval result.
    pub fn log_feedback(&self, log_id: i64, chunk_id: i64, signal: &str) -> Result<()> {
        let now = chrono::Utc::now().to_rfc3339();
        self.conn.execute(
            "INSERT INTO feedback (retrieval_log_id, chunk_id, signal, created_at)
             VALUES (?1, ?2, ?3, ?4)",
            params![log_id, chunk_id, signal, now],
        )?;
        Ok(())
    }

    /// Adjust salience for a memory, clamped to [0.0, 1.0].
    pub fn adjust_salience(&self, chunk_id: i64, delta: f64) -> Result<()> {
        let now = chrono::Utc::now().to_rfc3339();
        self.conn.execute(
            "UPDATE chunks SET
                salience = MIN(1.0, MAX(0.0, salience + ?1)),
                updated_at = ?2
             WHERE id = ?3 AND kind = 'memory'",
            params![delta, now, chunk_id],
        )?;
        Ok(())
    }

    /// Export training data: retrieval logs joined with feedback and chunk content.
    pub fn export_training_data(&self) -> Result<Vec<serde_json::Value>> {
        let mut log_stmt = self.conn.prepare(
            "SELECT id, query, result_ids FROM retrieval_log ORDER BY created_at ASC",
        )?;
        let logs: Vec<(i64, String, String)> = log_stmt
            .query_map([], |row| Ok((row.get(0)?, row.get(1)?, row.get(2)?)))?
            .filter_map(|r| r.ok())
            .collect();

        let mut examples = Vec::new();
        for (log_id, query, result_ids_json) in &logs {
            let result_ids: Vec<i64> = serde_json::from_str(result_ids_json).unwrap_or_default();
            if result_ids.is_empty() {
                continue;
            }

            // Get feedback for this log entry
            let mut fb_stmt = self.conn.prepare(
                "SELECT chunk_id, signal FROM feedback WHERE retrieval_log_id = ?1",
            )?;
            let feedback: Vec<(i64, String)> = fb_stmt
                .query_map(params![log_id], |row| Ok((row.get(0)?, row.get(1)?)))?
                .filter_map(|r| r.ok())
                .collect();

            if feedback.is_empty() {
                continue; // No signal, skip
            }

            let positive_ids: HashSet<i64> = feedback
                .iter()
                .filter(|(_, s)| s == "positive")
                .map(|(id, _)| *id)
                .collect();
            let negative_ids: HashSet<i64> = feedback
                .iter()
                .filter(|(_, s)| s == "negative")
                .map(|(id, _)| *id)
                .collect();

            // Fetch content for all result chunks
            let mut positive = Vec::new();
            let mut negative = Vec::new();
            let mut hard_negatives = Vec::new();
            for &cid in &result_ids {
                let content: Option<String> = self.conn.prepare(
                    "SELECT content FROM chunks WHERE id = ?1"
                )?.query_row(params![cid], |row| row.get(0)).optional()?;
                if let Some(content) = content {
                    if positive_ids.contains(&cid) {
                        positive.push(content);
                    } else if negative_ids.contains(&cid) {
                        negative.push(content);
                    } else {
                        hard_negatives.push(content);
                    }
                }
            }

            examples.push(serde_json::json!({
                "query": query,
                "positive": positive,
                "negative": negative,
                "hard_negatives": hard_negatives,
            }));
        }
        Ok(examples)
    }

    /// Look up a memory by content hash. Returns the chunk ID if found.
    pub fn get_memory_by_hash(&self, hash: &str) -> Result<Option<i64>> {
        let mut stmt = self.conn.prepare(
            "SELECT id FROM chunks WHERE content_hash = ?1 AND kind = 'memory' LIMIT 1",
        )?;
        let id = stmt
            .query_row(params![hash], |row| row.get(0))
            .optional()?;
        Ok(id)
    }

    /// Archive a memory (soft delete). Returns false if ID not found or not a memory.
    pub fn archive_memory(&self, id: i64) -> Result<bool> {
        let now = chrono::Utc::now().to_rfc3339();
        let rows = self.conn.execute(
            "UPDATE chunks SET archived = 1, updated_at = ?1 WHERE id = ?2 AND kind = 'memory'",
            params![now, id],
        )?;
        Ok(rows > 0)
    }

    /// Create a session handoff.
    pub fn create_handoff(
        &self,
        id: &str,
        summary: &str,
        next_steps: &str,
        project: &str,
    ) -> Result<()> {
        let now = chrono::Utc::now().to_rfc3339();
        self.conn.execute(
            "INSERT INTO handoffs (id, summary, next_steps, project, created_at)
             VALUES (?1, ?2, ?3, ?4, ?5)",
            params![id, summary, next_steps, project, now],
        )?;
        Ok(())
    }

    /// Get the latest handoff, optionally filtered by project.
    pub fn get_latest_handoff(&self, project: Option<&str>) -> Result<Option<Handoff>> {
        let sql = if project.is_some() {
            "SELECT id, summary, next_steps, project, created_at
             FROM handoffs WHERE project = ?1 ORDER BY created_at DESC LIMIT 1"
        } else {
            "SELECT id, summary, next_steps, project, created_at
             FROM handoffs ORDER BY created_at DESC LIMIT 1"
        };

        let mut stmt = self.conn.prepare(sql)?;
        let handoff = if let Some(p) = project {
            stmt.query_row(params![p], row_to_handoff)
        } else {
            stmt.query_row([], row_to_handoff)
        };

        match handoff {
            Ok(h) => Ok(Some(h)),
            Err(rusqlite::Error::QueryReturnedNoRows) => Ok(None),
            Err(e) => Err(e.into()),
        }
    }

    /// Raw connection access — restricted to this crate (tests only).
    #[cfg(test)]
    pub(crate) fn conn(&self) -> &Connection {
        &self.conn
    }

    // --- Codebase operations ---

    /// Get or create a codebase entry, returning its ID.
    pub fn get_or_create_codebase(&self, root_path: &str, name: &str) -> Result<i64> {
        self.conn.execute(
            "INSERT INTO codebases (root_path, name) VALUES (?1, ?2)
             ON CONFLICT(root_path) DO UPDATE SET name = ?2",
            params![root_path, name],
        )?;
        let id = self.conn.query_row(
            "SELECT id FROM codebases WHERE root_path = ?1",
            params![root_path],
            |row| row.get(0),
        )?;
        Ok(id)
    }

    /// Get all file hashes for a codebase (rel_path → hash).
    pub fn get_all_file_hashes(&self, codebase_id: i64) -> Result<HashMap<String, String>> {
        let mut stmt = self.conn.prepare(
            "SELECT file_path, file_hash FROM indexed_files WHERE codebase_id = ?1",
        )?;
        let map = stmt
            .query_map(params![codebase_id], |row| {
                Ok((row.get::<_, String>(0)?, row.get::<_, String>(1)?))
            })?
            .filter_map(|r| r.ok())
            .collect();
        Ok(map)
    }

    /// Update the indexed_at timestamp for a codebase.
    pub fn touch_codebase(&self, codebase_id: i64) -> Result<()> {
        self.conn.execute(
            "UPDATE codebases SET indexed_at = ?1 WHERE id = ?2",
            params![now_millis(), codebase_id],
        )?;
        Ok(())
    }

    // --- Code chunk operations ---

    /// Batch upsert chunks for multiple files within a transaction.
    /// Deletes old chunks for each file, inserts new ones. Returns total chunk count.
    pub fn batch_upsert_chunks(&self, codebase_id: i64, file_chunks: &[FileChunks]) -> Result<usize> {
        self.conn.execute_batch("BEGIN")?;
        let result = (|| -> Result<usize> {
            let mut total = 0;
            let now = chrono::Utc::now().to_rfc3339();
            let ts = now_millis();
            for fc in file_chunks {
                // Delete old FTS rows before removing chunks (avoids orphaned FTS entries)
                self.conn.execute(
                    "DELETE FROM chunks_fts WHERE rowid IN (
                        SELECT id FROM chunks WHERE codebase_id = ?1 AND file_path = ?2 AND kind = 'code'
                    )",
                    params![codebase_id, fc.file_path],
                )?;
                // Delete old chunks for this file
                self.conn.execute(
                    "DELETE FROM chunks WHERE codebase_id = ?1 AND file_path = ?2 AND kind = 'code'",
                    params![codebase_id, fc.file_path],
                )?;
                // Insert new chunks
                for c in &fc.chunks {
                    self.conn.execute(
                        "INSERT INTO chunks (kind, codebase_id, file_path, chunk_key, language,
                            symbol_kind, symbol_name, signature, content, snippet,
                            start_line, end_line, file_hash, indexed_at, created_at, updated_at)
                         VALUES ('code', ?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, ?14, ?14)
                         ON CONFLICT(codebase_id, chunk_key) DO UPDATE SET
                            content = excluded.content, snippet = excluded.snippet,
                            signature = excluded.signature, symbol_kind = excluded.symbol_kind,
                            symbol_name = excluded.symbol_name, file_hash = excluded.file_hash,
                            start_line = excluded.start_line, end_line = excluded.end_line,
                            indexed_at = excluded.indexed_at, updated_at = excluded.updated_at,
                            embedding = NULL, embedding_model = ''",
                        params![
                            codebase_id, c.file_path, c.chunk_key, c.language,
                            c.symbol_kind, c.symbol_name, c.signature, c.snippet, c.snippet,
                            c.start_line, c.end_line, c.file_hash, ts, now,
                        ],
                    )?;
                    total += 1;
                }
                // Upsert indexed_files
                self.conn.execute(
                    "INSERT INTO indexed_files (codebase_id, file_path, file_hash, chunk_count, indexed_at)
                     VALUES (?1, ?2, ?3, ?4, ?5)
                     ON CONFLICT(codebase_id, file_path) DO UPDATE SET
                        file_hash = excluded.file_hash, chunk_count = excluded.chunk_count,
                        indexed_at = excluded.indexed_at",
                    params![codebase_id, fc.file_path, fc.file_hash, fc.chunks.len() as i64, ts],
                )?;
            }
            Ok(total)
        })();
        match result {
            Ok(total) => {
                self.conn.execute_batch("COMMIT")?;
                Ok(total)
            }
            Err(e) => {
                let _ = self.conn.execute_batch("ROLLBACK");
                Err(e)
            }
        }
    }

    /// Remove indexed files and their chunks/graph edges that no longer exist on disk.
    pub fn remove_stale_files(&self, codebase_id: i64, active_files: &HashSet<String>) -> Result<usize> {
        // Get all indexed files for this codebase
        let mut stmt = self.conn.prepare(
            "SELECT file_path FROM indexed_files WHERE codebase_id = ?1",
        )?;
        let stale: Vec<String> = stmt
            .query_map(params![codebase_id], |row| row.get(0))?
            .filter_map(|r| r.ok())
            .filter(|p: &String| !active_files.contains(p))
            .collect();

        for file_path in &stale {
            // Delete FTS entries for these chunks
            self.conn.execute(
                "DELETE FROM chunks_fts WHERE rowid IN (
                    SELECT id FROM chunks WHERE codebase_id = ?1 AND file_path = ?2 AND kind = 'code'
                )",
                params![codebase_id, file_path],
            )?;
            self.conn.execute(
                "DELETE FROM chunks WHERE codebase_id = ?1 AND file_path = ?2 AND kind = 'code'",
                params![codebase_id, file_path],
            )?;
            self.conn.execute(
                "DELETE FROM graph WHERE codebase_id = ?1 AND file_path = ?2 AND role != 'associates'",
                params![codebase_id, file_path],
            )?;
            self.conn.execute(
                "DELETE FROM indexed_files WHERE codebase_id = ?1 AND file_path = ?2",
                params![codebase_id, file_path],
            )?;
        }

        Ok(stale.len())
    }

    // --- Graph operations ---

    /// Replace all code graph edges for a file. Preserves Hebbian associations.
    pub fn upsert_graph_edges_for_file(
        &self,
        codebase_id: i64,
        file_path: &str,
        tags: &[ferret::graph::Tag],
    ) -> Result<usize> {
        self.conn.execute(
            "DELETE FROM graph WHERE codebase_id = ?1 AND file_path = ?2 AND role != 'associates'",
            params![codebase_id, file_path],
        )?;
        for tag in tags {
            self.conn.execute(
                "INSERT OR IGNORE INTO graph (codebase_id, file_path, symbol, role, kind, line, strength)
                 VALUES (?1, ?2, ?3, ?4, ?5, ?6, 1.0)",
                params![codebase_id, file_path, tag.symbol, tag.role, tag.kind, tag.line as i64],
            )?;
        }
        Ok(tags.len())
    }

    /// Find all definitions of a symbol.
    pub fn find_definitions(&self, symbol: &str, codebase_id: Option<i64>) -> Result<Vec<GraphEdge>> {
        self.query_graph_edges(symbol, "definition", codebase_id)
    }

    /// Find all references to a symbol.
    pub fn find_references(&self, symbol: &str, codebase_id: Option<i64>) -> Result<Vec<GraphEdge>> {
        self.query_graph_edges(symbol, "reference", codebase_id)
    }

    fn query_graph_edges(&self, symbol: &str, role: &str, codebase_id: Option<i64>) -> Result<Vec<GraphEdge>> {
        if let Some(cb) = codebase_id {
            let mut stmt = self.conn.prepare(
                "SELECT file_path, symbol, role, kind, line FROM graph
                 WHERE symbol = ?1 AND role = ?2 AND codebase_id = ?3
                 ORDER BY file_path, line",
            )?;
            let edges = stmt
                .query_map(params![symbol, role, cb], row_to_graph_edge)?
                .filter_map(|r| r.ok())
                .collect();
            Ok(edges)
        } else {
            let mut stmt = self.conn.prepare(
                "SELECT file_path, symbol, role, kind, line FROM graph
                 WHERE symbol = ?1 AND role = ?2
                 ORDER BY file_path, line",
            )?;
            let edges = stmt
                .query_map(params![symbol, role], row_to_graph_edge)?
                .filter_map(|r| r.ok())
                .collect();
            Ok(edges)
        }
    }

    // --- FTS sync for code ---

    /// Incremental FTS sync for changed files. Uses code_expand() on symbol_name.
    pub fn sync_fts_for_files(&self, codebase_id: i64, changed_files: &[String]) -> Result<()> {
        for file_path in changed_files {
            self.conn.execute(
                "DELETE FROM chunks_fts WHERE rowid IN (
                    SELECT id FROM chunks WHERE codebase_id = ?1 AND file_path = ?2 AND kind = 'code'
                )",
                params![codebase_id, file_path],
            )?;
            self.conn.execute(
                "INSERT INTO chunks_fts (rowid, title, content, snippet, symbol_name, descriptors)
                 SELECT id, COALESCE(symbol_name, ''), content, snippet,
                        code_expand(COALESCE(symbol_name, '')), COALESCE(descriptors, '')
                 FROM chunks WHERE codebase_id = ?1 AND file_path = ?2 AND kind = 'code'",
                params![codebase_id, file_path],
            )?;
        }
        Ok(())
    }

    /// Full FTS rebuild for all code chunks in a codebase.
    pub fn rebuild_fts_for_codebase(&self, codebase_id: i64) -> Result<()> {
        self.conn.execute(
            "DELETE FROM chunks_fts WHERE rowid IN (
                SELECT id FROM chunks WHERE codebase_id = ?1 AND kind = 'code'
            )",
            params![codebase_id],
        )?;
        self.conn.execute(
            "INSERT INTO chunks_fts (rowid, title, content, snippet, symbol_name, descriptors)
             SELECT id, COALESCE(symbol_name, ''), content, snippet,
                    code_expand(COALESCE(symbol_name, '')), COALESCE(descriptors, '')
             FROM chunks WHERE codebase_id = ?1 AND kind = 'code'",
            params![codebase_id],
        )?;
        Ok(())
    }

    // --- Embedding operations ---

    /// Get chunks that need (re-)embedding: NULL embedding or wrong model.
    pub fn get_stale_embeddings(&self, codebase_id: i64, model_name: &str) -> Result<Vec<StaleChunk>> {
        let mut stmt = self.conn.prepare(
            "SELECT id, file_path, language, symbol_kind, symbol_name, signature, content
             FROM chunks
             WHERE codebase_id = ?1 AND kind = 'code'
               AND (embedding IS NULL OR embedding_model != ?2)",
        )?;
        let chunks = stmt
            .query_map(params![codebase_id, model_name], |row| {
                Ok(StaleChunk {
                    id: row.get(0)?,
                    file_path: row.get(1)?,
                    language: row.get(2)?,
                    symbol_kind: row.get(3)?,
                    symbol_name: row.get(4)?,
                    signature: row.get(5)?,
                    snippet: row.get(6)?,
                })
            })?
            .filter_map(|r| r.ok())
            .collect();
        Ok(chunks)
    }

    /// Batch update embeddings for chunks by ID.
    pub fn batch_upsert_embeddings(&self, items: &[(i64, &[f32], &str)]) -> Result<()> {
        let mut stmt = self.conn.prepare_cached(
            "UPDATE chunks SET embedding = ?1, embedding_model = ?2 WHERE id = ?3",
        )?;
        for &(id, embedding, model_name) in items {
            let blob = embedding_to_blob(embedding);
            stmt.execute(params![blob, model_name, id])?;
        }
        Ok(())
    }

    // --- Search operations ---

    /// Full-text search across code and/or memory chunks.
    pub fn fts_search(
        &self,
        query: &str,
        kind_filter: Option<&str>,
        limit: usize,
    ) -> Result<Vec<SearchHit>> {
        let prepared = ferret::tokenizer::prepare_fts_query(query);
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
            .filter_map(|r| r.ok())
            .collect();
        Ok(hits)
    }

    /// Brute-force vector search across all chunks with embeddings.
    pub fn vector_search(
        &self,
        query_embedding: &[f32],
        model_name: &str,
        kind_filter: Option<&str>,
        limit: usize,
    ) -> Result<Vec<SearchHit>> {
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
            let Some(emb) = blob_to_embedding(&blob) else { continue };
            let score = dot_product(query_embedding, emb) as f64;
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

        scored.sort_by(|a, b| b.score.partial_cmp(&a.score).unwrap_or(std::cmp::Ordering::Equal));
        scored.truncate(limit);
        Ok(scored)
    }

    // --- HNSW support ---

    /// Get all embeddings as (chunk_id, embedding_vector) pairs for HNSW index building.
    pub fn get_all_embeddings(&self) -> Result<Vec<(i64, Vec<f32>)>> {
        let mut stmt = self.conn.prepare(
            "SELECT id, embedding FROM chunks WHERE embedding IS NOT NULL",
        )?;
        let mut results = Vec::new();
        let mut dropped = 0u64;
        let raw_rows = stmt.query_map([], |row| {
            let id: i64 = row.get(0)?;
            let blob: Vec<u8> = row.get(1)?;
            Ok((id, blob))
        })?;
        for row in raw_rows {
            match row {
                Ok((id, blob)) => match blob_to_embedding(&blob) {
                    Some(emb) => results.push((id, emb.to_vec())),
                    None => dropped += 1,
                },
                Err(_) => dropped += 1,
            }
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
        hnsw: &ferret::hnsw::HnswIndex,
        query_embedding: &[f32],
        kind_filter: Option<&str>,
        limit: usize,
    ) -> Result<Vec<SearchHit>> {
        if hnsw.len() == 0 {
            return self.vector_search(query_embedding, ferret::embed::MODEL_NAME, kind_filter, limit);
        }

        // Retrieve 2x candidates for re-scoring (HNSW is approximate)
        let candidates = hnsw.search(query_embedding, limit * 2);
        if candidates.is_empty() {
            return self.vector_search(query_embedding, ferret::embed::MODEL_NAME, kind_filter, limit);
        }

        let kind_clause = match kind_filter {
            Some("code") => "AND kind = 'code'",
            Some("memory") => "AND kind = 'memory'",
            _ => "",
        };

        // Load full SearchHit data for HNSW candidates and rescore with exact embeddings
        let placeholders: String = candidates.iter().map(|_| "?").collect::<Vec<_>>().join(",");
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
        let param_refs: Vec<&dyn rusqlite::types::ToSql> = params.iter().map(|p| p.as_ref()).collect();

        let mut scored: Vec<SearchHit> = Vec::new();
        let mut rows = stmt.query(param_refs.as_slice())?;
        while let Some(row) = rows.next()? {
            let blob: Vec<u8> = row.get(11)?;
            let Some(emb) = blob_to_embedding(&blob) else { continue };
            let score = dot_product(query_embedding, emb) as f64;
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

        scored.sort_by(|a, b| b.score.partial_cmp(&a.score).unwrap_or(std::cmp::Ordering::Equal));
        scored.truncate(limit);
        Ok(scored)
    }

    // --- Cognitive memory queries ---

    /// List memories ordered by access count (most accessed first).
    pub fn list_memories_by_access(&self, limit: usize) -> Result<Vec<Chunk>> {
        let mut stmt = self.conn.prepare(
            "SELECT id, kind, title, content, snippet, symbol_name, symbol_kind, signature,
                    file_path, language, start_line, end_line, memory_type,
                    descriptors, source, access_count, last_accessed, salience, archived,
                    content_hash, agent_id, created_at, updated_at, codebase_id
             FROM chunks WHERE kind = 'memory' AND archived = 0
             ORDER BY access_count DESC LIMIT ?1",
        )?;
        let rows = stmt
            .query_map(params![limit as i64], row_to_chunk)?
            .filter_map(|r| r.ok())
            .collect();
        Ok(rows)
    }

    /// Count memories, optionally filtering by archived status.
    pub fn count_memories(&self, archived: bool) -> Result<i64> {
        let count: i64 = self.conn.query_row(
            "SELECT COUNT(*) FROM chunks WHERE kind = 'memory' AND archived = ?1",
            params![archived as i64],
            |row| row.get(0),
        )?;
        Ok(count)
    }

    /// Count memories grouped by memory_type. Returns HashMap<type, count>.
    pub fn count_memories_by_type(&self) -> Result<HashMap<String, i64>> {
        let mut stmt = self.conn.prepare(
            "SELECT memory_type, COUNT(*) FROM chunks
             WHERE kind = 'memory' AND archived = 0
             GROUP BY memory_type",
        )?;
        let map = stmt
            .query_map([], |row| {
                Ok((
                    row.get::<_, Option<String>>(0)?.unwrap_or_default(),
                    row.get::<_, i64>(1)?,
                ))
            })?
            .filter_map(|r| r.ok())
            .collect();
        Ok(map)
    }

    /// List stale memories (not accessed within `older_than_days`).
    /// Handles NULL last_accessed by treating those as stale if they're old enough.
    pub fn list_memories_stale(&self, older_than_days: i64, limit: usize) -> Result<Vec<Chunk>> {
        let mut stmt = self.conn.prepare(
            "SELECT id, kind, title, content, snippet, symbol_name, symbol_kind, signature,
                    file_path, language, start_line, end_line, memory_type,
                    descriptors, source, access_count, last_accessed, salience, archived,
                    content_hash, agent_id, created_at, updated_at, codebase_id
             FROM chunks WHERE kind = 'memory' AND archived = 0
               AND (last_accessed IS NULL OR julianday('now') - julianday(last_accessed) > ?1)
               AND julianday('now') - julianday(created_at) > ?1
             ORDER BY COALESCE(last_accessed, created_at) ASC LIMIT ?2",
        )?;
        let rows = stmt
            .query_map(params![older_than_days, limit as i64], row_to_chunk)?
            .filter_map(|r| r.ok())
            .collect();
        Ok(rows)
    }

    /// List growing memories: recently accessed with high access counts.
    pub fn list_memories_growing(&self, since_days: i64, limit: usize) -> Result<Vec<Chunk>> {
        let mut stmt = self.conn.prepare(
            "SELECT id, kind, title, content, snippet, symbol_name, symbol_kind, signature,
                    file_path, language, start_line, end_line, memory_type,
                    descriptors, source, access_count, last_accessed, salience, archived,
                    content_hash, agent_id, created_at, updated_at, codebase_id
             FROM chunks WHERE kind = 'memory' AND archived = 0
               AND last_accessed IS NOT NULL
               AND julianday('now') - julianday(last_accessed) <= ?1
             ORDER BY access_count DESC LIMIT ?2",
        )?;
        let rows = stmt
            .query_map(params![since_days, limit as i64], row_to_chunk)?
            .filter_map(|r| r.ok())
            .collect();
        Ok(rows)
    }

    /// List fading memories: previously accessed but not recently.
    pub fn list_memories_fading(&self, before_days: i64, limit: usize) -> Result<Vec<Chunk>> {
        let mut stmt = self.conn.prepare(
            "SELECT id, kind, title, content, snippet, symbol_name, symbol_kind, signature,
                    file_path, language, start_line, end_line, memory_type,
                    descriptors, source, access_count, last_accessed, salience, archived,
                    content_hash, agent_id, created_at, updated_at, codebase_id
             FROM chunks WHERE kind = 'memory' AND archived = 0
               AND access_count > 0
               AND last_accessed IS NOT NULL
               AND julianday('now') - julianday(last_accessed) > ?1
             ORDER BY last_accessed ASC LIMIT ?2",
        )?;
        let rows = stmt
            .query_map(params![before_days, limit as i64], row_to_chunk)?
            .filter_map(|r| r.ok())
            .collect();
        Ok(rows)
    }

    /// List memories that have Hebbian associations (via graph table).
    pub fn list_memories_with_associations(&self, limit: usize) -> Result<Vec<(Chunk, i64)>> {
        let mut stmt = self.conn.prepare(
            "SELECT c.id, c.kind, c.title, c.content, c.snippet, c.symbol_name, c.symbol_kind,
                    c.signature, c.file_path, c.language, c.start_line, c.end_line, c.memory_type,
                    c.descriptors, c.source, c.access_count, c.last_accessed, c.salience, c.archived,
                    c.content_hash, c.agent_id, c.created_at, c.updated_at, c.codebase_id,
                    COUNT(g.id) as assoc_count
             FROM chunks c
             JOIN graph g ON (g.source_chunk = c.id OR g.target_chunk = c.id) AND g.role = 'associates'
             WHERE c.kind = 'memory' AND c.archived = 0
             GROUP BY c.id
             ORDER BY assoc_count DESC LIMIT ?1",
        )?;
        let rows = stmt
            .query_map(params![limit as i64], |row| {
                let chunk = row_to_chunk(row)?;
                let count: i64 = row.get(24)?;
                Ok((chunk, count))
            })?
            .filter_map(|r| r.ok())
            .collect();
        Ok(rows)
    }

    /// Upsert a bidirectional Hebbian association between two memory chunks.
    pub fn upsert_association(&self, source_id: i64, target_id: i64) -> Result<()> {
        // Insert both directions (idempotent via unique index)
        self.conn.execute(
            "INSERT OR IGNORE INTO graph (source_chunk, target_chunk, role, strength)
             VALUES (?1, ?2, 'associates', 1.0)",
            params![source_id, target_id],
        )?;
        self.conn.execute(
            "INSERT OR IGNORE INTO graph (source_chunk, target_chunk, role, strength)
             VALUES (?1, ?2, 'associates', 1.0)",
            params![target_id, source_id],
        )?;
        Ok(())
    }

    /// Delete all entity edges for a memory chunk (used before re-inserting on update).
    pub fn delete_entity_edges(&self, chunk_id: i64) -> Result<usize> {
        let count = self.conn.execute(
            "DELETE FROM graph WHERE source_chunk = ?1 AND role = 'entity'",
            params![chunk_id],
        )?;
        Ok(count)
    }

    /// Batch insert entity graph edges for a memory chunk.
    /// Each entity is (value, kind). Idempotent via unique index.
    pub fn upsert_entity_edges(&self, chunk_id: i64, entities: &[(String, String)]) -> Result<usize> {
        let mut count = 0;
        for (value, kind) in entities {
            let rows = self.conn.execute(
                "INSERT OR IGNORE INTO graph (source_chunk, target_chunk, role, kind, symbol, strength)
                 VALUES (?1, NULL, 'entity', ?2, ?3, 1.0)",
                params![chunk_id, kind, value],
            )?;
            count += rows;
        }
        Ok(count)
    }

    /// Find all memories that mention a specific entity value.
    pub fn find_memories_by_entity(&self, entity_value: &str) -> Result<Vec<SearchHit>> {
        let mut stmt = self.conn.prepare(
            "SELECT c.id, c.kind, c.file_path, c.symbol_name, c.symbol_kind,
                    c.signature, c.title, c.snippet, c.start_line, c.end_line,
                    c.memory_type, c.access_count, c.last_accessed, c.salience,
                    c.created_at, c.archived, c.descriptors, c.content_hash
             FROM graph g
             JOIN chunks c ON c.id = g.source_chunk
             WHERE g.role = 'entity' AND g.symbol = ?1
               AND c.kind = 'memory' AND c.archived = 0",
        )?;
        let rows = stmt
            .query_map(params![entity_value], |row| {
                Ok(SearchHit {
                    id: row.get(0)?,
                    kind: row.get(1)?,
                    file_path: row.get(2)?,
                    symbol_name: row.get(3)?,
                    symbol_kind: row.get(4)?,
                    signature: row.get(5)?,
                    title: row.get(6)?,
                    snippet: row.get(7)?,
                    start_line: row.get(8)?,
                    end_line: row.get(9)?,
                    memory_type: row.get(10)?,
                    score: 0.0,
                    reranker_score: None,
                    access_count: row.get(11)?,
                    last_accessed: row.get(12)?,
                    salience: row.get(13)?,
                    created_at: row.get(14)?,
                    archived: row.get::<_, i64>(15)? != 0,
                    descriptors: row.get(16)?,
                })
            })?
            .filter_map(|r| r.ok())
            .collect();
        Ok(rows)
    }

    /// Create summary graph edges linking a summary chunk to its member chunks.
    pub fn insert_summary_edges(&self, summary_id: i64, member_ids: &[i64]) -> Result<()> {
        for &member_id in member_ids {
            self.conn.execute(
                "INSERT OR IGNORE INTO graph (source_chunk, target_chunk, role, strength)
                 VALUES (?1, ?2, 'summarizes', 1.0)",
                params![summary_id, member_id],
            )?;
            self.conn.execute(
                "INSERT OR IGNORE INTO graph (source_chunk, target_chunk, role, strength)
                 VALUES (?1, ?2, 'summarized_by', 1.0)",
                params![member_id, summary_id],
            )?;
        }
        Ok(())
    }

    /// Check if a summary already exists for the given set of member chunk IDs.
    /// Returns the summary chunk IDs if found.
    pub fn get_summaries_for_chunks(&self, member_ids: &[i64]) -> Result<Vec<i64>> {
        if member_ids.is_empty() {
            return Ok(vec![]);
        }
        // Find chunks that have 'summarizes' edges to ALL the given member IDs
        let placeholders: Vec<String> = member_ids.iter().map(|_| "?".to_string()).collect();
        // Find summaries that contain ALL requested members AND have exactly that many edges
        // (exact-set match, not superset match)
        let sql = format!(
            "SELECT g1.source_chunk FROM graph g1
             WHERE g1.role = 'summarizes' AND g1.target_chunk IN ({})
             GROUP BY g1.source_chunk
             HAVING COUNT(DISTINCT g1.target_chunk) = ?
               AND (SELECT COUNT(*) FROM graph g2
                    WHERE g2.source_chunk = g1.source_chunk AND g2.role = 'summarizes') = ?",
            placeholders.join(",")
        );
        let mut stmt = self.conn.prepare(&sql)?;
        let mut params_vec: Vec<Box<dyn rusqlite::types::ToSql>> = member_ids
            .iter()
            .map(|id| Box::new(*id) as Box<dyn rusqlite::types::ToSql>)
            .collect();
        params_vec.push(Box::new(member_ids.len() as i64));
        params_vec.push(Box::new(member_ids.len() as i64));
        let rows: Vec<i64> = stmt
            .query_map(rusqlite::params_from_iter(params_vec.iter().map(|p| p.as_ref())), |row| row.get(0))?
            .filter_map(|r| r.ok())
            .collect();
        Ok(rows)
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
            let Some(emb) = blob_to_embedding(&blob) else { continue };
            let sim = dot_product(query_embedding, emb) as f64;
            if sim >= threshold {
                scored.push((id, sim));
            }
        }
        scored.sort_by(|a, b| b.1.partial_cmp(&a.1).unwrap_or(std::cmp::Ordering::Equal));
        scored.truncate(limit);
        Ok(scored)
    }

    /// Get all non-archived memory embeddings for consolidation scanning.
    pub fn get_all_memory_embeddings(&self, model_name: &str) -> Result<Vec<(i64, Vec<f32>)>> {
        let mut stmt = self.conn.prepare(
            "SELECT id, embedding FROM chunks
             WHERE kind = 'memory' AND archived = 0
               AND embedding IS NOT NULL AND embedding_model = ?1",
        )?;
        let mut results = Vec::new();
        let mut rows = stmt.query(params![model_name])?;
        while let Some(row) = rows.next()? {
            let id: i64 = row.get(0)?;
            let blob: Vec<u8> = row.get(1)?;
            if let Some(emb) = blob_to_embedding(&blob) {
                results.push((id, emb.to_vec()));
            }
        }
        Ok(results)
    }

    /// List recent handoffs (for `me` active_projects).
    pub fn list_recent_handoffs(&self, limit: usize) -> Result<Vec<Handoff>> {
        let mut stmt = self.conn.prepare(
            "SELECT id, summary, next_steps, project, created_at
             FROM handoffs ORDER BY created_at DESC LIMIT ?1",
        )?;
        let rows = stmt
            .query_map(params![limit as i64], row_to_handoff)?
            .filter_map(|r| r.ok())
            .collect();
        Ok(rows)
    }

    // --- Graph query methods (for navigate/map/impact MCP tools) ---

    /// List all indexed codebases.
    pub fn list_codebases(&self) -> Result<Vec<(i64, String, String)>> {
        let mut stmt = self.conn.prepare(
            "SELECT id, root_path, name FROM codebases ORDER BY name",
        )?;
        let rows = stmt
            .query_map([], |row| Ok((row.get(0)?, row.get(1)?, row.get(2)?)))?
            .collect::<std::result::Result<Vec<_>, _>>()?;
        Ok(rows)
    }

    /// Get all definition edges for a codebase (used by map).
    pub fn get_all_definitions(&self, codebase_id: i64) -> Result<Vec<GraphEdge>> {
        let mut stmt = self.conn.prepare(
            "SELECT file_path, symbol, 'definition' AS role, MAX(kind) AS kind, line
             FROM graph
             WHERE codebase_id = ?1 AND role = 'definition'
             GROUP BY file_path, symbol, line
             ORDER BY file_path, line",
        )?;
        let rows = stmt
            .query_map(params![codebase_id], row_to_graph_edge)?
            .collect::<std::result::Result<Vec<_>, _>>()?;
        Ok(rows)
    }

    /// Count reference occurrences per symbol (used by map for ranking).
    pub fn count_graph_references(&self, codebase_id: i64) -> Result<HashMap<String, usize>> {
        let mut stmt = self.conn.prepare(
            "SELECT symbol, COUNT(*) FROM graph
             WHERE codebase_id = ?1 AND role = 'reference'
             GROUP BY symbol",
        )?;
        let mut counts = HashMap::new();
        let rows = stmt.query_map(params![codebase_id], |row| {
            Ok((row.get::<_, String>(0)?, row.get::<_, usize>(1)?))
        })?;
        for row in rows {
            let (symbol, count) = row?;
            counts.insert(symbol, count);
        }
        Ok(counts)
    }

    /// Get unique symbol names defined in a specific file (used by impact BFS).
    pub fn get_definitions_in_file(
        &self,
        file_path: &str,
        codebase_id: Option<i64>,
    ) -> Result<Vec<String>> {
        if let Some(cid) = codebase_id {
            let mut stmt = self.conn.prepare_cached(
                "SELECT DISTINCT symbol FROM graph
                 WHERE codebase_id = ?1 AND file_path = ?2 AND role = 'definition'",
            )?;
            let rows = stmt
                .query_map(params![cid, file_path], |row| row.get(0))?
                .collect::<std::result::Result<Vec<String>, _>>()?;
            Ok(rows)
        } else {
            let mut stmt = self.conn.prepare_cached(
                "SELECT DISTINCT symbol FROM graph
                 WHERE file_path = ?1 AND role = 'definition'",
            )?;
            let rows = stmt
                .query_map(params![file_path], |row| row.get(0))?
                .collect::<std::result::Result<Vec<String>, _>>()?;
            Ok(rows)
        }
    }

    /// BFS impact analysis: "if I change symbol X, what files might be affected?"
    ///
    /// Depth 1 = direct callers, depth 2+ = transitive dependents.
    pub fn find_impact(
        &self,
        start_symbol: &str,
        codebase_id: Option<i64>,
        max_depth: usize,
    ) -> Result<Vec<ImpactHit>> {
        use std::collections::{HashSet, VecDeque};

        const MAX_NODES: usize = 500;

        let mut hits = Vec::new();
        let mut visited_files: HashSet<String> = HashSet::new();
        let mut visited_symbols: HashSet<String> = HashSet::new();

        // Exclude the definition file(s) of the start symbol
        let start_defs = self.find_definitions(start_symbol, codebase_id)?;
        for d in &start_defs {
            visited_files.insert(d.file_path.clone());
        }
        visited_symbols.insert(start_symbol.to_owned());

        let mut frontier: VecDeque<String> = VecDeque::new();
        frontier.push_back(start_symbol.to_owned());

        for depth in 1..=max_depth {
            let mut next_frontier: HashSet<String> = HashSet::new();
            let current_batch: Vec<String> = frontier.drain(..).collect();

            if current_batch.is_empty() {
                break;
            }

            for sym in &current_batch {
                let refs = self.find_references(sym, codebase_id)?;
                let mut ref_files: HashSet<String> = HashSet::new();

                for r in &refs {
                    ref_files.insert(r.file_path.clone());
                    if visited_files.insert(r.file_path.clone()) {
                        hits.push(ImpactHit {
                            file_path: r.file_path.clone(),
                            via_symbol: sym.clone(),
                            depth,
                        });
                        if hits.len() >= MAX_NODES {
                            return Ok(hits);
                        }
                    }
                }

                for file in &ref_files {
                    let file_defs = self.get_definitions_in_file(file, codebase_id)?;
                    for def_sym in file_defs {
                        if visited_symbols.insert(def_sym.clone()) {
                            next_frontier.insert(def_sym);
                        }
                    }
                }
            }

            frontier.extend(next_frontier);
        }

        Ok(hits)
    }

    /// Get database file size in bytes via PRAGMA.
    pub fn db_size_bytes(&self) -> Result<i64> {
        let page_count: i64 = self.conn.query_row("PRAGMA page_count", [], |r| r.get(0))?;
        let page_size: i64 = self.conn.query_row("PRAGMA page_size", [], |r| r.get(0))?;
        Ok(page_count * page_size)
    }

    /// Count retrieval log entries.
    pub fn count_retrieval_logs(&self) -> Result<i64> {
        self.conn.query_row("SELECT COUNT(*) FROM retrieval_log", [], |r| r.get(0)).map_err(Into::into)
    }

    /// Count feedback entries.
    pub fn count_feedback(&self) -> Result<i64> {
        self.conn.query_row("SELECT COUNT(*) FROM feedback", [], |r| r.get(0)).map_err(Into::into)
    }

    /// Count access log entries.
    pub fn count_access_log(&self) -> Result<i64> {
        self.conn.query_row("SELECT COUNT(*) FROM access_log", [], |r| r.get(0)).map_err(Into::into)
    }

    /// Get the timestamp of the latest handoff (if any).
    pub fn latest_handoff_timestamp(&self) -> Result<Option<String>> {
        use rusqlite::OptionalExtension;
        self.conn.query_row(
            "SELECT created_at FROM handoffs ORDER BY created_at DESC LIMIT 1",
            [],
            |r| r.get(0),
        ).optional().map_err(Into::into)
    }

    /// Count indexed codebases.
    pub fn count_codebases(&self) -> Result<i64> {
        self.conn.query_row("SELECT COUNT(*) FROM codebases", [], |r| r.get(0)).map_err(Into::into)
    }

    /// Run PRAGMA optimize for query planner stats.
    pub fn optimize(&self) -> Result<()> {
        self.conn.execute_batch("PRAGMA optimize;")?;
        Ok(())
    }

    /// Run database maintenance: prune old logs, WAL checkpoint, ANALYZE, optimize.
    /// Returns counts of pruned rows.
    pub fn maintenance(&self, retention_days: i64) -> Result<MaintenanceReport> {
        let retention_days = retention_days.max(1);
        let cutoff = (chrono::Utc::now() - chrono::Duration::days(retention_days)).to_rfc3339();

        let tx = self.conn.unchecked_transaction()?;
        let retrieval_logs_pruned = tx.execute(
            "DELETE FROM retrieval_log WHERE created_at < ?1",
            params![cutoff],
        )?;
        let feedback_pruned = tx.execute(
            "DELETE FROM feedback WHERE created_at < ?1",
            params![cutoff],
        )?;
        let access_log_pruned = tx.execute(
            "DELETE FROM access_log WHERE accessed_at < ?1",
            params![cutoff],
        )?;
        tx.commit()?;

        // WAL checkpoint
        let (wal_pages_before, wal_pages_after): (i64, i64) = self.conn.query_row(
            "PRAGMA wal_checkpoint(TRUNCATE)",
            [],
            |row| Ok((row.get::<_, i64>(1).unwrap_or(0), row.get::<_, i64>(2).unwrap_or(0))),
        )?;

        self.conn.execute_batch("ANALYZE;")?;
        self.conn.execute_batch("PRAGMA optimize;")?;

        Ok(MaintenanceReport {
            retrieval_logs_pruned,
            feedback_pruned,
            access_log_pruned,
            wal_pages_before,
            wal_pages_after,
        })
    }

    /// Rebuild the database file (slow — use sparingly).
    pub fn vacuum(&self) -> Result<()> {
        self.conn.execute_batch("VACUUM;")?;
        Ok(())
    }
}

// --- Data types ---

/// Result of database maintenance operation.
#[derive(Debug, serde::Serialize)]
pub struct MaintenanceReport {
    pub retrieval_logs_pruned: usize,
    pub feedback_pruned: usize,
    pub access_log_pruned: usize,
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
pub struct Handoff {
    pub id: String,
    pub summary: String,
    pub next_steps: String,
    pub project: String,
    pub created_at: String,
}

#[derive(Debug, Clone)]
pub struct ImpactHit {
    pub file_path: String,
    pub via_symbol: String,
    pub depth: usize,
}

// --- Row mappers ---

fn row_to_chunk(row: &rusqlite::Row) -> rusqlite::Result<Chunk> {
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

fn row_to_handoff(row: &rusqlite::Row) -> rusqlite::Result<Handoff> {
    Ok(Handoff {
        id: row.get(0)?,
        summary: row.get(1)?,
        next_steps: row.get(2)?,
        project: row.get(3)?,
        created_at: row.get(4)?,
    })
}

fn row_to_graph_edge(row: &rusqlite::Row) -> rusqlite::Result<GraphEdge> {
    Ok(GraphEdge {
        file_path: row.get(0)?,
        symbol: row.get(1)?,
        role: row.get(2)?,
        kind: row.get(3)?,
        line: row.get(4)?,
    })
}

// --- Standalone search functions ---

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

// --- Utility functions ---

fn now_millis() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as i64
}

fn embedding_to_blob(embedding: &[f32]) -> Vec<u8> {
    embedding.iter().flat_map(|f| f.to_le_bytes()).collect()
}

fn blob_to_embedding(blob: &[u8]) -> Option<&[f32]> {
    bytemuck::try_cast_slice(blob).ok()
}

fn dot_product(a: &[f32], b: &[f32]) -> f32 {
    a.iter().zip(b).map(|(x, y)| x * y).sum()
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    #[test]
    fn test_open_creates_schema() {
        let dir = TempDir::new().unwrap();
        let db_path = dir.path().join("test.db");
        let store = Store::open(&db_path).unwrap();
        let (code, memory) = store.count_by_kind().unwrap();
        assert_eq!(code, 0);
        assert_eq!(memory, 0);
    }

    #[test]
    fn test_insert_and_get_memory() {
        let dir = TempDir::new().unwrap();
        let db_path = dir.path().join("test.db");
        let store = Store::open(&db_path).unwrap();

        let id = store
            .insert_memory(&MemoryParams {
                title: "Test preference",
                content: "Always use bun for running scripts",
                memory_type: "knowledge",
                descriptors: "tools, preferences",
                salience: 0.5,
                content_hash: "abc123",
                agent_id: "claude-code",
            })
            .unwrap();

        let chunk = store.get_chunk(id).unwrap().unwrap();
        assert_eq!(chunk.kind, "memory");
        assert_eq!(chunk.title, "Test preference");
        assert_eq!(chunk.memory_type, Some("knowledge".to_string()));
        assert_eq!(chunk.salience, 0.5);
        assert_eq!(chunk.access_count, 0);
    }

    #[test]
    fn test_touch_memory() {
        let dir = TempDir::new().unwrap();
        let db_path = dir.path().join("test.db");
        let store = Store::open(&db_path).unwrap();

        let id = store
            .insert_memory(&MemoryParams {
                title: "Test", content: "Content", memory_type: "knowledge",
                descriptors: "", salience: 0.5, content_hash: "", agent_id: "test",
            })
            .unwrap();

        store.touch_memory(id).unwrap();
        let chunk = store.get_chunk(id).unwrap().unwrap();
        assert_eq!(chunk.access_count, 1);
        assert_eq!(chunk.salience, 0.55);
        assert!(chunk.last_accessed.is_some());

        // Touch again — salience should stack
        store.touch_memory(id).unwrap();
        let chunk = store.get_chunk(id).unwrap().unwrap();
        assert_eq!(chunk.access_count, 2);
        assert!((chunk.salience - 0.6).abs() < 0.001);
    }

    #[test]
    fn test_list_memories() {
        let dir = TempDir::new().unwrap();
        let db_path = dir.path().join("test.db");
        let store = Store::open(&db_path).unwrap();

        store.insert_memory(&MemoryParams {
            title: "A", content: "Content A", memory_type: "knowledge",
            descriptors: "", salience: 0.5, content_hash: "", agent_id: "test",
        }).unwrap();
        store.insert_memory(&MemoryParams {
            title: "B", content: "Content B", memory_type: "episode",
            descriptors: "", salience: 0.5, content_hash: "", agent_id: "test",
        }).unwrap();
        store.insert_memory(&MemoryParams {
            title: "C", content: "Content C", memory_type: "identity",
            descriptors: "", salience: 1.0, content_hash: "", agent_id: "test",
        }).unwrap();

        let all = store.list_memories(None, false, 100).unwrap();
        assert_eq!(all.len(), 3);

        let knowledge = store.list_memories(Some("knowledge"), false, 100).unwrap();
        assert_eq!(knowledge.len(), 1);
        assert_eq!(knowledge[0].title, "A");
    }

    #[test]
    fn test_archive_memory() {
        let dir = TempDir::new().unwrap();
        let db_path = dir.path().join("test.db");
        let store = Store::open(&db_path).unwrap();

        let id = store.insert_memory(&MemoryParams {
            title: "Old", content: "Stale", memory_type: "episode",
            descriptors: "", salience: 0.5, content_hash: "", agent_id: "test",
        }).unwrap();

        let before = store.list_memories(None, false, 100).unwrap();
        assert_eq!(before.len(), 1);

        assert!(store.archive_memory(id).unwrap());
        // Archiving a non-existent ID returns false
        assert!(!store.archive_memory(99999).unwrap());

        let after = store.list_memories(None, false, 100).unwrap();
        assert_eq!(after.len(), 0);

        let with_archived = store.list_memories(None, true, 100).unwrap();
        assert_eq!(with_archived.len(), 1);
    }

    #[test]
    fn test_update_memory() {
        let dir = TempDir::new().unwrap();
        let db_path = dir.path().join("test.db");
        let store = Store::open(&db_path).unwrap();

        let id = store
            .insert_memory(&MemoryParams {
                title: "Original", content: "Old content", memory_type: "knowledge",
                descriptors: "tag1", salience: 0.5, content_hash: "hash1", agent_id: "test",
            })
            .unwrap();

        let updated = store
            .update_memory(id, &MemoryParams {
                title: "Updated", content: "New content", memory_type: "knowledge",
                descriptors: "tag1, tag2", salience: 0.5, content_hash: "hash2", agent_id: "test",
            })
            .unwrap();
        assert!(updated);

        let chunk = store.get_chunk(id).unwrap().unwrap();
        assert_eq!(chunk.title, "Updated");
        assert_eq!(chunk.content, "New content");
        assert_eq!(chunk.descriptors, "tag1, tag2");
        assert_eq!(chunk.content_hash, "hash2");
    }

    #[test]
    fn test_update_nonexistent_returns_false() {
        let dir = TempDir::new().unwrap();
        let db_path = dir.path().join("test.db");
        let store = Store::open(&db_path).unwrap();

        let updated = store.update_memory(999, &MemoryParams {
            title: "X", content: "Y", memory_type: "knowledge",
            descriptors: "", salience: 0.5, content_hash: "", agent_id: "test",
        }).unwrap();
        assert!(!updated);
    }

    #[test]
    fn test_fts_syncs_on_insert() {
        let dir = TempDir::new().unwrap();
        let db_path = dir.path().join("test.db");
        let store = Store::open(&db_path).unwrap();

        // insert_memory should auto-populate the FTS index
        store.insert_memory(&MemoryParams {
            title: "Bun preference", content: "Always use bun for running scripts",
            memory_type: "knowledge", descriptors: "tools",
            salience: 0.5, content_hash: "", agent_id: "test",
        }).unwrap();

        let count: i64 = store.conn().query_row(
            "SELECT COUNT(*) FROM chunks_fts WHERE chunks_fts MATCH 'bun'",
            [],
            |row| row.get(0),
        ).unwrap();
        assert_eq!(count, 1);
    }

    #[test]
    fn test_fts_syncs_on_update() {
        let dir = TempDir::new().unwrap();
        let db_path = dir.path().join("test.db");
        let store = Store::open(&db_path).unwrap();

        let id = store.insert_memory(&MemoryParams {
            title: "Original", content: "old content about bun",
            memory_type: "knowledge", descriptors: "",
            salience: 0.5, content_hash: "h1", agent_id: "test",
        }).unwrap();

        store.update_memory(id, &MemoryParams {
            title: "Updated", content: "new content about deno", memory_type: "knowledge",
            descriptors: "", salience: 0.5, content_hash: "h2", agent_id: "test",
        }).unwrap();

        // Old term gone from FTS
        let old: i64 = store.conn().query_row(
            "SELECT COUNT(*) FROM chunks_fts WHERE chunks_fts MATCH 'bun'",
            [], |row| row.get(0),
        ).unwrap();
        assert_eq!(old, 0);

        // New term present
        let new: i64 = store.conn().query_row(
            "SELECT COUNT(*) FROM chunks_fts WHERE chunks_fts MATCH 'deno'",
            [], |row| row.get(0),
        ).unwrap();
        assert_eq!(new, 1);
    }

    // --- Code indexing tests ---

    #[test]
    fn test_get_or_create_codebase() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        let id1 = store.get_or_create_codebase("/tmp/project", "project").unwrap();
        let id2 = store.get_or_create_codebase("/tmp/project", "project-renamed").unwrap();
        assert_eq!(id1, id2); // same path = same ID

        let id3 = store.get_or_create_codebase("/tmp/other", "other").unwrap();
        assert_ne!(id1, id3);
    }

    #[test]
    fn test_batch_upsert_chunks() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();
        let cb = store.get_or_create_codebase("/tmp/p", "p").unwrap();

        let fc = FileChunks {
            file_path: "src/main.rs".into(),
            file_hash: "aaa".into(),
            chunks: vec![
                CodeChunkParams {
                    chunk_key: "src/main.rs:function:hello:1:3".into(),
                    file_path: "src/main.rs".into(),
                    language: "rust".into(),
                    symbol_kind: "function_item".into(),
                    symbol_name: "hello".into(),
                    signature: "fn hello()".into(),
                    snippet: "fn hello() { println!(\"hi\"); }".into(),
                    start_line: 1, end_line: 3, file_hash: "aaa".into(),
                },
            ],
        };
        let count = store.batch_upsert_chunks(cb, &[fc]).unwrap();
        assert_eq!(count, 1);

        let (code, _) = store.count_by_kind().unwrap();
        assert_eq!(code, 1);

        // Verify indexed_files
        let hashes = store.get_all_file_hashes(cb).unwrap();
        assert_eq!(hashes.get("src/main.rs").unwrap(), "aaa");
    }

    #[test]
    fn test_batch_upsert_replaces_on_reindex() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();
        let cb = store.get_or_create_codebase("/tmp/p", "p").unwrap();

        let make_fc = |hash: &str, name: &str| FileChunks {
            file_path: "src/lib.rs".into(),
            file_hash: hash.into(),
            chunks: vec![CodeChunkParams {
                chunk_key: format!("src/lib.rs:function:{name}:1:5"),
                file_path: "src/lib.rs".into(),
                language: "rust".into(),
                symbol_kind: "function_item".into(),
                symbol_name: name.into(),
                signature: format!("fn {name}()"),
                snippet: format!("fn {name}() {{}}"),
                start_line: 1, end_line: 5, file_hash: hash.into(),
            }],
        };

        store.batch_upsert_chunks(cb, &[make_fc("v1", "old_fn")]).unwrap();
        store.batch_upsert_chunks(cb, &[make_fc("v2", "new_fn")]).unwrap();

        // Old chunk should be gone, new one present
        let (code, _) = store.count_by_kind().unwrap();
        assert_eq!(code, 1);

        let hashes = store.get_all_file_hashes(cb).unwrap();
        assert_eq!(hashes.get("src/lib.rs").unwrap(), "v2");
    }

    #[test]
    fn test_remove_stale_files() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();
        let cb = store.get_or_create_codebase("/tmp/p", "p").unwrap();

        let fc1 = FileChunks {
            file_path: "a.rs".into(), file_hash: "h1".into(),
            chunks: vec![CodeChunkParams {
                chunk_key: "a.rs:fn:f:1:2".into(), file_path: "a.rs".into(),
                language: "rust".into(), symbol_kind: "function_item".into(),
                symbol_name: "f".into(), signature: "fn f()".into(),
                snippet: "fn f() {}".into(), start_line: 1, end_line: 2, file_hash: "h1".into(),
            }],
        };
        let fc2 = FileChunks {
            file_path: "b.rs".into(), file_hash: "h2".into(),
            chunks: vec![CodeChunkParams {
                chunk_key: "b.rs:fn:g:1:2".into(), file_path: "b.rs".into(),
                language: "rust".into(), symbol_kind: "function_item".into(),
                symbol_name: "g".into(), signature: "fn g()".into(),
                snippet: "fn g() {}".into(), start_line: 1, end_line: 2, file_hash: "h2".into(),
            }],
        };
        store.batch_upsert_chunks(cb, &[fc1, fc2]).unwrap();
        assert_eq!(store.count_by_kind().unwrap().0, 2);

        // Only a.rs still active
        let active: HashSet<String> = ["a.rs".to_string()].into();
        let removed = store.remove_stale_files(cb, &active).unwrap();
        assert_eq!(removed, 1);
        assert_eq!(store.count_by_kind().unwrap().0, 1);
    }

    #[test]
    fn test_graph_edges() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();
        let cb = store.get_or_create_codebase("/tmp/p", "p").unwrap();

        let tags = vec![
            ferret::graph::Tag { symbol: "Store".into(), role: "definition".into(), kind: "struct".into(), line: 10 },
            ferret::graph::Tag { symbol: "Store".into(), role: "reference".into(), kind: "call".into(), line: 25 },
            ferret::graph::Tag { symbol: "open".into(), role: "definition".into(), kind: "function".into(), line: 14 },
        ];
        let count = store.upsert_graph_edges_for_file(cb, "src/store.rs", &tags).unwrap();
        assert_eq!(count, 3);

        let defs = store.find_definitions("Store", Some(cb)).unwrap();
        assert_eq!(defs.len(), 1);
        assert_eq!(defs[0].line, 10);

        let refs = store.find_references("Store", Some(cb)).unwrap();
        assert_eq!(refs.len(), 1);
        assert_eq!(refs[0].line, 25);
    }

    #[test]
    fn test_fts_code_expand() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();
        let cb = store.get_or_create_codebase("/tmp/p", "p").unwrap();

        let fc = FileChunks {
            file_path: "src/main.rs".into(), file_hash: "h".into(),
            chunks: vec![CodeChunkParams {
                chunk_key: "src/main.rs:fn:myFuncName:1:3".into(),
                file_path: "src/main.rs".into(), language: "rust".into(),
                symbol_kind: "function_item".into(), symbol_name: "myFuncName".into(),
                signature: "fn myFuncName()".into(), snippet: "fn myFuncName() {}".into(),
                start_line: 1, end_line: 3, file_hash: "h".into(),
            }],
        };
        store.batch_upsert_chunks(cb, &[fc]).unwrap();
        store.rebuild_fts_for_codebase(cb).unwrap();

        // code_expand should make camelCase searchable as separate words
        let hits = store.fts_search("func", None, 10).unwrap();
        assert!(!hits.is_empty(), "should find 'func' via code_expand of 'myFuncName'");
    }

    #[test]
    fn test_fts_search_kind_filter() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();
        let cb = store.get_or_create_codebase("/tmp/p", "p").unwrap();

        // Insert a code chunk
        let fc = FileChunks {
            file_path: "src/lib.rs".into(), file_hash: "h".into(),
            chunks: vec![CodeChunkParams {
                chunk_key: "src/lib.rs:fn:search:1:5".into(),
                file_path: "src/lib.rs".into(), language: "rust".into(),
                symbol_kind: "function_item".into(), symbol_name: "search".into(),
                signature: "fn search()".into(), snippet: "fn search() { query_database(); }".into(),
                start_line: 1, end_line: 5, file_hash: "h".into(),
            }],
        };
        store.batch_upsert_chunks(cb, &[fc]).unwrap();
        store.rebuild_fts_for_codebase(cb).unwrap();

        // Insert a memory
        store.insert_memory(&MemoryParams {
            title: "Search tips", content: "Use search with hybrid mode for best results",
            memory_type: "knowledge", descriptors: "", salience: 0.5,
            content_hash: "", agent_id: "test",
        }).unwrap();

        // All
        let all = store.fts_search("search", None, 10).unwrap();
        assert_eq!(all.len(), 2);

        // Code only
        let code = store.fts_search("search", Some("code"), 10).unwrap();
        assert_eq!(code.len(), 1);
        assert_eq!(code[0].kind, "code");

        // Memory only
        let mem = store.fts_search("search", Some("memory"), 10).unwrap();
        assert_eq!(mem.len(), 1);
        assert_eq!(mem[0].kind, "memory");
    }

    #[test]
    fn test_vector_search() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();
        let cb = store.get_or_create_codebase("/tmp/p", "p").unwrap();

        // Insert chunks
        let fc = FileChunks {
            file_path: "a.rs".into(), file_hash: "h".into(),
            chunks: vec![
                CodeChunkParams {
                    chunk_key: "a.rs:fn:a:1:2".into(), file_path: "a.rs".into(),
                    language: "rust".into(), symbol_kind: "fn".into(), symbol_name: "a".into(),
                    signature: "fn a()".into(), snippet: "fn a() {}".into(),
                    start_line: 1, end_line: 2, file_hash: "h".into(),
                },
                CodeChunkParams {
                    chunk_key: "a.rs:fn:b:3:4".into(), file_path: "a.rs".into(),
                    language: "rust".into(), symbol_kind: "fn".into(), symbol_name: "b".into(),
                    signature: "fn b()".into(), snippet: "fn b() {}".into(),
                    start_line: 3, end_line: 4, file_hash: "h".into(),
                },
            ],
        };
        store.batch_upsert_chunks(cb, &[fc]).unwrap();

        // Get chunk IDs
        let (code, _) = store.count_by_kind().unwrap();
        assert_eq!(code, 2);

        // Manually embed with simple vectors
        let emb_a: Vec<f32> = vec![1.0, 0.0, 0.0];
        let emb_b: Vec<f32> = vec![0.0, 1.0, 0.0];

        // Find the chunk IDs
        let chunk_a = store.conn().query_row(
            "SELECT id FROM chunks WHERE symbol_name = 'a'", [], |r| r.get::<_, i64>(0),
        ).unwrap();
        let chunk_b = store.conn().query_row(
            "SELECT id FROM chunks WHERE symbol_name = 'b'", [], |r| r.get::<_, i64>(0),
        ).unwrap();

        store.batch_upsert_embeddings(&[
            (chunk_a, &emb_a, "test-model"),
            (chunk_b, &emb_b, "test-model"),
        ]).unwrap();

        // Search near emb_a
        let query = vec![0.9, 0.1, 0.0];
        let results = store.vector_search(&query, "test-model", None, 10).unwrap();
        assert_eq!(results.len(), 2);
        assert_eq!(results[0].symbol_name.as_deref(), Some("a")); // closer to query
    }

    #[test]
    fn test_get_all_embeddings_and_hnsw_search() {
        let dir = TempDir::new().unwrap();
        let db_path = dir.path().join("test.db");
        let store = Store::open(&db_path).unwrap();
        let cb = store.get_or_create_codebase("/tmp/p", "p").unwrap();

        // Insert chunks with embeddings
        let fc = FileChunks {
            file_path: "a.rs".into(), file_hash: "h".into(),
            chunks: vec![
                CodeChunkParams {
                    chunk_key: "a.rs:fn:x:1:2".into(), file_path: "a.rs".into(),
                    language: "rust".into(), symbol_kind: "fn".into(), symbol_name: "x".into(),
                    signature: "fn x()".into(), snippet: "fn x() {}".into(),
                    start_line: 1, end_line: 2, file_hash: "h".into(),
                },
                CodeChunkParams {
                    chunk_key: "a.rs:fn:y:3:4".into(), file_path: "a.rs".into(),
                    language: "rust".into(), symbol_kind: "fn".into(), symbol_name: "y".into(),
                    signature: "fn y()".into(), snippet: "fn y() {}".into(),
                    start_line: 3, end_line: 4, file_hash: "h".into(),
                },
            ],
        };
        store.batch_upsert_chunks(cb, &[fc]).unwrap();

        let chunk_x = store.conn().query_row(
            "SELECT id FROM chunks WHERE symbol_name = 'x'", [], |r| r.get::<_, i64>(0),
        ).unwrap();
        let chunk_y = store.conn().query_row(
            "SELECT id FROM chunks WHERE symbol_name = 'y'", [], |r| r.get::<_, i64>(0),
        ).unwrap();

        // Use 10-dim embeddings (small for tests)
        let mut emb_x = vec![0.0f32; 10];
        emb_x[0] = 1.0;
        let mut emb_y = vec![0.0f32; 10];
        emb_y[1] = 1.0;

        store.batch_upsert_embeddings(&[
            (chunk_x, &emb_x, "test-model"),
            (chunk_y, &emb_y, "test-model"),
        ]).unwrap();

        // get_all_embeddings returns both
        let all = store.get_all_embeddings().unwrap();
        assert_eq!(all.len(), 2);

        // Build HNSW and search
        let hnsw = ferret::hnsw::HnswIndex::from_embeddings(&all).unwrap();
        assert_eq!(hnsw.len(), 2);

        // Query close to x
        let mut query = vec![0.0f32; 10];
        query[0] = 0.9;
        query[1] = 0.1;

        let results = store.vector_search_hnsw(&hnsw, &query, None, 10).unwrap();
        assert_eq!(results.len(), 2);
        assert_eq!(results[0].symbol_name.as_deref(), Some("x")); // closer to query

        // Test persistence
        let hnsw_path = ferret::hnsw::hnsw_path(&db_path);
        hnsw.save(&hnsw_path).unwrap();
        let loaded = ferret::hnsw::HnswIndex::load(&hnsw_path).unwrap();
        assert_eq!(loaded.len(), 2);

        let results2 = store.vector_search_hnsw(&loaded, &query, None, 10).unwrap();
        assert_eq!(results2[0].symbol_name.as_deref(), Some("x"));
    }

    fn test_hit(id: i64, title: &str, score: f64) -> SearchHit {
        SearchHit {
            id, kind: "code".into(), file_path: None, symbol_name: None,
            symbol_kind: None, signature: None, title: title.into(), snippet: String::new(),
            start_line: None, end_line: None, memory_type: None, score, reranker_score: None,
            access_count: 0, last_accessed: None, salience: 0.5,
            created_at: String::new(), archived: false, descriptors: String::new(),
        }
    }

    #[test]
    fn test_hybrid_search_rrf() {
        let fts = vec![test_hit(1, "A", 5.0), test_hit(2, "B", 3.0)];
        let vec_results = vec![test_hit(2, "B", 0.9), test_hit(3, "C", 0.8)];

        let merged = hybrid_search(&fts, &vec_results, 10);
        assert_eq!(merged.len(), 3);
        // ID 2 appears in both lists so should have highest RRF score
        assert_eq!(merged[0].id, 2);
    }

    #[test]
    fn test_stale_embeddings() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();
        let cb = store.get_or_create_codebase("/tmp/p", "p").unwrap();

        let fc = FileChunks {
            file_path: "a.rs".into(), file_hash: "h".into(),
            chunks: vec![CodeChunkParams {
                chunk_key: "a.rs:fn:f:1:2".into(), file_path: "a.rs".into(),
                language: "rust".into(), symbol_kind: "fn".into(), symbol_name: "f".into(),
                signature: "fn f()".into(), snippet: "fn f() {}".into(),
                start_line: 1, end_line: 2, file_hash: "h".into(),
            }],
        };
        store.batch_upsert_chunks(cb, &[fc]).unwrap();

        // All chunks should be stale (no embeddings yet)
        let stale = store.get_stale_embeddings(cb, "test-model").unwrap();
        assert_eq!(stale.len(), 1);
        assert_eq!(stale[0].symbol_name, "f");

        // Embed it
        let id = stale[0].id;
        store.batch_upsert_embeddings(&[(id, &[1.0_f32, 0.0, 0.0], "test-model")]).unwrap();

        // No longer stale
        let stale2 = store.get_stale_embeddings(cb, "test-model").unwrap();
        assert!(stale2.is_empty());

        // But stale for a different model
        let stale3 = store.get_stale_embeddings(cb, "other-model").unwrap();
        assert_eq!(stale3.len(), 1);
    }

    #[test]
    fn test_handoffs() {
        let dir = TempDir::new().unwrap();
        let db_path = dir.path().join("test.db");
        let store = Store::open(&db_path).unwrap();

        store.create_handoff("h1", "Finished Phase 0", "Start Phase 1", "grasshopper").unwrap();
        store.create_handoff("h2", "Finished Phase 1", "Start Phase 2", "grasshopper").unwrap();
        store.create_handoff("h3", "Fixed bug in ferret", "Deploy", "ferret").unwrap();

        let latest = store.get_latest_handoff(None).unwrap().unwrap();
        assert_eq!(latest.id, "h3");

        let gh_latest = store.get_latest_handoff(Some("grasshopper")).unwrap().unwrap();
        assert_eq!(gh_latest.id, "h2");
    }

    #[test]
    fn test_list_memories_by_access() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        let id1 = store.insert_memory(&MemoryParams {
            title: "Rarely used", content: "c1", memory_type: "knowledge",
            descriptors: "", salience: 0.5, content_hash: "", agent_id: "test",
        }).unwrap();
        let id2 = store.insert_memory(&MemoryParams {
            title: "Often used", content: "c2", memory_type: "knowledge",
            descriptors: "", salience: 0.5, content_hash: "", agent_id: "test",
        }).unwrap();

        store.touch_memory(id2).unwrap();
        store.touch_memory(id2).unwrap();
        store.touch_memory(id1).unwrap();

        let result = store.list_memories_by_access(10).unwrap();
        assert_eq!(result.len(), 2);
        assert_eq!(result[0].id, id2); // 2 accesses
        assert_eq!(result[1].id, id1); // 1 access
    }

    #[test]
    fn test_count_memories() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        let id = store.insert_memory(&MemoryParams {
            title: "A", content: "c", memory_type: "knowledge",
            descriptors: "", salience: 0.5, content_hash: "", agent_id: "test",
        }).unwrap();
        store.insert_memory(&MemoryParams {
            title: "B", content: "c", memory_type: "episode",
            descriptors: "", salience: 0.5, content_hash: "", agent_id: "test",
        }).unwrap();

        assert_eq!(store.count_memories(false).unwrap(), 2);
        assert_eq!(store.count_memories(true).unwrap(), 0);

        store.archive_memory(id).unwrap();
        assert_eq!(store.count_memories(false).unwrap(), 1);
        assert_eq!(store.count_memories(true).unwrap(), 1);
    }

    #[test]
    fn test_count_memories_by_type() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        store.insert_memory(&MemoryParams {
            title: "A", content: "c", memory_type: "knowledge",
            descriptors: "", salience: 0.5, content_hash: "", agent_id: "test",
        }).unwrap();
        store.insert_memory(&MemoryParams {
            title: "B", content: "c", memory_type: "knowledge",
            descriptors: "", salience: 0.5, content_hash: "", agent_id: "test",
        }).unwrap();
        store.insert_memory(&MemoryParams {
            title: "C", content: "c", memory_type: "identity",
            descriptors: "", salience: 1.0, content_hash: "", agent_id: "test",
        }).unwrap();

        let counts = store.count_memories_by_type().unwrap();
        assert_eq!(counts.get("knowledge"), Some(&2));
        assert_eq!(counts.get("identity"), Some(&1));
        assert_eq!(counts.get("episode"), None);
    }

    #[test]
    fn test_upsert_association() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        let id1 = store.insert_memory(&MemoryParams {
            title: "A", content: "c1", memory_type: "knowledge",
            descriptors: "", salience: 0.5, content_hash: "", agent_id: "test",
        }).unwrap();
        let id2 = store.insert_memory(&MemoryParams {
            title: "B", content: "c2", memory_type: "knowledge",
            descriptors: "", salience: 0.5, content_hash: "", agent_id: "test",
        }).unwrap();

        // First upsert — creates bidirectional edges
        store.upsert_association(id1, id2).unwrap();
        let count: i64 = store.conn().query_row(
            "SELECT COUNT(*) FROM graph WHERE role = 'associates'", [], |r| r.get(0),
        ).unwrap();
        assert_eq!(count, 2);

        // Second upsert — idempotent
        store.upsert_association(id1, id2).unwrap();
        let count2: i64 = store.conn().query_row(
            "SELECT COUNT(*) FROM graph WHERE role = 'associates'", [], |r| r.get(0),
        ).unwrap();
        assert_eq!(count2, 2);
    }

    #[test]
    fn test_search_similar_memories() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        let id1 = store.insert_memory(&MemoryParams {
            title: "A", content: "c1", memory_type: "knowledge",
            descriptors: "", salience: 0.5, content_hash: "", agent_id: "test",
        }).unwrap();
        let id2 = store.insert_memory(&MemoryParams {
            title: "B", content: "c2", memory_type: "knowledge",
            descriptors: "", salience: 0.5, content_hash: "", agent_id: "test",
        }).unwrap();

        // Embed both
        let emb1: Vec<f32> = vec![1.0, 0.0, 0.0];
        let emb2: Vec<f32> = vec![0.9, 0.1, 0.0]; // similar to emb1
        store.batch_upsert_embeddings(&[(id1, &emb1, "test-model"), (id2, &emb2, "test-model")]).unwrap();

        // Search near emb1
        let query: Vec<f32> = vec![1.0, 0.0, 0.0];
        let results = store.search_similar_memories(&query, "test-model", 0.5, 10).unwrap();
        assert_eq!(results.len(), 2);
        assert_eq!(results[0].0, id1); // exact match first

        // High threshold should filter
        let strict = store.search_similar_memories(&query, "test-model", 0.95, 10).unwrap();
        assert_eq!(strict.len(), 1);
        assert_eq!(strict[0].0, id1);
    }

    #[test]
    fn test_list_recent_handoffs() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        store.create_handoff("h1", "S1", "N1", "p1").unwrap();
        store.create_handoff("h2", "S2", "N2", "p2").unwrap();
        store.create_handoff("h3", "S3", "N3", "p1").unwrap();

        let recent = store.list_recent_handoffs(2).unwrap();
        assert_eq!(recent.len(), 2);
        assert_eq!(recent[0].id, "h3"); // most recent first
    }

    #[test]
    fn test_list_memories_with_associations() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        let id1 = store.insert_memory(&MemoryParams {
            title: "A", content: "c1", memory_type: "knowledge",
            descriptors: "", salience: 0.5, content_hash: "", agent_id: "test",
        }).unwrap();
        let id2 = store.insert_memory(&MemoryParams {
            title: "B", content: "c2", memory_type: "knowledge",
            descriptors: "", salience: 0.5, content_hash: "", agent_id: "test",
        }).unwrap();
        let id3 = store.insert_memory(&MemoryParams {
            title: "C", content: "c3", memory_type: "knowledge",
            descriptors: "", salience: 0.5, content_hash: "", agent_id: "test",
        }).unwrap();

        // id1 <-> id2, id1 <-> id3 (id1 has 2 associations, id2 and id3 have 1 each)
        store.upsert_association(id1, id2).unwrap();
        store.upsert_association(id1, id3).unwrap();

        let connected = store.list_memories_with_associations(10).unwrap();
        assert!(!connected.is_empty());
        // id1 should have highest count (bidirectional: 2 outgoing + 2 incoming edges)
        assert_eq!(connected[0].0.id, id1);
        assert!(connected[0].1 >= 2);
    }

    #[test]
    fn test_fts_search_includes_cognitive_fields() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        store.insert_memory(&MemoryParams {
            title: "Bun preference", content: "Always use bun",
            memory_type: "knowledge", descriptors: "tools",
            salience: 0.7, content_hash: "", agent_id: "test",
        }).unwrap();

        let hits = store.fts_search("bun", Some("memory"), 10).unwrap();
        assert_eq!(hits.len(), 1);
        assert_eq!(hits[0].access_count, 0);
        assert_eq!(hits[0].salience, 0.7);
        assert!(!hits[0].created_at.is_empty());
        assert!(!hits[0].archived);
        assert_eq!(hits[0].descriptors, "tools");
    }

    #[test]
    fn test_touch_creates_access_log() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        let id = store.insert_memory(&MemoryParams {
            title: "Test", content: "test content",
            memory_type: "knowledge", descriptors: "",
            salience: 0.5, content_hash: "", agent_id: "test",
        }).unwrap();

        store.touch_memory(id).unwrap();
        store.touch_memory(id).unwrap();

        let count: i64 = store.conn.query_row(
            "SELECT COUNT(*) FROM access_log WHERE chunk_id = ?1",
            params![id], |row| row.get(0),
        ).unwrap();
        assert_eq!(count, 2);
    }

    #[test]
    fn test_get_access_intervals() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        let id = store.insert_memory(&MemoryParams {
            title: "Test", content: "test content",
            memory_type: "knowledge", descriptors: "",
            salience: 0.5, content_hash: "", agent_id: "test",
        }).unwrap();

        // No accesses → empty intervals
        let intervals = store.get_access_intervals(id).unwrap();
        assert!(intervals.is_empty());

        // 1 access → still empty (need 2+ for intervals)
        store.touch_memory(id).unwrap();
        let intervals = store.get_access_intervals(id).unwrap();
        assert!(intervals.is_empty());

        // 2 accesses → 1 interval
        std::thread::sleep(std::time::Duration::from_millis(10));
        store.touch_memory(id).unwrap();
        let intervals = store.get_access_intervals(id).unwrap();
        assert_eq!(intervals.len(), 1);
        assert!(intervals[0] > 0.0);
    }

    #[test]
    fn test_upsert_entity_edges() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        let id = store.insert_memory(&MemoryParams {
            title: "Test", content: "test",
            memory_type: "knowledge", descriptors: "",
            salience: 0.5, content_hash: "", agent_id: "test",
        }).unwrap();

        let entities = vec![
            ("SearchHit".to_string(), "identifier".to_string()),
            ("/home/foo.rs".to_string(), "file_path".to_string()),
        ];
        let count = store.upsert_entity_edges(id, &entities).unwrap();
        assert_eq!(count, 2);

        // Idempotent — second insert returns 0
        let count = store.upsert_entity_edges(id, &entities).unwrap();
        assert_eq!(count, 0);
    }

    #[test]
    fn test_find_memories_by_entity() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        let id1 = store.insert_memory(&MemoryParams {
            title: "Memory A", content: "about foo",
            memory_type: "knowledge", descriptors: "",
            salience: 0.5, content_hash: "", agent_id: "test",
        }).unwrap();
        let id2 = store.insert_memory(&MemoryParams {
            title: "Memory B", content: "about bar",
            memory_type: "knowledge", descriptors: "",
            salience: 0.5, content_hash: "", agent_id: "test",
        }).unwrap();

        store.upsert_entity_edges(id1, &[("SearchHit".into(), "identifier".into())]).unwrap();
        store.upsert_entity_edges(id2, &[("SearchHit".into(), "identifier".into())]).unwrap();

        let hits = store.find_memories_by_entity("SearchHit").unwrap();
        assert_eq!(hits.len(), 2);

        let hits = store.find_memories_by_entity("NonExistent").unwrap();
        assert!(hits.is_empty());
    }

    #[test]
    fn test_insert_and_query_summary_edges() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        let id1 = store.insert_memory(&MemoryParams {
            title: "M1", content: "c1", memory_type: "episode",
            descriptors: "", salience: 0.5, content_hash: "", agent_id: "test",
        }).unwrap();
        let id2 = store.insert_memory(&MemoryParams {
            title: "M2", content: "c2", memory_type: "episode",
            descriptors: "", salience: 0.5, content_hash: "", agent_id: "test",
        }).unwrap();
        let summary_id = store.insert_memory(&MemoryParams {
            title: "Summary", content: "summary text", memory_type: "knowledge",
            descriptors: "summary", salience: 0.5, content_hash: "sum_hash", agent_id: "test",
        }).unwrap();

        store.insert_summary_edges(summary_id, &[id1, id2]).unwrap();

        let summaries = store.get_summaries_for_chunks(&[id1, id2]).unwrap();
        assert_eq!(summaries.len(), 1);
        assert_eq!(summaries[0], summary_id);

        // Partial match should NOT find the summary (exact-set match required)
        let summaries = store.get_summaries_for_chunks(&[id1]).unwrap();
        assert_eq!(summaries.len(), 0, "partial match should not find summary with exact-set matching");
    }

    #[test]
    fn test_log_retrieval() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        let log_id = store.log_retrieval("test query", "recall", &[(1, 0.9), (2, 0.5)], Some(42)).unwrap();
        assert!(log_id > 0);

        let count: i64 = store.conn.query_row(
            "SELECT COUNT(*) FROM retrieval_log", [], |row| row.get(0),
        ).unwrap();
        assert_eq!(count, 1);
    }

    #[test]
    fn test_log_feedback() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        let id = store.insert_memory(&MemoryParams {
            title: "Test", content: "test content",
            memory_type: "knowledge", descriptors: "",
            salience: 0.5, content_hash: "", agent_id: "test",
        }).unwrap();

        let log_id = store.log_retrieval("test", "recall", &[(id, 0.9)], None).unwrap();
        store.log_feedback(log_id, id, "positive").unwrap();

        let count: i64 = store.conn.query_row(
            "SELECT COUNT(*) FROM feedback", [], |row| row.get(0),
        ).unwrap();
        assert_eq!(count, 1);
    }

    #[test]
    fn test_adjust_salience_clamp() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        let id = store.insert_memory(&MemoryParams {
            title: "Test", content: "test",
            memory_type: "knowledge", descriptors: "",
            salience: 0.9, content_hash: "", agent_id: "test",
        }).unwrap();

        // Bump up — should clamp at 1.0
        store.adjust_salience(id, 0.5).unwrap();
        let sal: f64 = store.conn.query_row(
            "SELECT salience FROM chunks WHERE id = ?1", params![id], |row| row.get(0),
        ).unwrap();
        assert!((sal - 1.0).abs() < 1e-10, "should clamp at 1.0: {sal}");

        // Reduce below 0 — should clamp at 0.0
        store.adjust_salience(id, -2.0).unwrap();
        let sal: f64 = store.conn.query_row(
            "SELECT salience FROM chunks WHERE id = ?1", params![id], |row| row.get(0),
        ).unwrap();
        assert!((sal - 0.0).abs() < 1e-10, "should clamp at 0.0: {sal}");
    }

    #[test]
    fn test_export_training_data_empty() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();
        let examples = store.export_training_data().unwrap();
        assert!(examples.is_empty());
    }

    #[test]
    fn test_export_training_data_with_feedback() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        let id1 = store.insert_memory(&MemoryParams {
            title: "Good", content: "good content",
            memory_type: "knowledge", descriptors: "",
            salience: 0.5, content_hash: "h1", agent_id: "test",
        }).unwrap();
        let id2 = store.insert_memory(&MemoryParams {
            title: "Bad", content: "bad content",
            memory_type: "knowledge", descriptors: "",
            salience: 0.5, content_hash: "h2", agent_id: "test",
        }).unwrap();

        let log_id = store.log_retrieval("test query", "recall", &[(id1, 0.9), (id2, 0.5)], None).unwrap();
        store.log_feedback(log_id, id1, "positive").unwrap();
        store.log_feedback(log_id, id2, "negative").unwrap();

        let examples = store.export_training_data().unwrap();
        assert_eq!(examples.len(), 1);
        assert_eq!(examples[0]["query"], "test query");
        assert_eq!(examples[0]["positive"].as_array().unwrap().len(), 1);
        assert_eq!(examples[0]["negative"].as_array().unwrap().len(), 1);
    }

    #[test]
    fn test_validate_feedback_rejects_invalid_chunk() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        let id1 = store.insert_memory(&MemoryParams {
            title: "M1", content: "c1", memory_type: "knowledge",
            descriptors: "", salience: 0.5, content_hash: "vf1", agent_id: "test",
        }).unwrap();
        let id2 = store.insert_memory(&MemoryParams {
            title: "M2", content: "c2", memory_type: "knowledge",
            descriptors: "", salience: 0.5, content_hash: "vf2", agent_id: "test",
        }).unwrap();

        // Log retrieval with only id1 in results
        let log_id = store.log_retrieval("q", "recall", &[(id1, 0.9)], None).unwrap();

        // id1 should validate
        assert!(store.validate_feedback(log_id, id1).unwrap());
        // id2 was NOT in the retrieval results — must be rejected
        assert!(!store.validate_feedback(log_id, id2).unwrap());
        // Non-existent chunk should also be rejected
        assert!(!store.validate_feedback(log_id, 99999).unwrap());
    }

    #[test]
    fn test_validate_feedback_bad_log_id() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();
        // Non-existent log_id should error
        assert!(store.validate_feedback(99999, 1).is_err());
    }

    #[test]
    fn test_summary_dedup_superset_not_matched() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        let id1 = store.insert_memory(&MemoryParams {
            title: "A", content: "a", memory_type: "episode",
            descriptors: "", salience: 0.5, content_hash: "sa", agent_id: "test",
        }).unwrap();
        let id2 = store.insert_memory(&MemoryParams {
            title: "B", content: "b", memory_type: "episode",
            descriptors: "", salience: 0.5, content_hash: "sb", agent_id: "test",
        }).unwrap();
        let id3 = store.insert_memory(&MemoryParams {
            title: "C", content: "c", memory_type: "episode",
            descriptors: "", salience: 0.5, content_hash: "sc", agent_id: "test",
        }).unwrap();

        // Summary covers [A, B, C]
        let summary_id = store.insert_memory(&MemoryParams {
            title: "Summary ABC", content: "summary", memory_type: "knowledge",
            descriptors: "summary", salience: 0.5, content_hash: "sum_abc", agent_id: "test",
        }).unwrap();
        store.insert_summary_edges(summary_id, &[id1, id2, id3]).unwrap();

        // Exact match [A, B, C] → found
        let found = store.get_summaries_for_chunks(&[id1, id2, id3]).unwrap();
        assert_eq!(found.len(), 1, "exact match should find summary");

        // Subset [A, B] → NOT found (summary has 3 members, query has 2)
        let found = store.get_summaries_for_chunks(&[id1, id2]).unwrap();
        assert_eq!(found.len(), 0, "subset should not match — summary covers more members");

        // Superset [A, B, C, D-nonexistent] → NOT found
        let found = store.get_summaries_for_chunks(&[id1, id2, id3, 99999]).unwrap();
        assert_eq!(found.len(), 0, "superset should not match — query has more members than summary");
    }

    #[test]
    fn test_delete_entity_edges() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        let id = store.insert_memory(&MemoryParams {
            title: "Test", content: "test", memory_type: "knowledge",
            descriptors: "", salience: 0.5, content_hash: "de1", agent_id: "test",
        }).unwrap();

        // Insert entity edges
        let entities = vec![
            ("foo.rs".to_string(), "file_path".to_string()),
            ("SearchHit".to_string(), "identifier".to_string()),
        ];
        store.upsert_entity_edges(id, &entities).unwrap();
        assert_eq!(store.find_memories_by_entity("foo.rs").unwrap().len(), 1);
        assert_eq!(store.find_memories_by_entity("SearchHit").unwrap().len(), 1);

        // Delete entity edges
        let deleted = store.delete_entity_edges(id).unwrap();
        assert_eq!(deleted, 2);

        // Verify they're gone
        assert_eq!(store.find_memories_by_entity("foo.rs").unwrap().len(), 0);
        assert_eq!(store.find_memories_by_entity("SearchHit").unwrap().len(), 0);
    }

    #[test]
    fn test_insert_memory_nestable_in_savepoint() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        // Simulate outer transaction wrapping multiple inserts
        store.execute_batch("SAVEPOINT outer").unwrap();
        let id1 = store.insert_memory(&MemoryParams {
            title: "Nested1", content: "c1", memory_type: "knowledge",
            descriptors: "", salience: 0.5, content_hash: "nest1", agent_id: "test",
        }).unwrap();
        let id2 = store.insert_memory(&MemoryParams {
            title: "Nested2", content: "c2", memory_type: "knowledge",
            descriptors: "", salience: 0.5, content_hash: "nest2", agent_id: "test",
        }).unwrap();
        store.execute_batch("RELEASE outer").unwrap();

        // Both should exist
        assert!(store.get_chunk(id1).unwrap().is_some());
        assert!(store.get_chunk(id2).unwrap().is_some());
    }

    #[test]
    fn test_insert_memory_rollback_in_savepoint() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        // Outer savepoint with rollback should undo nested insert_memory
        store.execute_batch("SAVEPOINT outer").unwrap();
        let _id = store.insert_memory(&MemoryParams {
            title: "WillRollback", content: "c", memory_type: "knowledge",
            descriptors: "", salience: 0.5, content_hash: "rb1", agent_id: "test",
        }).unwrap();
        store.execute_batch("ROLLBACK TO outer").unwrap();
        store.execute_batch("RELEASE outer").unwrap();

        // Memory count should be 0 — the insert was rolled back
        let (_, mem_count) = store.count_by_kind().unwrap();
        assert_eq!(mem_count, 0, "insert_memory should be rollbackable from outer savepoint");
    }

    #[test]
    fn test_composite_recall_index_exists() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();
        let exists: bool = store.conn.query_row(
            "SELECT COUNT(*) > 0 FROM sqlite_master WHERE type='index' AND name='idx_chunks_recall_filter'",
            [],
            |row| row.get(0),
        ).unwrap();
        assert!(exists, "composite recall filter index should exist");
    }

    #[test]
    fn test_maintenance_prunes_old_logs() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        // Insert a memory and create old log entries
        let id = store.insert_memory(&MemoryParams {
            title: "M", content: "c", memory_type: "knowledge",
            descriptors: "", salience: 0.5, content_hash: "maint1", agent_id: "test",
        }).unwrap();

        // Insert old retrieval log (200 days ago)
        let old_date = (chrono::Utc::now() - chrono::Duration::days(200)).to_rfc3339();
        store.conn.execute(
            "INSERT INTO retrieval_log (query, tool, result_ids, scores, result_count, created_at) VALUES ('q', 'recall', '[]', '[]', 0, ?1)",
            params![old_date],
        ).unwrap();

        // Insert old access log
        store.conn.execute(
            "INSERT INTO access_log (chunk_id, accessed_at) VALUES (?1, ?2)",
            params![id, old_date],
        ).unwrap();

        // Insert recent retrieval log (today)
        let log_id = store.log_retrieval("q", "recall", &[(id, 0.9)], None).unwrap();

        // Insert old feedback
        store.conn.execute(
            "INSERT INTO feedback (retrieval_log_id, chunk_id, signal, created_at) VALUES (?1, ?2, 'positive', ?3)",
            params![log_id, id, old_date],
        ).unwrap();

        let report = store.maintenance(90).unwrap();
        assert_eq!(report.retrieval_logs_pruned, 1, "should prune 1 old retrieval log");
        assert_eq!(report.access_log_pruned, 1, "should prune 1 old access log");
        assert_eq!(report.feedback_pruned, 1, "should prune 1 old feedback");

        // Recent retrieval log should still exist
        let count: i64 = store.conn.query_row(
            "SELECT COUNT(*) FROM retrieval_log", [], |row| row.get(0),
        ).unwrap();
        assert_eq!(count, 1, "recent retrieval log should survive maintenance");
    }

    #[test]
    fn test_maintenance_clamps_negative_retention() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        // Insert a recent retrieval log (today)
        let id = store.insert_memory(&MemoryParams {
            title: "M", content: "c", memory_type: "knowledge",
            descriptors: "", salience: 0.5, content_hash: "clamp1", agent_id: "test",
        }).unwrap();
        store.log_retrieval("q", "recall", &[(id, 0.9)], None).unwrap();

        // Negative retention should be clamped to 1, not delete everything
        let report = store.maintenance(-5).unwrap();
        assert_eq!(report.retrieval_logs_pruned, 0, "negative retention should not delete recent logs");
    }

    #[test]
    fn test_vacuum_on_fresh_db() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();
        store.vacuum().unwrap(); // Should not error
    }

    #[test]
    fn test_db_size_bytes() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();
        let size = store.db_size_bytes().unwrap();
        assert!(size > 0, "database should have non-zero size");
    }

    #[test]
    fn test_count_helpers_empty() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();
        assert_eq!(store.count_retrieval_logs().unwrap(), 0);
        assert_eq!(store.count_feedback().unwrap(), 0);
        assert_eq!(store.count_access_log().unwrap(), 0);
        assert_eq!(store.count_codebases().unwrap(), 0);
        assert!(store.latest_handoff_timestamp().unwrap().is_none());
    }

    #[test]
    fn test_count_helpers_populated() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        let id = store.insert_memory(&MemoryParams {
            title: "T", content: "c", memory_type: "knowledge",
            descriptors: "", salience: 0.5, content_hash: "ch1", agent_id: "test",
        }).unwrap();

        store.log_retrieval("q", "recall", &[(id, 0.9)], None).unwrap();
        store.touch_memory(id).unwrap(); // creates access_log entry

        assert_eq!(store.count_retrieval_logs().unwrap(), 1);
        assert_eq!(store.count_access_log().unwrap(), 1);
    }

    #[test]
    fn test_latest_handoff_timestamp() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        assert!(store.latest_handoff_timestamp().unwrap().is_none());

        store.create_handoff("test-id", "summary", "next steps", "test-project").unwrap();
        let ts = store.latest_handoff_timestamp().unwrap();
        assert!(ts.is_some(), "should have a handoff timestamp");
    }
}
