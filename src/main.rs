use anyhow::Result;
use clap::{Parser, Subcommand, ValueEnum};
use grasshopper::store::{SearchHit, Store};
use std::io::IsTerminal;
use std::path::{Path, PathBuf};

#[derive(Parser)]
#[command(
    name = "grasshopper",
    version,
    about = "Persistent retrieval engine for AI agents",
    long_about = "Grasshopper indexes codebases and stores memories, then retrieves them\n\
                  with a pipeline combining full-text search, semantic embeddings,\n\
                  reciprocal rank fusion, and cross-encoder reranking.\n\n\
                  All inference runs locally via ONNX. No external API calls.",
    after_help = "EXAMPLES:\n  \
                  grasshopper index ./my-project --embed\n  \
                  grasshopper search \"error handling\"\n  \
                  grasshopper search Config --mode navigate\n  \
                  grasshopper store \"Always use .clamp() for bounded values\"\n  \
                  grasshopper status\n  \
                  grasshopper memories --type knowledge\n  \
                  grasshopper get 42"
)]
struct Cli {
    /// Database file path [default: ~/.grasshopper/brain.db]
    #[arg(long, global = true)]
    db: Option<PathBuf>,

    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand)]
enum Commands {
    /// Index a directory of source code
    #[command(after_help = "EXAMPLES:\n  \
                      grasshopper index .                    # FTS only (fast)\n  \
                      grasshopper index ./src --embed        # FTS + semantic (slow, ~4/sec CPU)")]
    Index {
        /// Directory to index
        dir: PathBuf,
        /// Generate embeddings for semantic search (slow, ~4/sec on CPU)
        #[arg(long)]
        embed: bool,
    },

