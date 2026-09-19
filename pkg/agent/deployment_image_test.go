// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

func deploymentImageFixture(t *testing.T) (*unstructured.Unstructured, []*unstructured.Unstructured, map[string]*unstructured.Unstructured) {
	t.Helper()
	live := deployment([2]string{"api", "example.invalid/api@" + digestNew})
	live.SetUID("deployment-uid")
	live.SetGeneration(3)
	require.NoError(t, unstructured.SetNestedField(live.Object, int64(2), "spec", "replicas"))
	require.NoError(t, unstructured.SetNestedStringMap(live.Object, map[string]string{"app": "api"}, "spec", "selector", "matchLabels"))
	require.NoError(t, unstructured.SetNestedMap(live.Object, map[string]interface{}{"observedGeneration": int64(3), "replicas": int64(2), "updatedReplicas": int64(2), "readyReplicas": int64(2), "availableReplicas": int64(2)}, "status"))
	controller := true
	rs := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "apps/v1", "kind": "ReplicaSet"}}
	rs.SetName("api-current")
	rs.SetNamespace("delivery")
	rs.SetUID("rs-uid")
	rs.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: live.GetName(), UID: live.GetUID(), Controller: &controller}})
	template, _, _ := unstructured.NestedMap(live.Object, "spec", "template")
	require.NoError(t, unstructured.SetNestedMap(rs.Object, template, "spec", "template"))
	pods := []*unstructured.Unstructured{}
	for _, name := range []string{"api-1", "api-2"} {
		p := runningPod([2]string{"api", digestNew})
		p.SetName(name)
		p.SetUID(types.UID(name + "-uid"))
		p.SetLabels(map[string]string{"app": "api"})
		p.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: rs.GetName(), UID: rs.GetUID(), Controller: &controller}})
		require.NoError(t, unstructured.SetNestedField(p.Object, "Running", "status", "phase"))
		require.NoError(t, unstructured.SetNestedSlice(p.Object, []interface{}{map[string]interface{}{"type": "Ready", "status": "True"}}, "status", "conditions"))
		pods = append(pods, p)
	}
	return live, pods, map[string]*unstructured.Unstructured{rs.GetName(): rs}
}

func TestDeploymentImageCoverage(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*unstructured.Unstructured, []*unstructured.Unstructured, map[string]*unstructured.Unstructured) []*unstructured.Unstructured
	}{
		{"complete", nil},
		{"too few pods", func(_ *unstructured.Unstructured, p []*unstructured.Unstructured, _ map[string]*unstructured.Unstructured) []*unstructured.Unstructured {
			return p[:1]
		}},
		{"stale generation", func(l *unstructured.Unstructured, p []*unstructured.Unstructured, _ map[string]*unstructured.Unstructured) []*unstructured.Unstructured {
			_ = unstructured.SetNestedField(l.Object, int64(2), "status", "observedGeneration")
			return p
		}},
		{"zero replicas", func(l *unstructured.Unstructured, p []*unstructured.Unstructured, _ map[string]*unstructured.Unstructured) []*unstructured.Unstructured {
			_ = unstructured.SetNestedField(l.Object, int64(0), "spec", "replicas")
			return nil
		}},
		{"missing count", func(l *unstructured.Unstructured, p []*unstructured.Unstructured, _ map[string]*unstructured.Unstructured) []*unstructured.Unstructured {
			unstructured.RemoveNestedField(l.Object, "status", "readyReplicas")
			return p
		}},
		{"old replica", func(_ *unstructured.Unstructured, p []*unstructured.Unstructured, r map[string]*unstructured.Unstructured) []*unstructured.Unstructured {
			_ = unstructured.SetNestedField(r["api-current"].Object, "old", "spec", "template", "spec", "serviceAccountName")
			return p
		}},
		{"foreign owner", func(_ *unstructured.Unstructured, p []*unstructured.Unstructured, r map[string]*unstructured.Unstructured) []*unstructured.Unstructured {
			refs := r["api-current"].GetOwnerReferences()
			refs[0].UID = "other"
			r["api-current"].SetOwnerReferences(refs)
			return p
		}},
		{"missing rs", func(_ *unstructured.Unstructured, p []*unstructured.Unstructured, r map[string]*unstructured.Unstructured) []*unstructured.Unstructured {
			delete(r, "api-current")
			return p
		}},
		{"missing owner", func(_ *unstructured.Unstructured, p []*unstructured.Unstructured, _ map[string]*unstructured.Unstructured) []*unstructured.Unstructured {
			p[0].SetOwnerReferences(nil)
			return p
		}},
		{"uid mismatch", func(_ *unstructured.Unstructured, p []*unstructured.Unstructured, r map[string]*unstructured.Unstructured) []*unstructured.Unstructured {
			r["api-current"].SetUID("recreated")
			return p
		}},
		{"duplicate pod", func(_ *unstructured.Unstructured, p []*unstructured.Unstructured, _ map[string]*unstructured.Unstructured) []*unstructured.Unstructured {
			return []*unstructured.Unstructured{p[0], p[0]}
		}},
		{"terminating", func(_ *unstructured.Unstructured, p []*unstructured.Unstructured, _ map[string]*unstructured.Unstructured) []*unstructured.Unstructured {
			now := metav1.Now()
			p[0].SetDeletionTimestamp(&now)
			return p
		}},
		{"not ready", func(_ *unstructured.Unstructured, p []*unstructured.Unstructured, _ map[string]*unstructured.Unstructured) []*unstructured.Unstructured {
			unstructured.RemoveNestedField(p[0].Object, "status", "conditions")
			return p
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			live, pods, rs := deploymentImageFixture(t)
			if tc.change != nil {
				pods = tc.change(live, pods, rs)
			}
			got := BuildDeploymentImageCoverage(live, pods, rs)
			require.Equal(t, tc.name == "complete", got.Complete, "%+v", got)
			if !got.Complete {
				require.NotEmpty(t, got.Reason)
			}
			if len(pods) == 2 {
				require.Equal(t, got, BuildDeploymentImageCoverage(live, []*unstructured.Unstructured{pods[1], pods[0]}, rs))
			}
		})
	}
}

