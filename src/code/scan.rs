use anyhow::Result;
use ignore::WalkBuilder;
use sha2::{Digest, Sha256};
use std::path::Path;

/// A source file discovered during scanning.
pub struct ScannedFile {
    pub rel_path: String,
    pub language: String,
    pub content: String,
    pub hash: String,
}

/// Result of scanning a directory.
pub struct ScanResult {
    pub files: Vec<ScannedFile>,
    pub errors: Vec<String>,
}

/// Detect language hint from file path.
/// Returns a language name for known extensions/filenames, the extension itself
/// for unknown extensions, or "unknown" for files with no extension.
/// This is NEVER used as a gate — every text file gets indexed.
pub fn detect_language_hint(path: &Path) -> String {
    let filename = path.file_name().and_then(|n| n.to_str()).unwrap_or("");

    // Try filename match first (Dockerfile, Makefile, etc.)
    if let Some(lang) = super::lang_hint_from_filename(filename) {
        return lang.to_owned();
    }

    // Try extension match (case-insensitive)
    if let Some(ext) = path.extension().and_then(|e| e.to_str()) {
        let ext_lower = ext.to_lowercase();
        if let Some(lang) = super::lang_hint_from_ext(&ext_lower) {
            return lang.to_owned();
        }
        // Unknown extension — use it as the hint
        return ext_lower;
    }

    "unknown".to_owned()
}

/// Check if file content is likely text (not binary).
/// Reads up to 512 bytes and rejects if any null bytes are found.
fn is_likely_text(content: &[u8]) -> bool {
    let check_len = content.len().min(512);
    !content[..check_len].contains(&0)
}

