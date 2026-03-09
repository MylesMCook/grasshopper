mod format;
mod transport;

pub use transport::{run_http, run_stdio};

use anyhow::{Context, Result};
use rmcp::handler::server::router::tool::ToolRouter;
use rmcp::handler::server::wrapper::Parameters;
use rmcp::model::{CallToolResult, Content, ServerCapabilities, ServerInfo, ToolAnnotations};
use rmcp::{ServerHandler, tool, tool_handler, tool_router};
use schemars::JsonSchema;
use serde::Deserialize;
use std::path::PathBuf;
use std::sync::{Arc, Mutex};

use crate::code::embed::Embedder;
use crate::store::Store;

use format::format_search_hits;

// --- Tool parameter types ---

#[derive(Debug, Deserialize, JsonSchema)]
pub struct SearchParams {
    /// Natural language query or keyword to search for
    pub query: String,
    /// Search mode. Values: "search" (default), "navigate", "map", "impact"
    pub mode: Option<String>,
    /// Filter by entry type. Values: "all" (default), "code", "memory"
    pub kind: Option<String>,
    /// Maximum results to return, 1-100 (default: 10)
    pub limit: Option<usize>,
    /// Minimum relevance threshold (0.0-1.0). Only for mode=search with kind=memory
    pub threshold: Option<f32>,
    /// Token budget for output (default: 4000). Only for mode=map
    pub budget: Option<usize>,
    /// BFS depth for impact analysis, 1-5 (default: 2). Only for mode=impact
    pub depth: Option<usize>,
    /// Scope to a codebase by directory name. Required when multiple codebases indexed
    pub dir: Option<String>,
    /// Which edges to follow for navigate. Values: "both" (default), "defs", "refs"
    pub direction: Option<String>,
}

#[derive(Debug, Deserialize, JsonSchema)]
pub struct IndexParams {
    /// Absolute path to the directory to index (e.g. "/home/user/projects/myapp")
    pub directory: String,
    /// Also generate semantic embeddings after indexing. Enables hybrid search but is slow (~4 embeddings/sec on CPU). Default: false
    pub embed: Option<bool>,
}

#[derive(Debug, Deserialize, JsonSchema)]
pub struct StoreParams {
    /// The information to store. Raw content — facts, decisions, preferences, observations, anything worth persisting
    pub content: String,
    /// Short title for this memory. Truncated from content if omitted
    pub title: Option<String>,
    /// Comma-separated tags for organization (e.g. "rust,grasshopper,architecture")
    pub tags: Option<String>,
    /// Memory type: "knowledge" (default), "identity" (never decays), "episode" (fast decay), "procedure" (medium decay)
    pub memory_type: Option<String>,
}

// --- MCP server handler ---

#[derive(Clone)]
pub struct GrasshopperMcp {
    db_path: PathBuf,
    embedder: Arc<Mutex<Option<Embedder>>>,
    reranker: Arc<Mutex<Option<crate::rerank::Reranker>>>,
    /// Epoch seconds of last reranker init failure (0 = never failed). Retry after 60s cooldown.
    reranker_failed_at: Arc<std::sync::atomic::AtomicI64>,
    hnsw: Arc<Mutex<Option<crate::code::hnsw::HnswIndex>>>,
    /// When true, skip lazy model initialization (for tests without model downloads).
    skip_model_init: bool,
    tool_router: ToolRouter<Self>,
}

#[tool_router]
impl GrasshopperMcp {
    pub fn new(db_path: PathBuf) -> Self {
        let hnsw = Self::try_load_hnsw(&db_path);
        Self {
            db_path,
            embedder: Arc::new(Mutex::new(None)),
            reranker: Arc::new(Mutex::new(None)),
            reranker_failed_at: Arc::new(std::sync::atomic::AtomicI64::new(0)),
            hnsw: Arc::new(Mutex::new(hnsw)),
            skip_model_init: false,
            tool_router: Self::annotated_router(),
        }
    }

    /// Create without model initialization (FTS-only). For tests and CI.
    #[doc(hidden)]
    pub fn new_without_models(db_path: PathBuf) -> Self {
        Self {
            db_path,
            embedder: Arc::new(Mutex::new(None)),
            reranker: Arc::new(Mutex::new(None)),
            reranker_failed_at: Arc::new(std::sync::atomic::AtomicI64::new(0)),
            hnsw: Arc::new(Mutex::new(None)),
            skip_model_init: true,
            tool_router: Self::annotated_router(),
        }
    }

