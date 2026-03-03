use anyhow::{Context, Result};
use rusqlite::functions::FunctionFlags;
use rusqlite::{params, Connection};
use std::path::Path;

/// Unified data store for code intelligence + cognitive memory.
/// Single SQLite database with WAL mode, FTS5 keyword search,
/// and shared schema for code chunks and memory entries.
pub struct Store {
    conn: Connection,
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
             PRAGMA cache_size = -64000;",
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
                chunk_key       TEXT UNIQUE,          -- dedup key for code chunks
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
            CREATE INDEX IF NOT EXISTS idx_chunks_content_hash
                ON chunks(content_hash) WHERE content_hash != '';

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
                strength        REAL NOT NULL DEFAULT 1.0,

                UNIQUE(codebase_id, file_path, role, kind, line)
            );

            CREATE INDEX IF NOT EXISTS idx_graph_symbol
                ON graph(codebase_id, symbol) WHERE symbol IS NOT NULL;
            CREATE INDEX IF NOT EXISTS idx_graph_source
                ON graph(source_chunk) WHERE source_chunk IS NOT NULL;
            CREATE INDEX IF NOT EXISTS idx_graph_target
                ON graph(target_chunk) WHERE target_chunk IS NOT NULL;

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
    pub fn insert_memory(
        &self,
        title: &str,
        content: &str,
        memory_type: &str,
        descriptors: &str,
        salience: f64,
        content_hash: &str,
        agent_id: &str,
    ) -> Result<i64> {
        let now = chrono::Utc::now().to_rfc3339();
        self.conn.execute(
            "INSERT INTO chunks (kind, title, content, memory_type, descriptors, salience,
                                 content_hash, agent_id, created_at, updated_at)
             VALUES ('memory', ?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?8)",
            params![title, content, memory_type, descriptors, salience, content_hash, agent_id, now],
        )?;
        Ok(self.conn.last_insert_rowid())
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
        let mut sql = String::from(
            "SELECT id, kind, title, content, snippet, symbol_name, symbol_kind, signature,
                    file_path, language, start_line, end_line, memory_type,
                    descriptors, source, access_count, last_accessed, salience, archived,
                    content_hash, agent_id, created_at, updated_at, codebase_id
             FROM chunks WHERE kind = 'memory'",
        );

        if !include_archived {
            sql.push_str(" AND archived = 0");
        }
        if memory_type.is_some() {
            sql.push_str(" AND memory_type = ?1");
        }
        sql.push_str(" ORDER BY created_at DESC LIMIT ?2");

        let mut stmt = self.conn.prepare(&sql)?;

        let rows = if let Some(mt) = memory_type {
            stmt.query_map(params![mt, limit as i64], row_to_chunk)?
        } else {
            // When no memory_type filter, ?1 is not used — shift limit to ?1
            let adjusted = sql.replace("?2", "?1");
            drop(stmt);
            let mut stmt2 = self.conn.prepare(&adjusted)?;
            let results: Vec<Chunk> = stmt2
                .query_map(params![limit as i64], row_to_chunk)?
                .filter_map(|r| r.ok())
                .collect();
            return Ok(results);
        };

        Ok(rows.filter_map(|r| r.ok()).collect())
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
        Ok(())
    }

    /// Archive a memory (soft delete).
    pub fn archive_memory(&self, id: i64) -> Result<()> {
        let now = chrono::Utc::now().to_rfc3339();
        self.conn.execute(
            "UPDATE chunks SET archived = 1, updated_at = ?1 WHERE id = ?2",
            params![now, id],
        )?;
        Ok(())
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

    /// Raw connection access for advanced operations (FTS setup, etc.)
    pub fn conn(&self) -> &Connection {
        &self.conn
    }
}

// --- Data types ---

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
            .insert_memory(
                "Test preference",
                "Always use bun for running scripts",
                "knowledge",
                "tools, preferences",
                0.5,
                "abc123",
                "claude-code",
            )
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
            .insert_memory("Test", "Content", "knowledge", "", 0.5, "", "test")
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

        store.insert_memory("A", "Content A", "knowledge", "", 0.5, "", "test").unwrap();
        store.insert_memory("B", "Content B", "episode", "", 0.5, "", "test").unwrap();
        store.insert_memory("C", "Content C", "identity", "", 1.0, "", "test").unwrap();

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

        let id = store.insert_memory("Old", "Stale", "episode", "", 0.5, "", "test").unwrap();

        let before = store.list_memories(None, false, 100).unwrap();
        assert_eq!(before.len(), 1);

        store.archive_memory(id).unwrap();

        let after = store.list_memories(None, false, 100).unwrap();
        assert_eq!(after.len(), 0);

        let with_archived = store.list_memories(None, true, 100).unwrap();
        assert_eq!(with_archived.len(), 1);
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
}
