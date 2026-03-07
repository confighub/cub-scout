package summarystore

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreWriteListRoundTrip(t *testing.T) {
	now := time.Date(2026, 3, 7, 10, 30, 0, 0, time.UTC)
	store, err := New(Options{
		RootDir:       t.TempDir(),
		RetentionDays: 30,
		Now: func() time.Time {
			return now
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	older := Record{
		Timestamp: now.Add(-30 * time.Hour),
		Type:      "scan",
		Cluster:   "kind-dev",
		Scope: Scope{
			Namespace: "prod",
		},
		Metrics: Metrics{
			RiskTotal:    3,
			RiskCritical: 1,
			RiskWarning:  1,
			RiskInfo:     1,
		},
	}
	newer := Record{
		Timestamp: now.Add(-2 * time.Hour),
		Type:      "gitops-status",
		Cluster:   "kind-dev",
		Scope: Scope{
			Namespace: "prod",
		},
		Metrics: Metrics{
			SyncTotal:     5,
			SyncFailed:    1,
			SyncOutOfSync: 1,
			DriftTotal:    1,
		},
	}

	if err := store.Write(older); err != nil {
		t.Fatalf("Write(older) error = %v", err)
	}
	if err := store.Write(newer); err != nil {
		t.Fatalf("Write(newer) error = %v", err)
	}

	got, err := store.List(Query{Since: now.Add(-24 * time.Hour)})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("List len = %d, want 1", len(got))
	}
	if got[0].Type != "gitops-status" {
		t.Fatalf("record type = %q, want gitops-status", got[0].Type)
	}
	if got[0].SchemaVersion != SchemaVersion {
		t.Fatalf("schema version = %q, want %q", got[0].SchemaVersion, SchemaVersion)
	}
	if got[0].Metrics.SyncOutOfSync != 1 {
		t.Fatalf("syncOutOfSync = %d, want 1", got[0].Metrics.SyncOutOfSync)
	}
}

func TestStoreListFilters(t *testing.T) {
	now := time.Date(2026, 3, 7, 10, 30, 0, 0, time.UTC)
	store, err := New(Options{
		RootDir:       t.TempDir(),
		RetentionDays: 30,
		Now: func() time.Time {
			return now
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	records := []Record{
		{
			Timestamp: now.Add(-2 * time.Hour),
			Type:      "scan",
			Cluster:   "cluster-a",
			Scope:     Scope{Namespace: "prod"},
			Metrics:   Metrics{RiskTotal: 2},
		},
		{
			Timestamp: now.Add(-90 * time.Minute),
			Type:      "scan",
			Cluster:   "cluster-b",
			Scope:     Scope{Namespace: "prod"},
			Metrics:   Metrics{RiskTotal: 1},
		},
		{
			Timestamp: now.Add(-1 * time.Hour),
			Type:      "gitops-status",
			Cluster:   "cluster-a",
			Scope:     Scope{Namespace: "dev"},
			Metrics:   Metrics{SyncOutOfSync: 1},
		},
	}
	for _, record := range records {
		if err := store.Write(record); err != nil {
			t.Fatalf("Write(%s/%s) error = %v", record.Cluster, record.Type, err)
		}
	}

	got, err := store.List(Query{
		Since:     now.Add(-24 * time.Hour),
		Type:      "scan",
		Cluster:   "cluster-a",
		Namespace: "prod",
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("List len = %d, want 1", len(got))
	}
	if got[0].Cluster != "cluster-a" || got[0].Type != "scan" || got[0].Scope.Namespace != "prod" {
		t.Fatalf("unexpected record: %+v", got[0])
	}
}

func TestStoreRetentionPrunesOldRecords(t *testing.T) {
	now := time.Date(2026, 3, 7, 10, 30, 0, 0, time.UTC)
	root := t.TempDir()
	store, err := New(Options{
		RootDir:       root,
		RetentionDays: 1,
		Now: func() time.Time {
			return now
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	oldRecord := Record{
		Timestamp: now.Add(-72 * time.Hour),
		Type:      "scan",
		Cluster:   "kind-dev",
		Scope:     Scope{Namespace: "prod"},
		Metrics:   Metrics{RiskTotal: 7},
	}
	newRecord := Record{
		Timestamp: now.Add(-1 * time.Hour),
		Type:      "scan",
		Cluster:   "kind-dev",
		Scope:     Scope{Namespace: "prod"},
		Metrics:   Metrics{RiskTotal: 1},
	}

	if err := store.Write(oldRecord); err != nil {
		t.Fatalf("Write(oldRecord) error = %v", err)
	}
	if err := store.Write(newRecord); err != nil {
		t.Fatalf("Write(newRecord) error = %v", err)
	}

	got, err := store.List(Query{Since: now.Add(-7 * 24 * time.Hour)})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("List len = %d, want 1", len(got))
	}
	if !got[0].Timestamp.Equal(newRecord.Timestamp) {
		t.Fatalf("got timestamp = %s, want %s", got[0].Timestamp, newRecord.Timestamp)
	}

	// Ensure old day file is physically pruned.
	oldFile := filepath.Join(root, "kind-dev", "scan", oldRecord.Timestamp.UTC().Format("2006-01-02")+".jsonl")
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatalf("expected old file to be pruned, stat err = %v", err)
	}
}

func TestDefaultRootDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := DefaultRootDir()
	if err != nil {
		t.Fatalf("DefaultRootDir() error = %v", err)
	}

	want := filepath.Join(home, ".confighub", "cub-scout", "summaries")
	if got != want {
		t.Fatalf("DefaultRootDir() = %q, want %q", got, want)
	}
}
