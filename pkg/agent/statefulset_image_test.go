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

func statefulImageFixture(t *testing.T) (*unstructured.Unstructured, []*unstructured.Unstructured, *unstructured.Unstructured) {
	t.Helper()
	live, pods, _ := deploymentImageFixture(t)
	live.SetKind("StatefulSet")
	live.SetName("api")
	live.SetUID("stateful-uid")
	require.NoError(t, unstructured.SetNestedMap(live.Object, map[string]interface{}{"observedGeneration": int64(3), "replicas": int64(2), "currentReplicas": int64(2), "updatedReplicas": int64(2), "readyReplicas": int64(2), "currentRevision": "api-current", "updateRevision": "api-current"}, "status"))
	controller := true
	revision := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "apps/v1", "kind": "ControllerRevision", "revision": int64(3)}}
	revision.SetName("api-current")
	revision.SetNamespace(live.GetNamespace())
	revision.SetUID("revision-uid")
	owner := metav1.OwnerReference{APIVersion: "apps/v1", Kind: "StatefulSet", Name: live.GetName(), UID: live.GetUID(), Controller: &controller}
	revision.SetOwnerReferences([]metav1.OwnerReference{owner})
	template, _, _ := unstructured.NestedMap(live.Object, "spec", "template")
	template["$patch"] = "replace"
	require.NoError(t, unstructured.SetNestedMap(revision.Object, template, "data", "spec", "template"))
	for i, pod := range pods {
		if i == 0 {
			pod.SetName("api-0")
		} else {
			pod.SetName("api-1")
		}
		pod.SetUID(types.UID(pod.GetName() + "-uid"))
		pod.SetLabels(map[string]string{"app": "api", "controller-revision-hash": "api-current"})
		pod.SetOwnerReferences([]metav1.OwnerReference{owner})
	}
	return live, pods, revision
}

