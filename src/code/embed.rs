use anyhow::{Context, Result};
use ndarray::Array2;
use ort::session::Session;
use ort::value::Tensor;
use std::io::{IsTerminal, Read};
use std::path::{Path, PathBuf};
use tokenizers::Tokenizer;

pub const MODEL_NAME: &str = "jina-embeddings-v2-base-code";
const EMBEDDING_DIM: usize = 768;
const HF_REPO: &str = "jinaai/jina-embeddings-v2-base-code";

/// Local embedding model using ONNX Runtime.
pub struct Embedder {
    session: Session,
    tokenizer: Tokenizer,
    /// Whether the ONNX model expects token_type_ids input.
    has_token_type_ids: bool,
}

impl Embedder {
    /// Load embedder from a model directory.
    /// Downloads model files on first run if not present.
    pub fn new(cache_dir: &Path) -> Result<Self> {
        let model_dir = cache_dir.join("models").join(MODEL_NAME);
        let model_path = model_dir.join("model.onnx");
        let tokenizer_path = model_dir.join("tokenizer.json");

        // Download if missing
        if !model_path.exists() || !tokenizer_path.exists() {
            download_model(&model_dir)?;
        }

        let num_cores = std::thread::available_parallelism()
            .map(|n| n.get())
            .unwrap_or(4);

        let session = Session::builder()
            .context("creating ONNX session builder")?
            .with_intra_threads(num_cores)
            .context("setting intra-op threads")?
            .with_inter_threads(1)
            .context("setting inter-op threads")?
            .commit_from_file(&model_path)
            .with_context(|| format!("loading ONNX model: {}", model_path.display()))?;

        let tokenizer = Tokenizer::from_file(&tokenizer_path)
            .map_err(|e| anyhow::anyhow!("loading tokenizer: {e}"))?;

        // Check if model expects token_type_ids (BERT does, ALiBi-based models may not)
        let has_token_type_ids = session
            .inputs()
            .iter()
            .any(|input| input.name() == "token_type_ids");

        tracing::info!(
            "embedder loaded: {MODEL_NAME} (dim={EMBEDDING_DIM}, token_type_ids={has_token_type_ids})"
        );
        Ok(Self {
            session,
            tokenizer,
            has_token_type_ids,
        })
    }

    /// Compute embeddings for a batch of texts.
    /// Returns one EMBEDDING_DIM vector per input text.
    pub fn embed_batch(&mut self, texts: &[String]) -> Result<Vec<Vec<f32>>> {
        if texts.is_empty() {
            return Ok(Vec::new());
        }

        let str_refs: Vec<&str> = texts.iter().map(String::as_str).collect();

        // Tokenize
        let encodings = self
            .tokenizer
            .encode_batch(str_refs, true)
            .map_err(|e| anyhow::anyhow!("tokenization failed: {e}"))?;

        let batch_size = encodings.len();
        let max_len = encodings
            .iter()
            .map(|e| e.get_ids().len())
            .max()
            .unwrap_or(0);

        if max_len == 0 {
            return Ok(vec![vec![0.0; EMBEDDING_DIM]; batch_size]);
        }

        // Build padded input tensors [batch_size, max_len]
        let mut input_ids = Array2::<i64>::zeros((batch_size, max_len));
        let mut attention_mask = Array2::<i64>::zeros((batch_size, max_len));

        for (i, enc) in encodings.iter().enumerate() {
            for (j, &id) in enc.get_ids().iter().enumerate() {
                input_ids[[i, j]] = id as i64;
            }
            for (j, &mask) in enc.get_attention_mask().iter().enumerate() {
                attention_mask[[i, j]] = mask as i64;
            }
        }

        // Run ONNX inference — token_type_ids only included if model expects it
        let input_ids_tensor = Tensor::from_array(input_ids)?;
        let attention_mask_tensor = Tensor::from_array(attention_mask)?;

        let outputs = if self.has_token_type_ids {
            let mut token_type_ids = Array2::<i64>::zeros((batch_size, max_len));
            for (i, enc) in encodings.iter().enumerate() {
                for (j, &tid) in enc.get_type_ids().iter().enumerate() {
                    token_type_ids[[i, j]] = tid as i64;
                }
            }
            let token_type_ids_tensor = Tensor::from_array(token_type_ids)?;
            self.session.run(ort::inputs! {
                "input_ids" => input_ids_tensor,
                "attention_mask" => attention_mask_tensor,
                "token_type_ids" => token_type_ids_tensor,
            })?
        } else {
            self.session.run(ort::inputs! {
                "input_ids" => input_ids_tensor,
                "attention_mask" => attention_mask_tensor,
            })?
        };

        // Extract output tensor: shape [batch_size, seq_len, dim] as flat data
        let (shape, output_data) = outputs[0]
            .try_extract_tensor::<f32>()
            .context("extracting output tensor")?;

        let out_seq_len = shape[1] as usize;

        // Mean pooling + L2 normalization per sample
        let mut embeddings = Vec::with_capacity(batch_size);
        for (i, enc) in encodings.iter().enumerate() {
            let mut pooled = vec![0.0f32; EMBEDDING_DIM];
            let mask = enc.get_attention_mask();
            let mut token_count: usize = 0;

            for (j, &m) in mask.iter().enumerate() {
                if m == 1 {
                    let offset = (i * out_seq_len + j) * EMBEDDING_DIM;
                    for d in 0..EMBEDDING_DIM {
                        pooled[d] += output_data[offset + d];
                    }
                    token_count += 1;
                }
            }

            if token_count > 0 {
                let count = token_count as f32;
                for val in &mut pooled {
                    *val /= count;
                }
            }

            // L2 normalize
            let norm: f32 = pooled.iter().map(|x| x * x).sum::<f32>().sqrt();
            if norm > 0.0 {
                for val in &mut pooled {
                    *val /= norm;
                }
            }

            embeddings.push(pooled);
        }

        Ok(embeddings)
    }
}