    /// Search code and memory
    #[command(after_help = "MODES:\n  \
                      search     Hybrid FTS + vector + reranking (default)\n  \
                      navigate   Find symbol definitions and references\n  \
                      map        Token-budgeted codebase overview\n  \
                      impact     BFS blast radius analysis\n\n\
                      PRESETS (--preset):\n  \
                      strict       Only high-confidence results (threshold 0.5)\n  \
                      balanced     Good precision/recall tradeoff (threshold 0.3)\n  \
                      exploratory  Cast a wide net (threshold 0.1)\n\n\
                      EXAMPLES:\n  \
                      grasshopper search \"auth middleware\"\n  \
                      grasshopper search Config --mode navigate\n  \
                      grasshopper search \"\" --mode map --budget 8000\n  \
                      grasshopper search Store --mode impact --depth 3\n  \
                      grasshopper search \"error\" --kind memory --preset strict")]
    Search {
        /// Search query
        query: String,
        /// Search mode
        #[arg(long, default_value = "search")]
        mode: String,
        /// Filter: code, memory, or all
        #[arg(long, default_value = "all")]
        kind: String,
        /// Maximum results (1-100)
        #[arg(long, default_value = "10")]
        limit: usize,
        /// Minimum relevance threshold (0.0-1.0) for memory results
        #[arg(long)]
        threshold: Option<f32>,
        /// Named threshold preset (overridden by --threshold)
        #[arg(long, value_enum)]
        preset: Option<Preset>,
        /// Token budget for map mode
        #[arg(long, default_value = "4000")]
        budget: usize,
        /// BFS depth for impact mode (1-5)
        #[arg(long, default_value = "2")]
        depth: usize,
        /// Scope to a codebase directory
        #[arg(long)]
        dir: Option<String>,
        /// Edge direction for navigate mode: both, defs, refs
        #[arg(long, default_value = "both")]
        direction: String,
        /// Output as JSON
        #[arg(long)]
        json: bool,
    },

    /// Store a memory (with automatic dedup)
    #[command(after_help = "TYPES:\n  \
                      knowledge    Facts, decisions, architecture (default, slow decay)\n  \
                      identity     Preferences, never fades\n  \
                      episode      Events, sessions (fast decay)\n  \
                      procedure    Workflows, how-tos (medium decay)\n\n\
                      EXAMPLES:\n  \
                      grasshopper store \"Always use bun for scripts\"\n  \
                      grasshopper store \"I prefer dark themes\" --memory-type identity\n  \
                      grasshopper store \"Deploy steps: build, test, push\" --tags ops,deploy")]
    Store {
        /// Memory content
        content: String,
        /// Optional title (auto-generated from content if omitted)
        #[arg(long)]
        title: Option<String>,
        /// Tags (comma-separated)
        #[arg(long, default_value = "")]
        tags: String,
        /// Memory type: knowledge (default), identity, episode, procedure
        #[arg(long)]
        memory_type: Option<String>,
    },

    /// Show database health and statistics
    #[command(after_help = "EXAMPLES:\n  \
                      grasshopper status\n  \
                      grasshopper status --json")]
    Status {
        /// Output as JSON
        #[arg(long)]
        json: bool,
    },

    /// Inspect a chunk by ID
    #[command(
        after_help = "Shows full content and metadata for any chunk (code or memory).\n\
                      Use search results to find chunk IDs.\n\n\
                      EXAMPLES:\n  \
                      grasshopper get 42\n  \
                      grasshopper get 42 --json"
    )]
    Get {
        /// Chunk ID
        id: i64,
        /// Output as JSON
        #[arg(long)]
        json: bool,
    },

    /// List and browse memories
    #[command(after_help = "EXAMPLES:\n  \
                      grasshopper memories\n  \
                      grasshopper memories --type identity\n  \
                      grasshopper memories --type episode --archived\n  \
                      grasshopper memories --limit 50 --json")]
    Memories {
        /// Filter by memory type
        #[arg(long, rename_all = "kebab-case")]
        r#type: Option<String>,
        /// Include archived entries
        #[arg(long)]
        archived: bool,
        /// Maximum entries to show
        #[arg(long, default_value = "20")]
        limit: usize,
        /// Output as JSON
        #[arg(long)]
        json: bool,
    },

    /// Start the MCP server
    #[command(after_help = "EXAMPLES:\n  \
                      grasshopper serve                    # HTTP on port 8106\n  \
                      grasshopper serve --port 9000        # Custom port\n  \
                      grasshopper serve --stdio            # stdio transport")]
    Serve {
        /// Port to listen on
        #[arg(long, default_value = "8106")]
        port: u16,
        /// Use stdio transport instead of HTTP
        #[arg(long)]
        stdio: bool,
    },
}

/// Named threshold presets for search.
#[derive(Clone, ValueEnum)]
enum Preset {
    /// Only high-confidence results (threshold 0.5)
    Strict,
    /// Good precision/recall tradeoff (threshold 0.3)
    Balanced,
    /// Cast a wide net (threshold 0.1)
    Exploratory,
}

impl Preset {
    fn threshold(&self) -> f32 {
        match self {
            Preset::Strict => 0.5,
            Preset::Balanced => 0.3,
            Preset::Exploratory => 0.1,
        }
    }
}

// ANSI color codes — only used when outputting to a terminal
struct Colors {
    green: &'static str,
    blue: &'static str,
    cyan: &'static str,
    magenta: &'static str,
    yellow: &'static str,
    dim: &'static str,
    bold: &'static str,
    reset: &'static str,
}

const COLORS_ON: Colors = Colors {
    green: "\x1b[32m",
    blue: "\x1b[34m",
    cyan: "\x1b[36m",
    magenta: "\x1b[35m",
    yellow: "\x1b[33m",
    dim: "\x1b[2m",
    bold: "\x1b[1m",
    reset: "\x1b[0m",
};

const COLORS_OFF: Colors = Colors {
    green: "",
    blue: "",
    cyan: "",
    magenta: "",
    yellow: "",
    dim: "",
    bold: "",
    reset: "",
};

fn colors() -> &'static Colors {
    if std::io::stdout().is_terminal() {
        &COLORS_ON
    } else {
        &COLORS_OFF
    }
}

fn default_db_path() -> PathBuf {
    dirs::home_dir()
        .unwrap_or_else(|| PathBuf::from("."))
        .join(".grasshopper")
        .join("brain.db")
}

