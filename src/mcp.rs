use anyhow::{Context, Result};
use rmcp::handler::server::router::tool::ToolRouter;
use rmcp::handler::server::wrapper::Parameters;
use rmcp::model::{CallToolResult, Content, ServerCapabilities, ServerInfo, ToolAnnotations};
use rmcp::{tool, tool_handler, tool_router, ServerHandler};
use schemars::JsonSchema;
use serde::Deserialize;
use std::collections::BTreeMap;
use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex};

use crate::store::Store;
use crate::code::embed::Embedder;

// --- Embedding cache ---

#[allow(dead_code)]
struct EmbedCache {
    entries: std::collections::HashMap<String, (std::time::Instant, Vec<f32>)>,
    ttl: std::time::Duration,
    max_entries: usize,
}

#[allow(dead_code)]
impl EmbedCache {
    fn new(ttl_secs: u64, max_entries: usize) -> Self {
        Self {
            entries: std::collections::HashMap::new(),
            ttl: std::time::Duration::from_secs(ttl_secs),
            max_entries,
        }
    }

    fn get(&self, query: &str) -> Option<Vec<f32>> {
        self.entries.get(query).and_then(|(instant, vec)| {
            if instant.elapsed() < self.ttl {
                Some(vec.clone())
            } else {
                None
            }
        })
    }

