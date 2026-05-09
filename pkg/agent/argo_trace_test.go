// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

// TestArgoTracer_MultiSource covers Argo CD ≥ v2.6 spec.sources[]
// detection (#409 Phase 3). Single-source apps use spec.source; new-form
// single-source apps use a one-element spec.sources[]; multi-source apps
// have len > 1. Only the last form should set MultiSource=true.
func TestArgoTracer_MultiSource(t *testing.T) {
	tracer := &ArgoTracer{}

	cases := []struct {
		name           string
		json           string
		wantMultiSrc   bool
		wantSourceURL  string
	}{
		{
			name: "legacy single-source via spec.source",
			json: `{
				"metadata": {"name": "a", "namespace": "argocd"},
				"spec": {"source": {"repoURL": "https://github.com/x/y", "targetRevision": "abc"}, "destination": {}},
				"status": {"sync": {}, "health": {}}
			}`,
			wantMultiSrc:  false,
			wantSourceURL: "https://github.com/x/y",
		},
		{
			name: "single-source new form: spec.sources[] with one entry",
			json: `{
				"metadata": {"name": "a", "namespace": "argocd"},
				"spec": {"sources": [{"repoURL": "https://github.com/x/y", "targetRevision": "abc"}], "destination": {}},
				"status": {"sync": {}, "health": {}}
			}`,
			wantMultiSrc:  false,
			wantSourceURL: "https://github.com/x/y",
		},
		{
			name: "multi-source: spec.sources[] with two entries",
			json: `{
				"metadata": {"name": "a", "namespace": "argocd"},
				"spec": {"sources": [
					{"repoURL": "https://github.com/x/y", "targetRevision": "abc"},
					{"repoURL": "https://charts.example.com", "chart": "redis", "targetRevision": "17.0.0"}
				], "destination": {}},
				"status": {"sync": {}, "health": {}}
			}`,
			wantMultiSrc:  true,
			wantSourceURL: "https://github.com/x/y",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tracer.parseAppOutput([]byte(tc.json), "a", "argocd")
			if err != nil {
				t.Fatalf("parseAppOutput error: %v", err)
			}
			if res.MultiSource != tc.wantMultiSrc {
				t.Errorf("MultiSource = %v, want %v", res.MultiSource, tc.wantMultiSrc)
			}
			if len(res.Chain) == 0 {
				t.Fatal("chain is empty")
			}
			if res.Chain[0].URL != tc.wantSourceURL {
				t.Errorf("Chain[0].URL = %q, want %q", res.Chain[0].URL, tc.wantSourceURL)
			}
		})
	}
}

func TestArgoTrace_SourceSyncSignals_OutOfSync(t *testing.T) {
	tracer := NewArgoTracer()

	jsonData := `{
		"metadata": {
			"name": "checkout-app",
			"namespace": "argocd"
		},
		"spec": {
			"source": {
				"repoURL": "https://github.com/acme/checkout.git",
				"path": "./apps/checkout",
				"targetRevision": "main"
			},
			"destination": {
				"server": "https://kubernetes.default.svc",
				"namespace": "checkout"
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
			"reconciledAt": "2026-03-01T12:34:56Z",
			"resources": []
		}
	}`

	result, err := tracer.parseAppOutput([]byte(jsonData), "checkout-app", "argocd")
	if err != nil {
		t.Fatalf("parseAppOutput() error = %v", err)
	}

	if len(result.Chain) == 0 {
		t.Fatal("expected source link in trace chain")
	}

	source := result.Chain[0]
	if source.Kind != "Source" {
		t.Fatalf("source kind = %q, want Source", source.Kind)
	}
	if source.Ready {
		t.Fatalf("source.Ready = true, want false when app sync is OutOfSync")
	}
	if !strings.Contains(source.Status, "OutOfSync") {
		t.Fatalf("source.Status = %q, want to contain OutOfSync", source.Status)
	}
	if !strings.Contains(source.Message, "reconciledAt=2026-03-01T12:34:56Z") {
		t.Fatalf("source.Message = %q, want reconciledAt signal", source.Message)
	}
	if source.LastTransitionTime == nil {
		t.Fatal("source.LastTransitionTime = nil, want parsed reconciledAt time")
	}
}

