// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package mapsvc

import (
	"encoding/json"
	"testing"
)

func TestBuildOwnershipTreeJSON_Basic(t *testing.T) {
	entries := []Entry{
		{Kind: "Deployment", Namespace: "web", Name: "frontend", Owner: "Flux", OwnerDetails: map[string]string{"name": "kustomization/web-apps"}},
		{Kind: "Deployment", Namespace: "api", Name: "backend", Owner: "Flux", OwnerDetails: map[string]string{"name": "kustomization/api-apps"}},
		{Kind: "Deployment", Namespace: "default", Name: "legacy", Owner: "Native"},
	}

	opts := OwnershipTreeOpts{}
	output := BuildOwnershipTreeJSON(entries, opts, "test-cluster", nil)

	// Verify structure
	if output.Command != "tree" {
		t.Errorf("expected command=tree, got %s", output.Command)
	}
	if output.Subtype != "ownership" {
		t.Errorf("expected subtype=ownership, got %s", output.Subtype)
	}
	if output.Context.Cluster != "test-cluster" {
		t.Errorf("expected cluster=test-cluster, got %s", output.Context.Cluster)
	}

	// Verify groups are in canonical owner order
	if len(output.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(output.Groups))
	}
	if output.Groups[0].Owner != "Flux" {
		t.Errorf("expected first group=Flux, got %s", output.Groups[0].Owner)
	}
	if output.Groups[1].Owner != "Native" {
		t.Errorf("expected second group=Native, got %s", output.Groups[1].Owner)
	}

	// Verify Flux items are sorted by (namespace, kind, name)
	fluxItems := output.Groups[0].Items
	if len(fluxItems) != 2 {
		t.Fatalf("expected 2 Flux items, got %d", len(fluxItems))
	}
	// api/backend should come before web/frontend
	if fluxItems[0].ID.Namespace != "api" || fluxItems[0].ID.Name != "backend" {
		t.Errorf("expected first Flux item to be api/backend, got %s/%s", fluxItems[0].ID.Namespace, fluxItems[0].ID.Name)
	}
	if fluxItems[1].ID.Namespace != "web" || fluxItems[1].ID.Name != "frontend" {
		t.Errorf("expected second Flux item to be web/frontend, got %s/%s", fluxItems[1].ID.Namespace, fluxItems[1].ID.Name)
	}

	// Verify ownerRef is populated for Flux items
	if fluxItems[0].OwnerRef == nil {
		t.Error("expected ownerRef for Flux item, got nil")
	} else if fluxItems[0].OwnerRef.Kind != "Kustomization" {
		t.Errorf("expected ownerRef.kind=Kustomization, got %s", fluxItems[0].OwnerRef.Kind)
	}

	// Verify Native item has nil ownerRef
	nativeItems := output.Groups[1].Items
	if len(nativeItems) != 1 {
		t.Fatalf("expected 1 Native item, got %d", len(nativeItems))
	}
	if nativeItems[0].OwnerRef != nil {
		t.Error("expected nil ownerRef for Native item")
	}

	// Verify summary
	if output.Summary.TotalResources != 3 {
		t.Errorf("expected totalResources=3, got %d", output.Summary.TotalResources)
	}
	if output.Summary.ByOwner["Flux"] != 2 {
		t.Errorf("expected byOwner[Flux]=2, got %d", output.Summary.ByOwner["Flux"])
	}
	if output.Summary.ByOwner["Native"] != 1 {
		t.Errorf("expected byOwner[Native]=1, got %d", output.Summary.ByOwner["Native"])
	}
}

func TestBuildOwnershipTreeJSON_FilterOwner(t *testing.T) {
	entries := []Entry{
		{Kind: "Deployment", Namespace: "web", Name: "frontend", Owner: "Flux"},
		{Kind: "Deployment", Namespace: "api", Name: "backend", Owner: "Flux"},
		{Kind: "Deployment", Namespace: "default", Name: "legacy", Owner: "Native"},
	}

	opts := OwnershipTreeOpts{FilterOwner: "Flux"}
	output := BuildOwnershipTreeJSON(entries, opts, "test-cluster", nil)

	// Should only have Flux group
	if len(output.Groups) != 1 {
		t.Fatalf("expected 1 group with filter, got %d", len(output.Groups))
	}
	if output.Groups[0].Owner != "Flux" {
		t.Errorf("expected filtered group=Flux, got %s", output.Groups[0].Owner)
	}

	// Context should show the filter
	if output.Context.Filters.Owner == nil || *output.Context.Filters.Owner != "Flux" {
		t.Error("expected context.filters.owner=Flux")
	}
}

