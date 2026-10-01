// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/rest"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
)

func watchEntry(id, kind, name, ns, owner string) MapEntry {
	return MapEntry{ID: id, Kind: kind, Name: name, Namespace: ns, Owner: owner, Status: "Ready"}
}

func TestBuildWatchEventsDeletion(t *testing.T) {
	prev := watchState{entriesByID: map[string]MapEntry{
		"a": watchEntry("a", "Deployment", "keep", "demo", "Flux"),
		"b": watchEntry("b", "Deployment", "gone", "demo", "ArgoCD"),
	}}
	curr := watchState{entriesByID: map[string]MapEntry{
		"a": watchEntry("a", "Deployment", "keep", "demo", "Flux"),
	}}
	events := buildWatchEvents(prev, curr, nil, "", func() time.Time { return time.Unix(1, 0) })
	if len(events) != 1 {
		t.Fatalf("expected exactly one deletion event, got %d: %+v", len(events), events)
	}
	e := events[0]
	if e.Type != "resource.deleted" || e.Resource.Name != "gone" || e.Resource.Namespace != "demo" {
		t.Fatalf("wrong deletion event: %+v", e)
	}
	if e.Owner == nil || e.Owner.Type != "ArgoCD" {
		t.Fatalf("deletion should preserve last owner: %+v", e.Owner)
	}

	// No deletion when everything persists.
	stable := buildWatchEvents(prev, prev, nil, "", func() time.Time { return time.Unix(1, 0) })
	for _, ev := range stable {
		if ev.Type == "resource.deleted" {
			t.Fatalf("unexpected deletion when inventory unchanged: %+v", ev)
		}
	}
}

func fakeDeployment(name, ns string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]interface{}{"name": name, "namespace": ns},
	}}
}

func fakePod(name, ns string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{"name": name, "namespace": ns},
		"status":   map[string]interface{}{"phase": "Running"},
	}}
}

func TestScannerWatchGVRsExcludedFromInventory(t *testing.T) {
	t.Setenv(customResourceConfigEnvVar, filepath.Join(t.TempDir(), "no-custom-resources.yaml"))
	pods := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	has := func(set []schema.GroupVersionResource, gvr schema.GroupVersionResource) bool {
		for _, g := range set {
			if g == gvr {
				return true
			}
		}
		return false
	}
	if has(collectWatchResourceList(), pods) {
		t.Error("pods must NOT be in the inventory sweep set (would enter the ownership map)")
	}
	if !has(scannerWatchGVRs(), pods) {
		t.Error("pods must be in the scanner watch set")
	}
	if !has(watchBackedCandidateGVRs(), pods) {
		t.Error("pods must be in the watch-backed candidate set")
	}
	seen := map[schema.GroupVersionResource]int{}
	for _, gvr := range watchBackedCandidateGVRs() {
		seen[gvr]++
		if seen[gvr] > 1 {
			t.Errorf("watch-backed candidate set has duplicate %v", gvr)
		}
	}
	// The candidate set must include every inventory type too.
	for _, gvr := range collectWatchResourceList() {
		if !has(watchBackedCandidateGVRs(), gvr) {
			t.Errorf("candidate set dropped inventory type %v", gvr)
		}
	}
}

