use std::collections::HashSet;

/// Split a camelCase or PascalCase identifier into sub-words (lowercase).
///
/// - `"myFunctionName"` → `["my", "function", "name"]`
/// - `"HTMLParser"` → `["html", "parser"]`
/// - `"simple"` → `["simple"]`
/// - `"already_snake"` → `["already_snake"]` (no camelCase to split)
fn split_camel(word: &str) -> Vec<String> {
    let chars: Vec<char> = word.chars().collect();
    if chars.is_empty() {
        return vec![];
    }

    let mut parts: Vec<String> = Vec::new();
    let mut current = String::new();

    for i in 0..chars.len() {
        let c = chars[i];
        if c.is_uppercase() && !current.is_empty() {
            // Start new part if:
            // - previous char was lowercase (e.g., "myF" → split before F)
            // - OR current is uppercase followed by lowercase and previous was uppercase
            //   (e.g., "HTMLParser" → split before P: "HTML" + "Parser")
            let prev_lower = i > 0 && chars[i - 1].is_lowercase();
            let acronym_end =
                i > 0 && chars[i - 1].is_uppercase() && i + 1 < chars.len() && chars[i + 1].is_lowercase();

            if prev_lower || acronym_end {
                parts.push(current.to_lowercase());
                current.clear();
            }
        }
        current.push(c);
    }

    if !current.is_empty() {
        parts.push(current.to_lowercase());
    }

    parts
}

/// Expand code text by splitting camelCase/PascalCase identifiers.
///
/// Each alphanumeric run is checked for camelCase boundaries. The original
/// text is preserved and sub-words are appended at the end, so both exact
/// and partial matches work in FTS5.
///
/// `"fn myFunctionName(x: i32)"` →
/// `"fn myFunctionName(x: i32) my function name"`
pub fn expand_code_tokens(text: &str) -> String {
    let mut result = text.to_string();
    let mut extras: Vec<String> = Vec::new();

    // Find alphanumeric runs and check for camelCase
    let mut word_start: Option<usize> = None;
    for (i, c) in text.char_indices() {
        if c.is_alphanumeric() {
            if word_start.is_none() {
                word_start = Some(i);
            }
        } else if let Some(start) = word_start {
            let word = &text[start..i];
            let parts = split_camel(word);
            if parts.len() > 1 {
                extras.extend(parts);
            }
            word_start = None;
        }
    }

    // Handle trailing word
    if let Some(start) = word_start {
        let word = &text[start..];
        let parts = split_camel(word);
        if parts.len() > 1 {
            extras.extend(parts);
        }
    }

    if !extras.is_empty() {
        result.push(' ');
        result.push_str(&extras.join(" "));
    }

    result
}

/// Prepare a query string for FTS5 with code-aware expansion.
///
/// Splits camelCase tokens and joins all unique terms with OR for
/// flexible matching across naming conventions.
///
/// `"myFunc"` → `"myfunc OR my OR func"`
/// `"cosine_similarity"` → `"cosine OR similarity"` (unicode61 splits on `_`)
pub fn prepare_fts_query(query: &str) -> String {
    let expanded = expand_code_tokens(query);

    let mut words = Vec::new();
    let mut seen = HashSet::new();

    for word in expanded.split(|c: char| !c.is_alphanumeric()).filter(|w| !w.is_empty()) {
        let lower = word.to_lowercase();
        if seen.insert(lower.clone()) {
            words.push(lower);
        }
    }

    if words.is_empty() {
        return query.to_string();
    }

    if words.len() == 1 {
        return words[0].clone();
    }

    words.join(" OR ")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn split_camel_simple() {
        assert_eq!(split_camel("myFunction"), vec!["my", "function"]);
    }

    #[test]
    fn split_camel_pascal_case() {
        assert_eq!(split_camel("MyClassName"), vec!["my", "class", "name"]);
    }

    #[test]
    fn split_camel_acronym() {
        assert_eq!(split_camel("HTMLParser"), vec!["html", "parser"]);
    }

    #[test]
    fn split_camel_trailing_acronym() {
        assert_eq!(split_camel("parseHTML"), vec!["parse", "html"]);
    }

    #[test]
    fn split_camel_no_split_needed() {
        assert_eq!(split_camel("simple"), vec!["simple"]);
    }

    #[test]
    fn split_camel_all_upper() {
        assert_eq!(split_camel("HTTP"), vec!["http"]);
    }

    #[test]
    fn split_camel_single_char() {
        assert_eq!(split_camel("x"), vec!["x"]);
    }

    #[test]
    fn split_camel_empty() {
        let result: Vec<String> = split_camel("");
        assert!(result.is_empty());
    }

    #[test]
    fn split_camel_multiple_acronyms() {
        assert_eq!(split_camel("XMLHTTPRequest"), vec!["xmlhttp", "request"]);
    }

    #[test]
    fn expand_preserves_original() {
        let expanded = expand_code_tokens("fn myFunction()");
        assert!(expanded.starts_with("fn myFunction()"));
        assert!(expanded.contains("my"));
        assert!(expanded.contains("function"));
    }

    #[test]
    fn expand_no_extras_for_snake_case() {
        let text = "my_function";
        let expanded = expand_code_tokens(text);
        // No camelCase → no extras appended (unicode61 handles underscore splitting)
        assert_eq!(expanded, text);
    }

    #[test]
    fn expand_handles_mixed() {
        let expanded = expand_code_tokens("myFunc and some_thing");
        assert!(expanded.starts_with("myFunc and some_thing"));
        // camelCase "myFunc" should have extras appended
        assert!(expanded.contains(" my"));
        assert!(expanded.contains(" func"));
        // "some_thing" has no camelCase, so no "thing" as an extra
        // (unicode61 handles underscore splitting in FTS5, not our expander)
        let extras = &expanded["myFunc and some_thing".len()..];
        assert!(!extras.contains("thing"));
    }

    #[test]
    fn query_single_word() {
        assert_eq!(prepare_fts_query("search"), "search");
    }

    #[test]
    fn query_camel_case_expanded() {
        let q = prepare_fts_query("myFunc");
        assert!(q.contains("OR"));
        assert!(q.contains("myfunc"));
        assert!(q.contains("my"));
        assert!(q.contains("func"));
    }

    #[test]
    fn query_snake_case() {
        let q = prepare_fts_query("my_func");
        // unicode61 splits on underscore, so we get "my" and "func"
        assert!(q.contains("my"));
        assert!(q.contains("func"));
    }

    #[test]
    fn query_deduplicates() {
        let q = prepare_fts_query("test test");
        assert_eq!(q, "test");
    }

    #[test]
    fn query_empty() {
        assert_eq!(prepare_fts_query(""), "");
    }
}