    pub fn with_embedder(
        db_path: PathBuf,
        embedder: Arc<Mutex<Option<Embedder>>>,
        reranker: Arc<Mutex<Option<crate::rerank::Reranker>>>,
        reranker_failed_at: Arc<std::sync::atomic::AtomicI64>,
        hnsw: Arc<Mutex<Option<crate::code::hnsw::HnswIndex>>>,
    ) -> Self {
        Self {
            db_path,
            embedder,
            reranker,
            reranker_failed_at,
            hnsw,
            skip_model_init: false,
            tool_router: Self::annotated_router(),
        }
    }

    pub(crate) fn try_load_hnsw(db_path: &std::path::Path) -> Option<crate::code::hnsw::HnswIndex> {
        let hnsw_path = crate::code::hnsw::hnsw_path(db_path);
        if hnsw_path.exists() {
            match crate::code::hnsw::HnswIndex::load(&hnsw_path) {
                Ok(idx) => {
                    tracing::info!("loaded HNSW index ({} points)", idx.len());
                    Some(idx)
                }
                Err(e) => {
                    tracing::warn!("failed to load HNSW index: {e}");
                    Self::try_rebuild_hnsw_from_db(db_path)
                }
            }
        } else {
            Self::try_rebuild_hnsw_from_db(db_path)
        }
    }

    /// Attempt to rebuild HNSW from database embeddings (used on startup if HNSW is missing/corrupt).
    fn try_rebuild_hnsw_from_db(db_path: &std::path::Path) -> Option<crate::code::hnsw::HnswIndex> {
        let store = Store::open(db_path).ok()?;
        let rows = store.get_all_embeddings().ok()?;
        if rows.is_empty() {
            return None;
        }
        tracing::info!(
            "rebuilding HNSW from {} embeddings (file missing or corrupt)",
            rows.len()
        );
        match crate::code::hnsw::HnswIndex::from_embeddings(&rows) {
            Ok(index) => {
                let hnsw_path = crate::code::hnsw::hnsw_path(db_path);
                if let Err(e) = index.save(&hnsw_path) {
                    tracing::warn!("failed to save rebuilt HNSW: {e}");
                }
                tracing::info!("rebuilt HNSW index ({} points)", index.len());
                Some(index)
            }
            Err(e) => {
                tracing::warn!("failed to rebuild HNSW from embeddings: {e}");
                None
            }
        }
    }

    /// Build tool router with MCP annotations.
    fn annotated_router() -> rmcp::handler::server::router::tool::ToolRouter<Self> {
        let mut router = Self::tool_router();

        // All tools are non-destructive writers (search updates salience + logs)
        let write = ToolAnnotations::new().read_only(false).destructive(false);

        for (_name, route) in router.map.iter_mut() {
            route.attr.annotations = Some(write.clone());
        }

        router
    }

    /// Initialize embedder if not yet loaded. MUST be called inside spawn_blocking
    /// (not on an async thread) since it acquires a blocking std::sync::Mutex.
    fn init_embedder_blocking(embedder: &Arc<Mutex<Option<Embedder>>>) {
        let mut guard = match embedder.lock() {
            Ok(g) => g,
            Err(poisoned) => {
                tracing::warn!("embedder mutex poisoned, recovering: {poisoned}");
                poisoned.into_inner()
            }
        };
        if guard.is_none() {
            let cache_dir = crate::code::embed::default_cache_dir();
            match Embedder::new(&cache_dir) {
                Ok(e) => *guard = Some(e),
                Err(e) => tracing::warn!("embedder init failed (graceful degrade): {e}"),
            }
        }
    }

    fn epoch_secs() -> i64 {
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .map(|d| d.as_secs() as i64)
            .unwrap_or(0)
    }

    /// Initialize reranker if not yet loaded. MUST be called inside spawn_blocking.
    /// Circuit breaker: skips retry for 60s after a failure, then allows retry.
    fn init_reranker_blocking(
        reranker: &Arc<Mutex<Option<crate::rerank::Reranker>>>,
        failed_at: &Arc<std::sync::atomic::AtomicI64>,
    ) {
        const COOLDOWN_SECS: i64 = 60;
        let last_fail = failed_at.load(std::sync::atomic::Ordering::Relaxed);
        if last_fail > 0 && Self::epoch_secs() - last_fail < COOLDOWN_SECS {
            return; // Still in cooldown
        }
        let mut guard = match reranker.lock() {
            Ok(g) => g,
            Err(poisoned) => {
                tracing::warn!("reranker mutex poisoned, recovering: {poisoned}");
                poisoned.into_inner()
            }
        };
        if guard.is_none() {
            match crate::rerank::Reranker::new() {
                Ok(r) => {
                    failed_at.store(0, std::sync::atomic::Ordering::Relaxed);
                    *guard = Some(r);
                }
                Err(e) => {
                    tracing::warn!("reranker init failed (retry in {COOLDOWN_SECS}s): {e}");
                    failed_at.store(Self::epoch_secs(), std::sync::atomic::Ordering::Relaxed);
                }
            }
        }
    }

