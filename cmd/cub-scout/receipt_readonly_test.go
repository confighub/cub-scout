// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReceiptPackageReadOnlyClient enforces the receipt invariant from
// #410/#428 and #446: the receipt code path MUST NOT call any mutating
// K8s client method. Receipts EMIT artifacts; they never write to a
// cluster, ConfigHub, or any external store.
//
// The check is intentionally a source-level grep rather than a runtime
// guard — it's a static contract that adding a mutating call to the
// receipt package will fail in CI. Future receipt code (batch 2, batch
// 3, source-truth-pass / no-manual-edits-since predicates) inherits this
// guard automatically.
//
// Scope (any source file whose basename CONTAINS "receipt", in either
// the cmd/cub-scout or pkg/agent tree):
//   - cmd/cub-scout/receipt*.go
//   - cmd/cub-scout/receipt_render*.go
//   - cmd/cub-scout/watch_receipt*.go (#449 v2: the watch-emit path
//     builds receipts in-process; same trust invariant applies)
//   - pkg/agent/receipt*.go
//
// The glob is `*receipt*` (substring), not `receipt*` (prefix). Earlier
// versions of this test used a prefix and silently missed
// `watch_receipt.go` — Codex round-6 catch (PR #463). Future files
// whose names embed "receipt" anywhere (e.g., `aggregate_receipt.go`,
// `receipt_chain.go`) are covered automatically.
//
// Forbidden tokens (each is a substring check; word-boundary not needed
// because the goal is "no mutating verb appears anywhere in this code"):
//
//	.Create(
//	.Update(
//	.UpdateStatus(
//	.Patch(
//	.Apply(
//	.ApplyStatus(
//	.Delete(
//	.DeleteCollection(
//
// .Watch( is NOT forbidden — Watch is a read-only API on K8s.
// .Get( and .List( are allowed.
//
// expectedFiles is a small allow-set the test asserts on at the end:
// if any listed file is NOT scanned, the test fails loudly rather than
// silently passing. This guards against future renames or basename
// changes that would silently drop a file from the guard. Adding a new
// receipt-shaped file means adding its basename here.
func TestReceiptPackageReadOnlyClient(t *testing.T) {
	forbidden := []string{
		".Create(",
		".Update(",
		".UpdateStatus(",
		".Patch(",
		".Apply(",
		".ApplyStatus(",
		".Delete(",
		".DeleteCollection(",
	}

	// Source roots to scan. Resolve relative to the test file; we are in
	// cmd/cub-scout/ so . is cmd/cub-scout/ and ../../pkg/agent/ is the
	// agent package.
	roots := []string{
		".",          // cmd/cub-scout
		"../../pkg/agent",
	}

	// expectedPaths names the receipt-shaped files this guard MUST scan,
	// keyed by `<root>/<basename>` so basename collisions across roots
	// (e.g. cmd/cub-scout/receipt.go and pkg/agent/receipt.go are
	// different files) are tracked independently. Mismatch (path
	// present in tree but skipped, or listed here but missing from
	// tree) fails the test. Keeps the glob honest as new files land.
	expectedPaths := map[string]bool{
		"cmd/cub-scout/receipt.go":           false,
		"cmd/cub-scout/receipt_render.go":    false,
		"cmd/cub-scout/receipt_show.go":      false,
		"cmd/cub-scout/receipt_validate.go":  false,
		"cmd/cub-scout/receipt_list.go":      false,
		"cmd/cub-scout/watch_receipt.go":     false,
		"pkg/agent/receipt.go":                       false,
		"pkg/agent/receipt_build.go":                 false,
		"pkg/agent/receipt_canonicaljson.go":         false,
		"pkg/agent/receipt_fingerprint.go":           false,
		"pkg/agent/receipt_inputattestations.go":     false,
		"pkg/agent/receipt_predicates.go":            false,
		"pkg/agent/receipt_store.go":                 false,
		"pkg/agent/receipt_subject.go":               false,
	}
	rootDisplay := map[string]string{
		".":               "cmd/cub-scout",
		"../../pkg/agent": "pkg/agent",
	}

	scannedPaths := map[string]bool{}
	var scanned int
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatalf("read %s: %v", root, err)
		}
		for _, e := range entries {
			name := e.Name()
			// Substring-match (not prefix). Catches `watch_receipt.go`,
			// `aggregate_receipt.go`, etc. — anything with "receipt" in
			// the basename.
			if !strings.Contains(name, "receipt") {
				continue
			}
			if !strings.HasSuffix(name, ".go") {
				continue
			}
			// Skip test files themselves — tests legitimately reference
			// the names in strings (this file is the most obvious case).
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			path := filepath.Join(root, name)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			src := string(data)
			for _, frag := range forbidden {
				if strings.Contains(src, frag) {
					t.Errorf("%s contains forbidden mutating call fragment %q; receipt code is read-only by design (#410/#428/#446)", path, frag)
				}
			}
			scannedPaths[rootDisplay[root]+"/"+name] = true
			scanned++
		}
	}

	// Defensive: if we didn't actually scan any receipt files, the test
	// is silently useless. Catch that.
	if scanned == 0 {
		t.Fatal("no receipt*.go source files scanned; test would silently pass — check roots")
	}

	// Stronger guard: every file in the expected allow-set must have
	// been scanned. A skipped file means the glob silently dropped it.
	// Codex round-6 caught the prior glob (`strings.HasPrefix(name,
	// "receipt")`) silently missing watch_receipt.go; this assertion
	// makes that class of bug loud rather than silent.
	for path := range expectedPaths {
		if !scannedPaths[path] {
			t.Errorf("expected to scan %q under the read-only guard but it was not visited; either the file was renamed or the glob is broken", path)
		}
	}
}