fn try_embedder() -> Option<grasshopper::code::embed::Embedder> {
    let cache_dir = grasshopper::code::embed::default_cache_dir();
    grasshopper::code::embed::Embedder::new(&cache_dir).ok()
}

fn try_reranker() -> Option<grasshopper::rerank::Reranker> {
    grasshopper::rerank::Reranker::new().ok()
}

fn try_hnsw(db_path: &Path) -> Option<grasshopper::code::hnsw::HnswIndex> {
    let hnsw_path = grasshopper::code::hnsw::hnsw_path(db_path);
    if hnsw_path.exists() {
        grasshopper::code::hnsw::HnswIndex::load(&hnsw_path).ok()
    } else {
        None
    }
}

/// Exit code 1 = no results found (grep convention).
const EXIT_NO_RESULTS: i32 = 1;

fn main() {
    if let Err(e) = run() {
        eprintln!("error: {e:#}");
        std::process::exit(2);
    }
}

fn run() -> Result<()> {
    let cli = Cli::parse();
    let db_path = cli.db.unwrap_or_else(default_db_path);

    // MCP/Serve commands init their own tracing — handle before CLI tracing
    if let Commands::Serve { port, stdio } = cli.command {
        let rt = tokio::runtime::Runtime::new()?;
        if stdio {
            return rt.block_on(grasshopper::mcp::run_stdio(db_path));
        } else {
            return rt.block_on(grasshopper::mcp::run_http(db_path, port));
        }
    }

    // CLI tracing
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("warn")),
        )
        .init();

    match cli.command {
        Commands::Index { dir, embed } => {
            let store = Store::open(&db_path)?;
            let result = grasshopper::index::index_directory(&store, &dir)?;

            let c = colors();
            println!(
                "{}Indexed{} {} ({} files, {} chunks)",
                c.bold,
                c.reset,
                dir.display(),
                result.files_scanned,
                result.chunks_written,
            );
            println!(
                "  Changed: {}  Skipped: {}  Removed: {}",
                result.files_changed, result.files_skipped, result.files_removed,
            );

            if !result.errors.is_empty() {
                eprintln!("Errors ({}):", result.errors.len());
                for e in &result.errors {
                    eprintln!("  {e}");
                }
            }

            if embed {
                let cache_dir = grasshopper::code::embed::default_cache_dir();
                let mut embedder = grasshopper::code::embed::Embedder::new(&cache_dir)?;
                let embedded =
                    grasshopper::index::embed_codebase(&store, &mut embedder, result.codebase_id)?;
                println!("Embedded: {} chunks", embedded);

                // Rebuild HNSW index after embedding
                rebuild_hnsw(&store, &db_path)?;
            }

            println!("{}Done in {}ms{}", c.dim, result.duration_ms, c.reset);
        }

        Commands::Search {
            query,
            mode,
            kind,
            limit,
            threshold,
            preset,
            budget,
            depth,
            dir,
            direction,
            json,
        } => {
            let store = Store::open(&db_path)?;

            let limit = limit.clamp(1, 100);
            let budget = budget.clamp(1, 200_000);

            // Resolve threshold: explicit --threshold wins, then --preset, then None
            let threshold = threshold.or_else(|| preset.map(|p| p.threshold()));

            match mode.as_str() {
                "search" => {
                    let kind_filter = match kind.as_str() {
                        "all" => None,
                        "code" | "memory" => Some(kind.as_str()),
                        k => {
                            anyhow::bail!("invalid kind '{k}': must be 'all', 'code', or 'memory'")
                        }
                    };

                    let mut embedder = try_embedder();
                    let mut reranker = try_reranker();
                    let hnsw = try_hnsw(&db_path);
                    let result =
                        grasshopper::search::unified_search(grasshopper::search::SearchContext {
                            store: &store,
                            query: &query,
                            kind_filter,
                            limit,
                            threshold,
                            embedder: embedder.as_mut(),
                            reranker: reranker.as_mut(),
                            hnsw: hnsw.as_ref(),
                        })?;

                    if json {
                        println!("{}", serde_json::to_string_pretty(&result.hits)?);
                        if result.hits.is_empty() {
                            std::process::exit(EXIT_NO_RESULTS);
                        }
                        return Ok(());
                    }

                    if result.hits.is_empty() {
                        println!("No results found.");
                        std::process::exit(EXIT_NO_RESULTS);
                    }

                    for (i, hit) in result.hits.iter().enumerate() {
                        print_hit(i + 1, hit);
                    }
                }
                "navigate" => {
                    let codebase_id = store.resolve_codebase(dir.as_deref())?;
                    let c = colors();

                    let (show_defs, show_refs) = match direction.as_str() {
                        "both" => (true, true),
                        "defs" | "def" => (true, false),
                        "refs" | "ref" => (false, true),
                        d => anyhow::bail!(
                            "invalid direction '{d}': must be 'both', 'defs', or 'refs'"
                        ),
                    };

                    let defs = if show_defs {
                        store.find_definitions(&query, codebase_id)?
                    } else {
                        vec![]
                    };
                    let refs = if show_refs {
                        store.find_references(&query, codebase_id)?
                    } else {
                        vec![]
                    };

                    if json {
                        let out = serde_json::json!({
                            "definitions": defs,
                            "references": refs,
                        });
                        println!("{}", serde_json::to_string_pretty(&out)?);
                        if defs.is_empty() && refs.is_empty() {
                            std::process::exit(EXIT_NO_RESULTS);
                        }
                    } else if defs.is_empty() && refs.is_empty() {
                        println!("No definitions or references found for '{query}'.");
                        std::process::exit(EXIT_NO_RESULTS);
                    } else {
                        if !defs.is_empty() {
                            println!(
                                "{}Definitions{} of '{}{query}{}':",
                                c.bold, c.reset, c.cyan, c.reset
                            );
                            for d in &defs {
                                println!(
                                    "  {}{}{}{}{}{} {}({} {}){}",
                                    c.green,
                                    d.file_path,
                                    c.reset,
                                    c.dim,
                                    format_line(d.line),
                                    c.reset,
                                    c.magenta,
                                    d.kind,
                                    d.symbol,
                                    c.reset,
                                );
                            }
                        }
                        if !refs.is_empty() {
                            if !defs.is_empty() {
                                println!();
                            }
                            println!(
                                "{}References{} to '{}{query}{}':",
                                c.bold, c.reset, c.cyan, c.reset
                            );
                            for r in &refs {
                                println!(
                                    "  {}{}{}{}{}{} {}({} {}){}",
                                    c.green,
                                    r.file_path,
                                    c.reset,
                                    c.dim,
                                    format_line(r.line),
                                    c.reset,
                                    c.magenta,
                                    r.kind,
                                    r.symbol,
                                    c.reset,
                                );
                            }
                        }
                    }
                }
                "map" => {
                    let codebase_id = store
                        .resolve_codebase(dir.as_deref())?
                        .ok_or_else(|| anyhow::anyhow!("no codebases indexed — run index first"))?;
                    let output = grasshopper::search::generate_map(&store, codebase_id, budget)?;
                    print!("{output}");
                    if output.starts_with("No definitions found") {
                        std::process::exit(EXIT_NO_RESULTS);
                    }
                }
                "impact" => {
                    let codebase_id = store.resolve_codebase(dir.as_deref())?;
                    let max_depth = depth.clamp(1, 5);
                    let hits = store.find_impact(&query, codebase_id, max_depth)?;
                    let c = colors();

                    if json {
                        println!("{}", serde_json::to_string_pretty(&hits)?);
                        if hits.is_empty() {
                            std::process::exit(EXIT_NO_RESULTS);
                        }
                        return Ok(());
                    }

                    if hits.is_empty() {
                        println!("No impact found for '{query}'.");
                        std::process::exit(EXIT_NO_RESULTS);
                    }

                    let defs = store.find_definitions(&query, codebase_id)?;
                    if !defs.is_empty() {
                        let locs: Vec<String> = defs
                            .iter()
                            .map(|d| format!("{}:{}", d.file_path, d.line))
                            .collect();
                        println!(
                            "{}Impact{} of changing '{}{query}{}' (defined at {}{}{}):",
                            c.bold,
                            c.reset,
                            c.cyan,
                            c.reset,
                            c.green,
                            locs.join(", "),
                            c.reset,
                        );
                    } else {
                        println!(
                            "{}Impact{} of changing '{}{query}{}':",
                            c.bold, c.reset, c.cyan, c.reset
                        );
                    }

                    let mut current_depth = 0;
                    for h in &hits {
                        if h.depth != current_depth {
                            current_depth = h.depth;
                            println!(
                                "\n  {}Depth {} ({}):{}",
                                c.yellow,
                                current_depth,
                                if current_depth == 1 {
                                    "direct"
                                } else {
                                    "transitive"
                                },
                                c.reset,
                            );
                        }
                        println!(
                            "    {}{}{} {}(via {}){}",
                            c.green, h.file_path, c.reset, c.dim, h.via_symbol, c.reset,
                        );
                    }

                    println!("\n{} file(s) affected.", hits.len());
                }
                m => anyhow::bail!(
                    "invalid mode '{m}': must be 'search', 'navigate', 'map', or 'impact'"
                ),
            }
        }

        Commands::Store {
            content,
            title,
            tags,
            memory_type,
        } => {
            let store = Store::open(&db_path)?;
            let mut embedder = try_embedder();
            let result = grasshopper::memory::store(
                &store,
                embedder.as_mut(),
                &content,
                title.as_deref(),
                &tags,
                memory_type.as_deref(),
            )?;

            let c = colors();
            if result.was_update {
                println!(
                    "{}Updated{} memory #{} \"{}\"",
                    c.yellow, c.reset, result.id, result.title,
                );
            } else {
                println!(
                    "{}Stored{} memory #{} \"{}\"",
                    c.green, c.reset, result.id, result.title,
                );
            }
        }

        Commands::Status { json } => {
            cmd_status(&db_path, json)?;
        }

        Commands::Get { id, json } => {
            cmd_get(&db_path, id, json)?;
        }

        Commands::Memories {
            r#type,
            archived,
            limit,
            json,
        } => {
            cmd_memories(&db_path, r#type.as_deref(), archived, limit, json)?;
        }

        Commands::Serve { .. } => unreachable!(),
    }

    Ok(())
}

