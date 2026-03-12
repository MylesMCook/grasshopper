//! Integration tests for MCP tool handlers.
//!
//! Tests call tool methods directly on GrasshopperMcp — no HTTP server needed.
//! All tests run without embedder/reranker (FTS-only paths), which is correct
//! for CI where model downloads would be slow and flaky.

use grasshopper::mcp::{GrasshopperMcp, IndexParams, SearchParams, StoreParams};
use rmcp::handler::server::wrapper::Parameters;
use rmcp::model::RawContent;
use std::sync::atomic::AtomicI64;
use std::sync::{Arc, Mutex};
use tempfile::TempDir;

/// Create a temp DB with model init disabled and return (mcp, db_dir).
fn setup() -> (GrasshopperMcp, TempDir) {
    let dir = TempDir::new().unwrap();
    let db_path = dir.path().join("test.db");
    let mcp = GrasshopperMcp::new_without_models(db_path);
    (mcp, dir)
}

/// Create a temp directory with sample source files for indexing.
fn create_sample_project() -> TempDir {
    let dir = TempDir::new().unwrap();

    std::fs::write(
        dir.path().join("main.rs"),
        r#"mod handlers;
mod service;

fn main() {
    handlers::run();
}
"#,
    )
    .unwrap();

    std::fs::write(
        dir.path().join("handlers.rs"),
        r#"use crate::service::greet;

pub fn run() {
    greet("world");
}
"#,
    )
    .unwrap();

    std::fs::write(
        dir.path().join("service.rs"),
        r#"pub struct Config {
    pub name: String,
    pub debug: bool,
}

pub fn greet(name: &str) {
    println!("Hello, {name}!");
}

impl Config {
    pub fn new() -> Self {
        Config { name: "default".into(), debug: false }
    }
}

pub fn process(config: &Config) {
    if config.debug {
        println!("processing: {}", config.name);
    }
}
"#,
    )
    .unwrap();

    dir
}

fn poisoned_reranker_mutex() -> Arc<Mutex<Option<grasshopper::rerank::Reranker>>> {
    let reranker = Arc::new(Mutex::new(None));
    let reranker_clone = Arc::clone(&reranker);

    let _ = std::thread::spawn(move || {
        let _guard = reranker_clone.lock().unwrap();
        panic!("poison reranker mutex for test");
    })
    .join();

    reranker
}

/// Extract text content from a CallToolResult.
fn result_text(result: &rmcp::model::CallToolResult) -> String {
    result
        .content
        .iter()
        .filter_map(|c| match &c.raw {
            RawContent::Text(t) => Some(t.text.as_str()),
            _ => None,
        })
        .collect::<Vec<_>>()
        .join("")
}

// --- Index tests ---

#[tokio::test]
async fn index_returns_file_counts() {
    let (mcp, _db_dir) = setup();
    let project = create_sample_project();

    let result = mcp
        .index_dir(Parameters(IndexParams {
            directory: project.path().to_string_lossy().into_owned(),
            embed: None,
        }))
        .await
        .unwrap();

    assert!(result.is_error.is_none() || !result.is_error.unwrap());
    let text = result_text(&result);
    let json: serde_json::Value = serde_json::from_str(&text).unwrap();
    assert_eq!(json["files_scanned"], 3);
    assert!(json["chunks_written"].as_i64().unwrap() > 0);
}

// --- Search tests ---

#[tokio::test]
async fn search_fts_returns_results() {
    let (mcp, _db_dir) = setup();
    let project = create_sample_project();

    // Index first
    mcp.index_dir(Parameters(IndexParams {
        directory: project.path().to_string_lossy().into_owned(),
        embed: None,
    }))
    .await
    .unwrap();

    // Search for "greet"
    let result = mcp
        .search(Parameters(SearchParams {
            query: "greet".into(),
            mode: None,
            kind: Some("code".into()),
            limit: Some(5),
            threshold: None,
            budget: None,
            depth: None,
            dir: None,
            direction: None,
        }))
        .await
        .unwrap();

    assert!(result.is_error.is_none() || !result.is_error.unwrap());
    let text = result_text(&result);
    let hits: Vec<serde_json::Value> = serde_json::from_str(&text).unwrap();
    assert!(
        !hits.is_empty(),
        "FTS search for 'greet' should return results"
    );
    assert!(hits.iter().any(|h| {
        h["symbol_name"]
            .as_str()
            .is_some_and(|s| s.contains("greet"))
    }));
}

