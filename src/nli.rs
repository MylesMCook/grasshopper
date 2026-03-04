use anyhow::{Context, Result};
use ndarray::Array2;
use ort::session::Session;
use ort::value::Tensor;
use std::path::PathBuf;

/// NLI prediction labels (3-class: entailment, neutral, contradiction).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum NliLabel {
    Entailment,
    Neutral,
    Contradiction,
}

/// A detected contradiction between two texts.
#[derive(Debug, Clone, serde::Serialize)]
pub struct Contradiction {
    pub id_a: i64,
    pub id_b: i64,
    pub title_a: String,
    pub title_b: String,
    pub score: f32,
}

/// NLI cross-encoder using ort (ONNX Runtime) directly.
/// Model: cross-encoder/nli-MiniLM-L6-v2 (22M params, 3-class NLI).
pub struct NliModel {
    session: Session,
    tokenizer: tokenizers::Tokenizer,
}

impl NliModel {
    /// Load model from the default cache directory.
    /// Expects ONNX model at `~/.cache/grasshopper/models/nli-MiniLM-L6-v2/model.onnx`
    /// and tokenizer at `~/.cache/grasshopper/models/nli-MiniLM-L6-v2/tokenizer.json`.
    pub fn new() -> Result<Self> {
        let model_dir = default_model_dir();
        Self::from_dir(&model_dir)
    }

    /// Load model from a specific directory.
    pub fn from_dir(dir: &std::path::Path) -> Result<Self> {
        let model_path = dir.join("model.onnx");
        let tokenizer_path = dir.join("tokenizer.json");

        anyhow::ensure!(
            model_path.exists(),
            "NLI model not found at {}. Export with: optimum-cli export onnx --model cross-encoder/nli-MiniLM-L6-v2 {}",
            model_path.display(),
            dir.display()
        );
        anyhow::ensure!(
            tokenizer_path.exists(),
            "NLI tokenizer not found at {}",
            tokenizer_path.display()
        );

        let session = Session::builder()?
            .with_optimization_level(ort::session::builder::GraphOptimizationLevel::Level3)?
            .with_intra_threads(1)?
            .commit_from_file(&model_path)
            .context("Failed to load NLI ONNX model")?;

        let tokenizer = tokenizers::Tokenizer::from_file(&tokenizer_path)
            .map_err(|e| anyhow::anyhow!("Failed to load tokenizer: {e}"))?;

        Ok(Self { session, tokenizer })
    }

    /// Predict NLI label for a premise-hypothesis pair.
    pub fn predict(&mut self, premise: &str, hypothesis: &str) -> Result<(NliLabel, f32)> {
        let encoding = self
            .tokenizer
            .encode((premise, hypothesis), true)
            .map_err(|e| anyhow::anyhow!("Tokenization failed: {e}"))?;

        let ids = encoding.get_ids();
        let mask = encoding.get_attention_mask();
        let seq_len = ids.len();

        let mut input_ids = Array2::<i64>::zeros((1, seq_len));
        let mut attention_mask = Array2::<i64>::zeros((1, seq_len));

        for (j, (&id, &m)) in ids.iter().zip(mask.iter()).enumerate() {
            input_ids[[0, j]] = id as i64;
            attention_mask[[0, j]] = m as i64;
        }

        let input_ids_tensor = Tensor::from_array(input_ids)?;
        let attention_mask_tensor = Tensor::from_array(attention_mask)?;

        let outputs = self.session.run(ort::inputs! {
            "input_ids" => input_ids_tensor,
            "attention_mask" => attention_mask_tensor,
        })?;

        // Output shape: [1, 3] — logits for 3 classes
        // nli-MiniLM-L6-v2 label order: 0=contradiction, 1=entailment, 2=neutral
        let (shape, logits_data) = outputs[0]
            .try_extract_tensor::<f32>()
            .context("Failed to extract NLI logits")?;

        let num_classes = shape[1] as usize;
        if num_classes < 3 {
            anyhow::bail!("Expected 3 logits, got {num_classes}");
        }

        let logits = &logits_data[..3];

        // Softmax
        let max_logit = logits.iter().cloned().fold(f32::NEG_INFINITY, f32::max);
        let exp: Vec<f32> = logits.iter().map(|&x| (x - max_logit).exp()).collect();
        let sum: f32 = exp.iter().sum();
        let probs: Vec<f32> = exp.iter().map(|&e| e / sum).collect();

        let (max_idx, max_prob) = probs
            .iter()
            .enumerate()
            .max_by(|(_, a), (_, b)| a.partial_cmp(b).unwrap())
            .map(|(i, &p)| (i, p))
            .unwrap();

        let label = match max_idx {
            0 => NliLabel::Contradiction,
            1 => NliLabel::Entailment,
            _ => NliLabel::Neutral,
        };

        Ok((label, max_prob))
    }