func TestArgoTrace_SourceStalenessSignal_FromHistoryFallback(t *testing.T) {
	tracer := NewArgoTracer()

	jsonData := `{
		"metadata": {
			"name": "billing-app",
			"namespace": "argocd"
		},
		"spec": {
			"source": {
				"repoURL": "https://github.com/acme/billing.git",
				"path": "./apps/billing",
				"targetRevision": "main"
			},
			"destination": {
				"server": "https://kubernetes.default.svc",
				"namespace": "billing"
			}
		},
		"status": {
			"sync": {
				"status": "Synced",
				"revision": "def456"
			},
			"health": {
				"status": "Healthy"
			},
			"resources": [],
			"history": [
				{
					"revision": "def456",
					"deployedAt": "2026-02-15T10:00:00Z"
				}
			]
		}
	}`

	result, err := tracer.parseAppOutput([]byte(jsonData), "billing-app", "argocd")
	if err != nil {
		t.Fatalf("parseAppOutput() error = %v", err)
	}

	if len(result.Chain) == 0 {
		t.Fatal("expected source link in trace chain")
	}

	source := result.Chain[0]
	if !source.Ready {
		t.Fatalf("source.Ready = false, want true for Synced app")
	}
	if !strings.Contains(source.Message, "history.deployedAt=2026-02-15T10:00:00Z") {
		t.Fatalf("source.Message = %q, want history fallback signal", source.Message)
	}
	if source.LastTransitionTime == nil {
		t.Fatal("source.LastTransitionTime = nil, want history-derived timing")
	}
}

// ============================================================================
// History Feature Tests (Task 2: ArgoCD history extraction)
// ============================================================================

func TestArgoParseHistory(t *testing.T) {
	tracer := NewArgoTracer()

	jsonData := `{
		"metadata": {
			"name": "nginx-app",
			"namespace": "argocd"
		},
		"spec": {
			"source": {
				"repoURL": "https://github.com/org/repo.git",
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
				"revision": "abc123"
			},
			"health": {
				"status": "Healthy"
			},
			"resources": [],
			"history": [
				{
					"revision": "abc123def456789",
					"deployedAt": "2026-01-28T10:00:00Z"
				},
				{
					"revision": "previous123456",
					"deployedAt": "2026-01-27T08:30:00Z"
				},
				{
					"revision": "older789012345",
					"deployedAt": "2026-01-25T14:15:00Z"
				}
			]
		}
	}`

	result, err := tracer.parseAppOutput([]byte(jsonData), "nginx-app", "argocd")
	if err != nil {
		t.Fatalf("parseAppOutput() error = %v", err)
	}

	// Verify history is populated
	if len(result.History) != 3 {
		t.Fatalf("Expected 3 history entries, got %d", len(result.History))
	}

	// Verify first entry (most recent)
	if result.History[0].Revision != "abc123def456789" {
		t.Errorf("History[0].Revision = %q, want %q", result.History[0].Revision, "abc123def456789")
	}
	if result.History[0].Status != "deployed" {
		t.Errorf("History[0].Status = %q, want %q", result.History[0].Status, "deployed")
	}
	if result.History[0].Timestamp.IsZero() {
		t.Error("History[0].Timestamp should not be zero")
	}

	// Verify second entry
	if result.History[1].Revision != "previous123456" {
		t.Errorf("History[1].Revision = %q, want %q", result.History[1].Revision, "previous123456")
	}

	// Verify third entry
	if result.History[2].Revision != "older789012345" {
		t.Errorf("History[2].Revision = %q, want %q", result.History[2].Revision, "older789012345")
	}
}

