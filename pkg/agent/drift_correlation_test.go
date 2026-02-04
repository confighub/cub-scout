// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"
)

func TestFindingsForObject(t *testing.T) {
	findings := []DriftFinding{
		{ObjectID: "Deployment:prod/api", Path: "spec.replicas"},
		{ObjectID: "Deployment:prod/api", Path: "spec.template.spec.containers[name=app].env[name=LOG_LEVEL]"},
		{ObjectID: "Deployment:prod/web", Path: "spec.replicas"},
		{ObjectID: "StatefulSet:prod/db", Path: "spec.replicas"},
	}

	tests := []struct {
		name     string
		objectID string
		want     int
	}{
		{"finds all for api", "Deployment:prod/api", 2},
		{"finds all for web", "Deployment:prod/web", 1},
		{"finds all for db", "StatefulSet:prod/db", 1},
		{"finds none for unknown", "Deployment:prod/unknown", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindingsForObject(findings, tt.objectID)
			if len(got) != tt.want {
				t.Errorf("FindingsForObject() returned %d findings, want %d", len(got), tt.want)
			}
		})
	}
}

func TestFindingsForObjectID(t *testing.T) {
	findings := []DriftFinding{
		{ObjectID: "Deployment:prod/api", Object: DriftObjectID{Kind: "Deployment", Namespace: "prod", Name: "api"}},
		{ObjectID: "Deployment:prod/web", Object: DriftObjectID{Kind: "Deployment", Namespace: "prod", Name: "web"}},
	}

	got := FindingsForObjectID(findings, "Deployment", "prod", "api")
	if len(got) != 1 {
		t.Errorf("FindingsForObjectID() returned %d findings, want 1", len(got))
	}
}

func TestEventsForObject(t *testing.T) {
	events := []TimelineEvent{
		{ResourceKind: "Deployment", ResourceName: "api"},
		{ResourceKind: "Deployment", ResourceName: "api"},
		{ResourceKind: "Pod", ResourceName: "api-xyz"},
		{ResourceKind: "Deployment", ResourceName: "web"},
	}

	tests := []struct {
		name string
		kind string
		rname string
		want int
	}{
		{"finds deployment events", "Deployment", "api", 2},
		{"finds pod events", "Pod", "api-xyz", 1},
		{"finds web events", "Deployment", "web", 1},
		{"finds none for unknown", "Deployment", "unknown", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EventsForObject(events, tt.kind, tt.rname)
			if len(got) != tt.want {
				t.Errorf("EventsForObject() returned %d events, want %d", len(got), tt.want)
			}
		})
	}
}

func TestEventsForFindings(t *testing.T) {
	findings := []DriftFinding{
		{ObjectID: "Deployment:prod/api", Object: DriftObjectID{Kind: "Deployment", Namespace: "prod", Name: "api"}},
	}

	events := []TimelineEvent{
		{ResourceKind: "Deployment", ResourceName: "api"},
		{ResourceKind: "Pod", ResourceName: "api-xyz"},
		{ResourceKind: "Deployment", ResourceName: "web"},
	}

	got := EventsForFindings(findings, events)
	if len(got) != 1 {
		t.Errorf("EventsForFindings() returned %d events, want 1", len(got))
	}
	if got[0].ResourceName != "api" {
		t.Errorf("EventsForFindings() returned wrong event: %s", got[0].ResourceName)
	}
}

func TestLogsForObject(t *testing.T) {
	logs := []ContainerLogResult{
		{Namespace: "prod", PodName: "api-abc", ContainerName: "app"},
		{Namespace: "prod", PodName: "api-xyz", ContainerName: "app"},
		{Namespace: "prod", PodName: "web-123", ContainerName: "app"},
		{Namespace: "staging", PodName: "api-abc", ContainerName: "app"},
	}

	got := LogsForObject(logs, "prod", []string{"api-abc", "api-xyz"})
	if len(got) != 2 {
		t.Errorf("LogsForObject() returned %d logs, want 2", len(got))
	}
}

