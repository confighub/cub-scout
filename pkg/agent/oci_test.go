// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"
)

func TestOCISourceCannotWidenConnectedScope(t *testing.T) {
	for _, path := range []string{"space/*", "space/a?scope=*", "space/a%2Fb", "space/..", "space/a#fragment", "target/*/cluster", "target/team/*"} {
		info := ParseOCISourceForRegistry("oci://localhost:32181/"+path, "localhost:32181")
		if info.IsConfigHub || info.Space != "" {
			t.Fatalf("malformed source acquired a connected scope: %+v", info)
		}
	}
}

func TestParseOCISource(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected OCISourceInfo
	}{
		{
			name: "ConfigHub OCI URL",
			url:  "oci://oci.api.confighub.com/target/prod/us-west",
			expected: OCISourceInfo{
				Raw:         "oci://oci.api.confighub.com/target/prod/us-west",
				IsConfigHub: true,
				Instance:    "api.confighub.com",
				Space:       "prod",
				Target:      "us-west",
				Registry:    "oci.api.confighub.com",
				Repository:  "target/prod/us-west",
			},
		},
		{
			name: "Unknown local registry is only a candidate",
			url:  "oci://oci.unknown.local:8080/target/qa/qa-cluster",
			expected: OCISourceInfo{
				Raw:         "oci://oci.unknown.local:8080/target/qa/qa-cluster",
				IsConfigHub: false,
				Registry:    "oci.unknown.local:8080",
				Repository:  "target/qa/qa-cluster",
			},
		},
		{
			name: "Generic OCI URL",
			url:  "oci://ghcr.io/my-org/my-repo",
			expected: OCISourceInfo{
				Raw:         "oci://ghcr.io/my-org/my-repo",
				IsConfigHub: false,
				Registry:    "ghcr.io",
				Repository:  "my-org/my-repo",
			},
		},
		{
			name: "Generic OCI URL with path",
			url:  "oci://registry.example.com/apps/backend",
			expected: OCISourceInfo{
				Raw:         "oci://registry.example.com/apps/backend",
				IsConfigHub: false,
				Registry:    "registry.example.com",
				Repository:  "apps/backend",
			},
		},
		{
			name: "OCI registry prefix without ConfigHub target path",
			url:  "oci://oci.confighub.com/some/other/path",
			expected: OCISourceInfo{
				Raw:         "oci://oci.confighub.com/some/other/path",
				IsConfigHub: false,
				Registry:    "oci.confighub.com",
				Repository:  "some/other/path",
			},
		},
		{
			name: "Invalid URL - no oci:// prefix",
			url:  "https://github.com/org/repo",
			expected: OCISourceInfo{
				Raw:         "https://github.com/org/repo",
				IsConfigHub: false,
			},
		},
		{
			name: "Invalid OCI URL - no registry",
			url:  "oci://",
			expected: OCISourceInfo{
				Raw:      "oci://",
				Registry: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseOCISource(tt.url)

			if result.Raw != tt.expected.Raw {
				t.Errorf("Raw = %q, want %q", result.Raw, tt.expected.Raw)
			}
			if result.IsConfigHub != tt.expected.IsConfigHub {
				t.Errorf("IsConfigHub = %v, want %v", result.IsConfigHub, tt.expected.IsConfigHub)
			}
			if result.Instance != tt.expected.Instance {
				t.Errorf("Instance = %q, want %q", result.Instance, tt.expected.Instance)
			}
			if result.Space != tt.expected.Space {
				t.Errorf("Space = %q, want %q", result.Space, tt.expected.Space)
			}
			if result.Target != tt.expected.Target {
				t.Errorf("Target = %q, want %q", result.Target, tt.expected.Target)
			}
			if result.Registry != tt.expected.Registry {
				t.Errorf("Registry = %q, want %q", result.Registry, tt.expected.Registry)
			}
			if result.Repository != tt.expected.Repository {
				t.Errorf("Repository = %q, want %q", result.Repository, tt.expected.Repository)
			}
		})
	}
}