func TestArgoHistoryEmpty(t *testing.T) {
	tracer := NewArgoTracer()

	// Application with no history (newly created or history cleared)
	jsonData := `{
		"metadata": {
			"name": "new-app",
			"namespace": "argocd"
		},
		"spec": {
			"source": {
				"repoURL": "https://github.com/org/new-repo.git",
				"targetRevision": "main"
			},
			"destination": {
				"server": "https://kubernetes.default.svc",
				"namespace": "default"
			}
		},
		"status": {
			"sync": {
				"status": "Synced",
				"revision": "abc123"
			},
			"health": {
				"status": "Healthy"
			},
			"resources": []
		}
	}`

	result, err := tracer.parseAppOutput([]byte(jsonData), "new-app", "argocd")
	if err != nil {
		t.Fatalf("parseAppOutput() error = %v", err)
	}

	// Empty history should be valid (nil or empty slice)
	if result.History != nil && len(result.History) != 0 {
		t.Errorf("Expected nil or empty history, got %d entries", len(result.History))
	}
}

func TestArgoHistoryWithEmptyArray(t *testing.T) {
	tracer := NewArgoTracer()

	// Application with explicit empty history array
	jsonData := `{
		"metadata": {
			"name": "empty-history-app",
			"namespace": "argocd"
		},
		"spec": {
			"source": {
				"repoURL": "https://github.com/org/repo.git",
				"targetRevision": "main"
			},
			"destination": {
				"server": "https://kubernetes.default.svc",
				"namespace": "default"
			}
		},
		"status": {
			"sync": {
				"status": "Synced",
				"revision": "abc123"
			},
			"health": {
				"status": "Healthy"
			},
			"resources": [],
			"history": []
		}
	}`

	result, err := tracer.parseAppOutput([]byte(jsonData), "empty-history-app", "argocd")
	if err != nil {
		t.Fatalf("parseAppOutput() error = %v", err)
	}

	// Empty history array should result in nil or empty History
	if len(result.History) != 0 {
		t.Errorf("Expected empty history, got %d entries", len(result.History))
	}
}

// ============================================================================
// App-of-Apps and ApplicationSet Lineage Tests (#194, #195)
// ============================================================================

func TestArgoTrace_AppOfApps_OwnerRef(t *testing.T) {
	tracer := NewArgoTracer()

	jsonData := `{
		"metadata": {
			"name": "payments-dev",
			"namespace": "argocd",
			"ownerReferences": [
				{
					"apiVersion": "argoproj.io/v1alpha1",
					"kind": "Application",
					"name": "platform-root",
					"controller": true
				}
			]
		},
		"spec": {
			"source": {
				"repoURL": "https://github.com/org/platform.git",
				"path": "apps/payments-dev",
				"targetRevision": "main"
			},
			"destination": {
				"server": "https://kubernetes.default.svc",
				"namespace": "payments"
			}
		},
		"status": {
			"sync": {"status": "Synced", "revision": "abc123"},
			"health": {"status": "Healthy"},
			"resources": []
		}
	}`

	result, err := tracer.parseAppOutput([]byte(jsonData), "payments-dev", "argocd")
	if err != nil {
		t.Fatalf("parseAppOutput() error = %v", err)
	}

	if result.ParentApplication != "platform-root" {
		t.Errorf("ParentApplication = %q, want platform-root", result.ParentApplication)
	}
	if result.LineageConfidence != "explicit" {
		t.Errorf("LineageConfidence = %q, want explicit", result.LineageConfidence)
	}
}

func TestArgoTrace_AppOfApps_Label(t *testing.T) {
	tracer := NewArgoTracer()

	jsonData := `{
		"metadata": {
			"name": "orders-dev",
			"namespace": "argocd",
			"labels": {
				"app.kubernetes.io/part-of": "platform-root"
			}
		},
		"spec": {
			"source": {
				"repoURL": "https://github.com/org/platform.git",
				"path": "apps/orders-dev",
				"targetRevision": "main"
			},
			"destination": {
				"server": "https://kubernetes.default.svc",
				"namespace": "orders"
			}
		},
		"status": {
			"sync": {"status": "Synced", "revision": "def456"},
			"health": {"status": "Healthy"},
			"resources": []
		}
	}`

	result, err := tracer.parseAppOutput([]byte(jsonData), "orders-dev", "argocd")
	if err != nil {
		t.Fatalf("parseAppOutput() error = %v", err)
	}

	if result.ParentApplication != "platform-root" {
		t.Errorf("ParentApplication = %q, want platform-root", result.ParentApplication)
	}
	if result.LineageConfidence != "inferred" {
		t.Errorf("LineageConfidence = %q, want inferred", result.LineageConfidence)
	}
}

