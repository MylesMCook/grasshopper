use regex::Regex;
use std::sync::LazyLock;

const MAX_CHUNK_LINES: usize = 150;
const MAX_CHUNK_CHARS: usize = 1500;
const MIN_MERGE_LINES: usize = 5;

/// Count non-whitespace characters in a string.
fn non_ws_chars(s: &str) -> usize {
    s.chars().filter(|c| !c.is_whitespace()).count()
}

/// A parsed code chunk ready for storage.
pub struct ParsedChunk {
    pub chunk_key: String,
    pub language: String,
    pub kind: String,
    pub name: String,
    pub signature: String,
    pub snippet: String,
    pub start_line: usize,
    pub end_line: usize,
}

/// Regex matching lines that typically start a new declaration/definition.
/// Language-agnostic: covers keywords from many languages without detecting which one.
static DECL_RE: LazyLock<Regex> = LazyLock::new(|| {
    Regex::new(
        r"(?m)^[ \t]*(pub\s+|export\s+|public\s+|private\s+|protected\s+|static\s+|async\s+|abstract\s+|virtual\s+|override\s+|inline\s+|extern\s+)*(fn |def |func |function |class |module |struct |enum |trait |impl |interface |type |package |import |const |let |var |sub |procedure |program |defmodule |defp? |describe |it |test |macro )"
    ).unwrap()
});

/// Regex to extract a name-like identifier after a declaration keyword.
/// Covers function/class-style (`fn foo`) and binding-style (`const FOO =`).
static NAME_RE: LazyLock<Regex> = LazyLock::new(|| {
    Regex::new(
        r"(?:fn |def |func |function |class |module |struct |enum |trait |impl |interface |type |sub |procedure |program |defmodule |defp |macro |const |let |var |package |import )([A-Za-z_][A-Za-z0-9_.]*)"
    ).unwrap()
});

/// Check if a line is a structural boundary (declaration or dedent to column 0).
fn is_boundary(line: &str) -> bool {
    if line.trim().is_empty() {
        return false;
    }
    DECL_RE.is_match(line)
}

/// Parse any source file into semantic chunks using structural heuristics.
/// Works identically for all languages — no tree-sitter, no language-specific logic.
///
/// Cannot fail: empty content returns empty vec.
pub fn chunk_content(file_path: &str, content: &str, language_hint: &str) -> Vec<ParsedChunk> {
    if content.trim().is_empty() {
        return Vec::new();
    }

    let lines: Vec<&str> = content.lines().collect();
    let raw_blocks = split_into_blocks(&lines);
    let merged = merge_small_blocks(raw_blocks, &lines);
    let sized = split_oversized(merged, &lines);

    sized
        .into_iter()
        .map(|(start, end)| {
            let snippet: String = lines[start..=end].join("\n");
            let name = extract_heuristic_name(&snippet);
            let signature = lines[start].trim_end().to_owned();

            ParsedChunk {
                chunk_key: format!("{file_path}:block:{name}:{}:{}", start + 1, end + 1),
                language: language_hint.to_owned(),
                kind: "block".to_owned(),
                name,
                signature,
                snippet,
                start_line: start + 1, // 1-indexed
                end_line: end + 1,
            }
        })
        .collect()
}

/// Split lines into blocks at blank-line boundaries and structural boundaries.
/// Returns (start_idx, end_idx) pairs (0-indexed, inclusive).
fn split_into_blocks(lines: &[&str]) -> Vec<(usize, usize)> {
    let mut blocks = Vec::new();
    let mut block_start: Option<usize> = None;

    for (i, line) in lines.iter().enumerate() {
        if line.trim().is_empty() {
            // End current block at blank line
            if let Some(start) = block_start.take() {
                blocks.push((start, i.saturating_sub(1).max(start)));
            }
            continue;
        }

        if block_start.is_none() {
            block_start = Some(i);
        } else if is_boundary(line) && i > block_start.unwrap() {
            // Structural boundary mid-block: end previous, start new
            let start = block_start.unwrap();
            blocks.push((start, i - 1));
            block_start = Some(i);
        }
    }

    // Close final block
    if let Some(start) = block_start {
        let end = lines.len() - 1;
        blocks.push((start, end));
    }

    blocks
}

