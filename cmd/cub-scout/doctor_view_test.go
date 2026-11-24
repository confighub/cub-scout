// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNormalizeDoctorView(t *testing.T) {
	v, err := normalizeDoctorView("", false, "ascii")
	if err != nil || v != doctorViewAll {
		t.Fatalf("default view = (%q, %v), want (all, nil)", v, err)
	}
	v, err = normalizeDoctorView("summary", true, "json")
	if err != nil || v != doctorViewSummary {
		t.Fatalf("summary json = (%q, %v), want (summary, nil)", v, err)
	}
	if _, err := normalizeDoctorView("summary", true, "ascii"); err == nil || !strings.Contains(err.Error(), "--format json") {
		t.Fatalf("expected --format json error for --view on ascii, got %v", err)
	}
	if _, err := normalizeDoctorView("bogus", true, "json"); err == nil || !strings.Contains(err.Error(), "valid: all, summary") {
		t.Fatalf("expected valid-values error for bogus view, got %v", err)
	}
}

func TestProjectDoctorViewSummaryPreservesAnswerAndShrinksPayload(t *testing.T) {
	full := DoctorSummary{
		Cluster:           "prod-east",
		KubernetesContext: "prod-east",
		Namespace:         "all",
		Resources:         DoctorResourceSummary{Total: 42},
		Ownership:         DoctorOwnershipSummary{Flux: 30, ArgoCD: 10, Native: 2, Unmanaged: 2},
		Health:            DoctorHealthSummary{Healthy: 40, Warning: 1, Error: 1},
		Risks:             DoctorRiskSummary{Total: 3, Critical: 1, Warning: 2},
		Drift: DoctorDriftSummary{Resources: 1},
		TopIssues: []DoctorIssue{
			{Severity: "CRITICAL", Resource: "Deployment/payments", Namespace: "prod", Message: "Container runs as root with privileged securityContext"},
			{Severity: "WARNING", Resource: "Deployment/api", Namespace: "prod", Message: "1/3 pods ready; CrashLoopBackOff"},
		},
		NextSteps: []StructuredHint{
			{ActionType: "read-only", NextCommand: "cub scout explain Deployment/payments -n prod"},
		},
		Warnings: []string{"optional connected evidence omitted"},
		ThreeWay: &DoctorThreeWaySummary{Available: true, Hint: "cub scout compare three-way --scope cluster"},
	}
	full.Answer = buildDoctorAnswer(full, time.Date(2026, 11, 24, 18, 0, 0, 0, time.UTC))

	compact := projectDoctorView(full, doctorViewSummary)

	if len(compact.TopIssues) != 0 || len(compact.NextSteps) != 0 || len(compact.Warnings) != 0 || compact.ThreeWay != nil || compact.Rollouts != nil || compact.Delivery != nil || compact.Attach != nil || compact.DeliveryEvidence != nil {
		t.Fatalf("summary view must omit detail blocks, got %+v", compact)
	}
	if compact.Resources != full.Resources || compact.Ownership != full.Ownership || compact.Health != full.Health || compact.Risks != full.Risks || compact.Drift != full.Drift {
		t.Fatalf("summary view lost top-level counts: %+v", compact)
	}

	fullAnswer, err := json.Marshal(full.Answer)
	if err != nil {
		t.Fatal(err)
	}
	compactAnswer, err := json.Marshal(compact.Answer)
	if err != nil {
		t.Fatal(err)
	}
	if string(fullAnswer) != string(compactAnswer) {
		t.Fatalf("summary view changed answer:\nfull:    %s\ncompact: %s", fullAnswer, compactAnswer)
	}

	fullBytes, err := json.Marshal(full)
	if err != nil {
		t.Fatal(err)
	}
	compactBytes, err := json.Marshal(compact)
	if err != nil {
		t.Fatal(err)
	}
	if len(compactBytes) >= len(fullBytes) {
		t.Fatalf("summary view (%d B) did not shrink full view (%d B)", len(compactBytes), len(fullBytes))
	}
}

func TestDoctorMCPViewArg(t *testing.T) {
	gw := newMCPGateway(nil)
	tool, ok := gw.tools["doctor"]
	if !ok {
		t.Fatal("doctor MCP tool not registered")
	}
	args, err := tool.BuildArgs(map[string]interface{}{"namespace": "prod", "view": "summary"})
	if err != nil {
		t.Fatalf("BuildArgs failed: %v", err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"doctor --format json", "-n prod", "--view summary"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
}
