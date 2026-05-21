// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// gen-receipt-examples emits the four canonical example receipts under
// examples/receipts/applied-matches-spec/. Run from the repo root:
//
//	go run ./tools/gen-receipt-examples examples/receipts/applied-matches-spec
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

type example struct {
	name string
	in   agent.BuildReceiptInput
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
		{"pass-controller-drift.json", passCase()},
		{"block-manual-edit.json", blockManualEdit()},
		{"block-anchor-mismatch.json", blockAnchorMismatch()},
		{"inconclusive-no-anchor.json", inconclusiveNoAnchor()},
	}

	for _, ex := range examples {
		stmt, err := agent.BuildReceipt(ex.in)
		if err != nil {
			fmt.Fprintf(os.Stderr, "build %s: %v\n", ex.name, err)
			os.Exit(1)
		}
		buf, err := json.MarshalIndent(stmt, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "marshal %s: %v\n", ex.name, err)
			os.Exit(1)
		}
		path := filepath.Join(outDir, ex.name)
		if err := os.WriteFile(path, append(buf, '\n'), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Println("wrote", path)
	}
}
