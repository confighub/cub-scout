// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
)

func TestExpandBindings_ArrayShape(t *testing.T) {
	raw := []byte(`[
		{"downstreamPath": ".spec.replicas", "upstreamPath": ".spec.scale.value"},
		{"downstreamPath": ".spec.template.spec.containers[0].image", "upstreamPath": ".spec.image", "transformExpr": "{{ . | trim }}"}
	]`)

	got := expandBindings(raw)
	if len(got) != 2 {
		t.Fatalf("expected 2 bindings; got %d", len(got))
	}
	if got[0].DownstreamPath != ".spec.replicas" || got[0].UpstreamPath != ".spec.scale.value" {
		t.Errorf("entry 0: %+v", got[0])
	}
	if got[1].TransformExpr != "{{ . | trim }}" {
		t.Errorf("entry 1 transform: %+v", got[1])
	}
}

func TestExpandBindings_ObjectShape(t *testing.T) {
	raw := []byte(`{
		".spec.replicas": {"upstreamPath": ".spec.scale.value"},
		".spec.image":    {"upstreamPath": ".spec.image", "transformExpr": "lower"}
	}`)

	got := expandBindings(raw)
	if len(got) != 2 {
		t.Fatalf("expected 2 bindings; got %d", len(got))
	}
	// Order isn't guaranteed for maps; check both entries are present.
	var seenReplicas, seenImage bool
	for _, b := range got {
		switch b.DownstreamPath {
		case ".spec.replicas":
			seenReplicas = true
			if b.UpstreamPath != ".spec.scale.value" {
				t.Errorf("replicas upstream wrong: %+v", b)
			}
		case ".spec.image":
			seenImage = true
			if b.TransformExpr != "lower" {
				t.Errorf("image transform wrong: %+v", b)
			}
		}
	}
	if !seenReplicas || !seenImage {
		t.Errorf("missing entries; got %+v", got)
	}
}

func TestExpandBindings_TargetSourceAliases(t *testing.T) {
	// Schema variant using target/source/transform naming.
	raw := []byte(`[
		{"target": ".spec.replicas", "source": ".spec.scale.value", "transform": "to_int"}
	]`)
	got := expandBindings(raw)
	if len(got) != 1 {
		t.Fatalf("expected 1 binding; got %d", len(got))
	}
	if got[0].DownstreamPath != ".spec.replicas" || got[0].UpstreamPath != ".spec.scale.value" {
		t.Errorf("alias normalization failed: %+v", got[0])
	}
	if got[0].TransformExpr != "to_int" {
		t.Errorf("transform alias failed: %+v", got[0])
	}
}

func TestExpandBindings_EmptyAndUnknownShapes(t *testing.T) {
	if got := expandBindings(nil); got != nil {
		t.Errorf("nil input → nil; got %v", got)
	}
	if got := expandBindings([]byte(`[]`)); got != nil {
		t.Errorf("empty array → nil; got %v", got)
	}
	if got := expandBindings([]byte(`"just a string"`)); got != nil {
		t.Errorf("string scalar → nil; got %v", got)
	}
}

func TestLookupFieldBindingSource(t *testing.T) {
	bindings := []IncomingBinding{
		{
			LinkID:   "L1",
			Slug:     "image-binding",
			ToUnitID: "upstream-A",
			Bindings: []FieldBinding{
				{DownstreamPath: ".spec.replicas", UpstreamPath: ".spec.scale.value"},
			},
		},
		{
			LinkID:   "L2",
			Slug:     "image-from-app",
			ToUnitID: "upstream-B",
			Bindings: []FieldBinding{
				{DownstreamPath: ".spec.template.spec.containers[0].image", UpstreamPath: ".spec.image", TransformExpr: "trim"},
			},
		},
	}

	t.Run("match by path", func(t *testing.T) {
		got := LookupFieldBindingSource(".spec.replicas", bindings)
		if got == nil {
			t.Fatal("expected non-nil")
		}
		if got.LinkID != "L1" || got.UpstreamUnitID != "upstream-A" {
			t.Errorf("wrong match: %+v", got)
		}
		if got.UpstreamPath != ".spec.scale.value" {
			t.Errorf("upstream path: %+v", got)
		}
	})

	t.Run("transform carried through", func(t *testing.T) {
		got := LookupFieldBindingSource(".spec.template.spec.containers[0].image", bindings)
		if got == nil || got.TransformExpr != "trim" {
			t.Errorf("transform not carried: %+v", got)
		}
	})

	t.Run("no match", func(t *testing.T) {
		if got := LookupFieldBindingSource(".spec.unused", bindings); got != nil {
			t.Errorf("expected nil; got %+v", got)
		}
	})

	t.Run("empty inputs", func(t *testing.T) {
		if got := LookupFieldBindingSource("", bindings); got != nil {
			t.Errorf("empty path → nil; got %+v", got)
		}
		if got := LookupFieldBindingSource(".spec.replicas", nil); got != nil {
			t.Errorf("nil bindings → nil; got %+v", got)
		}
	})
}

func TestParseLinkListJSON_ExpandsBindings(t *testing.T) {
	// End-to-end: list JSON → parsed IncomingBindings with expanded
	// FieldBinding entries.
	raw := []byte(`[
		{
			"LinkID": "L1",
			"Slug": "replicas-from-scale",
			"UpdateType": "NeedsProvides",
			"ToUnitID": "upstream-A",
			"Bindings": [
				{"downstreamPath": ".spec.replicas", "upstreamPath": ".spec.scale.value"}
			]
		}
	]`)

	got := parseLinkListJSON(raw)
	if len(got) != 1 {
		t.Fatalf("expected 1 link; got %d", len(got))
	}
	if got[0].BindingsCount != 1 {
		t.Errorf("count = %d, want 1", got[0].BindingsCount)
	}
	if len(got[0].Bindings) != 1 {
		t.Fatalf("expanded bindings = %d, want 1", len(got[0].Bindings))
	}
	if got[0].Bindings[0].DownstreamPath != ".spec.replicas" {
		t.Errorf("wrong downstream: %+v", got[0].Bindings[0])
	}
}

func TestRenderASCII_IncludesBindingSourceLine(t *testing.T) {
	result := compareResourceResult{
		Resource: "Deployment/api",
		Mode:     "dry-wet-live",
		Connected: true,
		Mismatches: []compareFieldMismatch{
			{
				Field: "replicas",
				Dry:   "3",
				Wet:   "3",
				Live:  "1",
				BindingSource: &FieldBindingSource{
					LinkID:         "L1",
					LinkSlug:       "replicas-from-scale",
					UpstreamUnitID: "upstream-A",
					UpstreamPath:   ".spec.scale.value",
				},
			},
		},
	}
	out := renderCompareResourceASCII(result)
	if !strings.Contains(out, "<- bound from unit:upstream-A path:.spec.scale.value via link:replicas-from-scale") {
		t.Errorf("expected binding line; got:\n%s", out)
	}
}