func TestLogsForFindings(t *testing.T) {
	findings := []DriftFinding{
		{ObjectID: "Deployment:prod/api", Object: DriftObjectID{Kind: "Deployment", Namespace: "prod", Name: "api"}},
	}

	logs := []ContainerLogResult{
		{Namespace: "prod", PodName: "api-abc", ContainerName: "app"},
		{Namespace: "prod", PodName: "web-123", ContainerName: "app"},
	}

	objectToPods := map[string][]string{
		"Deployment:prod/api": {"api-abc"},
		"Deployment:prod/web": {"web-123"},
	}

	got := LogsForFindings(findings, logs, objectToPods)
	if len(got) != 1 {
		t.Errorf("LogsForFindings() returned %d logs, want 1", len(got))
	}
}

func TestObjectsWithDrift(t *testing.T) {
	findings := []DriftFinding{
		{ObjectID: "Deployment:prod/api"},
		{ObjectID: "Deployment:prod/api"}, // duplicate
		{ObjectID: "Deployment:prod/web"},
	}

	got := ObjectsWithDrift(findings)
	if len(got) != 2 {
		t.Errorf("ObjectsWithDrift() returned %d objects, want 2", len(got))
	}
}

func TestObjectsWithCriticalDrift(t *testing.T) {
	findings := []DriftFinding{
		{ObjectID: "Deployment:prod/api", Severity: DriftSeverityCritical},
		{ObjectID: "Deployment:prod/api", Severity: DriftSeverityWarning}, // same object
		{ObjectID: "Deployment:prod/web", Severity: DriftSeverityWarning},
		{ObjectID: "Deployment:prod/db", Severity: DriftSeverityCritical},
	}

	got := ObjectsWithCriticalDrift(findings)
	if len(got) != 2 {
		t.Errorf("ObjectsWithCriticalDrift() returned %d objects, want 2", len(got))
	}
}

func TestCorrelateAll(t *testing.T) {
	findings := []DriftFinding{
		{ObjectID: "Deployment:prod/api", Object: DriftObjectID{Kind: "Deployment", Namespace: "prod", Name: "api"}},
	}

	events := []TimelineEvent{
		{ResourceKind: "Deployment", ResourceName: "api", K8sEvent: K8sEvent{Type: "Warning", Reason: "FailedScheduling"}},
	}

	logs := []ContainerLogResult{
		{Namespace: "prod", PodName: "api-abc", ContainerName: "app", Error: "connection refused"},
	}

	podNames := []string{"api-abc"}

	corr := CorrelateAll("Deployment:prod/api", "Deployment", "prod", "api", findings, events, logs, podNames)

	if corr.ObjectID != "Deployment:prod/api" {
		t.Errorf("ObjectID = %s, want Deployment:prod/api", corr.ObjectID)
	}
	if !corr.HasDrift {
		t.Error("HasDrift should be true")
	}
	if !corr.HasFailure {
		t.Error("HasFailure should be true (Warning event + log error)")
	}
	if len(corr.Findings) != 1 {
		t.Errorf("Findings count = %d, want 1", len(corr.Findings))
	}
	if len(corr.Events) != 1 {
		t.Errorf("Events count = %d, want 1", len(corr.Events))
	}
	if len(corr.Logs) != 1 {
		t.Errorf("Logs count = %d, want 1", len(corr.Logs))
	}
}

func TestCorrelateAll_NoDrift(t *testing.T) {
	findings := []DriftFinding{
		{ObjectID: "Deployment:prod/web"}, // different object
	}

	events := []TimelineEvent{
		{ResourceKind: "Deployment", ResourceName: "api", K8sEvent: K8sEvent{Type: "Warning"}},
	}

	logs := []ContainerLogResult{}
	podNames := []string{"api-abc"}

	corr := CorrelateAll("Deployment:prod/api", "Deployment", "prod", "api", findings, events, logs, podNames)

	if corr.HasDrift {
		t.Error("HasDrift should be false (no matching findings)")
	}
	if !corr.HasFailure {
		t.Error("HasFailure should be true (Warning event)")
	}
}