#[tokio::test]
async fn search_code_only_skips_poisoned_reranker_lock() {
    let dir = TempDir::new().unwrap();
    let db_path = dir.path().join("test.db");
    let mcp = GrasshopperMcp::with_state_for_tests(
        db_path,
        Arc::new(Mutex::new(None)),
        poisoned_reranker_mutex(),
        Arc::new(AtomicI64::new(0)),
        Arc::new(Mutex::new(None)),
    );
    let project = create_sample_project();

    mcp.index_dir(Parameters(IndexParams {
        directory: project.path().to_string_lossy().into_owned(),
        embed: None,
    }))
    .await
    .unwrap();

    let result = mcp
        .search(Parameters(SearchParams {
            query: "greet".into(),
            mode: None,
            kind: Some("code".into()),
            limit: Some(5),
            threshold: None,
            budget: None,
            depth: None,
            dir: None,
            direction: None,
        }))
        .await
        .unwrap();

    assert!(result.is_error.is_none() || !result.is_error.unwrap());
}

#[tokio::test]
async fn search_memory_attempts_reranker_lock() {
    let dir = TempDir::new().unwrap();
    let db_path = dir.path().join("test.db");
    let mcp = GrasshopperMcp::with_state_for_tests(
        db_path,
        Arc::new(Mutex::new(None)),
        poisoned_reranker_mutex(),
        Arc::new(AtomicI64::new(0)),
        Arc::new(Mutex::new(None)),
    );

    let result = mcp
        .search(Parameters(SearchParams {
            query: "anything".into(),
            mode: None,
            kind: Some("memory".into()),
            limit: Some(5),
            threshold: None,
            budget: None,
            depth: None,
            dir: None,
            direction: None,
        }))
        .await
        .unwrap();

    assert_eq!(result.is_error, Some(true));
    assert!(result_text(&result).contains("lock:"));
}

#[tokio::test]
async fn search_navigate_returns_definitions() {
    let (mcp, _db_dir) = setup();
    let project = create_sample_project();

    mcp.index_dir(Parameters(IndexParams {
        directory: project.path().to_string_lossy().into_owned(),
        embed: None,
    }))
    .await
    .unwrap();

    let result = mcp
        .search(Parameters(SearchParams {
            query: "Config".into(),
            mode: Some("navigate".into()),
            kind: None,
            limit: None,
            threshold: None,
            budget: None,
            depth: None,
            dir: None,
            direction: Some("defs".into()),
        }))
        .await
        .unwrap();

    assert!(result.is_error.is_none() || !result.is_error.unwrap());
    let text = result_text(&result);
    assert!(
        text.contains("Definitions of 'Config'"),
        "navigate mode should find Config definition, got: {text}"
    );
    assert!(text.contains("service.rs"));
}

#[tokio::test]
async fn search_navigate_returns_references() {
    let (mcp, _db_dir) = setup();
    let project = create_sample_project();

    mcp.index_dir(Parameters(IndexParams {
        directory: project.path().to_string_lossy().into_owned(),
        embed: None,
    }))
    .await
    .unwrap();

    let result = mcp
        .search(Parameters(SearchParams {
            query: "greet".into(),
            mode: Some("navigate".into()),
            kind: None,
            limit: None,
            threshold: None,
            budget: None,
            depth: None,
            dir: None,
            direction: Some("refs".into()),
        }))
        .await
        .unwrap();

    assert!(result.is_error.is_none() || !result.is_error.unwrap());
    let text = result_text(&result);
    assert!(
        text.contains("References to 'greet'"),
        "navigate refs should find greet references, got: {text}"
    );
    assert!(text.contains("handlers.rs"));
}

