// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT
package agent

import (
	"reflect"
	"sort"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
)

// StatefulSetImageCoverage is coverage at one observed generation/revision,
// separate from digest equality and application success.
type StatefulSetImageCoverage struct {
	Complete           bool   `json:"complete"`
	Reason             string `json:"reason,omitempty"`
	UID                string `json:"uid"`
	Generation         int64  `json:"generation"`
	ObservedGeneration int64  `json:"observedGeneration"`
	DesiredReplicas    int64  `json:"desiredReplicas"`
	StartOrdinal       int64  `json:"startOrdinal"`
	CurrentRevision    string `json:"currentRevision"`
	UpdateRevision     string `json:"updateRevision"`
	RevisionUID        string `json:"revisionUID,omitempty"`
	OwnedPods          int    `json:"ownedPods"`
}

func BuildStatefulSetImageCoverage(live *unstructured.Unstructured, pods []*unstructured.Unstructured, revision *unstructured.Unstructured) StatefulSetImageCoverage {
	c := StatefulSetImageCoverage{}
	fail := func(reason string) StatefulSetImageCoverage { c.Reason = reason; return c }
	if live == nil || live.GetAPIVersion() != "apps/v1" || live.GetKind() != "StatefulSet" {
		return fail("statefulset-evidence-missing")
	}
	c.UID, c.Generation = string(live.GetUID()), live.GetGeneration()
	c.ObservedGeneration, _, _ = unstructured.NestedInt64(live.Object, "status", "observedGeneration")
	if c.UID == "" || live.GetName() == "" || live.GetNamespace() == "" || c.Generation <= 0 || c.ObservedGeneration != c.Generation {
		return fail("statefulset-generation-unconfirmed")
	}
	if hasImageDeletionMetadata(live) {
		return fail("statefulset-terminating")
	}
	replicas, found, err := unstructured.NestedInt64(live.Object, "spec", "replicas")
	if err != nil {
		return fail("statefulset-replicas-unreadable")
	}
	if !found {
		replicas = 1
	}
	c.DesiredReplicas = replicas
	if replicas <= 0 {
		return fail("no-running-replicas")
	}
	for _, field := range []string{"replicas", "currentReplicas", "updatedReplicas", "readyReplicas"} {
		value, found, err := unstructured.NestedInt64(live.Object, "status", field)
		if err != nil || !found || value != replicas {
			return fail("statefulset-replicas-incomplete")
		}
	}
	c.StartOrdinal, _, err = unstructured.NestedInt64(live.Object, "spec", "ordinals", "start")
	if err != nil || c.StartOrdinal < 0 {
		return fail("statefulset-ordinals-unreadable")
	}
	c.CurrentRevision, _, _ = unstructured.NestedString(live.Object, "status", "currentRevision")
	c.UpdateRevision, _, _ = unstructured.NestedString(live.Object, "status", "updateRevision")
	if c.CurrentRevision == "" || c.CurrentRevision != c.UpdateRevision {
		return fail("statefulset-revision-unconfirmed")
	}
	strategy, _, err := unstructured.NestedString(live.Object, "spec", "updateStrategy", "type")
	if err != nil || (strategy != "" && strategy != "RollingUpdate" && strategy != "OnDelete") {
		return fail("statefulset-strategy-unsupported")
	}
	partition, _, err := unstructured.NestedInt64(live.Object, "spec", "updateStrategy", "rollingUpdate", "partition")
	if err != nil || partition > 0 || partition < 0 {
		return fail("statefulset-partition-unconfirmed")
	}
	if revision == nil || revision.GetAPIVersion() != "apps/v1" || revision.GetKind() != "ControllerRevision" || revision.GetName() != c.CurrentRevision || revision.GetNamespace() != live.GetNamespace() || revision.GetUID() == "" || hasImageDeletionMetadata(revision) {
		return fail("statefulset-revision-identity-unconfirmed")
	}
	owner := ControllerOwner(revision, "StatefulSet")
	if owner == nil || owner.UID != live.GetUID() || owner.Name != live.GetName() {
		return fail("statefulset-revision-owner-unconfirmed")
	}
	c.RevisionUID = string(revision.GetUID())
	template, tf, te := unstructured.NestedMap(live.Object, "spec", "template")
	recorded, rf, re := unstructured.NestedMap(revision.Object, "data", "spec", "template")
	// StatefulSet ControllerRevision data uses a strategic-merge replace marker.
	if marker, ok := recorded["$patch"].(string); !ok || marker != "replace" {
		return fail("statefulset-revision-template-unconfirmed")
	}
	delete(recorded, "$patch")
	if !tf || !rf || te != nil || re != nil || !reflect.DeepEqual(template, recorded) {
		return fail("statefulset-revision-template-unconfirmed")
	}
	if int64(len(pods)) != replicas {
		return fail("pod-count-incomplete")
	}
	selector, err := WorkloadPodSelector(live)
	if err != nil {
		return fail("selector-unsupported")
	}
	pods = append([]*unstructured.Unstructured(nil), pods...)
	sort.Slice(pods, func(i, j int) bool {
		if pods[i] == nil {
			return pods[j] != nil
		}
		if pods[j] == nil {
			return false
		}
		return pods[i].GetName() < pods[j].GetName()
	})
	seenUID, seenOrdinal := map[string]bool{}, map[int64]bool{}
	for _, pod := range pods {
		if pod == nil || pod.GetAPIVersion() != "v1" || pod.GetKind() != "Pod" || pod.GetUID() == "" || pod.GetNamespace() != live.GetNamespace() || !selector.Matches(labels.Set(pod.GetLabels())) {
			return fail("pod-identity-unconfirmed")
		}
		if hasImageDeletionMetadata(pod) {
			return fail("pod-terminating")
		}
		owner := ControllerOwner(pod, "StatefulSet")
		if owner == nil || owner.Name != live.GetName() || owner.UID != live.GetUID() {
			return fail("statefulset-pod-owner-unconfirmed")
		}
		suffix := strings.TrimPrefix(pod.GetName(), live.GetName()+"-")
		ordinal, err := strconv.ParseInt(suffix, 10, 64)
		if err != nil || suffix != strconv.FormatInt(ordinal, 10) || ordinal < c.StartOrdinal || ordinal-c.StartOrdinal >= replicas {
			return fail("statefulset-pod-ordinal-unconfirmed")
		}
		if seenUID[string(pod.GetUID())] || seenOrdinal[ordinal] {
			return fail("duplicate-pod-identity")
		}
		seenUID[string(pod.GetUID())], seenOrdinal[ordinal] = true, true
		if pod.GetLabels()["controller-revision-hash"] != c.CurrentRevision {
			return fail("statefulset-old-revision-observed")
		}
		if reason := runningImagePodReason(pod); reason != "" {
			return fail(reason)
		}
		c.OwnedPods++
	}
	c.Complete = true
	return c
}
