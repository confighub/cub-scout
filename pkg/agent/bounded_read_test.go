// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

const boundedDiscoveryFixture = `{"groupVersion":"apps/v1","resources":[{"name":"deployments","kind":"Deployment","namespaced":true,"verbs":["get"]}]}`

func boundedObjectFixture(namespace string) string {
	return fmt.Sprintf(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":%q,"uid":"uid-1","resourceVersion":"12","generation":2},"spec":{"replicas":1},"status":{"observedGeneration":1}}`, namespace)
}

func TestBoundedReadBudgetAndReuse(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		require.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/apps/v1":
			fmt.Fprint(w, boundedDiscoveryFixture)
		case "/apis/apps/v1/namespaces/team-a/deployments/api":
			fmt.Fprint(w, boundedObjectFixture("team-a"))
		default:
			t.Errorf("unexpected API request: %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	reader, err := NewBoundedResourceReader(&rest.Config{Host: server.URL}, "cluster-a")
	require.NoError(t, err)
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	reader.now = func() time.Time { return now }
	ref := BoundedResourceRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "team-a", Name: "api"}
	obj, first, err := reader.Read(context.Background(), ref, false)
	require.NoError(t, err)
	require.Equal(t, "miss", first.Cache)
	require.Equal(t, 1, first.Reads.Discovery)
	require.Equal(t, 1, first.Reads.Object)
	require.EqualValues(t, 2, requests.Load())
	obj.SetName("caller-mutation")
	now = now.Add(time.Second)
	obj, second, err := reader.Read(context.Background(), ref, false)
	require.NoError(t, err)
	require.Equal(t, "api", obj.GetName())
	require.Equal(t, "hit", second.Cache)
	require.Equal(t, first.ObservedAt, second.ObservedAt)
	require.Equal(t, BoundedReadCounts{}, second.Reads)
	require.EqualValues(t, 2, requests.Load())
	_, refreshed, err := reader.Read(context.Background(), ref, true)
	require.NoError(t, err)
	require.Equal(t, "refresh", refreshed.Cache)
	require.Equal(t, now, refreshed.ObservedAt)
	require.EqualValues(t, 4, requests.Load())
	now = refreshed.ExpiresAt
	_, expired, err := reader.Read(context.Background(), ref, false)
	require.NoError(t, err)
	require.Equal(t, "miss", expired.Cache)
	require.EqualValues(t, 6, requests.Load())
}

func TestBoundedReadRejectsUnsafeInputWithoutRequests(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1) }))
	defer server.Close()
	reader, err := NewBoundedResourceReader(&rest.Config{Host: server.URL}, "cluster-a")
	require.NoError(t, err)
	for _, ref := range []BoundedResourceRef{
		{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "team-a", Name: "../api"},
		{APIVersion: "apps/v1/extra", Kind: "Deployment", Name: "api"},
		{APIVersion: "v1", Kind: "Secret", Namespace: "team-a", Name: "token"},
		{APIVersion: "apps/v1", Kind: "Deployment/status", Name: "api"},
		{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "*", Name: "api"},
		{APIVersion: "apps/v1", Kind: "Deployment", Name: ""},
	} {
		_, _, err := reader.Read(context.Background(), ref, false)
		require.Error(t, err, "%+v", ref)
	}
	require.Zero(t, requests.Load())
}

func TestBoundedReadFailureDoesNotServeCachedSuccess(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusNotFound, http.StatusTooManyRequests} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var fail atomic.Bool
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if fail.Load() {
					w.Header().Set("Retry-After", "1")
					w.WriteHeader(status)
					fmt.Fprintf(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Unavailable","code":%d}`, status)
					return
				}
				if r.URL.Path == "/apis/apps/v1" {
					fmt.Fprint(w, boundedDiscoveryFixture)
				} else {
					fmt.Fprint(w, boundedObjectFixture("team-a"))
				}
			}))
			defer server.Close()
			reader, err := NewBoundedResourceReader(&rest.Config{Host: server.URL}, "cluster-a")
			require.NoError(t, err)
			ref := BoundedResourceRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "team-a", Name: "api"}
			_, _, err = reader.Read(context.Background(), ref, false)
			require.NoError(t, err)
			fail.Store(true)
			obj, evidence, err := reader.Read(context.Background(), ref, true)
			require.Error(t, err)
			require.Nil(t, obj)
			require.True(t, evidence.ObservedAt.IsZero())
			require.EqualValues(t, 3, requests.Load(), "no REST retries")
			_, _, err = reader.Read(context.Background(), ref, false)
			require.Error(t, err, "failed refresh must evict the previous success")
			require.EqualValues(t, 4, requests.Load())
		})
	}
}

func TestBoundedReadDoesNotFollowRedirects(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Redirect(w, r, "/unexpected-read", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	reader, err := NewBoundedResourceReader(&rest.Config{Host: server.URL}, "cluster-a")
	require.NoError(t, err)
	_, evidence, err := reader.Read(context.Background(), BoundedResourceRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "team-a", Name: "api"}, false)
	require.Error(t, err)
	require.False(t, evidence.Available)
	require.EqualValues(t, 1, requests.Load(), "redirects must not exceed the read budget")
}

