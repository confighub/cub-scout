// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
)

func TestIsValidSeverityLevel(t *testing.T) {
	tests := []struct {
		level string
		want  bool
	}{
		{"info", true},
		{"warning", true},
		{"critical", true},
		{"INFO", false},      // case sensitive
		{"warn", false},      // not a valid alias
		{"error", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			got := isValidSeverityLevel(tt.level)
			if got != tt.want {
				t.Errorf("isValidSeverityLevel(%q) = %v, want %v", tt.level, got, tt.want)
			}
		})
	}
}

func TestGetMaxSeverity(t *testing.T) {
	tests := []struct {
		name     string
		findings []agent.DriftFinding
		want     agent.DriftSeverity
	}{
		{
			name:     "empty findings",
			findings: []agent.DriftFinding{},
			want:     agent.DriftSeverityInfo,
		},
		{
			name: "single info",
			findings: []agent.DriftFinding{
				{Severity: agent.DriftSeverityInfo},
			},
			want: agent.DriftSeverityInfo,
		},
		{
			name: "single warning",
			findings: []agent.DriftFinding{
				{Severity: agent.DriftSeverityWarning},
			},
			want: agent.DriftSeverityWarning,
		},
		{
			name: "single critical",
			findings: []agent.DriftFinding{
				{Severity: agent.DriftSeverityCritical},
			},
			want: agent.DriftSeverityCritical,
		},
		{
			name: "mixed - critical wins",
			findings: []agent.DriftFinding{
				{Severity: agent.DriftSeverityInfo},
				{Severity: agent.DriftSeverityWarning},
				{Severity: agent.DriftSeverityCritical},
			},
			want: agent.DriftSeverityCritical,
		},
		{
			name: "mixed - warning wins over info",
			findings: []agent.DriftFinding{
				{Severity: agent.DriftSeverityInfo},
				{Severity: agent.DriftSeverityWarning},
				{Severity: agent.DriftSeverityInfo},
			},
			want: agent.DriftSeverityWarning,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getMaxSeverity(tt.findings)
			if got != tt.want {
				t.Errorf("getMaxSeverity() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSeverityMeetsThreshold(t *testing.T) {
	tests := []struct {
		maxSeverity agent.DriftSeverity
		threshold   string
		want        bool
	}{
		// info threshold - everything meets it
		{agent.DriftSeverityInfo, "info", true},
		{agent.DriftSeverityWarning, "info", true},
		{agent.DriftSeverityCritical, "info", true},

		// warning threshold
		{agent.DriftSeverityInfo, "warning", false},
		{agent.DriftSeverityWarning, "warning", true},
		{agent.DriftSeverityCritical, "warning", true},

		// critical threshold
		{agent.DriftSeverityInfo, "critical", false},
		{agent.DriftSeverityWarning, "critical", false},
		{agent.DriftSeverityCritical, "critical", true},
	}

	for _, tt := range tests {
		t.Run(string(tt.maxSeverity)+"_vs_"+tt.threshold, func(t *testing.T) {
			got := severityMeetsThreshold(tt.maxSeverity, tt.threshold)
			if got != tt.want {
				t.Errorf("severityMeetsThreshold(%v, %q) = %v, want %v",
					tt.maxSeverity, tt.threshold, got, tt.want)
			}
		})
	}
}

func TestComputeDriftExitCode(t *testing.T) {
	// Helper to create findings with specific severities
	makeFinding := func(sev agent.DriftSeverity) agent.DriftFinding {
		return agent.DriftFinding{
			ID:             "test",
			ObjectID:       "test",
			Severity:       sev,
			Classification: agent.DriftCapacity,
		}
	}

	tests := []struct {
		name     string
		findings []agent.DriftFinding
		failOn   string
		want     int
	}{
		// No --fail-on = always exit 0 (pure reporting)
		{
			name:     "no fail-on, no findings",
			findings: []agent.DriftFinding{},
			failOn:   "",
			want:     DriftExitOK,
		},
		{
			name:     "no fail-on, has findings",
			findings: []agent.DriftFinding{makeFinding(agent.DriftSeverityCritical)},
			failOn:   "",
			want:     DriftExitOK,
		},

		// No findings = exit 0 regardless of fail-on
		{
			name:     "fail-on warning, no findings",
			findings: []agent.DriftFinding{},
			failOn:   "warning",
			want:     DriftExitOK,
		},
		{
			name:     "fail-on critical, no findings",
			findings: []agent.DriftFinding{},
			failOn:   "critical",
			want:     DriftExitOK,
		},

		// Findings with max severity info
		{
			name:     "max info, fail-on info -> exit 2",
			findings: []agent.DriftFinding{makeFinding(agent.DriftSeverityInfo)},
			failOn:   "info",
			want:     DriftExitFailure,
		},
		{
			name:     "max info, fail-on warning -> exit 0",
			findings: []agent.DriftFinding{makeFinding(agent.DriftSeverityInfo)},
			failOn:   "warning",
			want:     DriftExitOK,
		},
		{
			name:     "max info, fail-on critical -> exit 0",
			findings: []agent.DriftFinding{makeFinding(agent.DriftSeverityInfo)},
			failOn:   "critical",
			want:     DriftExitOK,
		},

		// Findings with max severity warning
		{
			name:     "max warning, fail-on info -> exit 2",
			findings: []agent.DriftFinding{makeFinding(agent.DriftSeverityWarning)},
			failOn:   "info",
			want:     DriftExitFailure,
		},
		{
			name:     "max warning, fail-on warning -> exit 2",
			findings: []agent.DriftFinding{makeFinding(agent.DriftSeverityWarning)},
			failOn:   "warning",
			want:     DriftExitFailure,
		},
		{
			name:     "max warning, fail-on critical -> exit 0",
			findings: []agent.DriftFinding{makeFinding(agent.DriftSeverityWarning)},
			failOn:   "critical",
			want:     DriftExitOK,
		},

		// Findings with max severity critical
		{
			name:     "max critical, fail-on info -> exit 2",
			findings: []agent.DriftFinding{makeFinding(agent.DriftSeverityCritical)},
			failOn:   "info",
			want:     DriftExitFailure,
		},
		{
			name:     "max critical, fail-on warning -> exit 2",
			findings: []agent.DriftFinding{makeFinding(agent.DriftSeverityCritical)},
			failOn:   "warning",
			want:     DriftExitFailure,
		},
		{
			name:     "max critical, fail-on critical -> exit 2",
			findings: []agent.DriftFinding{makeFinding(agent.DriftSeverityCritical)},
			failOn:   "critical",
			want:     DriftExitFailure,
		},

		// Mixed findings - max severity determines outcome
		{
			name: "mixed info+warning, fail-on warning -> exit 2",
			findings: []agent.DriftFinding{
				makeFinding(agent.DriftSeverityInfo),
				makeFinding(agent.DriftSeverityWarning),
			},
			failOn: "warning",
			want:   DriftExitFailure,
		},
		{
			name: "mixed info+warning, fail-on critical -> exit 0",
			findings: []agent.DriftFinding{
				makeFinding(agent.DriftSeverityInfo),
				makeFinding(agent.DriftSeverityWarning),
			},
			failOn: "critical",
			want:   DriftExitOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := agent.DriftReport{
				Findings: tt.findings,
			}
			got := computeDriftExitCode(report, tt.failOn)
			if got != tt.want {
				t.Errorf("computeDriftExitCode() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestExitCodeIndependentOfFormat verifies the Leak Test:
// Exit behavior depends only on JSON facts (severity) and --fail-on flag,
// not on output format selection.
func TestExitCodeIndependentOfFormat(t *testing.T) {
	report := agent.DriftReport{
		Findings: []agent.DriftFinding{
			{
				ID:             "test:1",
				ObjectID:       "Deployment:prod/web",
				Severity:       agent.DriftSeverityWarning,
				Classification: agent.DriftCapacity,
			},
		},
	}

	// Same report, same --fail-on, different hypothetical formats
	// should produce the same exit code
	failOn := "warning"
	expectedExit := DriftExitFailure

	// The exit code computation is format-independent
	exitCode := computeDriftExitCode(report, failOn)
	if exitCode != expectedExit {
		t.Errorf("exit code = %d, want %d", exitCode, expectedExit)
	}

	// Verify the computation uses only JSON facts
	// (This is structural - the function signature doesn't take format)
	// The test documents the Leak Test compliance
}
