// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

func mapInstanceCluster(id string) *agent.ClusterIdentityEvidence {
	now := time.Date(2026, 10, 6, 8, 0, 0, 0, time.UTC)
	return &agent.ClusterIdentityEvidence{Identity: "verified", ID: id, IDSource: "v1/Namespace/kube-system", ObservedAt: &now}
}

func mapInstanceObject(uid string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]interface{}{"name": "same-name", "namespace": "team-a", "uid": uid, "labels": map[string]interface{}{"app.kubernetes.io/managed-by": "Helm"}}}}
}

func mapInstanceEntry(obj *unstructured.Unstructured, gvr schema.GroupVersionResource, cluster *agent.ClusterIdentityEvidence) MapEntry {
	return processResourceWithIdentity(obj, gvr, "legacy-label", nil, map[string]int{}, mapApplicationSetLookup{}, cluster)[0]
}

func TestMapResourceIdentityKeysSeparateClustersAndRecreatedInstances(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	keys := map[string]bool{}
	for _, pair := range [][2]string{{"cluster-a", "uid-a"}, {"cluster-b", "uid-a"}, {"cluster-a", "uid-b"}} {
		cluster := mapInstanceCluster(pair[0])
		entry := mapInstanceEntry(mapInstanceObject(pair[1]), gvr, cluster)
		evidence := mapEntryIdentityEvidence(entry, cluster)
		require.Equal(t, "verified", evidence.Status)
		want, _ := json.Marshal([7]string{"v1/Namespace/kube-system", pair[0], "apps", "Deployment", "team-a", "same-name", pair[1]})
		require.Equal(t, string(want), evidence.MergeKey)
		require.False(t, keys[evidence.MergeKey])
		keys[evidence.MergeKey] = true
		// Original legacy JSON contract gains neither UID nor instance evidence.
		raw, err := json.Marshal(entry)
		require.NoError(t, err)
		require.NotContains(t, string(raw), "resourceIdentity")
		require.NotContains(t, string(raw), pair[1])
		require.Equal(t, "Helm", entry.Owner)
	}
}

func TestMapResourceIdentityRefusesMissingAndMismatchedEvidence(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	cases := []struct {
		name, reason string
		change       func(*unstructured.Unstructured, *agent.ClusterIdentityEvidence, *schema.GroupVersionResource)
	}{
		{"missing UID", "object_identity_unavailable", func(o *unstructured.Unstructured, _ *agent.ClusterIdentityEvidence, _ *schema.GroupVersionResource) {
			delete(o.Object["metadata"].(map[string]interface{}), "uid")
		}},
		{"missing namespace", "object_identity_unavailable", func(o *unstructured.Unstructured, _ *agent.ClusterIdentityEvidence, _ *schema.GroupVersionResource) {
			delete(o.Object["metadata"].(map[string]interface{}), "namespace")
		}},
		{"wrong version", "resource_type_mismatch", func(o *unstructured.Unstructured, _ *agent.ClusterIdentityEvidence, _ *schema.GroupVersionResource) {
			o.SetAPIVersion("apps/v2")
		}},
		{"wrong type", "resource_type_mismatch", func(o *unstructured.Unstructured, _ *agent.ClusterIdentityEvidence, _ *schema.GroupVersionResource) {
			o.SetKind("StatefulSet")
		}},
		{"custom unknown scope", "resource_scope_unknown", func(o *unstructured.Unstructured, _ *agent.ClusterIdentityEvidence, g *schema.GroupVersionResource) {
			*g = schema.GroupVersionResource{Group: "example.org", Version: "v1", Resource: "widgets"}
			o.SetAPIVersion("example.org/v1")
			o.SetKind("Widget")
		}},
		{"denied cluster", "cluster_identity_unverified", func(_ *unstructured.Unstructured, c *agent.ClusterIdentityEvidence, _ *schema.GroupVersionResource) {
			c.Identity = "unverified"
			c.Omission = "forbidden"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := mapInstanceObject("uid-a")
			c := mapInstanceCluster("cluster-a")
			g := gvr
			tc.change(o, c, &g)
			before := o.DeepCopy()
			entry := mapInstanceEntry(o, g, c)
			e := mapEntryIdentityEvidence(entry, c)
			require.Equal(t, "unverified", e.Status)
			require.Equal(t, tc.reason, e.Omission)
			require.Nil(t, e.Observed)
			require.Empty(t, e.MergeKey)
			require.Equal(t, before, o)
			require.Equal(t, "Helm", entry.Owner)
		})
	}
}

func TestMapResourceIdentityRenderingRejectsRetainedWrongScope(t *testing.T) {
	c := mapInstanceCluster("cluster-a")
	entry := mapInstanceEntry(mapInstanceObject("uid-a"), schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, c)
	require.Contains(t, mapResourceIdentityText(entry, c), `UID="uid-a"`)
	different := mapInstanceCluster("cluster-b")
	require.Equal(t, "resource_cluster_mismatch", mapEntryIdentityEvidence(entry, different).Omission)
	different.Identity = "unverified"
	panel := (LocalClusterModel{clusterIdentity: different, entries: []MapEntry{entry}}).getPanelOwnershipEvidence()
	require.Contains(t, panel, "cluster_identity_unverified")
	require.NotContains(t, panel, `UID="uid-a"`)
	require.NotContains(t, panel, "merge-key=")
}

func TestMapResourceIdentityCLIAndStructuredMCPHaveSameFacts(t *testing.T) {
	setupMapIdentityFlags(t)
	f := newMapIdentityFixture(t)
	output := captureStdout(t, func() {
		require.NoError(t, runMapListFromClusterWithConfigAndIdentity(context.Background(), &rest.Config{Host: f.server.URL}, nil, false, "selected", true))
	})
	var report mapClusterIdentityOutput
	require.NoError(t, json.Unmarshal([]byte(output), &report))
	require.Len(t, report.Resources, 1)
	evidence := report.Resources[0].ResourceIdentity
	require.Equal(t, "verified", evidence.Status)
	require.Equal(t, "object-instance", evidence.Observed.UID)
	structured := buildMCPStructuredContent("map", output)
	raw, err := json.Marshal(structured.(map[string]interface{})["data"])
	require.NoError(t, err)
	var same mapClusterIdentityOutput
	require.NoError(t, json.Unmarshal(raw, &same))
	require.Equal(t, report, same)
	require.EqualValues(t, 1, f.identityReads.Load())
	require.EqualValues(t, 3, f.allReads.Load())
}
