// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"
)

func TestFluxTracerConfigHubOCIDetection(t *testing.T) {
	tests := []struct {
		name      string
		section   string
		expectOCI bool
		expectCH  bool
		space     string
		target    string
	}{
		{
			name: "ConfigHub OCIRepository",
			section: `OCIRepository: confighub-prod
Namespace:     flux-system
URL:           oci://oci.api.confighub.com/target/prod/us-west
Status:        stored artifact for revision 'latest@sha1:abc123def456'
Revision:      latest@sha1:abc123def456`,
			expectOCI: true,
			expectCH:  true,
			space:     "prod",
			target:    "us-west",
		},
		{
			name: "Generic OCIRepository",
			section: `OCIRepository: ghcr-repo
Namespace:     flux-system
URL:           oci://ghcr.io/my-org/my-repo
Status:        stored artifact for revision 'v1.0.0@sha1:abc123'
Revision:      v1.0.0@sha1:abc123`,
			expectOCI: true,
			expectCH:  false,
		},
		{
			name: "GitRepository",
			section: `GitRepository: flux-system
Namespace:     flux-system
URL:           https://github.com/org/repo
Status:        stored artifact for revision 'main@sha1:abc123'
Revision:      main@sha1:abc123`,
			expectOCI: false,
			expectCH:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracer := NewFluxTracer()
			link, err := tracer.parseSection(tt.section)

			if err != nil {
				t.Fatalf("parseSection failed: %v", err)
			}

			if link == nil {
				t.Fatal("expected link to be non-nil")
			}

			// Verify OCI source parsing
			if tt.expectOCI {
				if link.OCISource == nil {
					t.Error("expected OCISource to be set for OCI URL")
				} else {
					if link.OCISource.IsConfigHub != tt.expectCH {
						t.Errorf("IsConfigHub = %v, want %v", link.OCISource.IsConfigHub, tt.expectCH)
					}

					if tt.expectCH {
						if link.OCISource.Space != tt.space {
							t.Errorf("Space = %q, want %q", link.OCISource.Space, tt.space)
						}
						if link.OCISource.Target != tt.target {
							t.Errorf("Target = %q, want %q", link.OCISource.Target, tt.target)
						}
						if link.Kind != "ConfigHub OCI" {
							t.Errorf("Kind = %q, want %q", link.Kind, "ConfigHub OCI")
						}
					} else {
						if link.Kind != "OCIRepository" {
							t.Errorf("Kind = %q, want %q for generic OCI", link.Kind, "OCIRepository")
						}
					}
				}
			} else {
				if link.OCISource != nil {
					t.Error("expected OCISource to be nil for non-OCI URL")
				}
			}
		})
	}
}
