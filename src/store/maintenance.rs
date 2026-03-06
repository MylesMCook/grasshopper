use anyhow::Result;
use rusqlite::params;

use super::schema::Store;
use super::types::MaintenanceReport;

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

    /// Log a retrieval event. Returns the log entry ID.
    pub fn log_retrieval(
        &self,
        query: &str,
        tool: &str,
        results: &[(i64, f64)],
        latency_ms: Option<i64>,
    ) -> Result<i64> {
        let now = chrono::Utc::now().to_rfc3339();
        let (ids, scores): (Vec<i64>, Vec<f64>) = results.iter().copied().unzip();
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
