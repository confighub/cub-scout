// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"encoding/json"
	"testing"
)

func TestSortFindings(t *testing.T) {
	findings := []DriftFinding{
		NewDriftFinding(
			DriftObjectID{Kind: "Deployment", Namespace: "web", Name: "frontend"},
			"spec.replicas",
			3, 5,
			DriftCapacity,
			DriftSeverityWarning,
		),
		NewDriftFinding(
			DriftObjectID{Kind: "Deployment", Namespace: "api", Name: "backend"},
			"spec.template.spec.containers[0].image",
			"nginx:1.19", "nginx:1.20",
			DriftImage,
			DriftSeverityCritical,
		),
		NewDriftFinding(
			DriftObjectID{Kind: "Deployment", Namespace: "web", Name: "frontend"},
			"spec.template.spec.containers[0].env[0].value",
			"prod", "staging",
			DriftConfig,
			DriftSeverityInfo,
		),
		NewDriftFinding(
			DriftObjectID{Kind: "Deployment", Namespace: "api", Name: "backend"},
			"spec.replicas",
			2, 3,
			DriftCapacity,
			DriftSeverityCritical,
		),
	}

	SortFindings(findings)

	// Expected order:
	// 1. api/backend:replicas (critical, api < web alphabetically)
	// 2. api/backend:image (critical, same object, image path > replicas path)
	// 3. web/frontend:replicas (warning)
	// 4. web/frontend:env (info)

	expected := []struct {
		objectID string
		path     string
		severity DriftSeverity
	}{
		{"Deployment:api/backend", "spec.replicas", DriftSeverityCritical},
		{"Deployment:api/backend", "spec.template.spec.containers[0].image", DriftSeverityCritical},
		{"Deployment:web/frontend", "spec.replicas", DriftSeverityWarning},
		{"Deployment:web/frontend", "spec.template.spec.containers[0].env[0].value", DriftSeverityInfo},
	}

	if len(findings) != len(expected) {
		t.Fatalf("expected %d findings, got %d", len(expected), len(findings))
	}

	for i, exp := range expected {
		if findings[i].ObjectID != exp.objectID {
			t.Errorf("findings[%d].ObjectID = %q, want %q", i, findings[i].ObjectID, exp.objectID)
		}
		if findings[i].Path != exp.path {
			t.Errorf("findings[%d].Path = %q, want %q", i, findings[i].Path, exp.path)
		}
		if findings[i].Severity != exp.severity {
			t.Errorf("findings[%d].Severity = %q, want %q", i, findings[i].Severity, exp.severity)
		}
	}
}

func TestBuildDriftSummary(t *testing.T) {
	findings := []DriftFinding{
		NewDriftFinding(
			DriftObjectID{Kind: "Deployment", Namespace: "web", Name: "frontend"},
			"spec.replicas",
			3, 5,
			DriftCapacity,
			DriftSeverityWarning,
		),
		NewDriftFinding(
			DriftObjectID{Kind: "Deployment", Namespace: "api", Name: "backend"},
			"spec.template.spec.containers[0].image",
			"nginx:1.19", "nginx:1.20",
			DriftImage,
			DriftSeverityCritical,
		),
		NewDriftFinding(
			DriftObjectID{Kind: "Deployment", Namespace: "web", Name: "frontend"},
			"spec.template.spec.containers[0].env[0].value",
			"prod", "staging",
			DriftConfig,
			DriftSeverityWarning,
		),
	}

	summary := BuildDriftSummary(findings)

	if summary.TotalFindings != 3 {
		t.Errorf("TotalFindings = %d, want 3", summary.TotalFindings)
	}

	if summary.AffectedObjects != 2 {
		t.Errorf("AffectedObjects = %d, want 2", summary.AffectedObjects)
	}

	// Check severity counts in canonical order
	if len(summary.BySeverity) != 2 {
		t.Fatalf("BySeverity length = %d, want 2", len(summary.BySeverity))
	}
	// First should be critical (canonical order)
	if summary.BySeverity[0].Severity != DriftSeverityCritical || summary.BySeverity[0].Count != 1 {
		t.Errorf("BySeverity[0] = %+v, want critical:1", summary.BySeverity[0])
	}
	if summary.BySeverity[1].Severity != DriftSeverityWarning || summary.BySeverity[1].Count != 2 {
		t.Errorf("BySeverity[1] = %+v, want warning:2", summary.BySeverity[1])
	}

	// Check classification counts in canonical order
	if len(summary.ByClassification) != 3 {
		t.Fatalf("ByClassification length = %d, want 3", len(summary.ByClassification))
	}
	// First should be capacity (canonical order)
	if summary.ByClassification[0].Classification != DriftCapacity || summary.ByClassification[0].Count != 1 {
		t.Errorf("ByClassification[0] = %+v, want capacity:1", summary.ByClassification[0])
	}
	if summary.ByClassification[1].Classification != DriftImage || summary.ByClassification[1].Count != 1 {
		t.Errorf("ByClassification[1] = %+v, want image:1", summary.ByClassification[1])
	}
	if summary.ByClassification[2].Classification != DriftConfig || summary.ByClassification[2].Count != 1 {
		t.Errorf("ByClassification[2] = %+v, want config:1", summary.ByClassification[2])
	}
}

