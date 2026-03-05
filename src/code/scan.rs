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

/// Detect language from file extension.
/// Uses the canonical EXT_MAP from the parent module.
pub fn detect_language(path: &Path) -> Option<&'static str> {
    let ext = path.extension()?.to_str()?;
    super::EXT_MAP.iter().find(|(e, _)| *e == ext).map(|(_, lang)| *lang)
}

/// Scan a directory for source files, respecting .gitignore.
/// Returns files with content and SHA-256 hashes.
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

        // Only process files with known language extensions
        let language = match detect_language(path) {
            Some(lang) => lang,
            None => continue,
        };

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

        let content = match std::fs::read_to_string(path) {
            Ok(c) => c,
            Err(e) => {
                errors.push(format!("{}: {e}", path.display()));
                continue;
            }
        };

        let hash = {
            let mut hasher = Sha256::new();
            hasher.update(content.as_bytes());
            format!("{:x}", hasher.finalize())
        };

        let rel_path = path
            .strip_prefix(&root)
            .unwrap_or(path)
            .to_string_lossy()
            .into_owned();

        files.push(ScannedFile {
            rel_path,
            language: language.to_owned(),
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
    fn detect_language_from_known_extensions() {
        assert_eq!(detect_language(Path::new("main.rs")), Some("rust"));
        assert_eq!(detect_language(Path::new("app.tsx")), Some("tsx"));
        assert_eq!(detect_language(Path::new("index.js")), Some("javascript"));
        assert_eq!(detect_language(Path::new("lib.py")), Some("python"));
    }

    #[test]
    fn detect_language_returns_none_for_unknown() {
        assert_eq!(detect_language(Path::new("readme.md")), None);
        assert_eq!(detect_language(Path::new("Dockerfile")), None);
        assert_eq!(detect_language(Path::new(".env")), None);
    }

    #[test]
    fn scan_directory_basic() {
        let dir = tempfile::tempdir().unwrap();
        std::fs::write(dir.path().join("main.rs"), "fn main() {}").unwrap();
        std::fs::write(dir.path().join("lib.py"), "def foo(): pass").unwrap();
        std::fs::write(dir.path().join("readme.md"), "# Hello").unwrap();

        let result = scan_directory(dir.path()).unwrap();
        // Should find .rs and .py but not .md
        assert_eq!(result.files.len(), 2);
        let langs: Vec<&str> = result.files.iter().map(|f| f.language.as_str()).collect();
        assert!(langs.contains(&"rust"));
        assert!(langs.contains(&"python"));
    }

    #[test]
    fn scan_skips_large_files() {
        let dir = tempfile::tempdir().unwrap();
        std::fs::write(dir.path().join("small.rs"), "fn small() {}").unwrap();
        // Create a file >1MB
        let big_content = "x".repeat(1_100_000);
        std::fs::write(dir.path().join("big.rs"), &big_content).unwrap();

        let result = scan_directory(dir.path()).unwrap();
        assert_eq!(result.files.len(), 1, "should skip the >1MB file");
        assert_eq!(result.files[0].rel_path, "small.rs");
    }

    #[test]
    fn scan_respects_gitignore() {
        let dir = tempfile::tempdir().unwrap();
        // Initialize a git repo so .gitignore is respected
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
        assert!(paths.contains(&"kept.rs"), "kept.rs should be found");
        assert!(!paths.contains(&"ignored.rs"), "ignored.rs should be excluded by .gitignore");
    }

    #[test]
    fn scan_hash_determinism() {
        let dir = tempfile::tempdir().unwrap();
        let content = "fn deterministic() { 42 }";
        std::fs::write(dir.path().join("test.rs"), content).unwrap();

        let result1 = scan_directory(dir.path()).unwrap();
        let result2 = scan_directory(dir.path()).unwrap();

        assert_eq!(result1.files[0].hash, result2.files[0].hash,
            "same content should produce same hash");
        assert!(!result1.files[0].hash.is_empty(), "hash should not be empty");
    }
}
