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
	if proposal.App == "" {
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
	if result.Proposal == nil || result.Proposal.App == "" {
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
		App: "test-space",
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
		App: "orders-team",
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
	resources   []string
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
		resources:   importResources,
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
	importResources = next.resources

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
		importResources = prev.resources
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

// === Curated Selection Tests ===

func TestParseImportResourceSelections_Valid(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		wantLen  int
		wantKind string
		wantName string
	}{
		{
			name:     "deploy/name",
			input:    []string{"deploy/api"},
			wantLen:  1,
			wantKind: "Deployment",
			wantName: "api",
		},
		{
			name:     "statefulset/name",
			input:    []string{"statefulset/db"},
			wantLen:  1,
			wantKind: "StatefulSet",
			wantName: "db",
		},
		{
			name:     "daemonset/name",
			input:    []string{"ds/monitor"},
			wantLen:  1,
			wantKind: "DaemonSet",
			wantName: "monitor",
		},
		{
			name:    "multiple resources",
			input:   []string{"deploy/api", "statefulset/db"},
			wantLen: 2,
		},
		{
			name:     "case insensitive kind",
			input:    []string{"DEPLOY/api"},
			wantLen:  1,
			wantKind: "Deployment",
			wantName: "api",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseImportResourceSelections(tt.input)
			if err != nil {
				t.Fatalf("parseImportResourceSelections() error = %v", err)
			}
			if len(got) != tt.wantLen {
				t.Fatalf("len(selections) = %d, want %d", len(got), tt.wantLen)
			}
			if tt.wantKind != "" && got[0].Kind != tt.wantKind {
				t.Fatalf("kind = %q, want %q", got[0].Kind, tt.wantKind)
			}
			if tt.wantName != "" && got[0].Name != tt.wantName {
				t.Fatalf("name = %q, want %q", got[0].Name, tt.wantName)
			}
		})
	}
}

func TestParseImportResourceSelections_Dedupe(t *testing.T) {
	input := []string{"deploy/api", "deploy/api", "DEPLOY/API"}
	got, err := parseImportResourceSelections(input)
	if err != nil {
		t.Fatalf("parseImportResourceSelections() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(selections) = %d, want 1 (deduped)", len(got))
	}
}

