// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/confighub/cub-scout/v2/internal/mapsvc"
	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

func TestWatchCollectionDenialRecoveryAndConfirmedDeletion(t *testing.T) {
	var phase, requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		require.Equal(t, "GET", r.Method)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/applicationsets") && phase.Load() == 4 {
			w.WriteHeader(404)
			fmt.Fprint(w, `{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"NotFound","code":404}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/deployments") {
			require.Contains(t, r.URL.Path, "/namespaces/team-a/")
			if phase.Load() == 1 {
				w.WriteHeader(403)
				fmt.Fprint(w, `{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"Forbidden","code":403,"message":"private-error-not-for-output"}`)
				return
			}
			items := `[{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":"team-a","uid":"instance","labels":{"app.kubernetes.io/managed-by":"Helm"}}}]`
			if phase.Load() == 3 {
				items = "[]"
			}
			fmt.Fprintf(w, `{"apiVersion":"apps/v1","kind":"DeploymentList","items":%s}`, items)
			return
		}
		fmt.Fprint(w, `{"apiVersion":"v1","kind":"List","items":[]}`)
	}))
	t.Cleanup(server.Close)
	client, err := dynamic.NewForConfig(&rest.Config{Host: server.URL, QPS: -1})
	require.NoError(t, err)
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	read := func() watchState {
		state, err := collectWatchState(context.Background(), client, "team-a")
		require.NoError(t, err)
		return state
	}
	first := read()
	require.Len(t, first.entriesByID, 1)
	require.Empty(t, first.omissions)
	before := requests.Load()
	phase.Store(1)
	denied := read()
	readCount := requests.Load() - before
	require.Empty(t, denied.entriesByID)
	require.Len(t, denied.omissions, 1)
	require.Equal(t, "forbidden", denied.omissions[0].Reason)
	events := buildWatchEvents(first, denied, nil, "Helm", func() time.Time { return now })
	require.Len(t, events, 1)
	require.Equal(t, "collection.partial", events[0].Type)
	require.Nil(t, events[0].Observation)
	require.Nil(t, events[0].Owner)
	require.Equal(t, "partial", events[0].Collection.Status)
	require.Equal(t, "team-a", events[0].Collection.Omissions[0].Namespace)
	baseline := watchDiffBaseline(first, denied)
	require.Len(t, baseline.entriesByID, 1)
	require.Empty(t, denied.entriesByID)
	// Repeated denial retains continuity only; no renewed object evidence.
	again := read()
	require.Equal(t, readCount, requests.Load()-before-readCount)
	require.Len(t, buildWatchEvents(baseline, again, nil, "", func() time.Time { return now }), 1)
	baseline = watchDiffBaseline(baseline, again)
	phase.Store(2)
	recovered := read()
	require.Empty(t, buildWatchEvents(baseline, recovered, nil, "", func() time.Time { return now }))
	baseline = watchDiffBaseline(baseline, recovered)
	phase.Store(3)
	empty := read()
	events = buildWatchEvents(baseline, empty, nil, "", func() time.Time { return now })
	require.Len(t, events, 1)
	require.Equal(t, "resource.deleted", events[0].Type)
	require.Equal(t, "api", events[0].Resource.Name)
	baseline = watchDiffBaseline(baseline, empty)
	require.Empty(t, buildWatchEvents(baseline, empty, nil, "", func() time.Time { return now }))
	phase.Store(4)
	missingAPI := read()
	require.Len(t, missingAPI.omissions, 1)
	require.Equal(t, "applicationsets", missingAPI.omissions[0].Resource)
	require.Equal(t, "not_found", missingAPI.omissions[0].Reason)
	require.Len(t, missingAPI.entriesByID, 1) // Missing optional lookup keeps readable workloads.
}

func TestWatchCollectionScopesAndHistoryRemainSeparate(t *testing.T) {
	ts := time.Date(2026, 10, 4, 11, 0, 0, 0, time.UTC)
	oldObservation := agent.NewObservationEvidence("kubernetes-api", "watch-poll", ts, agent.ObservationScope{Namespace: "team-a", Kind: "Deployment"})
	prev := watchState{entriesByID: map[string]MapEntry{
		"denied":   {Name: "api", Namespace: "team-a", Kind: "Deployment", Observation: oldObservation},
		"readable": {Name: "api", Namespace: "team-a", Kind: "Service"},
		"other-ns": {Name: "api", Namespace: "team-b", Kind: "Deployment"},
	}, entryScopes: map[string]watchInventoryScope{
		"denied": {"apps/v1", "deployments", "team-a"}, "readable": {"v1", "services", "team-a"}, "other-ns": {"apps/v1", "deployments", "team-b"},
	}}
	curr := watchState{entriesByID: map[string]MapEntry{}, omissions: []mapsvc.CollectionOmission{{APIVersion: "apps/v1", Resource: "deployments", Namespace: "team-a", Reason: "forbidden"}}}
	events := buildWatchEvents(prev, curr, nil, "", func() time.Time { return ts.Add(time.Hour) })
	require.Len(t, events, 3)
	for _, event := range events {
		if event.Type == "resource.deleted" {
			require.NotEqual(t, "Deployment/team-a", event.Resource.Kind+"/"+event.Resource.Namespace)
		}
	}
	next := watchDiffBaseline(prev, curr)
	require.Len(t, next.entriesByID, 1)
	require.Equal(t, oldObservation, next.entriesByID["denied"].Observation)
	require.Equal(t, ts, next.entriesByID["denied"].Observation.ObservedAt)
	require.Empty(t, curr.entriesByID)
	require.Len(t, prev.entriesByID, 3)
	// No scope provenance is a conservative unknown, never inferred from Kind.
	delete(prev.entryScopes, "denied")
	require.True(t, watchInventoryUnreadable(prev, curr, "denied"))
}

