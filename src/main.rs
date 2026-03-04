use anyhow::Result;
use clap::{Parser, Subcommand};
use grasshopper::memory::{self, ReflectFocus};
use grasshopper::store::{SearchHit, Store};
use std::path::{Path, PathBuf};

#[derive(Parser)]
#[command(
    name = "grasshopper",
    version,
    about = "Unified agent brain — code intelligence + cognitive memory"
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

    /// Search code and memory
    Search {
        /// Search query
        query: String,
        /// Filter: code, memory, or all
        #[arg(long, default_value = "all")]
        kind: String,
        /// Maximum results
        #[arg(long, default_value = "10")]
        limit: usize,
    },

    /// Store a memory (with auto-classification and dedup)
    Remember {
        /// Memory content
        content: String,
        /// Optional title (auto-generated if omitted)
        #[arg(long)]
        title: Option<String>,
        /// Memory type (auto-classified if omitted)
        #[arg(long, value_parser = ["identity", "knowledge", "episode", "procedure"])]
        r#type: Option<String>,
        /// Descriptors (comma-separated tags)
        #[arg(long, default_value = "")]
        tags: String,
    },

    /// Cognitive-scored memory search
    Recall {
        /// Search query
        query: String,
        /// Maximum results
        #[arg(long, default_value = "10")]
        limit: usize,
    },

    /// Identity snapshot: who am I, what am I working on
    Me,

    /// Resume a previous session (load latest handoff + related memories)
    Pickup {
        /// Filter by project name
        #[arg(long)]
        project: Option<String>,
    },

    /// Meta-cognition analytics: growing, fading, connections, gaps
    Reflect {
        /// Focus area
        #[arg(long, default_value = "overview", value_parser = ["overview", "growing", "fading", "connections", "gaps"])]
        focus: String,
    },

    /// Memory hygiene: find duplicates, archive stale, cluster episodes
    Consolidate {
        /// Preview only, don't archive anything
        #[arg(long)]
        dry_run: bool,
        /// Days of inactivity before considering stale
        #[arg(long, default_value = "90")]
        stale_days: i64,
    },

    /// Rebuild the HNSW vector index from all embeddings
    RebuildHnsw,

    /// Show memory statistics
    Stats,

    /// Start the MCP server (HTTP daemon mode)
    Serve {
        /// Port to listen on
        #[arg(long, default_value = "8106")]
        port: u16,
    },

    /// Start the MCP server (stdio, for direct Claude Code integration)
    Mcp,
}

fn default_db_path() -> PathBuf {
    let home = std::env::var("HOME").unwrap_or_else(|_| ".".to_string());
    PathBuf::from(home).join(".grasshopper").join("brain.db")
}

fn try_embedder() -> Option<ferret::embed::Embedder> {
    let cache_dir = ferret::embed::default_cache_dir();
    ferret::embed::Embedder::new(&cache_dir).ok()
}

fn try_hnsw(db_path: &Path) -> Option<ferret::hnsw::HnswIndex> {
    let hnsw_path = ferret::hnsw::hnsw_path(db_path);
    if hnsw_path.exists() {
        ferret::hnsw::HnswIndex::load(&hnsw_path).ok()
    } else {
        None
    }
}

