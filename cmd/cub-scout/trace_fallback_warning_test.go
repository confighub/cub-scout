// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
)

func TestOutputTraceHuman_ShowsDegradedWarningWhenChainExists(t *testing.T) {
	result := &agent.TraceResult{
		Object: agent.ResourceRef{
			Kind:      "Application",
			Name:      "checkout",
			Namespace: "argocd",
		},
		Tool:         "argocd",
		FullyManaged: true,
		Error:        "ArgoCD CLI unavailable - showing kubectl-based Application trace only.",
		Chain: []agent.ChainLink{
			{Kind: "Source", Name: "acme/checkout", Ready: true},
			{Kind: "Application", Name: "checkout", Namespace: "argocd", Ready: true},
		},
	}

	out := captureStdout(t, func() {
		if err := outputTraceHuman(result, nil); err != nil {
			t.Fatalf("outputTraceHuman() error = %v", err)
		}
	})

	if !strings.Contains(out, "ArgoCD CLI unavailable") {
		t.Fatalf("expected degraded warning in human output, got:\n%s", out)
	}
}

func TestOutputTraceMarkdown_ShowsDegradedWarningWhenChainExists(t *testing.T) {
	result := &agent.TraceResult{
		Object: agent.ResourceRef{
			Kind:      "Application",
			Name:      "checkout",
			Namespace: "argocd",
		},
		Tool:         "argocd",
		FullyManaged: true,
		Error:        "ArgoCD CLI unavailable - showing kubectl-based Application trace only.",
		Chain: []agent.ChainLink{
			{Kind: "Source", Name: "acme/checkout", Ready: true},
			{Kind: "Application", Name: "checkout", Namespace: "argocd", Ready: true},
		},
	}

	out := captureStdout(t, func() {
		if err := outputTraceMarkdown(result, nil); err != nil {
			t.Fatalf("outputTraceMarkdown() error = %v", err)
		}
	})

	if !strings.Contains(out, "[warning] ArgoCD CLI unavailable") {
		t.Fatalf("expected degraded warning in markdown output, got:\n%s", out)
	}
}

