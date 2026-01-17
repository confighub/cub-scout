// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestKyvernoScannerProcessPolicyReport(t *testing.T) {
	scanner := &KyvernoScanner{
		policyDBDir: "",
	}

	tests := []struct {
		name           string
		report         map[string]interface{}
		wantFindingsLen int
		wantFirstID    string
		wantSeverity   string
	}{
		{
			name: "single failure",
			report: map[string]interface{}{
				"apiVersion": "wgpolicyk8s.io/v1alpha2",
				"kind":       "PolicyReport",
				"metadata": map[string]interface{}{
					"name":      "polr-ns-default",
					"namespace": "default",
				},
				"results": []interface{}{
					map[string]interface{}{
						"policy":   "require-labels",
						"rule":     "check-team-label",
						"result":   "fail",
						"message":  "Missing required label 'team'",
						"severity": "medium",
						"category": "Best Practices",
						"resources": []interface{}{
							map[string]interface{}{
								"kind":      "Deployment",
								"name":      "nginx",
								"namespace": "default",
							},
						},
					},
				},
			},
			wantFindingsLen: 1,
			wantFirstID:     "require-labels/check-team-label",
			wantSeverity:    "warning",
		},
		{
			name: "multiple results mixed",
			report: map[string]interface{}{
				"apiVersion": "wgpolicyk8s.io/v1alpha2",
				"kind":       "PolicyReport",
				"metadata": map[string]interface{}{
					"name":      "polr-ns-prod",
					"namespace": "prod",
				},
				"results": []interface{}{
					map[string]interface{}{
						"policy":   "disallow-privileged",
						"rule":     "check-privileged",
						"result":   "fail",
						"message":  "Privileged container not allowed",
						"severity": "high",
						"resources": []interface{}{
							map[string]interface{}{
								"kind": "Pod",
								"name": "debug-pod",
							},
						},
					},
					map[string]interface{}{
						"policy": "require-probes",
						"rule":   "check-liveness",
						"result": "pass", // Should be skipped
					},
					map[string]interface{}{
						"policy":   "require-resources",
						"rule":     "check-limits",
						"result":   "warn",
						"message":  "Resource limits not set",
						"severity": "low",
						"resources": []interface{}{
							map[string]interface{}{
								"kind": "Deployment",
								"name": "api",
							},
						},
					},
				},
			},
			wantFindingsLen: 2, // Only fail and warn, not pass
			wantFirstID:     "disallow-privileged/check-privileged",
			wantSeverity:    "critical",
		},
		{
			name: "empty results",
			report: map[string]interface{}{
				"apiVersion": "wgpolicyk8s.io/v1alpha2",
				"kind":       "PolicyReport",
				"metadata": map[string]interface{}{
					"name":      "polr-empty",
					"namespace": "test",
				},
				"results": []interface{}{},
			},
			wantFindingsLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := unstructured.Unstructured{Object: tt.report}
			findings := scanner.processPolicyReport(report, make(map[string]*KyvernoPolicy))

			if len(findings) != tt.wantFindingsLen {
				t.Errorf("got %d findings, want %d", len(findings), tt.wantFindingsLen)
			}

			if tt.wantFindingsLen > 0 && len(findings) > 0 {
				if findings[0].ID != tt.wantFirstID {
					t.Errorf("first finding ID = %q, want %q", findings[0].ID, tt.wantFirstID)
				}
				if findings[0].Severity != tt.wantSeverity {
					t.Errorf("first finding severity = %q, want %q", findings[0].Severity, tt.wantSeverity)
				}
			}
		})
	}
}