    fn insert(&mut self, query: String, embedding: Vec<f32>) {
        // Evict expired entries if at capacity
        if self.entries.len() >= self.max_entries {
            let ttl = self.ttl;
            self.entries.retain(|_, (instant, _)| instant.elapsed() < ttl);
        }
        // If still at capacity, evict oldest
        if self.entries.len() >= self.max_entries
            && let Some(oldest_key) = self.entries.iter()
                .min_by_key(|(_, (instant, _))| *instant)
                .map(|(k, _)| k.clone())
        {
            self.entries.remove(&oldest_key);
        }
        self.entries.insert(query, (std::time::Instant::now(), embedding));
    }
}

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
    #[allow(dead_code)]
    embed_cache: Arc<Mutex<EmbedCache>>,
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
            embed_cache: Arc::new(Mutex::new(EmbedCache::new(60, 100))),
            tool_router: Self::annotated_router(),
        }
    }

    pub fn with_embedder(
        db_path: PathBuf,
        embedder: Arc<Mutex<Option<Embedder>>>,
        reranker: Arc<Mutex<Option<crate::rerank::Reranker>>>,
        hnsw: Arc<Mutex<Option<crate::code::hnsw::HnswIndex>>>,
    ) -> Self {
        Self {
            db_path,
            embedder,
            reranker,
            reranker_failed_at: Arc::new(std::sync::atomic::AtomicI64::new(0)),
            hnsw,
            embed_cache: Arc::new(Mutex::new(EmbedCache::new(60, 100))),
            tool_router: Self::annotated_router(),
        }
    }

    fn try_load_hnsw(db_path: &std::path::Path) -> Option<crate::code::hnsw::HnswIndex> {
        let hnsw_path = crate::code::hnsw::hnsw_path(db_path);
        if hnsw_path.exists() {
            match crate::code::hnsw::HnswIndex::load(&hnsw_path) {
                Ok(idx) => {
                    tracing::info!("loaded HNSW index ({} points)", idx.len());
                    Some(idx)
                }
                Err(e) => {
                    tracing::warn!("failed to load HNSW index: {e}");
                    // If load fails but embeddings exist, rebuild from scratch
                    Self::try_rebuild_hnsw_from_db(db_path)
                }
            }
        } else {
            // If HNSW file doesn't exist but embeddings do, build automatically
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
        tracing::info!("rebuilding HNSW from {} embeddings (file missing or corrupt)", rows.len());
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

    /// Build tool router with MCP annotations so clients can categorize tools
    /// into read-only vs write/delete groups.
    fn annotated_router() -> rmcp::handler::server::router::tool::ToolRouter<Self> {
        let mut router = Self::tool_router();

        let read_only = ToolAnnotations::new()
            .read_only(true)
            .destructive(false);
        let write = ToolAnnotations::new()
            .read_only(false)
            .destructive(false);

        for (name, route) in router.map.iter_mut() {
            let ann = match name.as_ref() {
                // Read-only: search (touch_memory side effects are acceptable)
                "search" => read_only.clone(),
                // Write (non-destructive): index, store
                _ => write.clone(),
            };
            route.attr.annotations = Some(ann);
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

    /// Initialize reranker if not yet loaded. MUST be called inside spawn_blocking.
    /// Circuit breaker: skips retry for 60s after a failure, then allows retry.
    fn init_reranker_blocking(
        reranker: &Arc<Mutex<Option<crate::rerank::Reranker>>>,
        failed_at: &Arc<std::sync::atomic::AtomicI64>,
    ) {
        const COOLDOWN_SECS: i64 = 60;
        let last_fail = failed_at.load(std::sync::atomic::Ordering::Relaxed);
        if last_fail > 0 {
            let now = std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .map(|d| d.as_secs() as i64)
                .unwrap_or(0);
            if now - last_fail < COOLDOWN_SECS {
                return; // Still in cooldown
            }
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
                    let now = std::time::SystemTime::now()
                        .duration_since(std::time::UNIX_EPOCH)
                        .map(|d| d.as_secs() as i64)
                        .unwrap_or(0);
                    tracing::warn!("reranker init failed (retry in {COOLDOWN_SECS}s): {e}");
                    failed_at.store(now, std::sync::atomic::Ordering::Relaxed);
                }
            }
        }
    }

    #[tool(
        name = "search",
        description = "Search across code and memory, navigate symbols, map codebases, or analyze impact. Default mode searches with hybrid retrieval (keywords + semantics + reranking). Use mode='navigate' to find symbol definitions/references, mode='map' for codebase overview, mode='impact' to analyze change blast radius."
    )]
    async fn search(
        &self,
        Parameters(params): Parameters<SearchParams>,
    ) -> Result<CallToolResult, rmcp::ErrorData> {
        let db_path = self.db_path.clone();
        let embedder = Arc::clone(&self.embedder);
        let reranker = Arc::clone(&self.reranker);
        let rr_failed = Arc::clone(&self.reranker_failed_at);
        let hnsw = Arc::clone(&self.hnsw);

        let result = tokio::task::spawn_blocking(move || {
            let mode = params.mode.as_deref().unwrap_or("search");
            match mode {
                "search" => {
                    Self::init_embedder_blocking(&embedder);
                    Self::init_reranker_blocking(&reranker, &rr_failed);
                    let store = Store::open(&db_path)?;
                    let kind_filter = match params.kind.as_deref() {
                        Some("all") | None => None,
                        Some(k @ ("code" | "memory")) => Some(k),
                        Some(k) => anyhow::bail!("invalid kind '{k}': must be 'all', 'code', or 'memory'"),
                    };
                    let limit = params.limit.unwrap_or(10).clamp(1, 100);
                    let threshold = params.threshold;
                    let mut emb_guard = embedder.lock().map_err(|e| anyhow::anyhow!("lock: {e}"))?;
                    let mut rr_guard = reranker.lock().map_err(|e| anyhow::anyhow!("lock: {e}"))?;
                    let hnsw_guard = hnsw.lock().map_err(|e| anyhow::anyhow!("lock: {e}"))?;
                    let result = crate::search::unified_search(
                        &store, &params.query, kind_filter, limit,
                        threshold,
                        emb_guard.as_mut(), rr_guard.as_mut(),
                        hnsw_guard.as_ref(),
                    )?;
                    let json = serde_json::to_string_pretty(&format_search_hits(&result.hits))?;
                    Ok::<_, anyhow::Error>(json)
                }
                "navigate" => {
                    let store = Store::open(&db_path)?;
                    let codebase_id = resolve_codebase(&store, params.dir.as_deref())?
                        .context("no codebases indexed — run index first")?;
                    let codebase_id = Some(codebase_id);
                    let direction = params.direction.as_deref().unwrap_or("both");

                    let (show_defs, show_refs) = match direction {
                        "both" => (true, true),
                        "defs" | "def" => (true, false),
                        "refs" | "ref" => (false, true),
                        d => anyhow::bail!("invalid direction '{d}': must be 'both', 'defs', or 'refs'"),
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
                    let codebase_id = resolve_codebase(&store, params.dir.as_deref())?
                        .context("no codebases indexed — run index first")?;
                    generate_map(&store, codebase_id, budget)
                }
                "impact" => {
                    let store = Store::open(&db_path)?;
                    let codebase_id = resolve_codebase(&store, params.dir.as_deref())?
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
                        let locs: Vec<String> =
                            defs.iter().map(|d| format!("{}:{}", d.file_path, d.line)).collect();
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
                                if current_depth == 1 { "direct" } else { "transitive" },
                            ));
                        }
                        output.push_str(&format!("    {} (via {})\n", h.file_path, h.via_symbol));
                    }

                    let file_count = hits.len();
                    output.push_str(&format!("\n{file_count} file(s) affected."));
                    Ok(output)
                }
                m => anyhow::bail!("invalid mode '{m}': must be 'search', 'navigate', 'map', or 'impact'"),
            }
        })
        .await
        .map_err(|e| rmcp::ErrorData::internal_error(format!("task join: {e}"), None))?;

        match result {
            Ok(text) => Ok(CallToolResult::success(vec![Content::text(text)])),
            Err(e) => Ok(error_result(format!("{e:#}"))),
        }
    }

    #[tool(
        name = "index",
        description = "Index a source code directory for search and navigation. Indexes all non-hidden text files (<1MB) regardless of language — extracts symbols and structure using universal heuristics. Respects .gitignore. Incremental — only re-indexes changed files. Run once per codebase, then use search/navigate/map/impact."
    )]
    async fn index_dir(
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
                    let embedded =
                        crate::index::embed_codebase(&store, emb, result.codebase_id)?;
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
                                let mut hnsw_guard = hnsw.lock()
                                    .map_err(|e| anyhow::anyhow!("hnsw lock: {e}"))?;
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
                    output["embed_warning"] =
                        serde_json::json!("embedder not available, skipping");
                }
            }

            Ok::<_, anyhow::Error>(serde_json::to_string_pretty(&output)?)
        })
        .await
        .map_err(|e| rmcp::ErrorData::internal_error(format!("task join: {e}"), None))?;

        match result {
            Ok(json) => Ok(CallToolResult::success(vec![Content::text(json)])),
            Err(e) => Ok(error_result(format!("{e:#}"))),
        }
    }

    #[tool(
        name = "store",
        description = "Store raw content as a persistent memory. Deduplicates — if a near-duplicate exists, it updates the existing entry instead of creating a new one. Use proactively to save anything worth persisting across sessions: facts, decisions, preferences, observations."
    )]
    async fn store(
        &self,
        Parameters(params): Parameters<StoreParams>,
    ) -> Result<CallToolResult, rmcp::ErrorData> {
        let db_path = self.db_path.clone();
        let embedder = Arc::clone(&self.embedder);

        let result = tokio::task::spawn_blocking(move || {
            Self::init_embedder_blocking(&embedder);
            let store = Store::open(&db_path)?;
            let mut guard = embedder.lock().map_err(|e| anyhow::anyhow!("lock: {e}"))?;
            let tags = params.tags.as_deref().unwrap_or("");
            let result = crate::memory::store(
                &store,
                guard.as_mut(),
                &params.content,
                params.title.as_deref(),
                tags,
            )?;

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
            Err(e) => Ok(error_result(format!("{e:#}"))),
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

// --- Transport entry points ---

/// Run startup maintenance: log DB stats, PRAGMA optimize.
fn startup_maintenance(db_path: &Path) {
    match Store::open(db_path) {
        Ok(store) => {
            let (code, memory) = store.count_by_kind().unwrap_or((0, 0));
            let db_bytes = store.db_size_bytes().unwrap_or(0);
            let codebases = store.count_codebases().unwrap_or(0);
            tracing::info!(
                "startup: {} code chunks, {} memories, {:.1} KB, {} codebases",
                code, memory, db_bytes as f64 / 1024.0, codebases
            );
            if let Err(e) = store.optimize() {
                tracing::warn!("startup optimize failed: {e}");
            }
        }
        Err(e) => tracing::warn!("startup diagnostics failed: {e}"),
    }
}

/// Run the MCP server over stdio (for direct Claude Code integration).
pub async fn run_stdio(db_path: PathBuf) -> Result<()> {
    let _ = tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::from_default_env()
                .add_directive(tracing::Level::INFO.into())
                .add_directive("ort=warn".parse().unwrap()),
        )
        .with_writer(std::io::stderr)
        .with_target(false)
        .with_ansi(false)
        .try_init();

    tracing::info!("starting grasshopper MCP server (stdio)");
    startup_maintenance(&db_path);
    let server = GrasshopperMcp::new(db_path);
    let service = rmcp::ServiceExt::serve(server, rmcp::transport::stdio()).await?;
    service.waiting().await?;
    Ok(())
}

/// Run the MCP server over HTTP (daemon mode).
pub async fn run_http(db_path: PathBuf, port: u16) -> Result<()> {
    let _ = tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::from_default_env()
                .add_directive(tracing::Level::INFO.into())
                .add_directive("ort=warn".parse().unwrap()),
        )
        .with_target(false)
        .try_init();

    use rmcp::transport::streamable_http_server::{
        session::local::LocalSessionManager, StreamableHttpServerConfig, StreamableHttpService,
    };
    use tower_http::cors::CorsLayer;

    let ct = tokio_util::sync::CancellationToken::new();
    let shared_embedder: Arc<Mutex<Option<Embedder>>> = Arc::new(Mutex::new(None));
    let shared_reranker: Arc<Mutex<Option<crate::rerank::Reranker>>> = Arc::new(Mutex::new(None));
    let shared_hnsw: Arc<Mutex<Option<crate::code::hnsw::HnswIndex>>> =
        Arc::new(Mutex::new(GrasshopperMcp::try_load_hnsw(&db_path)));

    startup_maintenance(&db_path);

    let db = db_path.clone();
    let embedder_for_mcp = shared_embedder.clone();
    let reranker_for_mcp = shared_reranker.clone();
    let hnsw_for_mcp = shared_hnsw.clone();
    let mcp_service = StreamableHttpService::new(
        move || {
            Ok(GrasshopperMcp::with_embedder(
                db.clone(),
                embedder_for_mcp.clone(),
                reranker_for_mcp.clone(),
                hnsw_for_mcp.clone(),
            ))
        },
        LocalSessionManager::default().into(),
        StreamableHttpServerConfig {
            cancellation_token: ct.child_token(),
            ..Default::default()
        },
    );

    // No permissive CORS — MCP clients (Claude Code, agents) don't use browser fetch.
    // Only allow the specific headers MCP protocol needs, no wildcard origins.
    let cors = CorsLayer::new()
        .allow_methods([
            http::Method::GET,
            http::Method::POST,
            http::Method::DELETE,
            http::Method::OPTIONS,
        ])
        .allow_headers([
            http::header::CONTENT_TYPE,
            http::header::ACCEPT,
            http::HeaderName::from_static("mcp-session-id"),
            http::HeaderName::from_static("mcp-protocol-version"),
        ])
        .expose_headers([
            http::header::CONTENT_TYPE,
            http::HeaderName::from_static("mcp-session-id"),
            http::HeaderName::from_static("mcp-protocol-version"),
        ]);

    // Prevent Cloudflare edge from Brotli-compressing streaming MCP responses
    let no_transform = axum::middleware::from_fn(
        |req: axum::extract::Request, next: axum::middleware::Next| async move {
            let mut res = next.run(req).await;
            res.headers_mut().insert(
                http::header::CACHE_CONTROL,
                http::HeaderValue::from_static("no-transform"),
            );
            res
        },
    );

    let router = axum::Router::new()
        .route("/healthz", axum::routing::get(|| async { "ok" }))
        .nest_service("/mcp", mcp_service)
        .layer(cors)
        .layer(no_transform);

    let addr = format!("127.0.0.1:{port}");
    tracing::info!("starting grasshopper MCP server on http://{addr}/mcp");
    tracing::info!("health check at http://{addr}/healthz");

    let db_path_for_shutdown = db_path.clone();
    let listener = tokio::net::TcpListener::bind(&addr)
        .await
        .with_context(|| format!("binding to {addr}"))?;

    axum::serve(listener, router)
        .with_graceful_shutdown(async move {
            tokio::signal::ctrl_c().await.ok();
            tracing::info!("shutting down");
            // Run PRAGMA optimize on shutdown for query planner stats
            if let Ok(store) = Store::open(&db_path_for_shutdown)
                && let Err(e) = store.optimize()
            {
                tracing::warn!("shutdown optimize failed: {e}");
            }
            ct.cancel();
        })
        .await?;

    Ok(())
}

// --- Helper functions ---

/// Format SearchHit list into serializable JSON values.
fn format_search_hits(hits: &[crate::store::SearchHit]) -> Vec<serde_json::Value> {
    hits.iter()
        .map(|h| {
            let mut v = serde_json::json!({
                "id": h.id,
                "kind": h.kind,
                "title": h.title,
                "score": h.score,
            });
            if let Some(ref fp) = h.file_path {
                v["file_path"] = serde_json::json!(fp);
            }
            if let Some(ref sn) = h.symbol_name {
                v["symbol_name"] = serde_json::json!(sn);
            }
            if let Some(ref sk) = h.symbol_kind {
                v["symbol_kind"] = serde_json::json!(sk);
            }
            if let Some(ref sig) = h.signature {
                v["signature"] = serde_json::json!(sig);
            }
            if let Some(sl) = h.start_line {
                v["start_line"] = serde_json::json!(sl);
            }
            if let Some(el) = h.end_line {
                v["end_line"] = serde_json::json!(el);
            }
            if let Some(ref mt) = h.memory_type {
                v["memory_type"] = serde_json::json!(mt);
            }
            if h.access_count > 0 {
                v["access_count"] = serde_json::json!(h.access_count);
            }
            if (h.salience - 0.5).abs() > f64::EPSILON {
                v["salience"] = serde_json::json!(h.salience);
            }
            if !h.snippet.is_empty() {
                let preview: String = h.snippet.chars().take(200).collect();
                v["snippet"] = serde_json::json!(preview);
            }
            v
        })
        .collect()
}

/// Resolve a codebase directory filter to a codebase_id.
/// If multiple codebases are indexed and no dir is specified, returns an error
/// listing available codebases (consistent policy across all code-intel tools).
fn resolve_codebase(store: &Store, dir: Option<&str>) -> Result<Option<i64>> {
    let codebases = store.list_codebases()?;
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
            "Multiple codebases indexed. Use dir parameter to select one: {}",
            names.join(", ")
        );
    }
}

