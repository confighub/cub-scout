// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// gen-receipt-examples emits the canonical example receipts under
// examples/receipts/. Run from the repo root pointing at the receipts
// root (the generator writes one subdirectory per predicate):
//
//	go run ./tools/gen-receipt-examples examples/receipts
//
// The generator is deterministic: same input data + same VerifiedAt → same
// fingerprint. CI re-runs it and diffs against the committed files (see
// examples_receipts_test.go in cmd/cub-scout).
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func makeArgoLive() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "api",
				"namespace": "prod",
				"annotations": map[string]interface{}{
					"argocd.argoproj.io/tracking-id": "payments-api:apps/Deployment:prod/api",
				},
				"labels": map[string]interface{}{
					"argocd.argoproj.io/instance": "payments-api",
					"app":                          "api",
				},
			},
			"spec": map[string]interface{}{"replicas": int64(3)},
		},
	}
}

func baseInput() agent.BuildReceiptInput {
	return agent.BuildReceiptInput{
		Live: makeArgoLive(),
		Scope: agent.Scope{
			Kind:      "Deployment",
			Name:      "api",
			Namespace: "prod",
			Cluster:   "prod-use2",
		},
		Owner: agent.Ownership{Type: agent.OwnerArgo, SubType: "application"},
		PredicateName: agent.PredicateAppliedMatchesSpec,
		Spec: &agent.SpecAnchor{
			Anchor: agent.SpecAnchorBody{
				Type:     "git",
				RepoURL:  "https://github.com/org/platform-config",
				Revision: "abc123def456abc123def456abc123def456abcd",
				Path:     "apps/prod/api",
			},
		},
		Connected:  false,
		Verifier:   agent.Verifier{Tool: "cub-scout", Version: "v0.99.0-receipt-batch-1"},
		VerifiedAt: time.Date(2026, 5, 21, 10, 30, 0, 0, time.UTC),
	}
}

// passCase: Argo-owned with controller-drift cause + matched anchor → PASS.
func passCase() agent.BuildReceiptInput {
	in := baseInput()
	in.Evidence = agent.Evidence{
		Attribution: &agent.FieldMutationAttribution{
			Cause:       agent.CauseControllerDrift,
			ManagerHint: "argocd-controller",
		},
		GitSource: &agent.GitSourceAnchor{
			RepoURL:  "https://github.com/org/platform-config",
			Revision: "abc123def456abc123def456abc123def456abcd",
			Path:     "apps/prod/api",
		},
	}
	return in
}

// blockManualEdit: Argo-owned but a kubectl-edit happened → BLOCK.
func blockManualEdit() agent.BuildReceiptInput {
	in := baseInput()
	in.Evidence = agent.Evidence{
		Attribution: &agent.FieldMutationAttribution{
			Cause:       agent.CauseManualEdit,
			ManagerHint: "kubectl-edit",
		},
		GitSource: &agent.GitSourceAnchor{
			RepoURL:  "https://github.com/org/platform-config",
			Revision: "abc123def456abc123def456abc123def456abcd",
			Path:     "apps/prod/api",
		},
	}
	return in
}

// blockAnchorMismatch: live anchor is one revision; spec passed in
// --at-commit is a different revision → BLOCK.
func blockAnchorMismatch() agent.BuildReceiptInput {
	in := baseInput()
	in.Evidence = agent.Evidence{
		Attribution: &agent.FieldMutationAttribution{
			Cause:       agent.CauseControllerDrift,
			ManagerHint: "argocd-controller",
		},
		GitSource: &agent.GitSourceAnchor{
			RepoURL:  "https://github.com/org/platform-config",
			Revision: "abc123def456abc123def456abc123def456abcd",
			Path:     "apps/prod/api",
		},
	}
	in.Spec.Anchor.Revision = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	return in
}

// inconclusiveNoAnchor: no controller-resolved git anchor on the resource
// → INCONCLUSIVE + git-source-anchor omission.
func inconclusiveNoAnchor() agent.BuildReceiptInput {
	in := baseInput()
	in.Evidence = agent.Evidence{
		Attribution: &agent.FieldMutationAttribution{
			Cause:       agent.CauseUnknown,
			ManagerHint: "",
		},
		GitSource: nil,
	}
	return in
}

// --- batch 2: source-truth-pass examples ----------------------------

