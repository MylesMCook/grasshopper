use anyhow::{bail, Result};
use tree_sitter::{Language, Parser};

const MAX_CHUNK_LINES: usize = 150;
const MAX_CHUNK_CHARS: usize = 1500; // non-whitespace characters

/// Count non-whitespace characters in a string.
fn non_ws_chars(s: &str) -> usize {
    s.chars().filter(|c| !c.is_whitespace()).count()
}

/// Configuration for a supported language.
struct LangConfig {
    language: Language,
    /// AST node types to extract as top-level chunks.
    top_level_nodes: &'static [&'static str],
    /// Node types that can be split into sub-items if too large.
    split_nodes: &'static [&'static str],
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

/// Check if the chunker supports a given language name.
pub fn supports_language(lang: &str) -> bool {
    get_lang_config(lang).is_some()
}

/// Get the tree-sitter Language for a language name.
fn get_lang_config(lang: &str) -> Option<LangConfig> {
    match lang {
        "rust" => Some(LangConfig {
            language: tree_sitter_rust::LANGUAGE.into(),
            top_level_nodes: &[
                "function_item", "struct_item", "enum_item", "impl_item",
                "trait_item", "type_item", "const_item", "static_item",
                "macro_definition", "mod_item",
            ],
            split_nodes: &["impl_item", "trait_item", "mod_item"],
        }),
        "javascript" => Some(LangConfig {
            language: tree_sitter_javascript::LANGUAGE.into(),
            top_level_nodes: &[
                "function_declaration", "generator_function_declaration",
                "class_declaration", "variable_declaration", "lexical_declaration",
                "export_statement",
            ],
            split_nodes: &["class_declaration"],
        }),
        "typescript" => Some(LangConfig {
            language: tree_sitter_typescript::LANGUAGE_TYPESCRIPT.into(),
            top_level_nodes: &[
                "function_declaration", "generator_function_declaration",
                "class_declaration", "abstract_class_declaration",
                "interface_declaration", "type_alias_declaration",
                "enum_declaration", "variable_declaration", "lexical_declaration",
                "export_statement",
            ],
            split_nodes: &["class_declaration", "abstract_class_declaration", "interface_declaration"],
        }),
        "tsx" => Some(LangConfig {
            language: tree_sitter_typescript::LANGUAGE_TSX.into(),
            top_level_nodes: &[
                "function_declaration", "generator_function_declaration",
                "class_declaration", "abstract_class_declaration",
                "interface_declaration", "type_alias_declaration",
                "enum_declaration", "variable_declaration", "lexical_declaration",
                "export_statement",
            ],
            split_nodes: &["class_declaration", "abstract_class_declaration", "interface_declaration"],
        }),
        "python" => Some(LangConfig {
            language: tree_sitter_python::LANGUAGE.into(),
            top_level_nodes: &[
                "function_definition", "class_definition", "decorated_definition",
            ],
            split_nodes: &["class_definition"],
        }),
        "go" => Some(LangConfig {
            language: tree_sitter_go::LANGUAGE.into(),
            top_level_nodes: &[
                "function_declaration", "method_declaration", "type_declaration",
                "const_declaration", "var_declaration",
            ],
            split_nodes: &[],
        }),
        "java" => Some(LangConfig {
            language: tree_sitter_java::LANGUAGE.into(),
            top_level_nodes: &[
                "class_declaration", "interface_declaration", "enum_declaration",
                "annotation_type_declaration", "method_declaration",
            ],
            split_nodes: &["class_declaration", "interface_declaration"],
        }),
        "c" => Some(LangConfig {
            language: tree_sitter_c::LANGUAGE.into(),
            top_level_nodes: &[
                "function_definition", "struct_specifier", "enum_specifier",
                "type_definition", "declaration",
            ],
            split_nodes: &[],
        }),
        "cpp" => Some(LangConfig {
            language: tree_sitter_cpp::LANGUAGE.into(),
            top_level_nodes: &[
                "function_definition", "class_specifier", "struct_specifier",
                "enum_specifier", "namespace_definition", "template_declaration",
                "type_definition", "declaration",
            ],
            split_nodes: &["class_specifier", "namespace_definition"],
        }),
        "c_sharp" => Some(LangConfig {
            language: tree_sitter_c_sharp::LANGUAGE.into(),
            top_level_nodes: &[
                "class_declaration", "interface_declaration", "struct_declaration",
                "enum_declaration", "method_declaration", "namespace_declaration",
            ],
            split_nodes: &["class_declaration", "namespace_declaration"],
        }),
        "ruby" => Some(LangConfig {
            language: tree_sitter_ruby::LANGUAGE.into(),
            top_level_nodes: &[
                "method", "singleton_method", "class", "module",
            ],
            split_nodes: &["class", "module"],
        }),
        "php" => Some(LangConfig {
            language: tree_sitter_php::LANGUAGE_PHP.into(),
            top_level_nodes: &[
                "function_definition", "class_declaration", "interface_declaration",
                "trait_declaration", "enum_declaration", "method_declaration",
            ],
            split_nodes: &["class_declaration"],
        }),
        "scala" => Some(LangConfig {
            language: tree_sitter_scala::LANGUAGE.into(),
            top_level_nodes: &[
                "function_definition", "class_definition", "trait_definition",
                "object_definition", "val_definition", "var_definition",
            ],
            split_nodes: &["class_definition", "trait_definition", "object_definition"],
        }),
        _ => None,
    }
}