// TestWatchBackedScannerReadsPodsFromCache proves the follow-up's value: with
// pods watch-backed, the state scan's runtime-failure pod read is served from
// the informer cache, so an idle scan makes no pod list API calls.
func TestWatchBackedScannerReadsPodsFromCache(t *testing.T) {
	t.Setenv(customResourceConfigEnvVar, filepath.Join(t.TempDir(), "no-custom-resources.yaml"))
	t.Setenv("CLUSTER_NAME", "scan-cluster")
	podsGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	scheme := runtime.NewScheme()
	// The state scan also lists these GitOps types; register their list kinds so
	// the fake returns empty lists (they are not watched here) instead of panicking.
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			podsGVR: "PodList",
			{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"}:        "HelmReleaseList",
			{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"}: "KustomizationList",
			{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}:             "ApplicationList",
		},
		fakePod("p1", "scan"), fakePod("p2", "scan"))

	var mu sync.Mutex
	podLists := 0
	client.PrependReactor("list", "pods", func(clienttesting.Action) (bool, runtime.Object, error) {
		mu.Lock()
		podLists++
		mu.Unlock()
		return false, nil, nil
	})

	ctx := context.Background()
	wb, synced, stop, err := newWatchBackedClient(ctx, client, []schema.GroupVersionResource{podsGVR},
		map[schema.GroupVersionResource]resourceScope{podsGVR: resourceScopeNamespaced}, "scan")
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if len(synced) != 1 || synced[0] != podsGVR {
		t.Fatalf("expected pods to sync, got %v", synced)
	}
	mu.Lock()
	afterSync := podLists
	mu.Unlock()

	// Run the full state scan through the watch-backed client twice. Types the
	// fake does not know (helmreleases, etc.) error and are swallowed by the
	// scanner; the pod read is what we assert on.
	for i := 0; i < 2; i++ {
		if _, err := collectWatchFindings(ctx, wb, "scan"); err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	afterScan := podLists
	mu.Unlock()
	if afterScan != afterSync {
		t.Errorf("scan made %d extra pod list API calls, want 0 (served from watch cache)", afterScan-afterSync)
	}
}

func TestWatchBackedClientServesFromCache(t *testing.T) {
	depGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{depGVR: "DeploymentList"},
		fakeDeployment("app-1", "budget"), fakeDeployment("app-2", "budget"))

	var mu sync.Mutex
	listCount := 0
	client.PrependReactor("list", "deployments", func(clienttesting.Action) (bool, runtime.Object, error) {
		mu.Lock()
		listCount++
		mu.Unlock()
		return false, nil, nil // count only; let the default reactor serve
	})

	ctx := context.Background()
	wb, synced, stop, err := newWatchBackedClient(ctx, client, []schema.GroupVersionResource{depGVR},
		map[schema.GroupVersionResource]resourceScope{depGVR: resourceScopeNamespaced}, "budget")
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if len(synced) != 1 || synced[0] != depGVR {
		t.Fatalf("expected deployments to sync, got %v", synced)
	}
	mu.Lock()
	afterSync := listCount
	mu.Unlock()
	if afterSync < 1 {
		t.Fatalf("informer sync should have listed at least once, got %d", afterSync)
	}

	// Idle reads come from the cache: zero additional list API calls.
	for i := 0; i < 3; i++ {
		l, err := wb.Resource(depGVR).Namespace("budget").List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(l.Items) != 2 {
			t.Fatalf("cache read returned %d items, want 2", len(l.Items))
		}
	}
	mu.Lock()
	afterReads := listCount
	mu.Unlock()
	if afterReads != afterSync {
		t.Errorf("cache reads made %d extra list API calls, want 0", afterReads-afterSync)
	}

	// A selector-scoped list is not cacheable and falls through to the API.
	if _, err := wb.Resource(depGVR).Namespace("budget").List(ctx, metav1.ListOptions{LabelSelector: "app=x"}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	afterSelector := listCount
	mu.Unlock()
	if afterSelector <= afterReads {
		t.Errorf("selector list should pass through to the API; counts %d -> %d", afterReads, afterSelector)
	}
}

func newNamespaceListClient(t *testing.T, handler http.Handler) dynamic.Interface {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := dynamic.NewForConfig(&rest.Config{Host: server.URL, QPS: -1, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func namespaceCache(t *testing.T, objects ...*unstructured.Unstructured) cache.GenericLister {
	t.Helper()
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	for _, object := range objects {
		if err := indexer.Add(object.DeepCopy()); err != nil {
			t.Fatal(err)
		}
	}
	return cache.NewGenericLister(indexer, schema.GroupResource{Group: "apps", Resource: "deployments"})
}

func writeNamespaceList(w http.ResponseWriter, items ...*unstructured.Unstructured) {
	encoded := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		encoded = append(encoded, item.Object)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"apiVersion": "v1", "kind": "DeploymentList",
		"metadata": map[string]interface{}{"resourceVersion": "7"}, "items": encoded,
	})
}

func TestWatchBackedNamespaceScopedCacheFallsBackForOtherAndAllNamespaces(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	inA, inB := fakeDeployment("shared", "team-a"), fakeDeployment("shared", "team-b")
	var mu sync.Mutex
	requests := []string{}
	client := newNamespaceListClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Method+" "+r.URL.Path)
		mu.Unlock()
		if r.Method != http.MethodGet || r.URL.Query().Get("watch") != "" {
			http.Error(w, "unexpected request", http.StatusForbidden)
			return
		}
		switch r.URL.Path {
		case "/apis/apps/v1/namespaces/team-b/deployments":
			writeNamespaceList(w, inB)
		case "/apis/apps/v1/deployments":
			writeNamespaceList(w, inA, inB)
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	wb := &watchBackedClient{Interface: client,
		listers: map[schema.GroupVersionResource]cache.GenericLister{gvr: namespaceCache(t, inA)},
		scopes:  map[schema.GroupVersionResource]resourceScope{gvr: resourceScopeNamespaced}, namespace: "team-a"}
	ctx := context.Background()

	gotA, err := wb.Resource(gvr).Namespace("team-a").List(ctx, metav1.ListOptions{})
	if err != nil || len(gotA.Items) != 1 || gotA.Items[0].GetNamespace() != "team-a" {
		t.Fatalf("exact cached namespace read = %#v, %v; want team-a/shared", gotA, err)
	}
	gotB, err := wb.Resource(gvr).Namespace("team-b").List(ctx, metav1.ListOptions{})
	if err != nil || len(gotB.Items) != 1 || gotB.Items[0].GetNamespace() != "team-b" {
		t.Fatalf("other namespace read = %#v, %v; want live team-b/shared", gotB, err)
	}
	gotAll, err := wb.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil || len(gotAll.Items) != 2 {
		t.Fatalf("all-namespace read = %#v, %v; want both live objects", gotAll, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 || requests[0] != "GET /apis/apps/v1/namespaces/team-b/deployments" || requests[1] != "GET /apis/apps/v1/deployments" {
		t.Fatalf("live fallback requests = %v, want B and all-namespace LIST", requests)
	}
}

func TestWatchBackedAllNamespaceCacheFiltersAndSelectorFallbackPreservesDenial(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	inA, inB := fakeDeployment("shared", "team-a"), fakeDeployment("shared", "team-b")
	var requestCount int
	client := newNamespaceListClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		http.Error(w, "forbidden by fixture", http.StatusForbidden)
	}))
	wb := &watchBackedClient{Interface: client,
		listers: map[schema.GroupVersionResource]cache.GenericLister{gvr: namespaceCache(t, inA, inB)},
		scopes:  map[schema.GroupVersionResource]resourceScope{gvr: resourceScopeNamespaced}}
	ctx := context.Background()
	for namespace, want := range map[string]string{"team-a": "team-a", "team-b": "team-b"} {
		got, err := wb.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
		if err != nil || len(got.Items) != 1 || got.Items[0].GetNamespace() != want {
			t.Fatalf("all-scope cache read %q = %#v, %v; want only %s", namespace, got, err, want)
		}
	}
	gotAll, err := wb.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil || len(gotAll.Items) != 2 {
		t.Fatalf("all-scope cache root read = %#v, %v; want both objects", gotAll, err)
	}
	// A selector is not cacheable. A denied live read stays an error and cannot
	// be turned into a successful empty list by the cache.
	if _, err := wb.Resource(gvr).Namespace("team-a").List(ctx, metav1.ListOptions{LabelSelector: "app=api"}); err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("selector fallback error = %v; want preserved forbidden response", err)
	}
	if requestCount != 1 {
		t.Fatalf("HTTP request count = %d, want only selector fallback", requestCount)
	}
}

