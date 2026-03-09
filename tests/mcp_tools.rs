//! Integration tests for MCP tool handlers.
//!
//! Tests call tool methods directly on GrasshopperMcp — no HTTP server needed.
//! All tests run without embedder/reranker (FTS-only paths), which is correct
//! for CI where model downloads would be slow and flaky.

use grasshopper::mcp::{GrasshopperMcp, IndexParams, SearchParams, StoreParams};
use rmcp::handler::server::wrapper::Parameters;
use rmcp::model::RawContent;
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
        r#"fn main() {
    println!("hello");
    greet("world");
}

fn greet(name: &str) {
    println!("Hello, {name}!");
}
"#,
    )
    .unwrap();

    std::fs::write(
        dir.path().join("lib.rs"),
        r#"pub struct Config {
    pub name: String,
    pub debug: bool,
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
    assert_eq!(json["files_scanned"], 2);
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
    assert!(text.contains("lib.rs"));
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
        text.contains("lib"),
        "map should list lib.rs symbols, got: {text}"
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

    // Impact may or may not find results depending on FTS cross-references.
    // The key thing is it doesn't error.
    assert!(result.is_error.is_none() || !result.is_error.unwrap());
    let text = result_text(&result);
    // Either finds impact or reports none found — both are valid
    assert!(
        text.contains("Impact of changing") || text.contains("No impact found"),
        "impact mode should return a valid response, got: {text}"
    );
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