/// Parse a source file into semantic chunks using tree-sitter.
pub fn chunk_file(
    file_path: &str,
    content: &str,
    lang: &str,
) -> Result<Vec<ParsedChunk>> {
    let config = match get_lang_config(lang) {
        Some(c) => c,
        None => bail!("unsupported language: {lang}"),
    };

    let mut parser = Parser::new();
    parser.set_language(&config.language)?;

    let tree = match parser.parse(content, None) {
        Some(t) => t,
        None => bail!("failed to parse {file_path}"),
    };

    let source = content.as_bytes();
    let mut chunks = Vec::new();
    let top_set: std::collections::HashSet<&str> = config.top_level_nodes.iter().copied().collect();
    let split_set: std::collections::HashSet<&str> = config.split_nodes.iter().copied().collect();

    let root = tree.root_node();
    let mut cursor = root.walk();

    for child in root.children(&mut cursor) {
        let kind = child.kind();

        if !top_set.contains(kind) {
            continue;
        }

        let start_line = child.start_position().row + 1;
        let end_line = child.end_position().row + 1;
        let line_count = end_line - start_line + 1;
        let snippet = child.utf8_text(source).unwrap_or("").to_owned();
        let char_count = non_ws_chars(&snippet);

        // Try to split large nodes (either metric can trigger)
        if (char_count > MAX_CHUNK_CHARS || line_count > MAX_CHUNK_LINES) && split_set.contains(kind) {
            let parent_name = extract_name(&child, source);
            let sub_chunks = split_large_node(&child, source, file_path, lang, &parent_name);
            if !sub_chunks.is_empty() {
                chunks.extend(sub_chunks);
                continue;
            }
        }

        let name = extract_name(&child, source);
        let signature = extract_signature(&child, source);

        chunks.push(ParsedChunk {
            chunk_key: format!("{file_path}:{kind}:{name}:{start_line}:{end_line}"),
            language: lang.to_owned(),
            kind: kind.to_owned(),
            name,
            signature,
            snippet,
            start_line,
            end_line,
        });
    }

    Ok(chunks)
}

/// Split a large node (class, impl, module) into its child items.
/// Child names are qualified with the parent name (e.g., "Store::open").
fn split_large_node(
    node: &tree_sitter::Node<'_>,
    source: &[u8],
    file_path: &str,
    lang: &str,
    parent_name: &str,
) -> Vec<ParsedChunk> {
    let mut chunks = Vec::new();

    // Find the body/block child that contains sub-items
    let body = find_body_node(node);
    let container = body.unwrap_or(*node);

    let mut cursor = container.walk();
    for child in container.children(&mut cursor) {
        let kind = child.kind();

        // Skip punctuation, comments, whitespace-like nodes
        if !is_meaningful_node(kind) {
            continue;
        }

        let start_line = child.start_position().row + 1;
        let end_line = child.end_position().row + 1;
        let snippet = child.utf8_text(source).unwrap_or("").to_owned();
        let child_name = extract_name(&child, source);
        let signature = extract_signature(&child, source);

        // Qualify with parent: "Store::open" instead of just "open"
        let name = if !parent_name.is_empty() && !child_name.is_empty() {
            format!("{parent_name}::{child_name}")
        } else {
            child_name
        };

        chunks.push(ParsedChunk {
            chunk_key: format!("{file_path}:{kind}:{name}:{start_line}:{end_line}"),
            language: lang.to_owned(),
            kind: kind.to_owned(),
            name,
            signature,
            snippet,
            start_line,
            end_line,
        });
    }

    chunks
}