func TestArgoTrace_ApplicationSet_OwnerRef(t *testing.T) {
	tracer := NewArgoTracer()

	jsonData := `{
		"metadata": {
			"name": "workloads-dev",
			"namespace": "argocd",
			"ownerReferences": [
				{
					"apiVersion": "argoproj.io/v1alpha1",
					"kind": "ApplicationSet",
					"name": "workloads-generator"
				}
			]
		},
		"spec": {
			"source": {
				"repoURL": "https://github.com/org/platform.git",
				"path": "envs/dev",
				"targetRevision": "main"
			},
			"destination": {
				"server": "https://kubernetes.default.svc",
				"namespace": "default"
			}
		},
		"status": {
			"sync": {"status": "Synced", "revision": "abc123"},
			"health": {"status": "Healthy"},
			"resources": []
		}
	}`

	result, err := tracer.parseAppOutput([]byte(jsonData), "workloads-dev", "argocd")
	if err != nil {
		t.Fatalf("parseAppOutput() error = %v", err)
	}

	if result.GeneratedByApplicationSet != "workloads-generator" {
		t.Errorf("GeneratedByApplicationSet = %q, want workloads-generator", result.GeneratedByApplicationSet)
	}
	if result.LineageConfidence != "explicit" {
		t.Errorf("LineageConfidence = %q, want explicit", result.LineageConfidence)
	}
}

func TestArgoTrace_ApplicationSet_Label(t *testing.T) {
	tracer := NewArgoTracer()

	jsonData := `{
		"metadata": {
			"name": "workloads-prod",
			"namespace": "argocd",
			"labels": {
				"argocd.argoproj.io/application-set-name": "workloads-generator"
			}
		},
		"spec": {
			"source": {
				"repoURL": "https://github.com/org/platform.git",
				"path": "envs/prod",
				"targetRevision": "main"
			},
			"destination": {
				"server": "https://kubernetes.default.svc",
				"namespace": "default"
			}
		},
		"status": {
			"sync": {"status": "Synced", "revision": "def456"},
			"health": {"status": "Healthy"},
			"resources": []
		}
	}`

	result, err := tracer.parseAppOutput([]byte(jsonData), "workloads-prod", "argocd")
	if err != nil {
		t.Fatalf("parseAppOutput() error = %v", err)
	}

	if result.GeneratedByApplicationSet != "workloads-generator" {
		t.Errorf("GeneratedByApplicationSet = %q, want workloads-generator", result.GeneratedByApplicationSet)
	}
	if result.LineageConfidence != "inferred" {
		t.Errorf("LineageConfidence = %q, want inferred", result.LineageConfidence)
	}
}

func TestArgoTrace_NoLineage(t *testing.T) {
	tracer := NewArgoTracer()

	jsonData := `{
		"metadata": {
			"name": "standalone-app",
			"namespace": "argocd"
		},
		"spec": {
			"source": {
				"repoURL": "https://github.com/org/app.git",
				"targetRevision": "main"
			},
			"destination": {
				"server": "https://kubernetes.default.svc",
				"namespace": "default"
			}
		},
		"status": {
			"sync": {"status": "Synced", "revision": "abc123"},
			"health": {"status": "Healthy"},
			"resources": []
		}
	}`

	result, err := tracer.parseAppOutput([]byte(jsonData), "standalone-app", "argocd")
	if err != nil {
		t.Fatalf("parseAppOutput() error = %v", err)
	}

	if result.ParentApplication != "" {
		t.Errorf("ParentApplication = %q, want empty", result.ParentApplication)
	}
	if result.GeneratedByApplicationSet != "" {
		t.Errorf("GeneratedByApplicationSet = %q, want empty", result.GeneratedByApplicationSet)
	}
	if result.LineageConfidence != "" {
		t.Errorf("LineageConfidence = %q, want empty", result.LineageConfidence)
	}
}

