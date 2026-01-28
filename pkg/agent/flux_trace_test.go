// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"
)

func TestFluxTracerParseTraceOutput(t *testing.T) {
	tracer := NewFluxTracer()

	tests := []struct {
		name          string
		output        string
		kind          string
		resourceName  string
		namespace     string
		wantChainLen  int
		wantFullyMgd  bool
		wantFirstKind string
		wantLastKind  string
	}{
		{
			name: "healthy deployment trace",
			output: `Object:        Deployment/nginx
Namespace:     demo
Status:        Managed by Flux
---
Kustomization: apps
Namespace:     flux-system
Path:          ./clusters/prod/apps
Revision:      main@sha1:abc123
Status:        Applied revision main@sha1:abc123
---
GitRepository: infra-repo
Namespace:     flux-system
URL:           https://github.com/your-org/infra.git
Revision:      main@sha1:abc123
Status:        Artifact is up to date
`,
			kind:          "Deployment",
			resourceName:  "nginx",
			namespace:     "demo",
			wantChainLen:  3,
			wantFullyMgd:  false,           // "Managed by Flux" doesn't indicate ready status
			wantFirstKind: "GitRepository", // Reversed: source first
			wantLastKind:  "Deployment",    // Object parses as Deployment/nginx
		},
		{
			name: "helm release trace",
			output: `Object:        Deployment/redis
Namespace:     cache
Status:        Managed by Flux
---
HelmRelease:   redis
Namespace:     flux-system
Revision:      6.2.5
Status:        Release reconciliation succeeded
---
HelmRepository: bitnami
Namespace:     flux-system
URL:           https://charts.bitnami.com/bitnami
Status:        Stored artifact
`,
			kind:          "Deployment",
			resourceName:  "redis",
			namespace:     "cache",
			wantChainLen:  3,
			wantFullyMgd:  false, // "Managed by Flux" doesn't indicate ready status
			wantFirstKind: "HelmRepository",
			wantLastKind:  "Deployment", // Object parses as Deployment/redis
		},
		{
			name: "failed kustomization",
			output: `Object:        Deployment/broken
Namespace:     prod
Status:        Managed by Flux
---
Kustomization: apps
Namespace:     flux-system
Path:          ./clusters/prod/apps
Revision:      main@sha1:def456
Status:        kustomize build failed
---
GitRepository: infra-repo
Namespace:     flux-system
URL:           https://github.com/your-org/infra.git
Revision:      main@sha1:def456
Status:        Artifact is up to date
`,
			kind:         "Deployment",
			resourceName: "broken",
			namespace:    "prod",
			wantChainLen: 3,
			wantFullyMgd: false, // Failed kustomization
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tracer.parseTraceOutput(tt.output, tt.kind, tt.resourceName, tt.namespace)
			if err != nil {
				t.Fatalf("parseTraceOutput() error = %v", err)
			}

			if len(result.Chain) != tt.wantChainLen {
				t.Errorf("Chain length = %d, want %d", len(result.Chain), tt.wantChainLen)
			}

			if result.FullyManaged != tt.wantFullyMgd {
				t.Errorf("FullyManaged = %v, want %v", result.FullyManaged, tt.wantFullyMgd)
			}

			if tt.wantFirstKind != "" && len(result.Chain) > 0 {
				if result.Chain[0].Kind != tt.wantFirstKind {
					t.Errorf("First chain kind = %q, want %q", result.Chain[0].Kind, tt.wantFirstKind)
				}
			}

			if tt.wantLastKind != "" && len(result.Chain) > 0 {
				lastIdx := len(result.Chain) - 1
				if result.Chain[lastIdx].Kind != tt.wantLastKind {
					t.Errorf("Last chain kind = %q, want %q", result.Chain[lastIdx].Kind, tt.wantLastKind)
				}
			}

			if result.Tool != "flux" {
				t.Errorf("Tool = %q, want %q", result.Tool, "flux")
			}
		})
	}
}