func TestParseImportResourceSelections_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input []string
	}{
		{name: "missing slash", input: []string{"deployapi"}},
		{name: "empty name", input: []string{"deploy/"}},
		{name: "invalid kind", input: []string{"pod/mypod"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseImportResourceSelections(tt.input)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestParseImportResourceSelections_EmptyInput(t *testing.T) {
	got, err := parseImportResourceSelections(nil)
	if err != nil {
		t.Fatalf("parseImportResourceSelections(nil) error = %v", err)
	}
	if got != nil {
		t.Fatalf("got %v, want nil", got)
	}

	got, err = parseImportResourceSelections([]string{})
	if err != nil {
		t.Fatalf("parseImportResourceSelections([]) error = %v", err)
	}
	if got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

func TestFilterWorkloadsBySelection_AllMatched(t *testing.T) {
	workloads := []WorkloadInfo{
		{Kind: "Deployment", Namespace: "payments", Name: "api", Owner: "Native"},
		{Kind: "StatefulSet", Namespace: "payments", Name: "db", Owner: "Helm"},
	}
	selections := []importResourceSelection{
		{Kind: "Deployment", Name: "api"},
		{Kind: "StatefulSet", Name: "db"},
	}

	filtered, result := filterWorkloadsBySelection(workloads, selections, "payments")

	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2", len(filtered))
	}
	if len(result.Included) != 2 {
		t.Fatalf("len(included) = %d, want 2", len(result.Included))
	}
	if len(result.Missing) != 0 {
		t.Fatalf("len(missing) = %d, want 0", len(result.Missing))
	}
	if len(result.Skipped) != 0 {
		t.Fatalf("len(skipped) = %d, want 0", len(result.Skipped))
	}
}

func TestFilterWorkloadsBySelection_PartialMatch(t *testing.T) {
	workloads := []WorkloadInfo{
		{Kind: "Deployment", Namespace: "payments", Name: "api", Owner: "Native"},
		{Kind: "Deployment", Namespace: "payments", Name: "worker", Owner: "Native"},
	}
	selections := []importResourceSelection{
		{Kind: "Deployment", Name: "api"},
	}

	filtered, result := filterWorkloadsBySelection(workloads, selections, "payments")

	if len(filtered) != 1 {
		t.Fatalf("len(filtered) = %d, want 1", len(filtered))
	}
	if filtered[0].Name != "api" {
		t.Fatalf("filtered[0].name = %q, want api", filtered[0].Name)
	}
	if len(result.Included) != 1 {
		t.Fatalf("len(included) = %d, want 1", len(result.Included))
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("len(skipped) = %d, want 1", len(result.Skipped))
	}
	if result.Skipped[0].Name != "worker" {
		t.Fatalf("skipped[0].name = %q, want worker", result.Skipped[0].Name)
	}
}

func TestFilterWorkloadsBySelection_MissingResource(t *testing.T) {
	workloads := []WorkloadInfo{
		{Kind: "Deployment", Namespace: "payments", Name: "api", Owner: "Native"},
	}
	selections := []importResourceSelection{
		{Kind: "Deployment", Name: "api"},
		{Kind: "StatefulSet", Name: "db"},
	}

	filtered, result := filterWorkloadsBySelection(workloads, selections, "payments")

	if len(filtered) != 1 {
		t.Fatalf("len(filtered) = %d, want 1", len(filtered))
	}
	if len(result.Missing) != 1 {
		t.Fatalf("len(missing) = %d, want 1", len(result.Missing))
	}
	if result.Missing[0].Kind != "StatefulSet" || result.Missing[0].Name != "db" {
		t.Fatalf("missing[0] = %+v, want StatefulSet/db", result.Missing[0])
	}
}

func TestFilterWorkloadsBySelection_GitOpsUnsupported(t *testing.T) {
	workloads := []WorkloadInfo{
		{Kind: "Deployment", Namespace: "payments", Name: "api", Owner: "ArgoCD"},
		{Kind: "Deployment", Namespace: "payments", Name: "worker", Owner: "Flux"},
		{Kind: "StatefulSet", Namespace: "payments", Name: "db", Owner: "Helm"},
	}
	selections := []importResourceSelection{
		{Kind: "Deployment", Name: "api"},
		{Kind: "Deployment", Name: "worker"},
		{Kind: "StatefulSet", Name: "db"},
	}

	filtered, result := filterWorkloadsBySelection(workloads, selections, "payments")

	// Only the Helm workload should be included
	if len(filtered) != 1 {
		t.Fatalf("len(filtered) = %d, want 1", len(filtered))
	}
	if filtered[0].Name != "db" {
		t.Fatalf("filtered[0].name = %q, want db", filtered[0].Name)
	}

	// ArgoCD and Flux should be unsupported
	if len(result.Unsupported) != 2 {
		t.Fatalf("len(unsupported) = %d, want 2", len(result.Unsupported))
	}
	for _, u := range result.Unsupported {
		if !strings.Contains(u.Reason, "GitOps-managed") {
			t.Fatalf("unsupported reason should mention GitOps-managed: %q", u.Reason)
		}
	}

	if len(result.Included) != 1 {
		t.Fatalf("len(included) = %d, want 1", len(result.Included))
	}
}

func TestFilterWorkloadsBySelection_SelectedTracking(t *testing.T) {
	workloads := []WorkloadInfo{
		{Kind: "Deployment", Namespace: "payments", Name: "api", Owner: "Native"},
	}
	selections := []importResourceSelection{
		{Kind: "Deployment", Name: "api"},
		{Kind: "StatefulSet", Name: "db"},
	}

	_, result := filterWorkloadsBySelection(workloads, selections, "payments")

	if len(result.Selected) != 2 {
		t.Fatalf("len(selected) = %d, want 2", len(result.Selected))
	}
	if result.Selected[0] != "Deployment/api" {
		t.Fatalf("selected[0] = %q, want Deployment/api", result.Selected[0])
	}
	if result.Selected[1] != "StatefulSet/db" {
		t.Fatalf("selected[1] = %q, want StatefulSet/db", result.Selected[1])
	}
}

func TestRunImport_ResourceRequiresNamespace(t *testing.T) {
	restore := setImportFlagState(importFlagState{
		namespace: "",
		resources: []string{"deploy/api"},
		noLog:     true,
	})
	defer restore()

	err := runImport(nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "--resource requires --namespace") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunImport_ResourceWithBundleAllowed(t *testing.T) {
	bundleDir := writeImportBundleFixture(t, &agent.DebugBundle{
		Metadata: agent.BundleMetadata{
			Target: agent.BundleTarget{
				Kind:      "Deployment",
				Name:      "api",
				Namespace: "payments",
			},
		},
		Session: &agent.DebugSessionData{
			WorkloadHealth: &agent.WorkloadHealthSnapshot{
				Kind:          "Deployment",
				Name:          "api",
				Namespace:     "payments",
				Replicas:      2,
				ReadyReplicas: 2,
			},
			OwnershipChain: &agent.OwnershipSnapshot{
				Owner: "Native",
			},
		},
	})

	restore := setImportFlagState(importFlagState{
		fromBundle: bundleDir,
		resources:  []string{"deploy/api"},
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
		Selection *ImportSelectionResult `json:"selection"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("parse JSON: %v\n%s", err, output)
	}

	if result.Selection == nil {
		t.Fatal("selection is nil")
	}
	if len(result.Selection.Selected) != 1 {
		t.Fatalf("len(selected) = %d, want 1", len(result.Selection.Selected))
	}
	if len(result.Selection.Included) != 1 {
		t.Fatalf("len(included) = %d, want 1", len(result.Selection.Included))
	}
}

func TestRunImport_CuratedSelectionJSONContract(t *testing.T) {
	bundleDir := writeImportBundleFixture(t, &agent.DebugBundle{
		Metadata: agent.BundleMetadata{
			Target: agent.BundleTarget{
				Kind:      "Deployment",
				Name:      "api",
				Namespace: "payments",
			},
		},
		Session: &agent.DebugSessionData{
			WorkloadHealth: &agent.WorkloadHealthSnapshot{
				Kind:          "Deployment",
				Name:          "api",
				Namespace:     "payments",
				Replicas:      2,
				ReadyReplicas: 2,
			},
			OwnershipChain: &agent.OwnershipSnapshot{
				Owner: "ArgoCD", // GitOps-managed to test unsupported
			},
		},
	})

	restore := setImportFlagState(importFlagState{
		fromBundle: bundleDir,
		resources:  []string{"deploy/api", "statefulset/missing"},
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
		Selection *ImportSelectionResult `json:"selection"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("parse JSON: %v\n%s", err, output)
	}

	if result.Selection == nil {
		t.Fatal("selection is nil")
	}

	// Selected should have both
	if len(result.Selection.Selected) != 2 {
		t.Fatalf("len(selected) = %d, want 2", len(result.Selection.Selected))
	}

	// ArgoCD workload should be unsupported
	if len(result.Selection.Unsupported) != 1 {
		t.Fatalf("len(unsupported) = %d, want 1", len(result.Selection.Unsupported))
	}
	if result.Selection.Unsupported[0].Name != "api" {
		t.Fatalf("unsupported[0].name = %q, want api", result.Selection.Unsupported[0].Name)
	}

	// statefulset/missing should be missing
	if len(result.Selection.Missing) != 1 {
		t.Fatalf("len(missing) = %d, want 1", len(result.Selection.Missing))
	}
	if result.Selection.Missing[0].Name != "missing" {
		t.Fatalf("missing[0].name = %q, want missing", result.Selection.Missing[0].Name)
	}
}

func TestOutputCuratedSelectionJSON_Structure(t *testing.T) {
	proposal := &FullProposal{
		App: "test-space",
		Units: []UnitProposal{
			{Slug: "api-unit"},
		},
	}
	workloads := []WorkloadInfo{
		{Kind: "Deployment", Namespace: "payments", Name: "api", Owner: "Native"},
	}
	selection := &ImportSelectionResult{
		Selected: []string{"Deployment/api"},
		Included: []ImportSelectionEntry{
			{Kind: "Deployment", Namespace: "payments", Name: "api"},
		},
	}

	restore := setImportFlagState(importFlagState{noLog: true})
	defer restore()

	output := captureStdout(t, func() {
		if err := outputCuratedSelectionJSON(proposal, workloads, []string{"payments"}, selection); err != nil {
			t.Fatalf("outputCuratedSelectionJSON() error = %v", err)
		}
	})

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("parse JSON: %v\n%s", err, output)
	}

	// Check all expected fields exist
	requiredFields := []string{"namespaces", "workloads", "proposal", "evidence", "selection"}
	for _, field := range requiredFields {
		if _, ok := result[field]; !ok {
			t.Fatalf("missing required field %q in JSON output", field)
		}
	}

	// Check selection structure
	sel, ok := result["selection"].(map[string]interface{})
	if !ok {
		t.Fatal("selection is not an object")
	}
	if _, ok := sel["selected"]; !ok {
		t.Fatal("selection.selected is missing")
	}
	if _, ok := sel["included"]; !ok {
		t.Fatal("selection.included is missing")
	}
}
