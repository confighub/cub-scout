package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
)

func TestBuildExplainSummary_FromTraceResult(t *testing.T) {
	now := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	result := &agent.TraceResult{
		Tool: "flux",
		Object: agent.ResourceRef{
			Kind:      "Deployment",
			Name:      "payments-api",
			Namespace: "prod",
		},
		Chain: []agent.ChainLink{
			{
				Kind:      "GitRepository",
				Name:      "platform-config",
				Namespace: "flux-system",
				URL:       "https://github.com/acme/platform-config.git",
				Revision:  "main@sha1:abc1234",
				Ready:     true,
				Status:    "Ready",
			},
			{
				Kind:      "Kustomization",
				Name:      "payments",
				Namespace: "flux-system",
				Path:      "./apps/payments",
				Revision:  "main@sha1:abc1234",
				Ready:     true,
				Status:    "Ready",
			},
			{
				Kind:      "Deployment",
				Name:      "payments-api",
				Namespace: "prod",
				Ready:     true,
				Status:    "Healthy",
			},
		},
		TracedAt: now,
	}

	summary := buildExplainSummary(result)

	if summary.Owner != "Flux" {
		t.Fatalf("owner = %q, want Flux", summary.Owner)
	}
	if !strings.Contains(summary.Source, "https://github.com/acme/platform-config.git") {
		t.Fatalf("source missing git URL: %q", summary.Source)
	}
	if !strings.Contains(summary.Source, "./apps/payments") {
		t.Fatalf("source missing kustomize path: %q", summary.Source)
	}
	if !strings.Contains(summary.DeployedVia, "GitRepository/platform-config") || !strings.Contains(summary.DeployedVia, "Kustomization/payments") {
		t.Fatalf("deployed-via chain incomplete: %q", summary.DeployedVia)
	}
	if !strings.Contains(summary.Health, "Healthy") {
		t.Fatalf("health = %q, want Healthy", summary.Health)
	}
}

func TestBuildExplainSummary_ConfigHubURLsAndRevisionFacts(t *testing.T) {
	result := &agent.TraceResult{
		Tool: "argocd",
		Object: agent.ResourceRef{
			Kind:      "Deployment",
			Name:      "payments-api",
			Namespace: "prod",
		},
		ConfigHub: &agent.TraceConfigHub{
			UnitSlug:        "payments-api",
			UnitID:          "u-123",
			SpaceID:         "sp-123",
			RevisionNum:     "8",
			LiveRevisionNum: "9",
			UnitURL:         "https://confighub.com/units/sp-123/u-123",
			RevisionsURL:    "https://confighub.com/units/sp-123/u-123?tab=2",
			RemediationURL:  "https://confighub.com/units/sp-123/u-123",
		},
	}

	summary := buildExplainSummary(result)
	if summary.ConfigHubURL != "https://confighub.com/units/sp-123/u-123" {
		t.Fatalf("ConfigHubURL = %q, want canonical unit url", summary.ConfigHubURL)
	}
	if summary.ConfigHubRevisionsURL != "https://confighub.com/units/sp-123/u-123?tab=2" {
		t.Fatalf("ConfigHubRevisionsURL = %q, want canonical revisions url", summary.ConfigHubRevisionsURL)
	}
	if summary.ConfigHubRevisionNum != "8" || summary.ConfigHubLiveRevisionNum != "9" {
		t.Fatalf("revision facts = %q/%q, want 8/9", summary.ConfigHubRevisionNum, summary.ConfigHubLiveRevisionNum)
	}
}

func TestRenderExplainText_ContainsPlainEnglishSections(t *testing.T) {
	// Set NO_COLOR to get plain text output for string matching
	t.Setenv("NO_COLOR", "1")

	summary := ExplainSummary{
		Resource:    "Deployment/payments-api",
		Namespace:   "prod",
		Owner:       "ArgoCD",
		Source:      "git@github.com:acme/platform.git (path: apps/payments, revision: main@abc1234)",
		DeployedVia: "Application/payments -> HelmRelease/payments -> Deployment/payments-api",
		Health:      "Healthy",
		Risks:       "1 WARNING",
		Drift:       "None detected",
	}

	out := renderExplainText(summary, DefaultPresentationMode, false, DefaultHintContext())

	required := []string{
		"Deployment/payments-api in namespace prod:",
		"Owner: ArgoCD",
		"Source: git@github.com:acme/platform.git",
		"Deployed via: Application/payments -> HelmRelease/payments -> Deployment/payments-api",
		"Health: Healthy",
		"Risks: 1 WARNING",
		"Drift: None detected",
	}
	for _, s := range required {
		if !strings.Contains(out, s) {
			t.Fatalf("expected %q in explain text:\n%s", s, out)
		}
	}
}

