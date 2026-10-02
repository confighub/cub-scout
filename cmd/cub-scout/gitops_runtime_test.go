// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestEnrichArgoApplicationRuntimeStatus_AddsPodReadinessAndIssues(t *testing.T) {
	app := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "myapp-prod",
				"namespace": "argocd",
			},
			"spec": map[string]interface{}{
				"destination": map[string]interface{}{
					"namespace": "myapp-prod",
				},
			},
		},
	}

	imagePullPod := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "api-1",
				"namespace": "myapp-prod",
				"labels": map[string]interface{}{
					"argocd.argoproj.io/instance": "myapp-prod",
				},
			},
			"status": map[string]interface{}{
				"phase": "Pending",
				"containerStatuses": []interface{}{
					map[string]interface{}{
						"name":  "api",
						"ready": false,
						"state": map[string]interface{}{
							"waiting": map[string]interface{}{
								"reason":  "ImagePullBackOff",
								"message": "pull access denied",
							},
						},
					},
				},
			},
		},
	}

	runningPod := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "worker-1",
				"namespace": "myapp-prod",
				"labels": map[string]interface{}{
					"argocd.argoproj.io/instance": "myapp-prod",
				},
			},
			"status": map[string]interface{}{
				"phase": "Running",
				"containerStatuses": []interface{}{
					map[string]interface{}{
						"name":  "worker",
						"ready": true,
						"state": map[string]interface{}{
							"running": map[string]interface{}{},
						},
					},
				},
			},
		},
	}

	client := newGitOpsRuntimeFakeClient(app, imagePullPod, runningPod)
	status := &DeployerStatus{
		Kind:         "Application",
		Name:         "myapp-prod",
		Namespace:    "argocd",
		SyncStatus:   "Unknown",
		HealthStatus: "Healthy",
	}

	enrichArgoApplicationRuntimeStatus(context.Background(), client, status, app)

	if status.PodTotal != 2 {
		t.Fatalf("expected PodTotal=2, got %d", status.PodTotal)
	}
	if status.PodReady != 1 {
		t.Fatalf("expected PodReady=1, got %d", status.PodReady)
	}
	if len(status.RuntimeIssues) == 0 {
		t.Fatalf("expected runtime issues to be populated")
	}

	issue := status.RuntimeIssues[0]
	if issue.Reason != "ImagePullBackOff" {
		t.Fatalf("expected runtime reason ImagePullBackOff, got %q", issue.Reason)
	}
	if issue.Count != 1 {
		t.Fatalf("expected runtime issue count 1, got %d", issue.Count)
	}
}

func TestEnrichArgoApplicationRuntimeStatusMissingNamespaceIsOmittedNotHealthy(t *testing.T) {
	app := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata":   map[string]interface{}{"name": "app", "namespace": "argocd"},
		"spec":       map[string]interface{}{"destination": map[string]interface{}{}},
	}}
	client := newGitOpsRuntimeFakeClient()
	status := &DeployerStatus{Kind: "Application", Name: "app", Ready: true, Stage: "healthy", HealthStatus: "Healthy"}
	enrichArgoApplicationRuntimeStatus(context.Background(), client, status, app)
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	omission, ok := result["runtimeOmission"].(map[string]interface{})
	if !ok || omission["resource"] != "pods" || omission["reason"] != "destination_namespace_missing" {
		t.Fatalf("missing runtime namespace must be explicit and scoped, got: %s", encoded)
	}
	if !status.Ready || status.HealthStatus != "Healthy" || status.PodTotal != 0 || len(status.RuntimeIssues) != 0 {
		t.Fatalf("missing runtime metadata must not invent health or pod evidence: %+v", status)
	}
	if actions := client.Actions(); len(actions) != 0 {
		t.Fatalf("missing namespace should not issue a pod read, got %d actions", len(actions))
	}
}

func TestOutputDeployerStatus_WarnsWhenArgoHealthyButPodsNotReady(t *testing.T) {
	deployer := DeployerStatus{
		Kind:         "Application",
		Name:         "myapp-prod",
		Namespace:    "argocd",
		Ready:        false,
		Stage:        "sync",
		SyncStatus:   "Unknown",
		HealthStatus: "Healthy",
		PodReady:     0,
		PodTotal:     6,
		RuntimeIssues: []RuntimeIssue{
			{Reason: "ImagePullBackOff", Count: 6},
		},
	}

	out := captureStdout(t, func() {
		outputDeployerStatus(deployer)
	})

	if !strings.Contains(out, "Health:") || !strings.Contains(out, "Healthy (ArgoCD)") {
		t.Fatalf("expected ArgoCD health label in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Pods: 0/6 running") {
		t.Fatalf("expected pod readiness line in output, got:\n%s", out)
	}
	if !strings.Contains(out, "ArgoCD health may be misleading") {
		t.Fatalf("expected contradiction warning in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Runtime issues:") || !strings.Contains(out, "ImagePullBackOff: 6 pod(s)") {
		t.Fatalf("expected runtime issue breakdown in output, got:\n%s", out)
	}
}

func newGitOpsRuntimeFakeClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{
		{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}: "ApplicationList",
		{Group: "", Version: "v1", Resource: "pods"}:                          "PodList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objects...)
}
