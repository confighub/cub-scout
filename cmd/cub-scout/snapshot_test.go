// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestBuildOwnsRelations(t *testing.T) {
	// Create test resources simulating: Deployment -> ReplicaSet -> Pod

	deploymentUID := types.UID("deploy-uid-123")
	replicaSetUID := types.UID("rs-uid-456")
	podUID := types.UID("pod-uid-789")

	// Deployment (no owner)
	deployment := unstructured.Unstructured{}
	deployment.SetUID(deploymentUID)
	deployment.SetName("backend")
	deployment.SetNamespace("prod")
	deployment.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	})

	// ReplicaSet (owned by Deployment)
	replicaSet := unstructured.Unstructured{}
	replicaSet.SetUID(replicaSetUID)
	replicaSet.SetName("backend-xyz")
	replicaSet.SetNamespace("prod")
	replicaSet.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "ReplicaSet",
	})
	replicaSet.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "backend",
			UID:        deploymentUID,
		},
	})

	// Pod (owned by ReplicaSet)
	pod := unstructured.Unstructured{}
	pod.SetUID(podUID)
	pod.SetName("backend-xyz-abc")
	pod.SetNamespace("prod")
	pod.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Pod",
	})
	pod.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion: "apps/v1",
			Kind:       "ReplicaSet",
			Name:       "backend-xyz",
			UID:        replicaSetUID,
		},
	})

	items := []unstructured.Unstructured{deployment, replicaSet, pod}
	relations := buildOwnsRelations(items, "test-cluster")

	// Should have 2 relations:
	// 1. Deployment owns ReplicaSet
	// 2. ReplicaSet owns Pod
	if len(relations) != 2 {
		t.Errorf("expected 2 relations, got %d", len(relations))
	}

	// Check for Deployment -> ReplicaSet relation
	foundDeployToRS := false
	foundRSToPod := false

	for _, rel := range relations {
		if rel.Type != "owns" {
			t.Errorf("expected type 'owns', got %s", rel.Type)
		}

		// Deployment owns ReplicaSet
		if rel.From == "test-cluster/prod/apps/Deployment/backend" &&
			rel.To == "test-cluster/prod/apps/ReplicaSet/backend-xyz" {
			foundDeployToRS = true
		}

		// ReplicaSet owns Pod
		if rel.From == "test-cluster/prod/apps/ReplicaSet/backend-xyz" &&
			rel.To == "test-cluster/prod//Pod/backend-xyz-abc" {
			foundRSToPod = true
		}
	}

	if !foundDeployToRS {
		t.Error("missing Deployment -> ReplicaSet relation")
		t.Logf("relations: %+v", relations)
	}

	if !foundRSToPod {
		t.Error("missing ReplicaSet -> Pod relation")
		t.Logf("relations: %+v", relations)
	}
}

func TestBuildOwnsRelations_NoOwners(t *testing.T) {
	// Resource with no owner references should produce no relations
	deployment := unstructured.Unstructured{}
	deployment.SetUID("uid-123")
	deployment.SetName("standalone")
	deployment.SetNamespace("default")
	deployment.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	})

	items := []unstructured.Unstructured{deployment}
	relations := buildOwnsRelations(items, "test-cluster")

	if len(relations) != 0 {
		t.Errorf("expected 0 relations, got %d", len(relations))
	}
}

func TestBuildSelectsRelations(t *testing.T) {
	// Create a Service with selector and matching Pods

	// Service with selector app=backend
	service := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "backend-svc",
				"namespace": "prod",
				"uid":       "svc-uid-123",
			},
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{
					"app": "backend",
				},
			},
		},
	}

	// Pod that matches the selector
	matchingPod := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "backend-xyz-abc",
				"namespace": "prod",
				"uid":       "pod-uid-456",
				"labels": map[string]interface{}{
					"app": "backend",
				},
			},
		},
	}

	// Pod that doesn't match (different label)
	nonMatchingPod := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "frontend-xyz-abc",
				"namespace": "prod",
				"uid":       "pod-uid-789",
				"labels": map[string]interface{}{
					"app": "frontend",
				},
			},
		},
	}

	// Pod in different namespace (shouldn't match)
	differentNsPod := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "backend-other",
				"namespace": "staging",
				"uid":       "pod-uid-999",
				"labels": map[string]interface{}{
					"app": "backend",
				},
			},
		},
	}

	items := []unstructured.Unstructured{service, matchingPod, nonMatchingPod, differentNsPod}
	relations := buildSelectsRelations(items, "test-cluster")

	// Should have exactly 1 relation: Service -> matching Pod
	if len(relations) != 1 {
		t.Errorf("expected 1 relation, got %d", len(relations))
		t.Logf("relations: %+v", relations)
	}

	if len(relations) > 0 {
		rel := relations[0]
		if rel.Type != "selects" {
			t.Errorf("expected type 'selects', got %s", rel.Type)
		}
		if rel.From != "test-cluster/prod//Service/backend-svc" {
			t.Errorf("unexpected from: %s", rel.From)
		}
		if rel.To != "test-cluster/prod//Pod/backend-xyz-abc" {
			t.Errorf("unexpected to: %s", rel.To)
		}
	}
}

