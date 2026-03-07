// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import "testing"

func TestViewOptions_HasFilters(t *testing.T) {
	tests := []struct {
		name string
		opts ViewOptions
		want bool
	}{
		{"empty", ViewOptions{}, false},
		{"owner only", ViewOptions{Owner: "Flux"}, true},
		{"namespace only", ViewOptions{Namespace: "default"}, true},
		{"depth only", ViewOptions{Depth: 2}, true},
		{"kind only", ViewOptions{Kind: "Deployment"}, true},
		{"multiple", ViewOptions{Owner: "Flux", Depth: 3}, true},
		{"depth zero", ViewOptions{Depth: 0}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.opts.HasFilters()
			if got != tt.want {
				t.Errorf("HasFilters() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestViewOptions_String(t *testing.T) {
	tests := []struct {
		name string
		opts ViewOptions
		want string
	}{
		{"empty", ViewOptions{}, ""},
		{"owner", ViewOptions{Owner: "Flux"}, "owner=Flux"},
		{"namespace", ViewOptions{Namespace: "prod"}, "ns=prod"},
		{"depth", ViewOptions{Depth: 3}, "depth=3"},
		{"kind", ViewOptions{Kind: "StatefulSet"}, "kind=StatefulSet"},
		{"combo", ViewOptions{Owner: "ArgoCD", Depth: 2}, "owner=ArgoCD depth=2"},
		{"all fields", ViewOptions{Owner: "Helm", Namespace: "db", Depth: 1, Kind: "Deployment"},
			"owner=Helm ns=db depth=1 kind=Deployment"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.opts.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestViewOptions_FilterStripText(t *testing.T) {
	tests := []struct {
		name string
		opts ViewOptions
		want string
	}{
		{"empty", ViewOptions{}, ""},
		{"with filters", ViewOptions{Owner: "Flux", Depth: 2}, "Filters: owner=Flux depth=2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.opts.FilterStripText()
			if got != tt.want {
				t.Errorf("FilterStripText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateOwner(t *testing.T) {
	tests := []struct {
		name    string
		owner   string
		wantErr bool
	}{
		{"empty", "", false},
		{"valid Flux", "Flux", false},
		{"valid flux lowercase", "flux", false},
		{"valid ArgoCD", "ArgoCD", false},
		{"valid argocd lowercase", "argocd", false},
		{"valid Helm", "Helm", false},
		{"valid ConfigHub", "ConfigHub", false},
		{"valid Crossplane", "Crossplane", false},
		{"valid kro lowercase", "kro", false},
		{"valid Terraform", "Terraform", false},
		{"valid Native", "Native", false},
		{"invalid", "InvalidOwner", true},
		{"invalid typo", "Flx", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOwner(tt.owner)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateOwner(%q) error = %v, wantErr %v", tt.owner, err, tt.wantErr)
			}
		})
	}
}

func TestNormalizeOwner(t *testing.T) {
	tests := []struct {
		name  string
		owner string
		want  string
	}{
		{"empty", "", ""},
		{"canonical", "Flux", "Flux"},
		{"lowercase", "flux", "Flux"},
		{"mixed case argocd", "ARGOCD", "ArgoCD"},
		{"kro", "KRO", "kro"},
		{"unknown", "Unknown", "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeOwner(tt.owner)
			if got != tt.want {
				t.Errorf("NormalizeOwner(%q) = %q, want %q", tt.owner, got, tt.want)
			}
		})
	}
}