func TestNewDriftFinding_IDGeneration(t *testing.T) {
	tests := []struct {
		name     string
		obj      DriftObjectID
		path     string
		wantID   string
		wantObjID string
	}{
		{
			name:      "namespaced resource",
			obj:       DriftObjectID{Kind: "Deployment", Namespace: "prod", Name: "web"},
			path:      "spec.replicas",
			wantID:    "drift:Deployment:prod/web:spec.replicas",
			wantObjID: "Deployment:prod/web",
		},
		{
			name:      "cluster-scoped resource",
			obj:       DriftObjectID{Kind: "ClusterRole", Name: "admin"},
			path:      "rules[0].verbs",
			wantID:    "drift:ClusterRole:admin:rules[0].verbs",
			wantObjID: "ClusterRole:admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewDriftFinding(tt.obj, tt.path, nil, nil, DriftOther, DriftSeverityInfo)
			if f.ID != tt.wantID {
				t.Errorf("ID = %q, want %q", f.ID, tt.wantID)
			}
			if f.ObjectID != tt.wantObjID {
				t.Errorf("ObjectID = %q, want %q", f.ObjectID, tt.wantObjID)
			}
		})
	}
}

func TestDriftReport_JSONDeterminism(t *testing.T) {
	// Create a report and serialize it twice to verify determinism
	namespace := "production"
	report := DriftReport{
		Command: "drift",
		Context: DriftContext{
			Cluster:   "prod-east",
			Namespace: &namespace,
			DesiredSource: DriftSource{
				Type: "git",
				Ref:  "abc123",
				URL:  "https://github.com/acme/infra",
			},
			LiveSource: DriftSource{
				Type: "cluster",
				Ref:  "prod-east",
			},
		},
		Findings: []DriftFinding{
			NewDriftFinding(
				DriftObjectID{Kind: "Deployment", Namespace: "production", Name: "api"},
				"spec.replicas",
				3, 5,
				DriftCapacity,
				DriftSeverityWarning,
			),
		},
	}
	report.Summary = BuildDriftSummary(report.Findings)

	// Serialize twice
	json1, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("first marshal: %v", err)
	}

	json2, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("second marshal: %v", err)
	}

	if string(json1) != string(json2) {
		t.Errorf("JSON serialization is not deterministic")
	}
}

func TestDriftReport_EmptyFindings(t *testing.T) {
	// Ensure empty findings produces valid JSON
	report := DriftReport{
		Command: "drift",
		Context: DriftContext{
			Cluster: "test",
			DesiredSource: DriftSource{Type: "file", Ref: "test.yaml"},
			LiveSource:    DriftSource{Type: "cluster", Ref: "test"},
		},
		Findings: []DriftFinding{},
	}
	report.Summary = BuildDriftSummary(report.Findings)

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Verify it's valid JSON and has expected structure
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed["command"] != "drift" {
		t.Errorf("command = %v, want drift", parsed["command"])
	}

	findings, ok := parsed["findings"].([]interface{})
	if !ok {
		t.Fatalf("findings is not an array")
	}
	if len(findings) != 0 {
		t.Errorf("findings length = %d, want 0", len(findings))
	}
}

