/// HNSW (Hierarchical Navigable Small World) index for fast approximate nearest neighbor search.
///
/// Wraps the `instant-distance` crate. Builds an HNSW graph over all embeddings,
/// serializes it to a sidecar file, and loads it on demand for O(log N) search instead of O(N).
use anyhow::{Context, Result};
use bincode::Options;
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

    /// Magic bytes to identify the v2 format (with prepended point count).
    /// "GH02" in ASCII. Old files won't start with this, enabling format detection.
    const FORMAT_MAGIC: &'static [u8; 4] = b"GH02";

    /// Save the HNSW index to a file.
    /// Format: [4-byte magic "GH02"][8-byte little-endian point count][bincode HNSW data]
    pub fn save(&self, path: &Path) -> Result<()> {
        let hnsw_data = bincode::serialize(&self.map).context("serializing HNSW index")?;

        let mut data = Vec::with_capacity(4 + 8 + hnsw_data.len());
        data.extend_from_slice(Self::FORMAT_MAGIC);
        data.extend_from_slice(&(self.chunk_count as u64).to_le_bytes());
        data.extend_from_slice(&hnsw_data);

        std::fs::write(path, &data)
            .with_context(|| format!("writing HNSW file: {}", path.display()))?;
        tracing::info!(
            "saved HNSW index ({} points, {} bytes)",
            self.chunk_count,
            data.len()
        );
        Ok(())
    }

    /// Load an HNSW index from a file.
    /// Supports both v2 format (with magic + count header) and legacy format (raw bincode).
    pub fn load(path: &Path) -> Result<Self> {
        let data = std::fs::read(path)
            .with_context(|| format!("reading HNSW file: {}", path.display()))?;

        // Detect format: v2 starts with "GH02" magic bytes
        if data.len() >= 12 && &data[..4] == Self::FORMAT_MAGIC {
            let chunk_count = u64::from_le_bytes(data[4..12].try_into().unwrap()) as usize;
            // Bound deserialization to the actual file size to prevent OOM
            let map: HnswMap<EmbeddingPoint, i64> = bincode::options()
                .with_fixint_encoding()
                .allow_trailing_bytes()
                .with_limit(data.len() as u64)
                .deserialize(&data[12..])
                .context("deserializing HNSW index (v2)")?;
            // Validate header count against actual graph
            let actual_count = map.iter().count();
            let chunk_count = if chunk_count != actual_count {
                tracing::warn!(
                    "HNSW header count mismatch: header={chunk_count}, actual={actual_count}; using actual"
                );
                actual_count
            } else {
                chunk_count
            };
            tracing::info!("loaded HNSW index ({chunk_count} points)");
            Ok(Self { map, chunk_count })
        } else {
            // Legacy format: raw bincode without header. Fall back to iterate-and-count.
            let map: HnswMap<EmbeddingPoint, i64> = bincode::options()
                .with_fixint_encoding()
                .allow_trailing_bytes()
                .with_limit(data.len() as u64)
                .deserialize(&data)
                .context("deserializing HNSW index (legacy)")?;
            let chunk_count = map.iter().count();
            tracing::info!("loaded HNSW index ({chunk_count} points, legacy format)");
            Ok(Self { map, chunk_count })
        }
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

        let points = vec![EmbeddingPoint(v0), EmbeddingPoint(v1), EmbeddingPoint(v2)];
        let values = vec![100i64, 200, 300];

        let map = Builder::default().build(points, values);
        let index = HnswIndex {
            map,
            chunk_count: 3,
        };

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
        let index = HnswIndex {
            map,
            chunk_count: 1,
        };

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

    // ---------------------------------------------------------------
    // Adversarial HNSW corruption recovery tests
    // ---------------------------------------------------------------

    #[test]
    fn corrupt_zero_byte_file() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("empty.hnsw");
        std::fs::write(&path, b"").unwrap();
        let result = HnswIndex::load(&path);
        assert!(result.is_err(), "zero-byte file should return Err");
    }

    #[test]
    fn corrupt_random_garbage() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("garbage.hnsw");
        let garbage = vec![0xFFu8; 256];
        std::fs::write(&path, &garbage).unwrap();
        let result = HnswIndex::load(&path);
        assert!(
            result.is_err(),
            "256 bytes of 0xFF should return Err, not panic/OOM"
        );
    }

    #[test]
    fn corrupt_valid_header_truncated_body() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("truncated.hnsw");
        // Valid GH02 magic + reasonable point count + minimal junk body
        let mut data = Vec::new();
        data.extend_from_slice(b"GH02");
        data.extend_from_slice(&5u64.to_le_bytes()); // 5 points
        data.extend_from_slice(&[0xAB, 0xCD, 0xEF]); // 3 bytes of junk
        std::fs::write(&path, &data).unwrap();
        let result = HnswIndex::load(&path);
        assert!(
            result.is_err(),
            "valid header with truncated bincode should return Err"
        );
    }

    #[test]
    fn corrupt_absurd_point_count_no_oom() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("absurd.hnsw");
        // GH02 header with u64::MAX point count but only a few bytes of data.
        // The with_limit(data.len()) guard must prevent OOM — the critical property
        // is that this does not allocate u64::MAX memory. Whether deserialization
        // succeeds or fails depends on whether bincode can parse the payload,
        // but either outcome is safe.
        let mut data = Vec::new();
        data.extend_from_slice(b"GH02");
        data.extend_from_slice(&u64::MAX.to_le_bytes());
        data.extend_from_slice(&[0x00; 64]); // minimal payload
        std::fs::write(&path, &data).unwrap();
        // The key assertion: this completes without OOM/panic
        let _result = HnswIndex::load(&path);
        // If we reach here, the with_limit guard prevented catastrophic allocation.
        // The header's u64::MAX point count is NOT validated against actual data,
        // so chunk_count may be wrong — but that's a data integrity issue, not a
        // crash vector. The returned index (if Ok) will mismatch on len() vs actual.
    }

    #[test]
    fn corrupt_absurd_point_count_with_short_payload() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("absurd2.hnsw");
        // GH02 header with large point count and only 2 bytes of payload.
        // bincode should fail to deserialize the HnswMap structure.
        let mut data = Vec::new();
        data.extend_from_slice(b"GH02");
        data.extend_from_slice(&1_000_000u64.to_le_bytes());
        data.extend_from_slice(&[0xAB, 0xCD]); // too little for valid HnswMap
        std::fs::write(&path, &data).unwrap();
        let result = HnswIndex::load(&path);
        assert!(
            result.is_err(),
            "large point count with 2 bytes payload should return Err"
        );
    }

    #[test]
    fn corrupt_legacy_format_garbage() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("legacy_bad.hnsw");
        // Data that does NOT start with GH02 — triggers legacy path.
        // Random bytes that are not valid bincode for HnswMap.
        let garbage = vec![0x42u8; 128];
        std::fs::write(&path, &garbage).unwrap();
        let result = HnswIndex::load(&path);
        assert!(
            result.is_err(),
            "garbage in legacy format path should return Err"
        );
    }

    #[test]
    fn corrupt_header_too_short() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("short_header.hnsw");
        // Starts with "GH02" but has less than 12 bytes total (magic + count).
        // Should fall through to legacy path (data.len() < 12).
        let mut data = Vec::new();
        data.extend_from_slice(b"GH02");
        data.extend_from_slice(&[0x01, 0x02]); // only 6 bytes total
        std::fs::write(&path, &data).unwrap();
        let result = HnswIndex::load(&path);
        assert!(
            result.is_err(),
            "GH02 header with <12 bytes should return Err"
        );
    }

    #[test]
    fn corrupt_nonexistent_file() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("does_not_exist.hnsw");
        let result = HnswIndex::load(&path);
        assert!(result.is_err(), "missing file should return Err");
    }

    #[test]
    fn corrupt_single_byte() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("one_byte.hnsw");
        std::fs::write(&path, [0x00]).unwrap();
        let result = HnswIndex::load(&path);
        assert!(result.is_err(), "single-byte file should return Err");
    }

    #[test]
    fn corrupt_valid_header_empty_bincode() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("empty_bincode.hnsw");
        // Valid GH02 header with 0 points but completely empty bincode section
        let mut data = Vec::new();
        data.extend_from_slice(b"GH02");
        data.extend_from_slice(&0u64.to_le_bytes());
        // No bincode data at all
        std::fs::write(&path, &data).unwrap();
        let result = HnswIndex::load(&path);
        assert!(
            result.is_err(),
            "valid header with empty bincode should return Err"
        );
    }

    #[test]
    fn header_count_mismatch_corrected() {
        // Build a valid index with 1 point
        let mut v0 = vec![0.0f32; 10];
        v0[0] = 1.0;
        let points = vec![EmbeddingPoint(v0)];
        let values = vec![42i64];
        let map = Builder::default().build(points, values);
        let index = HnswIndex {
            map,
            chunk_count: 1,
        };

        // Save it, then tamper with the header count
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("tampered.hnsw");
        index.save(&path).unwrap();

        let mut data = std::fs::read(&path).unwrap();
        // Overwrite header count to 999 (wrong)
        data[4..12].copy_from_slice(&999u64.to_le_bytes());
        std::fs::write(&path, &data).unwrap();

        // Load should succeed but correct the count
        let loaded = HnswIndex::load(&path).unwrap();
        assert_eq!(loaded.len(), 1, "should use actual count, not header count");
    }
}
