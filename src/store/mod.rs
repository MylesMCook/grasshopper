mod schema;
mod types;
mod memory;
mod code;
mod search;
mod navigate;
mod maintenance;

pub use schema::Store;
pub use types::*;

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashSet;
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
    fn test_chunk_derived_definitions() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();
        let cb = store.get_or_create_codebase("/tmp/p", "p").unwrap();

        let fc = FileChunks {
            file_path: "src/store.rs".into(),
            file_hash: "abc123".into(),
            chunks: vec![
                CodeChunkParams {
                    chunk_key: "src/store.rs:block:Store:1:10".into(),
                    file_path: "src/store.rs".into(),
                    language: "rust".into(),
                    symbol_kind: "block".into(),
                    symbol_name: "Store".into(),
                    signature: "pub struct Store {".into(),
                    snippet: "pub struct Store {\n    conn: Connection,\n}".into(),
                    start_line: 1,
                    end_line: 10,
                    file_hash: "abc123".into(),
                },
                CodeChunkParams {
                    chunk_key: "src/store.rs:block:open:12:20".into(),
                    file_path: "src/store.rs".into(),
                    language: "rust".into(),
                    symbol_kind: "block".into(),
                    symbol_name: "open".into(),
                    signature: "pub fn open(path: &Path) -> Result<Self> {".into(),
                    snippet: "pub fn open(path: &Path) -> Result<Self> {\n    let conn = Store::new();\n}".into(),
                    start_line: 12,
                    end_line: 20,
                    file_hash: "abc123".into(),
                },
            ],
        };
        store.batch_upsert_chunks(cb, &[fc]).unwrap();
        store.rebuild_fts_for_codebase(cb).unwrap();

        // Definitions come from chunks table
        let defs = store.find_definitions("Store", Some(cb)).unwrap();
        assert_eq!(defs.len(), 1);
        assert_eq!(defs[0].line, 1);
        assert_eq!(defs[0].file_path, "src/store.rs");

        // References found via FTS — "open" chunk mentions "Store" in its snippet
        let refs = store.find_references("Store", Some(cb)).unwrap();
        assert!(!refs.is_empty(), "FTS should find 'Store' reference in open's snippet");
        assert!(refs.iter().all(|r| r.role == "reference"));
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
        let hnsw = crate::code::hnsw::HnswIndex::from_embeddings(&all).unwrap();
        assert_eq!(hnsw.len(), 2);

        // Query close to x
        let mut query = vec![0.0f32; 10];
        query[0] = 0.9;
        query[1] = 0.1;

        let results = store.vector_search_hnsw(&hnsw, &query, None, 10).unwrap();
        assert_eq!(results.len(), 2);
        assert_eq!(results[0].symbol_name.as_deref(), Some("x")); // closer to query

        // Test persistence
        let hnsw_path = crate::code::hnsw::hnsw_path(&db_path);
        hnsw.save(&hnsw_path).unwrap();
        let loaded = crate::code::hnsw::HnswIndex::load(&hnsw_path).unwrap();
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
        assert_eq!(store.count_codebases().unwrap(), 0);
    }

}
