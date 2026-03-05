use anyhow::{Context, Result};
use tree_sitter::{Language, Parser, Query, QueryCursor, StreamingIterator};

/// A tag extracted from source code via tree-sitter TAGS_QUERY.
#[derive(Debug, Clone, PartialEq)]
pub struct Tag {
    /// The identifier name (e.g., "Store", "open", "greet").
    pub symbol: String,
    /// "definition" or "reference".
    pub role: String,
    /// The kind after the role (e.g., "function", "class", "call", "method").
    pub kind: String,
    /// 1-indexed line number.
    pub line: usize,
}

/// Get the tree-sitter Language object for a language name.
/// Reuses the same language strings as chunk.rs.
pub fn get_language(lang: &str) -> Option<Language> {
    match lang {
        "rust" => Some(tree_sitter_rust::LANGUAGE.into()),
        "javascript" => Some(tree_sitter_javascript::LANGUAGE.into()),
        "typescript" => Some(tree_sitter_typescript::LANGUAGE_TYPESCRIPT.into()),
        "tsx" => Some(tree_sitter_typescript::LANGUAGE_TSX.into()),
        "python" => Some(tree_sitter_python::LANGUAGE.into()),
        "go" => Some(tree_sitter_go::LANGUAGE.into()),
        "java" => Some(tree_sitter_java::LANGUAGE.into()),
        "c" => Some(tree_sitter_c::LANGUAGE.into()),
        "cpp" => Some(tree_sitter_cpp::LANGUAGE.into()),
        "c_sharp" => Some(tree_sitter_c_sharp::LANGUAGE.into()),
        "ruby" => Some(tree_sitter_ruby::LANGUAGE.into()),
        "php" => Some(tree_sitter_php::LANGUAGE_PHP.into()),
        "scala" => Some(tree_sitter_scala::LANGUAGE.into()),
        _ => None,
    }
}

/// Get the TAGS_QUERY string for a language.
/// Returns None if the language doesn't have a tags query (graceful skip).
pub fn get_tags_query(lang: &str) -> Option<&'static str> {
    match lang {
        "rust" => Some(tree_sitter_rust::TAGS_QUERY),
        "javascript" => Some(tree_sitter_javascript::TAGS_QUERY),
        "typescript" | "tsx" => Some(tree_sitter_typescript::TAGS_QUERY),
        "python" => Some(tree_sitter_python::TAGS_QUERY),
        "go" => Some(tree_sitter_go::TAGS_QUERY),
        "java" => Some(tree_sitter_java::TAGS_QUERY),
        "c" => Some(tree_sitter_c::TAGS_QUERY),
        "cpp" => Some(tree_sitter_cpp::TAGS_QUERY),
        "ruby" => Some(tree_sitter_ruby::TAGS_QUERY),
        "php" => Some(tree_sitter_php::TAGS_QUERY),
        "c_sharp" => Some(include_str!("../../queries/c_sharp_tags.scm")),
        "scala" => Some(include_str!("../../queries/scala_tags.scm")),
        _ => None,
    }
}