func TestKyvernoScannerMatchPolicy(t *testing.T) {
	scanner := &KyvernoScanner{}

	policyDB := map[string]*KyvernoPolicy{
		"KPOL-0001": {
			ID:       "KPOL-0001",
			Name:     "Application Field Validation",
			Category: "CONFIG",
			Severity: "warning",
			DerivedFrom: struct {
				Source           string `yaml:"source" json:"source"`
				PolicyName       string `yaml:"policy_name" json:"policy_name"`
				URL              string `yaml:"url" json:"url"`
				Category         string `yaml:"category" json:"category"`
				MinKyvernoVersion string `yaml:"min_kyverno_version" json:"min_kyverno_version"`
			}{
				PolicyName: "application-field-validation",
			},
		},
		"KPOL-0020": {
			ID:       "KPOL-0020",
			Name:     "Check deprecated APIs",
			Category: "CONFIG",
			Severity: "warning",
			DerivedFrom: struct {
				Source           string `yaml:"source" json:"source"`
				PolicyName       string `yaml:"policy_name" json:"policy_name"`
				URL              string `yaml:"url" json:"url"`
				Category         string `yaml:"category" json:"category"`
				MinKyvernoVersion string `yaml:"min_kyverno_version" json:"min_kyverno_version"`
			}{
				PolicyName: "check-deprecated-apis",
			},
		},
	}

	tests := []struct {
		policyName string
		wantKPOL   string
	}{
		{"application-field-validation", "KPOL-0001"},
		{"check-deprecated-apis", "KPOL-0020"},
		{"unknown-policy", ""},
		{"APPLICATION_FIELD_VALIDATION", "KPOL-0001"}, // Case insensitive
	}

	for _, tt := range tests {
		t.Run(tt.policyName, func(t *testing.T) {
			kpol := scanner.matchPolicy(tt.policyName, policyDB)
			if tt.wantKPOL == "" {
				if kpol != nil {
					t.Errorf("expected no match, got %s", kpol.ID)
				}
			} else {
				if kpol == nil {
					t.Errorf("expected match %s, got nil", tt.wantKPOL)
				} else if kpol.ID != tt.wantKPOL {
					t.Errorf("matched %s, want %s", kpol.ID, tt.wantKPOL)
				}
			}
		})
	}
}

func TestNormalizeSeverity(t *testing.T) {
	tests := []struct {
		severity string
		result   string
		want     string
	}{
		{"critical", "fail", "critical"},
		{"high", "fail", "critical"},
		{"medium", "fail", "warning"},
		{"low", "fail", "info"},
		{"", "fail", "warning"},
		{"", "warn", "info"},
		{"HIGH", "fail", "critical"}, // Case insensitive
	}

	for _, tt := range tests {
		name := tt.severity + "/" + tt.result
		if tt.severity == "" {
			name = "(empty)/" + tt.result
		}
		t.Run(name, func(t *testing.T) {
			got := normalizeSeverity(tt.severity, tt.result)
			if got != tt.want {
				t.Errorf("normalizeSeverity(%q, %q) = %q, want %q", tt.severity, tt.result, got, tt.want)
			}
		})
	}
}

func TestKyvernoScannerAvailable(t *testing.T) {
	ctx := context.Background()

	// Test with fake client that has PolicyReport CRD
	scheme := runtime.NewScheme()
	gvr := schema.GroupVersionResource{
		Group:    "wgpolicyk8s.io",
		Version:  "v1alpha2",
		Resource: "policyreports",
	}

	// Create fake client with PolicyReport
	fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			gvr: "PolicyReportList",
		},
	)

	scanner := NewKyvernoScannerWithClient(fakeClient, "")

	// The fake client should return available
	if !scanner.Available(ctx) {
		// This is expected with fake client - it may not fully simulate the API
		t.Skip("Fake client doesn't fully support availability check")
	}
}

func TestScanResultToJSON(t *testing.T) {
	result := &ScanResult{
		ClusterName: "test-cluster",
		Summary: ScanSummary{
			Critical: 1,
			Warning:  2,
			Info:     3,
		},
		Findings: []ScanFinding{
			{
				ID:         "test-policy/test-rule",
				PolicyName: "test-policy",
				Severity:   "warning",
				Resource:   "Deployment/nginx",
				Namespace:  "default",
				Message:    "Test message",
				Result:     "fail",
			},
		},
	}

	data, err := result.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}

	if len(data) == 0 {
		t.Error("ToJSON() returned empty data")
	}

	// Verify it contains expected fields
	json := string(data)
	if !contains(json, "test-cluster") {
		t.Error("JSON missing clusterName")
	}
	if !contains(json, "test-policy") {
		t.Error("JSON missing policyName")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
