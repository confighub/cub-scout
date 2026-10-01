// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubetesting "k8s.io/client-go/testing"
)

func TestTimingEnricherKeepsPartialTimestampsAndReportsReadFailures(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	reads := map[string]int{}
	client.PrependReactor("get", "*", func(action kubetesting.Action) (bool, runtime.Object, error) {
		get := action.(kubetesting.GetAction)
		key := action.GetResource().Resource + "/" + get.GetNamespace() + "/" + get.GetName()
		reads[key]++
		switch get.GetName() {
		case "denied":
			return true, nil, apierrors.NewForbidden(action.GetResource().GroupResource(), get.GetName(), fmt.Errorf("private-response-marker"))
		case "gone":
			return true, nil, apierrors.NewNotFound(action.GetResource().GroupResource(), get.GetName())
		case "ready":
			return true, &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "source.toolkit.fluxcd.io/v1", "kind": "GitRepository", "metadata": map[string]interface{}{"name": "ready", "namespace": "ns"}, "status": map[string]interface{}{"artifact": map[string]interface{}{"lastUpdateTime": "2025-01-02T03:04:05Z"}}}}, nil
		default:
			return true, &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "source.toolkit.fluxcd.io/v1", "kind": "GitRepository", "metadata": map[string]interface{}{"name": get.GetName(), "namespace": "ns"}}}, nil
		}
	})

	known := time.Date(2024, 2, 3, 4, 5, 6, 0, time.UTC)
	chain := []ChainLink{
		{Kind: "GitRepository", Name: "ready", Namespace: "ns"},
		{Kind: "Bucket", Name: "denied", Namespace: "ns", LastTransitionTime: &known},
		{Kind: "HelmRelease", Name: "gone", Namespace: "ns"},
		{Kind: "GitRepository", Name: "no-time", Namespace: "ns", LastTransitionTime: &known},
		{Kind: "Job", Name: "unsupported", Namespace: "ns"},
	}
	enriched, errs := NewTimingEnricher(client).EnrichChainWithTimingAndErrors(context.Background(), chain)
	if got := enriched[0].LastTransitionTime; got == nil || !got.Equal(time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("successful timestamp=%v", got)
	}
	if enriched[1].LastTransitionTime == nil || !enriched[1].LastTransitionTime.Equal(known) {
		t.Fatal("failed read erased previously known timestamp")
	}
	if enriched[3].LastTransitionTime == nil || !enriched[3].LastTransitionTime.Equal(known) {
		t.Fatal("missing timing metadata erased previously known timestamp")
	}
	if enriched[2].LastTransitionTime != nil || enriched[4].LastTransitionTime != nil {
		t.Fatal("unexpected timestamp on not-found or unsupported link")
	}
	if len(errs) != 2 {
		t.Fatalf("got %d errors, want 2: %v", len(errs), errs)
	}
	if errs[0].Error() != "timing unavailable for Bucket/ns/denied: forbidden" || errs[1].Error() != "timing unavailable for HelmRelease/ns/gone: not found" {
		t.Fatalf("unexpected errors: %v", errs)
	}
	for _, err := range errs {
		if err.Error() == "" || strings.Contains(err.Error(), "private-response-marker") {
			t.Fatalf("unsafe timing error: %v", err)
		}
	}
	wantReads := map[string]int{"gitrepositories/ns/ready": 1, "buckets/ns/denied": 1, "helmreleases/ns/gone": 1, "gitrepositories/ns/no-time": 1}
	if len(reads) != len(wantReads) {
		t.Fatalf("reads=%v", reads)
	}
	for key, want := range wantReads {
		if reads[key] != want {
			t.Errorf("read count %s=%d want %d", key, reads[key], want)
		}
	}
}

func TestTimingEnricherNilClientFailsOnlyForSupportedKinds(t *testing.T) {
	_, errs := (*TimingEnricher)(nil).EnrichChainWithTimingAndErrors(context.Background(), []ChainLink{{Kind: "GitRepository", Name: "source", Namespace: "ns"}, {Kind: "Job", Name: "job"}})
	if len(errs) != 1 || errs[0].Error() != "timing unavailable for GitRepository/ns/source: Kubernetes client is unavailable" {
		t.Fatalf("errors=%v", errs)
	}
	legacy := NewTimingEnricher(nil).EnrichChainWithTiming(context.Background(), []ChainLink{{Kind: "GitRepository", Name: "source", Namespace: "ns"}})
	if len(legacy) != 1 || legacy[0].LastTransitionTime != nil {
		t.Fatalf("legacy method should remain safe: %#v", legacy)
	}
}
