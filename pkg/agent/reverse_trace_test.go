// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestKindToGVR(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		wantErr  bool
		wantRes  string
	}{
		{name: "pod", kind: "Pod", wantErr: false, wantRes: "pods"},
		{name: "pod lowercase", kind: "pod", wantErr: false, wantRes: "pods"},
		{name: "deployment", kind: "Deployment", wantErr: false, wantRes: "deployments"},
		{name: "deploy alias", kind: "deploy", wantErr: false, wantRes: "deployments"},
		{name: "replicaset", kind: "ReplicaSet", wantErr: false, wantRes: "replicasets"},
		{name: "statefulset", kind: "StatefulSet", wantErr: false, wantRes: "statefulsets"},
		{name: "sts alias", kind: "sts", wantErr: false, wantRes: "statefulsets"},
		{name: "daemonset", kind: "DaemonSet", wantErr: false, wantRes: "daemonsets"},
		{name: "ds alias", kind: "ds", wantErr: false, wantRes: "daemonsets"},
		{name: "service", kind: "Service", wantErr: false, wantRes: "services"},
		{name: "svc alias", kind: "svc", wantErr: false, wantRes: "services"},
		{name: "configmap", kind: "ConfigMap", wantErr: false, wantRes: "configmaps"},
		{name: "cm alias", kind: "cm", wantErr: false, wantRes: "configmaps"},
		{name: "secret", kind: "Secret", wantErr: false, wantRes: "secrets"},
		{name: "kustomization", kind: "Kustomization", wantErr: false, wantRes: "kustomizations"},
		{name: "ks alias", kind: "ks", wantErr: false, wantRes: "kustomizations"},
		{name: "helmrelease", kind: "HelmRelease", wantErr: false, wantRes: "helmreleases"},
		{name: "hr alias", kind: "hr", wantErr: false, wantRes: "helmreleases"},
		{name: "gitrepository", kind: "GitRepository", wantErr: false, wantRes: "gitrepositories"},
		{name: "ocirepository", kind: "OCIRepository", wantErr: false, wantRes: "ocirepositories"},
		{name: "ocirepository lowercase", kind: "ocirepository", wantErr: false, wantRes: "ocirepositories"},
		{name: "helmrepository", kind: "HelmRepository", wantErr: false, wantRes: "helmrepositories"},
		{name: "bucket", kind: "Bucket", wantErr: false, wantRes: "buckets"},
		{name: "application", kind: "Application", wantErr: false, wantRes: "applications"},
		{name: "app alias", kind: "app", wantErr: false, wantRes: "applications"},
		{name: "unknown", kind: "UnknownKind", wantErr: true, wantRes: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gvr, err := KindToGVR(tt.kind)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantRes, gvr.Resource)
			}
		})
	}
}

func TestKindToResource(t *testing.T) {
	tests := []struct {
		kind     string
		wantRes  string
	}{
		{kind: "Pod", wantRes: "pods"},
		{kind: "ReplicaSet", wantRes: "replicasets"},
		{kind: "Deployment", wantRes: "deployments"},
		{kind: "StatefulSet", wantRes: "statefulsets"},
		{kind: "DaemonSet", wantRes: "daemonsets"},
		{kind: "Job", wantRes: "jobs"},
		{kind: "CronJob", wantRes: "cronjobs"},
		{kind: "Service", wantRes: "services"},
		{kind: "ConfigMap", wantRes: "configmaps"},
		{kind: "Secret", wantRes: "secrets"},
		{kind: "Ingress", wantRes: "ingresses"},
		{kind: "Kustomization", wantRes: "kustomizations"},
		{kind: "HelmRelease", wantRes: "helmreleases"},
		{kind: "GitRepository", wantRes: "gitrepositories"},
		{kind: "OCIRepository", wantRes: "ocirepositories"},
		{kind: "HelmRepository", wantRes: "helmrepositories"},
		{kind: "Bucket", wantRes: "buckets"},
		{kind: "Application", wantRes: "applications"},
		{kind: "Unknown", wantRes: ""},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			res := KindToResource(tt.kind)
			assert.Equal(t, tt.wantRes, res)
		})
	}
}

func TestReverseTraceResult_Structure(t *testing.T) {
	result := &ReverseTraceResult{
		Object: ResourceRef{
			Kind:      "Pod",
			Name:      "nginx-abc123",
			Namespace: "default",
		},
		K8sChain: []ChainLink{
			{Kind: "Pod", Name: "nginx-abc123", Namespace: "default", Ready: true, Status: "Running"},
			{Kind: "ReplicaSet", Name: "nginx-789", Namespace: "default", Ready: true, Status: "3/3 ready"},
			{Kind: "Deployment", Name: "nginx", Namespace: "default", Ready: true, Status: "3/3 ready"},
		},
		Owner: "flux",
		OwnerDetails: &Ownership{
			Type:    OwnerFlux,
			SubType: "kustomization",
			Name:    "apps",
		},
		TopResource: &ResourceRef{
			Kind:      "Deployment",
			Name:      "nginx",
			Namespace: "default",
		},
	}

	assert.Equal(t, "Pod", result.Object.Kind)
	assert.Equal(t, 3, len(result.K8sChain))
	assert.Equal(t, "flux", result.Owner)
	assert.Equal(t, "Deployment", result.TopResource.Kind)
}

func TestExtractOrphanMetadata(t *testing.T) {
	// Create an unstructured resource with last-applied-configuration
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":              "orphan-app",
				"namespace":         "default",
				"creationTimestamp": "2026-01-15T10:30:00Z",
				"labels": map[string]interface{}{
					"app":     "orphan-app",
					"version": "1.0.0",
				},
				"annotations": map[string]interface{}{
					"kubectl.kubernetes.io/last-applied-configuration": `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"orphan-app"}}`,
					"description": "Test deployment",
				},
			},
		},
	}

	meta := extractOrphanMetadata(resource)

	// Verify labels
	assert.NotNil(t, meta.Labels)
	assert.Equal(t, "orphan-app", meta.Labels["app"])
	assert.Equal(t, "1.0.0", meta.Labels["version"])

	// Verify annotations (excluding last-applied-config)
	assert.NotNil(t, meta.Annotations)
	assert.Equal(t, "Test deployment", meta.Annotations["description"])
	assert.Empty(t, meta.Annotations["kubectl.kubernetes.io/last-applied-configuration"])

	// Verify last-applied-config is captured separately
	assert.Contains(t, meta.LastAppliedConfig, "apiVersion")
	assert.Contains(t, meta.LastAppliedConfig, "orphan-app")

	// Verify creation timestamp
	assert.NotNil(t, meta.CreatedAt)
}

func TestExtractOrphanMetadata_NoLastApplied(t *testing.T) {
	// Create a resource without last-applied-configuration (kubectl create)
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":              "debug-config",
				"namespace":         "default",
				"creationTimestamp": "2026-01-20T14:00:00Z",
				"labels": map[string]interface{}{
					"app": "debug",
				},
			},
		},
	}

	meta := extractOrphanMetadata(resource)

	// Verify no last-applied-config
	assert.Empty(t, meta.LastAppliedConfig)

	// Verify labels still captured
	assert.Equal(t, "debug", meta.Labels["app"])

	// Verify creation timestamp
	assert.NotNil(t, meta.CreatedAt)
}
