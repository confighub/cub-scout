// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
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
			{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"}:      "HelmReleaseList",
			{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"}: "KustomizationList",
			{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}:            "ApplicationList",
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
	wb, synced, stop, err := newWatchBackedClient(ctx, client, []schema.GroupVersionResource{podsGVR}, "scan")
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
	wb, synced, stop, err := newWatchBackedClient(ctx, client, []schema.GroupVersionResource{depGVR}, "budget")
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
