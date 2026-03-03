use anyhow::Result;
use clap::{Parser, Subcommand};
use grasshopper::store::{MemoryParams, SearchHit, Store};
use sha2::{Digest, Sha256};
use std::path::PathBuf;

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

    /// Store a memory
    Remember {
        /// Memory content
        content: String,
        /// Optional title
        #[arg(long)]
        title: Option<String>,
        /// Memory type: identity, knowledge, episode, procedure
        #[arg(long, default_value = "knowledge", value_parser = ["identity", "knowledge", "episode", "procedure"])]
        r#type: String,
        /// Descriptors (comma-separated tags)
        #[arg(long, default_value = "")]
        tags: String,
    },

    /// Show memory statistics
    Stats,

    /// Start the MCP server
    Serve {
        /// Port to listen on
        #[arg(long, default_value = "8101")]
        port: u16,
    },
}

fn default_db_path() -> PathBuf {
    let home = std::env::var("HOME").unwrap_or_else(|_| ".".to_string());
    PathBuf::from(home).join(".grasshopper").join("brain.db")
}

fn main() -> Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("warn")),
        )
        .init();

    let cli = Cli::parse();
    let db_path = cli.db.unwrap_or_else(default_db_path);

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
            }

            println!("Done in {}ms", result.duration_ms);
        }

        Commands::Search { query, kind, limit } => {
            let store = Store::open(&db_path)?;
            let kind_filter = match kind.as_str() {
                "all" => None,
                k => Some(k),
            };

            // Try loading embedder for hybrid search (falls back to FTS-only)
            let cache_dir = ferret::embed::default_cache_dir();
            let mut embedder = ferret::embed::Embedder::new(&cache_dir).ok();

            let results = grasshopper::search::search(
                &store,
                &query,
                kind_filter,
                limit,
                embedder.as_mut(),
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
            let title = title.unwrap_or_else(|| {
                content
                    .split('.')
                    .next()
                    .unwrap_or(&content)
                    .chars()
                    .take(80)
                    .collect()
            });
            let salience = if r#type == "identity" { 1.0 } else { 0.5 };
            let hash = format!(
                "{:x}",
                Sha256::new()
                    .chain_update(title.as_bytes())
                    .chain_update(content.as_bytes())
                    .finalize()
            );
            let id = store.insert_memory(&MemoryParams {
                title: &title,
                content: &content,
                memory_type: &r#type,
                descriptors: &tags,
                salience,
                content_hash: &hash,
                agent_id: "cli",
            })?;
            println!("Stored memory #{} (type: {}, salience: {})", id, r#type, salience);
        }

        Commands::Stats => {
            let store = Store::open(&db_path)?;
            let (code, memory) = store.count_by_kind()?;
            println!("Code chunks:  {}", code);
            println!("Memories:     {}", memory);
            println!("Database:     {}", db_path.display());
        }

        Commands::Serve { port } => {
            println!("TODO: MCP server on port {}", port);
            println!("(Phase 3 — not yet implemented)");
        }
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
