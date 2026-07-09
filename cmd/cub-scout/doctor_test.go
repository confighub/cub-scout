package main

import (
	"strings"
	"testing"

	"github.com/confighub/cub-scout/internal/scan"
	"github.com/confighub/cub-scout/pkg/agent"
)

func TestBuildDoctorSummary_ComputesCoreSections(t *testing.T) {
	entries := []MapEntry{
		{Kind: "Deployment", Name: "api", Namespace: "prod", Owner: "Flux", Status: "Ready"},
		{Kind: "Service", Name: "api", Namespace: "prod", Owner: "Helm", Status: "Ready"},
		{Kind: "Application", Name: "payments", Namespace: "argocd", Owner: "ArgoCD", Status: "NotReady"},
		{Kind: "ClusterProfile", Name: "platform", Namespace: "", Owner: "Sveltos", Status: "Ready"},
		{Kind: "ModelDeployment", Name: "qwen", Namespace: "models", Owner: "Modelplane", Status: "Ready"},
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

	if summary.Resources.Total != 8 {
		t.Fatalf("resources total = %d, want 8", summary.Resources.Total)
	}
	if summary.Ownership.Flux != 1 || summary.Ownership.ArgoCD != 1 || summary.Ownership.Helm != 1 {
		t.Fatalf("unexpected ownership core counts: %+v", summary.Ownership)
	}
	if summary.Ownership.Sveltos != 1 || summary.Ownership.Modelplane != 1 || summary.Ownership.ConfigHub != 1 {
		t.Fatalf("unexpected first-class ownership counts: %+v", summary.Ownership)
	}
	if summary.Ownership.Native != 2 || summary.Ownership.Unmanaged != 2 {
		t.Fatalf("unexpected unmanaged/native counts: %+v", summary.Ownership)
	}
	if summary.Health.Healthy != 4 || summary.Health.Warning != 4 || summary.Health.Error != 0 {
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

func TestBuildDoctorRolloutSummary_CountsAndOrdersCurrentChanges(t *testing.T) {
	decisions := []agent.RolloutDecision{
		doctorRolloutDecision("Deployment", "prod", "watch", agent.VerdictWATCH, agent.RolloutProgressRollingOut, agent.RolloutReasonProgressing),
		doctorRolloutDecision("Deployment", "prod", "pass", agent.VerdictPASS, agent.RolloutProgressComplete, agent.RolloutReasonConverged),
		doctorRolloutDecision("StatefulSet", "prod", "blocked", agent.VerdictBLOCK, agent.RolloutProgressStalled, agent.RolloutReasonRuntimeFailed),
		doctorRolloutDecision("Deployment", "prod", "unknown", agent.VerdictINCONCLUSIVE, agent.RolloutProgressUnknown, agent.RolloutReasonEvidenceMissing),
	}

	summary := buildDoctorRolloutSummary(decisions, 2)
	if summary.Total != 4 || summary.Pass != 1 || summary.Watch != 1 || summary.Block != 1 || summary.Inconclusive != 1 {
		t.Fatalf("unexpected rollout summary: %+v", summary)
	}
	if len(summary.CurrentChanges) != 2 {
		t.Fatalf("current changes = %d, want 2", len(summary.CurrentChanges))
	}
	if summary.CurrentChanges[0].Verdict != agent.VerdictBLOCK {
		t.Fatalf("first current change verdict = %s, want BLOCK", summary.CurrentChanges[0].Verdict)
	}
	if summary.CurrentChanges[1].Verdict != agent.VerdictINCONCLUSIVE {
		t.Fatalf("second current change verdict = %s, want INCONCLUSIVE", summary.CurrentChanges[1].Verdict)
	}
}

func doctorRolloutDecision(kind, namespace, name string, verdict agent.ReceiptVerdict, phase, reason string) agent.RolloutDecision {
	return agent.RolloutDecision{
		Resource: agent.ObjectSetObjectID{
			APIVersion: "apps/v1",
			Kind:       kind,
			Namespace:  namespace,
			Name:       name,
		},
		Verdict: verdict,
		Reason:  reason,
		Progress: agent.RolloutProgress{
			Phase: phase,
		},
	}
}

func TestRenderDoctorASCII_ContainsSummarySections(t *testing.T) {
	// Set NO_COLOR to get plain text output for string matching
	t.Setenv("NO_COLOR", "1")

	summary := DoctorSummary{
		Cluster:   "kind-dev",
		Namespace: "all",
		Resources: DoctorResourceSummary{Total: 10},
		Ownership: DoctorOwnershipSummary{Flux: 3, ArgoCD: 2, Sveltos: 1, Modelplane: 1, Helm: 1, Native: 2, Unmanaged: 2},
		Health:    DoctorHealthSummary{Healthy: 7, Warning: 2, Error: 1},
		Rollouts: &DoctorRolloutSummary{
			Total: 4,
			Pass:  2,
			Watch: 1,
			Block: 1,
			CurrentChanges: []agent.RolloutDecision{
				doctorRolloutDecision("Deployment", "prod", "api", agent.VerdictBLOCK, agent.RolloutProgressStalled, agent.RolloutReasonRuntimeFailed),
			},
		},
		Risks:     DoctorRiskSummary{Total: 5, Critical: 1, Warning: 3, Info: 1},
		Drift:     DoctorDriftSummary{Resources: 2},
		TopIssues: []DoctorIssue{{Severity: "CRITICAL", Resource: "Deployment/api", Namespace: "prod", Message: "missing limits"}},
	}

	out := renderDoctorASCII(summary, DefaultPresentationMode, false, DefaultHintContext())

	required := []string{
		"Cluster: kind-dev (namespace: all)",
		"Resources: 10 total",
		"Ownership:",
		"Sveltos: 1",
		"Modelplane: 1",
		"Health:",
		"Rollouts: 4 workloads",
		"2 PASS",
		"1 WATCH",
		"1 BLOCK",
		"Deployment/api (ns: prod) - BLOCK phase=stalled",
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

func TestRenderDoctorASCII_PresentationModes(t *testing.T) {
	summary := DoctorSummary{
		Cluster:   "kind-dev",
		Namespace: "prod",
		Resources: DoctorResourceSummary{Total: 10},
		Ownership: DoctorOwnershipSummary{Flux: 5, Native: 5, Unmanaged: 5},
		Health:    DoctorHealthSummary{Healthy: 8, Warning: 1, Error: 1},
		Risks:     DoctorRiskSummary{Total: 2, Critical: 1, Warning: 1},
		Drift:     DoctorDriftSummary{Resources: 1},
	}

	tests := []struct {
		mode     PresentationMode
		expected []string
		excluded []string
	}{
		{
			mode: PresentationHuman,
			expected: []string{
				"Cluster Health Summary",
				"Cluster: kind-dev (namespace: prod)",
				"TRY NEXT:",
			},
			excluded: []string{
				"CLUSTER HEALTH SUMMARY",
				"[scope:",
				"RECOMMENDED ACTIONS:",
			},
		},
		{
			mode: PresentationAI,
			expected: []string{
				"CLUSTER HEALTH SUMMARY",
				"[scope: cluster=kind-dev namespace=prod]",
				"OWNERSHIP:",
				"HEALTH:",
				"RISKS:",
				"DRIFT:",
				"RECOMMENDED ACTIONS:",
			},
			excluded: []string{
				"Cluster Health Summary\n", // human heading (newline to avoid matching AI uppercase)
				"Cluster: kind-dev",        // human intro
				"TRY NEXT:",
			},
		},
		{
			mode: PresentationPaired,
			expected: []string{
				"Cluster Health Summary",
				"Cluster: kind-dev (namespace: prod)",
				"TRY NEXT:",
			},
			excluded: []string{
				"CLUSTER HEALTH SUMMARY",
				"[scope:",
				"RECOMMENDED ACTIONS:",
			},
		},
	}

	for _, tc := range tests {
		t.Run(string(tc.mode), func(t *testing.T) {
			// explicitMode=true since we're testing explicit presentation modes
			// Hint context is independent of presentation mode
			out := renderDoctorASCII(summary, tc.mode, true, DefaultHintContext())

			for _, s := range tc.expected {
				if !strings.Contains(out, s) {
					t.Errorf("expected %q in %s mode output:\n%s", s, tc.mode, out)
				}
			}
			for _, s := range tc.excluded {
				if strings.Contains(out, s) {
					t.Errorf("did not expect %q in %s mode output:\n%s", s, tc.mode, out)
				}
			}
		})
	}
}
