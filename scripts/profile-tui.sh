#!/usr/bin/env bash
# Generate TUI performance profiles (CPU + heap).
#
# Usage:
#   ./scripts/profile-tui.sh
#
# Output:
#   tui_cpu.prof  — CPU profile (analyze with: go tool pprof tui_cpu.prof)
#   tui_heap.prof — Heap profile (analyze with: go tool pprof tui_heap.prof)
set -euo pipefail

cd "$(dirname "$0")/.."

echo "=== Generating TUI profiles (1000 resources) ==="
CUB_SCOUT_PROFILE=1 go test -run 'TestTUIProfile' ./cmd/cub-scout/ -timeout 60s -v

echo ""
echo "=== Profiles generated ==="
echo "  CPU:  go tool pprof tui_cpu.prof"
echo "  Heap: go tool pprof tui_heap.prof"