/// Public wrapper for CLI access to codebase map generation.
pub fn generate_map_cli(store: &Store, codebase_id: i64, token_budget: usize) -> Result<String> {
    generate_map(store, codebase_id, token_budget)
}

/// Generate a compact codebase map from definitions, ranked by definition density per file.
fn generate_map(store: &Store, codebase_id: i64, token_budget: usize) -> Result<String> {
    let definitions = store.get_all_definitions(codebase_id)?;
    let def_counts = store.count_definitions_per_file(codebase_id)?;

    if definitions.is_empty() {
        return Ok("No definitions found. Run index first.".into());
    }

    // Group definitions by file
    let mut by_file: BTreeMap<&str, Vec<&crate::store::GraphEdge>> = BTreeMap::new();
    for edge in &definitions {
        by_file.entry(&edge.file_path).or_default().push(edge);
    }

    // Score each file by definition count (more definitions = likely more important)
    let mut file_scores: Vec<(&str, i64)> = by_file
        .keys()
        .map(|&file| {
            let score = *def_counts.get(file).unwrap_or(&0) as i64;
            (file, score)
        })
        .collect();
    file_scores.sort_by(|a, b| b.1.cmp(&a.1));

    // Build the map, respecting token budget (~4 chars per token)
    let char_budget = token_budget.saturating_mul(4);
    if char_budget == 0 {
        return Ok(String::new());
    }
    let mut output = String::new();
    let mut chars_used = 0;

    for (file, _score) in &file_scores {
        let defs = &by_file[file];
        let mut section = format!("{file}\n");
        for def in defs {
            let prefix = kind_prefix(&def.kind);
            let line = format!("  {prefix} {}\n", def.symbol);
            section.push_str(&line);
        }

        if chars_used + section.len() > char_budget && chars_used > 0 {
            break;
        }

        output.push_str(&section);
        chars_used += section.len();
    }

    Ok(output)
}

