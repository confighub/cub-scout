// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestCompleteOwnersIncludesCrossplaneTerraformAndKro(t *testing.T) {
	cmd := &cobra.Command{}
	owners, _ := completeOwners(cmd, nil, "")

	foundTerraform := false
	foundCrossplane := false
	foundKro := false
	for _, o := range owners {
		if o == "Terraform" {
			foundTerraform = true
		}
		if o == "Crossplane" {
			foundCrossplane = true
		}
		if o == "kro" {
			foundKro = true
		}
	}
	if !foundTerraform {
		t.Fatalf("expected Terraform in owner completions, got: %v", owners)
	}
	if !foundCrossplane {
		t.Fatalf("expected Crossplane in owner completions, got: %v", owners)
	}
	if !foundKro {
		t.Fatalf("expected kro in owner completions, got: %v", owners)
	}
}

// TestCompleteOwnersMatchesValidOwners verifies that completeOwners returns
// the same set of owners as ValidOwners (CLI ↔ TUI symmetry #91/#93)
func TestCompleteOwnersMatchesValidOwners(t *testing.T) {
	cmd := &cobra.Command{}
	completions, _ := completeOwners(cmd, nil, "")

	if len(completions) != len(ValidOwners) {
		t.Errorf("completeOwners returned %d items, but ValidOwners has %d",
			len(completions), len(ValidOwners))
	}

	// Verify all ValidOwners are in completions
	for _, valid := range ValidOwners {
		found := false
		for _, c := range completions {
			if c == valid {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ValidOwner %q not found in completeOwners result", valid)
		}
	}
}

// TestFilterPrefix verifies prefix filtering works correctly
func TestFilterPrefix(t *testing.T) {
	items := []string{"Flux", "ArgoCD", "Helm", "Native"}

	tests := []struct {
		prefix string
		want   int // expected count
	}{
		{"", 4},    // no filter, all items
		{"F", 1},   // matches Flux
		{"f", 1},   // case-insensitive, matches Flux
		{"ar", 1},  // matches ArgoCD
		{"H", 1},   // matches Helm
		{"Na", 1},  // matches Native
		{"xyz", 0}, // no match
	}

	for _, tt := range tests {
		t.Run(tt.prefix, func(t *testing.T) {
			got := filterPrefix(items, tt.prefix)
			if len(got) != tt.want {
				t.Errorf("filterPrefix(%v, %q) = %d items, want %d", items, tt.prefix, len(got), tt.want)
			}
		})
	}
}

// TestCompleteKindsIncludesFluxAndArgoTypes verifies GitOps CRD kinds are included
func TestCompleteKindsIncludesFluxAndArgoTypes(t *testing.T) {
	cmd := &cobra.Command{}
	kinds, _ := completeKinds(cmd, nil, "")

	expected := []string{
		"Kustomization", // Flux
		"HelmRelease",   // Flux
		"GitRepository", // Flux
		"Application",   // ArgoCD
		"Deployment",    // Core K8s
	}

	for _, want := range expected {
		found := false
		for _, k := range kinds {
			if k == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected kind %q in completions, not found", want)
		}
	}
}
