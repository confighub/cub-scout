// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestAuditCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "audit" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("audit command is not registered on rootCmd")
	}
}

func TestBuildAuditEntriesFromChangeSets_FiltersAndSorts(t *testing.T) {
	now := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-7 * 24 * time.Hour)

	raw := `[
	  {
	    "Slug": "CS-1002",
	    "CreatedAt": "2026-03-06T11:30:00Z",
	    "Description": "break-glass accept: approved by sre lead",
	    "CreatedBy": {"Slug":"alex"},
	    "Labels": {"break-glass":"true", "namespace":"prod"}
	  },
	  {
	    "Slug": "CS-1001",
	    "CreatedAt": "2026-03-05T08:20:00Z",
	    "Description": "Break-Glass decision: reject temporary hotfix",
	    "CreatedBy": "oncall@example.com",
	    "Labels": {"source":"cub-scout-import"}
	  },
	  {
	    "Slug": "CS-0998",
	    "CreatedAt": "2026-03-04T08:20:00Z",
	    "Description": "regular rollout",
	    "CreatedBy": "release-bot"
	  },
	  {
	    "Slug": "CS-0800",
	    "CreatedAt": "2026-01-01T08:20:00Z",
	    "Description": "break-glass accept: too old",
	    "CreatedBy": "ops"
	  }
	]`

	got, err := buildAuditEntriesFromChangeSets(raw, "prod", cutoff)
	if err != nil {
		t.Fatalf("buildAuditEntriesFromChangeSets() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("entry count = %d, want 1", len(got))
	}
	if got[0].ChangeSet != "CS-1002" {
		t.Fatalf("changeset = %q, want CS-1002", got[0].ChangeSet)
	}
	if got[0].Actor != "alex" {
		t.Fatalf("actor = %q, want alex", got[0].Actor)
	}
	if !strings.Contains(strings.ToLower(got[0].Reason), "approved") {
		t.Fatalf("reason = %q, want approved text", got[0].Reason)
	}
}

func TestBuildAuditEntriesFromChangeSets_ExcludesSyntheticByDefault(t *testing.T) {
	cutoff := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	raw := `[
	  {
	    "Slug": "CS-REAL-1",
	    "CreatedAt": "2026-03-06T10:00:00Z",
	    "Description": "break-glass accept: real",
	    "CreatedBy": "sre",
	    "Labels": {"break-glass":"true"}
	  },
	  {
	    "Slug": "CS-DEMO-1",
	    "CreatedAt": "2026-03-06T11:00:00Z",
	    "Description": "break-glass accept: demo seed",
	    "CreatedBy": "demo-bot",
	    "Labels": {"break-glass":"true", "synthetic":"true"}
	  }
	]`

	got, err := buildAuditEntriesFromChangeSets(raw, "", cutoff)
	if err != nil {
		t.Fatalf("buildAuditEntriesFromChangeSets() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("entry count = %d, want 1", len(got))
	}
	if got[0].ChangeSet != "CS-REAL-1" {
		t.Fatalf("changeset = %q, want CS-REAL-1", got[0].ChangeSet)
	}
}

func TestBuildAuditEntriesFromChangeSetsWithOptions_IncludesSynthetic(t *testing.T) {
	cutoff := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	raw := `[
	  {
	    "Slug": "CS-REAL-1",
	    "CreatedAt": "2026-03-06T10:00:00Z",
	    "Description": "break-glass accept: real",
	    "CreatedBy": "sre",
	    "Labels": {"break-glass":"true"}
	  },
	  {
	    "Slug": "CS-DEMO-1",
	    "CreatedAt": "2026-03-06T11:00:00Z",
	    "Description": "break-glass accept: demo seed",
	    "CreatedBy": "demo-bot",
	    "Labels": {"break-glass":"true", "source":"cub-scout-demo-seed"}
	  }
	]`

	got, err := buildAuditEntriesFromChangeSetsWithOptions(raw, "", cutoff, true)
	if err != nil {
		t.Fatalf("buildAuditEntriesFromChangeSetsWithOptions() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("entry count = %d, want 2", len(got))
	}
}

func TestRenderAuditASCII_Empty(t *testing.T) {
	out := renderAuditASCII(auditListResult{Since: "7d", Namespace: "prod"})
	if !strings.Contains(out, "No break-glass decisions") {
		t.Fatalf("expected empty message, got:\n%s", out)
	}
}

func TestRunAuditList_NotConnected(t *testing.T) {
	restore := withAuditListFlagsForTest()
	defer restore()

	prevRequire := requireAuditConnectedFn
	requireAuditConnectedFn = func() error { return errAuditDisconnected }
	defer func() { requireAuditConnectedFn = prevRequire }()

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	err := runAuditList(cmd, nil)
	if err == nil {
		t.Fatal("expected error when not connected")
	}
	if !strings.Contains(err.Error(), "audit requires ConfigHub connection") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunAuditList_JSONOutput(t *testing.T) {
	restore := withAuditListFlagsForTest()
	defer restore()

	prevRequire := requireAuditConnectedFn
	requireAuditConnectedFn = func() error { return nil }
	defer func() { requireAuditConnectedFn = prevRequire }()

	prevFetch := fetchAuditEntriesFn
	fetchAuditEntriesFn = func(ctx context.Context, q auditListQuery) ([]auditEntry, error) {
		return []auditEntry{
			{
				Timestamp: time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC),
				Actor:     "sre-lead",
				Reason:    "approved by SRE lead for Q1 migration",
				What:      "imported unit payments-api",
				ChangeSet: "CS-1234",
			},
		}, nil
	}
	defer func() { fetchAuditEntriesFn = prevFetch }()

	auditListFormat = "json"
	auditListSince = "7d"
	auditListNamespace = "prod"

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	out := captureStdout(t, func() {
		if err := runAuditList(cmd, nil); err != nil {
			t.Fatalf("runAuditList() error = %v", err)
		}
	})

	var payload auditListResult
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("json decode output: %v\n%s", err, out)
	}
	if payload.Namespace != "prod" {
		t.Fatalf("namespace = %q, want prod", payload.Namespace)
	}
	if len(payload.Entries) != 1 || payload.Entries[0].ChangeSet != "CS-1234" {
		t.Fatalf("unexpected entries: %+v", payload.Entries)
	}
}

func withAuditListFlagsForTest() func() {
	prevFormat := auditListFormat
	prevNamespace := auditListNamespace
	prevSince := auditListSince
	prevIncludeSynthetic := auditListIncludeSynthetic
	auditListFormat = "ascii"
	auditListNamespace = ""
	auditListSince = "7d"
	auditListIncludeSynthetic = false
	return func() {
		auditListFormat = prevFormat
		auditListNamespace = prevNamespace
		auditListSince = prevSince
		auditListIncludeSynthetic = prevIncludeSynthetic
	}
}
