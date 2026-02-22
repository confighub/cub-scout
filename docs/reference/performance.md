# Performance Testing

cub-scout uses Go benchmarks for regression detection.

Performance testing is:
- non-blocking
- artifact-based
- baseline-driven

No semantic guarantees depend on benchmark output.

## CI Gate Tests

These tests enforce performance budgets and will fail CI if exceeded:

| Test | Budget | Measured |
|------|--------|----------|
| `TestScaleScanFile_1000Resources` | < 10s | ~300ms |
| `TestScaleScanFile_2000Resources` | < 20s | ~170ms |
| `TestAttributionGraphBuild_1000Nodes_Within3s` | < 3s | ~9ms |
| `TestAttributionGraphBuild_2000Nodes_Within5s` | < 5s | ~7ms |
| `TestOwnershipDetection_1000_Within2s` | < 2s | ~1ms |
| `TestOwnershipDetection_2000_Within3s` | < 3s | ~2ms |
| `TestTUIRender_Dashboard_500_Within3s` (100 renders) | < 3s | ~44ms |
| `TestTUIRender_AllViews_500_Within3s` (100 renders each) | < 3s | ~24ms |
| `TestTUIMemory_500Resources_Under200MB` | < 200MB | ~4MB |
| `TestTUIMemory_1000Resources_Under500MB` | < 500MB | ~4MB |

Measured values are from local development (Apple Silicon). CI runners may differ.

## Running Benchmarks

```bash
# Full benchmark suite
./scripts/perf-bench.sh

# Scale tests only
go test ./test/scale/ -v -timeout 120s

# Attribution benchmarks
go test -run=^$ -bench=. -benchmem ./pkg/agent/

# TUI render benchmarks
go test -run=^$ -bench=BenchmarkTUIView -benchmem ./cmd/cub-scout/

# CI gate tests only
go test ./test/scale/ ./pkg/agent/ ./cmd/cub-scout/ \
  -run 'TestScale|TestAttributionGraph|TestOwnershipDetection_\d|TestTUIRender_|TestTUIMemory_' \
  -v -timeout 120s
```

## Profiling

Generate CPU and heap profiles for the TUI at 1000 resources:

```bash
./scripts/profile-tui.sh

# Or manually:
CUB_SCOUT_PROFILE=1 go test -run 'TestTUIProfile' ./cmd/cub-scout/ -timeout 60s -v
go tool pprof tui_cpu.prof
go tool pprof tui_heap.prof
```
