package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/internal/scan"
	"github.com/spf13/cobra"
)

func TestQuickstartCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "quickstart" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("quickstart command is not registered on rootCmd")
	}
}

func TestPickQuickstartRepresentativePrefersWorkloadKinds(t *testing.T) {
	entries := []MapEntry{
		{Kind: "Service", Namespace: "prod", Name: "payments"},
		{Kind: "Deployment", Namespace: "prod", Name: "payments-api"},
		{Kind: "Pod", Namespace: "prod", Name: "payments-api-abc123"},
	}

	chosen, ok := pickQuickstartRepresentative(entries)
	if !ok {
		t.Fatal("expected representative workload to be selected")
	}
	if chosen.Kind != "Deployment" || chosen.Name != "payments-api" {
		t.Fatalf("unexpected representative: %s/%s", chosen.Kind, chosen.Name)
	}
}

func TestRunQuickstart_WithFixtureRendersGuidedFlow(t *testing.T) {
	fixture := quickstartFixtureInput{
		ClusterConnected: true,
		Context:          "kind-test",
		ConnectedPreview: true,
		Entries: []MapEntry{
			{Kind: "Deployment", Name: "payments-api", Namespace: "prod", Owner: "Flux", Status: "Ready"},
			{Kind: "Service", Name: "payments", Namespace: "prod", Owner: "Flux", Status: "Ready"},
		},
		Findings: []scan.NormalizedFinding{
			{Severity: "critical", Resource: "Deployment/payments-api", Namespace: "prod", Message: "missing limits"},
		},
	}

	fixturePath := writeQuickstartFixture(t, fixture)
	t.Setenv("CUB_SCOUT_TEST_QUICKSTART_JSON", fixturePath)

	restore := withQuickstartFlagsForTest(true)
	defer restore()

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	out := captureStdout(t, func() {
		if err := runQuickstart(cmd, nil); err != nil {
			t.Fatalf("runQuickstart failed: %v", err)
		}
	})

	required := []string{
		"Quickstart Wizard",
		"Step 1/7",
		"Doctor Summary",
		"Explain: Deployment/payments-api",
		"Ownership snapshot",
		"Top finding",
		"Connected mode preview available",
		"Next steps",
	}
	for _, s := range required {
		if !strings.Contains(out, s) {
			t.Fatalf("expected %q in output:\n%s", s, out)
		}
	}
}

func TestRunQuickstart_NoClusterFallback(t *testing.T) {
	fixture := quickstartFixtureInput{ClusterConnected: false}
	fixturePath := writeQuickstartFixture(t, fixture)
	t.Setenv("CUB_SCOUT_TEST_QUICKSTART_JSON", fixturePath)

	restore := withQuickstartFlagsForTest(true)
	defer restore()

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	out := captureStdout(t, func() {
		if err := runQuickstart(cmd, nil); err != nil {
			t.Fatalf("runQuickstart failed: %v", err)
		}
	})

	if !strings.Contains(out, "No cluster connection detected") {
		t.Fatalf("expected no-cluster fallback message, got:\n%s", out)
	}
	if !strings.Contains(out, "example bundle mode") {
		t.Fatalf("expected example bundle guidance, got:\n%s", out)
	}
}

func writeQuickstartFixture(t *testing.T, fixture quickstartFixtureInput) string {
	t.Helper()
	b, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "quickstart-fixture.json")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func withQuickstartFlagsForTest(yes bool) func() {
	prevYes := quickstartYes
	prevNamespace := quickstartNamespace
	prevTop := quickstartTopFinding
	quickstartYes = yes
	quickstartNamespace = ""
	quickstartTopFinding = 1

	return func() {
		quickstartYes = prevYes
		quickstartNamespace = prevNamespace
		quickstartTopFinding = prevTop
	}
}
