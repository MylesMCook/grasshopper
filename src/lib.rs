//! Persistent hybrid search for code and memory.
//!
//! Grasshopper indexes codebases and stores memories, then retrieves them
//! with a pipeline combining full-text search, semantic embeddings, reciprocal
//! rank fusion, and cross-encoder reranking — all running locally via ONNX.
//!
//! This crate is primarily used as a binary (`grasshopper`). The library API
//! is not yet stabilized for external consumption.

pub mod code;
pub mod index;
pub mod mcp;
pub mod memory;
pub mod rerank;
pub mod search;
pub mod store;
