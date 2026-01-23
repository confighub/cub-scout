// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package hierarchysvc

import "testing"

func TestExtractClusterName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "unknown",
			input:    "unknown",
			expected: "unknown",
		},
		{
			name:     "AWS EKS ARN",
			input:    "arn:aws:eks:us-west-2:123456789012:cluster/my-cluster",
			expected: "my-cluster",
		},
		{
			name:     "GKE context",
			input:    "gke_my-project_us-central1-a_production",
			expected: "production",
		},
		{
			name:     "kind cluster",
			input:    "kind-my-cluster",
			expected: "my-cluster",
		},
		{
			name:     "plain cluster name",
			input:    "production",
			expected: "production",
		},
		{
			name:     "minikube",
			input:    "minikube",
			expected: "minikube",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractClusterName(tt.input)
			if got != tt.expected {
				t.Errorf("ExtractClusterName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestMatchesCluster(t *testing.T) {
	tests := []struct {
		name           string
		targetCluster  string
		currentCluster string
		expected       bool
	}{
		{
			name:           "exact match",
			targetCluster:  "production",
			currentCluster: "production",
			expected:       true,
		},
		{
			name:           "target empty",
			targetCluster:  "",
			currentCluster: "production",
			expected:       false,
		},
		{
			name:           "current empty",
			targetCluster:  "production",
			currentCluster: "",
			expected:       false,
		},
		{
			name:           "partial match - target contains current",
			targetCluster:  "my-production-cluster",
			currentCluster: "production",
			expected:       true,
		},
		{
			name:           "partial match - current contains target",
			targetCluster:  "prod",
			currentCluster: "production",
			expected:       true,
		},
		{
			name:           "case insensitive match",
			targetCluster:  "Production",
			currentCluster: "production",
			expected:       true,
		},
		{
			name:           "no match",
			targetCluster:  "staging",
			currentCluster: "production",
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesCluster(tt.targetCluster, tt.currentCluster)
			if got != tt.expected {
				t.Errorf("MatchesCluster(%q, %q) = %v, want %v",
					tt.targetCluster, tt.currentCluster, got, tt.expected)
			}
		})
	}
}