func TestBuildOwnershipTreeJSON_DisplayMeta(t *testing.T) {
	entries := []Entry{
		{Kind: "Deployment", Namespace: "a", Name: "one", Owner: "Flux"},
		{Kind: "Deployment", Namespace: "b", Name: "two", Owner: "Flux"},
		{Kind: "Deployment", Namespace: "c", Name: "three", Owner: "Flux"},
		{Kind: "Deployment", Namespace: "d", Name: "four", Owner: "Flux"},
		{Kind: "Deployment", Namespace: "e", Name: "five", Owner: "Flux"},
	}

	opts := OwnershipTreeOpts{MaxDepth: 2}
	output := BuildOwnershipTreeJSON(entries, opts, "test-cluster", nil)

	// JSON should contain ALL items (lossless)
	if len(output.Groups[0].Items) != 5 {
		t.Errorf("expected 5 items (lossless), got %d", len(output.Groups[0].Items))
	}

	// DisplayMeta should indicate what ASCII would show
	if output.Groups[0].DisplayMeta.MaxItems != 2 {
		t.Errorf("expected displayMeta.maxItems=2, got %d", output.Groups[0].DisplayMeta.MaxItems)
	}
	if output.Groups[0].DisplayMeta.TotalItems != 5 {
		t.Errorf("expected displayMeta.totalItems=5, got %d", output.Groups[0].DisplayMeta.TotalItems)
	}
}

func TestBuildOwnershipTreeJSON_Deterministic(t *testing.T) {
	entries := []Entry{
		{Kind: "Deployment", Namespace: "web", Name: "frontend", Owner: "Flux"},
		{Kind: "Deployment", Namespace: "api", Name: "backend", Owner: "Flux"},
		{Kind: "StatefulSet", Namespace: "api", Name: "db", Owner: "ArgoCD"},
		{Kind: "Deployment", Namespace: "default", Name: "legacy", Owner: "Native"},
	}

	opts := OwnershipTreeOpts{}

	// Generate JSON twice and verify it's identical
	output1 := BuildOwnershipTreeJSON(entries, opts, "test-cluster", nil)
	output2 := BuildOwnershipTreeJSON(entries, opts, "test-cluster", nil)

	json1, _ := json.MarshalIndent(output1, "", "  ")
	json2, _ := json.MarshalIndent(output2, "", "  ")

	if string(json1) != string(json2) {
		t.Error("JSON output is not deterministic")
	}
}

func TestBuildOwnershipTreeJSON_CanonicalOwnerOrder(t *testing.T) {
	// Create entries in non-canonical order
	entries := []Entry{
		{Kind: "Deployment", Namespace: "a", Name: "one", Owner: "Native"},
		{Kind: "Deployment", Namespace: "b", Name: "two", Owner: "ConfigHub"},
		{Kind: "Deployment", Namespace: "c", Name: "three", Owner: "Helm"},
		{Kind: "Deployment", Namespace: "d", Name: "four", Owner: "ArgoCD"},
		{Kind: "Deployment", Namespace: "e", Name: "five", Owner: "Flux"},
	}

	opts := OwnershipTreeOpts{}
	output := BuildOwnershipTreeJSON(entries, opts, "test-cluster", nil)

	// Verify canonical order matches ASCII model: Flux, ArgoCD, Helm, ConfigHub, Native
	// (Crossplane is NOT a first-class category - falls to Native)
	expectedOrder := []string{"Flux", "ArgoCD", "Helm", "ConfigHub", "Native"}
	if len(output.Groups) != len(expectedOrder) {
		t.Fatalf("expected %d groups, got %d", len(expectedOrder), len(output.Groups))
	}

	for i, expected := range expectedOrder {
		if output.Groups[i].Owner != expected {
			t.Errorf("expected groups[%d].owner=%s, got %s", i, expected, output.Groups[i].Owner)
		}
	}
}

func TestBuildOwnershipTreeJSON_SortByNamespaceKindName(t *testing.T) {
	// Create entries that would sort differently if not using (namespace, kind, name)
	entries := []Entry{
		{Kind: "StatefulSet", Namespace: "api", Name: "cache", Owner: "Flux"},
		{Kind: "Deployment", Namespace: "api", Name: "backend", Owner: "Flux"},
		{Kind: "Deployment", Namespace: "web", Name: "frontend", Owner: "Flux"},
		{Kind: "Deployment", Namespace: "api", Name: "gateway", Owner: "Flux"},
	}

	opts := OwnershipTreeOpts{}
	output := BuildOwnershipTreeJSON(entries, opts, "test-cluster", nil)

	items := output.Groups[0].Items

	// Expected order: api/Deployment/backend, api/Deployment/gateway, api/StatefulSet/cache, web/Deployment/frontend
	expected := []struct {
		ns, kind, name string
	}{
		{"api", "Deployment", "backend"},
		{"api", "Deployment", "gateway"},
		{"api", "StatefulSet", "cache"},
		{"web", "Deployment", "frontend"},
	}

	for i, exp := range expected {
		if items[i].ID.Namespace != exp.ns || items[i].ID.Kind != exp.kind || items[i].ID.Name != exp.name {
			t.Errorf("item[%d]: expected %s/%s/%s, got %s/%s/%s",
				i, exp.ns, exp.kind, exp.name,
				items[i].ID.Namespace, items[i].ID.Kind, items[i].ID.Name)
		}
	}
}

