use anyhow::{Context, Result};
use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex};

use crate::code::embed::Embedder;
use crate::store::Store;

use super::GrasshopperMcp;

/// Run startup maintenance: log DB stats, PRAGMA optimize.
fn startup_maintenance(db_path: &Path) {
    match Store::open(db_path) {
        Ok(store) => {
            let (code, memory) = store.count_by_kind().unwrap_or((0, 0));
            let db_bytes = store.db_size_bytes().unwrap_or(0);
            let codebases = store.count_codebases().unwrap_or(0);
            tracing::info!(
                "startup: {} code chunks, {} memories, {:.1} KB, {} codebases",
                code,
                memory,
                db_bytes as f64 / 1024.0,
                codebases
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

    // Warm up embedder in background (eliminates ~500ms cold start on first search)
    let emb_arc = Arc::clone(&server.embedder);
    tokio::task::spawn_blocking(move || GrasshopperMcp::init_embedder_blocking(&emb_arc));

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
        StreamableHttpServerConfig, StreamableHttpService, session::local::LocalSessionManager,
    };
    use tower_http::cors::CorsLayer;

    let ct = tokio_util::sync::CancellationToken::new();
    let shared_embedder: Arc<Mutex<Option<Embedder>>> = Arc::new(Mutex::new(None));
    let shared_reranker: Arc<Mutex<Option<crate::rerank::Reranker>>> = Arc::new(Mutex::new(None));
    let shared_reranker_failed_at: Arc<std::sync::atomic::AtomicI64> =
        Arc::new(std::sync::atomic::AtomicI64::new(0));
    let shared_hnsw: Arc<Mutex<Option<crate::code::hnsw::HnswIndex>>> =
        Arc::new(Mutex::new(GrasshopperMcp::try_load_hnsw(&db_path)));

    startup_maintenance(&db_path);

    // Warm up embedder in background (eliminates ~500ms cold start on first search).
    // Reranker stays lazy — it loads on first memory search to keep startup VmRSS lower.
    let emb_warmup = shared_embedder.clone();
    tokio::task::spawn_blocking(move || GrasshopperMcp::init_embedder_blocking(&emb_warmup));

    let db = db_path.clone();
    let embedder_for_mcp = shared_embedder.clone();
    let reranker_for_mcp = shared_reranker.clone();
    let rr_failed_for_mcp = shared_reranker_failed_at.clone();
    let hnsw_for_mcp = shared_hnsw.clone();
    let mcp_service = StreamableHttpService::new(
        move || {
            Ok(GrasshopperMcp::with_embedder(
                db.clone(),
                embedder_for_mcp.clone(),
                reranker_for_mcp.clone(),
                rr_failed_for_mcp.clone(),
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
