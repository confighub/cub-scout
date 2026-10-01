// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubetesting "k8s.io/client-go/testing"
)

func TestTimingEnricherSkipsUnboundArgoOCISourceAndKeepsKnownTiming(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	reads := map[string]int{}
	client.PrependReactor("get", "*", func(action kubetesting.Action) (bool, runtime.Object, error) {
		get := action.(kubetesting.GetAction)
		key := action.GetResource().Resource + "/" + get.GetNamespace() + "/" + get.GetName()
		reads[key]++
		return true, &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "source.toolkit.fluxcd.io/v1", "kind": "GitRepository",
			"metadata": map[string]interface{}{"name": get.GetName(), "namespace": get.GetNamespace()},
			"status":   map[string]interface{}{"artifact": map[string]interface{}{"lastUpdateTime": "2026-09-01T12:00:00Z"}},
		}}, nil
	})

	known := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	chain := []ChainLink{
		{Kind: "GitRepository", Name: "platform", Namespace: "flux-system"},
		{Kind: "OCIRepository", Name: "team/repo", URL: "oci://registry.example/team/repo", Revision: "sha256:abc", LastTransitionTime: &known},
	}
	enriched, errs := NewTimingEnricher(client).EnrichChainWithTimingAndErrors(context.Background(), chain)
	if len(reads) != 1 || reads["gitrepositories/flux-system/platform"] != 1 {
		t.Fatalf("timing reads=%v, want only exact Flux source read", reads)
	}
	if enriched[0].LastTransitionTime == nil || !enriched[0].LastTransitionTime.Equal(time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("Flux source timing=%v, want fetched timestamp", enriched[0].LastTransitionTime)
	}
	if enriched[1].LastTransitionTime == nil || !enriched[1].LastTransitionTime.Equal(known) {
		t.Fatalf("existing Argo-derived timestamp was lost: %v", enriched[1].LastTransitionTime)
	}
	if enriched[1].URL != "oci://registry.example/team/repo" || enriched[1].Revision != "sha256:abc" {
		t.Fatalf("existing Argo-derived source facts changed: %#v", enriched[1])
	}
	if len(errs) != 1 || errs[0].Error() != "timing unavailable for OCIRepository/team/repo: exact Kubernetes name is missing or invalid" {
		t.Fatalf("identity omission=%v", errs)
	}
}

func TestTimingEnricherRequiresExactNamespaceAndValidIdentity(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	var actions []kubetesting.Action
	client.PrependReactor("get", "*", func(action kubetesting.Action) (bool, runtime.Object, error) {
		actions = append(actions, action)
		return true, nil, nil
	})
	chain := []ChainLink{
		{Kind: "GitRepository", Name: "source"},
		{Kind: "GitRepository", Name: "bad/name", Namespace: "sources"},
		{Kind: "GitRepository", Name: "source", Namespace: "bad_namespace"},
		{Kind: "Job", Name: "unsupported"},
	}
	_, errs := NewTimingEnricher(client).EnrichChainWithTimingAndErrors(context.Background(), chain)
	if len(actions) != 0 {
		t.Fatalf("invalid identities triggered actions: %#v", actions)
	}
	if len(errs) != 3 {
		t.Fatalf("omissions=%v, want three supported invalid identities and no warning for Job", errs)
	}
	if errs[0].Error() != "timing unavailable for GitRepository/source: exact Kubernetes namespace is required" ||
		errs[1].Error() != "timing unavailable for GitRepository/sources/bad/name: exact Kubernetes name is missing or invalid" ||
		errs[2].Error() != "timing unavailable for GitRepository/bad_namespace/source: exact Kubernetes namespace is invalid" {
		t.Fatalf("unexpected identity omissions: %v", errs)
	}
}
