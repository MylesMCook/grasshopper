pub mod chunk;
pub mod embed;
pub mod graph;
pub mod hnsw;
pub mod scan;
pub mod tokenizer;

/// Canonical file extension → language name mapping.
/// All other modules (chunk, graph) must handle every language listed here.
pub static EXT_MAP: &[(&str, &str)] = &[
    ("rs", "rust"),
    ("js", "javascript"),
    ("jsx", "javascript"),
    ("ts", "typescript"),
    ("tsx", "tsx"),
    ("mts", "typescript"),
    ("cts", "typescript"),
    ("py", "python"),
    ("go", "go"),
    ("java", "java"),
    ("c", "c"),
    ("h", "c"),
    ("cc", "cpp"),
    ("cpp", "cpp"),
    ("cxx", "cpp"),
    ("hpp", "cpp"),
    ("hh", "cpp"),
    ("cs", "c_sharp"),
    ("rb", "ruby"),
    ("php", "php"),
    ("scala", "scala"),
    ("sc", "scala"),
];

/// All unique language names from EXT_MAP.
pub fn supported_languages() -> Vec<&'static str> {
    let mut langs: Vec<&str> = EXT_MAP.iter().map(|(_, lang)| *lang).collect();
    langs.sort_unstable();
    langs.dedup();
    langs
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashSet;

    #[test]
    fn all_languages_have_chunk_and_graph_support() {
        let langs: HashSet<&str> = EXT_MAP.iter().map(|(_, lang)| *lang).collect();
        for lang in &langs {
            assert!(
                chunk::supports_language(lang),
                "chunk.rs missing config for language: {lang}"
            );
            assert!(
                graph::get_language(lang).is_some(),
                "graph.rs missing Language for: {lang}"
            );
            // tags queries are optional (some languages use built-in TAGS_QUERY)
        }
    }
}
