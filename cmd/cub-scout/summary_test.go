package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/confighub/cub-scout/internal/scan"
	"github.com/confighub/cub-scout/internal/summarystore"
	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/spf13/cobra"
)

func TestBuildScanSummaryRecord(t *testing.T) {
	now := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)

	result := &scan.CombinedResult{
		Kyverno: &agent.ScanResult{
			Findings: []agent.ScanFinding{
				{Severity: "critical"},
				{Severity: "warning"},
			},
		},
		State: &agent.StateScanResult{
			Findings: []agent.StuckFinding{
				{Severity: "warning"},
			},
		},
	}

	record, err := buildScanSummaryRecord(result, "kind-dev", "prod", now)
	if err != nil {
		t.Fatalf("buildScanSummaryRecord() error = %v", err)
	}
	if record.Type != "scan" {
		t.Fatalf("type = %q, want scan", record.Type)
	}
	if record.Cluster != "kind-dev" {
		t.Fatalf("cluster = %q, want kind-dev", record.Cluster)
	}
	if record.Scope.Namespace != "prod" {
		t.Fatalf("namespace = %q, want prod", record.Scope.Namespace)
	}
	if record.Metrics.RiskTotal != 3 || record.Metrics.RiskCritical != 1 || record.Metrics.RiskWarning != 2 {
		t.Fatalf("unexpected risk metrics: %+v", record.Metrics)
	}
	if record.SchemaVersion != summarystore.SchemaVersion {
		t.Fatalf("schemaVersion = %q, want %q", record.SchemaVersion, summarystore.SchemaVersion)
	}
}

func TestBuildGitOpsSummaryRecord(t *testing.T) {
	now := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	summary := GitOpsSummary{
		Deployers: []DeployerStatus{
			{Ready: true, Stage: "healthy", SyncStatus: "Synced"},
			{Ready: false, Stage: "sync", SyncStatus: "OutOfSync"},
		},
		Sources: []SourceStatus{
			{Ready: false},
		},
	}

	record := buildGitOpsSummaryRecord(summary, "kind-dev", "prod", now)
	if record.Type != "gitops-status" {
		t.Fatalf("type = %q, want gitops-status", record.Type)
	}
	if record.Metrics.SyncTotal != 3 {
		t.Fatalf("syncTotal = %d, want 3", record.Metrics.SyncTotal)
	}
	if record.Metrics.SyncFailed != 2 {
		t.Fatalf("syncFailed = %d, want 2", record.Metrics.SyncFailed)
	}
	if record.Metrics.SyncOutOfSync != 1 {
		t.Fatalf("syncOutOfSync = %d, want 1", record.Metrics.SyncOutOfSync)
	}
	if record.Metrics.DriftTotal != 1 {
		t.Fatalf("driftTotal = %d, want 1", record.Metrics.DriftTotal)
	}
}

func TestRunSummaryList_JSONOutput(t *testing.T) {
	restoreFlags := withSummaryFlagsForTest()
	defer restoreFlags()

	now := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	prevNow := summaryNowFn
	summaryNowFn = func() time.Time { return now }
	defer func() { summaryNowFn = prevNow }()

	dir := t.TempDir()
	store, err := summarystore.New(summarystore.Options{
		RootDir:       dir,
		RetentionDays: 30,
		Now: func() time.Time {
			return now
		},
	})
	if err != nil {
		t.Fatalf("summarystore.New() error = %v", err)
	}
	if err := store.Write(summarystore.Record{
		Timestamp: now.Add(-2 * time.Hour),
		Type:      "scan",
		Cluster:   "kind-dev",
		Scope:     summarystore.Scope{Namespace: "prod"},
		Metrics:   summarystore.Metrics{RiskTotal: 1, RiskWarning: 1},
	}); err != nil {
		t.Fatalf("write record: %v", err)
	}
	if err := store.Write(summarystore.Record{
		Timestamp: now.Add(-30 * time.Hour),
		Type:      "scan",
		Cluster:   "kind-dev",
		Scope:     summarystore.Scope{Namespace: "prod"},
		Metrics:   summarystore.Metrics{RiskTotal: 5, RiskCritical: 2},
	}); err != nil {
		t.Fatalf("write old record: %v", err)
	}

	t.Setenv("CUB_SCOUT_SUMMARY_DIR", dir)
	summaryListSince = "24h"
	summaryListFormat = "json"

	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())

	out := captureStdout(t, func() {
		if err := runSummaryList(cmd, nil); err != nil {
			t.Fatalf("runSummaryList() error = %v", err)
		}
	})

	var payload summaryListResult
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode json output: %v\n%s", err, out)
	}
	if len(payload.Entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(payload.Entries))
	}
	if payload.Entries[0].Metrics.RiskTotal != 1 {
		t.Fatalf("riskTotal = %d, want 1", payload.Entries[0].Metrics.RiskTotal)
	}
}

func TestSummaryCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "summary" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("summary command is not registered on rootCmd")
	}
}

func withSummaryFlagsForTest() func() {
	prevFormat := summaryListFormat
	prevSince := summaryListSince
	prevType := summaryListType
	prevCluster := summaryListCluster
	prevNamespace := summaryListNamespace
	prevJSON := summaryListJSON

	summaryListFormat = "ascii"
	summaryListSince = "24h"
	summaryListType = ""
	summaryListCluster = ""
	summaryListNamespace = ""
	summaryListJSON = false

	return func() {
		summaryListFormat = prevFormat
		summaryListSince = prevSince
		summaryListType = prevType
		summaryListCluster = prevCluster
		summaryListNamespace = prevNamespace
		summaryListJSON = prevJSON
	}
}

func TestSummaryStoreDefaultPathUsesEnvOverride(t *testing.T) {
	override := filepath.Join(t.TempDir(), "summaries")
	t.Setenv("CUB_SCOUT_SUMMARY_DIR", override)
	path, err := defaultSummaryStoreDir()
	if err != nil {
		t.Fatalf("defaultSummaryStoreDir() error = %v", err)
	}
	if path != override {
		t.Fatalf("defaultSummaryStoreDir() = %q, want %q", path, override)
	}
}
