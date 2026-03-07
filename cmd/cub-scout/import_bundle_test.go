// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
)

func TestBuildImportFromBundlePreview_BasicContract(t *testing.T) {
	bundleDir := t.TempDir()
	writer := agent.NewBundleWriter("test")
	bundle := &agent.DebugBundle{
		Metadata: agent.BundleMetadata{
			Target: agent.BundleTarget{
				Kind:      "Deployment",
				Name:      "payments-api",
				Namespace: "payments",
				Cluster:   "kind-dev",
			},
		},
		Session: &agent.DebugSessionData{
			Target: agent.BundleTarget{
				Kind:      "Deployment",
				Name:      "payments-api",
				Namespace: "payments",
			},
			StartedAt:   time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC),
			CompletedAt: time.Date(2026, 2, 1, 10, 0, 5, 0, time.UTC),
			WorkloadHealth: &agent.WorkloadHealthSnapshot{
				Kind:          "Deployment",
				Name:          "payments-api",
				Namespace:     "payments",
				Replicas:      3,
				ReadyReplicas: 3,
			},
			OwnershipChain: &agent.OwnershipSnapshot{
				Owner: "ArgoCD",
			},
			DeployerStatus: &agent.DeployerSnapshot{
				Kind:      "Application",
				Name:      "payments-prod",
				Namespace: "argocd",
				Ready:     true,
			},
		},
	}
	if err := writer.Write(bundle, bundleDir); err != nil {
		t.Fatalf("write bundle: %v", err)
	}

	proposal, workloads, namespaces, err := buildImportFromBundlePreview(bundleDir)
	if err != nil {
		t.Fatalf("buildImportFromBundlePreview() error = %v", err)
	}

	if proposal == nil {
		t.Fatal("proposal is nil")
	}
	if proposal.AppSpace == "" {
		t.Error("proposal.appSpace is empty")
	}
	if len(proposal.Units) == 0 {
		t.Error("proposal.units should not be empty")
	}

	if !reflect.DeepEqual(namespaces, []string{"payments"}) {
		t.Fatalf("namespaces = %v, want [payments]", namespaces)
	}

	if len(workloads) != 1 {
		t.Fatalf("len(workloads) = %d, want 1", len(workloads))
	}
	w := workloads[0]
	if w.Kind != "Deployment" || w.Name != "payments-api" || w.Namespace != "payments" {
		t.Fatalf("unexpected workload identity: %+v", w)
	}
	if w.Owner != "ArgoCD" {
		t.Fatalf("workload owner = %q, want ArgoCD", w.Owner)
	}
	if w.Replicas != 3 {
		t.Fatalf("workload replicas = %d, want 3", w.Replicas)
	}
	if !w.Ready {
		t.Fatal("workload ready = false, want true")
	}
}

func TestBuildImportFromBundlePreview_GracefulFallback(t *testing.T) {
	bundleDir := t.TempDir()
	writer := agent.NewBundleWriter("test")
	bundle := &agent.DebugBundle{
		Metadata: agent.BundleMetadata{
			Target: agent.BundleTarget{
				Kind:      "StatefulSet",
				Name:      "ledger",
				Namespace: "finance",
			},
		},
	}
	if err := writer.Write(bundle, bundleDir); err != nil {
		t.Fatalf("write bundle: %v", err)
	}

	proposal, workloads, namespaces, err := buildImportFromBundlePreview(bundleDir)
	if err != nil {
		t.Fatalf("buildImportFromBundlePreview() error = %v", err)
	}
	if proposal == nil {
		t.Fatal("proposal is nil")
	}
	if len(workloads) != 1 {
		t.Fatalf("len(workloads) = %d, want 1", len(workloads))
	}
	if !reflect.DeepEqual(namespaces, []string{"finance"}) {
		t.Fatalf("namespaces = %v, want [finance]", namespaces)
	}

	w := workloads[0]
	if w.Owner != "Native" {
		t.Fatalf("fallback owner = %q, want Native", w.Owner)
	}
	if w.Replicas != 0 {
		t.Fatalf("fallback replicas = %d, want 0", w.Replicas)
	}
	if w.Ready {
		t.Fatal("fallback ready = true, want false")
	}
}

