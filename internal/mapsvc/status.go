// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package mapsvc

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Status constants for resources.
const (
	StatusReady    = "Ready"
	StatusNotReady = "NotReady"
	StatusFailed   = "Failed"
	StatusPending  = "Pending"
	StatusUnknown  = "Unknown"
)

// DetectStatus determines the status of a Kubernetes resource.
// It examines conditions, phase, and other status fields to determine
// whether a resource is ready, pending, failed, or unknown.
func DetectStatus(obj *unstructured.Unstructured) string {
	kind := obj.GetKind()
	status, _, _ := unstructured.NestedMap(obj.Object, "status")
	if status == nil {
		return StatusUnknown
	}

	// Check for Flux-style Ready condition
	conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if found {
		for _, c := range conditions {
			cond, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			condType, _ := cond["type"].(string)
			condStatus, _ := cond["status"].(string)
			if condType == "Ready" {
				switch condStatus {
				case "True":
					return StatusReady
				case "False":
					return StatusNotReady
				default:
					return StatusPending
				}
			}
		}
	}

	// Check for phase field (used by Pods, PVCs, etc.)
	if phase, ok := status["phase"].(string); ok {
		switch phase {
		case "Running", "Succeeded", "Bound", "Active":
			return StatusReady
		case "Pending", "ContainerCreating":
			return StatusPending
		case "Failed", "Error", "CrashLoopBackOff":
			return StatusFailed
		}
	}

	// Check for Argo CD Application
	if kind == "Application" {
		return detectArgoStatus(obj)
	}

	// Check for Deployment readiness
	if kind == "Deployment" {
		return detectDeploymentStatus(obj)
	}

	// Check for StatefulSet readiness
	if kind == "StatefulSet" {
		return detectStatefulSetStatus(obj)
	}

	// Check for DaemonSet readiness
	if kind == "DaemonSet" {
		return detectDaemonSetStatus(obj)
	}

	return StatusUnknown
}

// detectArgoStatus determines the status of an Argo CD Application.
func detectArgoStatus(obj *unstructured.Unstructured) string {
	health, _, _ := unstructured.NestedString(obj.Object, "status", "health", "status")
	sync, _, _ := unstructured.NestedString(obj.Object, "status", "sync", "status")

	if health == "Healthy" && sync == "Synced" {
		return StatusReady
	}
	if health == "Degraded" || health == "Missing" {
		return StatusFailed
	}
	if sync == "OutOfSync" || health == "Progressing" {
		return StatusNotReady
	}
	return StatusUnknown
}

// detectDeploymentStatus determines the status of a Deployment.
func detectDeploymentStatus(obj *unstructured.Unstructured) string {
	replicas, _, _ := unstructured.NestedInt64(obj.Object, "status", "replicas")
	readyReplicas, _, _ := unstructured.NestedInt64(obj.Object, "status", "readyReplicas")
	updatedReplicas, _, _ := unstructured.NestedInt64(obj.Object, "status", "updatedReplicas")
	availableReplicas, _, _ := unstructured.NestedInt64(obj.Object, "status", "availableReplicas")

	desiredReplicas, _, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas")
	if desiredReplicas == 0 {
		desiredReplicas = 1 // Default to 1 if not specified
	}

	if readyReplicas == desiredReplicas && availableReplicas == desiredReplicas {
		return StatusReady
	}
	if replicas == 0 && desiredReplicas > 0 {
		return StatusPending
	}
	if updatedReplicas < desiredReplicas || readyReplicas < desiredReplicas {
		return StatusNotReady
	}
	return StatusUnknown
}

// detectStatefulSetStatus determines the status of a StatefulSet.
func detectStatefulSetStatus(obj *unstructured.Unstructured) string {
	replicas, _, _ := unstructured.NestedInt64(obj.Object, "status", "replicas")
	readyReplicas, _, _ := unstructured.NestedInt64(obj.Object, "status", "readyReplicas")

	desiredReplicas, _, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas")
	if desiredReplicas == 0 {
		desiredReplicas = 1
	}

	if readyReplicas == desiredReplicas {
		return StatusReady
	}
	if replicas == 0 && desiredReplicas > 0 {
		return StatusPending
	}
	return StatusNotReady
}

// detectDaemonSetStatus determines the status of a DaemonSet.
func detectDaemonSetStatus(obj *unstructured.Unstructured) string {
	desiredNumber, _, _ := unstructured.NestedInt64(obj.Object, "status", "desiredNumberScheduled")
	numberReady, _, _ := unstructured.NestedInt64(obj.Object, "status", "numberReady")

	if desiredNumber > 0 && numberReady == desiredNumber {
		return StatusReady
	}
	if numberReady == 0 {
		return StatusPending
	}
	return StatusNotReady
}

// IsResourceReady returns true if the resource is in a ready state.
func IsResourceReady(obj *unstructured.Unstructured) bool {
	return DetectStatus(obj) == StatusReady
}

// GetConditionReason extracts the reason from the Ready condition.
func GetConditionReason(obj *unstructured.Unstructured) string {
	conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if !found {
		return ""
	}

	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		condType, _ := cond["type"].(string)
		if condType == "Ready" {
			reason, _ := cond["reason"].(string)
			return reason
		}
	}
	return ""
}

// GetConditionMessage extracts the message from the Ready condition.
func GetConditionMessage(obj *unstructured.Unstructured) string {
	conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if !found {
		return ""
	}

	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		condType, _ := cond["type"].(string)
		if condType == "Ready" {
			message, _ := cond["message"].(string)
			return message
		}
	}
	return ""
}