// --- Status command ---

fn cmd_status(db_path: &Path, json: bool) -> Result<()> {
    let store = Store::open(db_path)?;
    let c = colors();

    let (code_count, memory_count) = store.count_by_kind()?;
    let db_size = store.db_size_bytes()?;
    let codebases = store.codebase_stats()?;
    let mem_stats = store.memory_stats()?;
    let total_embedded = store.count_embedded()?;

    let hnsw_path = grasshopper::code::hnsw::hnsw_path(db_path);
    let hnsw_exists = hnsw_path.exists();
    let hnsw_size = if hnsw_exists {
        std::fs::metadata(&hnsw_path).map(|m| m.len()).unwrap_or(0)
    } else {
        0
    };

    // Check model cache
    let cache_dir = grasshopper::code::embed::default_cache_dir();
    let embedder_cached = cache_dir
        .join("models")
        .join("jina-embeddings-v2-base-code")
        .exists();

    if json {
        let out = serde_json::json!({
            "database": {
                "path": db_path.display().to_string(),
                "size_bytes": db_size,
            },
            "chunks": {
                "code": code_count,
                "memory": memory_count,
                "total": code_count + memory_count,
                "embedded": total_embedded,
            },
            "codebases": codebases.iter().map(|cb| serde_json::json!({
                "id": cb.id,
                "name": cb.name,
                "path": cb.root_path,
                "chunks": cb.chunk_count,
                "files": cb.file_count,
                "embedded": cb.embedded_count,
            })).collect::<Vec<_>>(),
            "memory": {
                "active": mem_stats.total,
                "archived": mem_stats.archived,
                "embedded": mem_stats.with_embeddings,
                "avg_salience": mem_stats.avg_salience,
                "by_type": mem_stats.by_type.iter().map(|(t, n)| serde_json::json!({
                    "type": t, "count": n,
                })).collect::<Vec<_>>(),
            },
            "hnsw": {
                "exists": hnsw_exists,
                "size_bytes": hnsw_size,
            },
            "models": {
                "embedder_cached": embedder_cached,
            },
        });
        println!("{}", serde_json::to_string_pretty(&out)?);
        return Ok(());
    }

    // Human-readable output
    println!(
        "{}Grasshopper{} {}v{}{}",
        c.bold,
        c.reset,
        c.dim,
        env!("CARGO_PKG_VERSION"),
        c.reset
    );
    println!();

    // Database
    println!("{}Database{}", c.bold, c.reset);
    println!("  Path:  {}", db_path.display());
    println!("  Size:  {}", format_bytes(db_size as u64));
    println!(
        "  Total: {} chunks ({} code, {} memory)",
        code_count + memory_count,
        code_count,
        memory_count
    );
    println!(
        "  Embedded: {}/{} chunks",
        total_embedded,
        code_count + memory_count
    );
    println!();

    // Codebases
    if codebases.is_empty() {
        println!(
            "{}Codebases{} {}(none indexed){}\n",
            c.bold, c.reset, c.dim, c.reset
        );
    } else {
        println!("{}Codebases{}", c.bold, c.reset);
        for cb in &codebases {
            let name = if cb.name.is_empty() {
                &cb.root_path
            } else {
                &cb.name
            };
            let embed_pct = if cb.chunk_count > 0 {
                (cb.embedded_count as f64 / cb.chunk_count as f64 * 100.0) as u64
            } else {
                0
            };
            println!(
                "  {}{}{} {} files, {} chunks ({}% embedded)",
                c.green, name, c.reset, cb.file_count, cb.chunk_count, embed_pct
            );
            println!("    {}{}{}", c.dim, cb.root_path, c.reset);
        }
        println!();
    }

    // Memory
    println!("{}Memory{}", c.bold, c.reset);
    if mem_stats.total == 0 && mem_stats.archived == 0 {
        println!("  {}(empty){}\n", c.dim, c.reset);
    } else {
        println!(
            "  Active: {}  Archived: {}  Avg salience: {:.2}",
            mem_stats.total, mem_stats.archived, mem_stats.avg_salience
        );
        if !mem_stats.by_type.is_empty() {
            let types: Vec<String> = mem_stats
                .by_type
                .iter()
                .map(|(t, n)| format!("{t}: {n}"))
                .collect();
            println!("  {}{}{}", c.dim, types.join("  "), c.reset);
        }
        println!();
    }

    // HNSW
    println!("{}Search Index{}", c.bold, c.reset);
    if hnsw_exists {
        println!(
            "  HNSW:     {}ready{} ({})",
            c.green,
            c.reset,
            format_bytes(hnsw_size)
        );
    } else {
        println!(
            "  HNSW:     {}not built{} (run index --embed)",
            c.yellow, c.reset
        );
    }
    if embedder_cached {
        println!("  Embedder: {}cached{}", c.green, c.reset);
    } else {
        println!(
            "  Embedder: {}not downloaded{} (downloads on first use, ~270 MB)",
            c.yellow, c.reset
        );
    }

    Ok(())
}

