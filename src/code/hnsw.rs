/// HNSW (Hierarchical Navigable Small World) index for fast approximate nearest neighbor search.
///
/// Wraps the `instant-distance` crate. Builds an HNSW graph over all embeddings,
/// serializes it to a sidecar file, and loads it on demand for O(log N) search instead of O(N).
use anyhow::{Context, Result};
use instant_distance::{Builder, HnswMap, Point, Search};
use serde::{Deserialize, Serialize};
use std::path::{Path, PathBuf};

/// A point in the HNSW index — a normalized f32 embedding vector.
#[derive(Clone, Serialize, Deserialize)]
pub struct EmbeddingPoint(pub Vec<f32>);

impl instant_distance::Point for EmbeddingPoint {
    fn distance(&self, other: &Self) -> f32 {
        // Cosine distance = 1 - dot_product (for L2-normalized vectors)
        let dot: f32 = self.0.iter().zip(other.0.iter()).map(|(a, b)| a * b).sum();
        1.0 - dot
    }
}

/// Wrapper around the HNSW index with chunk key mapping.
pub struct HnswIndex {
    map: HnswMap<EmbeddingPoint, i64>, // maps to chunk IDs
    chunk_count: usize,
}

impl HnswIndex {
    /// Build an HNSW index from pre-loaded embedding rows (id, vector).
    pub fn from_embeddings(rows: &[(i64, Vec<f32>)]) -> Result<Self> {
        if rows.is_empty() {
            return Ok(Self {
                map: Builder::default().build(Vec::new(), Vec::new()),
                chunk_count: 0,
            });
        }

        let mut points = Vec::with_capacity(rows.len());
        let mut values = Vec::with_capacity(rows.len());

        for (chunk_id, embedding) in rows {
            points.push(EmbeddingPoint(embedding.clone()));
            values.push(*chunk_id);
        }

        let chunk_count = points.len();
        tracing::info!("building HNSW index for {chunk_count} embeddings");

        let map = Builder::default()
            .ef_construction(100)
            .build(points, values);

        tracing::info!("HNSW index built ({chunk_count} points)");

        Ok(Self { map, chunk_count })
    }

    /// Search the HNSW index for nearest neighbors.
    /// Returns chunk IDs sorted by distance (closest first).
    pub fn search(&self, query: &[f32], limit: usize) -> Vec<(i64, f32)> {
        if self.chunk_count == 0 {
            return Vec::new();
        }

        let query_point = EmbeddingPoint(query.to_vec());
        let mut search = Search::default();

        self.map
            .search(&query_point, &mut search)
            .take(limit)
            .map(|item| {
                let chunk_id = *item.value;
                let distance = item.point.distance(&query_point);
                let similarity = 1.0 - distance; // convert back to cosine similarity
                (chunk_id, similarity)
            })
            .collect()
    }

    /// Number of points in the index.
    pub fn len(&self) -> usize {
        self.chunk_count
    }

    /// Whether the index is empty.
    pub fn is_empty(&self) -> bool {
        self.chunk_count == 0
    }

    /// Save the HNSW index to a file.
    pub fn save(&self, path: &Path) -> Result<()> {
        let data = bincode::serialize(&self.map)
            .context("serializing HNSW index")?;
        std::fs::write(path, &data)
            .with_context(|| format!("writing HNSW file: {}", path.display()))?;
        tracing::info!("saved HNSW index ({} points, {} bytes)", self.chunk_count, data.len());
        Ok(())
    }

    /// Load an HNSW index from a file.
    pub fn load(path: &Path) -> Result<Self> {
        let data = std::fs::read(path)
            .with_context(|| format!("reading HNSW file: {}", path.display()))?;
        let map: HnswMap<EmbeddingPoint, i64> = bincode::deserialize(&data)
            .context("deserializing HNSW index")?;

        // Count points by iterating
        let chunk_count = map.iter().count();
        tracing::info!("loaded HNSW index ({chunk_count} points)");

        Ok(Self { map, chunk_count })
    }
}

/// Get the HNSW sidecar file path for a given database path.
pub fn hnsw_path(db_path: &Path) -> PathBuf {
    db_path.with_extension("hnsw")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn build_search_empty_index() {
        let index = HnswIndex {
            map: Builder::default().build(Vec::new(), Vec::new()),
            chunk_count: 0,
        };
        let results = index.search(&[0.0; 768], 10);
        assert!(results.is_empty());
    }

    #[test]
    fn build_and_search_finds_nearest() {
        let mut v0 = vec![0.0f32; 768];
        v0[0] = 1.0;
        let mut v1 = vec![0.0f32; 768];
        v1[1] = 1.0;
        let mut v2 = vec![0.0f32; 768];
        v2[2] = 1.0;

        let points = vec![
            EmbeddingPoint(v0),
            EmbeddingPoint(v1),
            EmbeddingPoint(v2),
        ];
        let values = vec![100i64, 200, 300];

        let map = Builder::default().build(points, values);
        let index = HnswIndex { map, chunk_count: 3 };

        let mut query = vec![0.0f32; 768];
        query[0] = 0.9;
        query[1] = 0.1;

        let results = index.search(&query, 3);
        assert_eq!(results.len(), 3);
        assert_eq!(results[0].0, 100);
        assert!(results[0].1 >= results[1].1);
        assert!(results[1].1 >= results[2].1);
    }

    #[test]
    fn serialize_roundtrip() {
        let mut v0 = vec![0.0f32; 10];
        v0[0] = 1.0;
        let points = vec![EmbeddingPoint(v0)];
        let values = vec![42i64];

        let map = Builder::default().build(points, values);
        let index = HnswIndex { map, chunk_count: 1 };

        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("test.hnsw");

        index.save(&path).unwrap();
        let loaded = HnswIndex::load(&path).unwrap();

        assert_eq!(loaded.len(), 1);

        let mut query = vec![0.0f32; 10];
        query[0] = 1.0;
        let results = loaded.search(&query, 1);
        assert_eq!(results.len(), 1);
        assert_eq!(results[0].0, 42);
    }
}
