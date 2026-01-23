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
