package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/confighub/cub-scout/internal/summarystore"
	"github.com/spf13/cobra"
)

func TestBuildSlackDigest(t *testing.T) {
	now := time.Date(2026, 3, 7, 11, 0, 0, 0, time.UTC)
	entries := []summarystore.Record{
		{
			Timestamp: now.Add(-20 * time.Minute),
			Type:      "scan",
			Cluster:   "kind-dev",
			Scope:     summarystore.Scope{Namespace: "prod"},
			Metrics: summarystore.Metrics{
				RiskTotal:    4,
				RiskCritical: 1,
				RiskWarning:  2,
				RiskInfo:     1,
			},
		},
		{
			Timestamp: now.Add(-15 * time.Minute),
			Type:      "gitops-status",
			Cluster:   "kind-dev",
			Scope:     summarystore.Scope{Namespace: "prod"},
			Metrics: summarystore.Metrics{
				SyncFailed:    2,
				SyncOutOfSync: 1,
				DriftTotal:    1,
			},
		},
	}

	digest := buildSlackDigest(entries, summarySlackDigestOptions{
		Since:     "24h",
		Cluster:   "kind-dev",
		Namespace: "prod",
		BatchSize: 10,
	})

	if digest.Count != 2 {
		t.Fatalf("count = %d, want 2", digest.Count)
	}
	if digest.Risk.Critical != 1 || digest.Risk.Warning != 2 || digest.Risk.Info != 1 {
		t.Fatalf("unexpected risk totals: %+v", digest.Risk)
	}
	if digest.Sync.Failed != 2 || digest.Sync.OutOfSync != 1 {
		t.Fatalf("unexpected sync totals: %+v", digest.Sync)
	}
	if digest.Drift.Total != 1 {
		t.Fatalf("drift total = %d, want 1", digest.Drift.Total)
	}
	if !strings.Contains(digest.NextAction, "cub-scout summary list") {
		t.Fatalf("next action missing summary command: %q", digest.NextAction)
	}
}