// --- Get command ---

fn cmd_get(db_path: &Path, id: i64, json: bool) -> Result<()> {
    let store = Store::open(db_path)?;
    let c = colors();

    let chunk = store
        .get_chunk(id)?
        .ok_or_else(|| anyhow::anyhow!("chunk #{id} not found"))?;

    if json {
        println!("{}", serde_json::to_string_pretty(&chunk)?);
        return Ok(());
    }

    // Header
    match chunk.kind.as_str() {
        "code" => {
            let name = chunk.symbol_name.as_deref().unwrap_or("(unnamed)");
            let kind = chunk.symbol_kind.as_deref().unwrap_or("block");
            let file = chunk.file_path.as_deref().unwrap_or("?");
            let lines = match (chunk.start_line, chunk.end_line) {
                (Some(s), Some(e)) => format!(":{s}-{e}"),
                _ => String::new(),
            };
            println!(
                "{}#{}{} {}{kind} {name}{} in {}{file}{lines}{}",
                c.dim, id, c.reset, c.magenta, c.reset, c.green, c.reset
            );
            if let Some(lang) = &chunk.language {
                println!("  {}Language: {lang}{}", c.dim, c.reset);
            }
            if let Some(sig) = &chunk.signature {
                if !sig.is_empty() {
                    println!("  {}Signature: {sig}{}", c.dim, c.reset);
                }
            }
        }
        "memory" => {
            let mtype = chunk.memory_type.as_deref().unwrap_or("?");
            println!(
                "{}#{}{} {}[{mtype}]{} {}{}{}",
                c.dim, id, c.reset, c.blue, c.reset, c.bold, chunk.title, c.reset
            );
            println!(
                "  Salience: {:.2}  Archived: {}",
                chunk.salience,
                if chunk.archived { "yes" } else { "no" }
            );
            if let Some(la) = &chunk.last_accessed {
                println!("  Last accessed: {la}");
            }
            if !chunk.descriptors.is_empty() {
                println!("  Tags: {}", chunk.descriptors);
            }
        }
        _ => {
            println!("#{id} [{}] {}", chunk.kind, chunk.title);
        }
    }

    // Timestamps
    println!(
        "  {}Created: {}  Updated: {}{}",
        c.dim, chunk.created_at, chunk.updated_at, c.reset
    );
    println!();

    // Content
    println!("{}", chunk.content);

    Ok(())
}

