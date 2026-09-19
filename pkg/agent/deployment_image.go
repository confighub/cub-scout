// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"fmt"
	"reflect"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeploymentImageCoverage binds a complete pod set to one observed Deployment
// generation. Complete is coverage, not image equality or application success.
type DeploymentImageCoverage struct {
	Complete           bool   `json:"complete"`
	Reason             string `json:"reason,omitempty"`
	UID                string `json:"uid"`
	Generation         int64  `json:"generation"`
	ObservedGeneration int64  `json:"observedGeneration"`
	DesiredReplicas    int64  `json:"desiredReplicas"`
	Replicas           int64  `json:"replicas"`
	UpdatedReplicas    int64  `json:"updatedReplicas"`
	ReadyReplicas      int64  `json:"readyReplicas"`
	AvailableReplicas  int64  `json:"availableReplicas"`
	OwnedPods          int    `json:"ownedPods"`
}

// WorkloadPodSelector preserves both matchLabels and matchExpressions. Empty or
// invalid selectors must not widen a bounded observation to a namespace list.
func WorkloadPodSelector(live *unstructured.Unstructured) (labels.Selector, error) {
	if live == nil {
		return nil, fmt.Errorf("workload missing")
	}
	m, found, err := unstructured.NestedMap(live.Object, "spec", "selector")
	if err != nil || !found {
		return nil, fmt.Errorf("workload selector missing or malformed")
	}
	var selector metav1.LabelSelector
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(m, &selector); err != nil {
		return nil, err
	}
	s, err := metav1.LabelSelectorAsSelector(&selector)
	if err != nil || s.Empty() {
		return nil, fmt.Errorf("workload selector empty or invalid")
	}
	return s, nil
}

// ControllerOwner returns only an unambiguous controlling owner reference.
func ControllerOwner(obj *unstructured.Unstructured, kind string) *metav1.OwnerReference {
	if obj == nil {
		return nil
	}
	var found *metav1.OwnerReference
	for _, ref := range obj.GetOwnerReferences() {
		if ref.Controller == nil || !*ref.Controller {
			continue
		}
		if found != nil || ref.APIVersion != "apps/v1" || ref.Kind != kind || ref.Name == "" || ref.UID == "" {
			return nil
		}
		copy := ref
		found = &copy
	}
	return found
}