func TestBoundedReadCoreClusterScopedResource(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1":
			fmt.Fprint(w, `{"groupVersion":"v1","resources":[{"name":"nodes","kind":"Node","namespaced":false,"verbs":["get"]}]}`)
		case "/api/v1/nodes/node-a":
			fmt.Fprint(w, `{"apiVersion":"v1","kind":"Node","metadata":{"name":"node-a","uid":"node-uid"}}`)
		default:
			t.Errorf("unexpected core resource path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	reader, err := NewBoundedResourceReader(&rest.Config{Host: server.URL}, "cluster-a")
	require.NoError(t, err)
	ref := BoundedResourceRef{APIVersion: "v1", Kind: "Node", Name: "node-a"}
	_, evidence, err := reader.Read(context.Background(), ref, false)
	require.NoError(t, err)
	require.True(t, evidence.Available)
	require.Equal(t, "node-uid", evidence.UID)
	require.EqualValues(t, 2, requests.Load())
	ref.Namespace = "team-a"
	_, evidence, err = reader.Read(context.Background(), ref, false)
	require.ErrorContains(t, err, "cluster-scoped")
	require.False(t, evidence.Available)
	require.EqualValues(t, 3, requests.Load(), "invalid namespace must not fetch the object")
}

func TestBoundedReadDiscoveryAndIdentityFailures(t *testing.T) {
	for _, tc := range []struct {
		name, discovery, object string
		namespace               string
		reads                   int32
	}{
		{"missing-crd", `{"groupVersion":"apps/v1","resources":[]}`, "", "team-a", 1},
		{"ambiguous", `{"groupVersion":"apps/v1","resources":[{"name":"a","kind":"Deployment","namespaced":true,"verbs":["get"]},{"name":"b","kind":"Deployment","namespaced":true,"verbs":["get"]}]}`, "", "team-a", 1},
		{"missing-namespace", boundedDiscoveryFixture, "", "", 1},
		{"wrong-discovery-version", `{"groupVersion":"other/v1","resources":[]}`, "", "team-a", 1},
		{"malformed-discovery", `{`, "", "team-a", 1},
		{"wrong-object-namespace", boundedDiscoveryFixture, boundedObjectFixture("team-b"), "team-a", 2},
		{"malformed-object", boundedDiscoveryFixture, `{`, "team-a", 2},
		{"empty-object", boundedDiscoveryFixture, `{}`, "team-a", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/apis/apps/v1" {
					fmt.Fprint(w, tc.discovery)
				} else {
					fmt.Fprint(w, tc.object)
				}
			}))
			defer server.Close()
			reader, err := NewBoundedResourceReader(&rest.Config{Host: server.URL}, "cluster-a")
			require.NoError(t, err)
			obj, _, err := reader.Read(context.Background(), BoundedResourceRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: tc.namespace, Name: "api"}, false)
			require.Error(t, err)
			require.Nil(t, obj)
			require.Equal(t, tc.reads, requests.Load())
		})
	}
}

func TestBoundedReadCancellationAndResponseLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, strings.Repeat("x", 100)) }))
	defer server.Close()
	reader, err := NewBoundedResourceReader(&rest.Config{Host: server.URL}, "cluster-a")
	require.NoError(t, err)
	reader.maxBytes = 16
	ref := BoundedResourceRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "team-a", Name: "api"}
	_, _, err = reader.Read(context.Background(), ref, false)
	require.ErrorContains(t, err, "response exceeds")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, evidence, err := reader.Read(ctx, ref, false)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, BoundedReadCounts{}, evidence.Reads)
}

func TestBoundedReadContextNamespaceIsolationAndEviction(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/apis/apps/v1" {
			fmt.Fprint(w, boundedDiscoveryFixture)
			return
		}
		parts := strings.Split(r.URL.Path, "/")
		fmt.Fprint(w, boundedObjectFixture(parts[5]))
	}))
	defer server.Close()
	readerA, err := NewBoundedResourceReader(&rest.Config{Host: server.URL}, "cluster-a")
	require.NoError(t, err)
	readerB, err := NewBoundedResourceReader(&rest.Config{Host: server.URL}, "cluster-b")
	require.NoError(t, err)
	readerA.maxEntries = 1
	a := BoundedResourceRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "team-a", Name: "api"}
	b := a
	b.Namespace = "team-b"
	_, ea, err := readerA.Read(context.Background(), a, false)
	require.NoError(t, err)
	_, eb, err := readerB.Read(context.Background(), a, false)
	require.NoError(t, err)
	require.NotEqual(t, ea.Context, eb.Context)
	_, _, err = readerA.Read(context.Background(), b, false)
	require.NoError(t, err)
	_, evicted, err := readerA.Read(context.Background(), a, false)
	require.NoError(t, err)
	require.Equal(t, "miss", evicted.Cache)
	require.EqualValues(t, 8, requests.Load())
	encoded, err := json.Marshal(evicted)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), server.URL, "do not expose endpoints or credentials")
}

func TestBoundedReadInFlightCancellationAndWaiting(t *testing.T) {
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
	}))
	defer server.Close()
	reader, err := NewBoundedResourceReader(&rest.Config{Host: server.URL}, "cluster-a")
	require.NoError(t, err)
	ref := BoundedResourceRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "team-a", Name: "api"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, _, err := reader.Read(ctx, ref, false); done <- err }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("request never started")
	}
	waitCtx, waitCancel := context.WithCancel(context.Background())
	waitDone := make(chan error, 1)
	go func() {
		_, e, err := reader.Read(waitCtx, ref, false)
		if e.Reads != (BoundedReadCounts{}) {
			err = fmt.Errorf("waiting caller performed reads")
		}
		waitDone <- err
	}()
	waitCancel()
	select {
	case err := <-waitDone:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("waiting read did not cancel")
	}
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("in-flight read did not cancel")
	}
}
