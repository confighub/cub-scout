// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestGitSourceAnchor_IsEmpty(t *testing.T) {
	tests := []struct {
		name   string
		anchor *GitSourceAnchor
		want   bool
	}{
		{"nil anchor", nil, true},
		{"all empty", &GitSourceAnchor{}, true},
		{"whitespace only", &GitSourceAnchor{RepoURL: "  ", Revision: "\t", Path: " "}, true},
		{"repo only", &GitSourceAnchor{RepoURL: "https://github.com/org/repo"}, false},
		{"revision only", &GitSourceAnchor{Revision: "abc123"}, false},
		{"path only", &GitSourceAnchor{Path: "apps/prod"}, false},
		{"all populated", &GitSourceAnchor{RepoURL: "https://github.com/o/r", Revision: "abc", Path: "apps", Line: 0}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.anchor.IsEmpty()
			if got != tc.want {
				t.Errorf("IsEmpty = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAnchorFromChainRoot(t *testing.T) {
	tests := []struct {
		name string
		root ChainLink
		want *GitSourceAnchor
	}{
		{
			name: "git-repo style root",
			root: ChainLink{Kind: "GitRepository", URL: "https://github.com/org/repo", Revision: "main@sha1:abc123", Path: "apps/prod"},
			want: &GitSourceAnchor{RepoURL: "https://github.com/org/repo", Revision: "main@sha1:abc123", Path: "apps/prod"},
		},
		{
			name: "argo source root",
			root: ChainLink{Kind: "Application", URL: "https://github.com/org/repo", Revision: "abc123", Path: "manifests"},
			want: &GitSourceAnchor{RepoURL: "https://github.com/org/repo", Revision: "abc123", Path: "manifests"},
		},
		{
			name: "OCI-style root with digest as revision",
			root: ChainLink{Kind: "OCIRepository", URL: "oci://ghcr.io/org/chart", Revision: "sha256:deadbeef"},
			want: &GitSourceAnchor{RepoURL: "oci://ghcr.io/org/chart", Revision: "sha256:deadbeef"},
		},
		{
			name: "empty chainlink returns nil",
			root: ChainLink{Kind: "Whatever"},
			want: nil,
		},
		{
			name: "whitespace-only fields return nil",
			root: ChainLink{URL: " ", Revision: " ", Path: " "},
			want: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := anchorFromChainRoot(tc.root)
			if (got == nil) != (tc.want == nil) {
				t.Fatalf("anchorFromChainRoot = %v, want %v", got, tc.want)
			}
			if got == nil {
				return
			}
			if got.RepoURL != tc.want.RepoURL || got.Revision != tc.want.Revision || got.Path != tc.want.Path {
				t.Errorf("anchorFromChainRoot = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestCollectGitSourceAnchor_BestEffortNil(t *testing.T) {
	// CollectGitSourceAnchor goes through real ArgoTracer/FluxTracer which
	// require CLI availability. In a test environment without those CLIs
	// (or without a real cluster), the function must silently return nil
	// rather than panic. That's the contract.
	t.Run("nil resource", func(t *testing.T) {
		if got := CollectGitSourceAnchor(context.Background(), nil); got != nil {
			t.Errorf("expected nil for nil resource; got %+v", got)
		}
	})

	t.Run("native resource without GitOps owner", func(t *testing.T) {
		obj := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "api",
				"namespace": "default",
				// No owner labels → owner type "k8s" or "unknown" → no tracer dispatch
			},
		}}
		if got := CollectGitSourceAnchor(context.Background(), obj); got != nil {
			t.Errorf("expected nil for resource without GitOps owner; got %+v", got)
		}
	})
}
