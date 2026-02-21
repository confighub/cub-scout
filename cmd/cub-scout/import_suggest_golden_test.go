// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// updateImportSuggestGolden controls golden file updates.
// Set via: UPDATE_GOLDEN=1 go test ./cmd/cub-scout/ -run TestImportSuggest
var updateImportSuggestGolden = os.Getenv("UPDATE_GOLDEN") == "1"

// TestImportSuggest_ArniePattern verifies the suggestion engine against
// an ArgoCD folder-per-environment cluster (the "Arnie" pattern).
//
// Input: 6 Deployments across 3 envs (dev/staging/prod), all ArgoCD-managed
// with Application paths like envs/dev, envs/staging, envs/prod.
// Plus 3 Helm-managed redis instances and 1 unmanaged ConfigMap.
//
// Expected: variant inference from Application paths, correct app grouping,
// Helm workloads grouped separately, Native workload detected.
func TestImportSuggest_ArniePattern(t *testing.T) {
	workloads := []WorkloadInfo{
		// ArgoCD-managed api deployments
		{
			Kind: "Deployment", Namespace: "myapp-dev", Name: "api",
			Owner: "ArgoCD", Ready: true, Replicas: 1,
			ApplicationPath: "envs/dev",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "api",
				"app.kubernetes.io/instance":   "api-dev",
				"argocd.argoproj.io/instance":  "myapp-dev",
			},
		},
		{
			Kind: "Deployment", Namespace: "myapp-staging", Name: "api",
			Owner: "ArgoCD", Ready: true, Replicas: 2,
			ApplicationPath: "envs/staging",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "api",
				"app.kubernetes.io/instance":   "api-staging",
				"argocd.argoproj.io/instance":  "myapp-staging",
			},
		},
		{
			Kind: "Deployment", Namespace: "myapp-prod", Name: "api",
			Owner: "ArgoCD", Ready: true, Replicas: 3,
			ApplicationPath: "envs/prod",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "api",
				"app.kubernetes.io/instance":   "api-prod",
				"argocd.argoproj.io/instance":  "myapp-prod",
			},
		},
		// ArgoCD-managed worker deployments
		{
			Kind: "Deployment", Namespace: "myapp-dev", Name: "worker",
			Owner: "ArgoCD", Ready: true, Replicas: 1,
			ApplicationPath: "envs/dev",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "worker",
				"app.kubernetes.io/instance":   "worker-dev",
				"argocd.argoproj.io/instance":  "myapp-dev",
			},
		},
		{
			Kind: "Deployment", Namespace: "myapp-staging", Name: "worker",
			Owner: "ArgoCD", Ready: true, Replicas: 1,
			ApplicationPath: "envs/staging",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "worker",
				"app.kubernetes.io/instance":   "worker-staging",
				"argocd.argoproj.io/instance":  "myapp-staging",
			},
		},
		{
			Kind: "Deployment", Namespace: "myapp-prod", Name: "worker",
			Owner: "ArgoCD", Ready: true, Replicas: 2,
			ApplicationPath: "envs/prod",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "worker",
				"app.kubernetes.io/instance":   "worker-prod",
				"argocd.argoproj.io/instance":  "myapp-prod",
			},
		},
		// Helm-managed redis
		{
			Kind: "StatefulSet", Namespace: "myapp-dev", Name: "redis",
			Owner: "Helm", Ready: true, Replicas: 1,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "redis",
				"app.kubernetes.io/managed-by": "Helm",
			},
		},
		{
			Kind: "StatefulSet", Namespace: "myapp-staging", Name: "redis",
			Owner: "Helm", Ready: true, Replicas: 1,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "redis",
				"app.kubernetes.io/managed-by": "Helm",
			},
		},
		{
			Kind: "StatefulSet", Namespace: "myapp-prod", Name: "redis",
			Owner: "Helm", Ready: true, Replicas: 3,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "redis",
				"app.kubernetes.io/managed-by": "Helm",
			},
		},
	}

	runImportSuggestGolden(t, "arnie-pattern", workloads)
}

