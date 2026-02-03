// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"
)

func TestApplyBackend_String(t *testing.T) {
	tests := []struct {
		backend ApplyBackend
		want    string
	}{
		{BackendFlux, "flux"},
		{BackendArgoCD, "argocd"},
		{BackendWorker, "worker"},
		{BackendNone, "none"},
	}

	for _, tt := range tests {
		if string(tt.backend) != tt.want {
			t.Errorf("ApplyBackend = %v, want %v", tt.backend, tt.want)
		}
	}
}

func TestTransportMode_String(t *testing.T) {
	tests := []struct {
		transport TransportMode
		want      string
	}{
		{TransportOCI, "oci"},
		{TransportGit, "git"},
		{TransportHelm, "helm"},
		{TransportUnknown, "unknown"},
	}

	for _, tt := range tests {
		if string(tt.transport) != tt.want {
			t.Errorf("TransportMode = %v, want %v", tt.transport, tt.want)
		}
	}
}

func TestIsOCIURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"oci://ghcr.io/acme/manifests", true},
		{"oci://registry.example.com/charts/app", true},
		{"https://github.com/acme/repo", false},
		{"git@github.com:acme/repo.git", false},
		{"", false},
		{"oci:/", false},
	}

	for _, tt := range tests {
		if got := isOCIURL(tt.url); got != tt.want {
			t.Errorf("isOCIURL(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestIsHelmURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://charts.bitnami.com/bitnami", true},
		{"https://kubernetes-charts.storage.googleapis.com", false},
		{"https://chartmuseum.example.com", true},
		{"https://helm.example.com/repo", true},
		{"https://github.com/acme/repo", false},
		{"oci://ghcr.io/acme/charts", false},
	}

	for _, tt := range tests {
		if got := isHelmURL(tt.url); got != tt.want {
			t.Errorf("isHelmURL(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestApplyBackendDetector_InferTransport(t *testing.T) {
	detector := &ApplyBackendDetector{}

	tests := []struct {
		name    string
		sources []SourceRef
		want    TransportMode
	}{
		{
			name:    "empty sources",
			sources: []SourceRef{},
			want:    TransportUnknown,
		},
		{
			name: "OCIRepository",
			sources: []SourceRef{
				{Kind: "OCIRepository", Name: "test", Namespace: "default"},
			},
			want: TransportOCI,
		},
		{
			name: "GitRepository",
			sources: []SourceRef{
				{Kind: "GitRepository", Name: "test", Namespace: "default"},
			},
			want: TransportGit,
		},
		{
			name: "HelmRepository",
			sources: []SourceRef{
				{Kind: "HelmRepository", Name: "test", Namespace: "default"},
			},
			want: TransportHelm,
		},
		{
			name: "mixed - first wins",
			sources: []SourceRef{
				{Kind: "OCIRepository", Name: "test1", Namespace: "default"},
				{Kind: "GitRepository", Name: "test2", Namespace: "default"},
			},
			want: TransportOCI,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detector.inferTransport(tt.sources)
			if got != tt.want {
				t.Errorf("inferTransport() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyBackendInfo_HasOCISources(t *testing.T) {
	tests := []struct {
		name string
		info ApplyBackendInfo
		want bool
	}{
		{
			name: "no sources",
			info: ApplyBackendInfo{
				Backend:   BackendFlux,
				Transport: TransportUnknown,
			},
			want: false,
		},
		{
			name: "OCI transport",
			info: ApplyBackendInfo{
				Backend:   BackendFlux,
				Transport: TransportOCI,
				Sources: []SourceRef{
					{Kind: "OCIRepository", Name: "test", Namespace: "default"},
				},
			},
			want: true,
		},
		{
			name: "Git transport",
			info: ApplyBackendInfo{
				Backend:   BackendFlux,
				Transport: TransportGit,
				Sources: []SourceRef{
					{Kind: "GitRepository", Name: "test", Namespace: "default"},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.info.Transport == TransportOCI
			if got != tt.want {
				t.Errorf("HasOCISources = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsCRDNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isCRDNotFound(tt.err)
			if got != tt.want {
				t.Errorf("isCRDNotFound() = %v, want %v", got, tt.want)
			}
		})
	}
}
