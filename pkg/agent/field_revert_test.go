// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestLookupCanonicalFieldValue(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name": "checkout",
				"labels": map[string]interface{}{
					"app.kubernetes.io/name": "checkout",
				},
			},
			"spec": map[string]interface{}{
				"replicas": int64(3),
				"paused":   false,
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{"name": "sidecar", "image": "envoy:v1"},
							map[string]interface{}{"name": "checkout", "image": "ghcr.io/shop/checkout:v2"},
						},
					},
				},
			},
		},
	}

	val, found := LookupCanonicalFieldValue(obj, `.spec.replicas`)
	if !found || val != int64(3) {
		t.Fatalf("replicas = (%v, %v), want (3, true)", val, found)
	}
	val, found = LookupCanonicalFieldValue(obj, `.spec.paused`)
	if !found || val != false {
		t.Fatalf("paused = (%v, %v), want (false, true)", val, found)
	}
	val, found = LookupCanonicalFieldValue(obj, `.metadata.labels["app.kubernetes.io/name"]`)
	if !found || val != "checkout" {
		t.Fatalf("label = (%v, %v), want (checkout, true)", val, found)
	}
	val, found = LookupCanonicalFieldValue(obj, `.spec.template.spec.containers[name="checkout"].image`)
	if !found || val != "ghcr.io/shop/checkout:v2" {
		t.Fatalf("keyed container image = (%v, %v), want (ghcr.io/shop/checkout:v2, true)", val, found)
	}
	val, found = LookupCanonicalFieldValue(obj, `.spec.template.spec.containers[0].image`)
	if !found || val != "envoy:v1" {
		t.Fatalf("indexed container image = (%v, %v), want (envoy:v1, true)", val, found)
	}
	if _, found := LookupCanonicalFieldValue(obj, `.spec.nonexistent`); found {
		t.Fatalf("expected missing path to return found=false")
	}
	if _, found := LookupCanonicalFieldValue(obj, `.spec.template.spec.containers[name="missing"].image`); found {
		t.Fatalf("expected missing keyed list item to return found=false")
	}
}

func TestPredictFieldRevert(t *testing.T) {
	fluxActive := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata":   map[string]interface{}{"name": "apps", "namespace": "flux-system"},
		"spec":       map[string]interface{}{"prune": true},
	}}
	fluxSuspended := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata":   map[string]interface{}{"name": "apps", "namespace": "flux-system"},
		"spec":       map[string]interface{}{"suspend": true},
	}}
	argoSelfHeal := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata":   map[string]interface{}{"name": "shop", "namespace": "argocd"},
		"spec": map[string]interface{}{
			"syncPolicy": map[string]interface{}{
				"automated": map[string]interface{}{"selfHeal": true},
			},
		},
	}}
	argoNoSelfHeal := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata":   map[string]interface{}{"name": "shop", "namespace": "argocd"},
		"spec": map[string]interface{}{
			"syncPolicy": map[string]interface{}{
				"automated": map[string]interface{}{"selfHeal": false},
			},
		},
	}}
	argoIgnored := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata":   map[string]interface{}{"name": "shop", "namespace": "argocd"},
		"spec": map[string]interface{}{
			"syncPolicy": map[string]interface{}{
				"automated": map[string]interface{}{"selfHeal": true},
			},
			"ignoreDifferences": []interface{}{
				map[string]interface{}{
					"kind":         "Deployment",
					"jsonPointers": []interface{}{"/spec/replicas"},
				},
			},
		},
	}}

	tests := []struct {
		name        string
		owner       Ownership
		deployer    *unstructured.Unstructured
		targetKind  string
		path        string
		wantVerdict string
		wantReason  string
		wantRef     string
	}{
		{"native no owner marker", Ownership{Type: OwnerKubernetes}, nil, "Deployment", ".spec.replicas", RevertVerdictUnknown, RevertReasonNoOwnerMarker, ""},
		{"flux deployer not read", Ownership{Type: OwnerFlux}, nil, "Deployment", ".spec.replicas", RevertVerdictUnknown, RevertReasonDeployerNotRead, ""},
		{"flux active", Ownership{Type: OwnerFlux}, fluxActive, "Deployment", ".spec.replicas", RevertVerdictLikelyReverted, RevertReasonFluxActive, "Kustomization/flux-system/apps"},
		{"flux suspended", Ownership{Type: OwnerFlux}, fluxSuspended, "Deployment", ".spec.replicas", RevertVerdictNotExpected, RevertReasonFluxSuspended, "Kustomization/flux-system/apps"},
		{"argo deployer not read", Ownership{Type: OwnerArgo}, nil, "Deployment", ".spec.replicas", RevertVerdictUnknown, RevertReasonDeployerNotRead, ""},
		{"argo self-heal enabled", Ownership{Type: OwnerArgo}, argoSelfHeal, "Deployment", ".spec.replicas", RevertVerdictLikelyReverted, RevertReasonArgoSelfHealEnabled, "Application/argocd/shop"},
		{"argo self-heal disabled", Ownership{Type: OwnerArgo}, argoNoSelfHeal, "Deployment", ".spec.replicas", RevertVerdictNotExpected, RevertReasonArgoSelfHealDisabled, "Application/argocd/shop"},
		{"argo ignoreDifferences", Ownership{Type: OwnerArgo}, argoIgnored, "Deployment", ".spec.replicas", RevertVerdictNotExpected, RevertReasonArgoFieldIgnored, "Application/argocd/shop"},
		{"unsupported controller", Ownership{Type: OwnerSveltos}, nil, "Deployment", ".spec.replicas", RevertVerdictUnknown, RevertReasonUnsupported, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := PredictFieldRevert(tc.owner, tc.deployer, tc.targetKind, tc.path)
			if got.Verdict != tc.wantVerdict || got.Reason != tc.wantReason || got.DeployerRef != tc.wantRef {
				t.Fatalf("PredictFieldRevert() = %+v, want verdict=%q reason=%q ref=%q", got, tc.wantVerdict, tc.wantReason, tc.wantRef)
			}
		})
	}
}
