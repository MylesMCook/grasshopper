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
            let acronym_end = i > 0
                && chars[i - 1].is_uppercase()
                && i + 1 < chars.len()
                && chars[i + 1].is_lowercase();

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

    for word in expanded
        .split(|c: char| !c.is_alphanumeric())
        .filter(|w| !w.is_empty())
    {
        let lower = word.to_lowercase();
        if seen.insert(lower.clone()) {
            words.push(lower);
        }
    }

    if words.is_empty() {
        return query.to_string();
    }

    if words.len() == 1 {
        // Quote single tokens to prevent FTS5 operator interpretation
        // (e.g. "not", "and", "near" are FTS5 keywords)
        return format!("\"{}\"", words.into_iter().next().unwrap());
    }

    // Quote each token so FTS5 treats them as literals, not operators.
    // Without quotes, a query like "error NOT found" would be interpreted
    // as an FTS5 NOT expression instead of searching for the word "not".
    words
        .into_iter()
        .map(|w| format!("\"{}\"", w))
        .collect::<Vec<_>>()
        .join(" OR ")
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
        assert_eq!(prepare_fts_query("search"), "\"search\"");
    }

    #[test]
    fn query_camel_case_expanded() {
        let q = prepare_fts_query("myFunc");
        assert!(q.contains("OR"));
        assert!(q.contains("\"myfunc\""));
        assert!(q.contains("\"my\""));
        assert!(q.contains("\"func\""));
    }

    #[test]
    fn query_snake_case() {
        let q = prepare_fts_query("my_func");
        // unicode61 splits on underscore, so we get "my" and "func"
        assert!(q.contains("\"my\""));
        assert!(q.contains("\"func\""));
    }

    #[test]
    fn query_deduplicates() {
        let q = prepare_fts_query("test test");
        assert_eq!(q, "\"test\"");
    }

    #[test]
    fn query_empty() {
        assert_eq!(prepare_fts_query(""), "");
    }

    #[test]
    fn query_fts5_reserved_words_are_quoted() {
        // FTS5 keywords (AND, OR, NOT, NEAR) are case-insensitive.
        // Without quoting, "error NOT found" would exclude "found" results.
        let q = prepare_fts_query("error NOT found");
        assert_eq!(q, "\"error\" OR \"not\" OR \"found\"");

        let q = prepare_fts_query("foo AND bar");
        assert_eq!(q, "\"foo\" OR \"and\" OR \"bar\"");

        let q = prepare_fts_query("NEAR");
        assert_eq!(q, "\"near\"");
    }

    // ---------------------------------------------------------------
    // Adversarial FTS5 query fuzzing
    // ---------------------------------------------------------------

    /// Helper: assert that prepare_fts_query does not crash and that any
    /// alphanumeric tokens in the output are properly quoted. When the input
    /// has no alphanumeric characters, the function returns the raw input
    /// as a passthrough (safe because FTS5 MATCH on non-word chars is benign).
    fn assert_safe_fts(input: &str) {
        let result = prepare_fts_query(input);
        // Must not panic — reaching here means no crash.
        // If result contains alphanumeric tokens, verify they are quoted.
        if result.is_empty() {
            return;
        }
        // Check that no FTS5 operator keywords appear unquoted
        let has_alphanumeric = result.chars().any(|c| c.is_alphanumeric());
        if !has_alphanumeric {
            // Pure non-alphanumeric passthrough — safe, FTS5 won't interpret as operators
            return;
        }
        // When alphanumeric tokens exist, they should be in quoted segments
        for segment in result.split(" OR ") {
            let trimmed = segment.trim();
            if trimmed.is_empty() || !trimmed.chars().any(|c| c.is_alphanumeric()) {
                continue;
            }
            assert!(
                trimmed.starts_with('"') && trimmed.ends_with('"'),
                "unquoted alphanumeric segment in output for input {:?}: segment = {:?}, full = {:?}",
                input,
                trimmed,
                result,
            );
        }
    }

    #[test]
    fn fuzz_fts5_operators() {
        // FTS5 boolean operators must be neutralised
        assert_safe_fts("error NOT found");
        assert_safe_fts("foo AND bar");
        assert_safe_fts("x OR y");
        assert_safe_fts("NEAR(x,y)");
        assert_safe_fts("NEAR/5(a b)");
        assert_safe_fts("NOT NOT NOT");
    }

    #[test]
    fn fuzz_fts5_column_filters() {
        // Column filter syntax should be stripped by the alphanumeric split
        assert_safe_fts("title:hack");
        assert_safe_fts("content:secret");
        assert_safe_fts("symbol_name:drop");
        assert_safe_fts("{col}:value");
    }

    #[test]
    fn fuzz_fts5_wildcards() {
        assert_safe_fts("test*");
        assert_safe_fts("*");
        assert_safe_fts("te*st");
        assert_safe_fts("***");
    }

    #[test]
    fn fuzz_sql_injection() {
        assert_safe_fts("'; DROP TABLE chunks; --");
        assert_safe_fts("\" OR 1=1");
        assert_safe_fts("'; DELETE FROM chunks WHERE ''='");
        assert_safe_fts("1; SELECT * FROM sqlite_master; --");
        assert_safe_fts("UNION SELECT * FROM chunks --");
    }

    #[test]
    fn fuzz_unicode_emoji() {
        // Emoji and pictographs
        assert_safe_fts("\u{1F600}"); // grinning face
        assert_safe_fts("\u{1F4A9}\u{1F525}"); // pile of poo + fire
        assert_safe_fts("search \u{1F50D} query");
    }

    #[test]
    fn fuzz_unicode_cjk() {
        assert_safe_fts("\u{4F60}\u{597D}"); // 你好
        assert_safe_fts("\u{3053}\u{3093}\u{306B}\u{3061}\u{306F}"); // こんにちは
        assert_safe_fts("\u{D55C}\u{AD6D}\u{C5B4}"); // 한국어
    }

    #[test]
    fn fuzz_unicode_rtl() {
        assert_safe_fts("\u{0645}\u{0631}\u{062D}\u{0628}\u{0627}"); // مرحبا (Arabic)
        assert_safe_fts("\u{05E9}\u{05DC}\u{05D5}\u{05DD}"); // שלום (Hebrew)
    }

    #[test]
    fn fuzz_unicode_zero_width() {
        // Zero-width chars should not produce unquoted operators
        assert_safe_fts("test\u{200B}word"); // zero-width space
        assert_safe_fts("a\u{200C}b"); // zero-width non-joiner
        assert_safe_fts("x\u{200D}y"); // zero-width joiner
        assert_safe_fts("\u{FEFF}bom"); // BOM / zero-width no-break space
    }

    #[test]
    fn fuzz_edge_empty_string() {
        let result = prepare_fts_query("");
        assert_eq!(result, "");
    }

    #[test]
    fn fuzz_edge_single_char() {
        assert_safe_fts("a");
        assert_safe_fts("1");
        assert_safe_fts("Z");
    }

    #[test]
    fn fuzz_edge_huge_string() {
        // 100KB of repeated text — must not panic or hang
        let big = "adversarial ".repeat(8500); // ~102KB
        let result = prepare_fts_query(&big);
        assert!(!result.is_empty());
        // Should deduplicate to a single quoted token
        assert_eq!(result, "\"adversarial\"");
    }

    #[test]
    fn fuzz_edge_null_bytes() {
        assert_safe_fts("hello\x00world");
        assert_safe_fts("\x00\x00\x00");
        assert_safe_fts("test\x00");
    }

    #[test]
    fn fuzz_fts5_special_syntax() {
        // FTS5 prefix token
        assert_safe_fts("^start");
        // FTS5 column set
        assert_safe_fts("{title content}:search");
        // NEAR with explicit distance
        assert_safe_fts("NEAR/5");
        // Initial token query
        assert_safe_fts("^ first");
    }

    #[test]
    fn fuzz_punctuation_only() {
        // Queries that are purely punctuation — no alphanumeric tokens
        let result = prepare_fts_query("!@#$%^&*()");
        // Should return original since words vec is empty
        assert_eq!(result, "!@#$%^&*()");
    }

    #[test]
    fn fuzz_mixed_scripts_and_operators() {
        assert_safe_fts("\u{4F60}\u{597D} AND \u{3053}\u{3093}\u{306B}\u{3061}\u{306F}");
        assert_safe_fts("error NOT \u{D55C}\u{AD6D}\u{C5B4}");
    }

    #[test]
    fn fuzz_deeply_nested_parens() {
        assert_safe_fts("((((((((((test))))))))))");
        assert_safe_fts("NEAR((((a))),((b)))");
    }

    #[test]
    fn fuzz_repeated_quotes() {
        // Attempt to break out of quoting
        assert_safe_fts("\"\"\"\"\"");
        assert_safe_fts("test\"escape\"attempt");
        assert_safe_fts("'single'quotes'");
    }

    #[test]
    fn fuzz_backslashes() {
        assert_safe_fts("path\\to\\file");
        assert_safe_fts("\\n\\r\\t\\0");
        assert_safe_fts("\\\\\\\\");
    }

    #[test]
    fn fuzz_whitespace_variants() {
        assert_safe_fts("  multiple   spaces  ");
        assert_safe_fts("\t\ttabs\t");
        assert_safe_fts("\nnewlines\r\n");
        assert_safe_fts("a\r\nb");
    }
}
