// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"strings"
	"testing"
)

func TestRenderCorrelationNarrative_DriftAndFailure(t *testing.T) {
	corr := DriftCorrelation{
		ObjectID: "Deployment:prod/api",
		Findings: []DriftFinding{
			{Path: "spec.replicas", Classification: DriftCapacity},
			{Path: "spec.template.spec.containers[name=app].image", Classification: DriftImage},
		},
		Events: []TimelineEvent{
			{K8sEvent: K8sEvent{Type: "Warning", Reason: "FailedScheduling"}},
		},
		Logs:       []ContainerLogResult{},
		HasDrift:   true,
		HasFailure: true,
	}

	narrative := RenderCorrelationNarrative(corr)

	// Check summary mentions both drift and failure
	if !strings.Contains(narrative.Summary, "both drift and failure") {
		t.Errorf("Summary should mention both drift and failure, got: %s", narrative.Summary)
	}

	// Check drift paths are captured
	if len(narrative.DriftPaths) != 2 {
		t.Errorf("Expected 2 drift paths, got %d", len(narrative.DriftPaths))
	}

	// Check failure signals are captured
	if len(narrative.FailureSignals) != 1 {
		t.Errorf("Expected 1 failure signal, got %d", len(narrative.FailureSignals))
	}

	// Check insight suggests correlation
	if !strings.Contains(narrative.Insight, "may have caused") {
		t.Errorf("Insight should suggest drift may have caused failure, got: %s", narrative.Insight)
	}
}

func TestRenderCorrelationNarrative_DriftOnly(t *testing.T) {
	corr := DriftCorrelation{
		ObjectID: "Deployment:prod/api",
		Findings: []DriftFinding{
			{Path: "spec.replicas", Classification: DriftCapacity},
		},
		Events:     []TimelineEvent{},
		Logs:       []ContainerLogResult{},
		HasDrift:   true,
		HasFailure: false,
	}

	narrative := RenderCorrelationNarrative(corr)

	// Check summary mentions drift but no failure
	if !strings.Contains(narrative.Summary, "drift but no failure") {
		t.Errorf("Summary should mention drift but no failure, got: %s", narrative.Summary)
	}

	// Check insight suggests intentional drift
	if !strings.Contains(narrative.Insight, "intentional") {
		t.Errorf("Insight should mention intentional possibility, got: %s", narrative.Insight)
	}
}

func TestRenderCorrelationNarrative_FailureOnly(t *testing.T) {
	corr := DriftCorrelation{
		ObjectID: "Deployment:prod/api",
		Findings: []DriftFinding{},
		Events: []TimelineEvent{
			{K8sEvent: K8sEvent{Type: "Warning", Reason: "CrashLoopBackOff"}},
		},
		Logs: []ContainerLogResult{
			{Error: "connection refused"},
		},
		HasDrift:   false,
		HasFailure: true,
	}

	narrative := RenderCorrelationNarrative(corr)

	// Check summary mentions failure but no drift
	if !strings.Contains(narrative.Summary, "failure signals but no drift") {
		t.Errorf("Summary should mention failure but no drift, got: %s", narrative.Summary)
	}

	// Check insight suggests runtime issue
	if !strings.Contains(narrative.Insight, "runtime") {
		t.Errorf("Insight should mention runtime issue, got: %s", narrative.Insight)
	}

	// Check no drift paths
	if len(narrative.DriftPaths) != 0 {
		t.Errorf("Expected 0 drift paths, got %d", len(narrative.DriftPaths))
	}

	// Check failure signals are captured
	if len(narrative.FailureSignals) < 1 {
		t.Errorf("Expected at least 1 failure signal, got %d", len(narrative.FailureSignals))
	}
}

func TestRenderCorrelationNarrative_Healthy(t *testing.T) {
	corr := DriftCorrelation{
		ObjectID:   "Deployment:prod/api",
		Findings:   []DriftFinding{},
		Events:     []TimelineEvent{{K8sEvent: K8sEvent{Type: "Normal"}}},
		Logs:       []ContainerLogResult{},
		HasDrift:   false,
		HasFailure: false,
	}

	narrative := RenderCorrelationNarrative(corr)

	// Check summary indicates no issues
	if !strings.Contains(narrative.Summary, "no drift or failure") {
		t.Errorf("Summary should indicate no issues, got: %s", narrative.Summary)
	}

	// Check insight indicates healthy
	if !strings.Contains(narrative.Insight, "healthy") {
		t.Errorf("Insight should mention healthy, got: %s", narrative.Insight)
	}
}

func TestRenderCorrelationASCII(t *testing.T) {
	corr := DriftCorrelation{
		ObjectID: "Deployment:prod/api",
		Findings: []DriftFinding{
			{Path: "spec.replicas", Classification: DriftCapacity},
		},
		Events: []TimelineEvent{
			{K8sEvent: K8sEvent{Type: "Warning", Reason: "FailedScheduling"}},
		},
		Logs:       []ContainerLogResult{},
		HasDrift:   true,
		HasFailure: true,
	}

	ascii := RenderCorrelationASCII(corr)

	// Check header
	if !strings.Contains(ascii, "Drift-Debug Correlation") {
		t.Error("ASCII should contain header")
	}

	// Check icon for drift+failure
	if !strings.Contains(ascii, "⚠") {
		t.Error("ASCII should contain warning icon for drift+failure")
	}

	// Check drift path is listed
	if !strings.Contains(ascii, "spec.replicas") {
		t.Error("ASCII should contain drift path")
	}

	// Check failure signal is listed
	if !strings.Contains(ascii, "FailedScheduling") {
		t.Error("ASCII should contain failure signal")
	}

	// Check insight is present
	if !strings.Contains(ascii, "💡") {
		t.Error("ASCII should contain insight indicator")
	}
}