// --- Memories command ---

fn cmd_memories(
    db_path: &Path,
    memory_type: Option<&str>,
    include_archived: bool,
    limit: usize,
    json: bool,
) -> Result<()> {
    let store = Store::open(db_path)?;
    let c = colors();

    // Validate memory type if provided
    if let Some(mt) = memory_type {
        match mt {
            "identity" | "knowledge" | "episode" | "procedure" => {}
            t => anyhow::bail!(
                "invalid memory type '{t}': must be identity, knowledge, episode, or procedure"
            ),
        }
    }

    let memories = store.list_memories(memory_type, include_archived, limit)?;

    if json {
        println!("{}", serde_json::to_string_pretty(&memories)?);
        if memories.is_empty() {
            std::process::exit(EXIT_NO_RESULTS);
        }
        return Ok(());
    }

    if memories.is_empty() {
        println!("No memories found.");
        std::process::exit(EXIT_NO_RESULTS);
    }

    let type_label = memory_type.unwrap_or("all");
    println!(
        "{}Memories{} ({type_label}, {} entries){}",
        c.bold,
        c.reset,
        memories.len(),
        if include_archived { " [+archived]" } else { "" }
    );
    println!();

    for mem in &memories {
        let mtype = mem.memory_type.as_deref().unwrap_or("?");
        let age = format_age(&mem.created_at);
        let archived_marker = if mem.archived {
            format!(" {}[archived]{}", c.yellow, c.reset)
        } else {
            String::new()
        };

        println!(
            "  {}#{:<5}{} {}[{mtype}]{} {}{}{}{archived_marker}",
            c.dim, mem.id, c.reset, c.blue, c.reset, c.bold, mem.title, c.reset,
        );
        println!(
            "         {}sal:{:.2}  age:{}  tags:{}{}",
            c.dim,
            mem.salience,
            age,
            if mem.descriptors.is_empty() {
                "-"
            } else {
                &mem.descriptors
            },
            c.reset,
        );

        // Show first line of content as preview
        if let Some(first_line) = mem.content.lines().next() {
            let preview = if first_line.len() > 80 {
                let mut end = 77;
                while end > 0 && !first_line.is_char_boundary(end) {
                    end -= 1;
                }
                format!("{}...", &first_line[..end])
            } else {
                first_line.to_string()
            };
            println!("         {}{}{}", c.dim, preview, c.reset);
        }
        println!();
    }

    Ok(())
}

