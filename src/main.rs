use anyhow::Result;
use clap::{Parser, Subcommand};
use grasshopper::store::Store;
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
        #[arg(long, default_value = "knowledge")]
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
    dirs_next().join("brain.db")
}

fn dirs_next() -> PathBuf {
    let home = std::env::var("HOME").unwrap_or_else(|_| ".".to_string());
    PathBuf::from(home).join(".grasshopper")
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
        Commands::Index { dir } => {
            let _store = Store::open(&db_path)?;
            println!("TODO: index {}", dir.display());
            println!("(Phase 1 — not yet implemented)");
        }

        Commands::Search { query, kind, limit } => {
            let _store = Store::open(&db_path)?;
            println!("TODO: search '{}' kind={} limit={}", query, kind, limit);
            println!("(Phase 1/2 — not yet implemented)");
        }

        Commands::Remember {
            content,
            title,
            r#type,
            tags,
        } => {
            let store = Store::open(&db_path)?;
            let title = title.unwrap_or_else(|| {
                content.split('.').next().unwrap_or(&content)
                    .chars().take(80).collect()
            });
            let salience = if r#type == "identity" { 1.0 } else { 0.5 };
            let hash = format!("{:x}", Sha256::new()
                .chain_update(title.as_bytes())
                .chain_update(content.as_bytes())
                .finalize());
            let id = store.insert_memory(&title, &content, &r#type, &tags, salience, &hash, "cli")?;
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
