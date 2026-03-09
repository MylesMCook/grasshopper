use anyhow::{Context, Result};
use rusqlite::{OptionalExtension, params};

use super::schema::Store;
use super::types::*;

impl Store {
    /// Insert a memory entry. Returns the new row ID.
    pub fn insert_memory(&self, p: &MemoryParams) -> Result<i64> {
        let now = chrono::Utc::now().to_rfc3339();
        self.conn.execute_batch("SAVEPOINT insert_memory")?;
        let result = (|| -> Result<i64> {
            self.conn.execute(
                "INSERT INTO chunks (kind, title, content, memory_type, descriptors, salience,
                                     content_hash, agent_id, created_at, updated_at)
                 VALUES ('memory', ?1, ?2, ?3, ?4, ?5, ?6, 'cli', ?7, ?7)",
                params![
                    p.title,
                    p.content,
                    p.memory_type,
                    p.descriptors,
                    p.salience,
                    p.content_hash,
                    now
                ],
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
            Ok(id) => {
                self.conn.execute_batch("RELEASE insert_memory")?;
                Ok(id)
            }
            Err(e) => {
                let _ = self
                    .conn
                    .execute_batch("ROLLBACK TO insert_memory; RELEASE insert_memory");
                Err(e)
            }
        }
    }

    /// Get a chunk by ID.
    pub fn get_chunk(&self, id: i64) -> Result<Option<Chunk>> {
        let mut stmt = self.conn.prepare(
            "SELECT id, kind, title, content, snippet, symbol_name, symbol_kind, signature,
                    file_path, language, start_line, end_line, memory_type,
                    descriptors, access_count, last_accessed, salience, archived,
                    content_hash, agent_id, created_at, updated_at, codebase_id
             FROM chunks WHERE id = ?1",
        )?;

        let chunk = stmt.query_row(params![id], row_to_chunk);

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
        let select =
            "SELECT id, kind, title, content, snippet, symbol_name, symbol_kind, signature,
                    file_path, language, start_line, end_line, memory_type,
                    descriptors, access_count, last_accessed, salience, archived,
                    content_hash, agent_id, created_at, updated_at, codebase_id
             FROM chunks WHERE kind = 'memory'";

        let archived_clause = if include_archived {
            ""
        } else {
            " AND archived = 0"
        };

        if let Some(mt) = memory_type {
            let sql = format!(
                "{select}{archived_clause} AND memory_type = ?1 ORDER BY created_at DESC LIMIT ?2"
            );
            let mut stmt = self.conn.prepare(&sql)?;
            let rows = stmt
                .query_map(params![mt, limit as i64], row_to_chunk)?
                .filter_map(log_and_skip("list_memories"))
                .collect();
            Ok(rows)
        } else {
            let sql = format!("{select}{archived_clause} ORDER BY created_at DESC LIMIT ?1");
            let mut stmt = self.conn.prepare(&sql)?;
            let rows = stmt
                .query_map(params![limit as i64], row_to_chunk)?
                .filter_map(log_and_skip("list_memories"))
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
                params![
                    p.title,
                    p.content,
                    p.memory_type,
                    p.descriptors,
                    p.salience,
                    p.content_hash,
                    now,
                    id
                ],
            )?;
            if rows > 0 {
                self.conn
                    .execute("DELETE FROM chunks_fts WHERE rowid = ?1", params![id])?;
                self.conn.execute(
                    "INSERT INTO chunks_fts (rowid, title, content, snippet, symbol_name, descriptors)
                     VALUES (?1, ?2, ?3, '', '', ?4)",
                    params![id, p.title, p.content, p.descriptors],
                )?;
            }
            Ok(rows > 0)
        })();
        match result {
            Ok(updated) => {
                self.conn.execute_batch("RELEASE update_memory")?;
                Ok(updated)
            }
            Err(e) => {
                let _ = self
                    .conn
                    .execute_batch("ROLLBACK TO update_memory; RELEASE update_memory");
                Err(e)
            }
        }
    }

    /// Update a memory entry's content fields only, preserving memory_type and salience.
    pub fn update_memory_content(
        &self,
        id: i64,
        title: &str,
        content: &str,
        descriptors: &str,
        content_hash: &str,
    ) -> Result<bool> {
        let now = chrono::Utc::now().to_rfc3339();
        self.conn.execute_batch("SAVEPOINT update_memory_content")?;
        let result = (|| -> Result<bool> {
            let rows = self.conn.execute(
                "UPDATE chunks SET title = ?1, content = ?2, descriptors = ?3,
                                   content_hash = ?4, updated_at = ?5
                 WHERE id = ?6 AND kind = 'memory'",
                params![title, content, descriptors, content_hash, now, id],
            )?;
            if rows > 0 {
                self.conn
                    .execute("DELETE FROM chunks_fts WHERE rowid = ?1", params![id])?;
                self.conn.execute(
                    "INSERT INTO chunks_fts (rowid, title, content, snippet, symbol_name, descriptors)
                     VALUES (?1, ?2, ?3, '', '', ?4)",
                    params![id, title, content, descriptors],
                )?;
            }
            Ok(rows > 0)
        })();
        match result {
            Ok(updated) => {
                self.conn.execute_batch("RELEASE update_memory_content")?;
                Ok(updated)
            }
            Err(e) => {
                let _ = self.conn.execute_batch(
                    "ROLLBACK TO update_memory_content; RELEASE update_memory_content",
                );
                Err(e)
            }
        }
    }

    /// Update access tracking for a memory (update timestamp, bump salience).
    /// Salience is the sole retrieval signal — it increases on each access and feeds
    /// into cognitive scoring. access_count is legacy and no longer incremented.
    pub fn touch_memory(&self, id: i64) -> Result<()> {
        let now = chrono::Utc::now().to_rfc3339();
        self.conn.execute(
            "UPDATE chunks SET
                last_accessed = ?1,
                salience = MIN(1.0, salience + 0.05),
                updated_at = ?1
             WHERE id = ?2 AND kind = 'memory'",
            params![now, id],
        )?;
        Ok(())
    }

    /// Batch-update access tracking for multiple memories in a single statement.
    pub fn batch_touch_memories(&self, ids: &[i64]) -> Result<()> {
        if ids.is_empty() {
            return Ok(());
        }
        let now = chrono::Utc::now().to_rfc3339();
        let placeholders: Vec<&str> = ids.iter().map(|_| "?").collect();
        let sql = format!(
            "UPDATE chunks SET
                last_accessed = ?1,
                salience = MIN(1.0, salience + 0.05),
                updated_at = ?1
             WHERE id IN ({}) AND kind = 'memory'",
            placeholders.join(",")
        );
        let mut params: Vec<Box<dyn rusqlite::types::ToSql>> = vec![Box::new(now)];
        for &id in ids {
            params.push(Box::new(id));
        }
        let param_refs: Vec<&dyn rusqlite::types::ToSql> =
            params.iter().map(|p| p.as_ref()).collect();
        self.conn.execute(&sql, param_refs.as_slice())?;
        Ok(())
    }

    /// Find an active (non-archived) memory by content hash.
    pub fn find_memory_by_hash(&self, hash: &str) -> Result<Option<i64>> {
        self.conn
            .query_row(
                "SELECT id FROM chunks WHERE kind = 'memory' AND content_hash = ?1 AND archived = 0 LIMIT 1",
                params![hash],
                |row| row.get(0),
            )
            .optional()
            .context("find_memory_by_hash")
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
}
