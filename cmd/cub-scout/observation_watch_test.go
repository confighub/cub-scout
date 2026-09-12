// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
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
