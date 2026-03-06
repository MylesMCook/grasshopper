use anyhow::Result;
use rusqlite::params;
use std::collections::{HashMap, HashSet};

use super::schema::Store;
use super::types::*;

impl Store {
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

    /// Get all file hashes for a codebase (rel_path -> hash).
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

    /// Batch upsert chunks for multiple files within a transaction.
    /// Deletes old chunks for each file, inserts new ones. Returns total chunk count.
    pub fn batch_upsert_chunks(&self, codebase_id: i64, file_chunks: &[FileChunks]) -> Result<usize> {
        let tx = self.conn.unchecked_transaction()?;
        let mut total = 0;
        let now = chrono::Utc::now().to_rfc3339();
        let ts = now_millis();
        for fc in file_chunks {
            // Delete old FTS rows before removing chunks (avoids orphaned FTS entries)
            tx.execute(
                "DELETE FROM chunks_fts WHERE rowid IN (
                    SELECT id FROM chunks WHERE codebase_id = ?1 AND file_path = ?2 AND kind = 'code'
                )",
                params![codebase_id, fc.file_path],
            )?;
            // Delete old chunks for this file
            tx.execute(
                "DELETE FROM chunks WHERE codebase_id = ?1 AND file_path = ?2 AND kind = 'code'",
                params![codebase_id, fc.file_path],
            )?;
            // Insert new chunks
            for c in &fc.chunks {
                tx.execute(
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
            tx.execute(
                "INSERT INTO indexed_files (codebase_id, file_path, file_hash, chunk_count, indexed_at)
                 VALUES (?1, ?2, ?3, ?4, ?5)
                 ON CONFLICT(codebase_id, file_path) DO UPDATE SET
                    file_hash = excluded.file_hash, chunk_count = excluded.chunk_count,
                    indexed_at = excluded.indexed_at",
                params![codebase_id, fc.file_path, fc.file_hash, fc.chunks.len() as i64, ts],
            )?;
        }
        tx.commit()?;
        Ok(total)
    }

    /// Remove indexed files and their chunks that no longer exist on disk.
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
                "DELETE FROM indexed_files WHERE codebase_id = ?1 AND file_path = ?2",
                params![codebase_id, file_path],
            )?;
        }

        Ok(stale.len())
    }

    /// Incremental FTS sync for changed files. Uses code_expand() on symbol_name.
    /// Wrapped in a transaction so FTS stays consistent if interrupted.
    pub fn sync_fts_for_files(&self, codebase_id: i64, changed_files: &[String]) -> Result<()> {
        let tx = self.conn.unchecked_transaction()?;
        for file_path in changed_files {
            tx.execute(
                "DELETE FROM chunks_fts WHERE rowid IN (
                    SELECT id FROM chunks WHERE codebase_id = ?1 AND file_path = ?2 AND kind = 'code'
                )",
                params![codebase_id, file_path],
            )?;
            tx.execute(
                "INSERT INTO chunks_fts (rowid, title, content, snippet, symbol_name, descriptors)
                 SELECT id, COALESCE(symbol_name, ''), content, snippet,
                        code_expand(COALESCE(symbol_name, '')), COALESCE(descriptors, '')
                 FROM chunks WHERE codebase_id = ?1 AND file_path = ?2 AND kind = 'code'",
                params![codebase_id, file_path],
            )?;
        }
        tx.commit()?;
        Ok(())
    }

    /// Full FTS rebuild for all code chunks in a codebase.
    /// Wrapped in a transaction so FTS stays consistent if interrupted.
    pub fn rebuild_fts_for_codebase(&self, codebase_id: i64) -> Result<()> {
        let tx = self.conn.unchecked_transaction()?;
        tx.execute(
            "DELETE FROM chunks_fts WHERE rowid IN (
                SELECT id FROM chunks WHERE codebase_id = ?1 AND kind = 'code'
            )",
            params![codebase_id],
        )?;
        tx.execute(
            "INSERT INTO chunks_fts (rowid, title, content, snippet, symbol_name, descriptors)
             SELECT id, COALESCE(symbol_name, ''), content, snippet,
                    code_expand(COALESCE(symbol_name, '')), COALESCE(descriptors, '')
             FROM chunks WHERE codebase_id = ?1 AND kind = 'code'",
            params![codebase_id],
        )?;
        tx.commit()?;
        Ok(())
    }

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
        let tx = self.conn.unchecked_transaction()?;
        {
            let mut stmt = tx.prepare_cached(
                "UPDATE chunks SET embedding = ?1, embedding_model = ?2 WHERE id = ?3",
            )?;
            for &(id, embedding, model_name) in items {
                let blob = embedding_to_blob(embedding);
                stmt.execute(params![blob, model_name, id])?;
            }
        }
        tx.commit()?;
        Ok(())
    }
}