    #[tool(
        name = "search",
        description = "Search across code and memory, navigate symbols, map codebases, or analyze impact. Default mode searches with hybrid retrieval (keywords + semantics + reranking). Use mode='navigate' to find symbol definitions/references, mode='map' for codebase overview, mode='impact' to analyze change blast radius."
    )]
    pub async fn search(
        &self,
        Parameters(params): Parameters<SearchParams>,
    ) -> Result<CallToolResult, rmcp::ErrorData> {
        let db_path = self.db_path.clone();
        let embedder = Arc::clone(&self.embedder);
        let reranker = Arc::clone(&self.reranker);
        let rr_failed = Arc::clone(&self.reranker_failed_at);
        let hnsw = Arc::clone(&self.hnsw);
        let skip_models = self.skip_model_init;

        let result = tokio::task::spawn_blocking(move || {
            let mode = params.mode.as_deref().unwrap_or("search");
            match mode {
                "search" => {
                    if !skip_models {
                        Self::init_embedder_blocking(&embedder);
                        Self::init_reranker_blocking(&reranker, &rr_failed);
                    }
                    let store = Store::open(&db_path)?;
                    let kind_filter = match params.kind.as_deref() {
                        Some("all") | None => None,
                        Some(k @ ("code" | "memory")) => Some(k),
                        Some(k) => {
                            anyhow::bail!("invalid kind '{k}': must be 'all', 'code', or 'memory'")
                        }
                    };
                    let limit = params.limit.unwrap_or(10).clamp(1, 100);
                    let threshold = params.threshold;
                    let mut emb_guard =
                        embedder.lock().map_err(|e| anyhow::anyhow!("lock: {e}"))?;
                    let mut rr_guard = reranker.lock().map_err(|e| anyhow::anyhow!("lock: {e}"))?;
                    let hnsw_guard = hnsw.lock().map_err(|e| anyhow::anyhow!("lock: {e}"))?;
                    let result = crate::search::unified_search(crate::search::SearchContext {
                        store: &store,
                        query: &params.query,
                        kind_filter,
                        limit,
                        threshold,
                        embedder: emb_guard.as_mut(),
                        reranker: rr_guard.as_mut(),
                        hnsw: hnsw_guard.as_ref(),
                    })?;
                    let json = serde_json::to_string_pretty(&format_search_hits(&result.hits))?;
                    Ok::<_, anyhow::Error>(json)
                }
                "navigate" => {
                    let store = Store::open(&db_path)?;
                    let codebase_id = store
                        .resolve_codebase(params.dir.as_deref())?
                        .context("no codebases indexed — run index first")?;
                    let codebase_id = Some(codebase_id);
                    let direction = params.direction.as_deref().unwrap_or("both");

                    let (show_defs, show_refs) = match direction {
                        "both" => (true, true),
                        "defs" | "def" => (true, false),
                        "refs" | "ref" => (false, true),
                        d => anyhow::bail!(
                            "invalid direction '{d}': must be 'both', 'defs', or 'refs'"
                        ),
                    };

                    let mut output = String::new();

                    if show_defs {
                        let defs = store.find_definitions(&params.query, codebase_id)?;
                        if !defs.is_empty() {
                            output.push_str(&format!("Definitions of '{}':\n", params.query));
                            for d in &defs {
                                output.push_str(&format!(
                                    "  {}:{} ({} {})\n",
                                    d.file_path, d.line, d.kind, d.symbol
                                ));
                            }
                        }
                    }

                    if show_refs {
                        let refs = store.find_references(&params.query, codebase_id)?;
                        if !refs.is_empty() {
                            if !output.is_empty() {
                                output.push('\n');
                            }
                            output.push_str(&format!("References to '{}':\n", params.query));
                            for r in &refs {
                                output.push_str(&format!(
                                    "  {}:{} ({} {})\n",
                                    r.file_path, r.line, r.kind, r.symbol
                                ));
                            }
                        }
                    }

                    if output.is_empty() {
                        output = format!("No results found for '{}'.", params.query);
                    }

                    Ok(output)
                }
                "map" => {
                    let store = Store::open(&db_path)?;
                    let budget = params.budget.unwrap_or(4000).clamp(1, 200_000);
                    let codebase_id = store
                        .resolve_codebase(params.dir.as_deref())?
                        .context("no codebases indexed — run index first")?;
                    crate::search::generate_map(&store, codebase_id, budget)
                }
                "impact" => {
                    let store = Store::open(&db_path)?;
                    let codebase_id = store
                        .resolve_codebase(params.dir.as_deref())?
                        .context("no codebases indexed — run index first")?;
                    let codebase_id = Some(codebase_id);
                    let max_depth = params.depth.unwrap_or(2).clamp(1, 5);

                    let hits = store.find_impact(&params.query, codebase_id, max_depth)?;

                    if hits.is_empty() {
                        return Ok(format!("No impact found for '{}'.", params.query));
                    }

                    let mut output = String::new();
                    let defs = store.find_definitions(&params.query, codebase_id)?;
                    if !defs.is_empty() {
                        let locs: Vec<String> = defs
                            .iter()
                            .map(|d| format!("{}:{}", d.file_path, d.line))
                            .collect();
                        output.push_str(&format!(
                            "Impact of changing '{}' (defined at {}):\n",
                            params.query,
                            locs.join(", ")
                        ));
                    } else {
                        output.push_str(&format!("Impact of changing '{}':\n", params.query));
                    }

                    let mut current_depth = 0;
                    for h in &hits {
                        if h.depth != current_depth {
                            current_depth = h.depth;
                            output.push_str(&format!(
                                "\n  Depth {} ({}):\n",
                                current_depth,
                                if current_depth == 1 {
                                    "direct"
                                } else {
                                    "transitive"
                                },
                            ));
                        }
                        output.push_str(&format!("    {} (via {})\n", h.file_path, h.via_symbol));
                    }

                    let file_count = hits.len();
                    output.push_str(&format!("\n{file_count} file(s) affected."));
                    Ok(output)
                }
                m => anyhow::bail!(
                    "invalid mode '{m}': must be 'search', 'navigate', 'map', or 'impact'"
                ),
            }
        })
        .await
        .map_err(|e| rmcp::ErrorData::internal_error(format!("task join: {e}"), None))?;

        match result {
            Ok(text) => Ok(CallToolResult::success(vec![Content::text(text)])),
            Err(e) => {
                let mut result = CallToolResult::success(vec![Content::text(format!("{e:#}"))]);
                result.is_error = Some(true);
                Ok(result)
            }
        }
    }

    #[tool(
        name = "index",
        description = "Index a source code directory for search and navigation. Indexes all non-hidden text files (<1MB) regardless of language — extracts symbols and structure using universal heuristics. Respects .gitignore. Incremental — only re-indexes changed files. Run once per codebase, then use search/navigate/map/impact."
    )]
    pub async fn index_dir(
        &self,
        Parameters(params): Parameters<IndexParams>,
    ) -> Result<CallToolResult, rmcp::ErrorData> {
        let db_path = self.db_path.clone();
        let embedder = Arc::clone(&self.embedder);
        let hnsw = Arc::clone(&self.hnsw);
        let embed = params.embed.unwrap_or(false);

        let result = tokio::task::spawn_blocking(move || {
            let store = Store::open(&db_path)?;
            let dir = std::path::Path::new(&params.directory);
            let result = crate::index::index_directory(&store, dir)?;

            // Invalidate stale HNSW when files changed (embeddings cleared on re-index)
            if (result.files_changed > 0 || result.files_removed > 0)
                && let Ok(mut hnsw_guard) = hnsw.lock()
                && hnsw_guard.is_some()
            {
                *hnsw_guard = None;
                let hnsw_file = crate::code::hnsw::hnsw_path(&db_path);
                if hnsw_file.exists() {
                    let _ = std::fs::remove_file(&hnsw_file);
                }
                tracing::debug!(
                    "invalidated HNSW after re-index ({} changed, {} removed)",
                    result.files_changed,
                    result.files_removed
                );
            }

            let mut output = serde_json::json!({
                "files_scanned": result.files_scanned,
                "files_changed": result.files_changed,
                "files_skipped": result.files_skipped,
                "files_removed": result.files_removed,
                "chunks_written": result.chunks_written,
                "duration_ms": result.duration_ms,
                "errors": result.errors,
            });

            if embed {
                Self::init_embedder_blocking(&embedder);
                let mut guard = embedder.lock().map_err(|e| anyhow::anyhow!("lock: {e}"))?;
                if let Some(emb) = guard.as_mut() {
                    let embedded = crate::index::embed_codebase(&store, emb, result.codebase_id)?;
                    output["embedded"] = serde_json::json!(embedded);

                    // Rebuild HNSW so searches immediately reflect new embeddings
                    let rows = store.get_all_embeddings()?;
                    if !rows.is_empty() {
                        match crate::code::hnsw::HnswIndex::from_embeddings(&rows) {
                            Ok(new_index) => {
                                let hnsw_file = crate::code::hnsw::hnsw_path(&db_path);
                                if let Err(e) = new_index.save(&hnsw_file) {
                                    tracing::warn!("failed to save HNSW after index: {e}");
                                }
                                let mut hnsw_guard =
                                    hnsw.lock().map_err(|e| anyhow::anyhow!("hnsw lock: {e}"))?;
                                *hnsw_guard = Some(new_index);
                                output["hnsw_rebuilt"] = serde_json::json!(rows.len());
                            }
                            Err(e) => {
                                tracing::warn!("failed to rebuild HNSW after index: {e}");
                                output["hnsw_warning"] =
                                    serde_json::json!(format!("rebuild failed: {e}"));
                            }
                        }
                    }
                } else {
                    output["embed_warning"] = serde_json::json!("embedder not available, skipping");
                }
            }

            Ok::<_, anyhow::Error>(serde_json::to_string_pretty(&output)?)
        })
        .await
        .map_err(|e| rmcp::ErrorData::internal_error(format!("task join: {e}"), None))?;

        match result {
            Ok(json) => Ok(CallToolResult::success(vec![Content::text(json)])),
            Err(e) => {
                let mut result = CallToolResult::success(vec![Content::text(format!("{e:#}"))]);
                result.is_error = Some(true);
                Ok(result)
            }
        }
    }

    #[tool(
        name = "store",
        description = "Store raw content as a persistent memory. Deduplicates — if a near-duplicate exists, it updates the existing entry instead of creating a new one. Use proactively to save anything worth persisting across sessions: facts, decisions, preferences, observations."
    )]
    pub async fn store(
        &self,
        Parameters(params): Parameters<StoreParams>,
    ) -> Result<CallToolResult, rmcp::ErrorData> {
        let db_path = self.db_path.clone();
        let embedder = Arc::clone(&self.embedder);
        let skip_models = self.skip_model_init;

        let result = tokio::task::spawn_blocking(move || {
            if !skip_models {
                Self::init_embedder_blocking(&embedder);
            }
            let store = Store::open(&db_path)?;
            let mut guard = embedder.lock().map_err(|e| anyhow::anyhow!("lock: {e}"))?;
            let tags = params.tags.as_deref().unwrap_or("");
            let result = crate::memory::store(
                &store,
                guard.as_mut(),
                &params.content,
                params.title.as_deref(),
                tags,
                params.memory_type.as_deref(),
            )?;

            // Keep stale HNSW — new memory is in FTS5, so hybrid search still finds it.
            // Destroying HNSW here would force O(N) brute-force for ALL future queries.

            let output = serde_json::json!({
                "id": result.id,
                "title": result.title,
                "was_update": result.was_update,
                "similar_id": result.similar_id,
            });
            Ok::<_, anyhow::Error>(serde_json::to_string_pretty(&output)?)
        })
        .await
        .map_err(|e| rmcp::ErrorData::internal_error(format!("task join: {e}"), None))?;

        match result {
            Ok(json) => Ok(CallToolResult::success(vec![Content::text(json)])),
            Err(e) => {
                let mut result = CallToolResult::success(vec![Content::text(format!("{e:#}"))]);
                result.is_error = Some(true);
                Ok(result)
            }
        }
    }
}

#[tool_handler]
impl ServerHandler for GrasshopperMcp {
    fn get_info(&self) -> ServerInfo {
        ServerInfo {
            instructions: Some(
                "Grasshopper is a persistent retrieval engine for AI agents.\n\n\
                 3 TOOLS:\n\
                 - search: find code, memories, symbols, codebase maps, or impact analysis (use mode parameter)\n\
                 - index: index a source code directory for search and navigation\n\
                 - store: persist facts, decisions, preferences (auto-deduplicates)\n\n\
                 SEARCH MODES:\n\
                 - mode=search (default): hybrid retrieval across code and memory\n\
                 - mode=navigate: find symbol definitions/references (query = symbol name)\n\
                 - mode=map: get a token-budgeted codebase overview\n\
                 - mode=impact: analyze change blast radius (query = symbol name)"
                    .into(),
            ),
            capabilities: ServerCapabilities::builder().enable_tools().build(),
            ..Default::default()
        }
    }
}