func TestIsConfigHubOCI(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{
			name:     "ConfigHub OCI URL",
			url:      "oci://oci.api.confighub.com/target/prod/us-west",
			expected: true,
		},
		{
			name:     "Unknown local registry",
			url:      "oci://oci.unknown.local:8080/target/qa/qa",
			expected: false,
		},
		{
			name:     "Generic OCI URL",
			url:      "oci://ghcr.io/org/repo",
			expected: false,
		},
		{
			name:     "ConfigHub OCI without target prefix",
			url:      "oci://oci.confighub.com/some/other/path",
			expected: false,
		},
		{
			name:     "Not an OCI URL",
			url:      "https://github.com/org/repo",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsConfigHubOCI(tt.url)
			if result != tt.expected {
				t.Errorf("IsConfigHubOCI(%q) = %v, want %v", tt.url, result, tt.expected)
			}
		})
	}
}

func TestParseOCISourceUsesConfiguredRegistryWithoutCallingBackForUnrelatedShapes(t *testing.T) {
	previous := ConfigHubOCIRegistryFn
	defer func() { ConfigHubOCIRegistryFn = previous }()

	var calls int
	ConfigHubOCIRegistryFn = func() string {
		calls++
		return "localhost:32181"
	}

	if got := ParseOCISource("oci://ghcr.io/org/repo"); got.IsConfigHub {
		t.Fatal("generic OCI source was claimed as ConfigHub")
	}
	if calls != 0 {
		t.Fatalf("registry callback calls = %d for unrelated repository shape, want 0", calls)
	}

	got := ParseOCISource("oci://localhost:32181/space/apps")
	if !got.IsConfigHub || got.Space != "apps" {
		t.Fatalf("configured source = %+v, want ConfigHub space/apps", got)
	}
	if calls != 1 {
		t.Fatalf("registry callback calls = %d, want 1 for a candidate layout", calls)
	}
}

func TestParseOCISourceRejectsUnrelatedOCIHostPrefix(t *testing.T) {
	got := ParseOCISourceForRegistry("oci://oci.unrelated.invalid/space/apps", "localhost:32181")
	if got.IsConfigHub {
		t.Fatal("unrelated oci.* registry was claimed as ConfigHub")
	}
	if got.SpaceCandidate != "apps" || got.Space != "" {
		t.Fatalf("candidate = %+v, want only SpaceCandidate=apps", got)
	}
}