fn kind_prefix(kind: &str) -> &str {
    match kind {
        "function" | "method" => "fn",
        "class" | "struct" => "struct",
        "interface" | "trait" => "trait",
        "module" => "mod",
        "macro" => "macro",
        "constant" => "const",
        "variable" => "let",
        "type" => "type",
        "implementation" => "impl",
        "enum" => "enum",
        other => other,
    }
}

/// Create an error CallToolResult.
fn error_result(msg: String) -> CallToolResult {
    let mut result = CallToolResult::success(vec![Content::text(msg)]);
    result.is_error = Some(true);
    result
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_embed_cache_insert_and_get() {
        let mut cache = EmbedCache::new(60, 10);
        cache.insert("hello".to_string(), vec![1.0, 2.0, 3.0]);
        let result = cache.get("hello");
        assert!(result.is_some());
        assert_eq!(result.unwrap(), vec![1.0, 2.0, 3.0]);
    }

    #[test]
    fn test_embed_cache_miss() {
        let cache = EmbedCache::new(60, 10);
        assert!(cache.get("missing").is_none());
    }

    #[test]
    fn test_embed_cache_expiry() {
        let mut cache = EmbedCache::new(0, 10); // 0s TTL = immediate expiry
        cache.insert("hello".to_string(), vec![1.0]);
        // With 0s TTL, elapsed >= ttl immediately
        std::thread::sleep(std::time::Duration::from_millis(1));
        assert!(cache.get("hello").is_none());
    }

    #[test]
    fn test_embed_cache_eviction_at_capacity() {
        let mut cache = EmbedCache::new(60, 2);
        cache.insert("first".to_string(), vec![1.0]);
        std::thread::sleep(std::time::Duration::from_millis(1));
        cache.insert("second".to_string(), vec![2.0]);
        std::thread::sleep(std::time::Duration::from_millis(1));
        // Third insert should evict "first" (oldest)
        cache.insert("third".to_string(), vec![3.0]);
        assert!(cache.get("first").is_none());
        assert!(cache.get("second").is_some());
        assert!(cache.get("third").is_some());
    }
}
