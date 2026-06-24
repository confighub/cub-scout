// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestGitOpsStatusSummary_Format(t *testing.T) {
	tests := []struct {
		name       string
		summary    GitOpsSummary
		wantFields []string
	}{
		{
			name: "healthy flux OCI",
			summary: GitOpsSummary{
				Backend:   string(agent.BackendFlux),
				Transport: string(agent.TransportOCI),
				Deployers: []DeployerStatus{
					{
						Kind:      "Kustomization",
						Name:      "app-deploy",
						Namespace: "flux-system",
						Ready:     true,
						Stage:     string(agent.StageHealthy),
					},
				},
				Sources: []SourceStatus{
					{
						Kind:      "OCIRepository",
						Name:      "manifests",
						Namespace: "flux-system",
						Ready:     true,
					},
				},
			},
			wantFields: []string{"flux", "oci"},
		},
		{
			name: "failed flux source",
			summary: GitOpsSummary{
				Backend:   string(agent.BackendFlux),
				Transport: string(agent.TransportOCI),
				Deployers: []DeployerStatus{
					{
						Kind:      "Kustomization",
						Name:      "broken-app",
						Namespace: "flux-system",
						Ready:     false,
						Stage:     string(agent.StageSource),
						Reason:    "ArtifactFailed",
						Message:   "Source 'OCIRepository/flux-system/manifests' is not ready",
					},
				},
				Sources: []SourceStatus{
					{
						Kind:      "OCIRepository",
						Name:      "manifests",
						Namespace: "flux-system",
						Ready:     false,
						Reason:    "AuthenticationFailed",
						Message:   "failed to login to OCI registry: UNAUTHORIZED",
					},
				},
			},
			wantFields: []string{"flux", "oci", "source", "AuthenticationFailed"},
		},
		{
			name: "failed argo sync",
			summary: GitOpsSummary{
				Backend:   string(agent.BackendArgoCD),
				Transport: string(agent.TransportOCI),
				Deployers: []DeployerStatus{
					{
						Kind:         "Application",
						Name:         "broken-app",
						Namespace:    "argocd",
						Ready:        false,
						Stage:        string(agent.StageSync),
						SyncStatus:   "OutOfSync",
						HealthStatus: "Degraded",
						Reason:       "Failed",
						Message:      "one or more objects failed to apply",
					},
				},
			},
			wantFields: []string{"argocd", "sync", "Failed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify the summary structure contains expected values
			if tt.summary.Backend == "" {
				t.Error("Backend should not be empty")
			}
			for _, wantField := range tt.wantFields {
				found := false
				if tt.summary.Backend == wantField {
					found = true
				}
				if tt.summary.Transport == wantField {
					found = true
				}
				for _, d := range tt.summary.Deployers {
					if d.Stage == wantField || d.Reason == wantField {
						found = true
					}
				}
				for _, s := range tt.summary.Sources {
					if s.Reason == wantField {
						found = true
					}
				}
				if !found {
					t.Errorf("Expected to find %q in summary", wantField)
				}
			}
		})
	}
}