func TestDriftReport_JSONRoundTrip(t *testing.T) {
	// Test that marshal/unmarshal preserves all fields
	namespace := "production"
	original := DriftReport{
		Command: "drift",
		Context: DriftContext{
			Cluster:   "prod-east",
			Namespace: &namespace,
			DesiredSource: DriftSource{
				Type:    "git",
				Ref:     "abc123def456",
				URL:     "https://github.com/acme/infra",
				Subpath: "clusters/prod",
			},
			LiveSource: DriftSource{
				Type: "cluster",
				Ref:  "prod-east",
			},
		},
		Findings: []DriftFinding{
			NewDriftFinding(
				DriftObjectID{Kind: "Deployment", Namespace: "production", Name: "api"},
				"spec.replicas",
				3, 5,
				DriftCapacity,
				DriftSeverityWarning,
			),
			NewDriftFinding(
				DriftObjectID{Kind: "Deployment", Namespace: "production", Name: "web"},
				"spec.template.spec.containers[0].image",
				"nginx:1.19", "nginx:1.20",
				DriftImage,
				DriftSeverityCritical,
			),
		},
	}
	original.Summary = BuildDriftSummary(original.Findings)

	// Marshal
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Unmarshal into new struct
	var restored DriftReport
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Verify key fields preserved
	if restored.Command != original.Command {
		t.Errorf("Command = %q, want %q", restored.Command, original.Command)
	}
	if restored.Context.Cluster != original.Context.Cluster {
		t.Errorf("Context.Cluster = %q, want %q", restored.Context.Cluster, original.Context.Cluster)
	}
	if *restored.Context.Namespace != *original.Context.Namespace {
		t.Errorf("Context.Namespace = %q, want %q", *restored.Context.Namespace, *original.Context.Namespace)
	}
	if len(restored.Findings) != len(original.Findings) {
		t.Fatalf("Findings length = %d, want %d", len(restored.Findings), len(original.Findings))
	}
	if restored.Findings[0].ID != original.Findings[0].ID {
		t.Errorf("Findings[0].ID = %q, want %q", restored.Findings[0].ID, original.Findings[0].ID)
	}
	if restored.Summary.TotalFindings != original.Summary.TotalFindings {
		t.Errorf("Summary.TotalFindings = %d, want %d", restored.Summary.TotalFindings, original.Summary.TotalFindings)
	}
}

func TestDriftFinding_RequiredFieldsPresent(t *testing.T) {
	// Contract: R1 - all structural facts must be in JSON
	// This test ensures classification and severity are never empty
	finding := NewDriftFinding(
		DriftObjectID{Kind: "Deployment", Namespace: "prod", Name: "web"},
		"spec.replicas",
		3, 5,
		DriftCapacity,
		DriftSeverityWarning,
	)

	// All required fields must be non-empty
	if finding.ID == "" {
		t.Error("ID is empty")
	}
	if finding.ObjectID == "" {
		t.Error("ObjectID is empty")
	}
	if finding.Path == "" {
		t.Error("Path is empty")
	}
	if finding.Classification == "" {
		t.Error("Classification is empty")
	}
	if finding.Severity == "" {
		t.Error("Severity is empty")
	}

	// Verify they serialize to JSON (not omitted)
	data, err := json.Marshal(finding)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	requiredFields := []string{"id", "object_id", "object", "path", "desired", "live", "classification", "severity"}
	for _, field := range requiredFields {
		if _, ok := parsed[field]; !ok {
			t.Errorf("required field %q missing from JSON", field)
		}
	}
}

func TestSortFindings_StableOrdering(t *testing.T) {
	// Contract: R6 - ordering is semantic (by severity field), not narrative
	// Same input in any order should produce identical output

	makeFindings := func() []DriftFinding {
		return []DriftFinding{
			NewDriftFinding(DriftObjectID{Kind: "Deployment", Namespace: "z", Name: "last"}, "spec.replicas", 1, 2, DriftCapacity, DriftSeverityInfo),
			NewDriftFinding(DriftObjectID{Kind: "Deployment", Namespace: "a", Name: "first"}, "spec.replicas", 1, 2, DriftCapacity, DriftSeverityCritical),
			NewDriftFinding(DriftObjectID{Kind: "Deployment", Namespace: "m", Name: "middle"}, "spec.replicas", 1, 2, DriftCapacity, DriftSeverityWarning),
		}
	}

	// Sort two copies made with same data
	findings1 := makeFindings()
	findings2 := makeFindings()

	// Reverse findings2 before sorting
	for i, j := 0, len(findings2)-1; i < j; i, j = i+1, j-1 {
		findings2[i], findings2[j] = findings2[j], findings2[i]
	}

	SortFindings(findings1)
	SortFindings(findings2)

	// Both should now be identical
	for i := range findings1 {
		if findings1[i].ID != findings2[i].ID {
			t.Errorf("position %d: findings1.ID=%q, findings2.ID=%q", i, findings1[i].ID, findings2[i].ID)
		}
	}

	// Verify order is critical > warning > info
	if findings1[0].Severity != DriftSeverityCritical {
		t.Errorf("first finding should be critical, got %s", findings1[0].Severity)
	}
	if findings1[1].Severity != DriftSeverityWarning {
		t.Errorf("second finding should be warning, got %s", findings1[1].Severity)
	}
	if findings1[2].Severity != DriftSeverityInfo {
		t.Errorf("third finding should be info, got %s", findings1[2].Severity)
	}
}