// --- Helpers ---

fn format_line(line: i64) -> String {
    format!(":{line}")
}

fn format_bytes(bytes: u64) -> String {
    if bytes < 1024 {
        format!("{bytes} B")
    } else if bytes < 1024 * 1024 {
        format!("{:.1} KB", bytes as f64 / 1024.0)
    } else if bytes < 1024 * 1024 * 1024 {
        format!("{:.1} MB", bytes as f64 / (1024.0 * 1024.0))
    } else {
        format!("{:.1} GB", bytes as f64 / (1024.0 * 1024.0 * 1024.0))
    }
}

fn format_age(iso_date: &str) -> String {
    let Ok(dt) = chrono::DateTime::parse_from_rfc3339(iso_date) else {
        return "?".into();
    };
    let now = chrono::Utc::now();
    let duration = now.signed_duration_since(dt);
    let days = duration.num_days();
    if days == 0 {
        let hours = duration.num_hours();
        if hours == 0 {
            format!("{}m", duration.num_minutes().max(1))
        } else {
            format!("{hours}h")
        }
    } else if days < 30 {
        format!("{days}d")
    } else {
        format!("{}mo", days / 30)
    }
}

fn snippet_preview(snippet: &str, max_lines: usize) -> String {
    let lines: Vec<&str> = snippet.lines().take(max_lines).collect();
    let mut out = String::new();
    for line in &lines {
        out.push_str("    ");
        if line.len() > 120 {
            // Find a valid char boundary at or before byte 120
            let mut end = 120;
            while end > 0 && !line.is_char_boundary(end) {
                end -= 1;
            }
            out.push_str(&line[..end]);
            out.push_str("...");
        } else {
            out.push_str(line);
        }
        out.push('\n');
    }
    out
}