func TestWatchCollectionMissingAPIAndDeterministicOmissions(t *testing.T) {
	curr := watchState{entriesByID: map[string]MapEntry{}, omissions: []mapsvc.CollectionOmission{
		{APIVersion: "apps/v1", Resource: "deployments", Reason: "forbidden"},
		{APIVersion: "example.invalid/v1", Resource: "widgets", Reason: "not_found"},
	}}
	now := func() time.Time { return time.Unix(1, 0) }
	one := buildWatchEvents(watchState{}, curr, map[string]struct{}{"critical": {}}, "Flux", now)
	curr.omissions[0], curr.omissions[1] = curr.omissions[1], curr.omissions[0]
	two := buildWatchEvents(watchState{}, curr, nil, "", now)
	require.Equal(t, one, two)
	require.Len(t, one, 1)
	require.Equal(t, "collection.partial", one[0].Type)
	require.Nil(t, one[0].Observation) // The event timestamp is not object freshness.
	complete := watchState{entriesByID: map[string]MapEntry{}}
	require.Empty(t, buildWatchEvents(watchState{}, complete, nil, "", now))
}

func TestWatchCollectionPartialCannotBuildObjectReceipt(t *testing.T) {
	emitOn, err := parseWatchEmitReceiptOn("collection.partial")
	require.NoError(t, err)
	require.Equal(t, []string{"collection.partial"}, unsupportedEmitReceiptTypes(emitOn))
	previous := watchBuildReceiptForEventFn
	t.Cleanup(func() { watchBuildReceiptForEventFn = previous })
	watchBuildReceiptForEventFn = func(context.Context, watchEvent, dynamic.Interface, bool) (*agent.Statement, error) {
		t.Fatal("collection omission attempted an object receipt/read")
		return nil, nil
	}
	events := buildWatchEvents(watchState{}, watchState{omissions: []mapsvc.CollectionOmission{{APIVersion: "apps/v1", Resource: "deployments", Reason: "forbidden"}}}, nil, "", func() time.Time { return time.Unix(1, 0) })
	output := attachReceiptsIfRequested(context.Background(), events, emitOn, nil, false, func(string, ...interface{}) {})
	require.Equal(t, events, output)
	require.Nil(t, output[0].Receipt)
}

func TestWatchCollectionStartupReportsOmissionsWithoutDiscovery(t *testing.T) {
	previousCap := watchReceiptBatchCap
	t.Cleanup(func() { watchReceiptBatchCap = previousCap })
	restore := overrideWatchDeps(t)
	t.Cleanup(restore)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)
	command := &cobra.Command{}
	command.SetContext(ctx)
	watchBuildConfig = func() (*rest.Config, error) { return &rest.Config{Host: "https://example.invalid"}, nil }
	reads := 0
	watchCollectState = func(context.Context, dynamic.Interface, string) (watchState, error) {
		reads++
		return watchState{entriesByID: map[string]MapEntry{"api": {Name: "api", Kind: "Deployment", Owner: "Flux"}}, findings: map[string]watchFinding{"finding": {Name: "api", Kind: "Deployment", Category: "STATE"}}, omissions: []mapsvc.CollectionOmission{{APIVersion: "apps/v1", Resource: "deployments", Reason: "forbidden"}}}, nil
	}
	posted := []watchEvent{}
	watchPostEvent = func(_ context.Context, _ string, event watchEvent) error {
		posted = append(posted, event)
		cancel()
		return nil
	}
	err := runWatchWithOptions(command, watchOptions{WebhookURL: "https://example.invalid/events", Interval: time.Hour, MaxQueuedEvents: 10, Owner: "Helm", Severity: "critical"})
	require.NoError(t, err)
	require.Equal(t, 1, reads)
	require.Len(t, posted, 1)
	require.Equal(t, "collection.partial", posted[0].Type)
	require.Nil(t, posted[0].Observation)
}