#[tokio::test]
async fn search_map_returns_file_listing() {
    let (mcp, _db_dir) = setup();
    let project = create_sample_project();

    mcp.index_dir(Parameters(IndexParams {
        directory: project.path().to_string_lossy().into_owned(),
        embed: None,
    }))
    .await
    .unwrap();

    let result = mcp
        .search(Parameters(SearchParams {
            query: "overview".into(),
            mode: Some("map".into()),
            kind: None,
            limit: None,
            threshold: None,
            budget: Some(4000),
            depth: None,
            dir: None,
            direction: None,
        }))
        .await
        .unwrap();

    assert!(result.is_error.is_none() || !result.is_error.unwrap());
    let text = result_text(&result);
    assert!(
        text.contains("main"),
        "map should list main.rs symbols, got: {text}"
    );
    assert!(
        text.contains("service"),
        "map should list service.rs symbols, got: {text}"
    );
}

#[tokio::test]
async fn search_impact_returns_affected_files() {
    let (mcp, _db_dir) = setup();
    let project = create_sample_project();

    mcp.index_dir(Parameters(IndexParams {
        directory: project.path().to_string_lossy().into_owned(),
        embed: None,
    }))
    .await
    .unwrap();

    let result = mcp
        .search(Parameters(SearchParams {
            query: "greet".into(),
            mode: Some("impact".into()),
            kind: None,
            limit: None,
            threshold: None,
            budget: None,
            depth: Some(2),
            dir: None,
            direction: None,
        }))
        .await
        .unwrap();

    assert!(result.is_error.is_none() || !result.is_error.unwrap());
    let text = result_text(&result);
    assert!(
        text.contains("Impact of changing 'greet'"),
        "impact mode should find greet impact, got: {text}"
    );
    assert!(text.contains("handlers.rs"));
    assert!(text.contains("main.rs"));
}

#[tokio::test]
async fn search_invalid_mode_returns_error() {
    let (mcp, _db_dir) = setup();

    let result = mcp
        .search(Parameters(SearchParams {
            query: "test".into(),
            mode: Some("invalid_mode".into()),
            kind: None,
            limit: None,
            threshold: None,
            budget: None,
            depth: None,
            dir: None,
            direction: None,
        }))
        .await
        .unwrap();

    assert_eq!(result.is_error, Some(true));
    let text = result_text(&result);
    assert!(text.contains("invalid mode"));
}

// --- Store tests ---

#[tokio::test]
async fn store_returns_id() {
    let (mcp, _db_dir) = setup();

    let result = mcp
        .store(Parameters(StoreParams {
            content: "Always use bun for package management".into(),
            title: Some("Package manager preference".into()),
            tags: Some("tooling,preferences".into()),
            memory_type: None,
        }))
        .await
        .unwrap();

    assert!(result.is_error.is_none() || !result.is_error.unwrap());
    let text = result_text(&result);
    let json: serde_json::Value = serde_json::from_str(&text).unwrap();
    assert!(json["id"].as_i64().unwrap() > 0);
    assert_eq!(json["was_update"], false);
}

#[tokio::test]
async fn store_duplicate_triggers_update() {
    let (mcp, _db_dir) = setup();

    // Store once
    let result1 = mcp
        .store(Parameters(StoreParams {
            content: "Use Rust for all new backend services".into(),
            title: Some("Language choice".into()),
            tags: None,
            memory_type: None,
        }))
        .await
        .unwrap();

    let json1: serde_json::Value = serde_json::from_str(&result_text(&result1)).unwrap();
    let id1 = json1["id"].as_i64().unwrap();

    // Store same content again — should dedup
    let result2 = mcp
        .store(Parameters(StoreParams {
            content: "Use Rust for all new backend services".into(),
            title: Some("Language choice".into()),
            tags: None,
            memory_type: None,
        }))
        .await
        .unwrap();

    let json2: serde_json::Value = serde_json::from_str(&result_text(&result2)).unwrap();
    assert_eq!(
        json2["was_update"], true,
        "duplicate content should trigger update"
    );
    assert_eq!(
        json2["id"].as_i64().unwrap(),
        id1,
        "should update same entry"
    );
}