func TestArgoTrace_SelfReferenceIgnored(t *testing.T) {
	tracer := NewArgoTracer()

	// App whose part-of label points to itself should not report as child
	jsonData := `{
		"metadata": {
			"name": "self-ref",
			"namespace": "argocd",
			"labels": {
				"app.kubernetes.io/part-of": "self-ref"
			}
		},
		"spec": {
			"source": {
				"repoURL": "https://github.com/org/app.git",
				"targetRevision": "main"
			},
			"destination": {
				"server": "https://kubernetes.default.svc",
				"namespace": "default"
			}
		},
		"status": {
			"sync": {"status": "Synced", "revision": "abc123"},
			"health": {"status": "Healthy"},
			"resources": []
		}
	}`

	result, err := tracer.parseAppOutput([]byte(jsonData), "self-ref", "argocd")
	if err != nil {
		t.Fatalf("parseAppOutput() error = %v", err)
	}

	if result.ParentApplication != "" {
		t.Errorf("ParentApplication = %q, want empty (self-reference should be ignored)", result.ParentApplication)
	}
}

func TestFormatArgoContextError(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		wantMatch  bool
		wantReason string
	}{
		{
			name:       "stale endpoint",
			output:     "rpc error: code = Unavailable desc = connection error: dial tcp 10.0.0.1:443: i/o timeout",
			wantMatch:  true,
			wantReason: "unreachable or stale",
		},
		{
			name:       "missing login",
			output:     "FATA[0000] Argo CD server address unspecified",
			wantMatch:  true,
			wantReason: "missing or expired",
		},
		{
			name:      "unrelated command failure",
			output:    "application not found",
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := FormatArgoContextError(tt.output)
			if ok != tt.wantMatch {
				t.Fatalf("FormatArgoContextError() match = %v, want %v (msg=%q)", ok, tt.wantMatch, got)
			}
			if !tt.wantMatch {
				return
			}

			// Ensure remediation path is always present and actionable.
			required := []string{
				"argocd context",
				"argocd app list",
				"argocd logout <server>",
				"argocd login <server>",
				"cub-scout trace --app <app-name>",
			}
			for _, r := range required {
				if !strings.Contains(got, r) {
					t.Fatalf("expected remediation command %q in message: %s", r, got)
				}
			}

			if !strings.Contains(got, tt.wantReason) {
				t.Fatalf("expected reason fragment %q in message: %s", tt.wantReason, got)
			}
		})
	}
}

func TestArgoTraceApplication_FallsBackToKubectlWhenArgoCLIContextIsStale(t *testing.T) {
	tempDir := t.TempDir()
	argocdPath := writeExecScript(t, tempDir, "argocd", `#!/usr/bin/env bash
echo "rpc error: code = Unavailable desc = connection error: dial tcp 10.0.0.1:443: i/o timeout" >&2
exit 1
`)
	kubectlPath := writeExecScript(t, tempDir, "kubectl", `#!/usr/bin/env bash
cat <<'JSON'
{
  "items": [
    {
      "metadata": {"name": "checkout", "namespace": "argocd"},
      "spec": {
        "source": {"repoURL": "https://github.com/acme/checkout.git", "path": "envs/prod", "targetRevision": "main"},
        "destination": {"server": "https://kubernetes.default.svc", "namespace": "prod"}
      },
      "status": {
        "sync": {"status": "Synced", "revision": "abc123"},
        "health": {"status": "Healthy"},
        "resources": []
      }
    }
  ]
}
JSON
`)

	tracer := NewArgoTracerWithPaths(argocdPath, kubectlPath)
	result, err := tracer.TraceApplication(context.Background(), "checkout")
	if err != nil {
		t.Fatalf("TraceApplication() fallback error = %v", err)
	}
	if result == nil {
		t.Fatalf("TraceApplication() result is nil")
	}
	if !strings.Contains(result.Error, "ArgoCD CLI unavailable") {
		t.Fatalf("expected degraded-mode warning in result.Error, got: %q", result.Error)
	}
	if result.Object.Namespace != "argocd" {
		t.Fatalf("result.Object.Namespace = %q, want argocd", result.Object.Namespace)
	}
	if len(result.Chain) < 2 {
		t.Fatalf("expected source + application chain from kubectl fallback, got len=%d", len(result.Chain))
	}
	if result.Chain[1].Kind != "Application" {
		t.Fatalf("expected second link kind Application, got %q", result.Chain[1].Kind)
	}
}

