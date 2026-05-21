// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"strings"
	"testing"
)

func TestCanonicalJSON_SortsObjectKeys(t *testing.T) {
	in := map[string]interface{}{
		"b": 1,
		"a": 2,
		"c": 3,
	}
	got, err := CanonicalJSON(in)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	want := `{"a":2,"b":1,"c":3}`
	if string(got) != want {
		t.Errorf("CanonicalJSON sorting failed: got %q, want %q", string(got), want)
	}
}

func TestCanonicalJSON_NestedSorting(t *testing.T) {
	in := map[string]interface{}{
		"b": map[string]interface{}{
			"d": 1,
			"c": 2,
		},
		"a": 3,
	}
	got, err := CanonicalJSON(in)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	want := `{"a":3,"b":{"c":2,"d":1}}`
	if string(got) != want {
		t.Errorf("nested sort failed: got %q, want %q", string(got), want)
	}
}

func TestCanonicalJSON_StructFieldsSortedByJSONTag(t *testing.T) {
	// Structs are round-tripped through generic maps so json-tag-named
	// fields end up sorted alphabetically. This is the key property
	// fingerprint stability depends on.
	type Inner struct {
		Z string `json:"z"`
		A string `json:"a"`
	}
	type Outer struct {
		Inner Inner  `json:"inner"`
		Top   string `json:"top"`
	}

	got, err := CanonicalJSON(Outer{
		Inner: Inner{Z: "last", A: "first"},
		Top:   "outer",
	})
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	want := `{"inner":{"a":"first","z":"last"},"top":"outer"}`
	if string(got) != want {
		t.Errorf("struct json-tag sort failed: got %q, want %q", string(got), want)
	}
}

func TestCanonicalJSON_NoTrailingNewline(t *testing.T) {
	got, err := CanonicalJSON(map[string]int{"a": 1})
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	if strings.HasSuffix(string(got), "\n") {
		t.Errorf("trailing newline must be stripped; got %q", string(got))
	}
}

func TestCanonicalJSON_NoExtraWhitespace(t *testing.T) {
	got, err := CanonicalJSON(map[string]interface{}{"a": 1, "b": []int{2, 3}})
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	for _, ch := range string(got) {
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			t.Errorf("compact output must not contain whitespace; got %q", string(got))
			break
		}
	}
}

func TestCanonicalJSON_DeterministicAcrossRuns(t *testing.T) {
	in := map[string]interface{}{
		"timestamp": "2026-05-21T10:30:00Z",
		"verifier":  map[string]string{"tool": "cub-scout", "version": "v1"},
		"verdict":   "PASS",
		"omissions": []interface{}{},
	}
	first, err := CanonicalJSON(in)
	if err != nil {
		t.Fatalf("first marshal: %v", err)
	}
	for i := 0; i < 20; i++ {
		next, err := CanonicalJSON(in)
		if err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
		if string(next) != string(first) {
			t.Errorf("non-deterministic: iter %d differs from first; first=%q next=%q", i, first, next)
		}
	}
}

func TestCanonicalJSON_HandlesEmptyContainers(t *testing.T) {
	got, err := CanonicalJSON(map[string]interface{}{
		"emptyArr": []interface{}{},
		"emptyObj": map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	want := `{"emptyArr":[],"emptyObj":{}}`
	if string(got) != want {
		t.Errorf("empty containers: got %q, want %q", string(got), want)
	}
}

func TestCanonicalJSON_PreservesIntegerPrecision(t *testing.T) {
	// Receipt revisions are integers that must round-trip lossy-free
	// through canonical-JSON. Verify a 64-bit boundary value survives.
	got, err := CanonicalJSON(map[string]interface{}{"rev": int64(9007199254740991)})
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	if !strings.Contains(string(got), `"rev":9007199254740991`) {
		t.Errorf("integer precision lost: got %q", string(got))
	}
}

func TestCanonicalJSON_RejectsUnencodable(t *testing.T) {
	// Channels and functions are unmarshalable; json.Marshal errors.
	_, err := CanonicalJSON(make(chan int))
	if err == nil {
		t.Error("expected error for unencodable input; got nil")
	}
}
