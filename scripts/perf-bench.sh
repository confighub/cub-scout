#!/usr/bin/env bash
set -euo pipefail

OUT=perf/out
BASELINE=perf/baseline/bench.txt

mkdir -p "$OUT"

echo "Running benchmarks…"
go test -run=^$ -bench=. -benchmem ./... | tee "$OUT/bench.txt"

if [[ -f "$BASELINE" ]]; then
  echo "Comparing against baseline…"
  benchstat "$BASELINE" "$OUT/bench.txt" | tee "$OUT/benchstat.txt"
else
  echo "No baseline found; skipping comparison."
fi