func TestBuildSelectsRelations_NoSelector(t *testing.T) {
	// Service without selector should produce no relations
	service := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "external-svc",
				"namespace": "prod",
				"uid":       "svc-uid-123",
			},
			"spec": map[string]interface{}{
				// No selector - e.g., ExternalName service
			},
		},
	}

	pod := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "some-pod",
				"namespace": "prod",
				"uid":       "pod-uid-456",
				"labels": map[string]interface{}{
					"app": "backend",
				},
			},
		},
	}

	items := []unstructured.Unstructured{service, pod}
	relations := buildSelectsRelations(items, "test-cluster")

	if len(relations) != 0 {
		t.Errorf("expected 0 relations, got %d", len(relations))
	}
}

func TestBuildMountsRelations(t *testing.T) {
	// Pod that mounts a ConfigMap and Secret
	pod := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "app-xyz",
				"namespace": "prod",
				"uid":       "pod-uid-123",
			},
			"spec": map[string]interface{}{
				"volumes": []interface{}{
					map[string]interface{}{
						"name": "config-vol",
						"configMap": map[string]interface{}{
							"name": "app-config",
						},
					},
					map[string]interface{}{
						"name": "secret-vol",
						"secret": map[string]interface{}{
							"secretName": "app-secrets",
						},
					},
					map[string]interface{}{
						"name": "empty-vol",
						"emptyDir": map[string]interface{}{},
					},
				},
			},
		},
	}

	items := []unstructured.Unstructured{pod}
	relations := buildMountsRelations(items, "test-cluster")

	// Should have 2 relations: ConfigMap and Secret (not emptyDir)
	if len(relations) != 2 {
		t.Errorf("expected 2 relations, got %d", len(relations))
		t.Logf("relations: %+v", relations)
	}

	foundConfigMap := false
	foundSecret := false

	for _, rel := range relations {
		if rel.Type != "mounts" {
			t.Errorf("expected type 'mounts', got %s", rel.Type)
		}
		if rel.From != "test-cluster/prod//Pod/app-xyz" {
			t.Errorf("unexpected from: %s", rel.From)
		}
		if rel.To == "test-cluster/prod//ConfigMap/app-config" {
			foundConfigMap = true
		}
		if rel.To == "test-cluster/prod//Secret/app-secrets" {
			foundSecret = true
		}
	}

	if !foundConfigMap {
		t.Error("missing Pod -> ConfigMap relation")
	}
	if !foundSecret {
		t.Error("missing Pod -> Secret relation")
	}
}

func TestBuildMountsRelations_NoVolumes(t *testing.T) {
	// Pod without volumes should produce no relations
	pod := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "simple-pod",
				"namespace": "prod",
				"uid":       "pod-uid-123",
			},
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "main",
						"image": "nginx",
					},
				},
			},
		},
	}

	items := []unstructured.Unstructured{pod}
	relations := buildMountsRelations(items, "test-cluster")

	if len(relations) != 0 {
		t.Errorf("expected 0 relations, got %d", len(relations))
	}
}

