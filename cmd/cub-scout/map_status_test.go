// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func statusObj(kind string, spec, status map[string]interface{}) *unstructured.Unstructured {
	obj := map[string]interface{}{"apiVersion": "v1", "kind": kind, "metadata": map[string]interface{}{"name": "x", "namespace": "ns"}}
	if spec != nil {
		obj["spec"] = spec
	}
	if status != nil {
		obj["status"] = status
	}
	return &unstructured.Unstructured{Object: obj}
}

func waiting(reason string) map[string]interface{} {
	return map[string]interface{}{"containerStatuses": []interface{}{
		map[string]interface{}{"name": "c", "state": map[string]interface{}{"waiting": map[string]interface{}{"reason": reason}}},
	}}
}

// #633: map reported idle Deployments, every DaemonSet and pods that could not
// pull their image as Pending, so doctor counted hundreds of false warnings and
// no errors.
func TestDetectStatusWorkloadsAndPods(t *testing.T) {
	for _, tc := range []struct {
		name string
		obj  *unstructured.Unstructured
		want string
	}{
		{"deployment scaled to zero and converged", statusObj("Deployment", map[string]interface{}{"replicas": int64(0)}, map[string]interface{}{"observedGeneration": int64(1)}), "Ready"},
		{"deployment scaling to zero, a pod still up", statusObj("Deployment", map[string]interface{}{"replicas": int64(0)}, map[string]interface{}{"replicas": int64(1)}), "Pending"},
		{"deployment replicas unset defaults to 1 and is ready", statusObj("Deployment", map[string]interface{}{}, map[string]interface{}{"replicas": int64(1), "readyReplicas": int64(1)}), "Ready"},
		{"deployment with an unavailable replica", statusObj("Deployment", map[string]interface{}{"replicas": int64(2)}, map[string]interface{}{"replicas": int64(2), "readyReplicas": int64(1), "unavailableReplicas": int64(1)}), "NotReady"},
		{"statefulset scaled to zero", statusObj("StatefulSet", map[string]interface{}{"replicas": int64(0)}, map[string]interface{}{"replicas": int64(0)}), "Ready"},
		{"daemonset fully ready", statusObj("DaemonSet", map[string]interface{}{}, map[string]interface{}{"desiredNumberScheduled": int64(1), "numberReady": int64(1)}), "Ready"},
		{"daemonset with an unavailable pod", statusObj("DaemonSet", map[string]interface{}{}, map[string]interface{}{"desiredNumberScheduled": int64(3), "numberReady": int64(2), "numberUnavailable": int64(1)}), "NotReady"},
		{"pod pending on ImagePullBackOff", statusObj("Pod", nil, merge(map[string]interface{}{"phase": "Pending"}, waiting("ImagePullBackOff"))), "Failed"},
		{"pod pending on ErrImagePull", statusObj("Pod", nil, merge(map[string]interface{}{"phase": "Pending"}, waiting("ErrImagePull"))), "Failed"},
		{"pod pending while its container is created", statusObj("Pod", nil, merge(map[string]interface{}{"phase": "Pending"}, waiting("ContainerCreating"))), "Pending"},
		{"pod running but crash-looping", statusObj("Pod", nil, merge(map[string]interface{}{"phase": "Running"}, waiting("CrashLoopBackOff"))), "Failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := detectStatus(tc.obj); got != tc.want {
				t.Errorf("detectStatus = %q, want %q", got, tc.want)
			}
		})
	}
}

func merge(a, b map[string]interface{}) map[string]interface{} {
	for k, v := range b {
		a[k] = v
	}
	return a
}

// #805: map status, issues, workloads and dashboard used a second copy of the
// readiness logic, which read an explicit replicas: 0 as "0 of 1 ready". A
// Deployment scaled to zero made map status exit 1 while map list printed
// Ready for the same object.
func TestWorkloadReadinessAgreesWithListedStatus(t *testing.T) {
	for _, tc := range []struct {
		name        string
		obj         *unstructured.Unstructured
		wantDesired int64
		wantReady   int64
		wantOK      bool
	}{
		{"deployment scaled to zero", statusObj("Deployment", map[string]interface{}{"replicas": int64(0)}, map[string]interface{}{}), 0, 0, true},
		{"statefulset scaled to zero", statusObj("StatefulSet", map[string]interface{}{"replicas": int64(0)}, map[string]interface{}{}), 0, 0, true},
		{"scaled to zero, pods still terminating", statusObj("Deployment", map[string]interface{}{"replicas": int64(0)}, map[string]interface{}{"replicas": int64(2)}), 0, 0, false},
		{"replicas unset, one ready", statusObj("Deployment", map[string]interface{}{}, map[string]interface{}{"readyReplicas": int64(1)}), 1, 1, true},
		{"replicas unset, no status yet", statusObj("Deployment", map[string]interface{}{}, map[string]interface{}{}), 1, 0, false},
		{"partially ready", statusObj("Deployment", map[string]interface{}{"replicas": int64(3)}, map[string]interface{}{"readyReplicas": int64(1)}), 3, 1, false},
		{"fully ready", statusObj("StatefulSet", map[string]interface{}{"replicas": int64(2)}, map[string]interface{}{"readyReplicas": int64(2)}), 2, 2, true},
		{"daemonset ready", statusObj("DaemonSet", map[string]interface{}{}, map[string]interface{}{"desiredNumberScheduled": int64(3), "numberReady": int64(3)}), 3, 3, true},
		{"daemonset not ready", statusObj("DaemonSet", map[string]interface{}{}, map[string]interface{}{"desiredNumberScheduled": int64(3), "numberReady": int64(1)}), 3, 1, false},
		{"daemonset with no eligible nodes", statusObj("DaemonSet", map[string]interface{}{}, map[string]interface{}{"desiredNumberScheduled": int64(0)}), 0, 0, true},
		{"daemonset with no status yet", statusObj("DaemonSet", map[string]interface{}{}, map[string]interface{}{}), 1, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			desired, ready := getWorkloadReplicas(tc.obj)
			if desired != tc.wantDesired || ready != tc.wantReady {
				t.Errorf("getWorkloadReplicas() = (%d, %d), want (%d, %d)", desired, ready, tc.wantDesired, tc.wantReady)
			}
			if got := isWorkloadReady(tc.obj); got != tc.wantOK {
				t.Errorf("isWorkloadReady() = %v, want %v", got, tc.wantOK)
			}
			if listed := detectStatus(tc.obj) == "Ready"; listed != isWorkloadReady(tc.obj) {
				t.Errorf("map list says ready=%v (%s) but map status says ready=%v", listed, detectStatus(tc.obj), isWorkloadReady(tc.obj))
			}
		})
	}
}