func TestArgoTraceApplication_FallbackHonorsNamespaceFilter(t *testing.T) {
	tempDir := t.TempDir()
	argocdPath := writeExecScript(t, tempDir, "argocd", `#!/usr/bin/env bash
echo "FATA[0000] Argo CD server address unspecified" >&2
exit 1
`)
	kubectlPath := writeExecScript(t, tempDir, "kubectl", `#!/usr/bin/env bash
cat <<'JSON'
{
  "items": [
    {
      "metadata": {"name": "checkout", "namespace": "argocd"},
      "spec": {
        "source": {"repoURL": "https://github.com/acme/checkout.git", "targetRevision": "main"},
        "destination": {"server": "https://kubernetes.default.svc", "namespace": "prod"}
      },
      "status": {"sync": {"status": "Synced", "revision": "a1"}, "health": {"status": "Healthy"}, "resources": []}
    },
    {
      "metadata": {"name": "checkout", "namespace": "team-b"},
      "spec": {
        "source": {"repoURL": "https://github.com/acme/checkout-alt.git", "targetRevision": "main"},
        "destination": {"server": "https://kubernetes.default.svc", "namespace": "team-b"}
      },
      "status": {"sync": {"status": "OutOfSync", "revision": "b2"}, "health": {"status": "Degraded"}, "resources": []}
    }
  ]
}
JSON
`)

	tracer := NewArgoTracerWithPaths(argocdPath, kubectlPath)
	result, err := tracer.traceApplication(context.Background(), "checkout", "team-b")
	if err != nil {
		t.Fatalf("traceApplication() fallback error = %v", err)
	}
	if result.Object.Namespace != "team-b" {
		t.Fatalf("result.Object.Namespace = %q, want team-b", result.Object.Namespace)
	}
	if result.Chain[0].URL != "https://github.com/acme/checkout-alt.git" {
		t.Fatalf("selected fallback application repoURL = %q, want team-b app", result.Chain[0].URL)
	}
}

func TestArgoTraceApplication_ContextError_WhenKubectlFallbackFails(t *testing.T) {
	tempDir := t.TempDir()
	argocdPath := writeExecScript(t, tempDir, "argocd", `#!/usr/bin/env bash
echo "rpc error: code = Unavailable desc = connection error: dial tcp 10.0.0.1:443: i/o timeout" >&2
exit 1
`)
	kubectlPath := writeExecScript(t, tempDir, "kubectl", `#!/usr/bin/env bash
echo "error: forbidden" >&2
exit 1
`)

	tracer := NewArgoTracerWithPaths(argocdPath, kubectlPath)
	_, err := tracer.TraceApplication(context.Background(), "checkout")
	if err == nil {
		t.Fatalf("expected error when both argocd and kubectl fallback fail")
	}
	got := err.Error()
	if !strings.Contains(got, "argocd context appears stale or invalid") {
		t.Fatalf("expected stale-context remediation, got: %s", got)
	}
	if !strings.Contains(got, "kubectl fallback failed") {
		t.Fatalf("expected kubectl fallback failure detail, got: %s", got)
	}
}

func writeExecScript(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write script %s: %v", name, err)
	}
	return path
}