fn main() -> Result<()> {
    let cli = Cli::parse();
    let db_path = cli.db.unwrap_or_else(default_db_path);

    // MCP commands init their own tracing — handle before CLI tracing
    match cli.command {
        Commands::Serve { port } => {
            let rt = tokio::runtime::Runtime::new()?;
            return rt.block_on(grasshopper::mcp::run_http(db_path, port));
        }
        Commands::Mcp => {
            let rt = tokio::runtime::Runtime::new()?;
            return rt.block_on(grasshopper::mcp::run_stdio(db_path));
        }
        _ => {}
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
                let cache_dir = ferret::embed::default_cache_dir();
                let mut embedder = ferret::embed::Embedder::new(&cache_dir)?;
                let embedded =
                    grasshopper::index::embed_codebase(&store, &mut embedder, result.codebase_id)?;
                println!("Embedded: {} chunks", embedded);

                // Rebuild HNSW index after embedding
                rebuild_hnsw(&store, &db_path)?;
            }

            println!("Done in {}ms", result.duration_ms);
        }

        Commands::Search { query, kind, limit } => {
            let store = Store::open(&db_path)?;
            let kind_filter = match kind.as_str() {
                "all" => None,
                k => Some(k),
            };

            let mut embedder = try_embedder();
            let hnsw = try_hnsw(&db_path);
            let results = grasshopper::search::search(
                &store,
                &query,
                kind_filter,
                limit,
                embedder.as_mut(),
                None,
                hnsw.as_ref(),
            )?;

            if results.is_empty() {
                println!("No results found.");
                return Ok(());
            }

            for (i, hit) in results.iter().enumerate() {
                println!("{}. [{:.4}] {}", i + 1, hit.score, format_hit(hit));
            }
        }

        Commands::Remember {
            content,
            title,
            r#type,
            tags,
        } => {
            let store = Store::open(&db_path)?;
            let mut embedder = try_embedder();
            let result = memory::remember(
                &store,
                embedder.as_mut(),
                &content,
                title.as_deref(),
                r#type.as_deref(),
                &tags,
            )?;

            if result.was_update {
                println!(
                    "Updated memory #{} (type: {}, salience: {:.2})",
                    result.id, result.memory_type, result.salience,
                );
            } else {
                println!(
                    "Stored memory #{} (type: {}, salience: {:.2})",
                    result.id, result.memory_type, result.salience,
                );
            }
        }

        Commands::Recall { query, limit } => {
            let store = Store::open(&db_path)?;
            let mut embedder = try_embedder();
            let hnsw = try_hnsw(&db_path);
            let result = memory::recall(&store, embedder.as_mut(), None, &query, limit, hnsw.as_ref())?;

            if result.hits.is_empty() {
                println!("No memories found.");
                return Ok(());
            }

            for (i, hit) in result.hits.iter().enumerate() {
                let mtype = hit.memory_type.as_deref().unwrap_or("?");
                println!(
                    "{}. [{:.4}] [{}] {} (accessed: {}, salience: {:.2})",
                    i + 1, hit.score, mtype, hit.title, hit.access_count, hit.salience,
                );
                if !hit.snippet.is_empty() {
                    let preview: String = hit.snippet.chars().take(120).collect();
                    println!("   {preview}");
                }
            }
        }

        Commands::Me => {
            let store = Store::open(&db_path)?;
            let result = memory::me(&store)?;

            if !result.identity.is_empty() {
                println!("## Identity");
                for entry in &result.identity {
                    println!("  - {}: {}", entry.title, truncate(&entry.content, 100));
                }
                println!();
            }

            if !result.active_projects.is_empty() {
                println!("## Active Projects");
                for h in &result.active_projects {
                    println!("  - [{}] {} → {}", h.project, h.summary, h.next_steps);
                }
                println!();
            }

            if !result.working_set.is_empty() {
                println!("## Working Set (most accessed)");
                for entry in &result.working_set {
                    let mtype = entry.memory_type.as_deref().unwrap_or("?");
                    println!(
                        "  - [{}] {} (accessed: {}, salience: {:.2})",
                        mtype, entry.title, entry.access_count, entry.salience,
                    );
                }
                println!();
            }

            println!(
                "## Stats: {} active, {} archived",
                result.active_count, result.archived_count,
            );
        }

        Commands::Pickup { project } => {
            let store = Store::open(&db_path)?;
            let mut embedder = try_embedder();
            let hnsw = try_hnsw(&db_path);
            let result = memory::pickup(&store, embedder.as_mut(), None, project.as_deref(), hnsw.as_ref())?;

            match result.handoff {
                Some(h) => {
                    println!("## Last Handoff ({})", h.project);
                    println!("Summary: {}", h.summary);
                    println!("Next steps: {}", h.next_steps);
                    println!("Created: {}", h.created_at);
                }
                None => println!("No handoff found."),
            }

            if !result.related_memories.is_empty() {
                println!("\n## Related Memories");
                for (i, hit) in result.related_memories.iter().enumerate() {
                    let mtype = hit.memory_type.as_deref().unwrap_or("?");
                    println!("  {}. [{}] {}", i + 1, mtype, hit.title);
                }
            }
        }

        Commands::Reflect { focus } => {
            let store = Store::open(&db_path)?;
            let focus = match focus.as_str() {
                "growing" => ReflectFocus::Growing,
                "fading" => ReflectFocus::Fading,
                "connections" => ReflectFocus::Connections,
                "gaps" => ReflectFocus::Gaps,
                _ => ReflectFocus::Overview,
            };

            let result = memory::reflect(&store, &focus)?;

            if !result.growing.is_empty() {
                println!("## Growing (recently active)");
                for entry in &result.growing {
                    println!(
                        "  - {} (accessed: {}, salience: {:.2})",
                        entry.title, entry.access_count, entry.salience,
                    );
                }
                println!();
            }

            if !result.fading.is_empty() {
                println!("## Fading (need attention)");
                for entry in &result.fading {
                    let last = entry.last_accessed.as_deref().unwrap_or("never");
                    println!("  - {} (last: {})", entry.title, last);
                }
                println!();
            }

            if !result.connections.is_empty() {
                println!("## Connected (Hebbian links)");
                for (entry, count) in &result.connections {
                    println!("  - {} ({} associations)", entry.title, count);
                }
                println!();
            }

            if !result.type_counts.is_empty() {
                println!("## Memory Types");
                for (mtype, count) in &result.type_counts {
                    println!("  - {}: {}", mtype, count);
                }
                println!();
            }

            for obs in &result.observations {
                println!("! {obs}");
            }

            println!(
                "\n{} active, {} archived",
                result.active_count, result.archived_count,
            );
        }

        Commands::Consolidate {
            dry_run,
            stale_days,
        } => {
            let store = Store::open(&db_path)?;
            let mut embedder = try_embedder();
            let result =
                memory::consolidate(&store, embedder.as_mut(), dry_run, stale_days)?;

            if dry_run {
                println!("(dry run — no changes made)\n");
            }

            if !result.near_duplicates.is_empty() {
                println!("## Near-Duplicates ({} pairs)", result.near_duplicates.len());
                for (id_a, id_b, sim) in &result.near_duplicates {
                    println!("  #{id_a} <-> #{id_b} (similarity: {sim:.4})");
                }
                println!();
            }

            if !result.auto_archived.is_empty() {
                println!(
                    "## Auto-Archived ({} entries with zero accesses)",
                    result.auto_archived.len(),
                );
                for id in &result.auto_archived {
                    println!("  #{id}");
                }
                println!();
            }

            if !result.stale_for_review.is_empty() {
                println!(
                    "## Stale (review needed, {} entries)",
                    result.stale_for_review.len(),
                );
                for entry in &result.stale_for_review {
                    let last = entry.last_accessed.as_deref().unwrap_or("never");
                    println!(
                        "  - #{} {} (accessed: {}, last: {})",
                        entry.id, entry.title, entry.access_count, last,
                    );
                }
                println!();
            }

            if !result.episode_clusters.is_empty() {
                println!(
                    "## Episode Clusters ({} tags with 3+ episodes)",
                    result.episode_clusters.len(),
                );
                for (tag, entries) in &result.episode_clusters {
                    println!("  [{}] {} episodes", tag, entries.len());
                }
            }

            if result.near_duplicates.is_empty()
                && result.auto_archived.is_empty()
                && result.stale_for_review.is_empty()
                && result.episode_clusters.is_empty()
            {
                println!("Memory is clean — nothing to consolidate.");
            }
        }

        Commands::RebuildHnsw => {
            let store = Store::open(&db_path)?;
            rebuild_hnsw(&store, &db_path)?;
        }

        Commands::Stats => {
            let store = Store::open(&db_path)?;
            let (code, memory) = store.count_by_kind()?;
            let types = store.count_memories_by_type()?;

            println!("Code chunks:  {}", code);
            println!("Memories:     {}", memory);
            if !types.is_empty() {
                for (mtype, count) in &types {
                    println!("  {}: {}", mtype, count);
                }
            }
            println!("Database:     {}", db_path.display());
        }

        Commands::Serve { .. } | Commands::Mcp => unreachable!(),
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

fn truncate(s: &str, max: usize) -> String {
    if s.chars().count() <= max {
        s.to_string()
    } else {
        let truncated: String = s.chars().take(max.saturating_sub(3)).collect();
        format!("{truncated}...")
    }
}

fn rebuild_hnsw(store: &Store, db_path: &Path) -> Result<()> {
    let rows = store.get_all_embeddings()?;
    if rows.is_empty() {
        println!("No embeddings found — skipping HNSW build.");
        return Ok(());
    }
    let index = ferret::hnsw::HnswIndex::from_embeddings(&rows)?;
    let hnsw_path = ferret::hnsw::hnsw_path(db_path);
    index.save(&hnsw_path)?;
    println!("HNSW index: {} points → {}", index.len(), hnsw_path.display());
    Ok(())
}
