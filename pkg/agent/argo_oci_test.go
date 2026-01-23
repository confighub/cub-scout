// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"
)

func TestArgoTracerConfigHubOCIDetection(t *testing.T) {
	// Test that Argo tracer correctly parses and identifies ConfigHub OCI sources
	tests := []struct {
		name      string
		repoURL   string
		expectOCI bool
		expectCH  bool
		space     string
		target    string
	}{
		{
			name:      "ConfigHub OCI URL",
			repoURL:   "oci://oci.api.confighub.com/target/prod/us-west",
			expectOCI: true,
			expectCH:  true,
			space:     "prod",
			target:    "us-west",
		},
		{
			name:      "ConfigHub OCI URL local instance",
			repoURL:   "oci://oci.localhost:8080/target/qa/qa-cluster",
			expectOCI: true,
			expectCH:  true,
			space:     "qa",
			target:    "qa-cluster",
		},
		{
			name:      "Generic OCI URL",
			repoURL:   "oci://ghcr.io/my-org/my-repo",
			expectOCI: true,
			expectCH:  false,
		},
		{
			name:      "Git URL",
			repoURL:   "https://github.com/org/repo",
			expectOCI: false,
			expectCH:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock ArgoCD app output
			appJSON := []byte(`{
				"metadata": {
					"name": "test-app",
					"namespace": "argocd"
				},
				"spec": {
					"source": {
						"repoURL": "` + tt.repoURL + `",
						"path": ".",
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
			}`)

			tracer := NewArgoTracer()
			result, err := tracer.parseAppOutput(appJSON, "test-app", "argocd")

			if err != nil {
				t.Fatalf("parseAppOutput failed: %v", err)
			}

			if len(result.Chain) == 0 {
				t.Fatal("expected at least one chain link (source)")
			}

			sourceLink := result.Chain[0]

			// Verify OCI source is parsed
			if tt.expectOCI {
				if sourceLink.OCISource == nil {
					t.Error("expected OCISource to be set for OCI URL")
				} else {
					if sourceLink.OCISource.IsConfigHub != tt.expectCH {
						t.Errorf("IsConfigHub = %v, want %v", sourceLink.OCISource.IsConfigHub, tt.expectCH)
					}

					if tt.expectCH {
						if sourceLink.OCISource.Space != tt.space {
							t.Errorf("Space = %q, want %q", sourceLink.OCISource.Space, tt.space)
						}
						if sourceLink.OCISource.Target != tt.target {
							t.Errorf("Target = %q, want %q", sourceLink.OCISource.Target, tt.target)
						}
						if sourceLink.Kind != "ConfigHub OCI" {
							t.Errorf("Kind = %q, want %q", sourceLink.Kind, "ConfigHub OCI")
						}
					} else {
						if sourceLink.Kind != "OCIRepository" {
							t.Errorf("Kind = %q, want %q for generic OCI", sourceLink.Kind, "OCIRepository")
						}
					}
				}
			} else {
				if sourceLink.OCISource != nil {
					t.Error("expected OCISource to be nil for non-OCI URL")
				}
				if sourceLink.Kind != "Source" {
					t.Errorf("Kind = %q, want %q for non-OCI source", sourceLink.Kind, "Source")
				}
			}
		})
	}
}
