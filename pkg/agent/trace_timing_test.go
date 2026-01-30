// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestExtractTimingFromResource_Kustomization(t *testing.T) {
	// Create a Kustomization resource with timing
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
			"kind":       "Kustomization",
			"metadata": map[string]interface{}{
				"name":      "apps",
				"namespace": "flux-system",
			},
			"status": map[string]interface{}{
				"lastHandledReconcileAt": "2026-01-30T10:00:00Z",
				"lastAttemptedRevision":  "main@sha1:abc123",
			},
		},
	}

	timing := extractTimingFromResource(resource, "Kustomization")
	if timing == nil {
		t.Fatal("Expected timing to be extracted")
	}

	expected := time.Date(2026, 1, 30, 10, 0, 0, 0, time.UTC)
	if !timing.Equal(expected) {
		t.Errorf("Expected %v, got %v", expected, *timing)
	}
}

func TestExtractTimingFromResource_GitRepository(t *testing.T) {
	// Create a GitRepository resource with timing
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "source.toolkit.fluxcd.io/v1",
			"kind":       "GitRepository",
			"metadata": map[string]interface{}{
				"name":      "platform-config",
				"namespace": "flux-system",
			},
			"status": map[string]interface{}{
				"artifact": map[string]interface{}{
					"lastUpdateTime": "2026-01-30T09:30:00Z",
					"revision":       "main@sha1:def456",
				},
			},
		},
	}

	timing := extractTimingFromResource(resource, "GitRepository")
	if timing == nil {
		t.Fatal("Expected timing to be extracted")
	}

	expected := time.Date(2026, 1, 30, 9, 30, 0, 0, time.UTC)
	if !timing.Equal(expected) {
		t.Errorf("Expected %v, got %v", expected, *timing)
	}
}

func TestExtractTimingFromResource_Deployment(t *testing.T) {
	// Create a Deployment resource with conditions
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "backend",
				"namespace": "prod",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":               "Progressing",
						"status":             "True",
						"lastTransitionTime": "2026-01-30T08:00:00Z",
					},
					map[string]interface{}{
						"type":               "Available",
						"status":             "True",
						"lastTransitionTime": "2026-01-30T08:30:00Z",
					},
				},
			},
		},
	}

	timing := extractTimingFromResource(resource, "Deployment")
	if timing == nil {
		t.Fatal("Expected timing to be extracted from Available condition")
	}

	expected := time.Date(2026, 1, 30, 8, 30, 0, 0, time.UTC)
	if !timing.Equal(expected) {
		t.Errorf("Expected %v, got %v", expected, *timing)
	}
}

func TestExtractTimingFromResource_ArgoApplication(t *testing.T) {
	// Create an ArgoCD Application resource with timing
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "guestbook",
				"namespace": "argocd",
			},
			"status": map[string]interface{}{
				"operationState": map[string]interface{}{
					"startedAt":  "2026-01-30T11:00:00Z",
					"finishedAt": "2026-01-30T11:01:00Z",
					"phase":      "Succeeded",
				},
				"reconciledAt": "2026-01-30T11:02:00Z",
			},
		},
	}

	timing := extractTimingFromResource(resource, "Application")
	if timing == nil {
		t.Fatal("Expected timing to be extracted")
	}

	// Should get finishedAt (11:01) as it takes precedence
	expected := time.Date(2026, 1, 30, 11, 1, 0, 0, time.UTC)
	if !timing.Equal(expected) {
		t.Errorf("Expected %v, got %v", expected, *timing)
	}
}

func TestExtractTimingFromResource_NoTiming(t *testing.T) {
	// Create a resource with no timing info
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "my-config",
				"namespace": "default",
			},
		},
	}

	timing := extractTimingFromResource(resource, "ConfigMap")
	if timing != nil {
		t.Errorf("Expected no timing, got %v", *timing)
	}
}

func TestParseTime(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
		wantNil  bool
	}{
		{
			input:    "2026-01-30T10:00:00Z",
			expected: time.Date(2026, 1, 30, 10, 0, 0, 0, time.UTC),
		},
		{
			input:    "2026-01-30T10:00:00.123456789Z",
			expected: time.Date(2026, 1, 30, 10, 0, 0, 123456789, time.UTC),
		},
		{
			input:    "2026-01-30T10:00:00+00:00",
			expected: time.Date(2026, 1, 30, 10, 0, 0, 0, time.UTC),
		},
		{
			input:   "",
			wantNil: true,
		},
		{
			input:   "invalid",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseTime(tt.input)
			if tt.wantNil {
				if result != nil {
					t.Errorf("Expected nil, got %v", *result)
				}
				return
			}
			if result == nil {
				t.Fatalf("Expected non-nil result for %q", tt.input)
			}
			if !result.Equal(tt.expected) {
				t.Errorf("Expected %v, got %v", tt.expected, *result)
			}
		})
	}
}
