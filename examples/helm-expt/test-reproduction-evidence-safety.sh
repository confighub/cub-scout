#!/usr/bin/env bash
# Verify the optional reproduction script refuses to overwrite a caller's evidence.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

EVIDENCE_DIR="$TMP_DIR/evidence"
mkdir "$EVIDENCE_DIR"
printf 'preserve-existing-evidence\n' >"$EVIDENCE_DIR/sentinel.txt"

if "$SCRIPT_DIR/reproduce-helm-release-secret-trace.sh" "$EVIDENCE_DIR" >"$TMP_DIR/stdout" 2>"$TMP_DIR/stderr"; then
	echo "expected reproduction script to refuse non-empty evidence directory" >&2
	exit 1
fi
grep -Fq "refusing to overwrite existing evidence" "$TMP_DIR/stderr"
test "$(cat "$EVIDENCE_DIR/sentinel.txt")" = "preserve-existing-evidence"
test "$(find "$EVIDENCE_DIR" -mindepth 1 -maxdepth 1 -type f | wc -l | tr -d ' ')" = 1