/// Build the text representation for embedding a code chunk.
///
/// Includes a language tag prefix for model context (e.g., `[rust]`)
/// and up to 1500 bytes of snippet (prepares for larger-context models).
pub fn build_embed_text(
    file_path: &str,
    language: &str,
    kind: &str,
    name: &str,
    signature: &str,
    snippet: &str,
) -> String {
    let estimated = file_path.len()
        + language.len()
        + kind.len()
        + name.len()
        + signature.len()
        + 1500.min(snippet.len())
        + 20;
    let mut text = String::with_capacity(estimated);

    // Language tag helps the model distinguish code conventions
    if !language.is_empty() {
        text.push('[');
        text.push_str(language);
        text.push_str("] ");
    }

    text.push_str(file_path);

    if !kind.is_empty() && !name.is_empty() {
        text.push_str(": ");
        text.push_str(kind);
        text.push(' ');
        text.push_str(name);
    } else if !name.is_empty() {
        text.push_str(": ");
        text.push_str(name);
    }

    if !signature.is_empty() {
        text.push('\n');
        text.push_str(signature);
    }

    if !snippet.is_empty() {
        text.push('\n');
        let preview = truncate_str(snippet, 1500);
        text.push_str(preview);
    }

    text
}

/// Truncate a string to at most `max_bytes` bytes at a char boundary.
fn truncate_str(s: &str, max_bytes: usize) -> &str {
    if s.len() <= max_bytes {
        return s;
    }
    // Find the last char boundary at or before max_bytes
    let mut end = max_bytes;
    while end > 0 && !s.is_char_boundary(end) {
        end -= 1;
    }
    &s[..end]
}

