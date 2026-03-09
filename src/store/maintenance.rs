use anyhow::Result;

use super::schema::Store;

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
}