func TestRenderExplainMarkdown_ContainsHeadingsAndFields(t *testing.T) {
	summary := ExplainSummary{
		Resource:    "Deployment/payments-api",
		Namespace:   "prod",
		Owner:       "Flux",
		Source:      "https://github.com/acme/platform-config.git (path: ./apps/payments, revision: main@abc1234)",
		DeployedVia: "GitRepository/platform-config -> Kustomization/payments -> Deployment/payments-api",
		Health:      "Healthy",
		Risks:       "0 findings",
		Drift:       "None detected",
	}

	out := renderExplainMarkdown(summary, DefaultPresentationMode, false, DefaultHintContext())

	required := []string{
		"## Explain",
		"- **Resource:** `Deployment/payments-api`",
		"- **Namespace:** `prod`",
		"- **Owner:** Flux",
		"- **Source:** https://github.com/acme/platform-config.git",
	}
	for _, s := range required {
		if !strings.Contains(out, s) {
			t.Fatalf("expected %q in explain markdown:\n%s", s, out)
		}
	}
}

func TestBuildExplainSummary_UnknownOwnerUsesExplicitMessage(t *testing.T) {
	result := &agent.TraceResult{
		Object: agent.ResourceRef{
			Kind:      "Deployment",
			Name:      "legacy-api",
			Namespace: "default",
		},
		Error: "resource not managed by any detected GitOps tool",
	}

	summary := buildExplainSummary(result)

	if summary.Owner != "Unknown - no recognized ownership labels found" {
		t.Fatalf("owner = %q, want explicit unknown-owner message", summary.Owner)
	}
	if len(summary.Notes) == 0 {
		t.Fatalf("expected partial-trace notes for unknown owner")
	}
}

func TestRenderExplainText_IncludesPartialTraceNotes(t *testing.T) {
	// Set NO_COLOR to get plain text output for string matching
	t.Setenv("NO_COLOR", "1")

	summary := ExplainSummary{
		Resource:    "Deployment/legacy-api",
		Namespace:   "default",
		Owner:       "Unknown - no recognized ownership labels found",
		Source:      "unknown",
		DeployedVia: "partial trace only",
		Health:      "Unavailable",
		Risks:       "Not assessed",
		Drift:       "Unknown",
		Notes: []string{
			"partial trace: no GitOps owner chain was discovered",
		},
	}

	out := renderExplainText(summary, DefaultPresentationMode, false, DefaultHintContext())
	if !strings.Contains(out, "Notes:") {
		t.Fatalf("expected Notes section in explain text:\\n%s", out)
	}
	if !strings.Contains(out, "partial trace: no GitOps owner chain was discovered") {
		t.Fatalf("expected partial-trace note in explain text:\\n%s", out)
	}
}

func TestExplainOwner_Custom(t *testing.T) {
	result := &agent.TraceResult{
		Object: agent.ResourceRef{
			Kind:      "Deployment",
			Name:      "platform-api",
			Namespace: "default",
		},
		Error: "custom owner detected: Internal Platform (trace chain unavailable for custom owners)",
	}

	summary := buildExplainSummary(result)
	if summary.Owner != "Internal Platform" {
		t.Fatalf("owner = %q, want %q", summary.Owner, "Internal Platform")
	}
}

