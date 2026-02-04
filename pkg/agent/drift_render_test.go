// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"strings"
	"testing"
)

func TestRenderDriftASCII_NoFindings(t *testing.T) {
	report := DriftReport{
		Context: DriftContext{
			Cluster:       "test-cluster",
			DesiredSource: DriftSource{Type: "file", Ref: "test.yaml"},
			LiveSource:    DriftSource{Type: "cluster", Ref: "test-cluster"},
		},
		Findings: []DriftFinding{},
		Summary:  BuildDriftSummary([]DriftFinding{}),
	}

	output := RenderDriftASCII(report)

	if !strings.Contains(output, "No drift detected") {
		t.Errorf("expected 'No drift detected' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "test-cluster") {
		t.Errorf("expected cluster name in output")
	}
}

func TestRenderDriftASCII_SingleFinding(t *testing.T) {
	findings := []DriftFinding{
		NewDriftFinding(
			DriftObjectID{Kind: "Deployment", Namespace: "prod", Name: "web"},
			"spec.replicas",
			3, 5,
			DriftCapacity,
			DriftSeverityWarning,
		),
	}

	report := DriftReport{
		Context: DriftContext{
			Cluster:       "prod-cluster",
			DesiredSource: DriftSource{Type: "file", Ref: "manifests/"},
			LiveSource:    DriftSource{Type: "cluster", Ref: "prod-cluster"},
		},
		Findings: findings,
		Summary:  BuildDriftSummary(findings),
	}

	output := RenderDriftASCII(report)

	// Verify structural facts are rendered (f layer)
	mustContain := []string{
		"Deployment:prod/web",  // object identity
		"spec.replicas",        // path
		"3",                    // desired value
		"5",                    // live value
		"WARNING",              // severity (inferable)
		"Capacity",             // classification (via grouping)
	}

	for _, s := range mustContain {
		if !strings.Contains(output, s) {
			t.Errorf("expected %q in output, got:\n%s", s, output)
		}
	}

	// Verify summary
	if !strings.Contains(output, "Total findings: 1") {
		t.Errorf("expected summary in output")
	}
}

func TestRenderDriftASCII_MultipleClassifications(t *testing.T) {
	findings := []DriftFinding{
		NewDriftFinding(
			DriftObjectID{Kind: "Deployment", Namespace: "prod", Name: "api"},
			"spec.replicas",
			3, 5,
			DriftCapacity,
			DriftSeverityWarning,
		),
		NewDriftFinding(
			DriftObjectID{Kind: "Deployment", Namespace: "prod", Name: "web"},
			"spec.template.spec.containers[0].image",
			"nginx:1.19", "nginx:1.21",
			DriftImage,
			DriftSeverityCritical,
		),
		NewDriftFinding(
			DriftObjectID{Kind: "Deployment", Namespace: "prod", Name: "worker"},
			"spec.template.spec.containers[0].env[0].value",
			"prod", "staging",
			DriftConfig,
			DriftSeverityInfo,
		),
	}

	report := DriftReport{
		Context: DriftContext{
			Cluster:       "prod-cluster",
			DesiredSource: DriftSource{Type: "git", Ref: "abc123"},
			LiveSource:    DriftSource{Type: "cluster", Ref: "prod-cluster"},
		},
		Findings: findings,
		Summary:  BuildDriftSummary(findings),
	}

	output := RenderDriftASCII(report)

	// Verify all classifications appear
	mustContain := []string{
		"Capacity Drift",
		"Image Drift",
		"Configuration Drift",
	}

	for _, s := range mustContain {
		if !strings.Contains(output, s) {
			t.Errorf("expected classification %q in output, got:\n%s", s, output)
		}
	}

	// Verify all findings are rendered (lossless)
	if !strings.Contains(output, "api") {
		t.Errorf("expected api finding in output")
	}
	if !strings.Contains(output, "web") {
		t.Errorf("expected web finding in output")
	}
	if !strings.Contains(output, "worker") {
		t.Errorf("expected worker finding in output")
	}
}

func TestRenderDriftASCII_GroupOrderBySeverity(t *testing.T) {
	// Create findings where Image (critical) should come before Capacity (warning)
	findings := []DriftFinding{
		NewDriftFinding(
			DriftObjectID{Kind: "Deployment", Namespace: "prod", Name: "api"},
			"spec.replicas",
			3, 5,
			DriftCapacity,
			DriftSeverityWarning,
		),
		NewDriftFinding(
			DriftObjectID{Kind: "Deployment", Namespace: "prod", Name: "web"},
			"spec.template.spec.containers[0].image",
			"nginx:1.19", "apache:2.4",
			DriftImage,
			DriftSeverityCritical,
		),
	}

	report := DriftReport{
		Context: DriftContext{
			Cluster:       "test",
			DesiredSource: DriftSource{Type: "file", Ref: "test.yaml"},
			LiveSource:    DriftSource{Type: "cluster", Ref: "test"},
		},
		Findings: findings,
		Summary:  BuildDriftSummary(findings),
	}

	output := RenderDriftASCII(report)

	// Image (critical) should appear before Capacity (warning)
	imageIdx := strings.Index(output, "Image Drift")
	capacityIdx := strings.Index(output, "Capacity Drift")

	if imageIdx == -1 || capacityIdx == -1 {
		t.Fatalf("both classifications should appear in output")
	}

	if imageIdx > capacityIdx {
		t.Errorf("Image (critical) should appear before Capacity (warning), but Image at %d, Capacity at %d",
			imageIdx, capacityIdx)
	}
}

func TestRenderDriftASCII_AllFindingsRendered(t *testing.T) {
	// Create multiple findings and verify each appears exactly once
	findings := []DriftFinding{
		NewDriftFinding(
			DriftObjectID{Kind: "Deployment", Namespace: "a", Name: "first"},
			"spec.replicas",
			1, 2,
			DriftCapacity,
			DriftSeverityInfo,
		),
		NewDriftFinding(
			DriftObjectID{Kind: "Deployment", Namespace: "b", Name: "second"},
			"spec.replicas",
			3, 4,
			DriftCapacity,
			DriftSeverityWarning,
		),
		NewDriftFinding(
			DriftObjectID{Kind: "Deployment", Namespace: "c", Name: "third"},
			"spec.template.spec.containers[0].image",
			"img:v1", "img:v2",
			DriftImage,
			DriftSeverityCritical,
		),
	}

	report := DriftReport{
		Context: DriftContext{
			Cluster:       "test",
			DesiredSource: DriftSource{Type: "file", Ref: "test.yaml"},
			LiveSource:    DriftSource{Type: "cluster", Ref: "test"},
		},
		Findings: findings,
		Summary:  BuildDriftSummary(findings),
	}

	output := RenderDriftASCII(report)

	// Each finding's path should appear exactly once
	for _, f := range findings {
		count := strings.Count(output, f.ObjectID)
		if count != 1 {
			t.Errorf("finding %s should appear exactly once, appeared %d times", f.ObjectID, count)
		}
	}
}

func TestRenderDriftASCII_NamespaceInContext(t *testing.T) {
	ns := "production"
	report := DriftReport{
		Context: DriftContext{
			Cluster:       "test",
			Namespace:     &ns,
			DesiredSource: DriftSource{Type: "file", Ref: "test.yaml"},
			LiveSource:    DriftSource{Type: "cluster", Ref: "test"},
		},
		Findings: []DriftFinding{},
		Summary:  BuildDriftSummary([]DriftFinding{}),
	}

	output := RenderDriftASCII(report)

	if !strings.Contains(output, "Namespace: production") {
		t.Errorf("expected namespace in output when set")
	}
}

func TestClassificationIcon(t *testing.T) {
	// Ensure all classifications have icons (no panics, no empty strings)
	classifications := []DriftClassification{
		DriftCapacity,
		DriftImage,
		DriftConfig,
		DriftResource,
		DriftRollout,
		DriftHealth,
		DriftLabel,
		DriftAnnotation,
		DriftOther,
	}

	for _, c := range classifications {
		icon := classificationIcon(c)
		if icon == "" {
			t.Errorf("classification %s has empty icon", c)
		}
	}
}

func TestSeverityIcon(t *testing.T) {
	// Ensure all severities have icons
	severities := []DriftSeverity{
		DriftSeverityCritical,
		DriftSeverityWarning,
		DriftSeverityInfo,
	}

	for _, s := range severities {
		icon := severityIcon(s)
		if icon == "" {
			t.Errorf("severity %s has empty icon", s)
		}
	}
}
