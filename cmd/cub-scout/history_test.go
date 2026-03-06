package main

import (
	"context"
	"encoding/json"
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
	if len(payload.Entries) != 1 || payload.Entries[0].ChangeSet != "CS-4821" {
		t.Fatalf("unexpected entries: %+v", payload.Entries)
	}
}

func withHistoryFlagsForTest() func() {
	prevFormat := historyFormat
	prevNamespace := historyNamespace
	prevSince := historySince
	historyFormat = "ascii"
	historyNamespace = ""
	historySince = "7d"
	return func() {
		historyFormat = prevFormat
		historyNamespace = prevNamespace
		historySince = prevSince
	}
}
