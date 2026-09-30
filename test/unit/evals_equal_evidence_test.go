// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package unit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

type attributionExport struct {
	Items []struct {
		Metadata struct {
			Name          string `yaml:"name"`
			Namespace     string `yaml:"namespace"`
			ManagedFields []struct {
				Manager string `yaml:"manager"`
			} `yaml:"managedFields"`
		} `yaml:"metadata"`
	} `yaml:"items"`
}

type explainMutation struct {
	MutationManager string `json:"mutationManager"`
	MutationCause   string `json:"mutationCause"`
}

func exportHasManager(data []byte, namespace, name, manager string) bool {
	var dump attributionExport
	if yaml.Unmarshal(data, &dump) != nil {
		return false
	}
	for _, item := range dump.Items {
		if item.Metadata.Namespace != namespace || item.Metadata.Name != name {
			continue
		}
		for _, field := range item.Metadata.ManagedFields {
			if field.Manager == manager {
				return true
			}
		}
		return false
	}
	return false
}

func TestEvalAttributionEvidenceMatchesManagedFields(t *testing.T) {
	root := filepath.Join("..", "..")
	exportPath := filepath.Join(root, "evals", "fixtures", "cluster", "deployments.yaml")
	data, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		caseName  string
		namespace string
		name      string
		manager   string
		cause     string
	}{
		{"changed-by-checkout", "shop", "checkout", "kubectl-set", "manual-edit"},
		{"changed-by-cart", "shop", "cart", "kubectl", "manual-edit"},
		{"changed-by-inventory", "inventory", "inventory", "kubectl-patch", "manual-edit"},
		{"changed-by-payments", "payments", "payments-api", "helm", "controller-drift"},
	}
	for _, tc := range cases {
		t.Run(tc.caseName, func(t *testing.T) {
			if _, err := os.Stat(filepath.Join(root, "evals", tc.caseName, "case.yaml")); err != nil {
				t.Fatalf("mapped eval case is missing: %v", err)
			}
			if !exportHasManager(data, tc.namespace, tc.name, tc.manager) {
				t.Fatalf("recorded Deployment %s/%s lacks expected managedFields manager %q", tc.namespace, tc.name, tc.manager)
			}

			// Prove this invariant would fail if managedFields were stripped from
			// the same parsed recording, without modifying any checked-in fixture.
			var stripped attributionExport
			if err := yaml.Unmarshal(data, &stripped); err != nil {
				t.Fatal(err)
			}
			found := false
			for i := range stripped.Items {
				m := &stripped.Items[i].Metadata
				if m.Namespace == tc.namespace && m.Name == tc.name {
					m.ManagedFields = nil
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("recorded Deployment %s/%s not found for negative check", tc.namespace, tc.name)
			}
			withoutManagedFields, err := yaml.Marshal(&stripped)
			if err != nil {
				t.Fatal(err)
			}
			if exportHasManager(withoutManagedFields, tc.namespace, tc.name, tc.manager) {
				t.Errorf("manager %q unexpectedly remains after removing managedFields", tc.manager)
			}

			explainPath := filepath.Join(root, "evals", "mocks", "cub-scout", "fixtures", "explain", tc.name+".txt")
			explainData, err := os.ReadFile(explainPath)
			if err != nil {
				t.Fatal(err)
			}
			var explain explainMutation
			if err := json.Unmarshal(explainData, &explain); err != nil {
				t.Fatalf("decode %s: %v", explainPath, err)
			}
			if explain.MutationManager != tc.manager || explain.MutationCause != tc.cause {
				t.Errorf("explain reports manager=%q cause=%q; want manager=%q cause=%q",
					explain.MutationManager, explain.MutationCause, tc.manager, tc.cause)
			}
		})
	}
}
