// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package unit

import (
	"go/build"
	"path/filepath"
	"runtime"
	"testing"
)

// Mutating suites must never enter the default test run through ambient credentials.
func TestLiveGoldenSuitesRequireIntegrationOptIn(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate repository")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..")
	for _, rel := range []string{
		"test/golden/map-list-json/map_list_json_test.go",
		"test/golden/map-status/map_status_test.go",
		"test/golden/map-deployers-json/deployers_json_test.go",
	} {
		t.Run(rel, func(t *testing.T) {
			for _, live := range []bool{false, true} {
				ctx := build.Default
				ctx.BuildTags = nil
				if live {
					ctx.BuildTags = []string{"integration"}
				}
				path := filepath.Join(root, rel)
				matched, err := ctx.MatchFile(filepath.Dir(path), filepath.Base(path))
				if err != nil {
					t.Fatal(err)
				}
				if matched != live {
					t.Fatalf("integration=%v: selected=%v", live, matched)
				}
			}
		})
	}
}
