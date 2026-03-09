pub mod chunk;
pub mod embed;
pub mod hnsw;
pub mod scan;
pub mod tokenizer;

/// Extension → language hint mapping. Covers common languages for metadata.
/// This is NOT a gate — files with extensions not listed here still get indexed
/// with the extension itself as the language hint.
pub static LANG_HINTS: &[(&str, &str)] = &[
    ("rs", "rust"),
    ("js", "javascript"),
    ("jsx", "javascript"),
    ("ts", "typescript"),
    ("tsx", "tsx"),
    ("mts", "typescript"),
    ("cts", "typescript"),
    ("py", "python"),
    ("pyi", "python"),
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
    ("ex", "elixir"),
    ("exs", "elixir"),
    ("erl", "erlang"),
    ("hrl", "erlang"),
    ("kt", "kotlin"),
    ("kts", "kotlin"),
    ("swift", "swift"),
    ("dart", "dart"),
    ("r", "r"),
    ("R", "r"),
    ("jl", "julia"),
    ("clj", "clojure"),
    ("cljs", "clojure"),
    ("cljc", "clojure"),
    ("hs", "haskell"),
    ("lhs", "haskell"),
    ("ml", "ocaml"),
    ("mli", "ocaml"),
    ("fs", "fsharp"),
    ("fsi", "fsharp"),
    ("fsx", "fsharp"),
    ("lua", "lua"),
    ("zig", "zig"),
    ("nim", "nim"),
    ("cr", "crystal"),
    ("v", "vlang"),
    ("pl", "perl"),
    ("pm", "perl"),
    ("cob", "cobol"),
    ("cbl", "cobol"),
    ("f90", "fortran"),
    ("f95", "fortran"),
    ("f03", "fortran"),
    ("pas", "pascal"),
    ("d", "dlang"),
    ("ada", "ada"),
    ("adb", "ada"),
    ("ads", "ada"),
    ("groovy", "groovy"),
    ("gradle", "groovy"),
    ("tf", "terraform"),
    ("hcl", "hcl"),
    ("yaml", "yaml"),
    ("yml", "yaml"),
    ("toml", "toml"),
    ("json", "json"),
    ("jsonc", "json"),
    ("xml", "xml"),
    ("html", "html"),
    ("htm", "html"),
    ("css", "css"),
    ("scss", "scss"),
    ("sass", "sass"),
    ("less", "less"),
    ("sql", "sql"),
    ("graphql", "graphql"),
    ("gql", "graphql"),
    ("proto", "protobuf"),
    ("md", "markdown"),
    ("mdx", "markdown"),
    ("rst", "restructuredtext"),
    ("tex", "latex"),
    ("csv", "csv"),
    ("sh", "shell"),
    ("bash", "shell"),
    ("zsh", "shell"),
    ("fish", "fish"),
    ("ps1", "powershell"),
    ("psm1", "powershell"),
    ("bat", "batch"),
    ("cmd", "batch"),
    ("cmake", "cmake"),
    ("mk", "make"),
    ("nix", "nix"),
    ("dhall", "dhall"),
    ("prisma", "prisma"),
];

/// Filename → language hint mapping for files without extensions.
pub static FILENAME_HINTS: &[(&str, &str)] = &[
    ("Dockerfile", "dockerfile"),
    ("Makefile", "make"),
    ("Rakefile", "ruby"),
    ("Gemfile", "ruby"),
    ("Justfile", "just"),
    ("CMakeLists.txt", "cmake"),
    ("Vagrantfile", "ruby"),
    ("Procfile", "procfile"),
    ("Taskfile.yml", "taskfile"),
];

/// Look up language hint from file extension.
pub fn lang_hint_from_ext(ext: &str) -> Option<&'static str> {
    LANG_HINTS
        .iter()
        .find(|(e, _)| *e == ext)
        .map(|(_, lang)| *lang)
}

/// Look up language hint from filename.
pub fn lang_hint_from_filename(name: &str) -> Option<&'static str> {
    FILENAME_HINTS
        .iter()
        .find(|(n, _)| *n == name)
        .map(|(_, lang)| *lang)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn lang_hint_lookups() {
        assert_eq!(lang_hint_from_ext("rs"), Some("rust"));
        assert_eq!(lang_hint_from_ext("ex"), Some("elixir"));
        assert_eq!(lang_hint_from_ext("cob"), Some("cobol"));
        assert_eq!(lang_hint_from_ext("xyz"), None);
        assert_eq!(lang_hint_from_filename("Dockerfile"), Some("dockerfile"));
        assert_eq!(lang_hint_from_filename("random.txt"), None);
    }
}
