#!/usr/bin/env bash
set -euo pipefail

OUT=perf/out
BASELINE=perf/baseline/bench.txt

mkdir -p "$OUT"

# test/scale runs the compiled binary via test/golden's harness, which expects
# ./cub-scout at the repo root and fails (rather than skips) when it is missing.
echo "=== Building cub-scout ==="
go build ./cmd/cub-scout

echo ""
echo "=== CI gate tests (must pass) ==="
go test ./test/fixtures/patterns/ -v -timeout 60s
go test ./test/scale/ -v -timeout 120s
go test ./pkg/agent/ -run 'TestAttributionGraphBuild_1000|TestAttributionGraphBuild_2000|TestOwnershipDetection_1000|TestOwnershipDetection_2000' -v
go test ./cmd/cub-scout/ -run 'TestTUIRender_|TestTUIMemory_' -v -timeout 60s

echo ""
echo "=== Running benchmarks… ==="
go test -run=^$ -bench=. -benchmem ./... | tee "$OUT/bench.txt"

echo ""
echo "=== Baseline comparison ==="
if [[ ! -f "$BASELINE" ]]; then
  echo "No baseline found; skipping comparison."
  echo "To create a baseline: cp $OUT/bench.txt $BASELINE"
elif ! command -v benchstat >/dev/null 2>&1; then
  # benchstat is optional: perf testing is non-blocking, so a missing tool
  # skips the comparison rather than failing a run that already has results.
  echo "benchstat not found on PATH; skipping comparison."
  echo "To install benchstat: go install golang.org/x/perf/cmd/benchstat@latest"
else
  echo "Comparing against baseline…"
  benchstat "$BASELINE" "$OUT/bench.txt" | tee "$OUT/benchstat.txt"
fi