/// Find the body/block child of a node (class_body, declaration_list, block, etc.).
fn find_body_node<'a>(node: &'a tree_sitter::Node<'a>) -> Option<tree_sitter::Node<'a>> {
    let body_kinds = &[
        "class_body", "declaration_list", "block", "body",
        "interface_body", "enum_body", "trait_body", "field_declaration_list",
    ];

    let mut cursor = node.walk();
    node.children(&mut cursor).find(|&child| body_kinds.contains(&child.kind()))
}

/// Check if a node kind is meaningful (not just syntax sugar).
fn is_meaningful_node(kind: &str) -> bool {
    !matches!(
        kind,
        "{" | "}" | "(" | ")" | "[" | "]" | ";" | "," | "comment"
            | "line_comment" | "block_comment" | "attribute_item"
    )
}

/// Extract the name from an AST node.
fn extract_name(node: &tree_sitter::Node<'_>, source: &[u8]) -> String {
    // Look for named children that typically hold the identifier.
    // "type" handles Rust `impl Foo` where the type field holds the name.
    let name_fields = &["name", "declarator", "pattern", "type"];

    for field in name_fields {
        if let Some(child) = node.child_by_field_name(field) {
            let text = child.utf8_text(source).unwrap_or("");
            // For declarators, get just the identifier part
            if (child.kind() == "function_declarator" || child.kind() == "init_declarator")
                && let Some(id) = child.child_by_field_name("declarator") {
                    return id.utf8_text(source).unwrap_or("").to_owned();
                }
            return text.to_owned();
        }
    }

    String::new()
}