func TestBuildReferencesRelations_EnvFrom(t *testing.T) {
	// Pod with envFrom referencing ConfigMap and Secret
	pod := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "app-xyz",
				"namespace": "prod",
				"uid":       "pod-uid-123",
			},
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "main",
						"image": "app:latest",
						"envFrom": []interface{}{
							map[string]interface{}{
								"configMapRef": map[string]interface{}{
									"name": "app-env",
								},
							},
							map[string]interface{}{
								"secretRef": map[string]interface{}{
									"name": "app-secrets",
								},
							},
						},
					},
				},
			},
		},
	}

	items := []unstructured.Unstructured{pod}
	relations := buildReferencesRelations(items, "test-cluster")

	// Should have 2 relations
	if len(relations) != 2 {
		t.Errorf("expected 2 relations, got %d", len(relations))
		t.Logf("relations: %+v", relations)
	}

	foundConfigMap := false
	foundSecret := false

	for _, rel := range relations {
		if rel.Type != "references" {
			t.Errorf("expected type 'references', got %s", rel.Type)
		}
		if rel.From != "test-cluster/prod//Pod/app-xyz" {
			t.Errorf("unexpected from: %s", rel.From)
		}
		if rel.To == "test-cluster/prod//ConfigMap/app-env" {
			foundConfigMap = true
		}
		if rel.To == "test-cluster/prod//Secret/app-secrets" {
			foundSecret = true
		}
	}

	if !foundConfigMap {
		t.Error("missing Pod -> ConfigMap relation")
	}
	if !foundSecret {
		t.Error("missing Pod -> Secret relation")
	}
}

func TestBuildReferencesRelations_EnvValueFrom(t *testing.T) {
	// Pod with env[].valueFrom referencing ConfigMap and Secret
	pod := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "app-xyz",
				"namespace": "prod",
				"uid":       "pod-uid-123",
			},
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "main",
						"image": "app:latest",
						"env": []interface{}{
							map[string]interface{}{
								"name": "DB_HOST",
								"valueFrom": map[string]interface{}{
									"configMapKeyRef": map[string]interface{}{
										"name": "db-config",
										"key":  "host",
									},
								},
							},
							map[string]interface{}{
								"name": "DB_PASSWORD",
								"valueFrom": map[string]interface{}{
									"secretKeyRef": map[string]interface{}{
										"name": "db-secrets",
										"key":  "password",
									},
								},
							},
							map[string]interface{}{
								"name":  "STATIC_VAR",
								"value": "static-value",
							},
						},
					},
				},
			},
		},
	}

	items := []unstructured.Unstructured{pod}
	relations := buildReferencesRelations(items, "test-cluster")

	// Should have 2 relations (static env var doesn't count)
	if len(relations) != 2 {
		t.Errorf("expected 2 relations, got %d", len(relations))
		t.Logf("relations: %+v", relations)
	}

	foundConfigMap := false
	foundSecret := false

	for _, rel := range relations {
		if rel.To == "test-cluster/prod//ConfigMap/db-config" {
			foundConfigMap = true
		}
		if rel.To == "test-cluster/prod//Secret/db-secrets" {
			foundSecret = true
		}
	}

	if !foundConfigMap {
		t.Error("missing Pod -> ConfigMap relation")
	}
	if !foundSecret {
		t.Error("missing Pod -> Secret relation")
	}
}