func TestDeployerStatus_IsHealthy(t *testing.T) {
	tests := []struct {
		name   string
		status DeployerStatus
		want   bool
	}{
		{
			name: "ready and healthy stage",
			status: DeployerStatus{
				Ready: true,
				Stage: string(agent.StageHealthy),
			},
			want: true,
		},
		{
			name: "not ready",
			status: DeployerStatus{
				Ready: false,
				Stage: string(agent.StageHealthy),
			},
			want: false,
		},
		{
			name: "source failure",
			status: DeployerStatus{
				Ready: false,
				Stage: string(agent.StageSource),
			},
			want: false,
		},
		{
			name: "suspended",
			status: DeployerStatus{
				Ready:     true,
				Stage:     string(agent.StageHealthy),
				Suspended: true,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsHealthy(); got != tt.want {
				t.Errorf("IsHealthy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSourceStatus_IsHealthy(t *testing.T) {
	tests := []struct {
		name   string
		status SourceStatus
		want   bool
	}{
		{
			name:   "ready",
			status: SourceStatus{Ready: true},
			want:   true,
		},
		{
			name:   "not ready",
			status: SourceStatus{Ready: false, Reason: "AuthenticationFailed"},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsHealthy(); got != tt.want {
				t.Errorf("IsHealthy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGitOpsSummary_HasFailures(t *testing.T) {
	tests := []struct {
		name    string
		summary GitOpsSummary
		want    bool
	}{
		{
			name: "all healthy",
			summary: GitOpsSummary{
				Deployers: []DeployerStatus{
					{Ready: true, Stage: string(agent.StageHealthy)},
				},
				Sources: []SourceStatus{
					{Ready: true},
				},
			},
			want: false,
		},
		{
			name: "source failure",
			summary: GitOpsSummary{
				Deployers: []DeployerStatus{
					{Ready: false, Stage: string(agent.StageSource)},
				},
				Sources: []SourceStatus{
					{Ready: false, Reason: "AuthenticationFailed"},
				},
			},
			want: true,
		},
		{
			name: "deployer failure",
			summary: GitOpsSummary{
				Deployers: []DeployerStatus{
					{Ready: false, Stage: string(agent.StageBuild)},
				},
				Sources: []SourceStatus{
					{Ready: true},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.summary.HasFailures(); got != tt.want {
				t.Errorf("HasFailures() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGitOpsSummary_GetFailureCount(t *testing.T) {
	summary := GitOpsSummary{
		Deployers: []DeployerStatus{
			{Ready: true, Stage: string(agent.StageHealthy)},
			{Ready: false, Stage: string(agent.StageSource)},
			{Ready: false, Stage: string(agent.StageBuild)},
		},
		Sources: []SourceStatus{
			{Ready: true},
			{Ready: false, Reason: "AuthenticationFailed"},
		},
	}

	if got := summary.GetFailedDeployerCount(); got != 2 {
		t.Errorf("GetFailedDeployerCount() = %v, want 2", got)
	}

	if got := summary.GetFailedSourceCount(); got != 1 {
		t.Errorf("GetFailedSourceCount() = %v, want 1", got)
	}
}

func TestBuildGitOpsSummaryIncludesFirstClassControllerDeployers(t *testing.T) {
	oldNamespace := gitopsNamespace
	gitopsNamespace = ""
	t.Cleanup(func() {
		gitopsNamespace = oldNamespace
	})

	clusterProfile := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "config.projectsveltos.io/v1beta1",
		"kind":       "ClusterProfile",
		"metadata": map[string]interface{}{
			"name": "config-to-production",
		},
		"status": map[string]interface{}{
			"updatedClusters": []interface{}{
				map[string]interface{}{"name": "prod"},
			},
		},
	}}
	modelDeployment := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "modelplane.ai/v1alpha1",
		"kind":       "ModelDeployment",
		"metadata": map[string]interface{}{
			"name":      "qwen-runtime",
			"namespace": "inference",
		},
		"status": map[string]interface{}{
			"replicas": map[string]interface{}{
				"total": int64(2),
				"ready": int64(1),
			},
		},
	}}

	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		firstClassControllerListKinds(),
		clusterProfile,
		modelDeployment,
	)
	summary := buildGitOpsSummary(context.Background(), client, &agent.ApplyBackendInfo{
		Backend:   agent.BackendNone,
		Transport: agent.TransportUnknown,
	})

	if summary.Backend != "controllers" {
		t.Fatalf("Backend = %q, want controllers", summary.Backend)
	}
	if got := len(summary.Deployers); got != 2 {
		t.Fatalf("len(Deployers) = %d, want 2", got)
	}
	if summary.HealthyCount != 1 || summary.FailedCount != 1 {
		t.Fatalf("Healthy/Failed = %d/%d, want 1/1", summary.HealthyCount, summary.FailedCount)
	}

	byKind := map[string]DeployerStatus{}
	for _, deployer := range summary.Deployers {
		byKind[deployer.Kind] = deployer
	}
	if got := byKind["ClusterProfile"]; got.Owner != "Sveltos" || !got.Ready {
		t.Fatalf("ClusterProfile status = %+v, want Sveltos ready", got)
	}
	if got := byKind["ModelDeployment"]; got.Owner != "Modelplane" || got.Ready || got.Stage != string(agent.StageSync) {
		t.Fatalf("ModelDeployment status = %+v, want Modelplane sync failure", got)
	}
}

func firstClassControllerListKinds() map[schema.GroupVersionResource]string {
	out := map[schema.GroupVersionResource]string{}
	for _, spec := range firstClassControllerResources() {
		out[spec.GVR] = spec.Kind + "List"
	}
	return out
}

func TestGitOpsTraceCommandOmitsEmptyNamespace(t *testing.T) {
	got := gitopsTraceCommand(DeployerStatus{Kind: "ClusterProfile", Name: "prod"})
	if got != "./cub-scout trace clusterprofile/prod" {
		t.Fatalf("gitopsTraceCommand() = %q", got)
	}

	got = gitopsTraceCommand(DeployerStatus{Kind: "ModelDeployment", Name: "qwen", Namespace: "inference"})
	if got != "./cub-scout trace modeldeployment/qwen -n inference" {
		t.Fatalf("gitopsTraceCommand() = %q", got)
	}
}
