use anyhow::Result;
use fastembed::{RerankInitOptions, RerankerModel, TextRerank};
use std::path::PathBuf;

pub struct Reranker {
    model: TextRerank,
}

impl Reranker {
    /// Initialize with Jina Reranker V1 Turbo (distilled cross-encoder).
    /// ~3x faster and ~3x less memory than BGE reranker base.
    pub fn new() -> Result<Self> {
        // fastembed defaults to a relative ".fastembed_cache" path. Use an
        // absolute home cache path so CLI/systemd/bench runs share one cache.
        let cache_dir = dirs::home_dir()
            .unwrap_or_else(|| PathBuf::from("."))
            .join(".fastembed_cache");
        let model = TextRerank::try_new(
            RerankInitOptions::new(RerankerModel::JINARerankerV1TurboEn)
                .with_cache_dir(cache_dir)
                .with_show_download_progress(true),
        )?;
        Ok(Self { model })
    }

    /// Score (query, passage) pairs via cross-encoder.
    /// Returns Vec<(original_index, relevance_score)> sorted by score descending,
    /// truncated to top_k.
    pub fn rerank(
        &mut self,
        query: &str,
        passages: &[String],
        top_k: usize,
    ) -> Result<Vec<(usize, f32)>> {
        if passages.is_empty() {
            return Ok(vec![]);
        }
        let str_passages: Vec<&str> = passages.iter().map(String::as_str).collect();
        let results = self
            .model
            .rerank(query, str_passages.as_slice(), false, None)?;
        let mut scored: Vec<(usize, f32)> = results.iter().map(|r| (r.index, r.score)).collect();
        scored.sort_by(|a, b| b.1.partial_cmp(&a.1).unwrap_or(std::cmp::Ordering::Equal));
        scored.truncate(top_k);
        Ok(scored)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Mutex;

    // Serialize all rerank tests — model download has a file lock that fails
    // under concurrent access, and we only need one instance anyway.
    static RERANKER: std::sync::OnceLock<Mutex<Reranker>> = std::sync::OnceLock::new();

    fn get_reranker() -> &'static Mutex<Reranker> {
        RERANKER.get_or_init(|| Mutex::new(Reranker::new().expect("model should load")))
    }

    #[test]
    fn test_rerank_empty() {
        let mut reranker = get_reranker().lock().unwrap();
        let results = reranker.rerank("test query", &[], 5).unwrap();
        assert!(results.is_empty());
    }

    #[test]
    fn test_rerank_ordering() {
        let mut reranker = get_reranker().lock().unwrap();
        let query = "What is Rust programming language?";
        let passages = vec![
            "Rust is a systems programming language focused on safety and performance.".into(),
            "Python is great for data science and machine learning.".into(),
            "Rust's borrow checker prevents memory safety bugs at compile time.".into(),
        ];
        let results = reranker.rerank(query, &passages, 2).unwrap();
        assert_eq!(results.len(), 2);
        // Top result should be a Rust-related passage (index 0 or 2)
        assert!(results[0].0 == 0 || results[0].0 == 2);
    }

    #[test]
    fn test_rerank_top_k_limits() {
        let mut reranker = get_reranker().lock().unwrap();
        let passages: Vec<String> = (0..10)
            .map(|i| format!("Document number {i} about various topics"))
            .collect();
        let results = reranker.rerank("topic", &passages, 3).unwrap();
        assert_eq!(results.len(), 3);
    }

    #[test]
    fn test_rerank_scores_descending() {
        let mut reranker = get_reranker().lock().unwrap();
        let passages = vec![
            "Completely unrelated content about cooking recipes.".into(),
            "Rust programming language overview and features.".into(),
            "More about Rust's ownership model and borrowing.".into(),
            "Weather forecast for tomorrow.".into(),
        ];
        let results = reranker.rerank("Rust programming", &passages, 4).unwrap();
        for w in results.windows(2) {
            assert!(w[0].1 >= w[1].1, "scores should be descending");
        }
    }
}
