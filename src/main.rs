use anyhow::Result;
use clap::{Parser, Subcommand};
use grasshopper::store::{SearchHit, Store};
use std::io::IsTerminal;
use std::path::{Path, PathBuf};

#[derive(Parser)]
#[command(
    name = "grasshopper",
    version,
    about = "Persistent retrieval engine for AI agents"
)]
struct Cli {
    /// Database file path (default: ~/.grasshopper/brain.db)
    #[arg(long, global = true)]
    db: Option<PathBuf>,

    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand)]
enum Commands {
    /// Index a directory of source code
    Index {
        /// Directory to index
        dir: PathBuf,
        /// Generate embeddings for semantic search (slow, ~4/sec on CPU)
        #[arg(long)]
        embed: bool,
    },

    /// Search code and memory, navigate symbols, map codebases, or analyze impact
    Search {
        /// Search query
        query: String,
        /// Search mode: search (default), navigate, map, impact
        #[arg(long, default_value = "search")]
        mode: String,
        /// Filter: code, memory, or all
        #[arg(long, default_value = "all")]
        kind: String,
        /// Maximum results
        #[arg(long, default_value = "10")]
        limit: usize,
        /// Minimum relevance threshold (0.0-1.0) for memory results
        #[arg(long)]
        threshold: Option<f32>,
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
        /// Output as JSON (machine-readable)
        #[arg(long)]
        json: bool,
    },

    /// Store raw content as a persistent memory (with dedup)
    Store {
        /// Memory content
        content: String,
        /// Optional title (truncated from content if omitted)
        #[arg(long)]
        title: Option<String>,
        /// Descriptors (comma-separated tags)
        #[arg(long, default_value = "")]
        tags: String,
        /// Memory type: knowledge (default), identity, episode, procedure
        #[arg(long)]
        memory_type: Option<String>,
    },

    /// Start the MCP server (HTTP by default, or stdio with --stdio)
    Serve {
        /// Port to listen on
        #[arg(long, default_value = "8106")]
        port: u16,
        /// Use stdio transport instead of HTTP (for direct Claude Code integration)
        #[arg(long)]
        stdio: bool,
    },
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
            budget,
            depth,
            dir,
            direction,
            json,
        } => {
            let store = Store::open(&db_path)?;

            let limit = limit.clamp(1, 100);
            let budget = budget.clamp(1, 200_000);

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

        Commands::Serve { .. } => unreachable!(),
    }

    Ok(())
}

fn format_line(line: i64) -> String {
    format!(":{line}")
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