/// Merge adjacent small blocks (< MIN_MERGE_LINES) into their neighbor.
/// Blocks that start with a declaration keyword are never merged into a previous block.
fn merge_small_blocks(blocks: Vec<(usize, usize)>, lines: &[&str]) -> Vec<(usize, usize)> {
    if blocks.is_empty() {
        return blocks;
    }

    let mut merged: Vec<(usize, usize)> = Vec::new();
    let mut current = blocks[0];

    for &(start, end) in &blocks[1..] {
        let current_lines = current.1 - current.0 + 1;
        let next_lines = end - start + 1;
        let next_is_decl = DECL_RE.is_match(lines[start]);

        if !next_is_decl && (current_lines < MIN_MERGE_LINES || next_lines < MIN_MERGE_LINES) {
            // Merge: extend current to cover next block
            current.1 = end;
        } else {
            merged.push(current);
            current = (start, end);
        }
    }
    merged.push(current);

    merged
}

/// Split blocks that exceed MAX_CHUNK_LINES or MAX_CHUNK_CHARS.
fn split_oversized(blocks: Vec<(usize, usize)>, lines: &[&str]) -> Vec<(usize, usize)> {
    let mut result = Vec::new();

    for (start, end) in blocks {
        let line_count = end - start + 1;
        let snippet: String = lines[start..=end].join("\n");
        let char_count = non_ws_chars(&snippet);

        if line_count <= MAX_CHUNK_LINES && char_count <= MAX_CHUNK_CHARS {
            result.push((start, end));
            continue;
        }

        // Split at next blank line or boundary after midpoint
        let mid = start + MAX_CHUNK_LINES.min(line_count) / 2;
        let mut split_at = None;

        for (i, line) in lines.iter().enumerate().skip(mid).take(end - mid + 1) {
            if line.trim().is_empty() || is_boundary(line) {
                split_at = Some(i);
                break;
            }
        }

        match split_at {
            Some(sp) if sp > start && sp < end => {
                // Recurse on both halves
                let left = vec![(start, sp.saturating_sub(1).max(start))];
                // Skip blank line at split point
                let right_start = if lines[sp].trim().is_empty() {
                    sp + 1
                } else {
                    sp
                };
                result.extend(split_oversized(left, lines));
                if right_start <= end {
                    result.extend(split_oversized(vec![(right_start, end)], lines));
                }
            }
            _ => {
                // No structural boundary found — hard split at MAX_CHUNK_LINES
                let mut pos = start;
                while pos <= end {
                    let chunk_end = (pos + MAX_CHUNK_LINES - 1).min(end);
                    result.push((pos, chunk_end));
                    pos = chunk_end + 1;
                }
            }
        }
    }

    result
}