func TestRenderCorrelationSummaryLine(t *testing.T) {
	tests := []struct {
		name     string
		corr     DriftCorrelation
		contains string
	}{
		{
			name: "drift and failure",
			corr: DriftCorrelation{
				HasDrift:   true,
				HasFailure: true,
				Findings:   []DriftFinding{{Path: "a"}},
				Events:     []TimelineEvent{{K8sEvent: K8sEvent{Type: "Warning"}}},
			},
			contains: "may be related",
		},
		{
			name: "drift only",
			corr: DriftCorrelation{
				HasDrift:   true,
				HasFailure: false,
				Findings:   []DriftFinding{{Path: "a"}},
			},
			contains: "may be intentional",
		},
		{
			name: "failure only",
			corr: DriftCorrelation{
				HasDrift:   false,
				HasFailure: true,
				Events:     []TimelineEvent{{K8sEvent: K8sEvent{Type: "Warning"}}},
			},
			contains: "runtime issue",
		},
		{
			name: "healthy",
			corr: DriftCorrelation{
				HasDrift:   false,
				HasFailure: false,
			},
			contains: "No drift or failures",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := RenderCorrelationSummaryLine(tt.corr)
			if !strings.Contains(line, tt.contains) {
				t.Errorf("Summary line should contain %q, got: %s", tt.contains, line)
			}
		})
	}
}

func TestExtractFailureSignalTypes(t *testing.T) {
	events := []TimelineEvent{
		{K8sEvent: K8sEvent{Type: "Warning", Reason: "FailedScheduling"}},
		{K8sEvent: K8sEvent{Type: "Warning", Reason: "BackOff"}},
		{K8sEvent: K8sEvent{Type: "Normal", Reason: "ScalingReplicaSet"}},
	}

	logs := []ContainerLogResult{
		{Patterns: []LogPattern{{Type: "error"}}},
		{Patterns: []LogPattern{{Type: "panic"}}},
	}

	types := extractFailureSignalTypes(events, logs)

	// Should have unique types
	typeSet := make(map[string]bool)
	for _, t := range types {
		typeSet[t] = true
	}

	if !typeSet["FailedScheduling"] {
		t.Error("Should contain FailedScheduling")
	}
	if !typeSet["BackOff"] {
		t.Error("Should contain BackOff")
	}
	if !typeSet["error"] {
		t.Error("Should contain error from log pattern")
	}
	if !typeSet["panic"] {
		t.Error("Should contain panic from log pattern")
	}
	if typeSet["ScalingReplicaSet"] {
		t.Error("Should not contain Normal event reason")
	}
}

// TestNarrativeDerivesFromFacts verifies that narrative content is derived
// from correlation facts, not invented. This is part of the Leak Test.
func TestNarrativeDerivesFromFacts(t *testing.T) {
	corr := DriftCorrelation{
		ObjectID: "Deployment:prod/api",
		Findings: []DriftFinding{
			{
				ID:             "drift:1",
				Path:           "spec.replicas",
				Classification: DriftCapacity,
				Severity:       DriftSeverityWarning,
			},
		},
		Events: []TimelineEvent{
			{
				ResourceKind: "Deployment",
				ResourceName: "api",
				K8sEvent:     K8sEvent{Type: "Warning", Reason: "FailedScheduling"},
			},
		},
		HasDrift:   true,
		HasFailure: true,
	}

	narrative := RenderCorrelationNarrative(corr)

	// Verify drift paths come from findings
	if len(narrative.DriftPaths) != 1 || narrative.DriftPaths[0] != "spec.replicas" {
		t.Error("DriftPaths should be derived from Findings.Path")
	}

	// Verify failure signals come from events
	found := false
	for _, sig := range narrative.FailureSignals {
		if sig == "FailedScheduling" {
			found = true
			break
		}
	}
	if !found {
		t.Error("FailureSignals should contain event reasons")
	}

	// Verify summary uses ObjectID from correlation
	if !strings.Contains(narrative.Summary, corr.ObjectID) {
		t.Error("Summary should use ObjectID from correlation")
	}
}

func TestCountFunctions(t *testing.T) {
	events := []TimelineEvent{
		{K8sEvent: K8sEvent{Type: "Warning"}},
		{K8sEvent: K8sEvent{Type: "Warning"}},
		{K8sEvent: K8sEvent{Type: "Normal"}},
		{Severity: "error"},
	}

	logs := []ContainerLogResult{
		{Error: "some error"},
		{Patterns: []LogPattern{{Type: "error"}, {Type: "panic"}}},
	}

	if got := countWarningEvents(events); got != 3 {
		t.Errorf("countWarningEvents = %d, want 3", got)
	}

	if got := countLogErrors(logs); got != 3 {
		t.Errorf("countLogErrors = %d, want 3 (1 error + 2 patterns)", got)
	}

	if got := countFailureSignals(events, logs); got != 6 {
		t.Errorf("countFailureSignals = %d, want 6", got)
	}
}