// =============================================================================
// Trace Schema Tests
// =============================================================================

func TestInferRole(t *testing.T) {
	tests := []struct {
		kind     string
		expected string
	}{
		{"GitRepository", RoleSource},
		{"OCIRepository", RoleSource},
		{"HelmRepository", RoleSource},
		{"Bucket", RoleSource},
		{"Kustomization", RoleDeployer},
		{"HelmRelease", RoleDeployer},
		{"Application", RoleDeployer},
		{"ReplicaSet", RoleIntermediate},
		{"Pod", RoleIntermediate},
		{"Deployment", RoleWorkload},
		{"StatefulSet", RoleWorkload},
		{"Service", RoleWorkload},
		{"ConfigMap", RoleWorkload},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			got := InferRole(tt.kind)
			if got != tt.expected {
				t.Errorf("InferRole(%q) = %q, want %q", tt.kind, got, tt.expected)
			}
		})
	}
}

func TestInferRelationship(t *testing.T) {
	tests := []struct {
		from, to string
		expected string
	}{
		{"GitRepository", "Kustomization", RelSources},
		{"Kustomization", "Deployment", RelApplies},
		{"Kustomization", "ReplicaSet", RelManages},
		{"ReplicaSet", "Pod", RelManages},
		{"Deployment", "ReplicaSet", RelManagedBy}, // Workload to intermediate is managed-by
	}

	for _, tt := range tests {
		t.Run(tt.from+"->"+tt.to, func(t *testing.T) {
			got := InferRelationship(tt.from, tt.to)
			if got != tt.expected {
				t.Errorf("InferRelationship(%q, %q) = %q, want %q", tt.from, tt.to, got, tt.expected)
			}
		})
	}
}

func TestTraceOutput_Structure(t *testing.T) {
	// Verify TraceOutput can be marshaled to JSON
	output := TraceOutput{
		Command: "trace",
		Target: ResourceID{
			Kind:      "Deployment",
			Namespace: "web",
			Name:      "frontend",
		},
		Chain: []ChainNode{
			{
				ID: ResourceID{
					Kind:      "GitRepository",
					Namespace: "flux-system",
					Name:      "flux-system",
				},
				Role:         RoleSource,
				Relationship: RelSources,
				Evidence: []Evidence{
					{
						Type:  EvidenceField,
						Key:   "spec.url",
						Value: "https://github.com/org/repo",
						Path:  "spec.url",
					},
				},
			},
			{
				ID: ResourceID{
					Kind:      "Kustomization",
					Namespace: "flux-system",
					Name:      "web-apps",
				},
				Role:         RoleDeployer,
				Relationship: RelApplies,
				Evidence: []Evidence{
					{
						Type:  EvidenceInventory,
						Key:   "status.inventory",
						Value: "Deployment/web/frontend",
						Path:  "status.inventory.entries",
					},
				},
			},
			{
				ID: ResourceID{
					Kind:      "Deployment",
					Namespace: "web",
					Name:      "frontend",
				},
				Role:         RoleWorkload,
				Relationship: RelManagedBy,
				Evidence: []Evidence{
					{
						Type:  EvidenceLabel,
						Key:   "kustomize.toolkit.fluxcd.io/name",
						Value: "web-apps",
						Path:  "metadata.labels",
					},
				},
			},
		},
		Summary: TraceSummary{
			OwnerType: "Flux",
			Source: &TraceSourceRef{
				Kind:      "GitRepository",
				Namespace: "flux-system",
				Name:      "flux-system",
				URL:       "https://github.com/org/repo",
			},
			Deployer: &ResourceID{
				Kind:      "Kustomization",
				Namespace: "flux-system",
				Name:      "web-apps",
			},
		},
	}

	// Marshal and verify structure
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal TraceOutput: %v", err)
	}

	// Verify key fields are present
	jsonStr := string(data)
	expectedKeys := []string{
		`"command": "trace"`,
		`"target"`,
		`"chain"`,
		`"role": "source"`,
		`"role": "deployer"`,
		`"role": "workload"`,
		`"relationship"`,
		`"evidence"`,
		`"type": "field"`,
		`"type": "inventory"`,
		`"type": "label"`,
		`"summary"`,
		`"ownerType": "Flux"`,
	}

	for _, key := range expectedKeys {
		if !contains(jsonStr, key) {
			t.Errorf("JSON output missing expected key: %s", key)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