func TestFluxTracerIsReadyStatus(t *testing.T) {
	tracer := NewFluxTracer()

	tests := []struct {
		status string
		want   bool
	}{
		// Positive cases
		{"Applied revision main@sha1:abc123", true},
		{"Artifact is up to date", true},
		{"Release reconciliation succeeded", true},
		{"Ready", true},
		{"Stored artifact", true},

		// Negative cases
		{"kustomize build failed", false},
		{"Reconciliation failed", false},
		{"Error: something went wrong", false},
		{"Not ready", false},
		{"Stalled", false},
		{"Suspended", false},

		// Ambiguous - defaults to false
		{"Reconciling", false},
		{"Pending", false},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			result := tracer.isReadyStatus(tt.status)
			if result != tt.want {
				t.Errorf("isReadyStatus(%q) = %v, want %v", tt.status, result, tt.want)
			}
		})
	}
}

func TestFluxTracerParseSection(t *testing.T) {
	tracer := NewFluxTracer()

	tests := []struct {
		name      string
		section   string
		wantKind  string
		wantName  string
		wantReady bool
		wantURL   string
		wantPath  string
	}{
		{
			name: "git repository section",
			section: `GitRepository: infra-repo
Namespace:     flux-system
URL:           https://github.com/your-org/infra.git
Revision:      main@sha1:abc123
Status:        Artifact is up to date`,
			wantKind:  "GitRepository",
			wantName:  "infra-repo",
			wantReady: true,
			wantURL:   "https://github.com/your-org/infra.git",
		},
		{
			name: "kustomization section",
			section: `Kustomization: apps
Namespace:     flux-system
Path:          ./clusters/prod/apps
Revision:      main@sha1:abc123
Status:        Applied revision main@sha1:abc123`,
			wantKind:  "Kustomization",
			wantName:  "apps",
			wantReady: true,
			wantPath:  "./clusters/prod/apps",
		},
		{
			name: "failed section",
			section: `Kustomization: broken
Namespace:     flux-system
Path:          ./bad/path
Status:        kustomize build failed`,
			wantKind:  "Kustomization",
			wantName:  "broken",
			wantReady: false,
		},
		{
			name: "object section",
			section: `Object:        Deployment/nginx
Namespace:     demo
Status:        Managed by Flux`,
			wantKind:  "Deployment",
			wantName:  "nginx",
			wantReady: false, // "Managed by Flux" doesn't match ready patterns
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			link, err := tracer.parseSection(tt.section)
			if err != nil {
				t.Fatalf("parseSection() error = %v", err)
			}

			if link.Kind != tt.wantKind {
				t.Errorf("Kind = %q, want %q", link.Kind, tt.wantKind)
			}

			if link.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", link.Name, tt.wantName)
			}

			if link.Ready != tt.wantReady {
				t.Errorf("Ready = %v, want %v", link.Ready, tt.wantReady)
			}

			if tt.wantURL != "" && link.URL != tt.wantURL {
				t.Errorf("URL = %q, want %q", link.URL, tt.wantURL)
			}

			if tt.wantPath != "" && link.Path != tt.wantPath {
				t.Errorf("Path = %q, want %q", link.Path, tt.wantPath)
			}
		})
	}
}

func TestExtractRevision(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"main@sha1:abc123def456789", "abc123d"},
		{"main@sha1:abc", "abc"},
		{"v1.0.0@sha1:1234567890", "1234567"},
		{"feature/test@abc123", "abc123"},
		{"", ""},
		{"no-at-sign", "no-at-sign"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractRevision(tt.input)
			if result != tt.want {
				t.Errorf("extractRevision(%q) = %q, want %q", tt.input, result, tt.want)
			}
		})
	}
}

// ============================================================================
// History Feature Tests (Task 3: Flux history extraction)
// ============================================================================

