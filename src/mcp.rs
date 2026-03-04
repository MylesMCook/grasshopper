use anyhow::{Context, Result};
use rmcp::handler::server::router::tool::ToolRouter;
use rmcp::handler::server::wrapper::Parameters;
use rmcp::model::{CallToolResult, Content, ServerCapabilities, ServerInfo, ToolAnnotations};
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
    /// Natural language query or keyword to search for
    pub query: String,
    /// Filter by entry type. Values: "all" (default), "code", "memory"
    pub kind: Option<String>,
    /// Maximum results to return, 1-100 (default: 10)
    pub limit: Option<usize>,
}

#[derive(Debug, Deserialize, JsonSchema)]
pub struct IndexParams {
    /// Absolute path to the directory to index (e.g. "/home/user/projects/myapp")
    pub directory: String,
    /// Also generate semantic embeddings after indexing. Enables hybrid search but is slow (~4 embeddings/sec on CPU). Default: false
    pub embed: Option<bool>,
}

#[derive(Debug, Deserialize, JsonSchema)]
pub struct NavigateParams {
    /// Exact symbol name to look up (e.g. "SearchParams", "run_http", "Store")
    pub symbol: String,
    /// Which edges to follow. Values: "both" (default), "defs", "refs"
    pub direction: Option<String>,
    /// Scope to a codebase by directory name or path suffix. Required when multiple codebases are indexed
    pub dir: Option<String>,
}

#[derive(Debug, Deserialize, JsonSchema)]
pub struct MapParams {
    /// Scope to a codebase by directory name or path suffix. Required when multiple codebases are indexed
    pub dir: Option<String>,
    /// Approximate token budget for output, 1-200000 (default: 4000). Larger budget = more detail
    pub budget: Option<usize>,
}

#[derive(Debug, Deserialize, JsonSchema)]
pub struct ImpactParams {
    /// Exact symbol name to analyze (e.g. "Store", "search")
    pub symbol: String,
    /// Maximum BFS traversal depth, 1-5 (default: 2). Depth 1 = direct callers only
    pub depth: Option<usize>,
    /// Scope to a codebase by directory name or path suffix. Required when multiple codebases are indexed
    pub dir: Option<String>,
}

#[derive(Debug, Deserialize, JsonSchema)]
pub struct RememberParams {
    /// The information to remember. Can be a fact, decision, preference, observation, or procedure
    pub content: String,
    /// Short title for this memory. Auto-generated from content if omitted
    pub title: Option<String>,
    /// Memory type. Values: "identity" (who I am), "knowledge" (facts/decisions), "episode" (events), "procedure" (how-to). Auto-classified if omitted
    pub r#type: Option<String>,
    /// Comma-separated tags for organization (e.g. "rust,grasshopper,architecture")
    pub tags: Option<String>,
}

#[derive(Debug, Deserialize, JsonSchema)]
pub struct RecallParams {
    /// Natural language query to search memories (not code). Results are ranked by cognitive score: recency, frequency, salience, and type-specific decay
    pub query: String,
    /// Maximum results to return, 1-50 (default: 10)
    pub limit: Option<usize>,
}

#[derive(Debug, Deserialize, JsonSchema)]
pub struct MeParams {}

#[derive(Debug, Deserialize, JsonSchema)]
pub struct PickupParams {
    /// Filter to a specific project's handoff (e.g. "grasshopper"). Returns latest handoff if omitted
    pub project: Option<String>,
}

#[derive(Debug, Deserialize, JsonSchema)]
pub struct HandoffParams {
    /// Project name this handoff belongs to (e.g. "grasshopper", "ferret")
    pub project: String,
    /// What was accomplished in this session
    pub summary: String,
    /// Concrete next steps for whoever picks up this project
    pub next_steps: String,
}

#[derive(Debug, Deserialize, JsonSchema)]
pub struct ConsolidateParams {
    /// Preview changes without applying them. Default: true (safe preview mode)
    pub dry_run: Option<bool>,
    /// Days of inactivity before a memory is considered stale, minimum 1 (default: 90)
    pub stale_days: Option<i64>,
}

#[derive(Debug, Deserialize, JsonSchema)]
pub struct ArchiveParams {
    /// Numeric ID of the memory to archive (from search, recall, or other tool outputs)
    pub id: i64,
}

#[derive(Debug, Deserialize, JsonSchema)]
pub struct ReflectParams {
    /// What aspect of memory to analyze. Values: "overview" (default), "growing" (recently active), "fading" (neglected), "connections" (Hebbian associations), "gaps" (missing coverage)
    pub focus: Option<String>,
}