func TestRenderExplainText_PresentationModes(t *testing.T) {
	summary := ExplainSummary{
		Resource:    "Deployment/payments-api",
		Namespace:   "prod",
		Owner:       "Flux",
		Source:      "https://github.com/acme/platform-config.git",
		DeployedVia: "GitRepository/platform-config -> Kustomization/payments -> Deployment/payments-api",
		Health:      "Healthy",
		Risks:       "0 findings",
		Drift:       "None detected",
	}

	tests := []struct {
		mode     PresentationMode
		expected []string
		excluded []string
	}{
		{
			mode: PresentationHuman,
			expected: []string{
				"Deployment/payments-api in namespace prod:",
				"Owner:",
				"Source:",
				"TRY NEXT:",
			},
			excluded: []string{
				"[resource:",
				"OWNER:",
				"RECOMMENDED ACTIONS:",
			},
		},
		{
			mode: PresentationAI,
			expected: []string{
				"[resource: Deployment/payments-api namespace: prod]",
				"OWNER:",
				"SOURCE:",
				"HEALTH:",
				"RECOMMENDED ACTIONS:",
			},
			excluded: []string{
				"Deployment/payments-api in namespace prod:",
				"TRY NEXT:",
			},
		},
		{
			mode: PresentationPaired,
			expected: []string{
				"Deployment/payments-api in namespace prod:",
				"Owner:",
				"TRY NEXT:",
			},
			excluded: []string{
				"[resource:",
				"OWNER:",
				"RECOMMENDED ACTIONS:",
			},
		},
	}

	for _, tc := range tests {
		t.Run(string(tc.mode), func(t *testing.T) {
			// explicitMode=true since we're testing explicit presentation modes
			// Hint context is independent of presentation mode
			out := renderExplainText(summary, tc.mode, true, DefaultHintContext())

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

// TestBuildExplainSummary_ArgoOwnershipPreserved tests the fix for #365:
// when ownership is detected via ArgoCD tracking-id but the tracer fails,
// the Owner should still show "ArgoCD" not "Unknown".
func TestBuildExplainSummary_ArgoOwnershipPreserved(t *testing.T) {
	// This simulates the case where:
	// 1. detectResourceOwnership found ArgoCD via tracking-id annotation
	// 2. ArgoTracer couldn't complete a full chain (it returns error for non-Application resources)
	// 3. buildOwnershipOnlyTraceResult created a partial result with Tool="argocd"
	result := &agent.TraceResult{
		Object: agent.ResourceRef{
			Kind:      "Deployment",
			Name:      "frontend",
			Namespace: "cubbychat",
		},
		Tool:  "argocd",
		Error: "ownership detected via annotation:argocd.argoproj.io/tracking-id; owner: cubbychat; full trace chain unavailable",
	}

	summary := buildExplainSummary(result)

	// The key assertion: Owner should be "ArgoCD", not "Unknown"
	if summary.Owner != "ArgoCD" {
		t.Fatalf("owner = %q, want ArgoCD (ownership should be preserved when detected)", summary.Owner)
	}

	// Verify other expected fields for partial traces
	if summary.DeployedVia != "partial trace only" {
		t.Fatalf("deployedVia = %q, want 'partial trace only' for incomplete traces", summary.DeployedVia)
	}
	if len(summary.Notes) == 0 || !strings.Contains(summary.Notes[0], "partial trace") {
		t.Fatalf("expected partial trace note, got notes: %v", summary.Notes)
	}
}

func TestProjectArgoApplicationTraceToResource(t *testing.T) {
	result := &agent.TraceResult{
		Object: agent.ResourceRef{
			Kind:      "Application",
			Name:      "demo-roundtrip-cubbychat-wet",
			Namespace: "argocd",
		},
		Tool: "argocd",
		Chain: []agent.ChainLink{
			{Kind: "OCIRepository", Name: "demo-roundtrip/cubbychat-wet", URL: "oci://oci.hub.confighub.com/unit/demo-roundtrip/cubbychat-wet", Revision: "latest"},
			{Kind: "Application", Name: "demo-roundtrip-cubbychat-wet", Namespace: "argocd", Status: "Synced / Healthy", Ready: true},
			{Kind: "Service", Name: "frontend", Namespace: "default", Status: "Synced", Ready: true},
			{Kind: "Deployment", Name: "frontend", Namespace: "default", Status: "Synced / Healthy", Ready: true},
		},
	}

	projected := projectArgoApplicationTraceToResource(result, "Deployment", "frontend", "default")
	if projected == nil {
		t.Fatal("projected result is nil")
	}
	if projected.Object.Kind != "Deployment" || projected.Object.Name != "frontend" || projected.Object.Namespace != "default" {
		t.Fatalf("projected object = %+v, want Deployment/frontend default", projected.Object)
	}
	if len(projected.Chain) != 3 {
		t.Fatalf("projected chain len = %d, want 3", len(projected.Chain))
	}
	if projected.Chain[0].Kind != "OCIRepository" {
		t.Fatalf("chain[0].Kind = %q, want OCIRepository", projected.Chain[0].Kind)
	}
	if projected.Chain[1].Kind != "Application" {
		t.Fatalf("chain[1].Kind = %q, want Application", projected.Chain[1].Kind)
	}
	if projected.Chain[2].Kind != "Deployment" || projected.Chain[2].Name != "frontend" {
		t.Fatalf("chain[2] = %+v, want Deployment/frontend", projected.Chain[2])
	}
}

func TestTraceOwnedArgoResourceForExplain_UsesApplicationTrace(t *testing.T) {
	ownership := &agent.Ownership{
		Type:   agent.OwnerArgo,
		Name:   "demo-roundtrip-cubbychat-wet",
		Source: "annotation:argocd.argoproj.io/tracking-id",
	}

	called := false
	result, err, attempted := traceOwnedArgoResourceForExplain(
		context.Background(),
		"Deployment",
		"frontend",
		"default",
		ownership,
		func(ctx context.Context, appName string) (*agent.TraceResult, error) {
			called = true
			if appName != "demo-roundtrip-cubbychat-wet" {
				t.Fatalf("appName = %q, want demo-roundtrip-cubbychat-wet", appName)
			}
			return &agent.TraceResult{
				Object: agent.ResourceRef{
					Kind:      "Application",
					Name:      appName,
					Namespace: "argocd",
				},
				Tool:  "argocd",
				Error: "ArgoCD CLI unavailable - showing kubectl-based Application trace only. Run 'argocd login <server>' for full CLI-backed trace context.",
				Chain: []agent.ChainLink{
					{Kind: "OCIRepository", Name: "demo-roundtrip/cubbychat-wet", URL: "oci://oci.hub.confighub.com:443/unit/demo-roundtrip/cubbychat-wet", Path: ".", Revision: "latest"},
					{Kind: "Application", Name: appName, Namespace: "argocd", Status: "Synced / Healthy", Ready: true},
					{Kind: "Deployment", Name: "frontend", Namespace: "default", Status: "Synced", Ready: true},
				},
			}, nil
		},
	)
	if !attempted {
		t.Fatal("attempted = false, want true")
	}
	if !called {
		t.Fatal("expected app trace function to be called")
	}
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if result.Object.Kind != "Deployment" || result.Object.Name != "frontend" {
		t.Fatalf("result.Object = %+v, want Deployment/frontend", result.Object)
	}
	summary := buildExplainSummary(result)
	if summary.Owner != "ArgoCD" {
		t.Fatalf("summary.Owner = %q, want ArgoCD", summary.Owner)
	}
	if summary.Source == "unknown" {
		t.Fatalf("summary.Source = %q, want concrete source from Argo application trace", summary.Source)
	}
	if !strings.Contains(summary.DeployedVia, "Application/demo-roundtrip-cubbychat-wet") || !strings.Contains(summary.DeployedVia, "Deployment/frontend") {
		t.Fatalf("summary.DeployedVia = %q, want projected Application -> Deployment chain", summary.DeployedVia)
	}
	if !strings.Contains(summary.Health, "Synced") {
		t.Fatalf("summary.Health = %q, want Synced", summary.Health)
	}
	if len(summary.Notes) == 0 || !strings.Contains(summary.Notes[0], "ArgoCD CLI unavailable") {
		t.Fatalf("summary.Notes = %v, want degraded Argo trace note", summary.Notes)
	}
}

// TestBuildOwnershipOnlyTraceResult verifies the helper that creates
// partial trace results when ownership is detected but tracer fails.
func TestBuildOwnershipOnlyTraceResult(t *testing.T) {
	tests := []struct {
		name          string
		ownership     *agent.Ownership
		traceErr      error
		expectedTool  string
		expectedOwner string // what explainOwner(result.Tool) should return
	}{
		{
			name: "ArgoCD via tracking-id",
			ownership: &agent.Ownership{
				Type:   agent.OwnerArgo,
				Name:   "cubbychat",
				Source: "annotation:argocd.argoproj.io/tracking-id",
			},
			traceErr:      nil,
			expectedTool:  "argocd",
			expectedOwner: "ArgoCD",
		},
		{
			name: "Flux via kustomization label",
			ownership: &agent.Ownership{
				Type:   agent.OwnerFlux,
				Name:   "apps",
				Source: "label:kustomize.toolkit.fluxcd.io/name",
			},
			traceErr:      nil,
			expectedTool:  "flux",
			expectedOwner: "Flux",
		},
		{
			name: "Helm via managed-by label",
			ownership: &agent.Ownership{
				Type:   agent.OwnerHelm,
				Name:   "my-release",
				Source: "label:app.kubernetes.io/managed-by=Helm",
			},
			traceErr:      nil,
			expectedTool:  "helm",
			expectedOwner: "Helm",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := buildOwnershipOnlyTraceResult("Deployment", "frontend", "prod", tc.ownership, tc.traceErr)

			if result.Tool != tc.expectedTool {
				t.Errorf("tool = %q, want %q", result.Tool, tc.expectedTool)
			}

			// Verify the summary built from this result preserves ownership
			summary := buildExplainSummary(result)
			if summary.Owner != tc.expectedOwner {
				t.Errorf("owner = %q, want %q", summary.Owner, tc.expectedOwner)
			}

			// Error should contain the ownership source
			if !strings.Contains(result.Error, tc.ownership.Source) {
				t.Errorf("error should contain ownership source %q, got: %s", tc.ownership.Source, result.Error)
			}

			// Error should contain the owner name when available
			if tc.ownership.Name != "" && !strings.Contains(result.Error, tc.ownership.Name) {
				t.Errorf("error should contain owner name %q, got: %s", tc.ownership.Name, result.Error)
			}
		})
	}
}

// TestBuildExplainSummary_UnknownOwnerStillUnknown verifies that truly unknown
// resources still show "Unknown - no recognized ownership labels found".
func TestBuildExplainSummary_UnknownOwnerStillUnknown(t *testing.T) {
	// This is the case where no ownership was detected at all
	result := &agent.TraceResult{
		Object: agent.ResourceRef{
			Kind:      "Deployment",
			Name:      "legacy-api",
			Namespace: "default",
		},
		Tool:  "", // No tool detected
		Error: "resource not managed by any detected GitOps tool",
	}

	summary := buildExplainSummary(result)

	// Should still show Unknown with the explicit message
	if summary.Owner != "Unknown - no recognized ownership labels found" {
		t.Fatalf("owner = %q, want explicit unknown-owner message for truly unknown resources", summary.Owner)
	}
}

// TestIsNegativeMismatchCandidate tests the helper that detects when a tracer
// returns a "not managed by me" result for a resource owned by a different tool.
func TestIsNegativeMismatchCandidate(t *testing.T) {
	tests := []struct {
		name      string
		result    *agent.TraceResult
		ownership *agent.Ownership
		wantMatch bool
	}{
		{
			name: "Flux says 'not managed' for ArgoCD-owned resource",
			result: &agent.TraceResult{
				Tool:  "flux",
				Error: "resource not managed by Flux",
			},
			ownership: &agent.Ownership{
				Type: agent.OwnerArgo,
				Name: "cubbychat",
			},
			wantMatch: true,
		},
		{
			name: "Helm says 'no release found' for ArgoCD-owned resource",
			result: &agent.TraceResult{
				Tool:  "helm",
				Error: "no Helm release found managing this resource",
			},
			ownership: &agent.Ownership{
				Type: agent.OwnerArgo,
				Name: "cubbychat",
			},
			wantMatch: true,
		},
		{
			name: "Flux says 'not managed' for Flux-owned resource (same tool)",
			result: &agent.TraceResult{
				Tool:  "flux",
				Error: "resource not managed by Flux",
			},
			ownership: &agent.Ownership{
				Type: agent.OwnerFlux,
				Name: "apps",
			},
			wantMatch: false, // Same tool - not a mismatch
		},
		{
			name: "Flux says 'not managed' with unknown ownership",
			result: &agent.TraceResult{
				Tool:  "flux",
				Error: "resource not managed by Flux",
			},
			ownership: &agent.Ownership{
				Type: agent.OwnerUnknown,
			},
			wantMatch: false, // Unknown ownership - can't determine mismatch
		},
		{
			name: "Successful result (no error) is not a negative mismatch",
			result: &agent.TraceResult{
				Tool:  "flux",
				Error: "",
				Chain: []agent.ChainLink{{Kind: "GitRepository", Name: "test"}},
			},
			ownership: &agent.Ownership{
				Type: agent.OwnerArgo,
				Name: "cubbychat",
			},
			wantMatch: false, // No error means not a negative result
		},
		{
			name: "ArgoCD error for ArgoCD-owned resource (same tool error)",
			result: &agent.TraceResult{
				Tool:  "argocd",
				Error: "application not found",
			},
			ownership: &agent.Ownership{
				Type: agent.OwnerArgo,
				Name: "cubbychat",
			},
			wantMatch: false, // Same tool - not a mismatch
		},
		{
			name: "nil ownership",
			result: &agent.TraceResult{
				Tool:  "flux",
				Error: "resource not managed by Flux",
			},
			ownership: nil,
			wantMatch: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isNegativeMismatchCandidate(tc.result, tc.ownership)
			if got != tc.wantMatch {
				t.Errorf("isNegativeMismatchCandidate() = %v, want %v", got, tc.wantMatch)
			}
		})
	}
}

// TestOwnerTypeToToolName verifies the ownership type to tool name mapping.
func TestOwnerTypeToToolName(t *testing.T) {
	tests := []struct {
		ownerType string
		wantTool  string
	}{
		{agent.OwnerArgo, "argocd"},
		{agent.OwnerFlux, "flux"},
		{agent.OwnerHelm, "helm"},
		{agent.OwnerUnknown, agent.OwnerUnknown},
		{"custom", "custom"},
	}

	for _, tc := range tests {
		t.Run(tc.ownerType, func(t *testing.T) {
			got := ownerTypeToToolName(tc.ownerType)
			if got != tc.wantTool {
				t.Errorf("ownerTypeToToolName(%q) = %q, want %q", tc.ownerType, got, tc.wantTool)
			}
		})
	}
}

// TestTracerSelectionWithNegativeMismatches exercises the core fix for #365:
// when ownership is detected as ArgoCD but ArgoTracer fails and Flux/Helm
// tracers return "not managed" partials, the final result should be ArgoCD
// (via ownership-preserving fallback), not Flux or Helm.
//
// This test uses selectBestTraceResult which encapsulates the tracer selection
// logic extracted from traceForExplain for testability.
func TestTracerSelectionWithNegativeMismatches(t *testing.T) {
	tests := []struct {
		name          string
		ownership     *agent.Ownership
		tracerResults []tracerTestResult // simulates tracer outputs in order
		wantTool      string
		wantHasChain  bool
	}{
		{
			name: "ArgoCD ownership + Argo error + Flux/Helm negative partials -> ArgoCD",
			ownership: &agent.Ownership{
				Type:   agent.OwnerArgo,
				Name:   "cubbychat",
				Source: "annotation:argocd.argoproj.io/tracking-id",
			},
			tracerResults: []tracerTestResult{
				{tool: "argocd", err: fmt.Errorf("for non-Application resources, use --app flag")},
				{tool: "flux", result: &agent.TraceResult{Tool: "flux", Error: "resource not managed by Flux"}},
				{tool: "helm", result: &agent.TraceResult{Tool: "helm", Error: "no Helm release found managing this resource"}},
			},
			wantTool:     "argocd", // Should use ownership-preserving fallback
			wantHasChain: false,
		},
		{
			name: "Flux ownership + Flux success -> Flux with chain",
			ownership: &agent.Ownership{
				Type:   agent.OwnerFlux,
				Name:   "apps",
				Source: "label:kustomize.toolkit.fluxcd.io/name",
			},
			tracerResults: []tracerTestResult{
				{tool: "flux", result: &agent.TraceResult{
					Tool:  "flux",
					Chain: []agent.ChainLink{{Kind: "GitRepository", Name: "platform"}},
				}},
			},
			wantTool:     "flux",
			wantHasChain: true,
		},
		{
			name: "Unknown ownership + multiple negative partials -> last partial wins",
			ownership: &agent.Ownership{
				Type: agent.OwnerUnknown,
			},
			tracerResults: []tracerTestResult{
				{tool: "flux", result: &agent.TraceResult{Tool: "flux", Error: "resource not managed by Flux"}},
				{tool: "argocd", err: fmt.Errorf("argocd context stale")},
				{tool: "helm", result: &agent.TraceResult{Tool: "helm", Error: "no Helm release found"}},
			},
			wantTool:     "helm", // With unknown ownership, last partial wins (no preference)
			wantHasChain: false,
		},
		{
			name: "Helm ownership + Helm success -> Helm with chain",
			ownership: &agent.Ownership{
				Type:   agent.OwnerHelm,
				Name:   "my-release",
				Source: "label:app.kubernetes.io/managed-by=Helm",
			},
			tracerResults: []tracerTestResult{
				{tool: "helm", result: &agent.TraceResult{
					Tool:  "helm",
					Chain: []agent.ChainLink{{Kind: "HelmChart", Name: "my-chart"}},
				}},
			},
			wantTool:     "helm",
			wantHasChain: true,
		},
		{
			name: "ArgoCD ownership + all tracers error -> ArgoCD via ownership fallback",
			ownership: &agent.Ownership{
				Type:   agent.OwnerArgo,
				Name:   "cubbychat",
				Source: "annotation:argocd.argoproj.io/tracking-id",
			},
			tracerResults: []tracerTestResult{
				{tool: "argocd", err: fmt.Errorf("argocd CLI not available")},
				{tool: "flux", err: fmt.Errorf("flux CLI not available")},
				{tool: "helm", err: fmt.Errorf("no k8s client")},
			},
			wantTool:     "argocd", // Should use ownership-preserving fallback
			wantHasChain: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := selectBestTraceResult(tc.ownership, tc.tracerResults)

			if result == nil {
				t.Fatal("selectBestTraceResult returned nil")
			}
			if result.Tool != tc.wantTool {
				t.Errorf("tool = %q, want %q", result.Tool, tc.wantTool)
			}
			hasChain := len(result.Chain) > 0
			if hasChain != tc.wantHasChain {
				t.Errorf("hasChain = %v, want %v", hasChain, tc.wantHasChain)
			}
		})
	}
}

// tracerTestResult simulates a tracer's output for testing.
type tracerTestResult struct {
	tool   string
	result *agent.TraceResult
	err    error
}

// selectBestTraceResult is the extracted tracer selection logic for testing.
// It mirrors the loop in traceForExplain but takes pre-computed tracer results.
func selectBestTraceResult(ownership *agent.Ownership, results []tracerTestResult) *agent.TraceResult {
	var (
		lastErr   error
		candidate *agent.TraceResult
	)

	for _, tr := range results {
		if tr.err != nil {
			lastErr = tr.err
			continue
		}
		if tr.result == nil {
			continue
		}

		result := tr.result
		if result.Object.Kind == "" {
			result.Object.Kind = "Deployment"
			result.Object.Name = "test"
			result.Object.Namespace = "default"
		}

		// Successful trace with chain - return immediately
		if len(result.Chain) > 0 {
			return result
		}

		// Successful trace without error - return immediately
		if strings.TrimSpace(result.Error) == "" {
			return result
		}

		// Check for negative mismatch
		if isNegativeMismatchCandidate(result, ownership) {
			continue
		}

		candidate = result
	}

	if candidate != nil {
		return candidate
	}

	// Ownership-preserving fallback
	if ownership != nil && ownership.Type != agent.OwnerUnknown {
		return buildOwnershipOnlyTraceResult("Deployment", "test", "default", ownership, lastErr)
	}

	return nil
}

func TestRenderExplainMarkdown_PresentationModes(t *testing.T) {
	summary := ExplainSummary{
		Resource:    "Deployment/payments-api",
		Namespace:   "prod",
		Owner:       "Flux",
		Source:      "https://github.com/acme/platform-config.git",
		DeployedVia: "GitRepository/platform-config -> Kustomization/payments -> Deployment/payments-api",
		Health:      "Healthy",
		Risks:       "0 findings",
		Drift:       "None detected",
	}

	tests := []struct {
		mode     PresentationMode
		expected []string
		excluded []string
	}{
		{
			mode: PresentationHuman,
			expected: []string{
				"## Explain",
				"- **Resource:** `Deployment/payments-api`",
				"### Try Next",
			},
			excluded: []string{
				"## RESOURCE CONTEXT",
				"[resource:",
				"### RECOMMENDED ACTIONS",
			},
		},
		{
			mode: PresentationAI,
			expected: []string{
				"## RESOURCE CONTEXT",
				"[resource: Deployment/payments-api namespace: prod]",
				"### RECOMMENDED ACTIONS",
				"[end resource context]",
			},
			excluded: []string{
				"## Explain\n",
				"### Try Next",
			},
		},
		{
			mode: PresentationPaired,
			expected: []string{
				"## Explain",
				"### Try Next",
			},
			excluded: []string{
				"## RESOURCE CONTEXT",
				"[resource:",
				"### RECOMMENDED ACTIONS",
			},
		},
	}

	for _, tc := range tests {
		t.Run(string(tc.mode), func(t *testing.T) {
			// explicitMode=true since we're testing explicit presentation modes
			// Hint context is independent of presentation mode
			out := renderExplainMarkdown(summary, tc.mode, true, DefaultHintContext())

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
