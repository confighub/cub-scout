// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestRunWatch_RequiresDestination(t *testing.T) {
	restore := overrideWatchDeps(t)
	defer restore()

	watchWebhookURL = ""
	watchOutputFile = ""
	watchInterval = 1 * time.Second
	watchMaxQueuedEvents = 100
	watchCmd.SetContext(context.Background())

	err := runWatch(watchCmd, nil)
	if err == nil {
		t.Fatal("expected destination validation error, got nil")
	}
	if !strings.Contains(err.Error(), "either --webhook or --output-file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildWatchSinks_FileOnly(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "events.jsonl")
	sinks, cleanup, err := buildWatchSinks("", outputPath)
	if err != nil {
		t.Fatalf("buildWatchSinks() error = %v", err)
	}
	defer cleanup()

	if len(sinks) != 1 {
		t.Fatalf("sink count = %d, want 1", len(sinks))
	}

	event := watchEvent{
		Type:      "scan.finding",
		Timestamp: time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC),
		Resource:  watchEventResource{Kind: "Deployment", Name: "api", Namespace: "prod"},
	}

	if err := sinks[0].Write(context.Background(), event); err != nil {
		t.Fatalf("sink write failed: %v", err)
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 {
		t.Fatalf("line count = %d, want 1", len(lines))
	}

	var decoded watchEvent
	if err := json.Unmarshal([]byte(lines[0]), &decoded); err != nil {
		t.Fatalf("decode event json: %v", err)
	}
	if decoded.Type != "scan.finding" {
		t.Fatalf("decoded type = %q, want scan.finding", decoded.Type)
	}
}

func TestBuildWatchSinks_WebhookAndFile(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	oldClient := watchDefaultClient
	watchDefaultClient = srv.Client()
	defer func() { watchDefaultClient = oldClient }()

	outputPath := filepath.Join(t.TempDir(), "events.jsonl")
	sinks, cleanup, err := buildWatchSinks(srv.URL, outputPath)
	if err != nil {
		t.Fatalf("buildWatchSinks() error = %v", err)
	}
	defer cleanup()

	if len(sinks) != 2 {
		t.Fatalf("sink count = %d, want 2", len(sinks))
	}

	queue := []watchEvent{
		{
			Type:      "resource.discovered",
			Timestamp: time.Date(2026, 3, 7, 18, 5, 0, 0, time.UTC),
			Resource:  watchEventResource{Kind: "Service", Name: "api", Namespace: "prod"},
		},
	}
	remaining, err := flushWatchQueue(context.Background(), sinks, queue)
	if err != nil {
		t.Fatalf("flushWatchQueue() error = %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("remaining len = %d, want 0", len(remaining))
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("webhook calls = %d, want 1", got)
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if !strings.Contains(string(raw), "\"resource.discovered\"") {
		t.Fatalf("expected event JSON in output file, got %q", string(raw))
	}
}

func TestFlushWatchQueue_WithFileSink(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "events.jsonl")
	sinks, cleanup, err := buildWatchSinks("", outputPath)
	if err != nil {
		t.Fatalf("buildWatchSinks() error = %v", err)
	}
	defer cleanup()

	queue := []watchEvent{
		{
			Type:      "resource.discovered",
			Timestamp: time.Date(2026, 3, 7, 18, 10, 0, 0, time.UTC),
			Resource:  watchEventResource{Kind: "Service", Name: "api", Namespace: "prod"},
		},
		{
			Type:      "scan.finding",
			Timestamp: time.Date(2026, 3, 7, 18, 10, 30, 0, time.UTC),
			Resource:  watchEventResource{Kind: "Deployment", Name: "api", Namespace: "prod"},
		},
	}

	remaining, err := flushWatchQueue(context.Background(), sinks, queue)
	if err != nil {
		t.Fatalf("flushWatchQueue() error = %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("remaining len = %d, want 0", len(remaining))
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("line count = %d, want 2", len(lines))
	}
}

type testWatchSink struct {
	calls     int
	failAt    int
	retryable bool
}

func (s *testWatchSink) Name() string { return "test" }
func (s *testWatchSink) Retryable() bool {
	return s.retryable
}

func (s *testWatchSink) Write(ctx context.Context, event watchEvent) error {
	s.calls++
	if s.failAt > 0 && s.calls == s.failAt {
		return errors.New("boom")
	}
	return nil
}

func TestFlushWatchQueue_PartialFailureKeepsRemaining(t *testing.T) {
	sink := &testWatchSink{failAt: 2, retryable: true}
	queue := []watchEvent{{Type: "e1"}, {Type: "e2"}, {Type: "e3"}}
	remaining, err := flushWatchQueue(context.Background(), []watchEventSink{sink}, queue)
	if err == nil {
		t.Fatal("expected flush error, got nil")
	}
	if sink.calls != 2 {
		t.Fatalf("sink call count = %d, want 2", sink.calls)
	}
	if len(remaining) != 2 || remaining[0].Type != "e2" || remaining[1].Type != "e3" {
		t.Fatalf("unexpected remaining queue: %#v", remaining)
	}
}

func TestFlushWatchQueue_NonRetryableFailureDropsQueue(t *testing.T) {
	sink := &testWatchSink{failAt: 1, retryable: false}
	queue := []watchEvent{{Type: "e1"}, {Type: "e2"}}

	remaining, err := flushWatchQueue(context.Background(), []watchEventSink{sink}, queue)
	if err == nil {
		t.Fatal("expected flush error, got nil")
	}
	if len(remaining) != 0 {
		t.Fatalf("remaining len = %d, want 0", len(remaining))
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
	watchOutputFile = ""
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