func TestFluxKustomizationHistory(t *testing.T) {
	// Simulated kubectl get kustomization -o json output
	jsonData := `{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind": "Kustomization",
		"metadata": {
			"name": "apps",
			"namespace": "flux-system"
		},
		"status": {
			"conditions": [
				{
					"type": "Ready",
					"status": "True",
					"reason": "ReconciliationSucceeded"
				}
			],
			"lastAppliedRevision": "main@sha1:abc123",
			"history": [
				{
					"digest": "sha256:abc123def456",
					"firstReconciled": "2026-01-28T08:00:00Z",
					"lastReconciled": "2026-01-28T10:00:00Z",
					"lastReconciledDuration": "2.5s",
					"lastReconciledStatus": "ReconciliationSucceeded",
					"totalReconciliations": 5,
					"metadata": {
						"revision": "main@sha1:abc123def456"
					}
				},
				{
					"digest": "sha256:older789",
					"firstReconciled": "2026-01-27T08:00:00Z",
					"lastReconciled": "2026-01-27T15:00:00Z",
					"lastReconciledDuration": "3.1s",
					"lastReconciledStatus": "ReconciliationSucceeded",
					"totalReconciliations": 10,
					"metadata": {
						"revision": "main@sha1:older789012"
					}
				}
			]
		}
	}`

	history, err := ParseFluxResourceHistory([]byte(jsonData))
	if err != nil {
		t.Fatalf("ParseFluxResourceHistory() error = %v", err)
	}

	// Verify history is populated
	if len(history) != 2 {
		t.Fatalf("Expected 2 history entries, got %d", len(history))
	}

	// Verify first entry (most recent)
	if history[0].Revision != "main@sha1:abc123def456" {
		t.Errorf("History[0].Revision = %q, want %q", history[0].Revision, "main@sha1:abc123def456")
	}
	if history[0].Status != "ReconciliationSucceeded" {
		t.Errorf("History[0].Status = %q, want %q", history[0].Status, "ReconciliationSucceeded")
	}
	if history[0].Duration != "2.5s" {
		t.Errorf("History[0].Duration = %q, want %q", history[0].Duration, "2.5s")
	}
	if history[0].Timestamp.IsZero() {
		t.Error("History[0].Timestamp should not be zero")
	}

	// Verify second entry
	if history[1].Revision != "main@sha1:older789012" {
		t.Errorf("History[1].Revision = %q, want %q", history[1].Revision, "main@sha1:older789012")
	}
}

func TestFluxHelmReleaseHistory(t *testing.T) {
	// Simulated kubectl get helmrelease -o json output
	jsonData := `{
		"apiVersion": "helm.toolkit.fluxcd.io/v2",
		"kind": "HelmRelease",
		"metadata": {
			"name": "redis",
			"namespace": "flux-system"
		},
		"status": {
			"conditions": [
				{
					"type": "Ready",
					"status": "True",
					"reason": "ReconciliationSucceeded"
				}
			],
			"lastAppliedRevision": "17.0.0",
			"history": [
				{
					"digest": "sha256:helm123",
					"firstReconciled": "2026-01-28T09:00:00Z",
					"lastReconciled": "2026-01-28T09:30:00Z",
					"lastReconciledDuration": "15.2s",
					"lastReconciledStatus": "ReconciliationSucceeded",
					"totalReconciliations": 3,
					"metadata": {
						"revision": "17.0.0"
					}
				}
			]
		}
	}`

	history, err := ParseFluxResourceHistory([]byte(jsonData))
	if err != nil {
		t.Fatalf("ParseFluxResourceHistory() error = %v", err)
	}

	if len(history) != 1 {
		t.Fatalf("Expected 1 history entry, got %d", len(history))
	}

	if history[0].Revision != "17.0.0" {
		t.Errorf("History[0].Revision = %q, want %q", history[0].Revision, "17.0.0")
	}
	if history[0].Duration != "15.2s" {
		t.Errorf("History[0].Duration = %q, want %q", history[0].Duration, "15.2s")
	}
}

func TestFluxHistoryEmpty(t *testing.T) {
	// Flux resource without history field
	jsonData := `{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind": "Kustomization",
		"metadata": {
			"name": "new-ks",
			"namespace": "flux-system"
		},
		"status": {
			"conditions": [
				{
					"type": "Ready",
					"status": "True"
				}
			]
		}
	}`

	history, err := ParseFluxResourceHistory([]byte(jsonData))
	if err != nil {
		t.Fatalf("ParseFluxResourceHistory() error = %v", err)
	}

	// Empty history should return nil or empty slice
	if len(history) != 0 {
		t.Errorf("Expected empty history, got %d entries", len(history))
	}
}

func TestFluxHistoryWithEmptyArray(t *testing.T) {
	// Flux resource with explicit empty history array
	jsonData := `{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind": "Kustomization",
		"metadata": {
			"name": "empty-history",
			"namespace": "flux-system"
		},
		"status": {
			"history": []
		}
	}`

	history, err := ParseFluxResourceHistory([]byte(jsonData))
	if err != nil {
		t.Fatalf("ParseFluxResourceHistory() error = %v", err)
	}

	if len(history) != 0 {
		t.Errorf("Expected empty history, got %d entries", len(history))
	}
}
