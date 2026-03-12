mod code;
mod maintenance;
mod memory;
mod navigate;
mod schema;
mod search;
mod types;

pub use maintenance::{CodebaseInfo, MemoryStats};
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
                title: "Test",
                content: "Content",
                memory_type: "knowledge",
                descriptors: "",
                salience: 0.5,
                content_hash: "",
            })
            .unwrap();

        store.touch_memory(id).unwrap();
        let chunk = store.get_chunk(id).unwrap().unwrap();
        assert_eq!(chunk.salience, 0.55);
        assert!(chunk.last_accessed.is_some());

        // Touch again — salience should stack
        store.touch_memory(id).unwrap();
        let chunk = store.get_chunk(id).unwrap().unwrap();
        assert!((chunk.salience - 0.6).abs() < 0.001);
    }

    #[test]
    fn test_list_memories() {
        let dir = TempDir::new().unwrap();
        let db_path = dir.path().join("test.db");
        let store = Store::open(&db_path).unwrap();

        store
            .insert_memory(&MemoryParams {
                title: "A",
                content: "Content A",
                memory_type: "knowledge",
                descriptors: "",
                salience: 0.5,
                content_hash: "",
            })
            .unwrap();
        store
            .insert_memory(&MemoryParams {
                title: "B",
                content: "Content B",
                memory_type: "episode",
                descriptors: "",
                salience: 0.5,
                content_hash: "",
            })
            .unwrap();
        store
            .insert_memory(&MemoryParams {
                title: "C",
                content: "Content C",
                memory_type: "identity",
                descriptors: "",
                salience: 1.0,
                content_hash: "",
            })
            .unwrap();

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

        let id = store
            .insert_memory(&MemoryParams {
                title: "Old",
                content: "Stale",
                memory_type: "episode",
                descriptors: "",
                salience: 0.5,
                content_hash: "",
            })
            .unwrap();

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
                title: "Original",
                content: "Old content",
                memory_type: "knowledge",
                descriptors: "tag1",
                salience: 0.5,
                content_hash: "hash1",
            })
            .unwrap();

        let updated = store
            .update_memory(
                id,
                &MemoryParams {
                    title: "Updated",
                    content: "New content",
                    memory_type: "knowledge",
                    descriptors: "tag1, tag2",
                    salience: 0.5,
                    content_hash: "hash2",
                },
            )
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

        let updated = store
            .update_memory(
                999,
                &MemoryParams {
                    title: "X",
                    content: "Y",
                    memory_type: "knowledge",
                    descriptors: "",
                    salience: 0.5,
                    content_hash: "",
                },
            )
            .unwrap();
        assert!(!updated);
    }

    #[test]
    fn test_fts_syncs_on_insert() {
        let dir = TempDir::new().unwrap();
        let db_path = dir.path().join("test.db");
        let store = Store::open(&db_path).unwrap();

        // insert_memory should auto-populate the FTS index
        store
            .insert_memory(&MemoryParams {
                title: "Bun preference",
                content: "Always use bun for running scripts",
                memory_type: "knowledge",
                descriptors: "tools",
                salience: 0.5,
                content_hash: "",
            })
            .unwrap();

        let count: i64 = store
            .conn()
            .query_row(
                "SELECT COUNT(*) FROM chunks_fts WHERE chunks_fts MATCH 'bun'",
                [],
                |row| row.get(0),
            )
            .unwrap();
        assert_eq!(count, 1);
    }

    #[test]
    fn test_fts_syncs_on_update() {
        let dir = TempDir::new().unwrap();
        let db_path = dir.path().join("test.db");
        let store = Store::open(&db_path).unwrap();

        let id = store
            .insert_memory(&MemoryParams {
                title: "Original",
                content: "old content about bun",
                memory_type: "knowledge",
                descriptors: "",
                salience: 0.5,
                content_hash: "h1",
            })
            .unwrap();

        store
            .update_memory(
                id,
                &MemoryParams {
                    title: "Updated",
                    content: "new content about deno",
                    memory_type: "knowledge",
                    descriptors: "",
                    salience: 0.5,
                    content_hash: "h2",
                },
            )
            .unwrap();

        // Old term gone from FTS
        let old: i64 = store
            .conn()
            .query_row(
                "SELECT COUNT(*) FROM chunks_fts WHERE chunks_fts MATCH 'bun'",
                [],
                |row| row.get(0),
            )
            .unwrap();
        assert_eq!(old, 0);

        // New term present
        let new: i64 = store
            .conn()
            .query_row(
                "SELECT COUNT(*) FROM chunks_fts WHERE chunks_fts MATCH 'deno'",
                [],
                |row| row.get(0),
            )
            .unwrap();
        assert_eq!(new, 1);
    }

    // --- Code indexing tests ---

    #[test]
    fn test_get_or_create_codebase() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        let id1 = store
            .get_or_create_codebase("/tmp/project", "project")
            .unwrap();
        let id2 = store
            .get_or_create_codebase("/tmp/project", "project-renamed")
            .unwrap();
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
            chunks: vec![CodeChunkParams {
                chunk_key: "src/main.rs:function:hello:1:3".into(),
                file_path: "src/main.rs".into(),
                language: "rust".into(),
                symbol_kind: "function_item".into(),
                symbol_name: "hello".into(),
                signature: "fn hello()".into(),
                snippet: "fn hello() { println!(\"hi\"); }".into(),
                start_line: 1,
                end_line: 3,
                file_hash: "aaa".into(),
            }],
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
                start_line: 1,
                end_line: 5,
                file_hash: hash.into(),
            }],
        };

        store
            .batch_upsert_chunks(cb, &[make_fc("v1", "old_fn")])
            .unwrap();
        store
            .batch_upsert_chunks(cb, &[make_fc("v2", "new_fn")])
            .unwrap();

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
            file_path: "a.rs".into(),
            file_hash: "h1".into(),
            chunks: vec![CodeChunkParams {
                chunk_key: "a.rs:fn:f:1:2".into(),
                file_path: "a.rs".into(),
                language: "rust".into(),
                symbol_kind: "function_item".into(),
                symbol_name: "f".into(),
                signature: "fn f()".into(),
                snippet: "fn f() {}".into(),
                start_line: 1,
                end_line: 2,
                file_hash: "h1".into(),
            }],
        };
        let fc2 = FileChunks {
            file_path: "b.rs".into(),
            file_hash: "h2".into(),
            chunks: vec![CodeChunkParams {
                chunk_key: "b.rs:fn:g:1:2".into(),
                file_path: "b.rs".into(),
                language: "rust".into(),
                symbol_kind: "function_item".into(),
                symbol_name: "g".into(),
                signature: "fn g()".into(),
                snippet: "fn g() {}".into(),
                start_line: 1,
                end_line: 2,
                file_hash: "h2".into(),
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
        assert!(
            !refs.is_empty(),
            "FTS should find 'Store' reference in open's snippet"
        );
        assert!(refs.iter().all(|r| r.role == "reference"));
    }

    #[test]
    fn test_fts_code_expand() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();
        let cb = store.get_or_create_codebase("/tmp/p", "p").unwrap();

        let fc = FileChunks {
            file_path: "src/main.rs".into(),
            file_hash: "h".into(),
            chunks: vec![CodeChunkParams {
                chunk_key: "src/main.rs:fn:myFuncName:1:3".into(),
                file_path: "src/main.rs".into(),
                language: "rust".into(),
                symbol_kind: "function_item".into(),
                symbol_name: "myFuncName".into(),
                signature: "fn myFuncName()".into(),
                snippet: "fn myFuncName() {}".into(),
                start_line: 1,
                end_line: 3,
                file_hash: "h".into(),
            }],
        };
        store.batch_upsert_chunks(cb, &[fc]).unwrap();
        store.rebuild_fts_for_codebase(cb).unwrap();

        // code_expand should make camelCase searchable as separate words
        let hits = store.fts_search("func", None, 10).unwrap();
        assert!(
            !hits.is_empty(),
            "should find 'func' via code_expand of 'myFuncName'"
        );
    }

    #[test]
    fn test_fts_search_kind_filter() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();
        let cb = store.get_or_create_codebase("/tmp/p", "p").unwrap();

        // Insert a code chunk
        let fc = FileChunks {
            file_path: "src/lib.rs".into(),
            file_hash: "h".into(),
            chunks: vec![CodeChunkParams {
                chunk_key: "src/lib.rs:fn:search:1:5".into(),
                file_path: "src/lib.rs".into(),
                language: "rust".into(),
                symbol_kind: "function_item".into(),
                symbol_name: "search".into(),
                signature: "fn search()".into(),
                snippet: "fn search() { query_database(); }".into(),
                start_line: 1,
                end_line: 5,
                file_hash: "h".into(),
            }],
        };
        store.batch_upsert_chunks(cb, &[fc]).unwrap();
        store.rebuild_fts_for_codebase(cb).unwrap();

        // Insert a memory
        store
            .insert_memory(&MemoryParams {
                title: "Search tips",
                content: "Use search with hybrid mode for best results",
                memory_type: "knowledge",
                descriptors: "",
                salience: 0.5,
                content_hash: "",
            })
            .unwrap();

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
            file_path: "a.rs".into(),
            file_hash: "h".into(),
            chunks: vec![
                CodeChunkParams {
                    chunk_key: "a.rs:fn:a:1:2".into(),
                    file_path: "a.rs".into(),
                    language: "rust".into(),
                    symbol_kind: "fn".into(),
                    symbol_name: "a".into(),
                    signature: "fn a()".into(),
                    snippet: "fn a() {}".into(),
                    start_line: 1,
                    end_line: 2,
                    file_hash: "h".into(),
                },
                CodeChunkParams {
                    chunk_key: "a.rs:fn:b:3:4".into(),
                    file_path: "a.rs".into(),
                    language: "rust".into(),
                    symbol_kind: "fn".into(),
                    symbol_name: "b".into(),
                    signature: "fn b()".into(),
                    snippet: "fn b() {}".into(),
                    start_line: 3,
                    end_line: 4,
                    file_hash: "h".into(),
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
        let chunk_a = store
            .conn()
            .query_row("SELECT id FROM chunks WHERE symbol_name = 'a'", [], |r| {
                r.get::<_, i64>(0)
            })
            .unwrap();
        let chunk_b = store
            .conn()
            .query_row("SELECT id FROM chunks WHERE symbol_name = 'b'", [], |r| {
                r.get::<_, i64>(0)
            })
            .unwrap();

        store
            .batch_upsert_embeddings(&[
                (chunk_a, &emb_a, "test-model"),
                (chunk_b, &emb_b, "test-model"),
            ])
            .unwrap();

        // Search near emb_a — emb_b has cosine sim ~0.1 (below MIN_VECTOR_SIMILARITY=0.3)
        let query = vec![0.9, 0.1, 0.0];
        let results = store.vector_search(&query, "test-model", None, 10).unwrap();
        assert_eq!(results.len(), 1);
        assert_eq!(results[0].symbol_name.as_deref(), Some("a")); // only match above threshold
    }

    #[test]
    fn test_get_all_embeddings_and_hnsw_search() {
        let dir = TempDir::new().unwrap();
        let db_path = dir.path().join("test.db");
        let store = Store::open(&db_path).unwrap();
        let cb = store.get_or_create_codebase("/tmp/p", "p").unwrap();

        // Insert chunks with embeddings
        let fc = FileChunks {
            file_path: "a.rs".into(),
            file_hash: "h".into(),
            chunks: vec![
                CodeChunkParams {
                    chunk_key: "a.rs:fn:x:1:2".into(),
                    file_path: "a.rs".into(),
                    language: "rust".into(),
                    symbol_kind: "fn".into(),
                    symbol_name: "x".into(),
                    signature: "fn x()".into(),
                    snippet: "fn x() {}".into(),
                    start_line: 1,
                    end_line: 2,
                    file_hash: "h".into(),
                },
                CodeChunkParams {
                    chunk_key: "a.rs:fn:y:3:4".into(),
                    file_path: "a.rs".into(),
                    language: "rust".into(),
                    symbol_kind: "fn".into(),
                    symbol_name: "y".into(),
                    signature: "fn y()".into(),
                    snippet: "fn y() {}".into(),
                    start_line: 3,
                    end_line: 4,
                    file_hash: "h".into(),
                },
            ],
        };
        store.batch_upsert_chunks(cb, &[fc]).unwrap();

        let chunk_x = store
            .conn()
            .query_row("SELECT id FROM chunks WHERE symbol_name = 'x'", [], |r| {
                r.get::<_, i64>(0)
            })
            .unwrap();
        let chunk_y = store
            .conn()
            .query_row("SELECT id FROM chunks WHERE symbol_name = 'y'", [], |r| {
                r.get::<_, i64>(0)
            })
            .unwrap();

        // Use 10-dim embeddings (small for tests)
        let mut emb_x = vec![0.0f32; 10];
        emb_x[0] = 1.0;
        let mut emb_y = vec![0.0f32; 10];
        emb_y[1] = 1.0;

        store
            .batch_upsert_embeddings(&[
                (chunk_x, &emb_x, "test-model"),
                (chunk_y, &emb_y, "test-model"),
            ])
            .unwrap();

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
        assert_eq!(results.len(), 1); // emb_y has cosine sim ~0.1, below MIN_VECTOR_SIMILARITY
        assert_eq!(results[0].symbol_name.as_deref(), Some("x")); // only match above threshold

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
            id,
            kind: "code".into(),
            file_path: None,
            symbol_name: None,
            symbol_kind: None,
            signature: None,
            title: title.into(),
            snippet: String::new(),
            start_line: None,
            end_line: None,
            memory_type: None,
            score,
            reranker_score: None,
            access_count: 0,
            last_accessed: None,
            salience: 0.5,
            created_at: String::new(),
            archived: false,
            descriptors: String::new(),
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
            file_path: "a.rs".into(),
            file_hash: "h".into(),
            chunks: vec![CodeChunkParams {
                chunk_key: "a.rs:fn:f:1:2".into(),
                file_path: "a.rs".into(),
                language: "rust".into(),
                symbol_kind: "fn".into(),
                symbol_name: "f".into(),
                signature: "fn f()".into(),
                snippet: "fn f() {}".into(),
                start_line: 1,
                end_line: 2,
                file_hash: "h".into(),
            }],
        };
        store.batch_upsert_chunks(cb, &[fc]).unwrap();

        // All chunks should be stale (no embeddings yet)
        let stale = store.get_stale_embeddings(cb, "test-model").unwrap();
        assert_eq!(stale.len(), 1);
        assert_eq!(stale[0].symbol_name, "f");

        // Embed it
        let id = stale[0].id;
        store
            .batch_upsert_embeddings(&[(id, &[1.0_f32, 0.0, 0.0], "test-model")])
            .unwrap();

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

        let id1 = store
            .insert_memory(&MemoryParams {
                title: "A",
                content: "c1",
                memory_type: "knowledge",
                descriptors: "",
                salience: 0.5,
                content_hash: "",
            })
            .unwrap();
        let id2 = store
            .insert_memory(&MemoryParams {
                title: "B",
                content: "c2",
                memory_type: "knowledge",
                descriptors: "",
                salience: 0.5,
                content_hash: "",
            })
            .unwrap();

        // Embed both
        let emb1: Vec<f32> = vec![1.0, 0.0, 0.0];
        let emb2: Vec<f32> = vec![0.9, 0.1, 0.0]; // similar to emb1
        store
            .batch_upsert_embeddings(&[(id1, &emb1, "test-model"), (id2, &emb2, "test-model")])
            .unwrap();

        // Search near emb1
        let query: Vec<f32> = vec![1.0, 0.0, 0.0];
        let results = store
            .search_similar_memories(&query, "test-model", 0.5, 10)
            .unwrap();
        assert_eq!(results.len(), 2);
        assert_eq!(results[0].0, id1); // exact match first

        // High threshold should filter
        let strict = store
            .search_similar_memories(&query, "test-model", 0.95, 10)
            .unwrap();
        assert_eq!(strict.len(), 1);
        assert_eq!(strict[0].0, id1);
    }

    #[test]
    fn test_fts_search_includes_cognitive_fields() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        store
            .insert_memory(&MemoryParams {
                title: "Bun preference",
                content: "Always use bun",
                memory_type: "knowledge",
                descriptors: "tools",
                salience: 0.7,
                content_hash: "",
            })
            .unwrap();

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
        let id1 = store
            .insert_memory(&MemoryParams {
                title: "Nested1",
                content: "c1",
                memory_type: "knowledge",
                descriptors: "",
                salience: 0.5,
                content_hash: "nest1",
            })
            .unwrap();
        let id2 = store
            .insert_memory(&MemoryParams {
                title: "Nested2",
                content: "c2",
                memory_type: "knowledge",
                descriptors: "",
                salience: 0.5,
                content_hash: "nest2",
            })
            .unwrap();
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
        let _id = store
            .insert_memory(&MemoryParams {
                title: "WillRollback",
                content: "c",
                memory_type: "knowledge",
                descriptors: "",
                salience: 0.5,
                content_hash: "rb1",
            })
            .unwrap();
        store.execute_batch("ROLLBACK TO outer").unwrap();
        store.execute_batch("RELEASE outer").unwrap();

        // Memory count should be 0 — the insert was rolled back
        let (_, mem_count) = store.count_by_kind().unwrap();
        assert_eq!(
            mem_count, 0,
            "insert_memory should be rollbackable from outer savepoint"
        );
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

    #[test]
    fn test_count_embedded_filtered() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        // Insert a memory with embedding
        let id = store
            .insert_memory(&MemoryParams {
                title: "M",
                content: "memory content",
                memory_type: "knowledge",
                descriptors: "",
                salience: 0.5,
                content_hash: "mem_hash",
            })
            .unwrap();
        let emb = vec![0.1f32; 768];
        store
            .batch_upsert_embeddings(&[(id, emb.as_slice(), "test-model")])
            .unwrap();

        // Global count = 1
        assert_eq!(store.count_embedded().unwrap(), 1);
        // Filtered by memory = 1
        assert_eq!(store.count_embedded_filtered(Some("memory")).unwrap(), 1);
        // Filtered by code = 0
        assert_eq!(store.count_embedded_filtered(Some("code")).unwrap(), 0);
        // No filter = 1
        assert_eq!(store.count_embedded_filtered(None).unwrap(), 1);
    }

    #[test]
    fn test_batch_touch_memories() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        let id1 = store
            .insert_memory(&MemoryParams {
                title: "A",
                content: "Content A",
                memory_type: "knowledge",
                descriptors: "",
                salience: 0.5,
                content_hash: "h1",
            })
            .unwrap();
        let id2 = store
            .insert_memory(&MemoryParams {
                title: "B",
                content: "Content B",
                memory_type: "episode",
                descriptors: "",
                salience: 0.5,
                content_hash: "h2",
            })
            .unwrap();

        // Batch touch both
        store.batch_touch_memories(&[id1, id2]).unwrap();

        let c1 = store.get_chunk(id1).unwrap().unwrap();
        let c2 = store.get_chunk(id2).unwrap().unwrap();
        assert_eq!(c1.salience, 0.55, "salience should be bumped by +0.05");
        assert_eq!(c2.salience, 0.55);
        assert!(c1.last_accessed.is_some(), "last_accessed should be set");
        assert!(c2.last_accessed.is_some());

        // Touch again — salience should stack
        store.batch_touch_memories(&[id1]).unwrap();
        let c1 = store.get_chunk(id1).unwrap().unwrap();
        assert!((c1.salience - 0.6).abs() < 0.001);

        // Empty slice is a no-op (should not error)
        store.batch_touch_memories(&[]).unwrap();
    }

    #[test]
    fn test_batch_touch_memories_salience_cap() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        let id = store
            .insert_memory(&MemoryParams {
                title: "High salience",
                content: "Already high",
                memory_type: "knowledge",
                descriptors: "",
                salience: 0.98,
                content_hash: "",
            })
            .unwrap();

        // Touch multiple times — salience should cap at 1.0
        store.batch_touch_memories(&[id]).unwrap();
        store.batch_touch_memories(&[id]).unwrap();
        store.batch_touch_memories(&[id]).unwrap();

        let chunk = store.get_chunk(id).unwrap().unwrap();
        assert!(
            chunk.salience <= 1.0,
            "salience should cap at 1.0, got {}",
            chunk.salience
        );
    }

    #[test]
    fn test_update_memory_content() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        let id = store
            .insert_memory(&MemoryParams {
                title: "Original title",
                content: "Original content about bun",
                memory_type: "procedure",
                descriptors: "tools",
                salience: 0.8,
                content_hash: "orig_hash",
            })
            .unwrap();

        // update_memory_content should change title/content/descriptors/hash
        // but preserve memory_type and salience
        let updated = store
            .update_memory_content(
                id,
                "New title",
                "New content about deno",
                "runtime",
                "new_hash",
            )
            .unwrap();
        assert!(updated);

        let chunk = store.get_chunk(id).unwrap().unwrap();
        assert_eq!(chunk.title, "New title");
        assert_eq!(chunk.content, "New content about deno");
        assert_eq!(chunk.descriptors, "runtime");
        assert_eq!(chunk.content_hash, "new_hash");
        // memory_type and salience should be preserved
        assert_eq!(chunk.memory_type.as_deref(), Some("procedure"));
        assert_eq!(chunk.salience, 0.8);
    }

    #[test]
    fn test_update_memory_content_nonexistent() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        let updated = store
            .update_memory_content(99999, "T", "C", "d", "h")
            .unwrap();
        assert!(!updated, "updating nonexistent ID should return false");
    }

    #[test]
    fn test_update_memory_content_fts_sync() {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();

        let id = store
            .insert_memory(&MemoryParams {
                title: "Bun preference",
                content: "Use bun for scripts",
                memory_type: "knowledge",
                descriptors: "",
                salience: 0.5,
                content_hash: "h1",
            })
            .unwrap();

        // FTS should find "bun" before update
        let hits = store.fts_search("bun", Some("memory"), 10).unwrap();
        assert_eq!(hits.len(), 1);

        // Update content via update_memory_content
        store
            .update_memory_content(id, "Deno preference", "Use deno for scripts", "", "h2")
            .unwrap();

        // "bun" should no longer match
        let hits = store.fts_search("bun", Some("memory"), 10).unwrap();
        assert_eq!(hits.len(), 0);

        // "deno" should now match
        let hits = store.fts_search("deno", Some("memory"), 10).unwrap();
        assert_eq!(hits.len(), 1);
    }

    // ---------------------------------------------------------------
    // Adversarial FTS5 integration fuzzing
    // ---------------------------------------------------------------
    //
    // These test that fts_search handles adversarial queries through the
    // full SQLite/FTS5 stack without panicking or returning Err.

    /// Helper: create a store with one memory entry for adversarial search tests.
    fn store_with_memory() -> (TempDir, Store) {
        let dir = TempDir::new().unwrap();
        let store = Store::open(&dir.path().join("test.db")).unwrap();
        store
            .insert_memory(&MemoryParams {
                title: "Test entry",
                content: "The quick brown fox jumps over the lazy dog",
                memory_type: "knowledge",
                descriptors: "animals, pangram",
                salience: 0.5,
                content_hash: "fts_fuzz",
            })
            .unwrap();
        (dir, store)
    }

    #[test]
    fn fts_integration_fts5_operators() {
        let (_dir, store) = store_with_memory();
        // FTS5 boolean operators — should not cause SQL errors
        for query in &[
            "error NOT found",
            "foo AND bar",
            "x OR y",
            "NEAR(quick,fox)",
            "NOT NOT NOT",
            "NEAR/5(a b)",
        ] {
            let result = store.fts_search(query, None, 10);
            assert!(
                result.is_ok(),
                "fts_search should not Err on FTS5 operator query {:?}: {:?}",
                query,
                result.err()
            );
        }
    }

    #[test]
    fn fts_integration_column_filters() {
        let (_dir, store) = store_with_memory();
        for query in &[
            "title:hack",
            "content:secret",
            "symbol_name:drop",
            "{col}:value",
        ] {
            let result = store.fts_search(query, None, 10);
            assert!(
                result.is_ok(),
                "fts_search should not Err on column filter {:?}: {:?}",
                query,
                result.err()
            );
        }
    }

    #[test]
    fn fts_integration_wildcards() {
        let (_dir, store) = store_with_memory();
        for query in &["test*", "*", "te*st", "***", "?", "test?"] {
            let result = store.fts_search(query, None, 10);
            assert!(
                result.is_ok(),
                "fts_search should not Err on wildcard {:?}: {:?}",
                query,
                result.err()
            );
        }
    }

    #[test]
    fn fts_integration_sql_injection() {
        let (_dir, store) = store_with_memory();
        for query in &[
            "'; DROP TABLE chunks; --",
            "\" OR 1=1",
            "'; DELETE FROM chunks WHERE ''='",
            "1; SELECT * FROM sqlite_master; --",
            "UNION SELECT * FROM chunks --",
        ] {
            let result = store.fts_search(query, None, 10);
            assert!(
                result.is_ok(),
                "fts_search should not Err on SQL injection {:?}: {:?}",
                query,
                result.err()
            );
        }
        // Verify the table still exists and has data
        let (_code, mem) = store.count_by_kind().unwrap();
        assert_eq!(mem, 1, "memory should not have been dropped or deleted");
    }

    #[test]
    fn fts_integration_unicode() {
        let (_dir, store) = store_with_memory();
        for query in &[
            "\u{1F600}",                                // emoji
            "\u{4F60}\u{597D}",                         // CJK
            "\u{0645}\u{0631}\u{062D}\u{0628}\u{0627}", // Arabic
            "\u{05E9}\u{05DC}\u{05D5}\u{05DD}",         // Hebrew
            "test\u{200B}word",                         // zero-width space
            "\u{FEFF}bom",                              // BOM
        ] {
            let result = store.fts_search(query, None, 10);
            assert!(
                result.is_ok(),
                "fts_search should not Err on unicode {:?}: {:?}",
                query,
                result.err()
            );
        }
    }

    #[test]
    fn fts_integration_edge_cases() {
        let (_dir, store) = store_with_memory();

        // Empty string
        let result = store.fts_search("", None, 10);
        assert!(result.is_ok());
        assert!(result.unwrap().is_empty());

        // Single char
        let result = store.fts_search("a", None, 10);
        assert!(result.is_ok());

        // Huge query (100KB)
        let big = "adversarial ".repeat(8500);
        let result = store.fts_search(&big, None, 10);
        assert!(result.is_ok(), "100KB query should not panic");

        // Null bytes
        let result = store.fts_search("hello\x00world", None, 10);
        assert!(result.is_ok());

        // Punctuation only
        let result = store.fts_search("!@#$%^&*()", None, 10);
        assert!(result.is_ok());
    }

    #[test]
    fn fts_integration_fts5_special_syntax() {
        let (_dir, store) = store_with_memory();
        for query in &[
            "^start",
            "{title content}:search",
            "NEAR/5",
            "^ first",
            "((((test))))",
            "\"\"\"\"\"",
            "test\"escape\"attempt",
        ] {
            let result = store.fts_search(query, None, 10);
            assert!(
                result.is_ok(),
                "fts_search should not Err on FTS5 syntax {:?}: {:?}",
                query,
                result.err()
            );
        }
    }

    #[test]
    fn fts_integration_adversarial_with_kind_filter() {
        let (_dir, store) = store_with_memory();
        // Ensure kind filter doesn't break adversarial queries
        for kind in &[Some("memory"), Some("code"), None] {
            let result = store.fts_search("'; DROP TABLE chunks; --", *kind, 10);
            assert!(
                result.is_ok(),
                "adversarial query with kind={:?} should not Err",
                kind
            );
        }
    }

    #[test]
    fn fts_integration_finds_real_content_after_adversarial() {
        let (_dir, store) = store_with_memory();
        // Run adversarial queries first, then verify real search still works
        let _ = store.fts_search("'; DROP TABLE chunks; --", None, 10);
        let _ = store.fts_search("UNION SELECT * FROM chunks", None, 10);

        // Real query should still find our memory
        let hits = store.fts_search("quick brown fox", None, 10).unwrap();
        assert!(
            !hits.is_empty(),
            "real search should still work after adversarial queries"
        );
    }
}