func TestBuildReferencesRelations_NoDuplicates(t *testing.T) {
	// Pod that references the same Secret multiple times should produce only one relation
	pod := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "app-xyz",
				"namespace": "prod",
				"uid":       "pod-uid-123",
			},
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "main",
						"image": "app:latest",
						"envFrom": []interface{}{
							map[string]interface{}{
								"secretRef": map[string]interface{}{
									"name": "shared-secrets",
								},
							},
						},
						"env": []interface{}{
							map[string]interface{}{
								"name": "SECRET_KEY",
								"valueFrom": map[string]interface{}{
									"secretKeyRef": map[string]interface{}{
										"name": "shared-secrets",
										"key":  "key",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	items := []unstructured.Unstructured{pod}
	relations := buildReferencesRelations(items, "test-cluster")

	// Should have only 1 relation (deduplicated)
	if len(relations) != 1 {
		t.Errorf("expected 1 relation (deduplicated), got %d", len(relations))
		t.Logf("relations: %+v", relations)
	}
}

func newFakeSnapshotDynamicClient(objs ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{
		{Group: "apps", Version: "v1", Resource: "deployments"}:                      "DeploymentList",
		{Group: "apps", Version: "v1", Resource: "statefulsets"}:                     "StatefulSetList",
		{Group: "apps", Version: "v1", Resource: "daemonsets"}:                       "DaemonSetList",
		{Group: "batch", Version: "v1", Resource: "jobs"}:                            "JobList",
		{Group: "batch", Version: "v1", Resource: "cronjobs"}:                        "CronJobList",
		{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"}: "KustomizationList",
		{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"}:        "HelmReleaseList",
		{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"}:   "GitRepositoryList",
		{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "ocirepositories"}:   "OCIRepositoryList",
		{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}:             "ApplicationList",
		{Group: "", Version: "v1", Resource: "secrets"}:                                   "SecretList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objs...)
}

func TestSnapshotRecording_SecretSanitization(t *testing.T) {
	helmSecret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"type":       "helm.sh/release.v1",
			"metadata": map[string]interface{}{
				"name":      "sh.helm.release.v1.web.v3",
				"namespace": "prod",
				"labels": map[string]interface{}{
					"owner":   "helm",
					"name":    "web",
					"status":  "deployed",
					"version": "3",
				},
				"annotations": map[string]interface{}{
					"meta.helm.sh/release-name":                  "web",
					"kubectl.kubernetes.io/last-applied-configuration": "MUST-NOT-LEAK-LAST-APPLIED",
				},
			},
			"data": map[string]interface{}{
				"release": "SUPER-SECRET-HELM-PAYLOAD-BASE64",
			},
			"stringData": map[string]interface{}{
				"token": "SUPER-SECRET-TOKEN",
			},
		},
	}
	opaqueSecret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"type":       "Opaque",
			"metadata": map[string]interface{}{
				"name":      "app-db-creds",
				"namespace": "prod",
			},
			"data": map[string]interface{}{
				"password": "OPAQUE-SECRET-PASSWORD",
			},
		},
	}

	dynClient := newFakeSnapshotDynamicClient(helmSecret, opaqueSecret)
	captured, truncated, err := collectRecordedSnapshotObjects(context.Background(), dynClient, "prod", "", 200)
	if err != nil {
		t.Fatalf("collectRecordedSnapshotObjects failed: %v", err)
	}
	if truncated {
		t.Fatalf("expected truncated=false")
	}
	if len(captured) != 1 {
		t.Fatalf("expected 1 captured object (Helm release Secret only), got %d", len(captured))
	}

	got := captured[0]
	if got.GetName() != "sh.helm.release.v1.web.v3" {
		t.Fatalf("captured secret name = %q, want sh.helm.release.v1.web.v3", got.GetName())
	}
	if _, hasData := got.Object["data"]; hasData {
		t.Fatal("Secret.data must be strictly stripped")
	}
	if _, hasStringData := got.Object["stringData"]; hasStringData {
		t.Fatal("Secret.stringData must be strictly stripped")
	}
	if gotType, _, _ := unstructured.NestedString(got.Object, "type"); gotType != "helm.sh/release.v1" {
		t.Fatalf("Secret.type = %q, want helm.sh/release.v1", gotType)
	}
	if got.GetLabels()["owner"] != "helm" || got.GetLabels()["version"] != "3" {
		t.Fatalf("Helm labels not preserved: %#v", got.GetLabels())
	}
	if got.GetAnnotations()["meta.helm.sh/release-name"] != "web" {
		t.Fatalf("Helm annotation not preserved: %#v", got.GetAnnotations())
	}
	if _, hasLastApplied := got.GetAnnotations()["kubectl.kubernetes.io/last-applied-configuration"]; hasLastApplied {
		t.Fatal("kubectl last-applied-configuration annotation on Secret must be stripped")
	}

	// Verify neither YAML nor JSON serialized output contains any secret payload.
	observedAt := time.Date(2026, 11, 24, 22, 0, 0, 0, time.UTC)
	for _, format := range []string{snapshotFormatYAML, snapshotFormatJSON} {
		var buf bytes.Buffer
		if err := encodeRecordedSnapshot(&buf, format, captured, observedAt, "kind-prod", false); err != nil {
			t.Fatalf("encodeRecordedSnapshot(%s) failed: %v", format, err)
		}
		serialized := buf.String()
		for _, forbidden := range []string{"SUPER-SECRET-HELM-PAYLOAD-BASE64", "SUPER-SECRET-TOKEN", "OPAQUE-SECRET-PASSWORD", "MUST-NOT-LEAK-LAST-APPLIED", "app-db-creds"} {
			if strings.Contains(serialized, forbidden) {
				t.Fatalf("format=%s leaked forbidden secret content %q", format, forbidden)
			}
		}
	}
}

func TestSnapshotRecording_BoundedCapture(t *testing.T) {
	objs := make([]runtime.Object, 0, 5)
	for i := 1; i <= 5; i++ {
		objs = append(objs, &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name":      fmt.Sprintf("app-%02d", i),
					"namespace": "prod",
				},
			},
		})
	}

	dynClient := newFakeSnapshotDynamicClient(objs...)
	captured, truncated, err := collectRecordedSnapshotObjects(context.Background(), dynClient, "prod", "", 3)
	if err != nil {
		t.Fatalf("collectRecordedSnapshotObjects failed: %v", err)
	}
	if !truncated {
		t.Fatal("expected truncated=true when cluster has more objects than maxObjects")
	}
	if len(captured) != 3 {
		t.Fatalf("expected 3 captured objects, got %d", len(captured))
	}
	if captured[0].GetName() != "app-01" || captured[1].GetName() != "app-02" || captured[2].GetName() != "app-03" {
		t.Fatalf("expected deterministic sorted order, got %q, %q, %q", captured[0].GetName(), captured[1].GetName(), captured[2].GetName())
	}
}