// TestImportSuggest_BankoPattern verifies the suggestion engine against
// a Flux monorepo cluster (the "Banko" pattern).
//
// Input: Deployments managed by Flux Kustomizations with paths like
// ./apps/payment-api/overlays/dev and ./apps/payment-api/overlays/prod.
//
// Expected: variant inference from Kustomization paths.
func TestImportSuggest_BankoPattern(t *testing.T) {
	workloads := []WorkloadInfo{
		{
			Kind: "Deployment", Namespace: "payment-dev", Name: "payment-api",
			Owner: "Flux", Ready: true, Replicas: 1,
			KustomizationPath: "./apps/payment-api/overlays/dev",
			Labels: map[string]string{
				"app.kubernetes.io/name":                   "payment-api",
				"kustomize.toolkit.fluxcd.io/name":         "payment-dev",
				"kustomize.toolkit.fluxcd.io/namespace":    "flux-system",
			},
		},
		{
			Kind: "Deployment", Namespace: "payment-prod", Name: "payment-api",
			Owner: "Flux", Ready: true, Replicas: 3,
			KustomizationPath: "./apps/payment-api/overlays/prod",
			Labels: map[string]string{
				"app.kubernetes.io/name":                   "payment-api",
				"kustomize.toolkit.fluxcd.io/name":         "payment-prod",
				"kustomize.toolkit.fluxcd.io/namespace":    "flux-system",
			},
		},
		{
			Kind: "Deployment", Namespace: "payment-dev", Name: "payment-worker",
			Owner: "Flux", Ready: true, Replicas: 1,
			KustomizationPath: "./apps/payment-worker/overlays/dev",
			Labels: map[string]string{
				"app.kubernetes.io/name":                   "payment-worker",
				"kustomize.toolkit.fluxcd.io/name":         "payment-dev",
				"kustomize.toolkit.fluxcd.io/namespace":    "flux-system",
			},
		},
		{
			Kind: "Deployment", Namespace: "payment-prod", Name: "payment-worker",
			Owner: "Flux", Ready: true, Replicas: 2,
			KustomizationPath: "./apps/payment-worker/overlays/prod",
			Labels: map[string]string{
				"app.kubernetes.io/name":                   "payment-worker",
				"kustomize.toolkit.fluxcd.io/name":         "payment-prod",
				"kustomize.toolkit.fluxcd.io/namespace":    "flux-system",
			},
		},
	}

	runImportSuggestGolden(t, "banko-pattern", workloads)
}

// TestImportSuggest_MixedOwners verifies the suggestion engine when
// Flux, ArgoCD, Helm, and Native workloads coexist in the same namespace.
//
// Expected: each workload correctly grouped by app, mixed owners handled gracefully.
func TestImportSuggest_MixedOwners(t *testing.T) {
	workloads := []WorkloadInfo{
		{
			Kind: "Deployment", Namespace: "checkout", Name: "checkout-api",
			Owner: "Flux", Ready: true, Replicas: 2,
			KustomizationPath: "./apps/checkout/overlays/prod",
			Labels: map[string]string{
				"app.kubernetes.io/name":                "checkout-api",
				"kustomize.toolkit.fluxcd.io/name":      "checkout-prod",
				"kustomize.toolkit.fluxcd.io/namespace": "flux-system",
			},
		},
		{
			Kind: "Deployment", Namespace: "checkout", Name: "checkout-frontend",
			Owner: "ArgoCD", Ready: true, Replicas: 2,
			ApplicationPath: "apps/checkout/prod",
			Labels: map[string]string{
				"app.kubernetes.io/name":      "checkout-frontend",
				"argocd.argoproj.io/instance": "checkout-prod",
			},
		},
		{
			Kind: "StatefulSet", Namespace: "checkout", Name: "checkout-db",
			Owner: "Helm", Ready: true, Replicas: 1,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "checkout-db",
				"app.kubernetes.io/managed-by": "Helm",
			},
		},
		{
			Kind: "Deployment", Namespace: "checkout", Name: "debug-tools",
			Owner: "Native", Ready: true, Replicas: 1,
			Labels: map[string]string{},
		},
	}

	runImportSuggestGolden(t, "mixed-owners", workloads)
}

// TestImportSuggest_Empty verifies graceful degradation with no workloads.
func TestImportSuggest_Empty(t *testing.T) {
	runImportSuggestGolden(t, "empty", []WorkloadInfo{})
}