func TestBuildImportFromBundlePreview_InvalidTarget(t *testing.T) {
	bundleDir := t.TempDir()
	writer := agent.NewBundleWriter("test")
	bundle := &agent.DebugBundle{
		Metadata: agent.BundleMetadata{
			Target: agent.BundleTarget{
				Kind:      "Deployment",
				Name:      "",
				Namespace: "payments",
			},
		},
	}
	if err := writer.Write(bundle, bundleDir); err != nil {
		t.Fatalf("write bundle: %v", err)
	}

	_, _, _, err := buildImportFromBundlePreview(bundleDir)
	if err == nil {
		t.Fatal("expected error for invalid target, got nil")
	}
	if !strings.Contains(err.Error(), "target") {
		t.Fatalf("error = %v, want target validation message", err)
	}
}

func TestRunImport_FromBundleJSONContract(t *testing.T) {
	bundleDir := writeImportBundleFixture(t, &agent.DebugBundle{
		Metadata: agent.BundleMetadata{
			Target: agent.BundleTarget{
				Kind:      "Deployment",
				Name:      "orders-api",
				Namespace: "orders",
			},
		},
		Session: &agent.DebugSessionData{
			WorkloadHealth: &agent.WorkloadHealthSnapshot{
				Kind:          "Deployment",
				Name:          "orders-api",
				Namespace:     "orders",
				Replicas:      2,
				ReadyReplicas: 2,
			},
			OwnershipChain: &agent.OwnershipSnapshot{
				Owner: "Flux",
			},
		},
	})

	restore := setImportFlagState(importFlagState{
		fromBundle: bundleDir,
		json:       true,
		noLog:      true,
	})
	defer restore()

	output := captureStdout(t, func() {
		if err := runImport(nil, nil); err != nil {
			t.Fatalf("runImport() error = %v", err)
		}
	})

	var result struct {
		Namespaces []string       `json:"namespaces"`
		Workloads  []WorkloadJSON `json:"workloads"`
		Proposal   *FullProposal  `json:"proposal"`
		Evidence   struct {
			Source     string `json:"source"`
			BundlePath string `json:"bundlePath,omitempty"`
		} `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput=%s", err, output)
	}

	if !reflect.DeepEqual(result.Namespaces, []string{"orders"}) {
		t.Fatalf("namespaces = %v, want [orders]", result.Namespaces)
	}
	if len(result.Workloads) != 1 {
		t.Fatalf("len(workloads) = %d, want 1", len(result.Workloads))
	}
	if result.Workloads[0].Owner != "Flux" {
		t.Fatalf("owner = %q, want Flux", result.Workloads[0].Owner)
	}
	if result.Proposal == nil || result.Proposal.AppSpace == "" {
		t.Fatalf("proposal missing or empty: %+v", result.Proposal)
	}
	if result.Evidence.Source != "bundle" {
		t.Fatalf("evidence.source = %q, want bundle", result.Evidence.Source)
	}
	if result.Evidence.BundlePath != bundleDir {
		t.Fatalf("evidence.bundlePath = %q, want %q", result.Evidence.BundlePath, bundleDir)
	}
}

func TestRunImport_FromBundleNamespaceConflict(t *testing.T) {
	restore := setImportFlagState(importFlagState{
		fromBundle: "/tmp/example-bundle",
		namespace:  "payments",
		noLog:      true,
	})
	defer restore()

	err := runImport(nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "--namespace cannot be used with --from-bundle") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunImport_FromBundleWizardConflict(t *testing.T) {
	restore := setImportFlagState(importFlagState{
		fromBundle: "/tmp/example-bundle",
		wizard:     true,
		noLog:      true,
	})
	defer restore()

	err := runImport(nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "--wizard cannot be used with --from-bundle") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunImport_FromBundleJSONGolden(t *testing.T) {
	restore := setImportFlagState(importFlagState{
		fromBundle: filepath.Join("..", "..", "examples", "workflows", "fleet-demo", "bundles", "dev"),
		json:       true,
		noLog:      true,
	})
	defer restore()

	actualOutput := captureStdout(t, func() {
		if err := runImport(nil, nil); err != nil {
			t.Fatalf("runImport() error = %v", err)
		}
	})

	expectedPath := filepath.Join("..", "..", "examples", "import-from-bundle", "expected-output", "suggestion.json")
	expectedOutput, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("read expected output: %v", err)
	}

	var actual interface{}
	if err := json.Unmarshal([]byte(actualOutput), &actual); err != nil {
		t.Fatalf("parse actual JSON: %v\n%s", err, actualOutput)
	}
	var expected interface{}
	if err := json.Unmarshal(expectedOutput, &expected); err != nil {
		t.Fatalf("parse expected JSON: %v", err)
	}
	normalizeBundlePath(actual)
	normalizeBundlePath(expected)

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("bundle import JSON does not match golden\nexpected=%s\nactual=%s", string(expectedOutput), actualOutput)
	}
}

func TestOutputProposalJSON_ClusterEvidenceSource(t *testing.T) {
	restore := setImportFlagState(importFlagState{
		fromBundle: "",
		noLog:      true,
	})
	defer restore()

	proposal := &FullProposal{
		AppSpace: "test-space",
		Units: []UnitProposal{
			{
				Slug:    "test-unit",
				App:     "test",
				Variant: "default",
				Status:  "cluster-only",
			},
		},
	}
	output := captureStdout(t, func() {
		if err := outputProposalJSON(proposal, nil, []string{"default"}); err != nil {
			t.Fatalf("outputProposalJSON() error = %v", err)
		}
	})

	var result struct {
		Evidence struct {
			Source string `json:"source"`
		} `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("parse JSON: %v\n%s", err, output)
	}
	if result.Evidence.Source != "cluster" {
		t.Fatalf("evidence.source = %q, want cluster", result.Evidence.Source)
	}
}

