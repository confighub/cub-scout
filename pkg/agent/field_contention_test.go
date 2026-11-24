// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"reflect"
	"testing"
)

func TestDetectFieldContention(t *testing.T) {
	tests := []struct {
		name           string
		owner          Ownership
		managers       []string
		wantContested  bool
		wantWriters    []string
		wantCategories []string
		wantReason     bool
	}{
		{
			name:           "controller plus G5 agent manager",
			owner:          Ownership{Type: OwnerFlux, SubType: "kustomization"},
			managers:       []string{ManagerFluxKustomize, "agent/remediator/run-42"},
			wantContested:  true,
			wantWriters:    []string{"agent/remediator/run-42", ManagerFluxKustomize},
			wantCategories: []string{"agent/remediator", WriterCategoryController},
			wantReason:     true,
		},
		{
			name:           "controller plus kubectl interactive manager",
			owner:          Ownership{Type: OwnerArgo},
			managers:       []string{ManagerArgoCD, ManagerKubectlEdit},
			wantContested:  true,
			wantWriters:    []string{ManagerArgoCD, ManagerKubectlEdit},
			wantCategories: []string{WriterCategoryController, WriterCategoryInteractive},
			wantReason:     true,
		},
		{
			name:           "two distinct G5 agent tools",
			owner:          Ownership{Type: OwnerKubernetes},
			managers:       []string{"agent/codex/sess-2", "agent/claude-code/sess-1"},
			wantContested:  true,
			wantWriters:    []string{"agent/claude-code/sess-1", "agent/codex/sess-2"},
			wantCategories: []string{"agent/claude-code", "agent/codex"},
			wantReason:     true,
		},
		{
			name:           "same G5 agent tool across two runs is not contested",
			owner:          Ownership{Type: OwnerKubernetes},
			managers:       []string{"agent/claude-code/run-1", "agent/claude-code/run-2"},
			wantContested:  false,
			wantWriters:    []string{"agent/claude-code/run-1", "agent/claude-code/run-2"},
			wantCategories: []string{"agent/claude-code"},
			wantReason:     false,
		},
		{
			name:           "single controller is not contested",
			owner:          Ownership{Type: OwnerFlux, SubType: "kustomization"},
			managers:       []string{ManagerFluxKustomize},
			wantContested:  false,
			wantWriters:    []string{ManagerFluxKustomize},
			wantCategories: []string{WriterCategoryController},
			wantReason:     false,
		},
		{
			name:           "argo controller plus CSA migration default share controller class",
			owner:          Ownership{Type: OwnerArgo},
			managers:       []string{ManagerArgoCD, ManagerKubectlCSA},
			wantContested:  false,
			wantWriters:    []string{ManagerArgoCD, ManagerKubectlCSA},
			wantCategories: []string{WriterCategoryController},
			wantReason:     false,
		},
		{
			name:           "single unknown manager is never guessed as contested",
			owner:          Ownership{Type: OwnerFlux},
			managers:       []string{"vendor-writer"},
			wantContested:  false,
			wantWriters:    []string{"vendor-writer"},
			wantCategories: nil,
			wantReason:     false,
		},
		{
			name:           "controller plus unknown manager does not guess contention",
			owner:          Ownership{Type: OwnerFlux},
			managers:       []string{ManagerFluxKustomize, "vendor-writer"},
			wantContested:  false,
			wantWriters:    []string{ManagerFluxKustomize, "vendor-writer"},
			wantCategories: []string{WriterCategoryController},
			wantReason:     false,
		},
		{
			name:           "agent plus interactive kubectl manager is contested",
			owner:          Ownership{Type: OwnerKubernetes},
			managers:       []string{"agent/remediator", ManagerKubectlSet},
			wantContested:  true,
			wantWriters:    []string{"agent/remediator", ManagerKubectlSet},
			wantCategories: []string{"agent/remediator", WriterCategoryInteractive},
			wantReason:     true,
		},
		{
			name:           "controller plus terraform is contested",
			owner:          Ownership{Type: OwnerFlux},
			managers:       []string{ManagerFluxKustomize, ManagerTerraformManifest},
			wantContested:  true,
			wantWriters:    []string{ManagerTerraformManifest, ManagerFluxKustomize},
			wantCategories: []string{WriterCategoryController, WriterCategoryTerraform},
			wantReason:     true,
		},
		{
			name:           "empty managers returns uncontested zero value",
			owner:          Ownership{Type: OwnerFlux},
			managers:       nil,
			wantContested:  false,
			wantWriters:    nil,
			wantCategories: nil,
			wantReason:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectFieldContention(tc.owner, tc.managers)
			if got.Contested != tc.wantContested {
				t.Fatalf("Contested = %v, want %v (got %+v)", got.Contested, tc.wantContested, got)
			}
			if !reflect.DeepEqual(got.Writers, tc.wantWriters) {
				t.Fatalf("Writers = %v, want %v", got.Writers, tc.wantWriters)
			}
			if !reflect.DeepEqual(got.Categories, tc.wantCategories) {
				t.Fatalf("Categories = %v, want %v", got.Categories, tc.wantCategories)
			}
			if tc.wantReason && got.Reason == "" {
				t.Fatalf("expected non-empty Reason for contested field")
			}
			if !tc.wantReason && got.Reason != "" {
				t.Fatalf("expected empty Reason for uncontested field, got %q", got.Reason)
			}
		})
	}
}