func TestStatefulSetImageCoverage(t *testing.T) {
	for _, tc := range []struct {
		name, reason string
		change       func(*unstructured.Unstructured, []*unstructured.Unstructured, *unstructured.Unstructured) []*unstructured.Unstructured
	}{
		{"complete", "", nil},
		{"missing pod", "pod-count-incomplete", func(_ *unstructured.Unstructured, p []*unstructured.Unstructured, _ *unstructured.Unstructured) []*unstructured.Unstructured {
			return p[:1]
		}},
		{"stale generation", "statefulset-generation-unconfirmed", func(l *unstructured.Unstructured, p []*unstructured.Unstructured, _ *unstructured.Unstructured) []*unstructured.Unstructured {
			_ = unstructured.SetNestedField(l.Object, int64(2), "status", "observedGeneration")
			return p
		}},
		{"zero desired", "no-running-replicas", func(l *unstructured.Unstructured, p []*unstructured.Unstructured, _ *unstructured.Unstructured) []*unstructured.Unstructured {
			_ = unstructured.SetNestedField(l.Object, int64(0), "spec", "replicas")
			return nil
		}},
		{"missing count", "statefulset-replicas-incomplete", func(l *unstructured.Unstructured, p []*unstructured.Unstructured, _ *unstructured.Unstructured) []*unstructured.Unstructured {
			unstructured.RemoveNestedField(l.Object, "status", "currentReplicas")
			return p
		}},
		{"old revision", "statefulset-revision-unconfirmed", func(l *unstructured.Unstructured, p []*unstructured.Unstructured, _ *unstructured.Unstructured) []*unstructured.Unstructured {
			_ = unstructured.SetNestedField(l.Object, "api-new", "status", "updateRevision")
			return p
		}},
		{"old pod", "statefulset-old-revision-observed", func(_ *unstructured.Unstructured, p []*unstructured.Unstructured, _ *unstructured.Unstructured) []*unstructured.Unstructured {
			p[0].SetLabels(map[string]string{"app": "api", "controller-revision-hash": "api-old"})
			return p
		}},
		{"foreign pod UID", "statefulset-pod-owner-unconfirmed", func(_ *unstructured.Unstructured, p []*unstructured.Unstructured, _ *unstructured.Unstructured) []*unstructured.Unstructured {
			o := p[0].GetOwnerReferences()
			o[0].UID = "other"
			p[0].SetOwnerReferences(o)
			return p
		}},
		{"foreign revision UID", "statefulset-revision-owner-unconfirmed", func(_ *unstructured.Unstructured, p []*unstructured.Unstructured, r *unstructured.Unstructured) []*unstructured.Unstructured {
			o := r.GetOwnerReferences()
			o[0].UID = "other"
			r.SetOwnerReferences(o)
			return p
		}},
		{"foreign namespace", "pod-identity-unconfirmed", func(_ *unstructured.Unstructured, p []*unstructured.Unstructured, _ *unstructured.Unstructured) []*unstructured.Unstructured {
			p[0].SetNamespace("other")
			return p
		}},
		{"duplicate pod", "duplicate-pod-identity", func(_ *unstructured.Unstructured, p []*unstructured.Unstructured, _ *unstructured.Unstructured) []*unstructured.Unstructured {
			return []*unstructured.Unstructured{p[0], p[0]}
		}},
		{"invalid ordinal", "statefulset-pod-ordinal-unconfirmed", func(_ *unstructured.Unstructured, p []*unstructured.Unstructured, _ *unstructured.Unstructured) []*unstructured.Unstructured {
			p[0].SetName("api-2")
			return p
		}},
		{"padded ordinal", "statefulset-pod-ordinal-unconfirmed", func(_ *unstructured.Unstructured, p []*unstructured.Unstructured, _ *unstructured.Unstructured) []*unstructured.Unstructured {
			p[0].SetName("api-00")
			return p
		}},
		{"terminating pod", "pod-terminating", func(_ *unstructured.Unstructured, p []*unstructured.Unstructured, _ *unstructured.Unstructured) []*unstructured.Unstructured {
			now := metav1.Now()
			p[0].SetDeletionTimestamp(&now)
			return p
		}},
		{"not ready", "pod-not-ready", func(_ *unstructured.Unstructured, p []*unstructured.Unstructured, _ *unstructured.Unstructured) []*unstructured.Unstructured {
			unstructured.RemoveNestedField(p[0].Object, "status", "conditions")
			return p
		}},
		{"template mismatch", "statefulset-revision-template-unconfirmed", func(_ *unstructured.Unstructured, p []*unstructured.Unstructured, r *unstructured.Unstructured) []*unstructured.Unstructured {
			_ = unstructured.SetNestedField(r.Object, "old", "data", "spec", "template", "spec", "serviceAccountName")
			return p
		}},
		{"partitioned", "statefulset-partition-unconfirmed", func(l *unstructured.Unstructured, p []*unstructured.Unstructured, _ *unstructured.Unstructured) []*unstructured.Unstructured {
			_ = unstructured.SetNestedField(l.Object, int64(1), "spec", "updateStrategy", "rollingUpdate", "partition")
			return p
		}},
		{"unknown strategy", "statefulset-strategy-unsupported", func(l *unstructured.Unstructured, p []*unstructured.Unstructured, _ *unstructured.Unstructured) []*unstructured.Unstructured {
			_ = unstructured.SetNestedField(l.Object, "Unknown", "spec", "updateStrategy", "type")
			return p
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l, p, r := statefulImageFixture(t)
			if tc.change != nil {
				p = tc.change(l, p, r)
			}
			beforeL, beforeR := l.DeepCopy(), r.DeepCopy()
			got := BuildStatefulSetImageCoverage(l, p, r)
			require.Equal(t, tc.reason, got.Reason)
			require.Equal(t, tc.reason == "", got.Complete)
			require.Equal(t, beforeL, l)
			require.Equal(t, beforeR, r)
			if len(p) == 2 {
				require.Equal(t, got, BuildStatefulSetImageCoverage(l, []*unstructured.Unstructured{p[1], p[0]}, r))
			}
		})
	}
}

func TestStatefulSetStartOrdinalAndRevisionOmission(t *testing.T) {
	l, p, r := statefulImageFixture(t)
	require.NoError(t, unstructured.SetNestedField(l.Object, int64(5), "spec", "ordinals", "start"))
	p[0].SetName("api-5")
	p[1].SetName("api-6")
	require.True(t, BuildStatefulSetImageCoverage(l, p, r).Complete)
	require.Equal(t, "statefulset-revision-identity-unconfirmed", BuildStatefulSetImageCoverage(l, p, nil).Reason)
	r.SetUID("")
	require.False(t, BuildStatefulSetImageCoverage(l, p, r).Complete)
}
