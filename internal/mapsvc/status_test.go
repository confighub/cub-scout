// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package mapsvc

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestDetectStatus(t *testing.T) {
	tests := []struct {
		name     string
		obj      *unstructured.Unstructured
		expected string
	}{
		{
			name: "Ready condition True",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind": "Kustomization",
					"status": map[string]interface{}{
						"conditions": []interface{}{
							map[string]interface{}{
								"type":   "Ready",
								"status": "True",
							},
						},
					},
				},
			},
			expected: StatusReady,
		},
		{
			name: "Ready condition False",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind": "Kustomization",
					"status": map[string]interface{}{
						"conditions": []interface{}{
							map[string]interface{}{
								"type":   "Ready",
								"status": "False",
							},
						},
					},
				},
			},
			expected: StatusNotReady,
		},
		{
			name: "No status",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind": "Pod",
				},
			},
			expected: StatusUnknown,
		},
		{
			name: "Pod Running phase",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind": "Pod",
					"status": map[string]interface{}{
						"phase": "Running",
					},
				},
			},
			expected: StatusReady,
		},
		{
			name: "Pod Pending phase",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind": "Pod",
					"status": map[string]interface{}{
						"phase": "Pending",
					},
				},
			},
			expected: StatusPending,
		},
		{
			name: "Pod Failed phase",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind": "Pod",
					"status": map[string]interface{}{
						"phase": "Failed",
					},
				},
			},
			expected: StatusFailed,
		},
		{
			name: "Argo CD Application Healthy and Synced",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind": "Application",
					"status": map[string]interface{}{
						"health": map[string]interface{}{
							"status": "Healthy",
						},
						"sync": map[string]interface{}{
							"status": "Synced",
						},
					},
				},
			},
			expected: StatusReady,
		},
		{
			name: "Argo CD Application Degraded",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind": "Application",
					"status": map[string]interface{}{
						"health": map[string]interface{}{
							"status": "Degraded",
						},
						"sync": map[string]interface{}{
							"status": "Synced",
						},
					},
				},
			},
			expected: StatusFailed,
		},
		{
			name: "Argo CD Application OutOfSync",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind": "Application",
					"status": map[string]interface{}{
						"health": map[string]interface{}{
							"status": "Healthy",
						},
						"sync": map[string]interface{}{
							"status": "OutOfSync",
						},
					},
				},
			},
			expected: StatusNotReady,
		},
		{
			name: "Deployment ready",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind": "Deployment",
					"spec": map[string]interface{}{
						"replicas": int64(3),
					},
					"status": map[string]interface{}{
						"replicas":          int64(3),
						"readyReplicas":     int64(3),
						"updatedReplicas":   int64(3),
						"availableReplicas": int64(3),
					},
				},
			},
			expected: StatusReady,
		},
		{
			name: "Deployment not ready",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind": "Deployment",
					"spec": map[string]interface{}{
						"replicas": int64(3),
					},
					"status": map[string]interface{}{
						"replicas":          int64(3),
						"readyReplicas":     int64(1),
						"updatedReplicas":   int64(3),
						"availableReplicas": int64(1),
					},
				},
			},
			expected: StatusNotReady,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectStatus(tt.obj)
			if got != tt.expected {
				t.Errorf("DetectStatus() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDisplayOwner(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"flux", "Flux"},
		{"Flux", "Flux"},
		{"argo", "ArgoCD"},
		{"Argo", "ArgoCD"},
		{"helm", "Helm"},
		{"Helm", "Helm"},
		{"confighub", "ConfigHub"},
		{"ConfigHub", "ConfigHub"},
		{"k8s", "Native"},
		{"native", "Native"},
		{"unknown", "Native"},
		{"", "Native"},
		{"custom", "custom"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := DisplayOwner(tt.input)
			if got != tt.expected {
				t.Errorf("DisplayOwner(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestEntryGetField(t *testing.T) {
	entry := Entry{
		ID:          "test-id",
		ClusterName: "prod",
		Namespace:   "default",
		Kind:        "Deployment",
		Name:        "nginx",
		Owner:       "helm",
		Status:      StatusReady,
		Labels: map[string]string{
			"app": "nginx",
			"env": "prod",
		},
	}

	tests := []struct {
		field    string
		expected string
		found    bool
	}{
		{"kind", "Deployment", true},
		{"namespace", "default", true},
		{"name", "nginx", true},
		{"owner", "helm", true},
		{"status", "Ready", true},
		{"cluster", "prod", true},
		{"clusterName", "prod", true},
		{"labels[app]", "nginx", true},
		{"labels[env]", "prod", true},
		{"labels[missing]", "", false},
		{"invalid", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			got, found := entry.GetField(tt.field)
			if got != tt.expected || found != tt.found {
				t.Errorf("GetField(%q) = (%q, %v), want (%q, %v)", tt.field, got, found, tt.expected, tt.found)
			}
		})
	}
}

func TestOwnerStats(t *testing.T) {
	stats := NewOwnerStats()

	entries := []Entry{
		{Owner: "flux", Kind: "Deployment", Status: StatusReady},
		{Owner: "flux", Kind: "Service", Status: StatusReady},
		{Owner: "helm", Kind: "Deployment", Status: StatusNotReady},
	}

	for _, e := range entries {
		stats.Add(e)
	}

	if stats.Total != 3 {
		t.Errorf("Total = %d, want 3", stats.Total)
	}
	if stats.ByOwner["flux"] != 2 {
		t.Errorf("ByOwner[flux] = %d, want 2", stats.ByOwner["flux"])
	}
	if stats.ByOwner["helm"] != 1 {
		t.Errorf("ByOwner[helm] = %d, want 1", stats.ByOwner["helm"])
	}
	if stats.ByKind["Deployment"] != 2 {
		t.Errorf("ByKind[Deployment] = %d, want 2", stats.ByKind["Deployment"])
	}
	if stats.ByStatus[StatusReady] != 2 {
		t.Errorf("ByStatus[Ready] = %d, want 2", stats.ByStatus[StatusReady])
	}
}