/// Scan a directory for source files, respecting .gitignore.
/// Returns ALL text files with content and SHA-256 hashes.
pub fn scan_directory(root: &Path) -> Result<ScanResult> {
    let root = root.canonicalize()?;
    let mut files = Vec::new();
    let mut errors = Vec::new();

    let walker = WalkBuilder::new(&root)
        .hidden(true) // skip dotfiles
        .git_ignore(true)
        .git_global(true)
        .git_exclude(true)
        .build();

    for entry in walker {
        let entry = match entry {
            Ok(e) => e,
            Err(e) => {
                errors.push(format!("walk error: {e}"));
                continue;
            }
        };

        // Skip directories
        if !entry.file_type().is_some_and(|ft| ft.is_file()) {
            continue;
        }

        let path = entry.path();

        // Skip files larger than 1MB
        let metadata = match path.metadata() {
            Ok(m) => m,
            Err(e) => {
                errors.push(format!("{}: {e}", path.display()));
                continue;
            }
        };
        if metadata.len() > 1_000_000 {
            continue;
        }

        // Read raw bytes for binary detection
        let raw = match std::fs::read(path) {
            Ok(r) => r,
            Err(e) => {
                errors.push(format!("{}: {e}", path.display()));
                continue;
            }
        };

        // Skip binary files
        if !is_likely_text(&raw) {
            continue;
        }

        // Convert to UTF-8 (skip files that aren't valid UTF-8, but report it)
        let content = match String::from_utf8(raw) {
            Ok(s) => s,
            Err(_) => {
                errors.push(format!("{}: not valid UTF-8, skipped", path.display()));
                continue;
            }
        };

        let hash = {
            let mut hasher = Sha256::new();
            hasher.update(content.as_bytes());
            format!("{:x}", hasher.finalize())
        };

        let language = detect_language_hint(path);

        let rel_path = path
            .strip_prefix(&root)
            .unwrap_or(path)
            .to_string_lossy()
            .into_owned();

        files.push(ScannedFile {
            rel_path,
            language,
            content,
            hash,
        });
    }

    Ok(ScanResult { files, errors })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn detect_known_extensions() {
        assert_eq!(detect_language_hint(Path::new("main.rs")), "rust");
        assert_eq!(detect_language_hint(Path::new("app.tsx")), "tsx");
        assert_eq!(detect_language_hint(Path::new("index.js")), "javascript");
        assert_eq!(detect_language_hint(Path::new("lib.py")), "python");
    }

    #[test]
    fn detect_new_language_hints() {
        assert_eq!(detect_language_hint(Path::new("worker.ex")), "elixir");
        assert_eq!(detect_language_hint(Path::new("main.kt")), "kotlin");
        assert_eq!(detect_language_hint(Path::new("hello.cob")), "cobol");
        assert_eq!(detect_language_hint(Path::new("infra.tf")), "terraform");
    }

    #[test]
    fn detect_filenames_without_extensions() {
        assert_eq!(detect_language_hint(Path::new("Dockerfile")), "dockerfile");
        assert_eq!(detect_language_hint(Path::new("Makefile")), "make");
    }

    #[test]
    fn detect_unknown_extension_uses_ext_itself() {
        assert_eq!(detect_language_hint(Path::new("file.xyz")), "xyz");
        assert_eq!(detect_language_hint(Path::new("app.weird")), "weird");
    }

    #[test]
    fn detect_no_extension_returns_unknown() {
        // Files without extension and not in FILENAME_HINTS
        assert_eq!(detect_language_hint(Path::new("README")), "unknown");
    }

    #[test]
    fn binary_detection() {
        assert!(is_likely_text(b"hello world\n"));
        assert!(is_likely_text(b"fn main() {}"));
        assert!(!is_likely_text(b"\x89PNG\r\n\x1a\n\x00\x00"));
        assert!(!is_likely_text(b"ELF\x00\x01\x02"));
        assert!(is_likely_text(b"")); // empty is text
    }

    #[test]
    fn scan_directory_indexes_all_text_files() {
        let dir = tempfile::tempdir().unwrap();
        std::fs::write(dir.path().join("main.rs"), "fn main() {}").unwrap();
        std::fs::write(dir.path().join("lib.py"), "def foo(): pass").unwrap();
        std::fs::write(dir.path().join("readme.md"), "# Hello").unwrap();
        std::fs::write(dir.path().join("worker.ex"), "defmodule W do\nend").unwrap();

        let result = scan_directory(dir.path()).unwrap();
        // Should find ALL text files, not just the 13 tree-sitter languages
        assert_eq!(result.files.len(), 4, "should index all 4 text files");
        let langs: Vec<&str> = result.files.iter().map(|f| f.language.as_str()).collect();
        assert!(langs.contains(&"rust"));
        assert!(langs.contains(&"python"));
        assert!(langs.contains(&"markdown"));
        assert!(langs.contains(&"elixir"));
    }

    #[test]
    fn scan_skips_binary_files() {
        let dir = tempfile::tempdir().unwrap();
        std::fs::write(dir.path().join("code.rs"), "fn small() {}").unwrap();
        // Write a binary file with null bytes
        std::fs::write(
            dir.path().join("image.png"),
            b"\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR",
        )
        .unwrap();

        let result = scan_directory(dir.path()).unwrap();
        assert_eq!(result.files.len(), 1, "should skip binary file");
        assert_eq!(result.files[0].rel_path, "code.rs");
    }

    #[test]
    fn scan_skips_large_files() {
        let dir = tempfile::tempdir().unwrap();
        std::fs::write(dir.path().join("small.rs"), "fn small() {}").unwrap();
        let big_content = "x".repeat(1_100_000);
        std::fs::write(dir.path().join("big.rs"), &big_content).unwrap();

        let result = scan_directory(dir.path()).unwrap();
        assert_eq!(result.files.len(), 1, "should skip the >1MB file");
    }

    #[test]
    fn scan_respects_gitignore() {
        let dir = tempfile::tempdir().unwrap();
        std::process::Command::new("git")
            .args(["init"])
            .current_dir(dir.path())
            .output()
            .unwrap();
        std::fs::write(dir.path().join(".gitignore"), "ignored.rs\n").unwrap();
        std::fs::write(dir.path().join("kept.rs"), "fn kept() {}").unwrap();
        std::fs::write(dir.path().join("ignored.rs"), "fn ignored() {}").unwrap();

        let result = scan_directory(dir.path()).unwrap();
        let paths: Vec<&str> = result.files.iter().map(|f| f.rel_path.as_str()).collect();
        assert!(paths.contains(&"kept.rs"));
        assert!(!paths.contains(&"ignored.rs"));
    }

    #[test]
    fn scan_hash_determinism() {
        let dir = tempfile::tempdir().unwrap();
        std::fs::write(dir.path().join("test.rs"), "fn deterministic() { 42 }").unwrap();

        let result1 = scan_directory(dir.path()).unwrap();
        let result2 = scan_directory(dir.path()).unwrap();

        assert_eq!(result1.files[0].hash, result2.files[0].hash);
        assert!(!result1.files[0].hash.is_empty());
    }
}
