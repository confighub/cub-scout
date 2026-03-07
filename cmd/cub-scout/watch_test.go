// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

func TestBuildWatchEvents_DiscoveredAndOwnershipChanged(t *testing.T) {
	prev := watchState{
		entriesByID: map[string]MapEntry{
			"id-1": {
				ID:        "id-1",
				Namespace: "prod",
				Kind:      "Deployment",
				Name:      "api",
				Owner:     "Flux",
			},
		},
		findings: map[string]watchFinding{},
	}
	curr := watchState{
		entriesByID: map[string]MapEntry{
			"id-1": {
				ID:        "id-1",
				Namespace: "prod",
				Kind:      "Deployment",
				Name:      "api",
				Owner:     "ArgoCD",
			},
			"id-2": {
				ID:        "id-2",
				Namespace: "prod",
				Kind:      "Service",
				Name:      "api",
				Owner:     "Internal Platform",
			},
		},
		findings: map[string]watchFinding{},
	}

	events := buildWatchEvents(prev, curr, nil, "", func() time.Time {
		return time.Date(2026, 3, 7, 11, 0, 0, 0, time.UTC)
	})
	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2", len(events))
	}

	types := map[string]bool{}
	for _, e := range events {
		types[e.Type] = true
	}
	if !types["resource.discovered"] {
		t.Fatalf("missing resource.discovered event: %#v", events)
	}
	if !types["ownership.changed"] {
		t.Fatalf("missing ownership.changed event: %#v", events)
	}
}

func TestBuildWatchEvents_FindingFilters(t *testing.T) {
	prev := watchState{
		entriesByID: map[string]MapEntry{
			"id-1": {
				ID:        "id-1",
				Namespace: "prod",
				Kind:      "Deployment",
				Name:      "api",
				Owner:     "Flux",
			},
		},
		findings: map[string]watchFinding{},
	}
	curr := watchState{
		entriesByID: prev.entriesByID,
		findings: map[string]watchFinding{
			"f1": {
				Key:       "f1",
				Category:  "STATE",
				Severity:  "warning",
				Kind:      "Deployment",
				Name:      "api",
				Namespace: "prod",
				Message:   "out of sync",
			},
			"f2": {
				Key:       "f2",
				Category:  "SILENT",
				Severity:  "info",
				Kind:      "Deployment",
				Name:      "api",
				Namespace: "prod",
				Message:   "minor",
			},
		},
	}

	events := buildWatchEvents(prev, curr, map[string]struct{}{"warning": {}}, "Flux", func() time.Time {
		return time.Date(2026, 3, 7, 11, 1, 0, 0, time.UTC)
	})

	// warning STATE finding should emit both scan.finding and drift.detected
	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2", len(events))
	}
	for _, e := range events {
		if e.Severity != "warning" {
			t.Fatalf("unexpected severity in filtered events: %#v", events)
		}
		if e.Type != "scan.finding" && e.Type != "drift.detected" {
			t.Fatalf("unexpected event type %q", e.Type)
		}
	}
}

func TestAppendWatchQueue_DropsOldest(t *testing.T) {
	queue := []watchEvent{{Type: "e1"}, {Type: "e2"}}
	queue = appendWatchQueue(queue, []watchEvent{{Type: "e3"}, {Type: "e4"}}, 3)

	if len(queue) != 3 {
		t.Fatalf("queue len = %d, want 3", len(queue))
	}
	if queue[0].Type != "e2" || queue[2].Type != "e4" {
		t.Fatalf("unexpected queue contents: %#v", queue)
	}
}

func TestFlushWatchQueue_PartialFailureKeepsRemaining(t *testing.T) {
	restore := overrideWatchDeps(t)
	defer restore()

	calls := 0
	watchPostEvent = func(ctx context.Context, webhookURL string, event watchEvent) error {
		calls++
		if calls == 2 {
			return errors.New("boom")
		}
		return nil
	}

	queue := []watchEvent{{Type: "e1"}, {Type: "e2"}, {Type: "e3"}}
	remaining, err := flushWatchQueue(context.Background(), "https://hooks.example.com/cub-scout", queue)
	if err == nil {
		t.Fatal("expected flush error, got nil")
	}
	if calls != 2 {
		t.Fatalf("post call count = %d, want 2", calls)
	}
	if len(remaining) != 2 || remaining[0].Type != "e2" || remaining[1].Type != "e3" {
		t.Fatalf("unexpected remaining queue: %#v", remaining)
	}
}

func TestPostWatchEvent_Retries(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("retry"))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	oldClient := watchDefaultClient
	oldSleep := watchSleepWithCtx
	watchDefaultClient = srv.Client()
	watchSleepWithCtx = func(ctx context.Context, d time.Duration) error { return nil }
	defer func() {
		watchDefaultClient = oldClient
		watchSleepWithCtx = oldSleep
	}()

	err := postWatchEvent(context.Background(), srv.URL, watchEvent{
		Type:      "scan.finding",
		Timestamp: time.Now().UTC(),
		Resource:  watchEventResource{Kind: "Deployment", Name: "api", Namespace: "prod"},
	})
	if err != nil {
		t.Fatalf("postWatchEvent() error = %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("webhook call count = %d, want 3", got)
	}
}

func TestRunWatch_OncePostsEvents(t *testing.T) {
	restore := overrideWatchDeps(t)
	defer restore()

	posted := []watchEvent{}
	watchBuildConfig = func() (*rest.Config, error) {
		return &rest.Config{Host: "https://example.invalid"}, nil
	}
	watchCollectState = func(ctx context.Context, dynClient dynamic.Interface, namespace string) (watchState, error) {
		return watchState{
			entriesByID: map[string]MapEntry{
				"id-1": {
					ID:        "id-1",
					Namespace: "prod",
					Kind:      "Deployment",
					Name:      "api",
					Owner:     "Flux",
				},
			},
			findings: map[string]watchFinding{
				"f1": {
					Key:       "f1",
					Category:  "STATE",
					Severity:  "warning",
					Kind:      "Deployment",
					Name:      "api",
					Namespace: "prod",
					Message:   "out of sync",
				},
			},
		}, nil
	}
	watchPostEvent = func(ctx context.Context, webhookURL string, event watchEvent) error {
		posted = append(posted, event)
		return nil
	}

	watchWebhookURL = "https://hooks.example.com/cub-scout"
	watchInterval = 1 * time.Second
	watchNamespace = "prod"
	watchOwner = ""
	watchSeverity = ""
	watchOnce = true
	watchMaxQueuedEvents = 100

	watchCmd.SetContext(context.Background())

	if err := runWatch(watchCmd, nil); err != nil {
		t.Fatalf("runWatch() error = %v", err)
	}
	if len(posted) == 0 {
		t.Fatal("expected posted events, got none")
	}
}

func overrideWatchDeps(t *testing.T) func() {
	t.Helper()
	oldBuildConfig := watchBuildConfig
	oldCollectState := watchCollectState
	oldPostEvent := watchPostEvent
	oldNow := watchEventNow
	oldSleep := watchSleepWithCtx
	oldClient := watchDefaultClient

	return func() {
		watchBuildConfig = oldBuildConfig
		watchCollectState = oldCollectState
		watchPostEvent = oldPostEvent
		watchEventNow = oldNow
		watchSleepWithCtx = oldSleep
		watchDefaultClient = oldClient
	}
}
