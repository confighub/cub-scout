#!/usr/bin/env bash
# Prepare a PATH for running eval cases live, against a real cluster, instead of
# against recordings (`claude plugin eval ... --mocks off`).
#
# The harness gives plugin MCP servers a sandboxed HOME and does not pass
# KUBECONFIG through; PATH does pass. This writes a directory holding a
# minified kubeconfig for one context and a `cub-scout` wrapper that sets
# KUBECONFIG and runs the given binary, and prints a PATH with that directory
# first and `cub` left out, so the server is standalone.
#
# Usage: PATH="$(evals/scripts/live-path.sh <kube-context> [cub-scout-binary])" claude plugin eval ...
set -euo pipefail
ctx="${1:?usage: live-path.sh <kube-context> [cub-scout-binary]}"
repo="$(cd "$(dirname "$0")/../.." && pwd)"
bin="$(cd "$(dirname "${2:-$repo/cub-scout}")" && pwd)/$(basename "${2:-$repo/cub-scout}")"
[ -x "$bin" ] || { echo "no cub-scout binary at $bin; run go build ./cmd/cub-scout" >&2; exit 1; }
dir="$(mktemp -d "${TMPDIR:-/tmp}/cub-scout-live.XXXXXX")"
kubectl config view --minify --flatten --context "$ctx" > "$dir/kubeconfig"
cat > "$dir/cub-scout" <<WRAPPER
#!/bin/sh
KUBECONFIG="$dir/kubeconfig" exec "$bin" "\$@"
WRAPPER
chmod +x "$dir/cub-scout"
keep="$dir"
IFS=: read -r -a parts <<< "$PATH"
for d in "${parts[@]}"; do
  [ -x "$d/cub" ] && continue
  keep="$keep:$d"
done
echo "$keep"