func TestCorrelateAll_NoFailure(t *testing.T) {
	findings := []DriftFinding{
		{ObjectID: "Deployment:prod/api"},
	}

	events := []TimelineEvent{
		{ResourceKind: "Deployment", ResourceName: "api", K8sEvent: K8sEvent{Type: "Normal"}},
	}

	logs := []ContainerLogResult{
		{Namespace: "prod", PodName: "api-abc", ContainerName: "app"}, // no error
	}

	podNames := []string{"api-abc"}

	corr := CorrelateAll("Deployment:prod/api", "Deployment", "prod", "api", findings, events, logs, podNames)

	if !corr.HasDrift {
		t.Error("HasDrift should be true")
	}
	if corr.HasFailure {
		t.Error("HasFailure should be false (Normal event, no log errors)")
	}
}

// TestNoNewFactsIntroduced verifies that correlation functions return
// references to existing data without creating new facts.
// This is the Leak Test for correlation helpers.
func TestNoNewFactsIntroduced(t *testing.T) {
	// Original finding
	original := DriftFinding{
		ID:             "drift:Deployment:prod/api:spec.replicas",
		ObjectID:       "Deployment:prod/api",
		Object:         DriftObjectID{Kind: "Deployment", Namespace: "prod", Name: "api"},
		Path:           "spec.replicas",
		Desired:        3,
		Live:           5,
		Classification: DriftCapacity,
		Severity:       DriftSeverityWarning,
	}

	findings := []DriftFinding{original}

	// Get filtered result
	filtered := FindingsForObject(findings, "Deployment:prod/api")

	if len(filtered) != 1 {
		t.Fatal("expected 1 finding")
	}

	// Verify the returned finding is the same data (no transformation)
	got := filtered[0]
	if got.ID != original.ID {
		t.Errorf("ID changed: got %s, want %s", got.ID, original.ID)
	}
	if got.ObjectID != original.ObjectID {
		t.Errorf("ObjectID changed: got %s, want %s", got.ObjectID, original.ObjectID)
	}
	if got.Path != original.Path {
		t.Errorf("Path changed: got %s, want %s", got.Path, original.Path)
	}
	if got.Desired != original.Desired {
		t.Errorf("Desired changed: got %v, want %v", got.Desired, original.Desired)
	}
	if got.Live != original.Live {
		t.Errorf("Live changed: got %v, want %v", got.Live, original.Live)
	}
	if got.Classification != original.Classification {
		t.Errorf("Classification changed: got %s, want %s", got.Classification, original.Classification)
	}
	if got.Severity != original.Severity {
		t.Errorf("Severity changed: got %s, want %s", got.Severity, original.Severity)
	}
}

func TestHasFailureSignals(t *testing.T) {
	tests := []struct {
		name   string
		events []TimelineEvent
		logs   []ContainerLogResult
		want   bool
	}{
		{
			name:   "no signals",
			events: []TimelineEvent{{K8sEvent: K8sEvent{Type: "Normal"}}},
			logs:   []ContainerLogResult{{}},
			want:   false,
		},
		{
			name:   "warning event",
			events: []TimelineEvent{{K8sEvent: K8sEvent{Type: "Warning"}}},
			logs:   []ContainerLogResult{},
			want:   true,
		},
		{
			name:   "error severity event",
			events: []TimelineEvent{{K8sEvent: K8sEvent{Type: "Normal"}, Severity: "error"}},
			logs:   []ContainerLogResult{},
			want:   true,
		},
		{
			name:   "log error",
			events: []TimelineEvent{},
			logs:   []ContainerLogResult{{Error: "connection refused"}},
			want:   true,
		},
		{
			name:   "log pattern error",
			events: []TimelineEvent{},
			logs:   []ContainerLogResult{{Patterns: []LogPattern{{Type: "error"}}}},
			want:   true,
		},
		{
			name:   "log pattern panic",
			events: []TimelineEvent{},
			logs:   []ContainerLogResult{{Patterns: []LogPattern{{Type: "panic"}}}},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasFailureSignals(tt.events, tt.logs)
			if got != tt.want {
				t.Errorf("hasFailureSignals() = %v, want %v", got, tt.want)
			}
		})
	}
}
