// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestClassifyTemplatedSource(t *testing.T) {
	cases := []struct {
		name  string
		chain []ChainLink
		want  string
	}{
		{"helm chart root", []ChainLink{{Kind: "HelmChart", URL: "https://repo"}}, GitSourceTypeHelm},
		{"helm release non-root", []ChainLink{{Kind: "GitRepository", URL: "g"}, {Kind: "HelmRelease"}}, GitSourceTypeHelm},
		{"kustomization non-root", []ChainLink{{Kind: "GitRepository", URL: "g"}, {Kind: "Kustomization", Path: "overlays/prod"}}, GitSourceTypeKustomize},
		{"raw git", []ChainLink{{Kind: "GitRepository", URL: "g"}}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st, res := classifyTemplatedSource(tc.chain)
			if st != tc.want {
				t.Fatalf("sourceType = %q, want %q", st, tc.want)
			}
			if tc.want == "" {
				if res != "" {
					t.Fatalf("raw must have empty resolution, got %q", res)
				}
			} else if res != GitSourceTemplatedNotResolved {
				t.Fatalf("templated must carry the honesty marker, got %q", res)
			}
		})
	}
}

func TestAnchorWithTemplatedSource_HelmMarker(t *testing.T) {
	anchor := anchorWithTemplatedSource([]ChainLink{{Kind: "HelmChart", URL: "https://charts.example.com", Revision: "1.2.3"}})
	if anchor == nil {
		t.Fatal("anchor is nil")
	}
	if anchor.SourceType != GitSourceTypeHelm || anchor.Resolution != GitSourceTemplatedNotResolved {
		t.Fatalf("sourceType=%q resolution=%q", anchor.SourceType, anchor.Resolution)
	}
	b, _ := json.Marshal(anchor)
	if !strings.Contains(string(b), `"sourceType":"helm"`) || !strings.Contains(string(b), `"resolution":"templated-source-not-resolved"`) {
		t.Fatalf("JSON missing the honesty marker: %s", b)
	}
}

// Raw sources must stay byte-identical (omitempty) so existing provenance
// output / golden tests are unaffected.
func TestAnchorWithTemplatedSource_RawUnmarked(t *testing.T) {
	anchor := anchorWithTemplatedSource([]ChainLink{{Kind: "GitRepository", URL: "https://github.com/x/y", Revision: "abc", Path: "manifests"}})
	if anchor == nil {
		t.Fatal("anchor is nil")
	}
	if anchor.SourceType != "" || anchor.Resolution != "" {
		t.Fatalf("raw must be unmarked, got sourceType=%q resolution=%q", anchor.SourceType, anchor.Resolution)
	}
	b, _ := json.Marshal(anchor)
	if strings.Contains(string(b), "sourceType") || strings.Contains(string(b), "resolution") {
		t.Fatalf("raw anchor must not serialize the new fields: %s", b)
	}
}
