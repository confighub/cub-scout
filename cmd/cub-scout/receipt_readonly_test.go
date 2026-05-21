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
// Scope:
//   - cmd/cub-scout/receipt*.go and cmd/cub-scout/receipt_render*.go
//   - pkg/agent/receipt*.go
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

	var scanned int
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatalf("read %s: %v", root, err)
		}
		for _, e := range entries {
			name := e.Name()
			if !strings.HasPrefix(name, "receipt") {
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
			scanned++
		}
	}

	// Defensive: if we didn't actually scan any receipt files, the test
	// is silently useless. Catch that.
	if scanned == 0 {
		t.Fatal("no receipt*.go source files scanned; test would silently pass — check roots")
	}
}