    /// Find contradictions among a set of text pairs.
    /// Runs pairwise NLI, returns detected contradictions (score > 0.7).
    pub fn find_contradictions(
        &mut self,
        entries: &[(i64, String, String)], // (id, title, content)
    ) -> Vec<Contradiction> {
        let mut contradictions = Vec::new();
        let n = entries.len();

        for i in 0..n {
            for j in (i + 1)..n {
                let premise = build_passage(&entries[i].1, &entries[i].2);
                let hypothesis = build_passage(&entries[j].1, &entries[j].2);

                // Truncate to avoid slow inference on long texts
                let premise = truncate_str(&premise, 256);
                let hypothesis = truncate_str(&hypothesis, 256);

                match self.predict(&premise, &hypothesis) {
                    Ok((NliLabel::Contradiction, score)) if score > 0.7 => {
                        contradictions.push(Contradiction {
                            id_a: entries[i].0,
                            id_b: entries[j].0,
                            title_a: entries[i].1.clone(),
                            title_b: entries[j].1.clone(),
                            score,
                        });
                    }
                    Ok(_) => {}
                    Err(e) => {
                        tracing::debug!("NLI failed for pair ({}, {}): {e}", entries[i].0, entries[j].0);
                    }
                }
            }
        }

        contradictions
    }
}

fn build_passage(title: &str, content: &str) -> String {
    if title.is_empty() {
        content.to_string()
    } else {
        format!("{title}: {content}")
    }
}

fn default_model_dir() -> PathBuf {
    let home = std::env::var("HOME").unwrap_or_else(|_| ".".to_string());
    PathBuf::from(home)
        .join(".cache")
        .join("grasshopper")
        .join("models")
        .join("nli-MiniLM-L6-v2")
}

fn truncate_str(s: &str, max_chars: usize) -> String {
    if s.chars().count() <= max_chars {
        s.to_string()
    } else {
        s.chars().take(max_chars).collect()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn model_available() -> bool {
        default_model_dir().join("model.onnx").exists()
    }

    #[test]
    #[ignore] // Requires NLI model to be downloaded
    fn test_contradiction_detection() {
        if !model_available() { return; }
        let mut model = NliModel::new().unwrap();
        let (label, score) = model.predict(
            "The server runs on port 8080",
            "The server runs on port 3000",
        ).unwrap();
        assert_eq!(label, NliLabel::Contradiction, "score={score}");
    }

    #[test]
    #[ignore]
    fn test_entailment_detection() {
        if !model_available() { return; }
        let mut model = NliModel::new().unwrap();
        let (label, _) = model.predict(
            "All dogs are animals",
            "A dog is an animal",
        ).unwrap();
        assert_eq!(label, NliLabel::Entailment);
    }

    #[test]
    #[ignore]
    fn test_neutral_detection() {
        if !model_available() { return; }
        let mut model = NliModel::new().unwrap();
        let (label, _) = model.predict(
            "The backup runs at 3am nightly",
            "SQLite uses WAL mode",
        ).unwrap();
        assert_eq!(label, NliLabel::Neutral);
    }

    #[test]
    #[ignore]
    fn test_find_contradictions_pairwise() {
        if !model_available() { return; }
        let mut model = NliModel::new().unwrap();
        let entries = vec![
            (1, "Server port".into(), "The server runs on port 8080".into()),
            (2, "Server port".into(), "The server runs on port 3000".into()),
            (3, "Backup schedule".into(), "Backups run at 3am".into()),
        ];
        let contradictions = model.find_contradictions(&entries);
        assert!(!contradictions.is_empty(), "should detect port contradiction");
        assert_eq!(contradictions[0].id_a, 1);
        assert_eq!(contradictions[0].id_b, 2);
    }
}