/// Download model files from HuggingFace Hub with progress indication.
fn download_model(model_dir: &Path) -> Result<()> {
    std::fs::create_dir_all(model_dir)
        .with_context(|| format!("creating model dir: {}", model_dir.display()))?;

    eprintln!(
        "First run — downloading {} to {}",
        MODEL_NAME,
        model_dir.display()
    );

    let files = &[
        ("onnx/model_quantized.onnx", "model.onnx"),
        ("tokenizer.json", "tokenizer.json"),
    ];

    for &(remote_path, local_name) in files {
        let url = format!("https://huggingface.co/{HF_REPO}/resolve/main/{remote_path}");
        let dest = model_dir.join(local_name);

        if dest.exists() {
            continue;
        }

        let resp = ureq::get(&url)
            .call()
            .with_context(|| format!("downloading {local_name} — check your network connection"))?;

        let content_length = resp
            .headers()
            .get("content-length")
            .and_then(|v| v.to_str().ok())
            .and_then(|s| s.parse::<u64>().ok());

        // Write to temp file then atomic rename to prevent corrupt partial downloads
        let tmp_dest = dest.with_extension("tmp");
        let mut file = std::fs::File::create(&tmp_dest).with_context(|| {
            format!(
                "creating {}: permission denied or disk full?",
                tmp_dest.display()
            )
        })?;

        let mut reader = resp.into_body().into_reader();
        let mut buf = [0u8; 65536];
        let mut downloaded: u64 = 0;
        let use_progress = std::io::stderr().is_terminal();

        let result = (|| -> Result<()> {
            loop {
                let n = reader
                    .read(&mut buf)
                    .context("reading model data — download interrupted?")?;
                if n == 0 {
                    break;
                }
                std::io::Write::write_all(&mut file, &buf[..n])
                    .with_context(|| format!("writing to {}: disk full?", tmp_dest.display()))?;
                downloaded += n as u64;

                if use_progress {
                    if let Some(total) = content_length.filter(|&t| t > 0) {
                        let pct = (downloaded * 100 / total).min(100);
                        let bar_width = 30;
                        let filled = (pct as usize * bar_width / 100).min(bar_width);
                        let arrow = if filled < bar_width { ">" } else { "" };
                        let spaces = bar_width - filled - if filled < bar_width { 1 } else { 0 };
                        let mb_done = downloaded / (1024 * 1024);
                        let mb_total = total / (1024 * 1024);
                        eprint!(
                            "\r  {} [{}{}{}] {}% ({}/{} MB)",
                            local_name,
                            "=".repeat(filled),
                            arrow,
                            " ".repeat(spaces),
                            pct,
                            mb_done,
                            mb_total,
                        );
                    } else {
                        let mb = downloaded / (1024 * 1024);
                        eprint!("\r  {} ... {} MB downloaded", local_name, mb);
                    }
                }
            }
            Ok(())
        })();

        if use_progress {
            eprintln!(); // newline after progress bar
        }

        // Clean up temp file on error
        if let Err(e) = result {
            let _ = std::fs::remove_file(&tmp_dest);
            return Err(e);
        }

        // Verify download completeness when Content-Length is known
        if let Some(total) = content_length.filter(|&t| t > 0 && downloaded != t) {
            let _ = std::fs::remove_file(&tmp_dest);
            anyhow::bail!(
                "incomplete download of {local_name}: got {} bytes, expected {total}",
                downloaded,
            );
        }

        drop(file);
        if let Err(e) = std::fs::rename(&tmp_dest, &dest) {
            let _ = std::fs::remove_file(&tmp_dest);
            return Err(e)
                .with_context(|| format!("renaming {} -> {}", tmp_dest.display(), dest.display()));
        }

        let size_mb = downloaded / (1024 * 1024);
        eprintln!("  saved {} ({} MB)", dest.display(), size_mb);
    }

    Ok(())
}

/// Return the default cache directory for embedding models.
pub fn default_cache_dir() -> PathBuf {
    dirs::cache_dir()
        .unwrap_or_else(|| PathBuf::from("."))
        .join("grasshopper")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn build_embed_text_with_all_fields() {
        let text = build_embed_text(
            "src/main.rs",
            "rust",
            "function",
            "main",
            "fn main()",
            "fn main() {}",
        );
        assert!(text.starts_with("[rust] src/main.rs: function main"));
        assert!(text.contains("fn main()"));
    }

    #[test]
    fn build_embed_text_truncates_long_snippets() {
        let long_snippet = "x".repeat(3000);
        let text = build_embed_text("test.rs", "rust", "function", "f", "fn f()", &long_snippet);
        // Should contain at most 1500 chars of snippet + header
        assert!(text.len() < 1600);
    }

    #[test]
    fn build_embed_text_handles_empty_fields() {
        let text = build_embed_text("test.rs", "", "", "", "", "");
        assert_eq!(text, "test.rs");
    }

    #[test]
    fn build_embed_text_includes_language_tag() {
        let text = build_embed_text("lib.py", "python", "function", "greet", "def greet()", "");
        assert!(text.starts_with("[python] lib.py"));
    }

    #[test]
    fn truncate_str_multibyte_boundary() {
        // 3-byte CJK chars: truncating mid-char must not panic
        let s = "Hello\u{4e16}\u{754c}World"; // "Hello世界World"
        let result = truncate_str(s, 7); // lands inside first CJK char (bytes 5,6,7)
        assert!(result.len() <= 7);
        assert!(result.is_char_boundary(result.len()));
        assert_eq!(result, "Hello"); // backed up to byte 5

        // 4-byte emoji
        let emoji = "ab\u{1F600}cd"; // "ab😀cd"
        let result = truncate_str(emoji, 4); // lands inside the emoji (bytes 2..6)
        assert!(result.len() <= 4);
        assert_eq!(result, "ab"); // backed up to byte 2
    }

    #[test]
    fn truncate_str_exact_boundary() {
        let s = "abc";
        assert_eq!(truncate_str(s, 3), "abc");
        assert_eq!(truncate_str(s, 2), "ab");
        assert_eq!(truncate_str(s, 100), "abc"); // no-op when max > len
    }

    #[test]
    fn truncate_str_empty() {
        assert_eq!(truncate_str("", 0), "");
        assert_eq!(truncate_str("", 10), "");
    }
}