/// Extract a type signature from an AST node.
fn extract_signature(node: &tree_sitter::Node<'_>, source: &[u8]) -> String {
    let text = node.utf8_text(source).unwrap_or("");

    // Take the first line as signature (function declaration line, struct header, etc.)
    let first_line = text.lines().next().unwrap_or("");

    // Trim trailing opening braces
    let sig = first_line.trim_end_matches(|c: char| c == '{' || c.is_whitespace());

    if sig.len() > 200 {
        format!("{}...", &sig[..200])
    } else {
        sig.to_owned()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn chunk_rust_function() {
        let code = r#"
fn hello(name: &str) -> String {
    format!("Hello, {name}!")
}

fn goodbye() {
    println!("bye");
}
"#;
        let chunks = chunk_file("test.rs", code, "rust").unwrap();
        assert_eq!(chunks.len(), 2);
        assert_eq!(chunks[0].name, "hello");
        assert_eq!(chunks[0].kind, "function_item");
        assert_eq!(chunks[1].name, "goodbye");
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
        let chunks = chunk_file("test.py", code, "python").unwrap();
        assert_eq!(chunks.len(), 2);
        assert_eq!(chunks[0].kind, "class_definition");
        assert_eq!(chunks[1].name, "standalone");
    }

    #[test]
    fn chunk_typescript_exports() {
        let code = r#"
export function greet(name: string): string {
    return `Hello, ${name}!`;
}

export const PI = 3.14;
"#;
        let chunks = chunk_file("test.ts", code, "typescript").unwrap();
        assert!(chunks.len() >= 2);
    }

    #[test]
    fn unsupported_language_returns_error() {
        let result = chunk_file("test.yaml", "key: value", "yaml");
        assert!(result.is_err());
    }

    #[test]
    fn chunk_javascript_function() {
        let code = r#"
function greet(name) {
    return "Hello, " + name;
}

const add = (a, b) => a + b;

class Animal {
    constructor(name) {
        this.name = name;
    }
}
"#;
        let chunks = chunk_file("test.js", code, "javascript").unwrap();
        assert!(chunks.len() >= 3, "expected function, const, and class; got {}", chunks.len());
        let names: Vec<&str> = chunks.iter().map(|c| c.name.as_str()).collect();
        assert!(names.contains(&"greet"), "should find 'greet' function");
    }

    #[test]
    fn chunk_go_function_and_struct() {
        let code = r#"
package main

func Hello(name string) string {
    return "Hello, " + name
}

type Server struct {
    Port int
    Host string
}

func (s *Server) Start() error {
    return nil
}
"#;
        let chunks = chunk_file("main.go", code, "go").unwrap();
        assert!(chunks.len() >= 2, "expected func + type; got {}", chunks.len());
        let names: Vec<&str> = chunks.iter().map(|c| c.name.as_str()).collect();
        assert!(names.contains(&"Hello"), "should find 'Hello' function");
    }

    #[test]
    fn chunk_java_class() {
        let code = r#"
public class Dog {
    private String name;

    public Dog(String name) {
        this.name = name;
    }

    public String bark() {
        return "woof";
    }
}
"#;
        let chunks = chunk_file("Dog.java", code, "java").unwrap();
        assert!(!chunks.is_empty(), "should find at least the class");
        assert_eq!(chunks[0].kind, "class_declaration");
    }

    #[test]
    fn chunk_c_function() {
        let code = r#"
int add(int a, int b) {
    return a + b;
}

struct Point {
    int x;
    int y;
};
"#;
        let chunks = chunk_file("math.c", code, "c").unwrap();
        assert!(chunks.len() >= 1, "should find at least the function");
        let kinds: Vec<&str> = chunks.iter().map(|c| c.kind.as_str()).collect();
        assert!(kinds.contains(&"function_definition"), "should find function_definition");
    }

    #[test]
    fn chunk_empty_file() {
        // Empty content should return empty chunks, not panic
        let chunks = chunk_file("empty.rs", "", "rust").unwrap();
        assert!(chunks.is_empty());

        let chunks = chunk_file("blank.py", "   \n\n  ", "python").unwrap();
        assert!(chunks.is_empty());
    }

    #[test]
    fn chunk_large_node_splits() {
        // Generate a Rust impl block with >150 lines that should trigger splitting
        let mut code = String::from("struct Big;\n\nimpl Big {\n");
        for i in 0..20 {
            code.push_str(&format!(
                "    fn method_{i}(&self) -> i32 {{\n        // line 1\n        // line 2\n        // line 3\n        // line 4\n        // line 5\n        // line 6\n        // line 7\n        {i}\n    }}\n\n",
            ));
        }
        code.push_str("}\n");

        let chunks = chunk_file("big.rs", &code, "rust").unwrap();
        // The impl has >150 lines, so it should be split into individual methods
        assert!(
            chunks.len() > 1,
            "large impl should split into sub-items; got {} chunks",
            chunks.len()
        );
        // Each chunk should be a method, not the whole impl
        for chunk in &chunks {
            let lines = chunk.end_line - chunk.start_line + 1;
            assert!(
                lines <= MAX_CHUNK_LINES,
                "chunk '{}' has {} lines, exceeding MAX_CHUNK_LINES",
                chunk.name,
                lines
            );
        }
    }

    #[test]
    fn split_children_have_parent_name() {
        // Generate a Rust impl that triggers splitting
        let mut code = String::from("struct Foo;\n\nimpl Foo {\n");
        for i in 0..20 {
            code.push_str(&format!(
                "    fn do_{i}(&self) -> i32 {{\n        // filler line 1\n        // filler line 2\n        // filler line 3\n        // filler line 4\n        // filler line 5\n        // filler line 6\n        // filler line 7\n        {i}\n    }}\n\n",
            ));
        }
        code.push_str("}\n");

        let chunks = chunk_file("foo.rs", &code, "rust").unwrap();
        // Methods from the split impl should be qualified with "Foo::"
        let method_chunks: Vec<_> = chunks.iter().filter(|c| c.kind == "function_item").collect();
        assert!(!method_chunks.is_empty());
        for chunk in &method_chunks {
            assert!(
                chunk.name.starts_with("Foo::"),
                "expected parent-qualified name, got '{}'",
                chunk.name,
            );
        }
    }

    #[test]
    fn non_ws_chars_counts_correctly() {
        assert_eq!(non_ws_chars(""), 0);
        assert_eq!(non_ws_chars("   \n\t  "), 0);
        assert_eq!(non_ws_chars("fn main() {}"), 10); // no spaces counted
        assert_eq!(non_ws_chars("  hello  world  "), 10);
    }

    #[test]
    fn char_based_split_triggers_on_dense_code() {
        // A compact impl with many one-liner methods — low line count but high non-ws chars
        let mut code = String::from("struct Dense;\n\nimpl Dense {\n");
        // Each method is ~3 lines but densely packed
        for i in 0..80 {
            code.push_str(&format!(
                "    fn m{i}(&self) -> String {{ format!(\"aaaaaaaaaaaaaaaa{i}\") }}\n",
            ));
        }
        code.push_str("}\n");

        let char_count = non_ws_chars(&code);
        assert!(
            char_count > MAX_CHUNK_CHARS,
            "test code should exceed char limit; got {char_count} non-ws chars",
        );

        let chunks = chunk_file("dense.rs", &code, "rust").unwrap();
        // Should split because non-ws chars exceed MAX_CHUNK_CHARS
        assert!(
            chunks.len() > 2,
            "dense impl should split; got {} chunks",
            chunks.len(),
        );
    }
}
