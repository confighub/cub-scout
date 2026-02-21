#!/usr/bin/env bash
set -euo pipefail

OUT=perf/out
BASELINE=perf/baseline/bench.txt

mkdir -p "$OUT"

echo "=== CI gate tests (must pass) ==="
go test ./test/fixtures/patterns/ -v -timeout 60s
go test ./test/scale/ -v -timeout 120s
go test ./pkg/agent/ -run 'TestAttributionGraphBuild_1000|TestAttributionGraphBuild_2000|TestOwnershipDetection_1000|TestOwnershipDetection_2000' -v
go test ./cmd/cub-scout/ -run 'TestTUIRender_|TestTUIMemory_' -v -timeout 60s

echo ""
echo "=== Running benchmarks… ==="
go test -run=^$ -bench=. -benchmem ./... | tee "$OUT/bench.txt"

if [[ -f "$BASELINE" ]]; then
  echo "Comparing against baseline…"
  benchstat "$BASELINE" "$OUT/bench.txt" | tee "$OUT/benchstat.txt"
else
  echo "No baseline found; skipping comparison."
  echo "To create a baseline: cp $OUT/bench.txt $BASELINE"
fi