func TestSnapshotRecording_YAMLAndJSONRoundTrip(t *testing.T) {
	deploy := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "api",
				"namespace": "prod",
				"labels": map[string]interface{}{
					"kustomize.toolkit.fluxcd.io/name":      "apps",
					"kustomize.toolkit.fluxcd.io/namespace": "flux-system",
				},
			},
			"spec":   map[string]interface{}{"replicas": int64(2)},
			"status": map[string]interface{}{"replicas": int64(2), "readyReplicas": int64(2)},
		},
	}
	ks := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
			"kind":       "Kustomization",
			"metadata": map[string]interface{}{
				"name":      "apps",
				"namespace": "flux-system",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Ready",
						"status": "True",
						"reason": "ReconciliationSucceeded",
					},
				},
			},
		},
	}

	dynClient := newFakeSnapshotDynamicClient(deploy, ks)
	captured, truncated, err := collectRecordedSnapshotObjects(context.Background(), dynClient, "", "", 200)
	if err != nil {
		t.Fatalf("collectRecordedSnapshotObjects failed: %v", err)
	}
	if truncated || len(captured) != 2 {
		t.Fatalf("captured=%d truncated=%v, want 2/false", len(captured), truncated)
	}

	observedAt := time.Date(2026, 11, 24, 21, 30, 0, 0, time.UTC)
	for _, format := range []string{snapshotFormatYAML, snapshotFormatJSON} {
		t.Run(format, func(t *testing.T) {
			var buf bytes.Buffer
			if err := encodeRecordedSnapshot(&buf, format, captured, observedAt, "prod-cluster", false); err != nil {
				t.Fatalf("encodeRecordedSnapshot failed: %v", err)
			}
			filePath := filepath.Join(t.TempDir(), "snap."+format)
			if err := os.WriteFile(filePath, buf.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			snap, err := readRecordedObjectSnapshot(filePath)
			if err != nil {
				t.Fatalf("readRecordedObjectSnapshot(%s) failed: %v", format, err)
			}
			if snap.ObservedAt != "2026-11-24T21:30:00Z" {
				t.Fatalf("snap.ObservedAt = %q, want 2026-11-24T21:30:00Z", snap.ObservedAt)
			}
			if snap.SourceContext != "prod-cluster" {
				t.Fatalf("snap.SourceContext = %q, want prod-cluster", snap.SourceContext)
			}
			if len(snap.Objects) != 2 {
				t.Fatalf("len(snap.Objects) = %d, want 2", len(snap.Objects))
			}
		})
	}
}

func TestSnapshotRecording_OverDocumentLimitUsesListYAML(t *testing.T) {
	objs := make([]*unstructured.Unstructured, 0, maxRecordedObjectDocuments+5)
	for i := 0; i < maxRecordedObjectDocuments+5; i++ {
		objs = append(objs, &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name":      fmt.Sprintf("svc-%03d", i),
					"namespace": "prod",
				},
			},
		})
	}
	var buf bytes.Buffer
	observedAt := time.Date(2026, 11, 24, 21, 45, 0, 0, time.UTC)
	if err := encodeRecordedSnapshot(&buf, snapshotFormatYAML, objs, observedAt, "large-cluster", false); err != nil {
		t.Fatalf("encodeRecordedSnapshot failed: %v", err)
	}
	snap, err := loadRecordedObjectSnapshot(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("loadRecordedObjectSnapshot failed: %v", err)
	}
	if len(snap.Objects) != maxRecordedObjectDocuments+5 {
		t.Fatalf("len(snap.Objects) = %d, want %d", len(snap.Objects), maxRecordedObjectDocuments+5)
	}
	if snap.ObservedAt != "2026-11-24T21:45:00Z" || snap.SourceContext != "large-cluster" {
		t.Fatalf("unexpected snapshot metadata: observedAt=%q sourceContext=%q", snap.ObservedAt, snap.SourceContext)
	}
}
