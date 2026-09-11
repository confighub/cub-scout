// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

type watchBudgetCount struct {
	Requests int `json:"requests"`
	Bytes    int `json:"responseBodyBytes"`
}

type watchBudgetFixture struct {
	client  dynamic.Interface
	mu      sync.Mutex
	changed bool
	counts  map[string]watchBudgetCount
}

// The fixture returns literal lists over HTTP so counters include actual
// dynamic-client requests, not fake-client actions or displayed item counts.
func newWatchBudgetFixture(tb testing.TB, size int) *watchBudgetFixture {
	tb.Helper()
	items := make([]map[string]interface{}, size)
	for i := range items {
		name := fmt.Sprintf("app-%04d", i)
		items[i] = map[string]interface{}{
			"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": map[string]interface{}{"name": name, "namespace": "budget", "uid": name, "generation": 1},
			"spec": map[string]interface{}{"replicas": 1, "selector": map[string]interface{}{"matchLabels": map[string]string{"app": name}},
				"template": map[string]interface{}{"metadata": map[string]interface{}{"labels": map[string]string{"app": name}},
					"spec": map[string]interface{}{"containers": []interface{}{map[string]interface{}{"name": "app", "image": "example.invalid/app:v1"}}}}},
			"status": map[string]interface{}{"observedGeneration": 1, "replicas": 1, "updatedReplicas": 1, "readyReplicas": 1, "availableReplicas": 1},
		}
	}
	encode := func(items interface{}) []byte {
		data, err := json.Marshal(map[string]interface{}{"apiVersion": "v1", "kind": "List", "metadata": map[string]string{"resourceVersion": "1"}, "items": items})
		if err != nil {
			tb.Fatal(err)
		}
		return data
	}
	initial := encode(items)
	items[0]["metadata"].(map[string]interface{})["labels"] = map[string]string{"app.kubernetes.io/managed-by": "Helm"}
	changed := encode(items)
	empty := encode([]interface{}{})
	f := &watchBudgetFixture{counts: map[string]watchBudgetCount{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/namespaces/budget/") || r.URL.Query().Get("watch") != "" {
			tb.Errorf("unexpected method/scope: %s %s", r.Method, r.URL)
			http.Error(w, "unsupported fixture request", http.StatusForbidden)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		body := empty
		if r.URL.Path == "/apis/apps/v1/namespaces/budget/deployments" {
			body = initial
			if f.changed {
				body = changed
			}
		}
		w.Header().Set("Content-Type", "application/json")
		n, err := w.Write(body)
		if err != nil {
			tb.Errorf("fixture write: %v", err)
		}
		key := r.Method + " " + r.URL.RequestURI()
		count := f.counts[key]
		count.Requests++
		count.Bytes += n
		f.counts[key] = count
	}))
	tb.Cleanup(server.Close)
	var err error
	// Disable throttling only in this loopback measurement. Production client
	// defaults and API-server latency are deliberately not modeled here.
	f.client, err = dynamic.NewForConfig(&rest.Config{Host: server.URL, QPS: -1, Timeout: 5 * time.Second})
	if err != nil {
		tb.Fatal(err)
	}
	return f
}

func (f *watchBudgetFixture) takeCounts() map[string]watchBudgetCount {
	f.mu.Lock()
	defer f.mu.Unlock()
	counts := f.counts
	f.counts = map[string]watchBudgetCount{}
	return counts
}

func watchBudgetTotals(counts map[string]watchBudgetCount) watchBudgetCount {
	var total watchBudgetCount
	for _, count := range counts {
		total.Requests += count.Requests
		total.Bytes += count.Bytes
	}
	return total
}

func TestWatchObservationBudget(t *testing.T) {
	t.Setenv(customResourceConfigEnvVar, filepath.Join(t.TempDir(), "no-custom-resources.yaml"))
	t.Setenv("CLUSTER_NAME", "budget-cluster")
	for _, size := range []int{100, 1000} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			fixture := newWatchBudgetFixture(t, size)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			var previous watchState
			var baseline map[string]watchBudgetCount
			for _, phase := range []string{"cold", "idle", "ownership-change"} {
				fixture.mu.Lock()
				fixture.changed = phase == "ownership-change"
				fixture.mu.Unlock()
				state, err := collectWatchState(ctx, fixture.client, "budget")
				if err != nil {
					t.Fatal(err)
				}
				if len(state.entriesByID) != size || len(state.findings) != 0 {
					t.Fatalf("unexpected inventory/findings: %d/%d", len(state.entriesByID), len(state.findings))
				}
				events := buildWatchEvents(previous, state, nil, "", func() time.Time { return time.Unix(1, 0) })
				wantEvents := 0
				if phase == "cold" {
					wantEvents = size
				} else if phase == "ownership-change" {
					wantEvents = 1
					if len(events) != 1 || events[0].Type != "ownership.changed" || events[0].Resource.Name != "app-0000" || events[0].Owner.Type != "Helm" {
						t.Fatalf("wrong ownership change: %+v", events)
					}
				}
				if len(events) != wantEvents {
					t.Fatalf("%s: got %d events, want %d", phase, len(events), wantEvents)
				}
				counts := fixture.takeCounts()
				total := watchBudgetTotals(counts)
				if total.Requests == 0 || total.Bytes == 0 {
					t.Fatal("fixture did not count real HTTP responses")
				}
				if total.Requests > 48 {
					t.Errorf("polling fixture exceeded its baseline budget: %d requests (maximum 48)", total.Requests)
				}
				if phase == "cold" {
					baseline = counts
				} else if phase == "idle" && !reflect.DeepEqual(counts, baseline) {
					t.Fatalf("unchanged polling baseline changed: cold=%v idle=%v", baseline, counts)
				}
				data, err := json.Marshal(map[string]interface{}{"objects": size, "phase": phase, "events": len(events), "total": total, "paths": counts})
				if err != nil {
					t.Fatal(err)
				}
				t.Log(string(data))
				previous = state
			}
		})
	}
}

func BenchmarkWatchObservation(b *testing.B) {
	b.Setenv(customResourceConfigEnvVar, filepath.Join(b.TempDir(), "no-custom-resources.yaml"))
	b.Setenv("CLUSTER_NAME", "budget-cluster")
	for _, size := range []int{100, 1000} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			fixture := newWatchBudgetFixture(b, size)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				state, err := collectWatchState(context.Background(), fixture.client, "budget")
				if err != nil || len(state.entriesByID) != size {
					b.Fatalf("collection failed: %v", err)
				}
			}
			b.StopTimer()
			total := watchBudgetTotals(fixture.takeCounts())
			b.ReportMetric(float64(total.Requests)/float64(b.N), "requests/op")
			b.ReportMetric(float64(total.Bytes)/float64(b.N), "response-bytes/op")
		})
	}
}
