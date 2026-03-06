package main

import (
	"strings"
	"testing"

	"github.com/confighub/cub-scout/internal/scan"
)

func TestBuildDoctorSummary_ComputesCoreSections(t *testing.T) {
	entries := []MapEntry{
		{Kind: "Deployment", Name: "api", Namespace: "prod", Owner: "Flux", Status: "Ready"},
		{Kind: "Service", Name: "api", Namespace: "prod", Owner: "Helm", Status: "Ready"},
		{Kind: "Application", Name: "payments", Namespace: "argocd", Owner: "ArgoCD", Status: "NotReady"},
		{Kind: "ConfigMap", Name: "feature-flags", Namespace: "prod", Owner: "ConfigHub", Status: "Pending"},
		{Kind: "Pod", Name: "debug-shell", Namespace: "prod", Owner: "Native", Status: "Drifted"},
		{Kind: "Deployment", Name: "legacy", Namespace: "default", Owner: "Native", Status: "NotReady"},
	}

	findings := []scan.NormalizedFinding{
		{Severity: "critical", Resource: "Deployment/api", Namespace: "prod", Message: "missing resource limits"},
		{Severity: "warning", Resource: "Deployment/legacy", Namespace: "default", Message: "no probes configured"},
		{Severity: "info", Resource: "ConfigMap/feature-flags", Namespace: "prod", Message: "stale metadata"},
	}

	summary := buildDoctorSummary(entries, findings, "kind-dev", "all", 3)

	if summary.Resources.Total != 6 {
		t.Fatalf("resources total = %d, want 6", summary.Resources.Total)
	}
	if summary.Ownership.Flux != 1 || summary.Ownership.ArgoCD != 1 || summary.Ownership.Helm != 1 {
		t.Fatalf("unexpected ownership core counts: %+v", summary.Ownership)
	}
	if summary.Ownership.Native != 2 || summary.Ownership.Unmanaged != 2 {
		t.Fatalf("unexpected unmanaged/native counts: %+v", summary.Ownership)
	}
	if summary.Health.Healthy != 2 || summary.Health.Warning != 4 || summary.Health.Error != 0 {
		t.Fatalf("unexpected health counts: %+v", summary.Health)
	}
	if summary.Risks.Total != 3 || summary.Risks.Critical != 1 || summary.Risks.Warning != 1 || summary.Risks.Info != 1 {
		t.Fatalf("unexpected risk summary: %+v", summary.Risks)
	}
	if summary.Drift.Resources != 2 {
		t.Fatalf("drift resources = %d, want 2", summary.Drift.Resources)
	}
	if len(summary.TopIssues) != 3 {
		t.Fatalf("top issues len = %d, want 3", len(summary.TopIssues))
	}
	if summary.TopIssues[0].Severity != "CRITICAL" {
		t.Fatalf("first issue severity = %q, want CRITICAL", summary.TopIssues[0].Severity)
	}
}

func TestBuildDoctorSummary_TopIssuesRespectsLimitAndOrdering(t *testing.T) {
	entries := []MapEntry{{Kind: "Deployment", Name: "api", Namespace: "prod", Owner: "Flux", Status: "Ready"}}
	findings := []scan.NormalizedFinding{
		{Severity: "warning", Resource: "Deployment/zeta", Namespace: "prod", Message: "zeta warning"},
		{Severity: "critical", Resource: "Deployment/beta", Namespace: "prod", Message: "beta critical"},
		{Severity: "critical", Resource: "Deployment/alpha", Namespace: "prod", Message: "alpha critical"},
		{Severity: "info", Resource: "Deployment/gamma", Namespace: "prod", Message: "gamma info"},
	}

	summary := buildDoctorSummary(entries, findings, "kind-dev", "all", 2)

	if len(summary.TopIssues) != 2 {
		t.Fatalf("top issues len = %d, want 2", len(summary.TopIssues))
	}
	if summary.TopIssues[0].Resource != "Deployment/alpha" || summary.TopIssues[1].Resource != "Deployment/beta" {
		t.Fatalf("unexpected top issue ordering: %+v", summary.TopIssues)
	}
}

func TestRenderDoctorASCII_ContainsSummarySections(t *testing.T) {
	summary := DoctorSummary{
		Cluster:   "kind-dev",
		Namespace: "all",
		Resources: DoctorResourceSummary{Total: 10},
		Ownership: DoctorOwnershipSummary{Flux: 3, ArgoCD: 2, Helm: 1, Native: 4, Unmanaged: 4},
		Health:    DoctorHealthSummary{Healthy: 7, Warning: 2, Error: 1},
		Risks:     DoctorRiskSummary{Total: 5, Critical: 1, Warning: 3, Info: 1},
		Drift:     DoctorDriftSummary{Resources: 2},
		TopIssues: []DoctorIssue{{Severity: "CRITICAL", Resource: "Deployment/api", Namespace: "prod", Message: "missing limits"}},
	}

	out := renderDoctorASCII(summary)

	required := []string{
		"Cluster: kind-dev (namespace: all)",
		"Resources: 10 total",
		"Ownership:",
		"Health:",
		"Risks: 5 findings (1 CRITICAL, 3 WARNING, 1 INFO)",
		"Drift: 2 resources drifted from declared state",
		"Top Issues:",
	}
	for _, s := range required {
		if !strings.Contains(out, s) {
			t.Fatalf("expected %q in output:\n%s", s, out)
		}
	}
}