#[derive(Debug, Deserialize, JsonSchema)]
pub struct GetParams {
    /// Numeric ID of the entry to retrieve (from search results or other tool outputs)
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
            tool_router: Self::annotated_router(),
        }
    }

    pub fn with_embedder(db_path: PathBuf, embedder: Arc<Mutex<Option<Embedder>>>) -> Self {
        Self {
            db_path,
            embedder,
            tool_router: Self::annotated_router(),
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
        let destructive_write = ToolAnnotations::new()
            .read_only(false)
            .destructive(true);

        for (name, route) in router.map.iter_mut() {
            let ann = match name.as_ref() {
                // Read-only: search, navigate, map, impact, me, pickup, reflect, get
                "search" | "navigate" | "map" | "impact" | "me" | "pickup" | "reflect"
                | "get" => read_only.clone(),
                // Destructive: consolidate and archive can remove memories
                "consolidate" | "archive" => destructive_write.clone(),
                // Write (non-destructive): index, remember, handoff
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
            let cache_dir = ferret::embed::default_cache_dir();
            match Embedder::new(&cache_dir) {
                Ok(e) => *guard = Some(e),
                Err(e) => tracing::warn!("embedder init failed (graceful degrade): {e}"),
            }
        }
    }

    // --- Code Intelligence Tools ---

    #[tool(
        name = "search",
        description = "Search across code and memory with a single query. Returns ranked results using hybrid search (keyword matching + semantic similarity). Use kind='code' to search only indexed source files, kind='memory' to search only stored memories, or omit for both. Start here when looking for anything."
    )]
    async fn search(
        &self,
        Parameters(params): Parameters<SearchParams>,
    ) -> Result<CallToolResult, rmcp::ErrorData> {
        let db_path = self.db_path.clone();
        let embedder = Arc::clone(&self.embedder);

        let result = tokio::task::spawn_blocking(move || {
            Self::init_embedder_blocking(&embedder);
            let store = Store::open(&db_path)?;
            let kind_filter = match params.kind.as_deref() {
                Some("all") | None => None,
                Some(k @ ("code" | "memory")) => Some(k),
                Some(k) => anyhow::bail!("invalid kind '{k}': must be 'all', 'code', or 'memory'"),
            };
            let limit = params.limit.unwrap_or(10).clamp(1, 100);
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
        name = "index",
        description = "Index a source code directory for search and navigation. Parses 13 languages (Rust, TypeScript, Python, Go, etc.) using tree-sitter, extracts symbols and references. Incremental — only re-indexes changed files. Run once per codebase, then use search/navigate/map/impact."
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
                Self::init_embedder_blocking(&embedder);
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
        name = "navigate",
        description = "Find where a symbol is defined and what references it. Returns file:line locations. Use when you know the exact symbol name and need to find its definition or callers."
    )]
    async fn navigate(
        &self,
        Parameters(params): Parameters<NavigateParams>,
    ) -> Result<CallToolResult, rmcp::ErrorData> {
        let db_path = self.db_path.clone();

        let result = tokio::task::spawn_blocking(move || {
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
        name = "map",
        description = "Get a high-level overview of a codebase. Returns all symbols (functions, structs, traits, etc.) grouped by file, ranked by how frequently they're referenced. Output is token-budgeted to fit in context. Use to orient yourself in an unfamiliar codebase."
    )]
    async fn map(
        &self,
        Parameters(params): Parameters<MapParams>,
    ) -> Result<CallToolResult, rmcp::ErrorData> {
        let db_path = self.db_path.clone();

        let result = tokio::task::spawn_blocking(move || {
            let store = Store::open(&db_path)?;
            let budget = params.budget.unwrap_or(4000).clamp(1, 200_000);

            let codebase_id = resolve_codebase(&store, params.dir.as_deref())?
                .context("no codebases indexed — run index first")?;

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
        name = "impact",
        description = "Analyze what would break if a symbol were changed. Walks the reference graph outward: depth 1 shows direct callers, depth 2+ shows transitive dependents. Use before refactoring to understand blast radius."
    )]
    async fn impact(
        &self,
        Parameters(params): Parameters<ImpactParams>,
    ) -> Result<CallToolResult, rmcp::ErrorData> {
        let db_path = self.db_path.clone();

        let result = tokio::task::spawn_blocking(move || {
            let store = Store::open(&db_path)?;
            let codebase_id = resolve_codebase(&store, params.dir.as_deref())?
                .context("no codebases indexed — run index first")?;
            let codebase_id = Some(codebase_id);
            let max_depth = params.depth.unwrap_or(2).clamp(1, 5);

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
        name = "remember",
        description = "Store a fact, decision, preference, or observation as a persistent memory. Auto-classifies the memory type and deduplicates — if a near-duplicate exists, it updates the existing entry instead of creating a new one. Use proactively to save anything worth remembering across sessions."
    )]
    async fn remember(
        &self,
        Parameters(params): Parameters<RememberParams>,
    ) -> Result<CallToolResult, rmcp::ErrorData> {
        let db_path = self.db_path.clone();
        let embedder = Arc::clone(&self.embedder);

        let result = tokio::task::spawn_blocking(move || {
            Self::init_embedder_blocking(&embedder);
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
        name = "recall",
        description = "Cognitive-scored memory search. Unlike search (which returns raw hybrid-search scores), recall applies the full cognitive scoring formula: recency boost, frequency boost, salience weighting, and type-specific exponential decay. Also has side effects: touched memories gain salience and co-retrieved memories form Hebbian associations. Use when you need the most relevant memories, not just keyword matches."
    )]
    async fn recall(
        &self,
        Parameters(params): Parameters<RecallParams>,
    ) -> Result<CallToolResult, rmcp::ErrorData> {
        let db_path = self.db_path.clone();
        let embedder = Arc::clone(&self.embedder);

        let result = tokio::task::spawn_blocking(move || {
            Self::init_embedder_blocking(&embedder);
            let store = Store::open(&db_path)?;
            let limit = params.limit.unwrap_or(10).clamp(1, 50);
            let mut guard = embedder.lock().map_err(|e| anyhow::anyhow!("lock: {e}"))?;
            let result = crate::memory::recall(&store, guard.as_mut(), &params.query, limit)?;
            let json = serde_json::to_string_pretty(&format_search_hits(&result.hits))?;
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
        name = "archive",
        description = "Soft-delete a memory by ID. Use to remove bad, outdated, or redundant memories that degrade search quality. The memory is marked archived, not permanently deleted — it won't appear in recall or search results. Get the ID from search, recall, or reflect output."
    )]
    async fn archive(
        &self,
        Parameters(params): Parameters<ArchiveParams>,
    ) -> Result<CallToolResult, rmcp::ErrorData> {
        let db_path = self.db_path.clone();

        let result = tokio::task::spawn_blocking(move || {
            let store = Store::open(&db_path)?;
            let archived = store.archive_memory(params.id)?;
            if archived {
                Ok(format!("Archived memory #{}.", params.id))
            } else {
                anyhow::bail!("no active memory with ID {}. It may already be archived or not exist.", params.id)
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
        name = "me",
        description = "Load identity and working context. Returns: who I am (identity memories), active projects (latest handoffs), most-accessed memories (working set), and memory counts. Call at session start to establish context."
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
        name = "pickup",
        description = "Resume where a previous session left off. Loads the latest handoff (summary + next steps) and retrieves related memories via cognitive search. Call at session start after me for continuity."
    )]
    async fn pickup(
        &self,
        Parameters(params): Parameters<PickupParams>,
    ) -> Result<CallToolResult, rmcp::ErrorData> {
        let db_path = self.db_path.clone();
        let embedder = Arc::clone(&self.embedder);

        let result = tokio::task::spawn_blocking(move || {
            Self::init_embedder_blocking(&embedder);
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
        name = "handoff",
        description = "Save a session handoff before ending work. Records what was accomplished and what should happen next. The next session retrieves this via pickup. Call at session end."
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
        name = "consolidate",
        description = "Clean up memory: find near-duplicate entries, auto-archive stale memories, and cluster episodes by tag. Defaults to dry_run=true (preview only). Run periodically to keep memory lean."
    )]
    async fn consolidate(
        &self,
        Parameters(params): Parameters<ConsolidateParams>,
    ) -> Result<CallToolResult, rmcp::ErrorData> {
        let db_path = self.db_path.clone();
        let embedder = Arc::clone(&self.embedder);

        let result = tokio::task::spawn_blocking(move || {
            Self::init_embedder_blocking(&embedder);
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
        name = "reflect",
        description = "Analyze the state of memory. Shows which memories are growing (recently active), fading (neglected), strongly connected (Hebbian associations), and overall distribution. Use to understand what's well-remembered and what needs attention."
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
        name = "get",
        description = "Retrieve the full content of an entry by its numeric ID. Returns all fields including content, metadata, and cognitive state. Use when search results show a relevant entry and you need the complete text."
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
                "Grasshopper is a unified agent brain combining code intelligence and cognitive memory.\n\n\
                 SESSION WORKFLOW:\n\
                 - Start: call me then pickup to load identity and resume context\n\
                 - During: use remember to save decisions, learnings, and preferences\n\
                 - End: call handoff to record progress and next steps\n\n\
                 CODE INTELLIGENCE (requires index first):\n\
                 - search: find code or memories by raw hybrid-search score. Use for code lookups or broad cross-domain queries\n\
                 - navigate: jump to a symbol's definition or find its callers\n\
                 - map: get a token-budgeted overview of an entire codebase\n\
                 - impact: see what breaks if you change a symbol\n\n\
                 COGNITIVE MEMORY:\n\
                 - recall: retrieve memories with cognitive scoring (recency, frequency, salience, decay). Use this for memory queries, not search\n\
                 - remember: store facts, decisions, preferences (auto-deduplicates)\n\
                 - archive: soft-delete a bad or outdated memory by ID\n\
                 - me: load identity and working context\n\
                 - pickup / handoff: session continuity\n\
                 - reflect: analyze memory health and patterns\n\
                 - consolidate: clean up duplicates and stale entries\n\n\
                 get retrieves the full content of any entry by ID."
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

/// Generate a compact codebase map from definitions, ranked by reference frequency.
fn generate_map(store: &Store, codebase_id: i64, token_budget: usize) -> Result<String> {
    let definitions = store.get_all_definitions(codebase_id)?;
    let ref_counts = store.count_graph_references(codebase_id)?;

    if definitions.is_empty() {
        return Ok("No definitions found. Run index first.".into());
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