func TestDeploymentImageSelectorExpressionsAndOrder(t *testing.T) {
	live, pods, rs := deploymentImageFixture(t)
	expressions := []interface{}{map[string]interface{}{"key": "tier", "operator": "In", "values": []interface{}{"web"}}}
	require.NoError(t, unstructured.SetNestedSlice(live.Object, expressions, "spec", "selector", "matchExpressions"))
	selector, err := WorkloadPodSelector(live)
	require.NoError(t, err)
	require.Equal(t, "app=api,tier in (web)", selector.String())
	require.False(t, BuildDeploymentImageCoverage(live, pods, rs).Complete, "matchLabels alone cannot satisfy a mixed selector")
	for _, p := range pods {
		p.SetLabels(map[string]string{"app": "api", "tier": "web"})
	}
	first := BuildDeploymentImageCoverage(live, pods, rs)
	require.True(t, first.Complete)
	require.Equal(t, first, BuildDeploymentImageCoverage(live, []*unstructured.Unstructured{pods[1], pods[0]}, rs))
	unstructured.RemoveNestedField(live.Object, "spec", "selector", "matchLabels")
	require.True(t, BuildDeploymentImageCoverage(live, pods, rs).Complete, "expressions-only selectors are supported")
}

func TestDeploymentImageRejectsMalformedDeletion(t *testing.T) {
	live, pods, rs := deploymentImageFixture(t)
	require.NoError(t, unstructured.SetNestedField(pods[0].Object, "invalid", "metadata", "deletionTimestamp"))
	require.False(t, BuildDeploymentImageCoverage(live, pods, rs).Complete)
}

func TestDeploymentTemplateComparisonPreservesInputs(t *testing.T) {
	live, _, rs := deploymentImageFixture(t)
	require.NoError(t, unstructured.SetNestedField(rs["api-current"].Object, "controller-hash", "spec", "template", "metadata", "labels", "pod-template-hash"))
	require.NoError(t, unstructured.SetNestedStringMap(live.Object, map[string]string{"app": "api"}, "spec", "template", "metadata", "labels"))
	require.NoError(t, unstructured.SetNestedField(rs["api-current"].Object, "api", "spec", "template", "metadata", "labels", "app"))
	beforeLive, beforeRS := live.DeepCopy(), rs["api-current"].DeepCopy()
	require.True(t, sameDeploymentTemplate(live, rs["api-current"]))
	require.Equal(t, beforeLive, live)
	require.Equal(t, beforeRS, rs["api-current"], "NestedMap must return copies")
}
