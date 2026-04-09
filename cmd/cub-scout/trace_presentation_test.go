// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
)

func TestTracePresentation_LegacyMode(t *testing.T) {
	result := &agent.TraceResult{
		Object: agent.ResourceRef{
			Kind:      "Deployment",
			Name:      "nginx",
			Namespace: "default",
		},
		Tool: "flux",
		Chain: []agent.ChainLink{
			{Kind: "GitRepository", Name: "flux-system", Namespace: "flux-system", Ready: true},
			{Kind: "Kustomization", Name: "apps", Namespace: "flux-system", Ready: true},
			{Kind: "Deployment", Name: "nginx", Namespace: "default", Ready: true},
		},
	}

	// Legacy mode (no --presentation flag)
	invCtx, _ := NewInvocationContext("", TransportCLI)
	out := captureStdout(t, func() {
		if err := outputTraceHuman(result, nil, invCtx); err != nil {
			t.Fatalf("outputTraceHuman() error = %v", err)
		}
	})

	// Legacy format uses "TRACE:" with colors
	if !strings.Contains(out, "TRACE:") {
		t.Errorf("legacy mode should contain 'TRACE:', got:\n%s", out)
	}
	// Should NOT have AI-style markers
	if strings.Contains(out, "[TRACE:") {
		t.Errorf("legacy mode should not have AI-style [TRACE:] marker, got:\n%s", out)
	}
	if strings.Contains(out, "OWNERSHIP CHAIN:") {
		t.Errorf("legacy mode should not have explicit OWNERSHIP CHAIN: heading, got:\n%s", out)
	}
}

func TestTracePresentation_HumanMode(t *testing.T) {
	result := &agent.TraceResult{
		Object: agent.ResourceRef{
			Kind:      "Deployment",
			Name:      "nginx",
			Namespace: "default",
		},
		Tool: "flux",
		Chain: []agent.ChainLink{
			{Kind: "GitRepository", Name: "flux-system", Namespace: "flux-system", Ready: true},
			{Kind: "Kustomization", Name: "apps", Namespace: "flux-system", Ready: true},
			{Kind: "Deployment", Name: "nginx", Namespace: "default", Ready: true},
		},
	}

	invCtx, _ := NewInvocationContext("human", TransportCLI)
	out := captureStdout(t, func() {
		if err := outputTraceHuman(result, nil, invCtx); err != nil {
			t.Fatalf("outputTraceHuman() error = %v", err)
		}
	})

	// Human mode uses "Trace:" format
	if !strings.Contains(out, "Trace:") {
		t.Errorf("human mode should contain 'Trace:', got:\n%s", out)
	}
	// Should have chain heading
	if !strings.Contains(out, "Ownership Chain:") {
		t.Errorf("human mode should have 'Ownership Chain:' heading, got:\n%s", out)
	}
}

func TestTracePresentation_AIMode(t *testing.T) {
	result := &agent.TraceResult{
		Object: agent.ResourceRef{
			Kind:      "Deployment",
			Name:      "nginx",
			Namespace: "default",
		},
		Tool: "flux",
		Chain: []agent.ChainLink{
			{Kind: "GitRepository", Name: "flux-system", Namespace: "flux-system", Ready: true},
			{Kind: "Kustomization", Name: "apps", Namespace: "flux-system", Ready: true},
			{Kind: "Deployment", Name: "nginx", Namespace: "default", Ready: true},
		},
	}

	invCtx, _ := NewInvocationContext("ai", TransportCLI)
	out := captureStdout(t, func() {
		if err := outputTraceHuman(result, nil, invCtx); err != nil {
			t.Fatalf("outputTraceHuman() error = %v", err)
		}
	})

	// AI mode uses bracket notation
	if !strings.Contains(out, "[TRACE:") {
		t.Errorf("AI mode should contain '[TRACE:', got:\n%s", out)
	}
	// Should have owner info
	if !strings.Contains(out, "[owner: flux]") {
		t.Errorf("AI mode should contain '[owner: flux]', got:\n%s", out)
	}
	// Should have uppercase chain heading
	if !strings.Contains(out, "OWNERSHIP CHAIN:") {
		t.Errorf("AI mode should have 'OWNERSHIP CHAIN:' heading, got:\n%s", out)
	}
	// Should have outro
	if !strings.Contains(out, "[end trace]") {
		t.Errorf("AI mode should contain '[end trace]', got:\n%s", out)
	}
}