// TestImportSuggest_NamespaceFallback verifies variant inference from namespace
// patterns when no GitOps paths are available (e.g., plain Helm releases).
func TestImportSuggest_NamespaceFallback(t *testing.T) {
	workloads := []WorkloadInfo{
		{
			Kind: "Deployment", Namespace: "orders-prod", Name: "orders-api",
			Owner: "Helm", Ready: true, Replicas: 3,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "orders-api",
				"app.kubernetes.io/managed-by": "Helm",
			},
		},
		{
			Kind: "Deployment", Namespace: "orders-staging", Name: "orders-api",
			Owner: "Helm", Ready: true, Replicas: 1,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "orders-api",
				"app.kubernetes.io/managed-by": "Helm",
			},
		},
		{
			Kind: "Deployment", Namespace: "orders-prod", Name: "orders-worker",
			Owner: "Helm", Ready: true, Replicas: 2,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "orders-worker",
				"app.kubernetes.io/managed-by": "Helm",
			},
		},
	}

	runImportSuggestGolden(t, "namespace-fallback", workloads)
}

// suggestGoldenOutput is the JSON structure we write to golden files.
// It captures both the HubAppSpaceSuggestion (primary) and the input summary.
type suggestGoldenOutput struct {
	TestDescription string                  `json:"testDescription"`
	InputSummary    suggestInputSummary     `json:"inputSummary"`
	Suggestion      suggestGoldenSuggestion `json:"suggestion"`
}

type suggestInputSummary struct {
	WorkloadCount int                    `json:"workloadCount"`
	Namespaces    []string               `json:"namespaces"`
	Owners        map[string]int         `json:"owners"`
}

type suggestGoldenSuggestion struct {
	AppSpace string            `json:"appSpace"`
	Units    []suggestGoldenUnit `json:"units"`
}

type suggestGoldenUnit struct {
	Slug      string   `json:"slug"`
	App       string   `json:"app"`
	Variant   string   `json:"variant"`
	Workloads []string `json:"workloads"`
}

func runImportSuggestGolden(t *testing.T, name string, workloads []WorkloadInfo) {
	t.Helper()

	// Run the suggestion engine
	suggestion := SuggestHubAppSpaceStructure(workloads, "")

	// Build input summary for context
	nsSet := make(map[string]bool)
	ownerCounts := make(map[string]int)
	for _, w := range workloads {
		nsSet[w.Namespace] = true
		ownerCounts[w.Owner]++
	}
	namespaces := make([]string, 0, len(nsSet))
	for ns := range nsSet {
		namespaces = append(namespaces, ns)
	}
	sort.Strings(namespaces)

	// Convert to golden output format
	units := make([]suggestGoldenUnit, len(suggestion.Units))
	for i, u := range suggestion.Units {
		wlNames := make([]string, len(u.Workloads))
		for j, w := range u.Workloads {
			wlNames[j] = fmt.Sprintf("%s/%s/%s", w.Kind, w.Namespace, w.Name)
		}
		sort.Strings(wlNames)
		units[i] = suggestGoldenUnit{
			Slug:      u.Slug,
			App:       u.App,
			Variant:   u.Variant,
			Workloads: wlNames,
		}
	}

	output := suggestGoldenOutput{
		TestDescription: name,
		InputSummary: suggestInputSummary{
			WorkloadCount: len(workloads),
			Namespaces:    namespaces,
			Owners:        ownerCounts,
		},
		Suggestion: suggestGoldenSuggestion{
			AppSpace: suggestion.AppSpace,
			Units:    units,
		},
	}

	// Marshal to stable JSON
	actual, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal output: %v", err)
	}
	actualStr := string(actual) + "\n"

	// Golden file path
	goldenPath := filepath.Join("testdata", "import-suggest", name+".golden.json")

	if updateImportSuggestGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(goldenPath), err)
		}
		if err := os.WriteFile(goldenPath, []byte(actualStr), 0o644); err != nil {
			t.Fatalf("write golden %s: %v", goldenPath, err)
		}
		t.Logf("updated golden: %s", goldenPath)
		return
	}

	// Read and compare
	expected, err := os.ReadFile(goldenPath)
	if os.IsNotExist(err) {
		t.Fatalf("golden file not found: %s\n\nActual output:\n%s\n\nRun with UPDATE_GOLDEN=1 to create it:\n  UPDATE_GOLDEN=1 go test ./cmd/cub-scout/ -run TestImportSuggest", goldenPath, actualStr)
	}
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}

	if actualStr != string(expected) {
		t.Errorf("output does not match golden file %s\n\n--- EXPECTED ---\n%s\n--- ACTUAL ---\n%s",
			goldenPath, string(expected), actualStr)
	}
}
