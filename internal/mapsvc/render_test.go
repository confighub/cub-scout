// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package mapsvc

import (
	"strings"
	"testing"
)

func TestFormatOwnershipTree_Basic(t *testing.T) {
	entries := []Entry{
		{Name: "frontend", Namespace: "web", Kind: "Deployment", Owner: "Flux"},
		{Name: "backend", Namespace: "api", Kind: "Deployment", Owner: "Flux"},
		{Name: "postgres", Namespace: "db", Kind: "StatefulSet", Owner: "ArgoCD"},
		{Name: "cronjob", Namespace: "jobs", Kind: "Deployment", Owner: "Native"},
	}

	opts := OwnershipTreeOpts{}
	got := FormatOwnershipTree(entries, opts)

	// Should have proper tree connectors
	if !strings.Contains(got, "├──") {
		t.Error("expected ├── connector for non-last items")
	}
	if !strings.Contains(got, "└──") {
		t.Error("expected └── connector for last items")
	}

	// Should be sorted by namespace/name
	if !strings.Contains(got, "api/backend") {
		t.Error("expected api/backend")
	}

	// Should have owner headers
	if !strings.Contains(got, "Flux (2)") {
		t.Error("expected Flux (2) header")
	}
}

func TestFormatOwnershipTree_FilterOwner(t *testing.T) {
	entries := []Entry{
		{Name: "frontend", Namespace: "web", Kind: "Deployment", Owner: "Flux"},
		{Name: "backend", Namespace: "api", Kind: "Deployment", Owner: "Flux"},
		{Name: "postgres", Namespace: "db", Kind: "StatefulSet", Owner: "ArgoCD"},
		{Name: "cronjob", Namespace: "jobs", Kind: "Deployment", Owner: "Native"},
	}

	opts := OwnershipTreeOpts{FilterOwner: "Flux"}
	got := FormatOwnershipTree(entries, opts)

	// Should only show Flux
	if !strings.Contains(got, "Flux (2)") {
		t.Error("expected Flux (2) header")
	}
	if strings.Contains(got, "ArgoCD") {
		t.Error("should not contain ArgoCD when filtering by Flux")
	}
	if strings.Contains(got, "Native") {
		t.Error("should not contain Native when filtering by Flux")
	}
}

func TestFormatOwnershipTree_MaxDepth(t *testing.T) {
	entries := []Entry{
		{Name: "app1", Namespace: "ns", Kind: "Deployment", Owner: "Flux"},
		{Name: "app2", Namespace: "ns", Kind: "Deployment", Owner: "Flux"},
		{Name: "app3", Namespace: "ns", Kind: "Deployment", Owner: "Flux"},
		{Name: "app4", Namespace: "ns", Kind: "Deployment", Owner: "Flux"},
		{Name: "app5", Namespace: "ns", Kind: "Deployment", Owner: "Flux"},
	}

	opts := OwnershipTreeOpts{MaxDepth: 2}
	got := FormatOwnershipTree(entries, opts)

	// Should show total count in header
	if !strings.Contains(got, "Flux (5)") {
		t.Error("expected Flux (5) header showing total count")
	}

	// Should show first 2 items
	if !strings.Contains(got, "ns/app1") {
		t.Error("expected app1 (within depth)")
	}
	if !strings.Contains(got, "ns/app2") {
		t.Error("expected app2 (within depth)")
	}

	// Should show "(+3 more)" indicator
	if !strings.Contains(got, "(+3 more)") {
		t.Errorf("expected (+3 more) indicator, got:\n%s", got)
	}

	// Should NOT show app3, app4, app5 individually
	if strings.Contains(got, "ns/app3") {
		t.Error("should not show app3 (beyond depth)")
	}
}

func TestFormatOwnershipTree_FilterOwnerAndDepth(t *testing.T) {
	entries := []Entry{
		{Name: "app1", Namespace: "ns", Kind: "Deployment", Owner: "Flux"},
		{Name: "app2", Namespace: "ns", Kind: "Deployment", Owner: "Flux"},
		{Name: "app3", Namespace: "ns", Kind: "Deployment", Owner: "Flux"},
		{Name: "db", Namespace: "ns", Kind: "Deployment", Owner: "ArgoCD"},
	}

	opts := OwnershipTreeOpts{FilterOwner: "Flux", MaxDepth: 2}
	got := FormatOwnershipTree(entries, opts)

	// Should show Flux with total count
	if !strings.Contains(got, "Flux (3)") {
		t.Error("expected Flux (3) header")
	}

	// Should show "(+1 more)" for Flux
	if !strings.Contains(got, "(+1 more)") {
		t.Errorf("expected (+1 more) indicator, got:\n%s", got)
	}

	// Should NOT show ArgoCD at all
	if strings.Contains(got, "ArgoCD") {
		t.Error("should not contain ArgoCD when filtering by Flux")
	}
}