func TestTracePresentation_PairedMode(t *testing.T) {
	result := &agent.TraceResult{
		Object: agent.ResourceRef{
			Kind:      "Deployment",
			Name:      "nginx",
			Namespace: "default",
		},
		Tool: "argocd",
		Chain: []agent.ChainLink{
			{Kind: "Application", Name: "nginx-app", Namespace: "argocd", Ready: true},
			{Kind: "Deployment", Name: "nginx", Namespace: "default", Ready: true},
		},
	}

	invCtx, _ := NewInvocationContext("paired", TransportCLI)
	out := captureStdout(t, func() {
		if err := outputTraceHuman(result, nil, invCtx); err != nil {
			t.Fatalf("outputTraceHuman() error = %v", err)
		}
	})

	// Paired mode uses "Trace:" format (title case)
	if !strings.Contains(out, "Trace:") {
		t.Errorf("paired mode should contain 'Trace:', got:\n%s", out)
	}
	// Should have owner info
	if !strings.Contains(out, "Owner: argocd") {
		t.Errorf("paired mode should contain 'Owner: argocd', got:\n%s", out)
	}
	// Should have chain heading (title case)
	if !strings.Contains(out, "Ownership Chain:") {
		t.Errorf("paired mode should have 'Ownership Chain:' heading, got:\n%s", out)
	}
	// Should NOT have AI-style outro
	if strings.Contains(out, "[end trace]") {
		t.Errorf("paired mode should not have AI-style [end trace], got:\n%s", out)
	}
}

func TestTracePresentationHelpers(t *testing.T) {
	tests := []struct {
		name     string
		mode     PresentationMode
		resource string
		tool     string
		wantHead string
		wantInt  string
		wantOut  string
		wantCH   string
	}{
		{
			name:     "AI mode",
			mode:     PresentationAI,
			resource: "Deployment/nginx",
			tool:     "flux",
			wantHead: "[TRACE: Deployment/nginx]",
			wantInt:  "[owner: flux]",
			wantOut:  "[end trace]",
			wantCH:   "OWNERSHIP CHAIN:",
		},
		{
			name:     "Human mode",
			mode:     PresentationHuman,
			resource: "Deployment/nginx",
			tool:     "argocd",
			wantHead: "Trace: Deployment/nginx",
			wantInt:  "Owner: argocd",
			wantOut:  "",
			wantCH:   "Ownership Chain:",
		},
		{
			name:     "Paired mode",
			mode:     PresentationPaired,
			resource: "StatefulSet/redis",
			tool:     "helm",
			wantHead: "Trace: StatefulSet/redis",
			wantInt:  "Owner: helm",
			wantOut:  "",
			wantCH:   "Ownership Chain:",
		},
		{
			name:     "Legacy mode",
			mode:     PresentationLegacy,
			resource: "Deployment/app",
			tool:     "",
			wantHead: "TRACE: Deployment/app",
			wantInt:  "",
			wantOut:  "",
			wantCH:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			heading := TraceHeading(tt.mode, tt.resource)
			if heading != tt.wantHead {
				t.Errorf("TraceHeading() = %q, want %q", heading, tt.wantHead)
			}

			intro := TraceIntro(tt.mode, tt.tool)
			if intro != tt.wantInt {
				t.Errorf("TraceIntro() = %q, want %q", intro, tt.wantInt)
			}

			outro := TraceOutro(tt.mode)
			if outro != tt.wantOut {
				t.Errorf("TraceOutro() = %q, want %q", outro, tt.wantOut)
			}

			chainHeading := TraceChainHeading(tt.mode)
			if chainHeading != tt.wantCH {
				t.Errorf("TraceChainHeading() = %q, want %q", chainHeading, tt.wantCH)
			}
		})
	}
}
