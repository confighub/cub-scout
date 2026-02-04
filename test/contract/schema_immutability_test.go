// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package contract provides contract tests for locked schemas.
//
// These tests enforce schema immutability for v0.16+ locked schemas.
// Any change to a locked schema struct (fields, types, JSON tags) will
// fail this test. To intentionally change a schema, you must:
//   1. Bump the schema version
//   2. Create new snapshot files
//   3. Update the schema version constant
//
// Contract: Locked schemas are immutable. Silent meaning changes are forbidden.
package contract

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
)

type schemaTarget struct {
	Name string
	Type any
	File string
}

// TestLockedSchemas_ImmutabilitySnapshot verifies that locked schema structs
// have not changed. This is a contract enforcement test.
func TestLockedSchemas_ImmutabilitySnapshot(t *testing.T) {
	targets := []schemaTarget{
		{
			Name: "attribution-graph.v1",
			Type: agent.AttributionGraph{},
			File: "testdata/schema/attribution-graph.v1.snapshot.txt",
		},
		{
			Name: "attribution-report.v1",
			Type: agent.AttributionReport{},
			File: "testdata/schema/attribution-report.v1.snapshot.txt",
		},
	}

	for _, tt := range targets {
		t.Run(tt.Name, func(t *testing.T) {
			got := snapshotStruct(tt.Type)

			snapshotPath := filepath.Join(testDir(t), tt.File)
			wantBytes, err := os.ReadFile(snapshotPath)
			if err != nil {
				t.Fatalf("missing snapshot %q: %v\n\nRun: ./scripts/contract-update.sh", tt.File, err)
			}
			want := string(wantBytes)

			if got != want {
				t.Fatalf(
					"LOCKED SCHEMA CHANGED for %s\n\n--- want ---\n%s\n--- got ---\n%s\n\n"+
						"If intentional, bump schema version and create a new schema (no silent meaning changes).",
					tt.Name, want, got,
				)
			}
		})
	}
}

// snapshotStruct produces a deterministic text representation of a struct's
// exported fields, including JSON tags. This captures the "shape" of the schema.
func snapshotStruct(v any) string {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	var lines []string
	walkType(t, t.Name(), &lines)

	sort.Strings(lines)
	return strings.Join(lines, "\n") + "\n"
}

// walkType recursively walks a struct type and collects field information.
func walkType(t reflect.Type, prefix string, out *[]string) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)

		// Skip unexported fields
		if f.PkgPath != "" {
			continue
		}

		jsonTag := f.Tag.Get("json")
		ft := f.Type

		line := fmt.Sprintf("%s.%s json=%q type=%s", prefix, f.Name, jsonTag, ft.String())
		*out = append(*out, line)

		// Recurse into nested structs that are in the same package
		n := ft
		if n.Kind() == reflect.Pointer {
			n = n.Elem()
		}
		if n.Kind() == reflect.Slice {
			n = n.Elem()
			if n.Kind() == reflect.Pointer {
				n = n.Elem()
			}
		}
		// Only recurse into structs from the agent package (same schema)
		if n.Kind() == reflect.Struct && strings.Contains(n.PkgPath(), "cub-scout/pkg/agent") {
			walkType(n, prefix+"."+f.Name+"[]", out)
		}
	}
}

// testDir returns the directory containing this test file.
func testDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test directory")
	}
	return filepath.Dir(filename)
}