/// Extract definition and reference tags from source code using tree-sitter's TAGS_QUERY.
///
/// This is fully language-agnostic: the query file defines what constitutes a definition
/// or reference for each language. We just look for standardized capture names:
/// - `@definition.*` → a symbol definition (function, class, method, etc.)
/// - `@reference.*` → a symbol reference (call, class usage, etc.)
/// - `@name` → the identifier text (nested capture within def/ref patterns)
pub fn extract_tags(source: &str, language: Language, tags_query: &str) -> Result<Vec<Tag>> {
    if source.is_empty() {
        return Ok(Vec::new());
    }

    let mut parser = Parser::new();
    parser
        .set_language(&language)
        .context("setting parser language")?;

    let tree = parser
        .parse(source, None)
        .context("parsing source for tag extraction")?;

    let query = Query::new(&language, tags_query).context("compiling tags query")?;
    let capture_names = query.capture_names();

    let name_idx = capture_names
        .iter()
        .position(|n| *n == "name")
        .map(|i| i as u32);

    let mut role_map: Vec<Option<(&str, &str)>> = vec![None; capture_names.len()];
    for (i, name) in capture_names.iter().enumerate() {
        if let Some(kind) = name.strip_prefix("definition.") {
            role_map[i] = Some(("definition", kind));
        } else if let Some(kind) = name.strip_prefix("reference.") {
            role_map[i] = Some(("reference", kind));
        }
    }

    let source_bytes = source.as_bytes();
    let mut cursor = QueryCursor::new();
    let mut tags = Vec::new();

    let mut matches = cursor.matches(&query, tree.root_node(), source_bytes);
    while let Some(m) = matches.next() {
        let mut role_info: Option<(&str, &str, usize)> = None;
        let mut symbol_text: Option<&str> = None;

        for capture in m.captures {
            let idx = capture.index as usize;

            if Some(capture.index) == name_idx {
                symbol_text = capture.node.utf8_text(source_bytes).ok();
            }

            if idx < role_map.len() && let Some((role, kind)) = role_map[idx] {
                let line = capture.node.start_position().row + 1;
                role_info = Some((role, kind, line));
            }
        }

        if let (Some((role, kind, line)), Some(name)) = (role_info, symbol_text)
            && !name.is_empty()
        {
            tags.push(Tag {
                symbol: name.to_owned(),
                role: role.to_owned(),
                kind: kind.to_owned(),
                line,
            });
        }
    }

    Ok(tags)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn extract_rust_definitions_and_references() {
        let code = r#"
fn hello(name: &str) -> String {
    format!("Hello, {name}!")
}

fn goodbye() {
    hello("world");
}

struct Config {
    port: u16,
}
"#;
        let language = get_language("rust").unwrap();
        let query = get_tags_query("rust").unwrap();
        let tags = extract_tags(code, language, query).unwrap();

        let defs: Vec<_> = tags.iter().filter(|t| t.role == "definition").collect();
        let refs: Vec<_> = tags.iter().filter(|t| t.role == "reference").collect();

        let def_names: Vec<&str> = defs.iter().map(|t| t.symbol.as_str()).collect();
        assert!(def_names.contains(&"hello"), "should find 'hello' definition, got: {def_names:?}");
        assert!(def_names.contains(&"goodbye"), "should find 'goodbye' definition, got: {def_names:?}");
        assert!(def_names.contains(&"Config"), "should find 'Config' definition, got: {def_names:?}");

        let ref_names: Vec<&str> = refs.iter().map(|t| t.symbol.as_str()).collect();
        assert!(ref_names.contains(&"hello"), "should find 'hello' reference/call, got: {ref_names:?}");

        let hello_def = defs.iter().find(|t| t.symbol == "hello").unwrap();
        assert_eq!(hello_def.kind, "function");

        let config_def = defs.iter().find(|t| t.symbol == "Config").unwrap();
        assert_eq!(config_def.kind, "class");
    }

    #[test]
    fn extract_python_definitions_and_references() {
        let code = r#"
class Dog:
    def __init__(self, name):
        self.name = name

    def bark(self):
        return "woof"

def greet(name):
    print(name)

greet("buddy")
"#;
        let language = get_language("python").unwrap();
        let query = get_tags_query("python").unwrap();
        let tags = extract_tags(code, language, query).unwrap();

        let def_names: Vec<&str> = tags
            .iter()
            .filter(|t| t.role == "definition")
            .map(|t| t.symbol.as_str())
            .collect();

        assert!(def_names.contains(&"Dog"), "should find 'Dog' class, got: {def_names:?}");
        assert!(def_names.contains(&"greet"), "should find 'greet' function, got: {def_names:?}");

        let ref_names: Vec<&str> = tags
            .iter()
            .filter(|t| t.role == "reference")
            .map(|t| t.symbol.as_str())
            .collect();

        assert!(ref_names.contains(&"greet"), "should find 'greet' call reference, got: {ref_names:?}");
    }

    #[test]
    fn extract_javascript_definitions() {
        let code = r#"
function greet(name) {
    return "Hello, " + name;
}

class Animal {
    constructor(name) {
        this.name = name;
    }
}

greet("world");
"#;
        let language = get_language("javascript").unwrap();
        let query = get_tags_query("javascript").unwrap();
        let tags = extract_tags(code, language, query).unwrap();

        let def_names: Vec<&str> = tags
            .iter()
            .filter(|t| t.role == "definition")
            .map(|t| t.symbol.as_str())
            .collect();

        assert!(def_names.contains(&"greet"), "should find 'greet' function, got: {def_names:?}");
        assert!(def_names.contains(&"Animal"), "should find 'Animal' class, got: {def_names:?}");
    }

    #[test]
    fn extract_go_definitions() {
        let code = r#"
package main

func Hello(name string) string {
    return "Hello, " + name
}

type Server struct {
    Port int
}
"#;
        let language = get_language("go").unwrap();
        let query = get_tags_query("go").unwrap();
        let tags = extract_tags(code, language, query).unwrap();

        let def_names: Vec<&str> = tags
            .iter()
            .filter(|t| t.role == "definition")
            .map(|t| t.symbol.as_str())
            .collect();

        assert!(def_names.contains(&"Hello"), "should find 'Hello' function, got: {def_names:?}");
        assert!(def_names.contains(&"Server"), "should find 'Server' struct, got: {def_names:?}");
    }

    #[test]
    fn no_tags_query_returns_none_for_unknown_language() {
        assert!(get_tags_query("brainfuck").is_none());
        assert!(get_tags_query("yaml").is_none());
    }

    #[test]
    fn all_supported_languages_have_tags_query() {
        let langs = ["rust", "javascript", "typescript", "tsx", "python", "go",
                     "java", "c", "cpp", "c_sharp", "ruby", "php", "scala"];
        for lang in &langs {
            assert!(get_tags_query(lang).is_some(), "{lang} should have a tags query");
            assert!(get_language(lang).is_some(), "{lang} should have a language");
        }
    }

    #[test]
    fn empty_source_returns_empty() {
        let language = get_language("rust").unwrap();
        let query = get_tags_query("rust").unwrap();
        let tags = extract_tags("", language, query).unwrap();
        assert!(tags.is_empty());
    }

    #[test]
    fn tags_have_correct_line_numbers() {
        let code = "fn first() {}\n\nfn second() {}\n\nfn third() {}\n";
        let language = get_language("rust").unwrap();
        let query = get_tags_query("rust").unwrap();
        let tags = extract_tags(code, language, query).unwrap();

        let defs: Vec<_> = tags.iter().filter(|t| t.role == "definition").collect();
        assert!(defs.len() >= 3, "should find 3 function definitions");

        let first = defs.iter().find(|t| t.symbol == "first").unwrap();
        assert_eq!(first.line, 1);

        let second = defs.iter().find(|t| t.symbol == "second").unwrap();
        assert_eq!(second.line, 3);

        let third = defs.iter().find(|t| t.symbol == "third").unwrap();
        assert_eq!(third.line, 5);
    }
}
