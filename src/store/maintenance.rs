use anyhow::Result;

use super::schema::Store;
use super::types::log_and_skip;

/// Per-codebase statistics.
pub struct CodebaseInfo {
    pub id: i64,
    pub name: String,
    pub root_path: String,
    pub chunk_count: i64,
    pub file_count: i64,
    pub embedded_count: i64,
    pub indexed_at: i64,
}

/// Memory statistics grouped by type.
pub struct MemoryStats {
    pub total: i64,
    pub by_type: Vec<(String, i64)>,
    pub archived: i64,
    pub with_embeddings: i64,
    pub avg_salience: f64,
}

impl Store {
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

    /// Get database file size in bytes via PRAGMA.
    pub fn db_size_bytes(&self) -> Result<i64> {
        let page_count: i64 = self.conn.query_row("PRAGMA page_count", [], |r| r.get(0))?;
        let page_size: i64 = self.conn.query_row("PRAGMA page_size", [], |r| r.get(0))?;
        Ok(page_count * page_size)
    }

    /// Count indexed codebases.
    pub fn count_codebases(&self) -> Result<i64> {
        self.conn
            .query_row("SELECT COUNT(*) FROM codebases", [], |r| r.get(0))
            .map_err(Into::into)
    }

    /// Detailed per-codebase statistics.
    pub fn codebase_stats(&self) -> Result<Vec<CodebaseInfo>> {
        let mut stmt = self.conn.prepare(
            "SELECT c.id, c.name, c.root_path, c.indexed_at,
                    COALESCE(ch.cnt, 0),
                    COALESCE(f.cnt, 0),
                    COALESCE(e.cnt, 0)
             FROM codebases c
             LEFT JOIN (SELECT codebase_id, COUNT(*) as cnt FROM chunks WHERE kind='code' GROUP BY codebase_id) ch
                ON ch.codebase_id = c.id
             LEFT JOIN (SELECT codebase_id, COUNT(*) as cnt FROM indexed_files GROUP BY codebase_id) f
                ON f.codebase_id = c.id
             LEFT JOIN (SELECT codebase_id, COUNT(*) as cnt FROM chunks WHERE kind='code' AND embedding IS NOT NULL GROUP BY codebase_id) e
                ON e.codebase_id = c.id
             ORDER BY c.name",
        )?;
        let rows = stmt.query_map([], |row| {
            Ok(CodebaseInfo {
                id: row.get(0)?,
                name: row.get(1)?,
                root_path: row.get(2)?,
                indexed_at: row.get(3)?,
                chunk_count: row.get(4)?,
                file_count: row.get(5)?,
                embedded_count: row.get(6)?,
            })
        })?;
        Ok(rows.filter_map(log_and_skip("maintenance_stats")).collect())
    }

    /// Memory statistics by type.
    pub fn memory_stats(&self) -> Result<MemoryStats> {
        let total: i64 = self.conn.query_row(
            "SELECT COUNT(*) FROM chunks WHERE kind = 'memory' AND archived = 0",
            [],
            |r| r.get(0),
        )?;
        let archived: i64 = self.conn.query_row(
            "SELECT COUNT(*) FROM chunks WHERE kind = 'memory' AND archived = 1",
            [],
            |r| r.get(0),
        )?;
        let with_embeddings: i64 = self.conn.query_row(
            "SELECT COUNT(*) FROM chunks WHERE kind = 'memory' AND embedding IS NOT NULL",
            [],
            |r| r.get(0),
        )?;
        let avg_salience: f64 = self.conn.query_row(
            "SELECT COALESCE(AVG(salience), 0.0) FROM chunks WHERE kind = 'memory' AND archived = 0",
            [],
            |r| r.get(0),
        )?;

        let mut stmt = self.conn.prepare(
            "SELECT COALESCE(memory_type, 'unknown'), COUNT(*)
             FROM chunks WHERE kind = 'memory' AND archived = 0
             GROUP BY memory_type ORDER BY COUNT(*) DESC",
        )?;
        let by_type: Vec<(String, i64)> = stmt
            .query_map([], |row| Ok((row.get(0)?, row.get(1)?)))?
            .filter_map(log_and_skip("maintenance_stats"))
            .collect();

        Ok(MemoryStats {
            total,
            by_type,
            archived,
            with_embeddings,
            avg_salience,
        })
    }

    /// Count total embedded chunks (code + memory).
    pub fn count_embedded(&self) -> Result<i64> {
        self.conn
            .query_row(
                "SELECT COUNT(*) FROM chunks WHERE embedding IS NOT NULL",
                [],
                |r| r.get(0),
            )
            .map_err(Into::into)
    }

    /// Run PRAGMA optimize for query planner stats.
    pub fn optimize(&self) -> Result<()> {
        self.conn.execute_batch("PRAGMA optimize;")?;
        Ok(())
    }

    /// Rebuild the database file (slow — use sparingly).
    pub fn vacuum(&self) -> Result<()> {
        self.conn.execute_batch("VACUUM;")?;
        Ok(())
    }

    /// Run integrity check. Returns "ok" on success or error details.
    pub fn integrity_check(&self) -> Result<String> {
        let result: String = self.conn.query_row(
            "PRAGMA integrity_check",
            [],
            |r| r.get(0),
        )?;
        Ok(result)
    }

    /// Check FTS index integrity. Returns true if healthy.
    pub fn fts_integrity_check(&self) -> Result<bool> {
        match self.conn.execute_batch(
            "INSERT INTO chunks_fts(chunks_fts) VALUES('integrity-check')",
        ) {
            Ok(_) => Ok(true),
            Err(_) => Ok(false),
        }
    }
}