func TestWatchBackedNamespaceMismatchPreservesDeniedFallback(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	inA := fakeDeployment("shared", "team-a")
	var requests int
	client := newNamespaceListClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "forbidden for team-b", http.StatusForbidden)
	}))
	wb := &watchBackedClient{Interface: client,
		listers: map[schema.GroupVersionResource]cache.GenericLister{gvr: namespaceCache(t, inA)},
		scopes:  map[schema.GroupVersionResource]resourceScope{gvr: resourceScopeNamespaced}, namespace: "team-a"}
	if _, err := wb.Resource(gvr).Namespace("team-b").List(context.Background(), metav1.ListOptions{}); err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("team-b fallback error = %v; want preserved forbidden response", err)
	}
	if requests != 1 {
		t.Fatalf("team-b fallback request count = %d, want 1", requests)
	}
}

func TestWatchBackedScopedCacheDoesNotGuessClusterScope(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}
	node := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Node", "metadata": map[string]interface{}{"name": "worker-1"},
	}}
	var requests int
	client := newNamespaceListClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "namespace is invalid for cluster-scoped resource", http.StatusBadRequest)
	}))
	wb := &watchBackedClient{Interface: client,
		listers: map[schema.GroupVersionResource]cache.GenericLister{gvr: cache.NewGenericLister(
			func() cache.Indexer {
				indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
				_ = indexer.Add(node)
				return indexer
			}(),
			schema.GroupResource{Resource: "nodes"})},
		scopes: map[schema.GroupVersionResource]resourceScope{gvr: resourceScopeCluster}, namespace: "team-a"}
	if _, err := wb.Resource(gvr).Namespace("team-a").List(context.Background(), metav1.ListOptions{}); err == nil {
		t.Fatal("cluster-scoped namespaced request unexpectedly returned cached empty success")
	}
	if requests != 1 {
		t.Fatalf("cluster-scope fallback request count = %d, want 1", requests)
	}
	allScope := &watchBackedClient{Interface: client,
		listers: map[schema.GroupVersionResource]cache.GenericLister{gvr: wb.listers[gvr]},
		scopes:  map[schema.GroupVersionResource]resourceScope{gvr: resourceScopeCluster}}
	all, err := allScope.Resource(gvr).List(context.Background(), metav1.ListOptions{})
	if err != nil || len(all.Items) != 1 || all.Items[0].GetName() != "worker-1" {
		t.Fatalf("all-scope cluster list = %#v, %v; want cached worker-1", all, err)
	}
	if _, err := allScope.Resource(gvr).Namespace("team-a").List(context.Background(), metav1.ListOptions{}); err == nil {
		t.Fatal("all-scope cache answered a namespaced read for a cluster-scoped GVR")
	}
	if requests != 2 {
		t.Fatalf("cluster-scope requests after all-scope reads = %d, want 2 (both invalid namespaced reads fell back)", requests)
	}
	unknownScope := &watchBackedClient{Interface: client,
		listers: map[schema.GroupVersionResource]cache.GenericLister{gvr: wb.listers[gvr]},
		scopes:  map[schema.GroupVersionResource]resourceScope{gvr: resourceScopeUnknown}}
	if _, err := unknownScope.Resource(gvr).List(context.Background(), metav1.ListOptions{}); err == nil {
		t.Fatal("unknown GVR scope unexpectedly returned a cached list")
	}
	if requests != 3 {
		t.Fatalf("unknown-scope fallback request count = %d, want 3", requests)
	}
}