/// Extract a name from a chunk using heuristic regex matching.
fn extract_heuristic_name(snippet: &str) -> String {
    // Try the first few lines for a declaration keyword with a name
    for line in snippet.lines().take(3) {
        if let Some(caps) = NAME_RE.captures(line)
            && let Some(m) = caps.get(1)
        {
            return m.as_str().to_owned();
        }
    }
    String::new()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn chunk_rust_functions() {
        let code = r#"
fn hello(name: &str) -> String {
    format!("Hello, {name}!")
}

fn goodbye() {
    println!("bye");
}
"#;
        let chunks = chunk_content("test.rs", code, "rust");
        assert!(
            chunks.len() >= 2,
            "expected >=2 chunks, got {}",
            chunks.len()
        );
        let names: Vec<&str> = chunks.iter().map(|c| c.name.as_str()).collect();
        assert!(names.contains(&"hello"), "should find 'hello'");
        assert!(names.contains(&"goodbye"), "should find 'goodbye'");
    }

    #[test]
    fn chunk_python_class() {
        let code = r#"
class Dog:
    def __init__(self, name):
        self.name = name

    def bark(self):
        return "woof"

def standalone():
    pass
"#;
        let chunks = chunk_content("test.py", code, "python");
        assert!(!chunks.is_empty());
        let names: Vec<&str> = chunks.iter().map(|c| c.name.as_str()).collect();
        assert!(names.contains(&"Dog") || names.iter().any(|n| n.contains("Dog")));
    }

    #[test]
    fn chunk_elixir_module() {
        let code = r#"
defmodule MyApp.Worker do
  def start_link(opts) do
    GenServer.start_link(__MODULE__, opts)
  end

  def handle_call(:status, _from, state) do
    {:reply, :ok, state}
  end
end
"#;
        let chunks = chunk_content("worker.ex", code, "elixir");
        assert!(!chunks.is_empty(), "should chunk Elixir code");
        assert_eq!(chunks[0].language, "elixir");
    }

    #[test]
    fn chunk_cobol_code() {
        let code = r#"
       IDENTIFICATION DIVISION.
       PROGRAM-ID. HELLO-WORLD.

       PROCEDURE DIVISION.
           DISPLAY "Hello, World!".
           STOP RUN.
"#;
        let chunks = chunk_content("hello.cob", code, "cobol");
        assert!(!chunks.is_empty(), "should chunk COBOL code");
        assert_eq!(chunks[0].language, "cobol");
    }

    #[test]
    fn chunk_yaml_config() {
        let code = r#"
services:
  web:
    image: nginx
    ports:
      - "80:80"

  db:
    image: postgres
    environment:
      POSTGRES_DB: mydb
"#;
        let chunks = chunk_content("compose.yaml", code, "yaml");
        assert!(!chunks.is_empty(), "should chunk YAML");
    }

    #[test]
    fn chunk_dockerfile() {
        let code = r#"FROM node:20-alpine AS builder

WORKDIR /app

COPY package*.json ./
RUN npm ci

COPY . .
RUN npm run build

FROM node:20-alpine
COPY --from=builder /app/dist /app
CMD ["node", "/app/index.js"]
"#;
        let chunks = chunk_content("Dockerfile", code, "dockerfile");
        assert!(!chunks.is_empty(), "should chunk Dockerfile");
    }

    #[test]
    fn chunk_empty_file() {
        let chunks = chunk_content("empty.rs", "", "rust");
        assert!(chunks.is_empty());

        let chunks = chunk_content("blank.py", "   \n\n  ", "python");
        assert!(chunks.is_empty());
    }

    #[test]
    fn chunk_large_file_splits() {
        // Generate a file with >150 lines that should trigger splitting
        let mut code = String::new();
        for i in 0..200 {
            code.push_str(&format!("fn method_{i}() {{ /* body */ }}\n"));
        }
        let chunks = chunk_content("big.rs", &code, "rust");
        assert!(
            chunks.len() > 1,
            "large file should produce multiple chunks; got {}",
            chunks.len()
        );
    }

    #[test]
    fn all_chunks_have_language_hint() {
        let code = "fn main() {}\n";
        let chunks = chunk_content("test.rs", code, "rust");
        for chunk in &chunks {
            assert_eq!(chunk.language, "rust");
        }

        let code = "defmodule Foo do\nend\n";
        let chunks = chunk_content("foo.ex", code, "elixir");
        for chunk in &chunks {
            assert_eq!(chunk.language, "elixir");
        }
    }

    #[test]
    fn chunks_respect_max_lines() {
        // Generate a dense file with no blank lines — should hard-split
        let mut code = String::new();
        for i in 0..300 {
            code.push_str(&format!("line_{i} = value_{i}\n"));
        }
        let chunks = chunk_content("dense.txt", &code, "unknown");
        assert!(
            chunks.len() >= 2,
            "300-line file should produce multiple chunks"
        );
        for chunk in &chunks {
            let lines = chunk.end_line - chunk.start_line + 1;
            assert!(
                lines <= MAX_CHUNK_LINES,
                "chunk has {} lines, exceeds MAX_CHUNK_LINES ({})",
                lines,
                MAX_CHUNK_LINES,
            );
        }
    }

    #[test]
    fn non_ws_chars_counts_correctly() {
        assert_eq!(non_ws_chars(""), 0);
        assert_eq!(non_ws_chars("   \n\t  "), 0);
        assert_eq!(non_ws_chars("fn main() {}"), 10);
        assert_eq!(non_ws_chars("  hello  world  "), 10);
    }

    #[test]
    fn heuristic_name_extraction() {
        assert_eq!(extract_heuristic_name("fn hello(name: &str) {"), "hello");
        assert_eq!(extract_heuristic_name("def bark(self):"), "bark");
        assert_eq!(extract_heuristic_name("class Dog:"), "Dog");
        assert_eq!(
            extract_heuristic_name("defmodule MyApp.Worker do"),
            "MyApp.Worker"
        );
        assert_eq!(extract_heuristic_name("  key: value"), "");
    }
}