func BuildDeploymentImageCoverage(live *unstructured.Unstructured, pods []*unstructured.Unstructured, replicaSets map[string]*unstructured.Unstructured) DeploymentImageCoverage {
	pods = append([]*unstructured.Unstructured(nil), pods...)
	sort.Slice(pods, func(i, j int) bool {
		a, b := pods[i], pods[j]
		if a == nil {
			return b != nil
		}
		if b == nil {
			return false
		}
		if a.GetName() != b.GetName() {
			return a.GetName() < b.GetName()
		}
		return a.GetUID() < b.GetUID()
	})
	c := DeploymentImageCoverage{}
	fail := func(reason string) DeploymentImageCoverage { c.Reason = reason; return c }
	if live == nil || live.GetAPIVersion() != "apps/v1" || live.GetKind() != "Deployment" {
		return fail("deployment-evidence-missing")
	}
	c.UID, c.Generation = string(live.GetUID()), live.GetGeneration()
	c.ObservedGeneration, _, _ = unstructured.NestedInt64(live.Object, "status", "observedGeneration")
	if c.UID == "" || c.Generation <= 0 || c.ObservedGeneration != c.Generation {
		return fail("deployment-generation-unconfirmed")
	}
	if hasImageDeletionMetadata(live) {
		return fail("deployment-terminating")
	}
	var found bool
	var err error
	c.DesiredReplicas, found, err = unstructured.NestedInt64(live.Object, "spec", "replicas")
	if err != nil {
		return fail("deployment-replicas-unreadable")
	}
	if !found {
		c.DesiredReplicas = 1
	}
	if c.DesiredReplicas <= 0 {
		return fail("no-running-replicas")
	}
	for _, field := range []struct {
		name  string
		value *int64
	}{
		{"replicas", &c.Replicas}, {"updatedReplicas", &c.UpdatedReplicas}, {"readyReplicas", &c.ReadyReplicas}, {"availableReplicas", &c.AvailableReplicas},
	} {
		*field.value, found, err = unstructured.NestedInt64(live.Object, "status", field.name)
		if err != nil || !found || *field.value != c.DesiredReplicas {
			return fail("deployment-replicas-incomplete")
		}
	}
	if int64(len(pods)) != c.DesiredReplicas {
		return fail("pod-count-incomplete")
	}
	selector, err := WorkloadPodSelector(live)
	if err != nil {
		return fail("selector-unsupported")
	}
	seen := map[string]bool{}
	for _, pod := range pods {
		if pod == nil || pod.GetUID() == "" || pod.GetName() == "" || pod.GetNamespace() != live.GetNamespace() || !selector.Matches(labels.Set(pod.GetLabels())) {
			return fail("pod-identity-unconfirmed")
		}
		if seen[string(pod.GetUID())] || seen["name:"+pod.GetName()] {
			return fail("duplicate-pod-identity")
		}
		seen[string(pod.GetUID())], seen["name:"+pod.GetName()] = true, true
		if hasImageDeletionMetadata(pod) {
			return fail("pod-terminating")
		}
		owner := ControllerOwner(pod, "ReplicaSet")
		if owner == nil {
			return fail("pod-owner-unconfirmed")
		}
		rs := replicaSets[owner.Name]
		if rs == nil || rs.GetAPIVersion() != "apps/v1" || rs.GetKind() != "ReplicaSet" || rs.GetName() != owner.Name || rs.GetUID() != owner.UID || rs.GetNamespace() != live.GetNamespace() || hasImageDeletionMetadata(rs) {
			return fail("replicaset-identity-unconfirmed")
		}
		deployment := ControllerOwner(rs, "Deployment")
		if deployment == nil || deployment.UID != live.GetUID() || deployment.Name != live.GetName() {
			return fail("deployment-owner-unconfirmed")
		}
		if !sameDeploymentTemplate(live, rs) {
			return fail("old-replicaset-observed")
		}
		if reason := runningImagePodReason(pod); reason != "" {
			return fail(reason)
		}
		c.OwnedPods++
	}
	c.Complete = true
	return c
}

func runningImagePodReason(pod *unstructured.Unstructured) string {
	if pod == nil {
		return "pod-unreadable"
	}
	if hasImageDeletionMetadata(pod) {
		return "pod-terminating"
	}
	phase, _, _ := unstructured.NestedString(pod.Object, "status", "phase")
	if phase != "Running" {
		return "pod-not-running"
	}
	conditions, _, _ := unstructured.NestedSlice(pod.Object, "status", "conditions")
	ready, readyCount := false, 0
	for _, item := range conditions {
		if m, ok := item.(map[string]interface{}); ok && m["type"] == "Ready" {
			ready = m["status"] == "True"
			readyCount++
		}
	}
	if !ready || readyCount != 1 {
		return "pod-not-ready"
	}
	return ""
}

func hasImageDeletionMetadata(obj *unstructured.Unstructured) bool {
	v, found, err := unstructured.NestedFieldNoCopy(obj.Object, "metadata", "deletionTimestamp")
	return err != nil || (found && v != nil)
}

func sameDeploymentTemplate(deployment, rs *unstructured.Unstructured) bool {
	a, af, ae := unstructured.NestedMap(deployment.Object, "spec", "template")
	b, bf, be := unstructured.NestedMap(rs.Object, "spec", "template")
	if !af || !bf || ae != nil || be != nil {
		return false
	}
	// The Deployment controller adds only this identity label to its RS template.
	unstructured.RemoveNestedField(a, "metadata", "labels", "pod-template-hash")
	unstructured.RemoveNestedField(b, "metadata", "labels", "pod-template-hash")
	return reflect.DeepEqual(a, b)
}
