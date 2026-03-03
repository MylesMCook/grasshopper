use anyhow::{Context, Result};
use rmcp::handler::server::router::tool::ToolRouter;
use rmcp::handler::server::wrapper::Parameters;
use rmcp::model::{CallToolResult, Content, ServerCapabilities, ServerInfo};
use rmcp::{tool, tool_handler, tool_router, ServerHandler};
use schemars::JsonSchema;
use serde::Deserialize;
use std::collections::BTreeMap;
use std::path::PathBuf;
use std::sync::{Arc, Mutex};

use crate::store::Store;
use ferret::embed::Embedder;

// --- Tool parameter types ---

#[derive(Debug, Deserialize, JsonSchema)]
pub struct SearchParams {
    /// Search query string
    pub query: String,
    /// Filter results: "all" (default), "code", or "memory"
    pub kind: Option<String>,
    /// Maximum number of results (default: 10)
    pub limit: Option<usize>,
}

#[derive(Debug, Deserialize, JsonSchema)]
pub struct IndexParams {
    /// Directory path to index (absolute path)
    pub directory: String,
    /// Generate embeddings for semantic search (slower, ~4/sec on CPU)
    pub embed: Option<bool>,
}

#[derive(Debug, Deserialize, JsonSchema)]
pub struct NavigateParams {
    /// Symbol name to look up
    pub symbol: String,
    /// Direction: "refs" (references only), "defs" (definitions only), or "both" (default)
    pub direction: Option<String>,
    /// Filter results to files under this directory path
    pub dir: Option<String>,
}

#[derive(Debug, Deserialize, JsonSchema)]
pub struct MapParams {
    /// Filter to a specific codebase directory path
    pub dir: Option<String>,
    /// Token budget for output (default: 4000)
    pub budget: Option<usize>,
}

#[derive(Debug, Deserialize, JsonSchema)]
pub struct ImpactParams {
    /// Symbol name to analyze
    pub symbol: String,
    /// Maximum BFS depth (default: 2)
    pub depth: Option<usize>,
    /// Filter results to files under this directory path
    pub dir: Option<String>,
}

#[derive(Debug, Deserialize, JsonSchema)]
pub struct RememberParams {
    /// Memory content to store
    pub content: String,
    /// Title (auto-generated from content if omitted)
    pub title: Option<String>,
    /// Memory type: identity, knowledge, episode, procedure (auto-classified if omitted)
    pub r#type: Option<String>,
    /// Comma-separated descriptor tags
    pub tags: Option<String>,
}

#[derive(Debug, Deserialize, JsonSchema)]
pub struct MeParams {}

#[derive(Debug, Deserialize, JsonSchema)]
pub struct PickupParams {
    /// Filter by project name
    pub project: Option<String>,
}

#[derive(Debug, Deserialize, JsonSchema)]
pub struct HandoffParams {
    /// Project name this handoff belongs to
    pub project: String,
    /// Summary of what was accomplished
    pub summary: String,
    /// What should happen next
    pub next_steps: String,
}

#[derive(Debug, Deserialize, JsonSchema)]
pub struct ConsolidateParams {
    /// Preview only — do not archive anything
    pub dry_run: Option<bool>,
    /// Days of inactivity before considering stale (default: 90)
    pub stale_days: Option<i64>,
}

#[derive(Debug, Deserialize, JsonSchema)]
pub struct ReflectParams {
    /// Focus area: overview (default), growing, fading, connections, gaps
    pub focus: Option<String>,
}

#[derive(Debug, Deserialize, JsonSchema)]
pub struct GetParams {
    /// Chunk/memory ID to retrieve
    pub id: i64,
}

// --- MCP server handler ---

#[derive(Clone)]
pub struct GrasshopperMcp {
    db_path: PathBuf,
    embedder: Arc<Mutex<Option<Embedder>>>,
    tool_router: ToolRouter<Self>,
}

#[tool_router]
impl GrasshopperMcp {
    pub fn new(db_path: PathBuf) -> Self {
        Self {
            db_path,
            embedder: Arc::new(Mutex::new(None)),
            tool_router: Self::tool_router(),
        }
    }

    pub fn with_embedder(db_path: PathBuf, embedder: Arc<Mutex<Option<Embedder>>>) -> Self {
        Self {
            db_path,
            embedder,
            tool_router: Self::tool_router(),
        }
    }

