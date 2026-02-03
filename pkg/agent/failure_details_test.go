// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestExtractFluxSourceFailure_OCIRepository_Healthy(t *testing.T) {
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "source.toolkit.fluxcd.io/v1beta2",
			"kind":       "OCIRepository",
			"metadata": map[string]interface{}{
				"name":      "app-manifests",
				"namespace": "flux-system",
			},
			"spec": map[string]interface{}{
				"url": "oci://ghcr.io/acme/app-manifests",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":               "Ready",
						"status":             "True",
						"reason":             "Succeeded",
						"message":            "stored artifact for digest 'sha256:abc123'",
						"lastTransitionTime": "2026-02-03T10:00:00Z",
					},
				},
				"artifact": map[string]interface{}{
					"revision": "sha256:abc123def456",
				},
			},
		},
	}

	details := ExtractFluxSourceFailure(resource)

	if details.Stage != StageHealthy {
		t.Errorf("Stage = %v, want %v", details.Stage, StageHealthy)
	}
	if !details.Ready {
		t.Error("Ready = false, want true")
	}
	if details.SourceURL != "oci://ghcr.io/acme/app-manifests" {
		t.Errorf("SourceURL = %v, want oci://ghcr.io/acme/app-manifests", details.SourceURL)
	}
	if details.ArtifactDigest != "sha256:abc123def456" {
		t.Errorf("ArtifactDigest = %v, want sha256:abc123def456", details.ArtifactDigest)
	}
}

func TestExtractFluxSourceFailure_OCIRepository_AuthFailed(t *testing.T) {
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "source.toolkit.fluxcd.io/v1beta2",
			"kind":       "OCIRepository",
			"metadata": map[string]interface{}{
				"name":      "private-manifests",
				"namespace": "flux-system",
			},
			"spec": map[string]interface{}{
				"url": "oci://ghcr.io/acme/private-manifests",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":    "Ready",
						"status":  "False",
						"reason":  "AuthenticationFailed",
						"message": "failed to login to OCI registry: UNAUTHORIZED",
					},
				},
			},
		},
	}

	details := ExtractFluxSourceFailure(resource)

	if details.Stage != StageSource {
		t.Errorf("Stage = %v, want %v", details.Stage, StageSource)
	}
	if details.Ready {
		t.Error("Ready = true, want false")
	}
	if details.Reason != "AuthenticationFailed" {
		t.Errorf("Reason = %v, want AuthenticationFailed", details.Reason)
	}
}

func TestExtractFluxSourceFailure_OCIRepository_NotFound(t *testing.T) {
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "source.toolkit.fluxcd.io/v1beta2",
			"kind":       "OCIRepository",
			"metadata": map[string]interface{}{
				"name":      "missing-tag",
				"namespace": "flux-system",
			},
			"spec": map[string]interface{}{
				"url": "oci://ghcr.io/acme/app-manifests",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":    "Ready",
						"status":  "False",
						"reason":  "OCIOperationFailed",
						"message": "manifest unknown: not found",
					},
				},
			},
		},
	}

	details := ExtractFluxSourceFailure(resource)

	if details.Stage != StageSource {
		t.Errorf("Stage = %v, want %v", details.Stage, StageSource)
	}
	if details.Reason != "OCIOperationFailed" {
		t.Errorf("Reason = %v, want OCIOperationFailed", details.Reason)
	}
}

func TestExtractFluxDeployerFailure_Kustomization_Healthy(t *testing.T) {
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
			"kind":       "Kustomization",
			"metadata": map[string]interface{}{
				"name":      "app-deploy",
				"namespace": "flux-system",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":    "Ready",
						"status":  "True",
						"reason":  "ReconciliationSucceeded",
						"message": "Applied revision: sha256:abc123",
					},
				},
				"lastAppliedRevision":   "sha256:abc123",
				"lastAttemptedRevision": "sha256:abc123",
			},
		},
	}

	details := ExtractFluxDeployerFailure(resource)

	if details.Stage != StageHealthy {
		t.Errorf("Stage = %v, want %v", details.Stage, StageHealthy)
	}
	if !details.Ready {
		t.Error("Ready = false, want true")
	}
	if details.HasRevisionMismatch() {
		t.Error("HasRevisionMismatch() = true, want false")
	}
}

func TestExtractFluxDeployerFailure_Kustomization_BuildFailed(t *testing.T) {
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
			"kind":       "Kustomization",
			"metadata": map[string]interface{}{
				"name":      "broken-app",
				"namespace": "flux-system",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":    "Ready",
						"status":  "False",
						"reason":  "BuildFailed",
						"message": "kustomize build failed: accumulating resources: yaml: line 15: error",
					},
				},
			},
		},
	}

	details := ExtractFluxDeployerFailure(resource)

	if details.Stage != StageBuild {
		t.Errorf("Stage = %v, want %v", details.Stage, StageBuild)
	}
	if details.Ready {
		t.Error("Ready = true, want false")
	}
	if details.Reason != "BuildFailed" {
		t.Errorf("Reason = %v, want BuildFailed", details.Reason)
	}
}

func TestExtractFluxDeployerFailure_Kustomization_SourceNotReady(t *testing.T) {
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
			"kind":       "Kustomization",
			"metadata": map[string]interface{}{
				"name":      "waiting-app",
				"namespace": "flux-system",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":    "Ready",
						"status":  "False",
						"reason":  "ArtifactFailed",
						"message": "Source 'OCIRepository/flux-system/manifests' is not ready, artifact not found",
					},
				},
			},
		},
	}

	details := ExtractFluxDeployerFailure(resource)

	if details.Stage != StageSource {
		t.Errorf("Stage = %v, want %v", details.Stage, StageSource)
	}
}

