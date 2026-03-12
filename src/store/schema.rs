use anyhow::{Context, Result};
use rusqlite::Connection;
use rusqlite::functions::FunctionFlags;
use std::path::Path;

/// Unified data store for code intelligence + cognitive memory.
/// Single SQLite database with WAL mode, FTS5 keyword search,
/// and shared schema for code chunks and memory entries.
pub struct Store {
    pub(crate) conn: Connection,
}

impl Store {
    pub fn open(path: &Path) -> Result<Self> {
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent)
                .with_context(|| format!("creating directory for {}", path.display()))?;
            // Restrict directory permissions on Unix (user-only access)
            #[cfg(unix)]
            {
                use std::os::unix::fs::PermissionsExt;
                let _ = std::fs::set_permissions(parent, std::fs::Permissions::from_mode(0o700));
            }
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
                Ok(crate::code::tokenizer::expand_code_tokens(&text))
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
                -- Deprecated: access_count is no longer incremented. Salience is the sole
                -- retrieval signal. Column kept for backward compatibility (migration cost > benefit).
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

            -- FTS5 keyword search across all chunks (code + memory)
            CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
                title,
                content,
                snippet,
                symbol_name,
                descriptors,
                tokenize='porter unicode61'
            );

",
        )?;
        Ok(())
    }

    /// Raw connection access for tests and benchmarks.
    pub fn conn(&self) -> &Connection {
        &self.conn
    }
}
