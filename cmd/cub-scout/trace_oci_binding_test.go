// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestTraceOCISourceBinding(t *testing.T) {
	for _, backend := range []string{"argo", "flux"} {
		t.Run(backend, func(t *testing.T) {
			f := newReleaseFixture(t, backend)
			key := "Application/api"
			if backend == "flux" {
				key = "OCIRepository/config"
			}
			obj := f.objects[key]
			digest := firstOCIDigest(f.options.Bundle)
			require.True(t, traceOCISourceBindingMatches(obj, "oci://example.invalid/config", digest))
			stale := obj.DeepCopy()
			if backend == "argo" {
				_ = unstructured.SetNestedField(stale.Object, "oci://example.invalid/other", "spec", "source", "repoURL")
			} else {
				stale.SetGeneration(stale.GetGeneration() + 1)
			}
			require.False(t, traceOCISourceBindingMatches(stale, "oci://example.invalid/config", digest))
			missing := obj.DeepCopy()
			if backend == "argo" {
				unstructured.RemoveNestedField(missing.Object, "status", "sync", "comparedTo")
			} else {
				unstructured.RemoveNestedField(missing.Object, "status", "conditions")
			}
			require.False(t, traceOCISourceBindingMatches(missing, "oci://example.invalid/config", digest))
		})
	}
}

func TestConfirmTraceOCISourceUsesBoundedReadAndFailsClosed(t *testing.T) {
	f := newReleaseFixture(t, "argo")
	digest := firstOCIDigest(f.options.Bundle)
	result := &agent.TraceResult{Tool: "argocd", Chain: []agent.ChainLink{
		{Kind: "ConfigHub OCI", OCISource: &agent.OCISourceInfo{Raw: "oci://example.invalid/config", IsConfigHub: true}},
		{Kind: "Application", Name: "api", Namespace: "delivery", Revision: digest},
	}}
	c := agent.TraceDeliveryCorrelation{OCIIdentityStatus: "exact", OCIDigest: digest}
	confirmTraceOCISource(context.Background(), result, &c)
	require.True(t, c.OCISourceVerified)
	require.Equal(t, agent.BoundedReadCounts{Discovery: 1, Object: 1}, c.OCISourceRead.Reads)
	f.change = func(o *unstructured.Unstructured, _ int) int {
		if o.GetKind() == "Application" {
			unstructured.RemoveNestedField(o.Object, "status", "sync", "comparedTo")
		}
		return 0
	}
	c = agent.TraceDeliveryCorrelation{OCIIdentityStatus: "exact", OCIDigest: digest, OCIRegistry: "example.invalid", OCIRegistryVerified: true, OCISpace: "apps"}
	confirmTraceOCISource(context.Background(), result, &c)
	require.Equal(t, "unverified-source", c.OCIIdentityStatus)
	rows, omission := matchTraceReleases(c, []ConfigHubReleaseEvidence{{Space: "apps", ManifestDigest: digest}})
	require.Empty(t, rows)
	require.Contains(t, omission.Reason, "unverified-source")
}
