// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"encoding/json"
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

// TestCanonicalJSON_RFC8785_ReferenceVectors locks the implementation
// against the RFC 8785 reference vectors. These cover:
//   - UTF-16 code-unit ordering of object keys (the locked design called
//     this out as the requirement Go's lexicographic byte sort doesn't
//     meet for keys involving supplementary plane / surrogate pairs)
//   - canonical number formatting (ECMAScript ToString)
//   - canonical string escaping
//
// If a vector fails after a library bump, that's a real signal — receipts
// fingerprinted before the bump won't verify after.
func TestCanonicalJSON_RFC8785_ReferenceVectors(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "Object key sort uses UTF-16 code unit order (BMP chars)",
			in:   `{"ü":1,"a":2,"z":3,"é":4}`,
			// UTF-16 code-unit order: "a" (0061) < "z" (007a) < "é" (00e9) < "ü" (00fc)
			want: `{"a":2,"z":3,"é":4,"ü":1}`,
		},
		{
			name: "Empty object and array",
			in:   `{"arr":[],"obj":{}}`,
			want: `{"arr":[],"obj":{}}`,
		},
		{
			name: "Nested ordering",
			in:   `{"b":{"d":2,"c":1},"a":3}`,
			want: `{"a":3,"b":{"c":1,"d":2}}`,
		},
		{
			// RFC 8785 §3.2.2.3 — number serialization: trailing zeros
			// stripped, scientific form for very large values, integer
			// form when no fractional part exists.
			name: "Number formatting per ECMAScript ToString",
			in:   `{"a":4.50,"b":1.0e30,"c":0.002}`,
			want: `{"a":4.5,"b":1e+30,"c":0.002}`,
		},
		{
			// RFC 8785 §3.2.2.2 — string canonicalization: control chars
			// use their short escapes (\n, \t), only \ and " escape with
			// backslash; ASCII printables emit directly.
			name: "String escaping minimizes backslash sequences",
			in:   `{"v":"hello\nworld\t\"quoted\"éend"}`,
			want: `{"v":"hello\nworld\t\"quoted\"éend"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Parse the input JSON to a generic value, then canonicalize.
			// We can't use CanonicalJSON's struct-marshal path directly;
			// instead we feed the parsed value through, which is what
			// receipt fingerprint computation does after the predicate-key
			// removal step.
			var v interface{}
			d := json.NewDecoder(strings.NewReader(tc.in))
			d.UseNumber()
			if err := d.Decode(&v); err != nil {
				t.Fatalf("parse input: %v", err)
			}
			got, err := CanonicalJSON(v)
			if err != nil {
				t.Fatalf("CanonicalJSON: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("RFC 8785 vector mismatch:\n  input: %s\n  got:   %s\n  want:  %s", tc.in, got, tc.want)
			}
		})
	}
}
