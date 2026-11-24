// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNormalizeGitOpsStatusView(t *testing.T) {
	v, err := normalizeGitOpsStatusView("", false, "ascii")
	if err != nil || v != gitopsStatusViewAll {
		t.Fatalf("default view = (%q, %v), want (all, nil)", v, err)
	}
	v, err = normalizeGitOpsStatusView("summary", true, "json")
	if err != nil || v != gitopsStatusViewSummary {
		t.Fatalf("summary json = (%q, %v), want (summary, nil)", v, err)
	}
	if _, err := normalizeGitOpsStatusView("summary", true, "ascii"); err == nil || !strings.Contains(err.Error(), "--format json") {
		t.Fatalf("expected --format json error for --view on ascii, got %v", err)
	}
	if _, err := normalizeGitOpsStatusView("bogus", true, "json"); err == nil || !strings.Contains(err.Error(), "valid: all, summary") {
		t.Fatalf("expected valid-values error for bogus view, got %v", err)
	}
}

func TestProjectGitOpsStatusViewSummaryPreservesAnswerAndShrinksPayload(t *testing.T) {
	full := GitOpsSummary{
		Context:      "prod-east",
		Backend:      "flux",
		Transport:    "oci",
		HealthyCount: 2,
		FailedCount:  1,
		Deployers: []DeployerStatus{
			{Kind: "Kustomization", Name: "apps", Namespace: "flux-system", Ready: true, Stage: "healthy", Reason: "ReconciliationSucceeded", Message: "Applied revision: main@sha1:abc123", LastAppliedRevision: "main@sha1:abc123"},
			{Kind: "Kustomization", Name: "infra", Namespace: "flux-system", Ready: true, Stage: "healthy", Reason: "ReconciliationSucceeded", Message: "Applied revision: main@sha1:def456", LastAppliedRevision: "main@sha1:def456"},
			{Kind: "HelmRelease", Name: "payments", Namespace: "payments", Ready: false, Stage: "apply", Reason: "UpgradeFailed", Message: "Helm upgrade failed: timed out waiting for the condition"},
		},
		Sources: []SourceStatus{
			{Kind: "OCIRepository", Name: "apps-oci", Namespace: "flux-system", Ready: true, URL: "oci://ghcr.io/example/apps:latest", ArtifactDigest: "sha256:0828"},
			{Kind: "GitRepository", Name: "infra-git", Namespace: "flux-system", Ready: true, URL: "https://github.com/example/infra", ArtifactDigest: "sha256:9999"},
		},
		ControllerCoverage: []ControllerCoverageStatus{
			{Family: "flux", Status: controllerCoverageFound, Found: 3, FoundKinds: []string{"Kustomization", "HelmRelease"}},
			{Family: "argocd", Status: controllerCoverageNotFound, Found: 0},
		},
	}
	full.Answer = buildGitOpsStatusAnswer(full, time.Date(2026, 11, 24, 18, 0, 0, 0, time.UTC), nil)

	compact := projectGitOpsStatusView(full, gitopsStatusViewSummary)

	if len(compact.Deployers) != 0 || len(compact.Sources) != 0 || compact.SveltosControllerReports != nil || compact.DeliveryEvidence != nil {
		t.Fatalf("summary view must omit heavy arrays, got deployers=%d sources=%d", len(compact.Deployers), len(compact.Sources))
	}
	if compact.Backend != full.Backend || compact.Transport != full.Transport || compact.HealthyCount != full.HealthyCount || compact.FailedCount != full.FailedCount || len(compact.ControllerCoverage) != len(full.ControllerCoverage) {
		t.Fatalf("summary view lost top-level counts/coverage: %+v", compact)
	}

	fullAnswer, err := json.Marshal(full.Answer)
	if err != nil {
		t.Fatal(err)
	}
	compactAnswer, err := json.Marshal(compact.Answer)
	if err != nil {
		t.Fatal(err)
	}
	if string(fullAnswer) != string(compactAnswer) {
		t.Fatalf("summary view changed answer:\nfull:    %s\ncompact: %s", fullAnswer, compactAnswer)
	}

	fullBytes, err := json.Marshal(full)
	if err != nil {
		t.Fatal(err)
	}
	compactBytes, err := json.Marshal(compact)
	if err != nil {
		t.Fatal(err)
	}
	if len(compactBytes) >= len(fullBytes) {
		t.Fatalf("summary view (%d B) did not shrink full view (%d B)", len(compactBytes), len(fullBytes))
	}
}

func TestGitOpsStatusMCPViewArg(t *testing.T) {
	gw := newMCPGateway(nil)
	tool, ok := gw.tools["gitops_status"]
	if !ok {
		t.Fatal("gitops_status MCP tool not registered")
	}
	args, err := tool.BuildArgs(map[string]interface{}{"namespace": "flux-system", "view": "summary"})
	if err != nil {
		t.Fatalf("BuildArgs failed: %v", err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"gitops status --format json", "-n flux-system", "--view summary"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
}
