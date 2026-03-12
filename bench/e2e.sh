#!/usr/bin/env bash
# End-to-end CLI wall-clock benchmarks for Grasshopper.
# Measures real wall-clock including process startup and model loading.
#
# Usage: bash bench/e2e.sh [grasshopper_binary]

set -euo pipefail

BINARY="${1:-grasshopper}"
RESULTS_DIR="bench/results"
RESULTS_FILE="$RESULTS_DIR/e2e.json"
SCRATCH=$(mktemp -d)
trap 'rm -rf "$SCRATCH"' EXIT

mkdir -p "$RESULTS_DIR"

# Check binary exists
if ! command -v "$BINARY" &>/dev/null; then
    echo "Error: $BINARY not found in PATH" >&2
    echo "Usage: bash bench/e2e.sh [/path/to/grasshopper]" >&2
    exit 1
fi

echo "=== Grasshopper E2E CLI Benchmarks ==="
echo "Binary: $(which "$BINARY")"
echo "Version: $("$BINARY" --help 2>&1 | head -1 || echo 'unknown')"
echo ""

# Helper: time a command and return milliseconds.
# Exit code 1 = no results (grep convention, acceptable).
# Any other non-zero exit aborts the benchmark.
time_ms() {
    local start end exit_code
    start=$(date +%s%N)
    # Disable set -e for this command so we can capture the exit code
    set +e
    "$@" >/dev/null 2>&1
    exit_code=$?
    set -e
    end=$(date +%s%N)
    if [ "$exit_code" -ne 0 ] && [ "$exit_code" -ne 1 ]; then
        echo "FATAL: command failed with exit code $exit_code: $*" >&2
        exit 1
    fi
    echo $(( (end - start) / 1000000 ))
}

# Helper: median of array
median() {
    printf '%s\n' "$@" | sort -n | awk '{a[NR]=$1} END{if(NR%2==1)print a[(NR+1)/2];else print (a[NR/2]+a[NR/2+1])/2}'
}

results='{"benchmarks":[]}'

add_result() {
    local name="$1" value="$2" unit="$3"
    results=$(echo "$results" | python3 -c "
import json,sys
d=json.load(sys.stdin)
d['benchmarks'].append({'name':'$name','value':$value,'unit':'$unit'})
json.dump(d,sys.stdout)
" 2>/dev/null || echo "$results")
    printf "  %-40s %s %s\n" "$name" "$value" "$unit"
}

# --- Index (FTS only) ---
echo "--- Index (FTS only) ---"
ms=$(time_ms "$BINARY" --db "$SCRATCH/bench.db" index ./src)
add_result "index_fts_only" "$ms" "ms"

# --- Index (with embeddings) ---
echo "--- Index (with embeddings) ---"
rm -f "$SCRATCH/bench2.db" "$SCRATCH/bench2.hnsw"
ms=$(time_ms "$BINARY" --db "$SCRATCH/bench2.db" index ./src --embed)
add_result "index_with_embed" "$ms" "ms"

# --- Search queries (10 iterations each, capture median) ---
echo "--- Search queries (10 iterations) ---"
DB="$SCRATCH/bench2.db"

queries=(
    "how does search work"
    "database schema"
    "error handling"
    "embedding model"
    "MCP server"
)

for query in "${queries[@]}"; do
    times=()
    for i in $(seq 1 10); do
        ms=$(time_ms "$BINARY" --db "$DB" search "$query" --limit 5)
        times+=("$ms")
    done
    med=$(median "${times[@]}")
    slug=$(echo "$query" | tr ' ' '_' | tr -cd 'a-z_')
    add_result "search_${slug}" "$med" "ms_median"
done

# --- Store + dedup timing ---
echo "--- Store timing ---"
ms=$(time_ms "$BINARY" --db "$DB" store "This is a test memory for benchmarking purposes")
add_result "store_new" "$ms" "ms"

ms=$(time_ms "$BINARY" --db "$DB" store "This is a test memory for benchmarking purposes")
add_result "store_dedup" "$ms" "ms"

# --- Status command ---
echo "--- Status ---"
ms=$(time_ms "$BINARY" --db "$DB" status)
add_result "status" "$ms" "ms"

# --- Warm start (model already cached) ---
echo "--- Warm start ---"
warm_times=()
for i in $(seq 1 3); do
    ms=$(time_ms "$BINARY" --db "$DB" search "test query" --limit 1)
    warm_times+=("$ms")
done
warm_med=$(median "${warm_times[@]}")
add_result "search_warm" "$warm_med" "ms_median"

# Save results
echo ""
echo "$results" | python3 -m json.tool > "$RESULTS_FILE" 2>/dev/null || echo "$results" > "$RESULTS_FILE"
echo "Results saved to $RESULTS_FILE"
