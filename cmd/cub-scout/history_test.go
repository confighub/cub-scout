package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestHistoryCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "history" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("history command is not registered on rootCmd")
	}
}

func TestParseHistorySince(t *testing.T) {
	tests := []struct {
		raw     string
		want    time.Duration
		wantErr bool
	}{
		{raw: "24h", want: 24 * time.Hour},
		{raw: "7d", want: 7 * 24 * time.Hour},
		{raw: "2w", want: 14 * 24 * time.Hour},
		{raw: "0", wantErr: true},
		{raw: "garbage", wantErr: true},
	}

	for _, tt := range tests {
		got, err := parseHistorySince(tt.raw)
		if tt.wantErr {
			if err == nil {
				t.Fatalf("parseHistorySince(%q) expected error", tt.raw)
			}
			continue
		}
		if err != nil {
			t.Fatalf("parseHistorySince(%q) error = %v", tt.raw, err)
		}
		if got != tt.want {
			t.Fatalf("parseHistorySince(%q) = %v, want %v", tt.raw, got, tt.want)
		}
	}
}

func TestBuildHistoryEntriesFromChangeSets_FiltersAndSorts(t *testing.T) {
	now := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-7 * 24 * time.Hour)

	raw := `[
	  {
	    "Slug": "CS-4821",
	    "CreatedAt": "2026-03-03T14:22:00Z",
	    "Description": "image: v1.4.2 -> v1.4.3",
	    "CreatedBy": {"Slug":"ci-bot"}
	  },
	  {
	    "Slug": "CS-4701",
	    "CreatedAt": "2026-02-10T09:15:00Z",
	    "Description": "old change",
	    "CreatedBy": "alex"
	  },
	  {
	    "Slug": "CS-4799",
	    "CreatedAt": "2026-03-01T09:15:00Z",
	    "Description": "replicas: 2 -> 3",
	    "CreatedBy": {"Email":"sarah@example.com"}
	  }
	]`

	got, err := buildHistoryEntriesFromChangeSets(raw, cutoff)
	if err != nil {
		t.Fatalf("buildHistoryEntriesFromChangeSets() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("entry count = %d, want 2", len(got))
	}
	if got[0].ChangeSet != "CS-4821" || got[1].ChangeSet != "CS-4799" {
		t.Fatalf("unexpected ordering or changesets: %+v", got)
	}
	if got[0].Actor != "ci-bot" {
		t.Fatalf("entry[0] actor = %q, want ci-bot", got[0].Actor)
	}
	if got[1].Actor != "sarah@example.com" {
		t.Fatalf("entry[1] actor = %q, want sarah@example.com", got[1].Actor)
	}
}

