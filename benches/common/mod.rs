pub mod fixtures;

use std::collections::HashMap;
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use tracing::Subscriber;
use tracing::span::{Attributes, Id};
use tracing_subscriber::Registry;
use tracing_subscriber::layer::{Context, Layer};
use tracing_subscriber::prelude::*;

// ---------------------------------------------------------------------------
// TimingLayer — captures span durations into a shared HashMap
// ---------------------------------------------------------------------------

/// Shared storage for span timings collected during benchmarks.
#[derive(Clone, Default)]
pub struct SpanTimings(pub Arc<Mutex<HashMap<String, Vec<Duration>>>>);

impl SpanTimings {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn get(&self, name: &str) -> Vec<Duration> {
        self.0
            .lock()
            .unwrap()
            .get(name)
            .cloned()
            .unwrap_or_default()
    }

    pub fn clear(&self) {
        self.0.lock().unwrap().clear();
    }

    /// Install this as the global tracing subscriber.
    /// Returns the timings handle for reading results.
    pub fn install(self) -> Self {
        let layer = TimingLayer {
            timings: self.clone(),
            open_spans: Arc::new(Mutex::new(HashMap::new())),
        };
        let subscriber = Registry::default().with(layer);
        let _ = tracing::subscriber::set_global_default(subscriber);
        self
    }
}

struct TimingLayer {
    timings: SpanTimings,
    open_spans: Arc<Mutex<HashMap<u64, (String, Instant)>>>,
}

impl<S: Subscriber> Layer<S> for TimingLayer {
    fn on_new_span(&self, attrs: &Attributes<'_>, id: &Id, _ctx: Context<'_, S>) {
        let name = attrs.metadata().name().to_string();
        self.open_spans
            .lock()
            .unwrap()
            .insert(id.into_u64(), (name, Instant::now()));
    }

    fn on_close(&self, id: Id, _ctx: Context<'_, S>) {
        if let Some((name, start)) = self.open_spans.lock().unwrap().remove(&id.into_u64()) {
            let duration = start.elapsed();
            self.timings
                .0
                .lock()
                .unwrap()
                .entry(name)
                .or_default()
                .push(duration);
        }
    }
}

// ---------------------------------------------------------------------------
// VmRSS reader (Linux only)
// ---------------------------------------------------------------------------

/// Read current VmRSS from /proc/self/status in bytes.
/// Returns 0 on non-Linux platforms.
pub fn read_vmrss() -> u64 {
    #[cfg(target_os = "linux")]
    {
        if let Ok(status) = std::fs::read_to_string("/proc/self/status") {
            for line in status.lines() {
                if line.starts_with("VmRSS:") {
                    let parts: Vec<&str> = line.split_whitespace().collect();
                    if parts.len() >= 2 {
                        if let Ok(kb) = parts[1].parse::<u64>() {
                            return kb * 1024;
                        }
                    }
                }
            }
        }
        0
    }
    #[cfg(not(target_os = "linux"))]
    {
        0
    }
}

// ---------------------------------------------------------------------------
// Statistics
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, serde::Serialize)]
pub struct PercentileStats {
    pub mean_ms: f64,
    pub stddev_ms: f64,
    pub p50_ms: f64,
    pub p95_ms: f64,
    pub p99_ms: f64,
    pub min_ms: f64,
    pub max_ms: f64,
    pub count: usize,
}

impl PercentileStats {
    pub fn from_durations(durations: &[Duration]) -> Self {
        if durations.is_empty() {
            return Self {
                mean_ms: 0.0,
                stddev_ms: 0.0,
                p50_ms: 0.0,
                p95_ms: 0.0,
                p99_ms: 0.0,
                min_ms: 0.0,
                max_ms: 0.0,
                count: 0,
            };
        }

        let mut ms: Vec<f64> = durations.iter().map(|d| d.as_secs_f64() * 1000.0).collect();
        ms.sort_by(|a, b| a.partial_cmp(b).unwrap());

        let count = ms.len();
        let sum: f64 = ms.iter().sum();
        let mean = sum / count as f64;
        let variance: f64 = ms.iter().map(|x| (x - mean).powi(2)).sum::<f64>() / count as f64;

        Self {
            mean_ms: mean,
            stddev_ms: variance.sqrt(),
            p50_ms: percentile(&ms, 50.0),
            p95_ms: percentile(&ms, 95.0),
            p99_ms: percentile(&ms, 99.0),
            min_ms: ms[0],
            max_ms: ms[count - 1],
            count,
        }
    }
}

fn percentile(sorted: &[f64], pct: f64) -> f64 {
    if sorted.is_empty() {
        return 0.0;
    }
    let idx = (pct / 100.0 * (sorted.len() - 1) as f64).round() as usize;
    sorted[idx.min(sorted.len() - 1)]
}

// ---------------------------------------------------------------------------
// Benchmark report
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, serde::Serialize)]
pub struct BenchReport {
    pub name: String,
    pub timestamp: String,
    pub sections: Vec<ReportSection>,
}

#[derive(Debug, Clone, serde::Serialize)]
pub struct ReportSection {
    pub name: String,
    pub metrics: serde_json::Value,
}

impl BenchReport {
    pub fn new(name: &str) -> Self {
        Self {
            name: name.to_string(),
            timestamp: chrono::Utc::now().to_rfc3339(),
            sections: Vec::new(),
        }
    }

    pub fn add_section(&mut self, name: &str, metrics: serde_json::Value) {
        self.sections.push(ReportSection {
            name: name.to_string(),
            metrics,
        });
    }

    pub fn save(&self, path: &str) -> std::io::Result<()> {
        let dir = std::path::Path::new(path).parent().unwrap();
        std::fs::create_dir_all(dir)?;
        let json = serde_json::to_string_pretty(self).unwrap();
        std::fs::write(path, json)?;
        eprintln!("Report saved to {path}");
        Ok(())
    }
}

// ---------------------------------------------------------------------------
// IR Metrics
// ---------------------------------------------------------------------------

/// Precision@K: fraction of top-k results in the gold set.
pub fn precision_at_k(ranked: &[String], gold: &[String], k: usize) -> f64 {
    if gold.is_empty() || k == 0 {
        return 0.0;
    }
    let top_k: Vec<&String> = ranked.iter().take(k).collect();
    let hits = top_k.iter().filter(|r| gold.contains(r)).count();
    hits as f64 / k as f64
}

/// Mean Reciprocal Rank: 1/rank of first gold result.
pub fn mrr(ranked: &[String], gold: &[String]) -> f64 {
    for (i, item) in ranked.iter().enumerate() {
        if gold.contains(item) {
            return 1.0 / (i as f64 + 1.0);
        }
    }
    0.0
}

/// NDCG@K: Normalized Discounted Cumulative Gain.
pub fn ndcg_at_k(ranked: &[String], gold: &[String], k: usize) -> f64 {
    if gold.is_empty() || k == 0 {
        return 0.0;
    }

    let dcg: f64 = ranked
        .iter()
        .take(k)
        .enumerate()
        .map(|(i, item)| {
            let rel = if gold.contains(item) { 1.0 } else { 0.0 };
            rel / (i as f64 + 2.0).log2()
        })
        .sum();

    // Ideal DCG: all gold items at the top
    let ideal_k = k.min(gold.len());
    let idcg: f64 = (0..ideal_k).map(|i| 1.0 / (i as f64 + 2.0).log2()).sum();

    if idcg == 0.0 { 0.0 } else { dcg / idcg }
}