fn print_hit(rank: usize, hit: &SearchHit) {
    let c = colors();
    match hit.kind.as_str() {
        "code" => {
            let name = hit.symbol_name.as_deref().unwrap_or("?");
            let kind = hit.symbol_kind.as_deref().unwrap_or("?");
            let file = hit.file_path.as_deref().unwrap_or("?");
            let lines = match (hit.start_line, hit.end_line) {
                (Some(s), Some(e)) => format!(":{s}-{e}"),
                _ => String::new(),
            };
            println!(
                "{}{}. {}{file}{lines}{} {}{kind} {name}{} {}{:.4}{}",
                c.dim, rank, c.green, c.reset, c.magenta, c.reset, c.dim, hit.score, c.reset,
            );
            if !hit.snippet.is_empty() {
                print!("{}", snippet_preview(&hit.snippet, 4));
            }
        }
        "memory" => {
            let mtype = hit.memory_type.as_deref().unwrap_or("?");
            println!(
                "{}{}. {}[{mtype}]{} {}{}{} {}{:.4}{}",
                c.dim, rank, c.blue, c.reset, c.bold, hit.title, c.reset, c.dim, hit.score, c.reset,
            );
            if !hit.snippet.is_empty() {
                print!("{}{}", c.dim, snippet_preview(&hit.snippet, 2));
                print!("{}", c.reset);
            }
        }
        _ => {
            println!(
                "{}. [{}] {} {}{:.4}{}",
                rank, hit.kind, hit.title, c.dim, hit.score, c.reset,
            );
        }
    }
}

fn rebuild_hnsw(store: &Store, db_path: &Path) -> Result<()> {
    let rows = store.get_all_embeddings()?;
    if rows.is_empty() {
        println!("No embeddings found — skipping HNSW build.");
        return Ok(());
    }
    let index = grasshopper::code::hnsw::HnswIndex::from_embeddings(&rows)?;
    let hnsw_path = grasshopper::code::hnsw::hnsw_path(db_path);
    index.save(&hnsw_path)?;
    println!(
        "HNSW index: {} points → {}",
        index.len(),
        hnsw_path.display()
    );
    Ok(())
}
