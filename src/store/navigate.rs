use anyhow::Result;
use rusqlite::params;
use std::collections::HashMap;

use super::schema::Store;
use super::types::*;

impl Store {
    /// List all indexed codebases.
    pub fn list_codebases(&self) -> Result<Vec<(i64, String, String)>> {
        let mut stmt = self
            .conn
            .prepare("SELECT id, root_path, name FROM codebases ORDER BY name")?;
        let rows = stmt
            .query_map([], |row| Ok((row.get(0)?, row.get(1)?, row.get(2)?)))?
            .collect::<std::result::Result<Vec<_>, _>>()?;
        Ok(rows)
    }

    /// Resolve a codebase by directory name. Returns None if no codebases exist.
    /// Errors if the dir doesn't match or is ambiguous.
    pub fn resolve_codebase(&self, dir: Option<&str>) -> Result<Option<i64>> {
        let codebases = self.list_codebases()?;
        if let Some(dir) = dir {
            let matches: Vec<_> = codebases
                .iter()
                .filter(|(_, root, name)| name == dir || root.ends_with(dir))
                .collect();
            match matches.len() {
                0 => anyhow::bail!("no indexed codebase matching '{dir}'"),
                1 => Ok(Some(matches[0].0)),
                _ => {
                    let names: Vec<&str> = matches.iter().map(|(_, _, n)| n.as_str()).collect();
                    anyhow::bail!(
                        "ambiguous dir '{dir}' matches {} codebases: {}. Use a more specific path.",
                        matches.len(),
                        names.join(", ")
                    );
                }
            }
        } else if codebases.len() <= 1 {
            Ok(codebases.first().map(|(id, _, _)| *id))
        } else {
            let names: Vec<&str> = codebases.iter().map(|(_, _, n)| n.as_str()).collect();
            anyhow::bail!(
                "Multiple codebases indexed. Use dir to select one: {}",
                names.join(", ")
            );
        }
    }

    /// Find all definitions of a symbol by querying chunks with matching symbol_name.
    pub fn find_definitions(
        &self,
        symbol: &str,
        codebase_id: Option<i64>,
    ) -> Result<Vec<GraphEdge>> {
        if let Some(cb) = codebase_id {
            let mut stmt = self.conn.prepare_cached(
                "SELECT file_path, symbol_name, 'definition', symbol_kind, start_line
                 FROM chunks
                 WHERE kind = 'code' AND symbol_name = ?1 AND codebase_id = ?2
                 ORDER BY file_path, start_line",
            )?;
            let edges = stmt
                .query_map(params![symbol, cb], row_to_graph_edge)?
                .filter_map(log_and_skip("navigate"))
                .collect();
            Ok(edges)
        } else {
            let mut stmt = self.conn.prepare_cached(
                "SELECT file_path, symbol_name, 'definition', symbol_kind, start_line
                 FROM chunks
                 WHERE kind = 'code' AND symbol_name = ?1
                 ORDER BY file_path, start_line",
            )?;
            let edges = stmt
                .query_map(params![symbol], row_to_graph_edge)?
                .filter_map(log_and_skip("navigate"))
                .collect();
            Ok(edges)
        }
    }

