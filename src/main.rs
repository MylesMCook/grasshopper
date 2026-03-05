use anyhow::Result;
use clap::{Parser, Subcommand};
use grasshopper::store::{SearchHit, Store};
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

fn default_db_path() -> PathBuf {
    let home = std::env::var("HOME").unwrap_or_else(|_| ".".to_string());
    PathBuf::from(home).join(".grasshopper").join("brain.db")
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

fn main() -> Result<()> {
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

            println!(
                "Indexed {} ({} files, {} chunks, {} edges)",
                dir.display(),
                result.files_scanned,
                result.chunks_written,
                result.edges_written,
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

            println!("Done in {}ms", result.duration_ms);
        }

        Commands::Search {
            query, mode, kind, limit, threshold, budget, depth, dir, direction,
        } => {
            let store = Store::open(&db_path)?;

            match mode.as_str() {
                "search" => {
                    let kind_filter = match kind.as_str() {
                        "all" => None,
                        k => Some(k),
                    };

                    let mut embedder = try_embedder();
                    let mut reranker = try_reranker();
                    let hnsw = try_hnsw(&db_path);
                    let result = grasshopper::search::unified_search(
                        &store,
                        &query,
                        kind_filter,
                        limit,
                        threshold,
                        embedder.as_mut(),
                        reranker.as_mut(),
                        hnsw.as_ref(),
                    )?;

                    if result.hits.is_empty() {
                        println!("No results found.");
                        return Ok(());
                    }

                    for (i, hit) in result.hits.iter().enumerate() {
                        println!("{}. [{:.4}] {}", i + 1, hit.score, format_hit(hit));
                    }
                }
                "navigate" => {
                    let codebases = store.list_codebases()?;
                    let codebase_id = resolve_codebase_cli(&codebases, dir.as_deref())?;

                    let (show_defs, show_refs) = match direction.as_str() {
                        "both" => (true, true),
                        "defs" | "def" => (true, false),
                        "refs" | "ref" => (false, true),
                        d => anyhow::bail!("invalid direction '{d}': must be 'both', 'defs', or 'refs'"),
                    };

                    if show_defs {
                        let defs = store.find_definitions(&query, codebase_id)?;
                        if !defs.is_empty() {
                            println!("Definitions of '{query}':");
                            for d in &defs {
                                println!("  {}:{} ({} {})", d.file_path, d.line, d.kind, d.symbol);
                            }
                        }
                    }

                    if show_refs {
                        let refs = store.find_references(&query, codebase_id)?;
                        if !refs.is_empty() {
                            println!("References to '{query}':");
                            for r in &refs {
                                println!("  {}:{} ({} {})", r.file_path, r.line, r.kind, r.symbol);
                            }
                        }
                    }
                }
                "map" => {
                    let codebases = store.list_codebases()?;
                    let codebase_id = resolve_codebase_cli(&codebases, dir.as_deref())?
                        .ok_or_else(|| anyhow::anyhow!("no codebases indexed — run index first"))?;
                    let output = grasshopper::mcp::generate_map_cli(&store, codebase_id, budget)?;
                    print!("{output}");
                }
                "impact" => {
                    let codebases = store.list_codebases()?;
                    let codebase_id = resolve_codebase_cli(&codebases, dir.as_deref())?;
                    let max_depth = depth.clamp(1, 5);
                    let hits = store.find_impact(&query, codebase_id, max_depth)?;

                    if hits.is_empty() {
                        println!("No impact found for '{query}'.");
                        return Ok(());
                    }

                    let defs = store.find_definitions(&query, codebase_id)?;
                    if !defs.is_empty() {
                        let locs: Vec<String> =
                            defs.iter().map(|d| format!("{}:{}", d.file_path, d.line)).collect();
                        println!("Impact of changing '{}' (defined at {}):", query, locs.join(", "));
                    } else {
                        println!("Impact of changing '{query}':");
                    }

                    let mut current_depth = 0;
                    for h in &hits {
                        if h.depth != current_depth {
                            current_depth = h.depth;
                            println!(
                                "\n  Depth {} ({}):",
                                current_depth,
                                if current_depth == 1 { "direct" } else { "transitive" },
                            );
                        }
                        println!("    {} (via {})", h.file_path, h.via_symbol);
                    }

                    println!("\n{} file(s) affected.", hits.len());
                }
                m => anyhow::bail!("invalid mode '{m}': must be 'search', 'navigate', 'map', or 'impact'"),
            }
        }

        Commands::Store {
            content,
            title,
            tags,
        } => {
            let store = Store::open(&db_path)?;
            let mut embedder = try_embedder();
            let result = grasshopper::memory::store(
                &store,
                embedder.as_mut(),
                &content,
                title.as_deref(),
                &tags,
            )?;

            if result.was_update {
                println!(
                    "Updated memory #{} \"{}\"",
                    result.id, result.title,
                );
            } else {
                println!(
                    "Stored memory #{} \"{}\"",
                    result.id, result.title,
                );
            }
        }

        Commands::Serve { .. } => unreachable!(),
    }

    Ok(())
}

fn format_hit(hit: &SearchHit) -> String {
    match hit.kind.as_str() {
        "code" => {
            let name = hit.symbol_name.as_deref().unwrap_or("?");
            let kind = hit.symbol_kind.as_deref().unwrap_or("?");
            let file = hit.file_path.as_deref().unwrap_or("?");
            let lines = match (hit.start_line, hit.end_line) {
                (Some(s), Some(e)) => format!(":{s}-{e}"),
                _ => String::new(),
            };
            format!("{kind} {name} in {file}{lines}")
        }
        "memory" => {
            let mtype = hit.memory_type.as_deref().unwrap_or("?");
            format!("[{}] {}", mtype, hit.title)
        }
        _ => format!("{}: {}", hit.kind, hit.title),
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
    println!("HNSW index: {} points → {}", index.len(), hnsw_path.display());
    Ok(())
}

/// CLI-specific codebase resolution (works with pre-fetched codebases list).
fn resolve_codebase_cli(
    codebases: &[(i64, String, String)],
    dir: Option<&str>,
) -> Result<Option<i64>> {
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
                    "ambiguous dir '{dir}' matches {} codebases: {}",
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
            "Multiple codebases indexed. Use --dir to select one: {}",
            names.join(", ")
        );
    }
}
