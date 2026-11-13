// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

// On Flux 2.10 an OCIRepository is served as v1; reading v1beta2 alone gave
// "timing unavailable ... not found" on a healthy chain (CI run 43451238398).
// A cluster with Flux older than 2.6 serves only v1beta2 and must still read.
func TestTimingReadsAnOCIRepositoryInWhicheverVersionIsServed(t *testing.T) {
	for _, served := range []string{"v1", "v1beta2"} {
		t.Run(served, func(t *testing.T) {
			repo := &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "source.toolkit.fluxcd.io/" + served,
				"kind":       "OCIRepository",
				"metadata":   map[string]interface{}{"name": "apps", "namespace": "flux-system"},
				"status": map[string]interface{}{"conditions": []interface{}{map[string]interface{}{
					"type": "Ready", "status": "True", "lastTransitionTime": "2026-11-13T15:13:45Z",
				}}},
			}}
			gvr := schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: served, Resource: "ocirepositories"}
			client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{gvr: "OCIRepositoryList"}, repo)
			_, errs := NewTimingEnricher(client).EnrichChainWithTimingAndErrors(context.Background(), []ChainLink{{Kind: "OCIRepository", Name: "apps", Namespace: "flux-system"}})
			if len(errs) != 0 {
				t.Fatalf("an OCIRepository served as %s could not be read: %v", served, errs)
			}
		})
	}
}