func TestOutputProposalJSON_ConnectedFallbackFromExistingUnits(t *testing.T) {
	restore := setImportFlagState(importFlagState{
		fromBundle: "",
		noLog:      true,
	})
	defer restore()

	prevLookup := listUnitSlugsForSpace
	listUnitSlugsForSpace = func(space string) (map[string]bool, error) {
		if space != "orders-team" {
			t.Fatalf("space lookup = %q, want orders-team", space)
		}
		return map[string]bool{
			"orders-api": true,
		}, nil
	}
	defer func() {
		listUnitSlugsForSpace = prevLookup
	}()

	proposal := &FullProposal{
		AppSpace: "orders-team",
		Units: []UnitProposal{
			{
				Slug:      "orders-api",
				Workloads: []string{"Deployment/orders/orders-api"},
			},
		},
	}
	workloads := []WorkloadInfo{
		{
			Kind:      "Deployment",
			Namespace: "orders",
			Name:      "orders-api",
			Owner:     "Native",
		},
	}

	output := captureStdout(t, func() {
		if err := outputProposalJSON(proposal, workloads, []string{"orders"}); err != nil {
			t.Fatalf("outputProposalJSON() error = %v", err)
		}
	})

	var result struct {
		Workloads []WorkloadJSON `json:"workloads"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("parse JSON: %v\n%s", err, output)
	}
	if len(result.Workloads) != 1 {
		t.Fatalf("len(workloads) = %d, want 1", len(result.Workloads))
	}
	if !result.Workloads[0].Connected {
		t.Fatal("workloads[0].connected = false, want true")
	}
	if result.Workloads[0].UnitSlug != "orders-api" {
		t.Fatalf("workloads[0].unitSlug = %q, want orders-api", result.Workloads[0].UnitSlug)
	}
}

func TestCanonicalWorkloadRef(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "namespace/name", in: "payments/api", want: "payments/api"},
		{name: "kind/namespace/name", in: "Deployment/payments/api", want: "payments/api"},
		{name: "empty", in: "", want: ""},
		{name: "invalid", in: "payments", want: ""},
		{name: "invalid-empty-segment", in: "Deployment//api", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canonicalWorkloadRef(tt.in)
			if got != tt.want {
				t.Fatalf("canonicalWorkloadRef(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseUnitSlugsFromListJSON(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{
			name: "array with Unit wrapper",
			raw: `[
				{"Unit":{"Slug":"api-dev"}},
				{"Unit":{"Slug":"worker-dev"}}
			]`,
			want: []string{"api-dev", "worker-dev"},
		},
		{
			name: "top-level units key",
			raw: `{
				"units": [
					{"unit":{"slug":"api-prod"}},
					{"slug":"worker-prod"}
				]
			}`,
			want: []string{"api-prod", "worker-prod"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseUnitSlugsFromListJSON([]byte(tt.raw))
			if err != nil {
				t.Fatalf("parseUnitSlugsFromListJSON() error = %v", err)
			}
			for _, slug := range tt.want {
				if !got[slug] {
					t.Fatalf("expected slug %q in %v", slug, got)
				}
			}
		})
	}
}

func TestLinkUnitWorkloadsToCluster(t *testing.T) {
	unit := UnitProposal{
		Slug:      "orders-api",
		Workloads: []string{"orders/api", "Deployment/orders/worker", "bad-ref"},
	}
	workloadIndex := map[string]WorkloadInfo{
		"orders/api": {
			Kind:      "Deployment",
			Namespace: "orders",
			Name:      "api",
		},
		"orders/worker": {
			Kind:      "Deployment",
			Namespace: "orders",
			Name:      "worker",
		},
	}

	var calls []string
	failures := linkUnitWorkloadsToCluster(unit, workloadIndex, func(kind, namespace, name, unitSlug string) error {
		calls = append(calls, fmt.Sprintf("%s/%s/%s->%s", kind, namespace, name, unitSlug))
		if name == "worker" {
			return fmt.Errorf("simulated label failure")
		}
		return nil
	}, nil)

	if failures != 2 {
		t.Fatalf("link failures = %d, want 2", failures)
	}
	if len(calls) != 2 {
		t.Fatalf("label call count = %d, want 2", len(calls))
	}
	if calls[0] != "Deployment/orders/api->orders-api" {
		t.Fatalf("first label call = %q, want Deployment/orders/api->orders-api", calls[0])
	}
	if calls[1] != "Deployment/orders/worker->orders-api" {
		t.Fatalf("second label call = %q, want Deployment/orders/worker->orders-api", calls[1])
	}
}

type importFlagState struct {
	namespace   string
	dryRun      bool
	yes         bool
	json        bool
	noLog       bool
	wizard      bool
	connect     bool
	noConnect   bool
	fromBundle  string
	auditReason string
}

func setImportFlagState(next importFlagState) func() {
	prev := importFlagState{
		namespace:   importNamespace,
		dryRun:      importDryRun,
		yes:         importYes,
		json:        importJSON,
		noLog:       importNoLog,
		wizard:      importWizard,
		connect:     importConnect,
		noConnect:   importNoConnect,
		fromBundle:  importFromBundle,
		auditReason: importAuditReason,
	}

	importNamespace = next.namespace
	importDryRun = next.dryRun
	importYes = next.yes
	importJSON = next.json
	importNoLog = next.noLog
	importWizard = next.wizard
	importConnect = next.connect
	importNoConnect = next.noConnect
	importFromBundle = next.fromBundle
	importAuditReason = next.auditReason

	return func() {
		importNamespace = prev.namespace
		importDryRun = prev.dryRun
		importYes = prev.yes
		importJSON = prev.json
		importNoLog = prev.noLog
		importWizard = prev.wizard
		importConnect = prev.connect
		importNoConnect = prev.noConnect
		importFromBundle = prev.fromBundle
		importAuditReason = prev.auditReason
	}
}

func writeImportBundleFixture(t *testing.T, bundle *agent.DebugBundle) string {
	t.Helper()
	bundleDir := t.TempDir()
	writer := agent.NewBundleWriter("test")
	if err := writer.Write(bundle, bundleDir); err != nil {
		t.Fatalf("write bundle: %v", err)
	}
	return bundleDir
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	originalStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}

	os.Stdout = writePipe
	defer func() {
		os.Stdout = originalStdout
	}()

	fn()

	if err := writePipe.Close(); err != nil {
		t.Fatalf("close write pipe: %v", err)
	}
	out, err := io.ReadAll(readPipe)
	if err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	if err := readPipe.Close(); err != nil {
		t.Fatalf("close read pipe: %v", err)
	}
	return string(out)
}

func normalizeBundlePath(v interface{}) {
	switch typed := v.(type) {
	case map[string]interface{}:
		for key, value := range typed {
			if key == "bundlePath" {
				if _, ok := value.(string); ok {
					typed[key] = "<BUNDLE_PATH>"
				}
				continue
			}
			normalizeBundlePath(value)
		}
	case []interface{}:
		for _, item := range typed {
			normalizeBundlePath(item)
		}
	}
}