func TestBuildHistoryEntriesFromChangeSets_ExcludesSyntheticByDefault(t *testing.T) {
	cutoff := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	raw := `[
	  {
	    "Slug": "CS-REAL-1",
	    "CreatedAt": "2026-03-06T10:00:00Z",
	    "Description": "real rollout",
	    "CreatedBy": "release-bot"
	  },
	  {
	    "Slug": "CS-DEMO-1",
	    "CreatedAt": "2026-03-06T11:00:00Z",
	    "Description": "demo seed",
	    "CreatedBy": "demo-bot",
	    "Labels": {"synthetic":"true", "source":"cub-scout-demo-seed"}
	  }
	]`

	got, err := buildHistoryEntriesFromChangeSets(raw, cutoff)
	if err != nil {
		t.Fatalf("buildHistoryEntriesFromChangeSets() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("entry count = %d, want 1", len(got))
	}
	if got[0].ChangeSet != "CS-REAL-1" {
		t.Fatalf("changeset = %q, want CS-REAL-1", got[0].ChangeSet)
	}
}

func TestBuildHistoryEntriesFromChangeSetsWithOptions_IncludesSynthetic(t *testing.T) {
	cutoff := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	raw := `[
	  {
	    "Slug": "CS-REAL-1",
	    "CreatedAt": "2026-03-06T10:00:00Z",
	    "Description": "real rollout",
	    "CreatedBy": "release-bot"
	  },
	  {
	    "Slug": "CS-DEMO-1",
	    "CreatedAt": "2026-03-06T11:00:00Z",
	    "Description": "demo seed",
	    "CreatedBy": "demo-bot",
	    "Labels": {"demo":"true"}
	  }
	]`

	got, err := buildHistoryEntriesFromChangeSetsWithOptions(raw, cutoff, true)
	if err != nil {
		t.Fatalf("buildHistoryEntriesFromChangeSetsWithOptions() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("entry count = %d, want 2", len(got))
	}
}

func TestRenderHistoryASCII_Empty(t *testing.T) {
	out := renderHistoryASCII(historyResult{
		Resource:  "deploy/my-app",
		Namespace: "prod",
		Since:     "7d",
		Entries:   nil,
	})
	if !strings.Contains(out, "No history available") {
		t.Fatalf("expected empty-history message, got:\n%s", out)
	}
}

func TestRunHistory_NotConnected(t *testing.T) {
	restoreFlags := withHistoryFlagsForTest()
	defer restoreFlags()

	prevRequire := requireHistoryConnectedFn
	requireHistoryConnectedFn = func() error {
		return errHistoryDisconnected
	}
	defer func() { requireHistoryConnectedFn = prevRequire }()

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	err := runHistory(cmd, []string{"deploy/my-app"})
	if err == nil {
		t.Fatal("expected error when not connected")
	}
	if !strings.Contains(err.Error(), "history requires ConfigHub connection") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunHistory_JSONOutput(t *testing.T) {
	restoreFlags := withHistoryFlagsForTest()
	defer restoreFlags()

	prevRequire := requireHistoryConnectedFn
	requireHistoryConnectedFn = func() error { return nil }
	defer func() { requireHistoryConnectedFn = prevRequire }()

	prevFetch := fetchHistoryEntriesFn
	fetchHistoryEntriesFn = func(ctx context.Context, q historyQuery) ([]historyEntry, error) {
		return []historyEntry{
			{
				Timestamp: time.Date(2026, 3, 3, 14, 22, 0, 0, time.UTC),
				Actor:     "ci-bot",
				Change:    "image: v1.4.2 -> v1.4.3",
				ChangeSet: "CS-4821",
			},
		}, nil
	}
	defer func() { fetchHistoryEntriesFn = prevFetch }()

	prevNav := resolveHistoryNavigationFn
	resolveHistoryNavigationFn = func(ctx context.Context, q historyQuery) historyNavigation {
		return historyNavigation{
			ConfigHubURL:          "https://confighub.com/units/sp-123/u-123",
			ConfigHubRevisionsURL: "https://confighub.com/units/sp-123/u-123?tab=2",
			NextSteps: []StructuredHint{
				{
					ActionType:  ActionReadOnly,
					Reason:      "Review revision drift before treating this history as converged.",
					NextSurface: "https://confighub.com/units/sp-123/u-123?tab=2",
				},
			},
		}
	}
	defer func() { resolveHistoryNavigationFn = prevNav }()

	historyFormat = "json"
	historySince = "7d"
	historyNamespace = "prod"

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	out := captureStdout(t, func() {
		if err := runHistory(cmd, []string{"deploy/my-app"}); err != nil {
			t.Fatalf("runHistory() error = %v", err)
		}
	})

	var payload historyResult
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("json decode output: %v\n%s", err, out)
	}
	if payload.Resource != "deploy/my-app" {
		t.Fatalf("resource = %q, want deploy/my-app", payload.Resource)
	}
	if payload.Namespace != "prod" {
		t.Fatalf("namespace = %q, want prod", payload.Namespace)
	}
	if payload.ConfigHubURL != "https://confighub.com/units/sp-123/u-123" {
		t.Fatalf("confighubUrl = %q, want exact unit url", payload.ConfigHubURL)
	}
	if payload.ConfigHubRevisionsURL != "https://confighub.com/units/sp-123/u-123?tab=2" {
		t.Fatalf("confighubRevisionsUrl = %q, want exact revisions url", payload.ConfigHubRevisionsURL)
	}
	if len(payload.Entries) != 1 || payload.Entries[0].ChangeSet != "CS-4821" {
		t.Fatalf("unexpected entries: %+v", payload.Entries)
	}
	if len(payload.NextSteps) != 1 {
		t.Fatalf("nextSteps count = %d, want 1", len(payload.NextSteps))
	}
	if payload.NextSteps[0].NextSurface != "https://confighub.com/units/sp-123/u-123?tab=2" {
		t.Fatalf("next surface = %q, want exact revisions url", payload.NextSteps[0].NextSurface)
	}
}

func TestBuildHistoryNavigation_UsesChangeSetAndUnitTrustSurface(t *testing.T) {
	raw := `[
	  {
	    "Space": {"Slug":"payments","SpaceID":"sp-123"},
	    "Unit": {
	      "Slug":"payments-api",
	      "UnitID":"u-123",
	      "HeadRevisionNum":9,
	      "LiveRevisionNum":7
	    },
	    "ChangeSet": {"Slug":"release-42","ID":"cs-123"},
	    "CreatedAt": "2026-03-03T14:22:00Z",
	    "Description": "image: v1.4.2 -> v1.4.3",
	    "CreatedBy": {"Slug":"ci-bot"}
	  }
	]`

	nav := buildHistoryNavigation(raw)
	if nav.ConfigHubURL != "https://confighub.com/units/sp-123/u-123" {
		t.Fatalf("confighubUrl = %q, want exact unit url", nav.ConfigHubURL)
	}
	if nav.ConfigHubRevisionsURL != "https://confighub.com/units/sp-123/u-123?tab=2" {
		t.Fatalf("confighubRevisionsUrl = %q, want exact revisions url", nav.ConfigHubRevisionsURL)
	}
	if len(nav.NextSteps) < 3 {
		t.Fatalf("nextSteps count = %d, want at least 3", len(nav.NextSteps))
	}
	if nav.NextSteps[0].NextCommand != "cub changeset get release-42 --json --space payments" {
		t.Fatalf("first next command = %q, want exact changeset get command", nav.NextSteps[0].NextCommand)
	}
	if nav.NextSteps[1].NextSurface != "https://confighub.com/units/sp-123/u-123?tab=2" {
		t.Fatalf("second next surface = %q, want exact revisions url", nav.NextSteps[1].NextSurface)
	}
	if !strings.Contains(nav.NextSteps[1].Reason, "Head revision 9 is ahead of live revision 7") {
		t.Fatalf("second next reason = %q, want revision-drift rationale", nav.NextSteps[1].Reason)
	}
	if nav.NextSteps[2].NextSurface != "https://confighub.com/units/sp-123/u-123" {
		t.Fatalf("third next surface = %q, want exact unit url", nav.NextSteps[2].NextSurface)
	}
}

func withHistoryFlagsForTest() func() {
	prevFormat := historyFormat
	prevNamespace := historyNamespace
	prevSince := historySince
	prevIncludeSynthetic := historyIncludeSynthetic
	historyFormat = "ascii"
	historyNamespace = ""
	historySince = "7d"
	historyIncludeSynthetic = false
	return func() {
		historyFormat = prevFormat
		historyNamespace = prevNamespace
		historySince = prevSince
		historyIncludeSynthetic = prevIncludeSynthetic
	}
}

func TestRunHistory_UsesFixtureJSONWithoutConnection(t *testing.T) {
	restoreFlags := withHistoryFlagsForTest()
	defer restoreFlags()

	prevRequire := requireHistoryConnectedFn
	requireHistoryConnectedFn = func() error { return errHistoryDisconnected }
	defer func() { requireHistoryConnectedFn = prevRequire }()

	prevFetch := fetchHistoryEntriesFn
	fetchHistoryEntriesFn = func(ctx context.Context, q historyQuery) ([]historyEntry, error) {
		t.Fatalf("fetchHistoryEntriesFn should not be called in fixture mode")
		return nil, nil
	}
	defer func() { fetchHistoryEntriesFn = prevFetch }()

	prevNow := historyNowFn
	historyNowFn = func() time.Time { return time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC) }
	defer func() { historyNowFn = prevNow }()

	fixturePath := filepath.Join(t.TempDir(), "changesets.json")
	fixtureJSON := `[
	  {
	    "Slug": "CS-9901",
	    "CreatedAt": "2026-03-05T09:20:00Z",
	    "Description": "image: v2.3.1 -> v2.3.2",
	    "CreatedBy": {"Slug":"release-bot"}
	  }
	]`
	if err := os.WriteFile(fixturePath, []byte(fixtureJSON), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	t.Setenv("CUB_SCOUT_TEST_HISTORY_JSON", fixturePath)

	historyFormat = "json"
	historySince = "7d"
	historyNamespace = "prod"

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	out := captureStdout(t, func() {
		if err := runHistory(cmd, []string{"deploy/checkout"}); err != nil {
			t.Fatalf("runHistory() error = %v", err)
		}
	})

	var payload historyResult
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("json decode output: %v\n%s", err, out)
	}
	if len(payload.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(payload.Entries))
	}
	if payload.Entries[0].ChangeSet != "CS-9901" {
		t.Fatalf("changeset = %q, want CS-9901", payload.Entries[0].ChangeSet)
	}
}

func TestRunHistory_FixtureReadError(t *testing.T) {
	restoreFlags := withHistoryFlagsForTest()
	defer restoreFlags()

	prevRequire := requireHistoryConnectedFn
	requireHistoryConnectedFn = func() error { return nil }
	defer func() { requireHistoryConnectedFn = prevRequire }()

	prevFetch := fetchHistoryEntriesFn
	fetchHistoryEntriesFn = func(ctx context.Context, q historyQuery) ([]historyEntry, error) {
		return []historyEntry{}, nil
	}
	defer func() { fetchHistoryEntriesFn = prevFetch }()

	t.Setenv("CUB_SCOUT_TEST_HISTORY_JSON", filepath.Join(t.TempDir(), "missing.json"))

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	err := runHistory(cmd, []string{"deploy/checkout"})
	if err == nil {
		t.Fatal("expected error for missing fixture file")
	}
	if !strings.Contains(err.Error(), "read history fixture") {
		t.Fatalf("unexpected error: %v", err)
	}
}
