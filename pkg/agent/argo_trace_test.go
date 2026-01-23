// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"
)

func TestArgoTracerParseAppOutput(t *testing.T) {
	tracer := NewArgoTracer()

	tests := []struct {
		name           string
		jsonData       string
		appName        string
		namespace      string
		wantChainLen   int
		wantFullyMgd   bool
		wantSyncStatus string
	}{
		{
			name: "healthy synced application",
			jsonData: `{
				"metadata": {
					"name": "frontend-app",
					"namespace": "argocd"
				},
				"spec": {
					"source": {
						"repoURL": "https://github.com/your-org/frontend.git",
						"path": "./deploy",
						"targetRevision": "main"
					},
					"destination": {
						"server": "https://kubernetes.default.svc",
						"namespace": "production"
					}
				},
				"status": {
					"sync": {
						"status": "Synced",
						"revision": "abc123def456"
					},
					"health": {
						"status": "Healthy"
					},
					"resources": [
						{
							"group": "apps",
							"version": "v1",
							"kind": "Deployment",
							"namespace": "production",
							"name": "frontend",
							"status": "Synced",
							"health": {
								"status": "Healthy"
							}
						},
						{
							"group": "",
							"version": "v1",
							"kind": "Service",
							"namespace": "production",
							"name": "frontend",
							"status": "Synced",
							"health": {
								"status": "Healthy"
							}
						}
					]
				}
			}`,
			appName:        "frontend-app",
			namespace:      "argocd",
			wantChainLen:   4, // Source + Application + 2 resources
			wantFullyMgd:   true,
			wantSyncStatus: "Synced / Healthy",
		},
		{
			name: "out of sync application",
			jsonData: `{
				"metadata": {
					"name": "backend-app",
					"namespace": "argocd"
				},
				"spec": {
					"source": {
						"repoURL": "https://github.com/your-org/backend.git",
						"path": "./k8s",
						"targetRevision": "main"
					},
					"destination": {
						"server": "https://kubernetes.default.svc",
						"namespace": "production"
					}
				},
				"status": {
					"sync": {
						"status": "OutOfSync",
						"revision": "abc123"
					},
					"health": {
						"status": "Healthy"
					},
					"resources": [
						{
							"kind": "Deployment",
							"namespace": "production",
							"name": "backend",
							"status": "OutOfSync",
							"health": {
								"status": "Healthy"
							}
						}
					]
				}
			}`,
			appName:        "backend-app",
			namespace:      "argocd",
			wantChainLen:   3, // Source + Application + 1 resource
			wantFullyMgd:   false,
			wantSyncStatus: "OutOfSync / Healthy",
		},
		{
			name: "degraded application",
			jsonData: `{
				"metadata": {
					"name": "failing-app",
					"namespace": "argocd"
				},
				"spec": {
					"source": {
						"repoURL": "https://github.com/your-org/failing.git",
						"targetRevision": "main"
					},
					"destination": {
						"server": "https://kubernetes.default.svc",
						"namespace": "staging"
					}
				},
				"status": {
					"sync": {
						"status": "Synced",
						"revision": "def456"
					},
					"health": {
						"status": "Degraded",
						"message": "Pod is crash looping"
					},
					"resources": [
						{
							"kind": "Deployment",
							"namespace": "staging",
							"name": "failing",
							"status": "Synced",
							"health": {
								"status": "Degraded",
								"message": "Pod is crash looping"
							}
						}
					]
				}
			}`,
			appName:        "failing-app",
			namespace:      "argocd",
			wantChainLen:   3,
			wantFullyMgd:   false,
			wantSyncStatus: "Synced / Degraded",
		},
		{
			name: "helm chart application",
			jsonData: `{
				"metadata": {
					"name": "redis",
					"namespace": "argocd"
				},
				"spec": {
					"source": {
						"repoURL": "https://charts.bitnami.com/bitnami",
						"chart": "redis",
						"targetRevision": "17.0.0"
					},
					"destination": {
						"server": "https://kubernetes.default.svc",
						"namespace": "cache"
					}
				},
				"status": {
					"sync": {
						"status": "Synced",
						"revision": "17.0.0"
					},
					"health": {
						"status": "Healthy"
					},
					"resources": []
				}
			}`,
			appName:        "redis",
			namespace:      "argocd",
			wantChainLen:   2, // HelmChart source + Application (no resources)
			wantFullyMgd:   true,
			wantSyncStatus: "Synced / Healthy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tracer.parseAppOutput([]byte(tt.jsonData), tt.appName, tt.namespace)
			if err != nil {
				t.Fatalf("parseAppOutput() error = %v", err)
			}

			if len(result.Chain) != tt.wantChainLen {
				t.Errorf("Chain length = %d, want %d", len(result.Chain), tt.wantChainLen)
				for i, link := range result.Chain {
					t.Logf("  Chain[%d]: %s/%s", i, link.Kind, link.Name)
				}
			}

			if result.FullyManaged != tt.wantFullyMgd {
				t.Errorf("FullyManaged = %v, want %v", result.FullyManaged, tt.wantFullyMgd)
			}

			// Check Application link status
			for _, link := range result.Chain {
				if link.Kind == "Application" {
					if link.Status != tt.wantSyncStatus {
						t.Errorf("Application status = %q, want %q", link.Status, tt.wantSyncStatus)
					}
					break
				}
			}

			if result.Tool != "argocd" {
				t.Errorf("Tool = %q, want %q", result.Tool, "argocd")
			}
		})
	}
}

func TestExtractRepoName(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://github.com/your-org/infra.git", "your-org/infra"},
		{"https://github.com/your-org/infra", "your-org/infra"},
		{"git@github.com:your-org/infra.git", "your-org/infra"},
		{"https://charts.bitnami.com/bitnami", "charts.bitnami.com/bitnami"},
		{"ssh://git@gitlab.com/team/project.git", "team/project"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result := extractRepoName(tt.url)
			if result != tt.want {
				t.Errorf("extractRepoName(%q) = %q, want %q", tt.url, result, tt.want)
			}
		})
	}
}

func TestArgoTracerToolName(t *testing.T) {
	tracer := NewArgoTracer()
	if tracer.ToolName() != "argocd" {
		t.Errorf("ToolName() = %q, want %q", tracer.ToolName(), "argocd")
	}
}

func TestArgoTracerWithPath(t *testing.T) {
	tracer := NewArgoTracerWithPath("/custom/path/argocd")
	if tracer.argocdPath != "/custom/path/argocd" {
		t.Errorf("argocdPath = %q, want %q", tracer.argocdPath, "/custom/path/argocd")
	}
}

func TestArgoTracerParseAppOutputError(t *testing.T) {
	tracer := NewArgoTracer()

	// Invalid JSON
	_, err := tracer.parseAppOutput([]byte("not json"), "app", "ns")
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}

	// Empty JSON
	_, err = tracer.parseAppOutput([]byte("{}"), "app", "ns")
	if err != nil {
		t.Errorf("Unexpected error for empty JSON: %v", err)
	}
}