func TestFormatConfigHubOCISource(t *testing.T) {
	tests := []struct {
		name     string
		info     OCISourceInfo
		expected string
	}{
		{
			name: "ConfigHub source with space and target",
			info: OCISourceInfo{
				Raw:         "oci://oci.api.confighub.com/target/prod/us-west",
				IsConfigHub: true,
				Space:       "prod",
				Target:      "us-west",
			},
			expected: "prod/us-west",
		},
		{
			name: "ConfigHub source without target info",
			info: OCISourceInfo{
				Raw:         "oci://oci.api.confighub.com/target/prod",
				IsConfigHub: true,
				Space:       "prod",
			},
			expected: "oci://oci.api.confighub.com/target/prod",
		},
		{
			name: "Generic OCI source",
			info: OCISourceInfo{
				Raw:         "oci://ghcr.io/org/repo",
				IsConfigHub: false,
			},
			expected: "oci://ghcr.io/org/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatConfigHubOCISource(tt.info)
			if result != tt.expected {
				t.Errorf("FormatConfigHubOCISource() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// The layouts ConfigHub has actually emitted, and what may be claimed about
// each. The `space/<slug>` shapes and the `localhost:32181` registry below are
// what a live ConfigHub v0.5.1 server produces: it advertises OCIHost/OCIPort
// at /api/info, and its registry answers any other path shape with
// `Invalid path format. Expected: /v2/space/{space}/manifests/{ref}`.
func TestParseOCISourceRecognisesBothConfigHubLayouts(t *testing.T) {
	const localRegistry = "localhost:32181"

	for _, tt := range []struct {
		name      string
		url       string
		registry  string // what the ConfigHub server advertises, "" if unknown
		claimed   bool   // IsConfigHub
		layout    OCILayout
		space     string // Space, only set when claimed
		candidate string // SpaceCandidate, set whoever serves it
		target    string
		reference string
	}{
		{
			name: "hosted target layout", url: "oci://oci.api.confighub.com/target/prod/us-west",
			claimed: true, layout: OCILayoutTarget, space: "prod", candidate: "prod", target: "us-west",
		},
		{
			name: "hosted space layout", url: "oci://oci.api.confighub.com/space/apps",
			claimed: true, layout: OCILayoutSpace, space: "apps", candidate: "apps",
		},
		{
			name: "local server, registry known", url: "oci://localhost:32181/space/apps",
			registry: localRegistry, claimed: true, layout: OCILayoutSpace, space: "apps", candidate: "apps",
		},
		{
			// The case the issue was filed for: the layout is recognised, but
			// with nothing to corroborate the host, nothing is claimed.
			name: "local server, registry unknown", url: "oci://localhost:32181/space/apps",
			claimed: false, layout: OCILayoutSpace, candidate: "apps",
		},
		{
			// A third-party registry may serve a repository called space/foo.
			// The path shape alone must not make it ConfigHub's.
			name: "someone else's space/ path", url: "oci://ghcr.io/space/apps",
			registry: localRegistry, claimed: false, layout: OCILayoutSpace, candidate: "apps",
		},
		{
			name: "release tag", url: "oci://localhost:32181/space/apps:release-3",
			registry: localRegistry, claimed: true, layout: OCILayoutSpace, space: "apps", candidate: "apps", reference: "release-3",
		},
		{
			name: "pinned by digest", url: "oci://localhost:32181/space/apps@sha256:9c8b626269b155494826050dc379cf66ca5772cc6a5778bc59d7cb7bf18958d5",
			registry: localRegistry, claimed: true, layout: OCILayoutSpace, space: "apps", candidate: "apps",
			reference: "sha256:9c8b626269b155494826050dc379cf66ca5772cc6a5778bc59d7cb7bf18958d5",
		},
		{
			// Deeper than the server serves, so it is not this layout.
			name: "space path with an extra segment", url: "oci://localhost:32181/space/apps/extra",
			registry: localRegistry, claimed: false, layout: OCILayoutNone,
		},
		{
			// The legacy target layout requires both the space and target.
			name: "target path without target", url: "oci://localhost:32181/target/apps",
			registry: localRegistry, claimed: false, layout: OCILayoutNone,
		},
		{
			name: "not ConfigHub at all", url: "oci://ghcr.io/org/repo",
			registry: localRegistry, claimed: false, layout: OCILayoutNone,
		},
		{
			name: "not an OCI url", url: "https://github.com/org/repo",
			registry: localRegistry, claimed: false, layout: OCILayoutNone,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseOCISourceForRegistry(tt.url, tt.registry)
			if got.IsConfigHub != tt.claimed {
				t.Fatalf("IsConfigHub = %v, want %v", got.IsConfigHub, tt.claimed)
			}
			if got.Layout != tt.layout {
				t.Fatalf("Layout = %v, want %v", got.Layout, tt.layout)
			}
			if got.Space != tt.space {
				t.Fatalf("Space = %q, want %q", got.Space, tt.space)
			}
			if got.SpaceCandidate != tt.candidate {
				t.Fatalf("SpaceCandidate = %q, want %q", got.SpaceCandidate, tt.candidate)
			}
			if got.Target != tt.target {
				t.Fatalf("Target = %q, want %q", got.Target, tt.target)
			}
			if got.Reference != tt.reference {
				t.Fatalf("Reference = %q, want %q", got.Reference, tt.reference)
			}
		})
	}
}

// A space is never claimed on a source cub-scout has not established is
// ConfigHub's: the candidate is there for a caller that can corroborate it.
func TestParseOCISourceClaimsNoSpaceWithoutEvidence(t *testing.T) {
	got := ParseOCISourceForRegistry("oci://ghcr.io/space/apps", "localhost:32181")
	if got.Space != "" {
		t.Fatalf("Space = %q for a registry that is not ConfigHub's, want empty", got.Space)
	}
	if got.SpaceCandidate != "apps" {
		t.Fatalf("SpaceCandidate = %q, want the slug the path names", got.SpaceCandidate)
	}
	if IsConfigHubOCIForRegistry("oci://ghcr.io/space/apps", "localhost:32181") {
		t.Fatal("IsConfigHubOCIForRegistry = true for someone else's registry")
	}
}
