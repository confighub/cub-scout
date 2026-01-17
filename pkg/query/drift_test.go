// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package query

import (
	"testing"
)

func TestDriftDetector_compare(t *testing.T) {
	dd := &DriftDetector{}

	tests := []struct {
		name       string
		declared   interface{}
		live       interface{}
		wantDrift  bool
	}{
		{
			name:      "identical strings",
			declared:  "hello",
			live:      "hello",
			wantDrift: false,
		},
		{
			name:      "different strings",
			declared:  "hello",
			live:      "world",
			wantDrift: true,
		},
		{
			name:      "identical numbers",
			declared:  float64(42),
			live:      float64(42),
			wantDrift: false,
		},
		{
			name:      "different numbers",
			declared:  float64(42),
			live:      float64(43),
			wantDrift: true,
		},
		{
			name: "identical maps",
			declared: map[string]interface{}{
				"key": "value",
			},
			live: map[string]interface{}{
				"key": "value",
			},
			wantDrift: false,
		},
		{
			name: "different map values",
			declared: map[string]interface{}{
				"key": "value1",
			},
			live: map[string]interface{}{
				"key": "value2",
			},
			wantDrift: true,
		},
		{
			name: "extra key in live",
			declared: map[string]interface{}{
				"key": "value",
			},
			live: map[string]interface{}{
				"key":   "value",
				"extra": "added",
			},
			wantDrift: true,
		},
		{
			name:      "nil declared",
			declared:  nil,
			live:      "something",
			wantDrift: true,
		},
		{
			name:      "both nil",
			declared:  nil,
			live:      nil,
			wantDrift: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changes := dd.compare("test", tt.declared, tt.live)
			hasDrift := len(changes) > 0

			if hasDrift != tt.wantDrift {
				t.Errorf("compare() hasDrift = %v, want %v", hasDrift, tt.wantDrift)
			}
		})
	}
}

func TestDriftDetector_shouldIgnore(t *testing.T) {
	dd := &DriftDetector{}

	tests := []struct {
		path   string
		ignore bool
	}{
		{"metadata.resourceVersion", true},
		{"metadata.uid", true},
		{"metadata.generation", true},
		{"metadata.creationTimestamp", true},
		{"metadata.managedFields", true},
		{"metadata.managedFields[0]", true},
		{"status", true},
		{"status.replicas", true},
		{"spec.replicas", false},
		{"spec.template.spec.containers", false},
		{"metadata.name", false},
		{"metadata.labels", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := dd.shouldIgnore(tt.path); got != tt.ignore {
				t.Errorf("shouldIgnore(%q) = %v, want %v", tt.path, got, tt.ignore)
			}
		})
	}
}

func TestFormatDrift(t *testing.T) {
	// Test empty drift
	output := FormatDrift(nil)
	if output != "No drift detected" {
		t.Errorf("FormatDrift(nil) = %q, want %q", output, "No drift detected")
	}

	// Test with drift
	drifted := []DriftedResource{
		{
			Resource: ResourceID{
				Kind:      "Deployment",
				Name:      "nginx",
				Namespace: "default",
			},
			Changes: []DriftChange{
				{
					Path:     "spec.replicas",
					Declared: float64(3),
					Live:     float64(5),
				},
			},
		},
	}

	output = FormatDrift(drifted)
	if output == "" {
		t.Error("FormatDrift returned empty for drifted resources")
	}
	if !containsString(output, "nginx") {
		t.Error("FormatDrift should contain resource name")
	}
	if !containsString(output, "spec.replicas") {
		t.Error("FormatDrift should contain changed path")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