func TestRenderSlackPayload(t *testing.T) {
	digest := summarySlackDigest{
		Since: "24h",
		Count: 3,
		Risk: summarySlackRiskTotals{
			Critical: 1,
			Warning:  1,
			Info:     1,
		},
		Sync: summarySlackSyncTotals{
			Failed:    2,
			OutOfSync: 1,
		},
		Drift:    summarySlackDriftTotals{Total: 1},
		Clusters: []string{"kind-dev"},
		Scopes:   []string{"prod"},
		Top: []summarySlackTopEntry{
			{When: "2026-03-07T10:40:00Z", Type: "scan", Cluster: "kind-dev", Namespace: "prod", Summary: "risk C/W/I 1/1/1"},
		},
		NextAction: "cub-scout summary list --since 24h",
	}

	payload := buildSlackPayload(digest)
	if !strings.Contains(payload.Text, "Connected digest") {
		t.Fatalf("payload text missing heading: %q", payload.Text)
	}
	if len(payload.Blocks) == 0 {
		t.Fatal("expected Slack blocks")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if !strings.Contains(string(body), "cub-scout summary list --since 24h") {
		t.Fatalf("payload missing next action command: %s", body)
	}
}

func TestRunSummarySlack_PostsToWebhook(t *testing.T) {
	restoreFlags := withSummarySlackFlagsForTest()
	defer restoreFlags()

	now := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	prevNow := summaryNowFn
	summaryNowFn = func() time.Time { return now }
	defer func() { summaryNowFn = prevNow }()

	dir := t.TempDir()
	t.Setenv("CUB_SCOUT_SUMMARY_DIR", dir)
	store, err := summarystore.New(summarystore.Options{RootDir: dir, RetentionDays: 30, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("summarystore.New() error = %v", err)
	}
	if err := store.Write(summarystore.Record{
		Timestamp: now.Add(-30 * time.Minute),
		Type:      "scan",
		Cluster:   "kind-dev",
		Scope:     summarystore.Scope{Namespace: "prod"},
		Metrics:   summarystore.Metrics{RiskTotal: 2, RiskCritical: 1, RiskWarning: 1},
	}); err != nil {
		t.Fatalf("store.Write() error = %v", err)
	}

	var (
		mu       sync.Mutex
		requests [][]byte
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode webhook payload: %v", err)
		}
		raw, _ := json.Marshal(payload)
		mu.Lock()
		requests = append(requests, raw)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	summarySlackWebhookURL = server.URL
	summarySlackSince = "24h"
	summarySlackBatchSize = 5
	summarySlackForce = true

	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	if err := runSummarySlack(cmd, nil); err != nil {
		t.Fatalf("runSummarySlack() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 1 {
		t.Fatalf("webhook request count = %d, want 1", len(requests))
	}
	if !strings.Contains(string(requests[0]), "risk") {
		t.Fatalf("payload missing risk fields: %s", requests[0])
	}
}

func TestRunSummarySlack_DedupeSkipsWithinWindow(t *testing.T) {
	restoreFlags := withSummarySlackFlagsForTest()
	defer restoreFlags()

	now := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	prevNow := summaryNowFn
	summaryNowFn = func() time.Time { return now }
	defer func() { summaryNowFn = prevNow }()

	dir := t.TempDir()
	t.Setenv("CUB_SCOUT_SUMMARY_DIR", dir)
	store, err := summarystore.New(summarystore.Options{RootDir: dir, RetentionDays: 30, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("summarystore.New() error = %v", err)
	}
	if err := store.Write(summarystore.Record{
		Timestamp: now.Add(-10 * time.Minute),
		Type:      "scan",
		Cluster:   "kind-dev",
		Scope:     summarystore.Scope{Namespace: "prod"},
		Metrics:   summarystore.Metrics{RiskTotal: 1, RiskWarning: 1},
	}); err != nil {
		t.Fatalf("store.Write() error = %v", err)
	}

	var (
		mu    sync.Mutex
		count int
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		count++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	summarySlackWebhookURL = server.URL
	summarySlackSince = "24h"
	summarySlackBatchSize = 5
	summarySlackDedupeWindow = "2h"
	summarySlackForce = false

	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	if err := runSummarySlack(cmd, nil); err != nil {
		t.Fatalf("first runSummarySlack() error = %v", err)
	}
	if err := runSummarySlack(cmd, nil); err != nil {
		t.Fatalf("second runSummarySlack() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if count != 1 {
		t.Fatalf("webhook post count = %d, want 1 (dedupe skip)", count)
	}
}

func TestRunSummarySlack_BatchSizeLimitsEntries(t *testing.T) {
	now := time.Date(2026, 3, 7, 11, 0, 0, 0, time.UTC)
	entries := []summarystore.Record{
		{Timestamp: now.Add(-1 * time.Minute), Type: "scan", Cluster: "a", Scope: summarystore.Scope{Namespace: "prod"}, Metrics: summarystore.Metrics{RiskTotal: 1}},
		{Timestamp: now.Add(-2 * time.Minute), Type: "scan", Cluster: "a", Scope: summarystore.Scope{Namespace: "prod"}, Metrics: summarystore.Metrics{RiskTotal: 1}},
		{Timestamp: now.Add(-3 * time.Minute), Type: "scan", Cluster: "a", Scope: summarystore.Scope{Namespace: "prod"}, Metrics: summarystore.Metrics{RiskTotal: 1}},
	}

	digest := buildSlackDigest(entries, summarySlackDigestOptions{Since: "24h", BatchSize: 2})
	if len(digest.Top) != 2 {
		t.Fatalf("top entries len = %d, want 2", len(digest.Top))
	}
}

func TestSummarySlackCommandRegistered(t *testing.T) {
	summary := findRootCommand(t, "summary")
	if summary == nil {
		t.Fatal("summary command not found")
	}
	for _, sub := range summary.Commands() {
		if sub.Name() == "slack" {
			return
		}
	}
	t.Fatal("summary slack subcommand not found")
}

func withSummarySlackFlagsForTest() func() {
	prevWebhook := summarySlackWebhookURL
	prevSince := summarySlackSince
	prevType := summarySlackType
	prevCluster := summarySlackCluster
	prevNamespace := summarySlackNamespace
	prevBatch := summarySlackBatchSize
	prevDedupe := summarySlackDedupeWindow
	prevForce := summarySlackForce
	prevDryRun := summarySlackDryRun

	summarySlackWebhookURL = ""
	summarySlackSince = "24h"
	summarySlackType = ""
	summarySlackCluster = ""
	summarySlackNamespace = ""
	summarySlackBatchSize = 10
	summarySlackDedupeWindow = "30m"
	summarySlackForce = false
	summarySlackDryRun = false

	return func() {
		summarySlackWebhookURL = prevWebhook
		summarySlackSince = prevSince
		summarySlackType = prevType
		summarySlackCluster = prevCluster
		summarySlackNamespace = prevNamespace
		summarySlackBatchSize = prevBatch
		summarySlackDedupeWindow = prevDedupe
		summarySlackForce = prevForce
		summarySlackDryRun = prevDryRun
	}
}

func findRootCommand(t *testing.T, name string) *cobra.Command {
	t.Helper()
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == name {
			return cmd
		}
	}
	return nil
}
