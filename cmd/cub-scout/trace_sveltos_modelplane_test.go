// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestBuildSveltosObservedTraceResult_UsesOwnerAndReferenceAnnotations(t *testing.T) {
	deploy := &unstructured.Unstructured{}
	deploy.SetAPIVersion("apps/v1")
	deploy.SetKind("Deployment")
	deploy.SetNamespace("prod")
	deploy.SetName("web")
	deploy.SetAnnotations(map[string]string{
		"projectsveltos.io/owner-kind":          "ClusterProfile",
		"projectsveltos.io/owner-name":          "config-to-production",
		"projectsveltos.io/reference-kind":      "ConfigMap",
		"projectsveltos.io/reference-name":      "webster-production",
		"projectsveltos.io/reference-namespace": "control-clusters-config",
	})

	profile := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"updatedClusters": []interface{}{
					map[string]interface{}{"name": "prod-east"},
				},
			},
		},
	}
	profile.SetAPIVersion("config.projectsveltos.io/v1beta1")
	profile.SetKind("ClusterProfile")
	profile.SetName("config-to-production")

	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), deploy, profile)
	result := buildSveltosObservedTraceResult(context.Background(), client, "Deployment", "web", "prod", &agent.Ownership{
		Type:    agent.OwnerSveltos,
		SubType: "clusterprofile",
		Name:    "config-to-production",
	})

	if result.Tool != "sveltos" || !result.FullyManaged {
		t.Fatalf("unexpected trace summary: tool=%q fullyManaged=%v", result.Tool, result.FullyManaged)
	}
	if len(result.Chain) != 3 {
		t.Fatalf("chain len = %d, want 3: %#v", len(result.Chain), result.Chain)
	}
	if result.Chain[0].Kind != "SveltosReference" {
		t.Fatalf("chain[0].Kind = %q, want SveltosReference", result.Chain[0].Kind)
	}
	if result.Chain[1].Kind != "ClusterProfile" || result.Chain[1].Name != "config-to-production" {
		t.Fatalf("unexpected owner link: %#v", result.Chain[1])
	}
	if result.Chain[2].Kind != "Deployment" || result.Chain[2].Name != "web" {
		t.Fatalf("unexpected workload link: %#v", result.Chain[2])
	}
}

func TestBuildModelplaneObservedTraceResult_LinksDeploymentAndComposedChildren(t *testing.T) {
	workload := &unstructured.Unstructured{}
	workload.SetAPIVersion("apps/v1")
	workload.SetKind("Deployment")
	workload.SetNamespace("models")
	workload.SetName("qwen-engine")
	workload.SetLabels(map[string]string{"modelplane.ai/deployment": "qwen"})

	modelDeployment := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"replicas": map[string]interface{}{
					"total": int64(1),
					"ready": int64(1),
				},
			},
		},
	}
	modelDeployment.SetAPIVersion("modelplane.ai/v1alpha1")
	modelDeployment.SetKind("ModelDeployment")
	modelDeployment.SetNamespace("models")
	modelDeployment.SetName("qwen")

	replica := &unstructured.Unstructured{}
	replica.SetAPIVersion("modelplane.ai/v1alpha1")
	replica.SetKind("ModelReplica")
	replica.SetNamespace("models")
	replica.SetName("qwen-0")
	replica.SetLabels(map[string]string{"modelplane.ai/deployment": "qwen"})

	listKinds := map[schema.GroupVersionResource]string{
		{Group: "modelplane.ai", Version: "v1alpha1", Resource: "modelreplicas"}:  "ModelReplicaList",
		{Group: "modelplane.ai", Version: "v1alpha1", Resource: "modelendpoints"}: "ModelEndpointList",
	}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds, workload, modelDeployment, replica)

	result := buildModelplaneObservedTraceResult(context.Background(), client, "Deployment", "qwen-engine", "models", &agent.Ownership{
		Type:      agent.OwnerModelplane,
		SubType:   "modeldeployment",
		Name:      "qwen",
		Namespace: "models",
		Source:    "label:modelplane.ai/deployment",
	})

	if result.Tool != "modelplane" || !result.FullyManaged {
		t.Fatalf("unexpected trace summary: tool=%q fullyManaged=%v", result.Tool, result.FullyManaged)
	}
	if len(result.Chain) != 2 {
		t.Fatalf("chain len = %d, want 2: %#v", len(result.Chain), result.Chain)
	}
	if result.Chain[0].Kind != "ModelDeployment" || result.Chain[0].Name != "qwen" {
		t.Fatalf("unexpected model owner link: %#v", result.Chain[0])
	}
	if len(result.Chain[0].Children) != 1 || result.Chain[0].Children[0].Kind != "ModelReplica" {
		t.Fatalf("unexpected model owner children: %#v", result.Chain[0].Children)
	}
	if result.Chain[1].Kind != "Deployment" || result.Chain[1].Name != "qwen-engine" {
		t.Fatalf("unexpected workload link: %#v", result.Chain[1])
	}
}

func TestNormalizeKind_FirstClassControllerAliases(t *testing.T) {
	tests := map[string]string{
		"clusterprofile":   "ClusterProfile",
		"clusterprofiles":  "ClusterProfile",
		"modeldeployment":  "ModelDeployment",
		"modeldeployments": "ModelDeployment",
		"md":               "ModelDeployment",
		"inferencegateway": "InferenceGateway",
	}
	for input, want := range tests {
		if got := normalizeKind(input); got != want {
			t.Fatalf("normalizeKind(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestFirstConditionTransition_IgnoresMissingConditions(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetCreationTimestamp(metav1.Now())
	if got := firstConditionTransition(obj); !got.IsZero() {
		t.Fatalf("firstConditionTransition() = %v, want zero", got)
	}
}