func sourceTruthPassInput() agent.BuildReceiptInput {
	in := baseInput()
	in.PredicateName = agent.PredicateSourceTruthPass
	in.Spec = nil // source-truth-pass doesn't use a spec anchor
	in.Strategy = string(agent.StrategyGitArgo)
	in.Evidence = agent.Evidence{
		SourceTruth: &agent.SourceTruthEvidence{
			DeclaredStrategy: agent.StrategyGitArgo.Human(),
			Status:           agent.StatusPASS,
			SourceTruth:      agent.VerdictAGREED,
			Outlier:          agent.OutlierUnknown,
			Surfaces: agent.SourceTruthSurfaces{
				ConfigHub:  &agent.ConfigHubSurface{Space: "payments", Unit: "api", Revision: "42"},
				Controller: &agent.ControllerSurface{Kind: "Argo", Source: "https://github.com/org/platform-config", RevisionOrDigest: "abc123def456abc123def456abc123def456abcd"},
				Runtime:    &agent.RuntimeSurface{Resource: "Deployment/api in prod", Field: "spec.template.spec.containers[0].image", Value: "ghcr.io/org/api:v2.3.0", Health: "Ready"},
			},
		},
	}
	return in
}

func sourceTruthBlockMismatch() agent.BuildReceiptInput {
	in := sourceTruthPassInput()
	in.Evidence.SourceTruth.Status = agent.StatusBLOCK
	in.Evidence.SourceTruth.SourceTruth = agent.VerdictMISMATCH
	in.Evidence.SourceTruth.Outlier = agent.OutlierController
	in.Evidence.SourceTruth.Surfaces.Controller.RevisionOrDigest = "feedfacefeedfacefeedfacefeedfacefeedface"
	return in
}

// --- batch 2: no-manual-edits-since examples ------------------------

func noManualEditsSincePass() agent.BuildReceiptInput {
	cutoff := time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC)
	before := metav1.NewTime(time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC))

	in := baseInput()
	in.PredicateName = agent.PredicateNoManualEditsSince
	in.Spec = nil
	in.Strategy = ""
	in.Since = cutoff
	in.Evidence = agent.Evidence{}

	// Live carries the managedFields the predicate walks. The base Live
	// is a bare Deployment; rebuild with a single Argo controller entry
	// dated before the cutoff so the predicate emits PASS.
	live := makeArgoLive()
	live.SetManagedFields([]metav1.ManagedFieldsEntry{
		{Manager: "argocd-controller", Operation: metav1.ManagedFieldsOperationApply, Time: &before},
	})
	in.Live = live
	return in
}

func noManualEditsSinceBlock() agent.BuildReceiptInput {
	after := metav1.NewTime(time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC))

	in := noManualEditsSincePass()
	live := makeArgoLive()
	live.SetManagedFields([]metav1.ManagedFieldsEntry{
		{Manager: "argocd-controller", Operation: metav1.ManagedFieldsOperationApply, Time: &after},
		{Manager: "kubectl-edit", Operation: metav1.ManagedFieldsOperationUpdate, Time: &after},
	})
	in.Live = live
	return in
}

type example struct {
	predicateDir string // subdirectory under outDir, e.g. "applied-matches-spec"
	name         string // filename within that subdirectory
	in           agent.BuildReceiptInput
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: gen-receipt-examples <output-dir>")
		os.Exit(2)
	}
	outDir := os.Args[1]
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", outDir, err)
		os.Exit(1)
	}

	examples := []example{
		// applied-matches-spec (batch 1).
		{"applied-matches-spec", "pass-controller-drift.json", passCase()},
		{"applied-matches-spec", "block-manual-edit.json", blockManualEdit()},
		{"applied-matches-spec", "block-anchor-mismatch.json", blockAnchorMismatch()},
		{"applied-matches-spec", "inconclusive-no-anchor.json", inconclusiveNoAnchor()},
		// source-truth-pass (batch 2).
		{"source-truth-pass", "pass-agreed.json", sourceTruthPassInput()},
		{"source-truth-pass", "block-mismatch.json", sourceTruthBlockMismatch()},
		// no-manual-edits-since (batch 2).
		{"no-manual-edits-since", "pass-controller-only.json", noManualEditsSincePass()},
		{"no-manual-edits-since", "block-kubectl-edit.json", noManualEditsSinceBlock()},
	}

	for _, ex := range examples {
		stmt, err := agent.BuildReceipt(ex.in)
		if err != nil {
			fmt.Fprintf(os.Stderr, "build %s/%s: %v\n", ex.predicateDir, ex.name, err)
			os.Exit(1)
		}
		buf, err := json.MarshalIndent(stmt, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "marshal %s/%s: %v\n", ex.predicateDir, ex.name, err)
			os.Exit(1)
		}
		predicateDir := filepath.Join(outDir, ex.predicateDir)
		if err := os.MkdirAll(predicateDir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", predicateDir, err)
			os.Exit(1)
		}
		path := filepath.Join(predicateDir, ex.name)
		if err := os.WriteFile(path, append(buf, '\n'), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Println("wrote", path)
	}
}
