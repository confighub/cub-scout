package main

import (
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

	out := renderExplainText(summary)

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

	out := renderExplainMarkdown(summary)

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

	out := renderExplainText(summary)
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