    /// Find references to a symbol using FTS5 on chunk content.
    /// Excludes chunks where the symbol is defined (avoids self-references).
    pub fn find_references(
        &self,
        symbol: &str,
        codebase_id: Option<i64>,
    ) -> Result<Vec<GraphEdge>> {
        use std::collections::HashSet;
        // Get definition chunk IDs to exclude
        let def_ids: HashSet<i64> = if let Some(cb) = codebase_id {
            let mut stmt = self.conn.prepare_cached(
                "SELECT id FROM chunks WHERE kind = 'code' AND symbol_name = ?1 AND codebase_id = ?2",
            )?;
            stmt.query_map(params![symbol, cb], |row| row.get(0))?
                .filter_map(log_and_skip("navigate"))
                .collect()
        } else {
            let mut stmt = self
                .conn
                .prepare_cached("SELECT id FROM chunks WHERE kind = 'code' AND symbol_name = ?1")?;
            stmt.query_map(params![symbol], |row| row.get(0))?
                .filter_map(log_and_skip("navigate"))
                .collect()
        };

        // FTS5 search for the symbol in chunk content, excluding definition chunks.
        // Double-quoting makes this safe against all FTS5 syntax injection:
        // operators (AND/OR/NOT/NEAR), prefix wildcards (*), and column filters (col:).
        // Internal double-quotes are escaped by doubling ("").
        let fts_query = format!("\"{}\"", symbol.replace('"', "\"\""));

        let raw_rows: Vec<(i64, String, String, i64)> = if let Some(cb) = codebase_id {
            let mut stmt = self.conn.prepare_cached(
                "SELECT c.id, c.file_path, c.symbol_kind, c.start_line
                 FROM chunks_fts f
                 JOIN chunks c ON c.id = f.rowid
                 WHERE chunks_fts MATCH ?1 AND c.kind = 'code' AND c.codebase_id = ?2
                 ORDER BY c.file_path, c.start_line",
            )?;
            stmt.query_map(params![fts_query, cb], |row| {
                Ok((row.get(0)?, row.get(1)?, row.get(2)?, row.get(3)?))
            })?
            .filter_map(log_and_skip("navigate"))
            .collect()
        } else {
            let mut stmt = self.conn.prepare_cached(
                "SELECT c.id, c.file_path, c.symbol_kind, c.start_line
                 FROM chunks_fts f
                 JOIN chunks c ON c.id = f.rowid
                 WHERE chunks_fts MATCH ?1 AND c.kind = 'code'
                 ORDER BY c.file_path, c.start_line",
            )?;
            stmt.query_map(params![fts_query], |row| {
                Ok((row.get(0)?, row.get(1)?, row.get(2)?, row.get(3)?))
            })?
            .filter_map(log_and_skip("navigate"))
            .collect()
        };

        let edges = raw_rows
            .into_iter()
            .filter(|(id, _, _, _)| !def_ids.contains(id))
            .map(|(_, file_path, kind, line)| GraphEdge {
                file_path,
                symbol: symbol.to_owned(),
                role: "reference".to_owned(),
                kind,
                line,
            })
            .collect();

        Ok(edges)
    }

    /// Get all definitions for a codebase (used by map). Derived from chunks table.
    pub fn get_all_definitions(&self, codebase_id: i64) -> Result<Vec<GraphEdge>> {
        let mut stmt = self.conn.prepare(
            "SELECT file_path, symbol_name, 'definition', symbol_kind, start_line
             FROM chunks
             WHERE codebase_id = ?1 AND kind = 'code' AND symbol_name != ''
             ORDER BY file_path, start_line",
        )?;
        let rows = stmt
            .query_map(params![codebase_id], row_to_graph_edge)?
            .collect::<std::result::Result<Vec<_>, _>>()?;
        Ok(rows)
    }

    /// Count definitions per file (used by map for ranking).
    /// Files with more definitions are likely more important.
    pub fn count_definitions_per_file(&self, codebase_id: i64) -> Result<HashMap<String, usize>> {
        let mut stmt = self.conn.prepare(
            "SELECT file_path, COUNT(*) FROM chunks
             WHERE codebase_id = ?1 AND kind = 'code' AND symbol_name != ''
             GROUP BY file_path",
        )?;
        let mut counts = HashMap::new();
        let rows = stmt.query_map(params![codebase_id], |row| {
            Ok((row.get::<_, String>(0)?, row.get::<_, usize>(1)?))
        })?;
        for row in rows {
            let (file, count) = row?;
            counts.insert(file, count);
        }
        Ok(counts)
    }

    /// Get unique symbol names defined in a specific file (used by impact BFS).
    /// Derived from chunks table.
    pub fn get_definitions_in_file(
        &self,
        file_path: &str,
        codebase_id: Option<i64>,
    ) -> Result<Vec<String>> {
        if let Some(cid) = codebase_id {
            let mut stmt = self.conn.prepare_cached(
                "SELECT DISTINCT symbol_name FROM chunks
                 WHERE codebase_id = ?1 AND file_path = ?2 AND kind = 'code' AND symbol_name != ''",
            )?;
            let rows = stmt
                .query_map(params![cid, file_path], |row| row.get(0))?
                .collect::<std::result::Result<Vec<String>, _>>()?;
            Ok(rows)
        } else {
            let mut stmt = self.conn.prepare_cached(
                "SELECT DISTINCT symbol_name FROM chunks
                 WHERE file_path = ?1 AND kind = 'code' AND symbol_name != ''",
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
}