    fn try_init_embedder(embedder: &Arc<Mutex<Option<Embedder>>>) {
        let mut guard = match embedder.lock() {
            Ok(g) => g,
            Err(_) => return,
        };
        if guard.is_none() {
            let cache_dir = ferret::embed::default_cache_dir();
            match Embedder::new(&cache_dir) {
                Ok(e) => *guard = Some(e),
                Err(e) => tracing::warn!("embedder init failed (graceful degrade): {e}"),
            }
        }
    }

    // --- Code Intelligence Tools ---

    #[tool(
        name = "grasshopper_search",
        description = "Search code and memory in one query. Hybrid search (keyword + semantic) across indexed codebases and stored memories. Use kind='code' or kind='memory' to filter, or 'all' (default) for both."
    )]
    async fn search(
        &self,
        Parameters(params): Parameters<SearchParams>,
    ) -> Result<CallToolResult, rmcp::ErrorData> {
        let db_path = self.db_path.clone();
        let embedder = Arc::clone(&self.embedder);
        Self::try_init_embedder(&embedder);

        let result = tokio::task::spawn_blocking(move || {
            let store = Store::open(&db_path)?;
            let kind_filter = match params.kind.as_deref() {
                Some("all") | None => None,
                Some(k @ ("code" | "memory")) => Some(k),
                Some(k) => anyhow::bail!("invalid kind '{k}': must be 'all', 'code', or 'memory'"),
            };
            let limit = params.limit.unwrap_or(10).min(100);
            let mut guard = embedder.lock().map_err(|e| anyhow::anyhow!("lock: {e}"))?;
            let results =
                crate::search::search(&store, &params.query, kind_filter, limit, guard.as_mut())?;
            let json = serde_json::to_string_pretty(&format_search_hits(&results))?;
            Ok::<_, anyhow::Error>(json)
        })
        .await
        .map_err(|e| rmcp::ErrorData::internal_error(format!("task join: {e}"), None))?;

        match result {
            Ok(json) => Ok(CallToolResult::success(vec![Content::text(json)])),
            Err(e) => Ok(error_result(format!("{e:#}"))),
        }
    }

    #[tool(
        name = "grasshopper_index",
        description = "Index a directory of source code for search. Supports 13 languages. Incremental: only re-indexes changed files. Use embed=true to generate semantic embeddings (slow on CPU)."
    )]
    async fn index_dir(
        &self,
        Parameters(params): Parameters<IndexParams>,
    ) -> Result<CallToolResult, rmcp::ErrorData> {
        let db_path = self.db_path.clone();
        let embedder = Arc::clone(&self.embedder);
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
                "edges_written": result.edges_written,
                "duration_ms": result.duration_ms,
                "errors": result.errors,
            });

            if embed {
                Self::try_init_embedder(&embedder);
                let mut guard = embedder.lock().map_err(|e| anyhow::anyhow!("lock: {e}"))?;
                if let Some(emb) = guard.as_mut() {
                    let embedded =
                        crate::index::embed_codebase(&store, emb, result.codebase_id)?;
                    output["embedded"] = serde_json::json!(embedded);
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
        name = "grasshopper_navigate",
        description = "Navigate code graph: find where a symbol is defined and what references it. Use direction='refs' (callers), 'defs' (definitions), or 'both' (default)."
    )]
    async fn navigate(
        &self,
        Parameters(params): Parameters<NavigateParams>,
    ) -> Result<CallToolResult, rmcp::ErrorData> {
        let db_path = self.db_path.clone();

        let result = tokio::task::spawn_blocking(move || {
            let store = Store::open(&db_path)?;
            let codebase_id = resolve_codebase(&store, params.dir.as_deref())?;
            let direction = params.direction.as_deref().unwrap_or("both");

            let (show_defs, show_refs) = match direction {
                "both" => (true, true),
                "defs" | "def" => (true, false),
                "refs" | "ref" => (false, true),
                d => anyhow::bail!("invalid direction '{d}': must be 'both', 'defs', or 'refs'"),
            };

            let mut output = String::new();

            if show_defs {
                let defs = store.find_definitions(&params.symbol, codebase_id)?;
                if !defs.is_empty() {
                    output.push_str(&format!("Definitions of '{}':\n", params.symbol));
                    for d in &defs {
                        output.push_str(&format!(
                            "  {}:{} ({} {})\n",
                            d.file_path, d.line, d.kind, d.symbol
                        ));
                    }
                }
            }

            if show_refs {
                let refs = store.find_references(&params.symbol, codebase_id)?;
                if !refs.is_empty() {
                    if !output.is_empty() {
                        output.push('\n');
                    }
                    output.push_str(&format!("References to '{}':\n", params.symbol));
                    for r in &refs {
                        output.push_str(&format!(
                            "  {}:{} ({} {})\n",
                            r.file_path, r.line, r.kind, r.symbol
                        ));
                    }
                }
            }

            if output.is_empty() {
                output = format!("No results found for '{}'.", params.symbol);
            }

            Ok::<_, anyhow::Error>(output)
        })
        .await
        .map_err(|e| rmcp::ErrorData::internal_error(format!("task join: {e}"), None))?;

        match result {
            Ok(text) => Ok(CallToolResult::success(vec![Content::text(text)])),
            Err(e) => Ok(error_result(format!("{e:#}"))),
        }
    }

    #[tool(
        name = "grasshopper_map",
        description = "Show codebase map: definitions grouped by file, ranked by reference frequency. Token-budgeted output for LLM context. Use dir parameter to scope to a specific codebase."
    )]
    async fn map(
        &self,
        Parameters(params): Parameters<MapParams>,
    ) -> Result<CallToolResult, rmcp::ErrorData> {
        let db_path = self.db_path.clone();

        let result = tokio::task::spawn_blocking(move || {
            let store = Store::open(&db_path)?;
            let budget = params.budget.unwrap_or(4000).min(200_000);

            let codebase_id = resolve_codebase(&store, params.dir.as_deref())?
                .context("no codebases indexed — run grasshopper_index first")?;

            generate_map(&store, codebase_id, budget)
        })
        .await
        .map_err(|e| rmcp::ErrorData::internal_error(format!("task join: {e}"), None))?;

        match result {
            Ok(text) => Ok(CallToolResult::success(vec![Content::text(text)])),
            Err(e) => Ok(error_result(format!("{e:#}"))),
        }
    }

    #[tool(
        name = "grasshopper_impact",
        description = "Analyze impact of changing a symbol. BFS traversal through the reference graph: depth 1 = direct callers, depth 2+ = transitive dependents. Shows which files would be affected."
    )]
    async fn impact(
        &self,
        Parameters(params): Parameters<ImpactParams>,
    ) -> Result<CallToolResult, rmcp::ErrorData> {
        let db_path = self.db_path.clone();

        let result = tokio::task::spawn_blocking(move || {
            let store = Store::open(&db_path)?;
            let codebase_id = resolve_codebase(&store, params.dir.as_deref())?;
            let max_depth = params.depth.unwrap_or(2).min(5);

            let hits = store.find_impact(&params.symbol, codebase_id, max_depth)?;

            if hits.is_empty() {
                return Ok(format!("No impact found for '{}'.", params.symbol));
            }

            let mut output = String::new();
            let defs = store.find_definitions(&params.symbol, codebase_id)?;
            if !defs.is_empty() {
                let locs: Vec<String> =
                    defs.iter().map(|d| format!("{}:{}", d.file_path, d.line)).collect();
                output.push_str(&format!(
                    "Impact of changing '{}' (defined at {}):\n",
                    params.symbol,
                    locs.join(", ")
                ));
            } else {
                output.push_str(&format!("Impact of changing '{}':\n", params.symbol));
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
            Ok::<_, anyhow::Error>(output)
        })
        .await
        .map_err(|e| rmcp::ErrorData::internal_error(format!("task join: {e}"), None))?;

        match result {
            Ok(text) => Ok(CallToolResult::success(vec![Content::text(text)])),
            Err(e) => Ok(error_result(format!("{e:#}"))),
        }
    }

    // --- Cognitive Memory Tools ---

    #[tool(
        name = "grasshopper_remember",
        description = "Store a memory with auto-classification and dedup. Content is auto-classified as identity/knowledge/episode/procedure if type omitted. Near-duplicate memories are updated rather than duplicated."
    )]
    async fn remember(
        &self,
        Parameters(params): Parameters<RememberParams>,
    ) -> Result<CallToolResult, rmcp::ErrorData> {
        let db_path = self.db_path.clone();
        let embedder = Arc::clone(&self.embedder);
        Self::try_init_embedder(&embedder);

        let result = tokio::task::spawn_blocking(move || {
            let store = Store::open(&db_path)?;
            let mut guard = embedder.lock().map_err(|e| anyhow::anyhow!("lock: {e}"))?;
            let tags = params.tags.as_deref().unwrap_or("");
            let result = crate::memory::remember(
                &store,
                guard.as_mut(),
                &params.content,
                params.title.as_deref(),
                params.r#type.as_deref(),
                tags,
            )?;
            let output = serde_json::json!({
                "id": result.id,
                "title": result.title,
                "memory_type": result.memory_type,
                "salience": result.salience,
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

    #[tool(
        name = "grasshopper_me",
        description = "Identity snapshot: who am I, what am I working on, working memory. Returns identity entries, active projects (latest handoffs), most-accessed memories, and counts."
    )]
    async fn me(
        &self,
        Parameters(_params): Parameters<MeParams>,
    ) -> Result<CallToolResult, rmcp::ErrorData> {
        let db_path = self.db_path.clone();

        let result = tokio::task::spawn_blocking(move || {
            let store = Store::open(&db_path)?;
            let me = crate::memory::me(&store)?;
            let output = serde_json::json!({
                "identity": me.identity.iter().map(|c| serde_json::json!({
                    "title": c.title, "content": c.content,
                })).collect::<Vec<_>>(),
                "active_projects": me.active_projects,
                "working_set": me.working_set.iter().map(|c| serde_json::json!({
                    "id": c.id, "title": c.title, "memory_type": c.memory_type,
                    "access_count": c.access_count, "salience": c.salience,
                })).collect::<Vec<_>>(),
                "active_count": me.active_count,
                "archived_count": me.archived_count,
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

    #[tool(
        name = "grasshopper_pickup",
        description = "Resume a previous session. Loads the latest handoff and retrieves related memories via cognitive search. Use at session start for continuity."
    )]
    async fn pickup(
        &self,
        Parameters(params): Parameters<PickupParams>,
    ) -> Result<CallToolResult, rmcp::ErrorData> {
        let db_path = self.db_path.clone();
        let embedder = Arc::clone(&self.embedder);
        Self::try_init_embedder(&embedder);

        let result = tokio::task::spawn_blocking(move || {
            let store = Store::open(&db_path)?;
            let mut guard = embedder.lock().map_err(|e| anyhow::anyhow!("lock: {e}"))?;
            let result =
                crate::memory::pickup(&store, guard.as_mut(), params.project.as_deref())?;
            let output = serde_json::json!({
                "handoff": result.handoff,
                "related_memories": format_search_hits(&result.related_memories),
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

    #[tool(
        name = "grasshopper_handoff",
        description = "Create a session handoff for continuity. Records what was accomplished, what should happen next, and which project this belongs to. Retrieved later via grasshopper_pickup."
    )]
    async fn handoff(
        &self,
        Parameters(params): Parameters<HandoffParams>,
    ) -> Result<CallToolResult, rmcp::ErrorData> {
        let db_path = self.db_path.clone();

        let result = tokio::task::spawn_blocking(move || {
            let store = Store::open(&db_path)?;
            let id = uuid::Uuid::new_v4().to_string();
            store.create_handoff(&id, &params.summary, &params.next_steps, &params.project)?;
            let output = serde_json::json!({
                "id": id,
                "project": params.project,
                "summary": params.summary,
                "next_steps": params.next_steps,
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

    #[tool(
        name = "grasshopper_consolidate",
        description = "Memory hygiene: find near-duplicates, auto-archive stale entries, cluster episodes by tag. Use dry_run=true to preview without changes."
    )]
    async fn consolidate(
        &self,
        Parameters(params): Parameters<ConsolidateParams>,
    ) -> Result<CallToolResult, rmcp::ErrorData> {
        let db_path = self.db_path.clone();
        let embedder = Arc::clone(&self.embedder);
        Self::try_init_embedder(&embedder);

        let result = tokio::task::spawn_blocking(move || {
            let store = Store::open(&db_path)?;
            let mut guard = embedder.lock().map_err(|e| anyhow::anyhow!("lock: {e}"))?;
            let dry_run = params.dry_run.unwrap_or(true);
            let stale_days = params.stale_days.unwrap_or(90).max(1);
            let result =
                crate::memory::consolidate(&store, guard.as_mut(), dry_run, stale_days)?;
            let output = serde_json::json!({
                "dry_run": dry_run,
                "near_duplicates": result.near_duplicates.iter().map(|(a, b, sim)| {
                    serde_json::json!({"id_a": a, "id_b": b, "similarity": sim})
                }).collect::<Vec<_>>(),
                "auto_archived": result.auto_archived,
                "stale_for_review": result.stale_for_review.iter().map(chunk_summary).collect::<Vec<_>>(),
                "episode_clusters": result.episode_clusters.iter().map(|(tag, entries)| {
                    serde_json::json!({"tag": tag, "count": entries.len()})
                }).collect::<Vec<_>>(),
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

    #[tool(
        name = "grasshopper_reflect",
        description = "Meta-cognition analytics: growing memories (recently active), fading memories (need attention), Hebbian connections, memory type distribution, and auto-generated observations."
    )]
    async fn reflect(
        &self,
        Parameters(params): Parameters<ReflectParams>,
    ) -> Result<CallToolResult, rmcp::ErrorData> {
        let db_path = self.db_path.clone();

        let result = tokio::task::spawn_blocking(move || {
            let store = Store::open(&db_path)?;
            let focus = match params.focus.as_deref() {
                Some("overview") | None => crate::memory::ReflectFocus::Overview,
                Some("growing") => crate::memory::ReflectFocus::Growing,
                Some("fading") => crate::memory::ReflectFocus::Fading,
                Some("connections") => crate::memory::ReflectFocus::Connections,
                Some("gaps") => crate::memory::ReflectFocus::Gaps,
                Some(f) => anyhow::bail!(
                    "invalid focus '{f}': must be 'overview', 'growing', 'fading', 'connections', or 'gaps'"
                ),
            };
            let result = crate::memory::reflect(&store, &focus)?;
            let output = serde_json::json!({
                "growing": result.growing.iter().map(chunk_summary).collect::<Vec<_>>(),
                "fading": result.fading.iter().map(chunk_summary).collect::<Vec<_>>(),
                "connections": result.connections.iter().map(|(c, count)| {
                    let mut s = chunk_summary(c);
                    s.as_object_mut().unwrap().insert(
                        "association_count".into(), serde_json::json!(count),
                    );
                    s
                }).collect::<Vec<_>>(),
                "type_counts": result.type_counts,
                "observations": result.observations,
                "active_count": result.active_count,
                "archived_count": result.archived_count,
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

    // --- Data Management Tools ---

    #[tool(
        name = "grasshopper_get",
        description = "Fetch a specific chunk or memory by ID. Returns full content, metadata, and cognitive fields."
    )]
    async fn get(
        &self,
        Parameters(params): Parameters<GetParams>,
    ) -> Result<CallToolResult, rmcp::ErrorData> {
        let db_path = self.db_path.clone();

        let result = tokio::task::spawn_blocking(move || {
            let store = Store::open(&db_path)?;
            match store.get_chunk(params.id)? {
                Some(chunk) => Ok::<_, anyhow::Error>(serde_json::to_string_pretty(&chunk)?),
                None => Ok(format!("No entry found with id {}.", params.id)),
            }
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
                "Grasshopper: Unified agent brain — code intelligence + cognitive memory. \
                 12 tools: search code and memory (grasshopper_search), \
                 index codebases (grasshopper_index), \
                 navigate code graph (grasshopper_navigate), \
                 codebase map (grasshopper_map), \
                 impact analysis (grasshopper_impact), \
                 store memories (grasshopper_remember), \
                 identity snapshot (grasshopper_me), \
                 session continuity (grasshopper_pickup, grasshopper_handoff), \
                 meta-cognition (grasshopper_reflect), \
                 memory hygiene (grasshopper_consolidate), \
                 get entry by ID (grasshopper_get)."
                    .into(),
            ),
            capabilities: ServerCapabilities::builder().enable_tools().build(),
            ..Default::default()
        }
    }
}

// --- Transport entry points ---

/// Run the MCP server over stdio (for direct Claude Code integration).
pub async fn run_stdio(db_path: PathBuf) -> Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::from_default_env()
                .add_directive(tracing::Level::INFO.into())
                .add_directive("ort=warn".parse().unwrap()),
        )
        .with_writer(std::io::stderr)
        .with_target(false)
        .with_ansi(false)
        .init();

    tracing::info!("starting grasshopper MCP server (stdio)");
    let server = GrasshopperMcp::new(db_path);
    let service = rmcp::ServiceExt::serve(server, rmcp::transport::stdio()).await?;
    service.waiting().await?;
    Ok(())
}

/// Run the MCP server over HTTP (daemon mode).
pub async fn run_http(db_path: PathBuf, port: u16) -> Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::from_default_env()
                .add_directive(tracing::Level::INFO.into())
                .add_directive("ort=warn".parse().unwrap()),
        )
        .with_target(false)
        .init();

    use rmcp::transport::streamable_http_server::{
        session::local::LocalSessionManager, StreamableHttpServerConfig, StreamableHttpService,
    };
    use tower_http::cors::CorsLayer;

    let ct = tokio_util::sync::CancellationToken::new();
    let shared_embedder: Arc<Mutex<Option<Embedder>>> = Arc::new(Mutex::new(None));

    let db = db_path.clone();
    let embedder_for_mcp = shared_embedder.clone();
    let mcp_service = StreamableHttpService::new(
        move || {
            Ok(GrasshopperMcp::with_embedder(
                db.clone(),
                embedder_for_mcp.clone(),
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

    let router = axum::Router::new()
        .route("/healthz", axum::routing::get(|| async { "ok" }))
        .nest_service("/mcp", mcp_service)
        .layer(cors);

    let addr = format!("127.0.0.1:{port}");
    tracing::info!("starting grasshopper MCP server on http://{addr}/mcp");
    tracing::info!("health check at http://{addr}/healthz");

    let listener = tokio::net::TcpListener::bind(&addr)
        .await
        .with_context(|| format!("binding to {addr}"))?;

    axum::serve(listener, router)
        .with_graceful_shutdown(async move {
            tokio::signal::ctrl_c().await.ok();
            tracing::info!("shutting down");
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

/// Summarize a Chunk for JSON output (used by reflect, consolidate).
fn chunk_summary(c: &crate::store::Chunk) -> serde_json::Value {
    serde_json::json!({
        "id": c.id,
        "title": c.title,
        "memory_type": c.memory_type,
        "access_count": c.access_count,
        "salience": c.salience,
        "last_accessed": c.last_accessed,
        "created_at": c.created_at,
    })
}

/// Resolve a codebase directory filter to a codebase_id.
/// If multiple codebases are indexed and no dir is specified, returns an error
/// listing available codebases (consistent policy across all code-intel tools).
fn resolve_codebase(store: &Store, dir: Option<&str>) -> Result<Option<i64>> {
    let codebases = store.list_codebases()?;
    if let Some(dir) = dir {
        let found = codebases
            .iter()
            .find(|(_, root, name)| name == dir || root.ends_with(dir))
            .map(|(id, _, _)| Some(*id))
            .context(format!("no indexed codebase matching '{dir}'"))?;
        Ok(found)
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

/// Generate a compact codebase map from definitions, ranked by reference frequency.
fn generate_map(store: &Store, codebase_id: i64, token_budget: usize) -> Result<String> {
    let definitions = store.get_all_definitions(codebase_id)?;
    let ref_counts = store.count_graph_references(codebase_id)?;

    if definitions.is_empty() {
        return Ok("No definitions found. Run grasshopper_index first.".into());
    }

    // Group definitions by file
    let mut by_file: BTreeMap<&str, Vec<&crate::store::GraphEdge>> = BTreeMap::new();
    for edge in &definitions {
        by_file.entry(&edge.file_path).or_default().push(edge);
    }

    // Score each file by total reference count of its defined symbols
    let mut file_scores: Vec<(&str, i64)> = by_file
        .keys()
        .map(|&file| {
            let score: i64 = by_file[file]
                .iter()
                .map(|e| *ref_counts.get(&e.symbol).unwrap_or(&0) as i64)
                .sum();
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