func TestExtractFluxDeployerFailure_Kustomization_RevisionMismatch(t *testing.T) {
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
			"kind":       "Kustomization",
			"metadata": map[string]interface{}{
				"name":      "lagging-app",
				"namespace": "flux-system",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":    "Ready",
						"status":  "False",
						"reason":  "ReconciliationFailed",
						"message": "dry-run failed",
					},
				},
				"lastAppliedRevision":   "sha256:old123",
				"lastAttemptedRevision": "sha256:new456",
			},
		},
	}

	details := ExtractFluxDeployerFailure(resource)

	if !details.HasRevisionMismatch() {
		t.Error("HasRevisionMismatch() = false, want true")
	}
	if details.LastAppliedRevision != "sha256:old123" {
		t.Errorf("LastAppliedRevision = %v, want sha256:old123", details.LastAppliedRevision)
	}
	if details.LastAttemptedRevision != "sha256:new456" {
		t.Errorf("LastAttemptedRevision = %v, want sha256:new456", details.LastAttemptedRevision)
	}
}

func TestExtractFluxDeployerFailure_Kustomization_Suspended(t *testing.T) {
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
			"kind":       "Kustomization",
			"metadata": map[string]interface{}{
				"name":      "suspended-app",
				"namespace": "flux-system",
			},
			"spec": map[string]interface{}{
				"suspend": true,
			},
			"status": map[string]interface{}{},
		},
	}

	details := ExtractFluxDeployerFailure(resource)

	if !details.Suspended {
		t.Error("Suspended = false, want true")
	}
	if details.Stage != StageHealthy {
		t.Errorf("Stage = %v, want %v (suspended resources are healthy)", details.Stage, StageHealthy)
	}
}

func TestExtractArgoFailure_Healthy(t *testing.T) {
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "my-app",
				"namespace": "argocd",
			},
			"status": map[string]interface{}{
				"sync": map[string]interface{}{
					"status":   "Synced",
					"revision": "abc123",
				},
				"health": map[string]interface{}{
					"status": "Healthy",
				},
				"operationState": map[string]interface{}{
					"phase":      "Succeeded",
					"finishedAt": "2026-02-03T10:00:00Z",
				},
			},
		},
	}

	details := ExtractArgoFailure(resource)

	if details.Stage != StageHealthy {
		t.Errorf("Stage = %v, want %v", details.Stage, StageHealthy)
	}
	if !details.Ready {
		t.Error("Ready = false, want true")
	}
	if details.SyncStatus != "Synced" {
		t.Errorf("SyncStatus = %v, want Synced", details.SyncStatus)
	}
	if details.HealthStatus != "Healthy" {
		t.Errorf("HealthStatus = %v, want Healthy", details.HealthStatus)
	}
}

func TestExtractArgoFailure_SyncFailed(t *testing.T) {
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "broken-app",
				"namespace": "argocd",
			},
			"status": map[string]interface{}{
				"sync": map[string]interface{}{
					"status": "OutOfSync",
				},
				"health": map[string]interface{}{
					"status":  "Degraded",
					"message": "Deployment has 0 available replicas",
				},
				"operationState": map[string]interface{}{
					"phase":   "Failed",
					"message": "one or more objects failed to apply",
				},
			},
		},
	}

	details := ExtractArgoFailure(resource)

	if details.Stage != StageSync {
		t.Errorf("Stage = %v, want %v", details.Stage, StageSync)
	}
	if details.Ready {
		t.Error("Ready = true, want false")
	}
	if details.OperationPhase != "Failed" {
		t.Errorf("OperationPhase = %v, want Failed", details.OperationPhase)
	}
}

func TestExtractArgoFailure_ChartNotFound(t *testing.T) {
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "missing-chart",
				"namespace": "argocd",
			},
			"status": map[string]interface{}{
				"sync": map[string]interface{}{
					"status": "Unknown",
				},
				"health": map[string]interface{}{
					"status": "Unknown",
				},
				"operationState": map[string]interface{}{
					"phase":   "Error",
					"message": "failed to fetch chart: chart not found",
				},
			},
		},
	}

	details := ExtractArgoFailure(resource)

	if details.Stage != StageSource {
		t.Errorf("Stage = %v, want %v", details.Stage, StageSource)
	}
	if details.OperationPhase != "Error" {
		t.Errorf("OperationPhase = %v, want Error", details.OperationPhase)
	}
}

func TestFailureDetails_IsHealthy(t *testing.T) {
	tests := []struct {
		name    string
		details FailureDetails
		want    bool
	}{
		{
			name: "healthy",
			details: FailureDetails{
				Stage: StageHealthy,
				Ready: true,
			},
			want: true,
		},
		{
			name: "not ready",
			details: FailureDetails{
				Stage: StageHealthy,
				Ready: false,
			},
			want: false,
		},
		{
			name: "suspended",
			details: FailureDetails{
				Stage:     StageHealthy,
				Ready:     true,
				Suspended: true,
			},
			want: false,
		},
		{
			name: "source failure",
			details: FailureDetails{
				Stage: StageSource,
				Ready: false,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.details.IsHealthy(); got != tt.want {
				t.Errorf("IsHealthy() = %v, want %v", got, tt.want)
			}
		})
	}
}
