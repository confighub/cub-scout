// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"
)

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
			name: "ConfigHub OCI URL with local instance",
			url:  "oci://oci.localhost:8080/target/qa/qa-cluster",
			expected: OCISourceInfo{
				Raw:         "oci://oci.localhost:8080/target/qa/qa-cluster",
				IsConfigHub: true,
				Instance:    "localhost:8080",
				Space:       "qa",
				Target:      "qa-cluster",
				Registry:    "oci.localhost:8080",
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
			name:     "ConfigHub OCI URL - local",
			url:      "oci://oci.localhost:8080/target/qa/qa",
			expected: true,
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
