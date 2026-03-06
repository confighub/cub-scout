package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/internal/scan"
	"github.com/spf13/cobra"
)

func TestContextPackCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "context-pack" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("context-pack command is not registered on rootCmd")
	}
}

func TestBuildContextPackTraceSeeds_DeterministicOrder(t *testing.T) {
	entries := []MapEntry{
		{Kind: "Deployment", Name: "checkout", Namespace: "prod", Owner: "Flux", Status: "Ready"},
		{Kind: "Deployment", Name: "api", Namespace: "prod", Owner: "Native", Status: "NotReady"},
		{Kind: "Application", Name: "payments", Namespace: "argocd", Owner: "ArgoCD", Status: "OutOfSync"},
	}

	got := buildContextPackTraceSeeds(entries, 2)
	if len(got) != 2 {
		t.Fatalf("seed count = %d, want 2", len(got))
	}
	if got[0].Kind != "Application" || got[0].Name != "payments" {
		t.Fatalf("unexpected first seed ordering: %+v", got[0])
	}
	if got[0].Confidence != "high" {
		t.Fatalf("seed confidence = %q, want high", got[0].Confidence)
	}
	if !strings.Contains(got[1].TraceCommand, "cub-scout trace deployment/") {
		t.Fatalf("unexpected trace command: %q", got[1].TraceCommand)
	}
}

func TestApplyContextPackSizeLimit_Truncates(t *testing.T) {
	pack := contextPackV2{
		Version:     "v2",
		GeneratedAt: "2026-03-06T20:00:00Z",
		Cluster:     "kind-dev",
		Namespace:   "all",
		TopRisks: []DoctorIssue{
			{Severity: "CRITICAL", Resource: "deploy/a", Namespace: "prod", Message: strings.Repeat("x", 80)},
			{Severity: "WARNING", Resource: "deploy/b", Namespace: "prod", Message: strings.Repeat("y", 80)},
		},
		TraceSeeds: []contextPackTraceSeed{
			{Kind: "Deployment", Name: "a", Namespace: "prod", TraceCommand: "cub-scout trace deployment/a -n prod"},
			{Kind: "Deployment", Name: "b", Namespace: "prod", TraceCommand: "cub-scout trace deployment/b -n prod"},
		},
	}

	got, payload, err := applyContextPackSizeLimit(pack, 650)
	if err != nil {
		t.Fatalf("applyContextPackSizeLimit error = %v", err)
	}
	if len(payload) > 650 {
		t.Fatalf("payload size = %d, want <= 650", len(payload))
	}
	if !got.Truncated {
		t.Fatalf("expected truncated=true when limiting payload")
	}
}

func TestRunContextPack_FromFixtureJSON(t *testing.T) {
	restore := withContextPackFlagsForTest()
	defer restore()

	fixture := contextPackInput{
		Cluster: "kind-dev",
		Entries: []MapEntry{
			{Kind: "Deployment", Name: "checkout", Namespace: "prod", Owner: "Flux", Status: "Ready"},
			{Kind: "Deployment", Name: "debug-shell", Namespace: "prod", Owner: "Native", Status: "NotReady"},
		},
		Findings: []scan.NormalizedFinding{
			{Severity: "critical", Resource: "Deployment/checkout", Namespace: "prod", Message: "missing limits"},
			{Severity: "warning", Resource: "Deployment/debug-shell", Namespace: "prod", Message: "no probes"},
		},
	}
	fixturePath := filepath.Join(t.TempDir(), "context-pack-input.json")
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	if err := os.WriteFile(fixturePath, raw, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	t.Setenv("CUB_SCOUT_TEST_CONTEXT_PACK_INPUT_JSON", fixturePath)
	t.Setenv("CUB_SCOUT_TEST_TIME", "2026-03-06T20:30:00Z")

	contextPackFormat = "json"
	contextPackNamespace = ""
	contextPackTopRisks = 2
	contextPackTraceN = 2
	contextPackMaxBytes = 4096
	contextPackNowFn = func() time.Time { return time.Date(2026, 3, 6, 20, 30, 0, 0, time.UTC) }

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	out := captureStdout(t, func() {
		if err := runContextPack(cmd, nil); err != nil {
			t.Fatalf("runContextPack() error = %v", err)
		}
	})

	var payload contextPackV2
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode context-pack json: %v\n%s", err, out)
	}

	if payload.Version != "v2" {
		t.Fatalf("version = %q, want v2", payload.Version)
	}
	if payload.Cluster != "kind-dev" {
		t.Fatalf("cluster = %q, want kind-dev", payload.Cluster)
	}
	if payload.OwnershipSummary.Flux != 1 || payload.OwnershipSummary.Native != 1 {
		t.Fatalf("unexpected ownership summary: %+v", payload.OwnershipSummary)
	}
	if len(payload.TopRisks) != 2 {
		t.Fatalf("topRisks len = %d, want 2", len(payload.TopRisks))
	}
	if len(payload.TraceSeeds) != 2 {
		t.Fatalf("traceSeeds len = %d, want 2", len(payload.TraceSeeds))
	}
	if payload.SizeBytes <= 0 {
		t.Fatalf("sizeBytes = %d, want > 0", payload.SizeBytes)
	}
}

func withContextPackFlagsForTest() func() {
	prevNamespace := contextPackNamespace
	prevFormat := contextPackFormat
	prevTopRisks := contextPackTopRisks
	prevTraceN := contextPackTraceN
	prevMaxBytes := contextPackMaxBytes
	prevNowFn := contextPackNowFn

	contextPackNamespace = ""
	contextPackFormat = "json"
	contextPackTopRisks = 5
	contextPackTraceN = 5
	contextPackMaxBytes = 16384
	contextPackNowFn = time.Now

	return func() {
		contextPackNamespace = prevNamespace
		contextPackFormat = prevFormat
		contextPackTopRisks = prevTopRisks
		contextPackTraceN = prevTraceN
		contextPackMaxBytes = prevMaxBytes
		contextPackNowFn = prevNowFn
	}
}
