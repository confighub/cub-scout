package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
)

func TestExplainText_GoldenByOwner(t *testing.T) {
	cases := []struct {
		name   string
		result *agent.TraceResult
		golden string
	}{
		{
			name: "flux",
			result: &agent.TraceResult{
				Tool: "flux",
				Object: agent.ResourceRef{Kind: "Deployment", Name: "payments-api", Namespace: "prod"},
				Chain: []agent.ChainLink{
					{Kind: "GitRepository", Name: "platform-config", Namespace: "flux-system", URL: "https://github.com/acme/platform-config.git", Revision: "main@sha1:abc1234", Ready: true, Status: "Ready"},
					{Kind: "Kustomization", Name: "payments", Namespace: "flux-system", Path: "./apps/payments", Revision: "main@sha1:abc1234", Ready: true, Status: "Ready"},
					{Kind: "Deployment", Name: "payments-api", Namespace: "prod", Ready: true, Status: "Healthy"},
				},
			},
			golden: "flux_text.golden.txt",
		},
		{
			name: "argocd",
			result: &agent.TraceResult{
				Tool: "argocd",
				Object: agent.ResourceRef{Kind: "Deployment", Name: "frontend", Namespace: "prod"},
				Chain: []agent.ChainLink{
					{Kind: "Application", Name: "frontend-app", Namespace: "argocd", URL: "https://github.com/acme/apps.git", Revision: "main@sha1:def4567", Ready: true, Status: "Synced/Healthy"},
					{Kind: "Deployment", Name: "frontend", Namespace: "prod", Ready: true, Status: "Healthy"},
				},
			},
			golden: "argocd_text.golden.txt",
		},
		{
			name: "helm",
			result: &agent.TraceResult{
				Tool: "helm",
				Object: agent.ResourceRef{Kind: "StatefulSet", Name: "redis", Namespace: "prod"},
				Chain: []agent.ChainLink{
					{Kind: "HelmRelease", Name: "redis", Namespace: "prod", Revision: "chart@1.2.3", Ready: true, Status: "Ready"},
					{Kind: "StatefulSet", Name: "redis", Namespace: "prod", Ready: true, Status: "Healthy"},
				},
			},
			golden: "helm_text.golden.txt",
		},
		{
			name: "unknown",
			result: &agent.TraceResult{
				Object: agent.ResourceRef{Kind: "Deployment", Name: "legacy-api", Namespace: "default"},
				Error:  "resource not managed by any detected GitOps tool",
			},
			golden: "unknown_text.golden.txt",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			summary := buildExplainSummary(tc.result)
			actual := renderExplainText(summary)
			assertExplainGolden(t, tc.golden, actual)
		})
	}
}

func TestExplainMarkdown_Golden(t *testing.T) {
	result := &agent.TraceResult{
		Tool: "flux",
		Object: agent.ResourceRef{Kind: "Deployment", Name: "payments-api", Namespace: "prod"},
		Chain: []agent.ChainLink{
			{Kind: "GitRepository", Name: "platform-config", Namespace: "flux-system", URL: "https://github.com/acme/platform-config.git", Revision: "main@sha1:abc1234", Ready: true, Status: "Ready"},
			{Kind: "Kustomization", Name: "payments", Namespace: "flux-system", Path: "./apps/payments", Revision: "main@sha1:abc1234", Ready: true, Status: "Ready"},
			{Kind: "Deployment", Name: "payments-api", Namespace: "prod", Ready: true, Status: "Healthy"},
		},
	}

	summary := buildExplainSummary(result)
	actual := renderExplainMarkdown(summary)
	assertExplainGolden(t, "flux_md.golden.md", actual)
}

func TestExplainJSON_Golden(t *testing.T) {
	result := &agent.TraceResult{
		Tool: "flux",
		Object: agent.ResourceRef{Kind: "Deployment", Name: "payments-api", Namespace: "prod"},
		Chain: []agent.ChainLink{
			{Kind: "GitRepository", Name: "platform-config", Namespace: "flux-system", URL: "https://github.com/acme/platform-config.git", Revision: "main@sha1:abc1234", Ready: true, Status: "Ready"},
			{Kind: "Kustomization", Name: "payments", Namespace: "flux-system", Path: "./apps/payments", Revision: "main@sha1:abc1234", Ready: true, Status: "Ready"},
			{Kind: "Deployment", Name: "payments-api", Namespace: "prod", Ready: true, Status: "Healthy"},
		},
	}

	summary := buildExplainSummary(result)
	b, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	actual := string(b) + "\n"
	assertExplainGolden(t, "flux_json.golden.json", actual)
}

func assertExplainGolden(t *testing.T, goldenFile string, actual string) {
	t.Helper()
	goldenPath := filepath.Join("testdata", "explain", goldenFile)
	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenPath, err)
	}
	if actual != string(expected) {
		t.Fatalf("golden mismatch for %s\n--- expected ---\n%s\n--- actual ---\n%s", goldenFile, string(expected), actual)
	}
}
