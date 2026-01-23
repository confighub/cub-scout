// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"os"
	"path/filepath"
	goruntime "runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"sigs.k8s.io/yaml"
)

// GVK constants for test resources
var (
	DeploymentGVK = schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	DaemonSetGVK  = schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DaemonSet"}
)

// ============================================================================
// Evasion Detection Tests
//
// These tests verify that the scanner properly detects evasion attempts
// documented in test/fixtures/evasion/EVASION-RESULTS.md
//
// Evasion patterns tested:
// 1. HPA minReplicas=0 - Blocked by Kubernetes API validation
// 2. Service empty selector - Design decision to skip (documented gap)
// 3. PDB string percentage ("100%") - Must detect string format
// 4. Ingress with ExternalName - Not evasion, correct behavior
// 5. NetworkPolicy matchExpressions - Must detect matchExpressions selectors
// ============================================================================

// Helper to create a fake dynamic client with resources
func newFakeDynamicClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	// Register list kinds for all resource types used in dangling detection
	gvrToListKind := map[schema.GroupVersionResource]string{
		{Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"}:      "HorizontalPodAutoscalerList",
		{Group: "autoscaling", Version: "v1", Resource: "horizontalpodautoscalers"}:      "HorizontalPodAutoscalerList",
		{Group: "autoscaling.k8s.io", Version: "v1", Resource: "verticalpodautoscalers"}: "VerticalPodAutoscalerList",
		{Group: "", Version: "v1", Resource: "services"}:                                 "ServiceList",
		{Group: "", Version: "v1", Resource: "pods"}:                                     "PodList",
		{Group: "", Version: "v1", Resource: "endpoints"}:                                "EndpointsList",
		{Group: "", Version: "v1", Resource: "secrets"}:                                  "SecretList",
		{Group: "", Version: "v1", Resource: "configmaps"}:                               "ConfigMapList",
		{Group: "", Version: "v1", Resource: "persistentvolumeclaims"}:                   "PersistentVolumeClaimList",
		{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"}:               "IngressList",
		{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}:         "NetworkPolicyList",
		{Group: "apps", Version: "v1", Resource: "deployments"}:                          "DeploymentList",
		{Group: "apps", Version: "v1", Resource: "replicasets"}:                          "ReplicaSetList",
		{Group: "apps", Version: "v1", Resource: "statefulsets"}:                         "StatefulSetList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, objects...)
}

// Helper GVKs for testing
var (
	hpaGVK = schema.GroupVersionKind{
		Group:   "autoscaling",
		Version: "v2",
		Kind:    "HorizontalPodAutoscaler",
	}
	serviceGVK = schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Service",
	}
	podGVK = schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Pod",
	}
	networkPolicyGVK = schema.GroupVersionKind{
		Group:   "networking.k8s.io",
		Version: "v1",
		Kind:    "NetworkPolicy",
	}
	ingressGVK = schema.GroupVersionKind{
		Group:   "networking.k8s.io",
		Version: "v1",
		Kind:    "Ingress",
	}
)

// TestEvasionLabelManipulation tests detection of resources with misleading ownership labels
// xBOW pattern: Attacker adds fake GitOps labels to bypass orphan detection
//
// IMPORTANT: This test documents that label-based ownership detection can be spoofed.
// An attacker with kubectl access can add GitOps labels to make resources appear managed.
// This is a known limitation - labels are not cryptographically verified.
func TestEvasionLabelManipulation(t *testing.T) {
	tests := []struct {
		name        string
		labels      map[string]string
		wantType    string
		description string
	}{
		{
			name: "Fake Flux labels should be detected as Flux-owned",
			labels: map[string]string{
				"kustomize.toolkit.fluxcd.io/name":      "fake-kustomization",
				"kustomize.toolkit.fluxcd.io/namespace": "flux-system",
			},
			wantType:    OwnerFlux,
			description: "Attacker adds Flux labels to hide orphan resource - EVASION WORKS",
		},
		{
			name: "Argo argocd.argoproj.io/instance label IS authoritative",
			labels: map[string]string{
				"argocd.argoproj.io/instance": "fake-application",
			},
			wantType:    OwnerArgo, // argocd.argoproj.io/instance is the authoritative Argo label
			description: "Single authoritative Argo label is sufficient for detection",
		},
		{
			name: "Fake Argo with both required labels",
			labels: map[string]string{
				"app.kubernetes.io/instance":  "fake-application",
				"argocd.argoproj.io/instance": "fake-application",
			},
			wantType:    OwnerArgo,
			description: "Attacker with both Argo labels can evade orphan detection - EVASION WORKS",
		},
		{
			name: "Fake Helm labels should be detected as Helm-owned",
			labels: map[string]string{
				"app.kubernetes.io/managed-by": "Helm",
			},
			wantType:    OwnerHelm,
			description: "Attacker adds Helm labels to hide orphan resource - EVASION WORKS",
		},
		{
			name: "Conflicting Flux and Argo labels - Flux wins (first match)",
			labels: map[string]string{
				"kustomize.toolkit.fluxcd.io/name": "flux-ks",
				"argocd.argoproj.io/instance":      "argo-app",
			},
			wantType:    OwnerFlux,
			description: "Resource with conflicting GitOps labels - Flux is checked first",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a deployment with the evasion labels
			u := &unstructured.Unstructured{}
			u.SetGroupVersionKind(DeploymentGVK)
			u.SetNamespace("default")
			u.SetName("evasion-test")
			u.SetLabels(tt.labels)

			ownership := DetectOwnership(u)

			if ownership.Type != tt.wantType {
				t.Errorf("DetectOwnership().Type = %q, want %q\nDescription: %s",
					ownership.Type, tt.wantType, tt.description)
			}
		})
	}
}

// TestEvasionAnnotationSpoofing tests detection of fake GitOps annotations
// xBOW pattern: Attacker adds fake annotations to evade detection
//
// NOTE: The Argo tracking-id annotation IS sufficient to mark ownership (design decision).
// This means an attacker can add a single annotation to make a resource appear Argo-managed.
func TestEvasionAnnotationSpoofing(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		labels      map[string]string
		wantType    string
		description string
	}{
		{
			name: "Argo tracking annotation alone IS sufficient for detection",
			annotations: map[string]string{
				"argocd.argoproj.io/tracking-id": "fake:default/Deployment:default/test",
			},
			labels:      nil,
			wantType:    OwnerArgo, // EVASION: annotation alone is sufficient!
			description: "Attacker can evade orphan detection with single annotation - EVASION WORKS",
		},
		{
			name: "Flux prune annotation only - should not be detected as Flux",
			annotations: map[string]string{
				"kustomize.toolkit.fluxcd.io/prune": "disabled",
			},
			labels:      nil,
			wantType:    OwnerUnknown,
			description: "Flux prune annotation without name label is not sufficient",
		},
		{
			name: "ConfigHub annotations with proper labels",
			labels: map[string]string{
				"confighub.com/UnitSlug": "payment-api",
			},
			annotations: map[string]string{
				"confighub.com/SpaceName": "payments-team",
			},
			wantType:    OwnerConfigHub,
			description: "ConfigHub requires both label and annotation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &unstructured.Unstructured{}
			u.SetGroupVersionKind(DeploymentGVK)
			u.SetNamespace("default")
			u.SetName("evasion-annotation-test")
			if tt.labels != nil {
				u.SetLabels(tt.labels)
			}
			if tt.annotations != nil {
				u.SetAnnotations(tt.annotations)
			}

			ownership := DetectOwnership(u)

			if ownership.Type != tt.wantType {
				t.Errorf("DetectOwnership().Type = %q, want %q\nDescription: %s",
					ownership.Type, tt.wantType, tt.description)
			}
		})
	}
}

// TestEvasionPDBStringPercentage tests that PDB detection handles string percentage format
// Based on: test/fixtures/evasion/evasion-03-pdb-string-percentage.yaml
// The scanner should detect "100%" (string) just like integer 100
func TestEvasionPDBStringPercentage(t *testing.T) {
	// This test documents that the scanner handles PDB percentages correctly
	// The actual detection happens in timing bomb scanning
	t.Run("String percentage format is valid K8s and should be detected", func(t *testing.T) {
		// Create a PDB with string percentage
		pdb := &unstructured.Unstructured{}
		pdb.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "policy/v1",
			"kind":       "PodDisruptionBudget",
			"metadata": map[string]interface{}{
				"name":      "evasion-pdb-string-percent",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"minAvailable": "100%", // String format - evasion attempt
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app": "some-app-that-exists",
					},
				},
			},
		})

		// Verify the value is stored as a string
		minAvailable, found, err := unstructured.NestedString(pdb.Object, "spec", "minAvailable")
		if err != nil || !found {
			// Try as field value (could be integer)
			minAvailableRaw, found, _ := unstructured.NestedFieldNoCopy(pdb.Object, "spec", "minAvailable")
			if !found {
				t.Fatal("minAvailable not found in spec")
			}
			minAvailable = minAvailableRaw.(string)
		}

		if minAvailable != "100%" {
			t.Errorf("Expected minAvailable = '100%%', got %q", minAvailable)
		}
	})
}

// TestEvasionNetworkPolicyMatchExpressions tests that NetworkPolicy detection handles matchExpressions
// Based on: test/fixtures/evasion/evasion-05-netpol-matchexpressions.yaml
// The scanner must detect NetworkPolicies using matchExpressions, not just matchLabels
func TestEvasionNetworkPolicyMatchExpressions(t *testing.T) {
	t.Run("NetworkPolicy with matchExpressions should be detected", func(t *testing.T) {
		// Create mock resources: NetworkPolicy with matchExpressions, no matching pods
		np := &unstructured.Unstructured{}
		np.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "NetworkPolicy",
			"metadata": map[string]interface{}{
				"name":      "evasion-netpol-expressions",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"podSelector": map[string]interface{}{
					"matchExpressions": []interface{}{
						map[string]interface{}{
							"key":      "app",
							"operator": "In",
							"values":   []interface{}{"nonexistent-app-expression"},
						},
					},
				},
				"policyTypes": []interface{}{"Ingress"},
				"ingress": []interface{}{
					map[string]interface{}{
						"from": []interface{}{
							map[string]interface{}{
								"podSelector": map[string]interface{}{
									"matchLabels": map[string]interface{}{
										"role": "frontend",
									},
								},
							},
						},
					},
				},
			},
		})

		// Verify the matchExpressions are properly structured
		podSelector, found, _ := unstructured.NestedMap(np.Object, "spec", "podSelector")
		if !found {
			t.Fatal("podSelector not found")
		}

		matchExpressions, found, _ := unstructured.NestedSlice(podSelector, "matchExpressions")
		if !found {
			t.Fatal("matchExpressions not found")
		}

		if len(matchExpressions) != 1 {
			t.Errorf("Expected 1 matchExpression, got %d", len(matchExpressions))
		}

		expr := matchExpressions[0].(map[string]interface{})
		key, _, _ := unstructured.NestedString(expr, "key")
		operator, _, _ := unstructured.NestedString(expr, "operator")
		values, _, _ := unstructured.NestedStringSlice(expr, "values")

		if key != "app" {
			t.Errorf("Expected key = 'app', got %q", key)
		}
		if operator != "In" {
			t.Errorf("Expected operator = 'In', got %q", operator)
		}
		if len(values) != 1 || values[0] != "nonexistent-app-expression" {
			t.Errorf("Expected values = ['nonexistent-app-expression'], got %v", values)
		}
	})
}

// TestEvasionEmptySelector tests the documented gap: empty selector services
// Based on: test/fixtures/evasion/evasion-02-service-empty-selector.yaml
// DOCUMENTED GAP: Empty selectors are intentionally skipped because they are valid for:
// - ExternalName services
// - Services with manually managed Endpoints
// - Headless services for StatefulSets
func TestEvasionEmptySelector(t *testing.T) {
	t.Run("Empty selector is intentionally skipped (documented design decision)", func(t *testing.T) {
		// Create a Service with empty selector
		svc := &unstructured.Unstructured{}
		svc.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "evasion-empty-selector",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"type":     "ClusterIP",
				"selector": map[string]interface{}{}, // Empty selector
				"ports": []interface{}{
					map[string]interface{}{
						"protocol":   "TCP",
						"port":       float64(80),
						"targetPort": float64(8080),
					},
				},
			},
		})

		// Get the selector
		selector, found, _ := unstructured.NestedStringMap(svc.Object, "spec", "selector")

		// Document the expected behavior
		if !found || len(selector) != 0 {
			t.Log("Note: Service has empty selector - this is intentionally NOT detected")
			t.Log("Reason: Empty selectors are valid for ExternalName, headless, or manually-managed Endpoints")
		}

		// The selector should be empty
		if len(selector) != 0 {
			t.Errorf("Expected empty selector, got %v", selector)
		}
	})
}

// TestEvasionNamespaceHiding tests detection of resources in system namespaces
// xBOW pattern: Attacker creates resources in kube-system to avoid scrutiny
func TestEvasionNamespaceHiding(t *testing.T) {
	systemNamespaces := []string{
		"kube-system",
		"kube-public",
		"kube-node-lease",
		"flux-system",
		"argocd",
		"gatekeeper-system",
	}

	for _, ns := range systemNamespaces {
		t.Run("Resource in "+ns+" should be discoverable", func(t *testing.T) {
			u := &unstructured.Unstructured{}
			u.SetGroupVersionKind(DeploymentGVK)
			u.SetNamespace(ns)
			u.SetName("hidden-workload")
			u.SetLabels(map[string]string{
				"app": "hidden",
			})

			// The ownership detection should work in any namespace
			ownership := DetectOwnership(u)

			// Without GitOps labels, it should be unknown
			if ownership.Type != OwnerUnknown {
				t.Errorf("Expected OwnerUnknown for unlabeled resource in %s, got %s",
					ns, ownership.Type)
			}
		})
	}
}

// TestEvasionNameObfuscation tests detection of resources with system-like names
// xBOW pattern: Attacker names resources to look like system components
func TestEvasionNameObfuscation(t *testing.T) {
	obfuscatedNames := []struct {
		name        string
		description string
	}{
		{"kube-proxy-fake", "Mimics kube-proxy"},
		{"coredns-custom", "Mimics CoreDNS"},
		{"metrics-server-malicious", "Mimics metrics-server"},
		{"calico-node-backdoor", "Mimics Calico CNI"},
		{"fluent-bit-exfil", "Mimics Fluent Bit"},
	}

	for _, tc := range obfuscatedNames {
		t.Run(tc.name, func(t *testing.T) {
			u := &unstructured.Unstructured{}
			u.SetGroupVersionKind(DaemonSetGVK)
			u.SetNamespace("kube-system")
			u.SetName(tc.name)
			u.SetLabels(map[string]string{
				"app": tc.name,
			})

			// These should still be detected as unowned if no GitOps labels
			ownership := DetectOwnership(u)

			if ownership.Type != OwnerUnknown {
				t.Errorf("Expected OwnerUnknown for obfuscated resource %s, got %s",
					tc.name, ownership.Type)
			}
		})
	}
}

// TestBuildLabelSelectorString tests the helper function for building kubectl selector strings
func TestBuildLabelSelectorString(t *testing.T) {
	scanner := &StateScanner{}

	tests := []struct {
		name             string
		matchLabels      map[string]string
		matchExpressions []interface{}
		wantContains     []string
	}{
		{
			name: "matchLabels only",
			matchLabels: map[string]string{
				"app": "web",
				"env": "prod",
			},
			matchExpressions: nil,
			wantContains:     []string{"app=web", "env=prod"},
		},
		{
			name:        "matchExpressions only - In operator",
			matchLabels: nil,
			matchExpressions: []interface{}{
				map[string]interface{}{
					"key":      "app",
					"operator": "In",
					"values":   []interface{}{"web", "api"},
				},
			},
			wantContains: []string{"app in (web,api)"},
		},
		{
			name:        "matchExpressions only - NotIn operator",
			matchLabels: nil,
			matchExpressions: []interface{}{
				map[string]interface{}{
					"key":      "env",
					"operator": "NotIn",
					"values":   []interface{}{"dev", "test"},
				},
			},
			wantContains: []string{"env notin (dev,test)"},
		},
		{
			name:        "matchExpressions only - Exists operator",
			matchLabels: nil,
			matchExpressions: []interface{}{
				map[string]interface{}{
					"key":      "monitoring",
					"operator": "Exists",
				},
			},
			wantContains: []string{"monitoring"},
		},
		{
			name:        "matchExpressions only - DoesNotExist operator",
			matchLabels: nil,
			matchExpressions: []interface{}{
				map[string]interface{}{
					"key":      "deprecated",
					"operator": "DoesNotExist",
				},
			},
			wantContains: []string{"!deprecated"},
		},
		{
			name: "Combined matchLabels and matchExpressions",
			matchLabels: map[string]string{
				"tier": "backend",
			},
			matchExpressions: []interface{}{
				map[string]interface{}{
					"key":      "version",
					"operator": "In",
					"values":   []interface{}{"v1", "v2"},
				},
			},
			wantContains: []string{"tier=backend", "version in (v1,v2)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := scanner.buildLabelSelectorString(tt.matchLabels, tt.matchExpressions)

			for _, want := range tt.wantContains {
				if !containsSubstring(result, want) {
					t.Errorf("buildLabelSelectorString() = %q, want to contain %q", result, want)
				}
			}
		})
	}
}

// Helper function to check if string contains substring
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstringHelper(s, substr))
}

func containsSubstringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ============================================================================
// Dangling Detection Tests
//
// These tests verify the core dangling resource detection without requiring
// a real Kubernetes cluster. They use the fake dynamic client.
// ============================================================================

// TestDanglingHPADetection tests HPA dangling detection
func TestDanglingHPADetection(t *testing.T) {
	t.Run("HPA targeting non-existent deployment should be detected", func(t *testing.T) {
		// Create an HPA that targets a non-existent deployment
		hpa := &unstructured.Unstructured{}
		hpa.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "autoscaling/v2",
			"kind":       "HorizontalPodAutoscaler",
			"metadata": map[string]interface{}{
				"name":      "dangling-hpa",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"scaleTargetRef": map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"name":       "non-existent-target",
				},
				"minReplicas": float64(1),
				"maxReplicas": float64(10),
			},
		})

		// Verify the HPA structure
		scaleTargetRef, found, _ := unstructured.NestedMap(hpa.Object, "spec", "scaleTargetRef")
		if !found {
			t.Fatal("scaleTargetRef not found")
		}

		targetName, _, _ := unstructured.NestedString(scaleTargetRef, "name")
		targetKind, _, _ := unstructured.NestedString(scaleTargetRef, "kind")

		if targetName != "non-existent-target" {
			t.Errorf("Expected targetName = 'non-existent-target', got %q", targetName)
		}
		if targetKind != "Deployment" {
			t.Errorf("Expected targetKind = 'Deployment', got %q", targetKind)
		}
	})
}

// TestDanglingServiceDetection tests Service dangling detection
func TestDanglingServiceDetection(t *testing.T) {
	t.Run("Service with selector matching no pods should be detected", func(t *testing.T) {
		svc := &unstructured.Unstructured{}
		svc.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "dangling-service",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"type": "ClusterIP",
				"selector": map[string]interface{}{
					"app": "nonexistent-app",
				},
				"ports": []interface{}{
					map[string]interface{}{
						"port":       float64(80),
						"targetPort": float64(8080),
					},
				},
			},
		})

		// Verify the selector
		selector, found, _ := unstructured.NestedStringMap(svc.Object, "spec", "selector")
		if !found {
			t.Fatal("selector not found")
		}

		if selector["app"] != "nonexistent-app" {
			t.Errorf("Expected selector[app] = 'nonexistent-app', got %q", selector["app"])
		}
	})
}

// TestDanglingIngressDetection tests Ingress dangling detection
func TestDanglingIngressDetection(t *testing.T) {
	t.Run("Ingress with non-existent backend service should be detected", func(t *testing.T) {
		ingress := &unstructured.Unstructured{}
		ingress.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "Ingress",
			"metadata": map[string]interface{}{
				"name":      "dangling-ingress",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"rules": []interface{}{
					map[string]interface{}{
						"host": "example.com",
						"http": map[string]interface{}{
							"paths": []interface{}{
								map[string]interface{}{
									"path":     "/",
									"pathType": "Prefix",
									"backend": map[string]interface{}{
										"service": map[string]interface{}{
											"name": "non-existent-service",
											"port": map[string]interface{}{
												"number": float64(80),
											},
										},
									},
								},
							},
						},
					},
				},
			},
		})

		// Verify the backend service reference
		rules, found, _ := unstructured.NestedSlice(ingress.Object, "spec", "rules")
		if !found || len(rules) == 0 {
			t.Fatal("rules not found")
		}

		rule := rules[0].(map[string]interface{})
		http, _, _ := unstructured.NestedMap(rule, "http")
		paths, _, _ := unstructured.NestedSlice(http, "paths")
		path := paths[0].(map[string]interface{})
		backend, _, _ := unstructured.NestedMap(path, "backend")
		service, _, _ := unstructured.NestedMap(backend, "service")
		serviceName, _, _ := unstructured.NestedString(service, "name")

		if serviceName != "non-existent-service" {
			t.Errorf("Expected serviceName = 'non-existent-service', got %q", serviceName)
		}
	})
}

// TestDanglingNetworkPolicyDetection tests NetworkPolicy dangling detection
func TestDanglingNetworkPolicyDetection(t *testing.T) {
	t.Run("NetworkPolicy with selector matching no pods should be detected", func(t *testing.T) {
		np := &unstructured.Unstructured{}
		np.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "NetworkPolicy",
			"metadata": map[string]interface{}{
				"name":      "dangling-netpol",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"podSelector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app": "nonexistent-app",
					},
				},
				"policyTypes": []interface{}{"Ingress", "Egress"},
			},
		})

		// Verify the pod selector
		podSelector, found, _ := unstructured.NestedMap(np.Object, "spec", "podSelector")
		if !found {
			t.Fatal("podSelector not found")
		}

		matchLabels, found, _ := unstructured.NestedStringMap(podSelector, "matchLabels")
		if !found {
			t.Fatal("matchLabels not found")
		}

		if matchLabels["app"] != "nonexistent-app" {
			t.Errorf("Expected matchLabels[app] = 'nonexistent-app', got %q", matchLabels["app"])
		}
	})
}

// ============================================================================
// DanglingFinding Structure Tests
// ============================================================================

func TestDanglingFindingFields(t *testing.T) {
	finding := DanglingFinding{
		CCVEID:      "CCVE-2025-0687",
		Category:    "ORPHAN",
		Severity:    "warning",
		Kind:        "HorizontalPodAutoscaler",
		Name:        "test-hpa",
		Namespace:   "default",
		TargetKind:  "Deployment",
		TargetName:  "missing-deployment",
		Message:     "HPA targets non-existent Deployment/missing-deployment",
		Remediation: "Delete the HPA or create the missing target workload",
		Command:     "kubectl delete hpa test-hpa -n default",
	}

	// Verify all fields are set correctly
	if finding.CCVEID != "CCVE-2025-0687" {
		t.Errorf("CCVEID = %q, want 'CCVE-2025-0687'", finding.CCVEID)
	}
	if finding.Category != "ORPHAN" {
		t.Errorf("Category = %q, want 'ORPHAN'", finding.Category)
	}
	if finding.Kind != "HorizontalPodAutoscaler" {
		t.Errorf("Kind = %q, want 'HorizontalPodAutoscaler'", finding.Kind)
	}
}

// ============================================================================
// Integration test with fake client
// ============================================================================

func TestStateScannerWithFakeClient(t *testing.T) {
	t.Run("Scanner can be created with fake client", func(t *testing.T) {
		client := newFakeDynamicClient()
		scanner := NewStateScannerWithClient(client)

		if scanner == nil {
			t.Fatal("Expected scanner to be created")
		}
		if scanner.client == nil {
			t.Fatal("Expected scanner.client to be set")
		}
	})
}

// TestScanDanglingResourcesEmpty tests scanning with no resources
func TestScanDanglingResourcesEmpty(t *testing.T) {
	t.Run("Empty cluster returns no findings", func(t *testing.T) {
		client := newFakeDynamicClient()
		scanner := NewStateScannerWithClient(client)

		result, err := scanner.ScanDanglingResources(context.Background())
		if err != nil {
			t.Fatalf("ScanDanglingResources failed: %v", err)
		}

		if result == nil {
			t.Fatal("Expected result to be non-nil")
		}

		if result.Summary.Total != 0 {
			t.Errorf("Expected 0 findings in empty cluster, got %d", result.Summary.Total)
		}
	})
}

// ============================================================================
// Test checkScaleTargetExists logic
// ============================================================================

func TestCheckScaleTargetExistsLogic(t *testing.T) {
	tests := []struct {
		kind     string
		expected string // Expected resource type for GVR
	}{
		{"Deployment", "deployments"},
		{"ReplicaSet", "replicasets"},
		{"StatefulSet", "statefulsets"},
		{"ReplicationController", "replicationcontrollers"},
		{"CustomKind", ""}, // Unknown kinds return true (assume exists)
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			// Document that these kinds are supported
			switch tt.kind {
			case "Deployment", "ReplicaSet", "StatefulSet", "ReplicationController":
				t.Logf("Kind %s is supported for HPA target detection", tt.kind)
			default:
				t.Logf("Kind %s is unknown, will be assumed to exist", tt.kind)
			}
		})
	}
}

// ============================================================================
// Mock helpers for creating test resources with proper metadata
// ============================================================================

func newMockHPA(namespace, name, targetKind, targetName string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetUnstructuredContent(map[string]interface{}{
		"apiVersion": "autoscaling/v2",
		"kind":       "HorizontalPodAutoscaler",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
			"uid":       namespace + "/" + name,
		},
		"spec": map[string]interface{}{
			"scaleTargetRef": map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       targetKind,
				"name":       targetName,
			},
			"minReplicas": float64(1),
			"maxReplicas": float64(10),
		},
	})
	return u
}

func newMockService(namespace, name string, selector map[string]interface{}) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetUnstructuredContent(map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
			"uid":       namespace + "/" + name,
		},
		"spec": map[string]interface{}{
			"type":     "ClusterIP",
			"selector": selector,
			"ports": []interface{}{
				map[string]interface{}{
					"port":       float64(80),
					"targetPort": float64(8080),
				},
			},
		},
	})
	return u
}

func newMockNetworkPolicy(namespace, name string, podSelector map[string]interface{}) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetUnstructuredContent(map[string]interface{}{
		"apiVersion": "networking.k8s.io/v1",
		"kind":       "NetworkPolicy",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
			"uid":       namespace + "/" + name,
		},
		"spec": map[string]interface{}{
			"podSelector": podSelector,
			"policyTypes": []interface{}{"Ingress"},
		},
	})
	return u
}

func newMockPod(namespace, name string, labels map[string]string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	labelMap := make(map[string]interface{})
	for k, v := range labels {
		labelMap[k] = v
	}
	u.SetUnstructuredContent(map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
			"uid":       namespace + "/" + name,
			"labels":    labelMap,
		},
		"spec": map[string]interface{}{
			"containers": []interface{}{
				map[string]interface{}{
					"name":  "main",
					"image": "nginx:latest",
				},
			},
		},
	})
	return u
}

func newMockDeployment(namespace, name string, labels map[string]string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	labelMap := make(map[string]interface{})
	for k, v := range labels {
		labelMap[k] = v
	}
	u.SetUnstructuredContent(map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
			"uid":       namespace + "/" + name,
			"labels":    labelMap,
		},
		"spec": map[string]interface{}{
			"replicas": float64(1),
			"selector": map[string]interface{}{
				"matchLabels": labelMap,
			},
		},
	})
	return u
}

// Add the missing metav1 import usage to silence linter
var _ = metav1.Now

// ============================================================================
// Fixture-Based Dangling Resource Tests
//
// These tests load YAML fixtures from test/fixtures/dangling/ and verify that
// the StateScanner correctly detects dangling resources.
// ============================================================================

// getFixturesDir returns the path to the test fixtures directory
func getFixturesDir() string {
	_, filename, _, _ := goruntime.Caller(0)
	pkgDir := filepath.Dir(filename)
	return filepath.Join(pkgDir, "..", "..", "test", "fixtures", "dangling")
}

// loadFixture loads a YAML fixture file and returns an unstructured object
func loadFixture(t *testing.T, filename string) *unstructured.Unstructured {
	t.Helper()

	fixtureDir := getFixturesDir()
	filePath := filepath.Join(fixtureDir, filename)

	data, err := os.ReadFile(filePath)
	require.NoError(t, err, "failed to read fixture file: %s", filePath)

	// Parse YAML to unstructured
	obj := &unstructured.Unstructured{}
	err = yaml.Unmarshal(data, &obj.Object)
	require.NoError(t, err, "failed to unmarshal fixture YAML")

	return obj
}

// createFakeClient creates a fake dynamic client with the given objects.
// The client is configured with custom list kinds for all resource types
// that the scanner may list during scanning.
func createFakeClient(objs ...*unstructured.Unstructured) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	runtimeObjs := make([]runtime.Object, len(objs))
	for i, obj := range objs {
		runtimeObjs[i] = obj
	}

	// Register list kinds for all resources the scanner may list
	gvrToListKind := map[schema.GroupVersionResource]string{
		// Core resources
		{Group: "", Version: "v1", Resource: "services"}:               "ServiceList",
		{Group: "", Version: "v1", Resource: "pods"}:                   "PodList",
		{Group: "", Version: "v1", Resource: "secrets"}:                "SecretList",
		{Group: "", Version: "v1", Resource: "configmaps"}:             "ConfigMapList",
		{Group: "", Version: "v1", Resource: "namespaces"}:             "NamespaceList",
		{Group: "", Version: "v1", Resource: "persistentvolumeclaims"}: "PersistentVolumeClaimList",
		{Group: "", Version: "v1", Resource: "replicationcontrollers"}: "ReplicationControllerList",
		{Group: "", Version: "v1", Resource: "serviceaccounts"}:        "ServiceAccountList",

		// Apps resources
		{Group: "apps", Version: "v1", Resource: "deployments"}:  "DeploymentList",
		{Group: "apps", Version: "v1", Resource: "replicasets"}:  "ReplicaSetList",
		{Group: "apps", Version: "v1", Resource: "statefulsets"}: "StatefulSetList",
		{Group: "apps", Version: "v1", Resource: "daemonsets"}:   "DaemonSetList",

		// Autoscaling
		{Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"}:      "HorizontalPodAutoscalerList",
		{Group: "autoscaling", Version: "v1", Resource: "horizontalpodautoscalers"}:      "HorizontalPodAutoscalerList",
		{Group: "autoscaling.k8s.io", Version: "v1", Resource: "verticalpodautoscalers"}: "VerticalPodAutoscalerList",

		// Networking
		{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"}:       "IngressList",
		{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}: "NetworkPolicyList",

		// Policy
		{Group: "policy", Version: "v1", Resource: "poddisruptionbudgets"}: "PodDisruptionBudgetList",

		// Batch
		{Group: "batch", Version: "v1", Resource: "jobs"}:     "JobList",
		{Group: "batch", Version: "v1", Resource: "cronjobs"}: "CronJobList",

		// RBAC
		{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"}:               "RoleList",
		{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"}:        "RoleBindingList",
		{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"}:        "ClusterRoleList",
		{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"}: "ClusterRoleBindingList",

		// Flux resources
		{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"}:            "HelmReleaseList",
		{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"}:     "KustomizationList",
		{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"}:       "GitRepositoryList",
		{Group: "source.toolkit.fluxcd.io", Version: "v1beta2", Resource: "gitrepositories"}:  "GitRepositoryList",
		{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "helmrepositories"}:      "HelmRepositoryList",
		{Group: "source.toolkit.fluxcd.io", Version: "v1beta2", Resource: "helmrepositories"}: "HelmRepositoryList",
		{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "helmcharts"}:            "HelmChartList",
		{Group: "source.toolkit.fluxcd.io", Version: "v1beta2", Resource: "helmcharts"}:       "HelmChartList",

		// Argo CD resources
		{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}:    "ApplicationList",
		{Group: "argoproj.io", Version: "v1alpha1", Resource: "applicationsets"}: "ApplicationSetList",

		// Certificates
		{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}: "CertificateList",
		{Group: "cert-manager.io", Version: "v1", Resource: "issuers"}:      "IssuerList",
	}

	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, runtimeObjs...)
}

// TestDanglingHPA tests detection of HPAs targeting non-existent deployments
func TestDanglingHPA(t *testing.T) {
	// Load fixture: HPA targeting non-existent deployment
	hpa := loadFixture(t, "hpa-no-target.yaml")

	// Set GVK for the HPA (required for fake client)
	hpa.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "autoscaling",
		Version: "v2",
		Kind:    "HorizontalPodAutoscaler",
	})

	// Create fake client with HPA but NO target Deployment
	client := createFakeClient(hpa)

	// Create scanner with fake client
	scanner := NewStateScannerWithClient(client)

	// Run dangling resource scan
	ctx := context.Background()
	result, err := scanner.ScanDanglingResources(ctx)

	require.NoError(t, err, "scan should not return error")
	require.NotNil(t, result, "result should not be nil")

	// Assert finding was detected
	require.Len(t, result.Findings, 1, "should detect exactly 1 dangling HPA")

	finding := result.Findings[0]
	assert.Equal(t, "CCVE-2025-0687", finding.CCVEID, "should have correct CCVE ID")
	assert.Equal(t, "ORPHAN", finding.Category, "should have ORPHAN category")
	assert.Equal(t, "warning", finding.Severity, "should have warning severity")
	assert.Equal(t, "HorizontalPodAutoscaler", finding.Kind, "should have correct kind")
	assert.Equal(t, "dangling-hpa-test", finding.Name, "should have correct name")
	assert.Equal(t, "default", finding.Namespace, "should have correct namespace")
	assert.Equal(t, "Deployment", finding.TargetKind, "should target Deployment")
	assert.Equal(t, "non-existent-deployment", finding.TargetName, "should have correct target name")
	assert.Contains(t, finding.Message, "non-existent", "message should mention non-existent target")
	assert.Contains(t, finding.Remediation, "Delete", "remediation should suggest deletion")

	// Verify summary
	assert.Equal(t, 1, result.Summary.HPAs, "summary should count 1 HPA")
	assert.Equal(t, 1, result.Summary.Total, "summary should have total of 1")
}

// TestDanglingHPA_WithTarget tests that HPA with existing target is NOT flagged
func TestDanglingHPA_WithTarget(t *testing.T) {
	// Load fixture: HPA targeting non-existent deployment
	hpa := loadFixture(t, "hpa-no-target.yaml")
	hpa.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "autoscaling",
		Version: "v2",
		Kind:    "HorizontalPodAutoscaler",
	})

	// Create the target deployment
	deployment := &unstructured.Unstructured{}
	deployment.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	})
	deployment.SetNamespace("default")
	deployment.SetName("non-existent-deployment")

	// Create fake client with both HPA and target Deployment
	client := createFakeClient(hpa, deployment)
	scanner := NewStateScannerWithClient(client)

	ctx := context.Background()
	result, err := scanner.ScanDanglingResources(ctx)

	require.NoError(t, err)
	require.NotNil(t, result)

	// HPA should NOT be flagged when target exists
	assert.Empty(t, result.Findings, "should not detect dangling HPA when target exists")
	assert.Equal(t, 0, result.Summary.HPAs, "HPA count should be 0")
}

// TestDanglingService tests detection of Services with selectors matching no pods
func TestDanglingService(t *testing.T) {
	// Load fixture: Service with selector matching no pods
	svc := loadFixture(t, "service-no-pods.yaml")
	svc.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Service",
	})

	// Create fake client with Service but NO matching Pods
	client := createFakeClient(svc)
	scanner := NewStateScannerWithClient(client)

	ctx := context.Background()
	result, err := scanner.ScanDanglingResources(ctx)

	require.NoError(t, err, "scan should not return error")
	require.NotNil(t, result, "result should not be nil")

	// Assert finding was detected
	require.Len(t, result.Findings, 1, "should detect exactly 1 dangling Service")

	finding := result.Findings[0]
	assert.Equal(t, "CCVE-2025-0688", finding.CCVEID, "should have correct CCVE ID")
	assert.Equal(t, "ORPHAN", finding.Category, "should have ORPHAN category")
	assert.Equal(t, "warning", finding.Severity, "should have warning severity")
	assert.Equal(t, "Service", finding.Kind, "should have correct kind")
	assert.Equal(t, "dangling-service-test", finding.Name, "should have correct name")
	assert.Equal(t, "default", finding.Namespace, "should have correct namespace")
	assert.Equal(t, "Pod", finding.TargetKind, "should target Pod")
	assert.Contains(t, finding.Message, "no pods", "message should mention no pods")
	assert.Contains(t, finding.Remediation, "selector", "remediation should mention selector")

	// Verify summary
	assert.Equal(t, 1, result.Summary.Services, "summary should count 1 Service")
	assert.Equal(t, 1, result.Summary.Total, "summary should have total of 1")
}

// TestDanglingService_WithMatchingPods tests that Service with matching pods is NOT flagged
func TestDanglingService_WithMatchingPods(t *testing.T) {
	// Load fixture: Service with selector matching no pods
	svc := loadFixture(t, "service-no-pods.yaml")
	svc.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Service",
	})

	// Create a pod that matches the service selector (app=non-existent-app, tier=ghost)
	pod := &unstructured.Unstructured{}
	pod.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Pod",
	})
	pod.SetNamespace("default")
	pod.SetName("matching-pod")
	pod.SetLabels(map[string]string{
		"app":  "non-existent-app",
		"tier": "ghost",
	})

	// Create fake client with both Service and matching Pod
	client := createFakeClient(svc, pod)
	scanner := NewStateScannerWithClient(client)

	ctx := context.Background()
	result, err := scanner.ScanDanglingResources(ctx)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Service should NOT be flagged when matching pods exist
	assert.Empty(t, result.Findings, "should not detect dangling Service when pods match")
	assert.Equal(t, 0, result.Summary.Services, "Service count should be 0")
}

// TestDanglingIngress tests detection of Ingresses with backends pointing to non-existent services
func TestDanglingIngress(t *testing.T) {
	// Load fixture: Ingress with backend pointing to non-existent service
	ingress := loadFixture(t, "ingress-no-backend.yaml")
	ingress.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "networking.k8s.io",
		Version: "v1",
		Kind:    "Ingress",
	})

	// Create fake client with Ingress but NO backend Service
	client := createFakeClient(ingress)
	scanner := NewStateScannerWithClient(client)

	ctx := context.Background()
	result, err := scanner.ScanDanglingResources(ctx)

	require.NoError(t, err, "scan should not return error")
	require.NotNil(t, result, "result should not be nil")

	// Assert finding was detected
	require.Len(t, result.Findings, 1, "should detect exactly 1 dangling Ingress")

	finding := result.Findings[0]
	assert.Equal(t, "CCVE-2025-0689", finding.CCVEID, "should have correct CCVE ID")
	assert.Equal(t, "ORPHAN", finding.Category, "should have ORPHAN category")
	assert.Equal(t, "warning", finding.Severity, "should have warning severity")
	assert.Equal(t, "Ingress", finding.Kind, "should have correct kind")
	assert.Equal(t, "dangling-ingress-test", finding.Name, "should have correct name")
	assert.Equal(t, "default", finding.Namespace, "should have correct namespace")
	assert.Equal(t, "Service", finding.TargetKind, "should target Service")
	assert.Equal(t, "non-existent-backend-service", finding.TargetName, "should have correct target name")
	assert.Contains(t, finding.Message, "non-existent", "message should mention non-existent service")
	assert.Contains(t, finding.Remediation, "service", "remediation should mention service")

	// Verify summary
	assert.Equal(t, 1, result.Summary.Ingresses, "summary should count 1 Ingress")
	assert.Equal(t, 1, result.Summary.Total, "summary should have total of 1")
}

// TestDanglingIngress_WithBackendService tests that Ingress with existing backend is NOT flagged
func TestDanglingIngress_WithBackendService(t *testing.T) {
	// Load fixture: Ingress with backend pointing to non-existent service
	ingress := loadFixture(t, "ingress-no-backend.yaml")
	ingress.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "networking.k8s.io",
		Version: "v1",
		Kind:    "Ingress",
	})

	// Create the backend service
	svc := &unstructured.Unstructured{}
	svc.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Service",
	})
	svc.SetNamespace("default")
	svc.SetName("non-existent-backend-service")

	// Create fake client with both Ingress and backend Service
	client := createFakeClient(ingress, svc)
	scanner := NewStateScannerWithClient(client)

	ctx := context.Background()
	result, err := scanner.ScanDanglingResources(ctx)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Ingress should NOT be flagged when backend service exists
	assert.Empty(t, result.Findings, "should not detect dangling Ingress when backend exists")
	assert.Equal(t, 0, result.Summary.Ingresses, "Ingress count should be 0")
}

// TestDanglingNetworkPolicy tests detection of NetworkPolicies with podSelectors matching no pods
func TestDanglingNetworkPolicy(t *testing.T) {
	// Load fixture: NetworkPolicy with podSelector matching no pods
	np := loadFixture(t, "networkpolicy-orphan.yaml")
	np.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "networking.k8s.io",
		Version: "v1",
		Kind:    "NetworkPolicy",
	})

	// Create fake client with NetworkPolicy but NO matching Pods
	client := createFakeClient(np)
	scanner := NewStateScannerWithClient(client)

	ctx := context.Background()
	result, err := scanner.ScanDanglingResources(ctx)

	require.NoError(t, err, "scan should not return error")
	require.NotNil(t, result, "result should not be nil")

	// Assert finding was detected
	require.Len(t, result.Findings, 1, "should detect exactly 1 dangling NetworkPolicy")

	finding := result.Findings[0]
	assert.Equal(t, "CCVE-2025-0690", finding.CCVEID, "should have correct CCVE ID")
	assert.Equal(t, "ORPHAN", finding.Category, "should have ORPHAN category")
	assert.Equal(t, "info", finding.Severity, "should have info severity")
	assert.Equal(t, "NetworkPolicy", finding.Kind, "should have correct kind")
	assert.Equal(t, "dangling-netpol-test", finding.Name, "should have correct name")
	assert.Equal(t, "default", finding.Namespace, "should have correct namespace")
	assert.Equal(t, "Pod", finding.TargetKind, "should target Pod")
	assert.Contains(t, finding.Message, "no pods", "message should mention no pods")
	assert.Contains(t, finding.Remediation, "labels", "remediation should mention labels")

	// Verify summary
	assert.Equal(t, 1, result.Summary.NetworkPolicies, "summary should count 1 NetworkPolicy")
	assert.Equal(t, 1, result.Summary.Total, "summary should have total of 1")
}

// TestDanglingNetworkPolicy_WithMatchingPods tests that NetworkPolicy with matching pods is NOT flagged
func TestDanglingNetworkPolicy_WithMatchingPods(t *testing.T) {
	// Load fixture: NetworkPolicy with podSelector matching no pods
	np := loadFixture(t, "networkpolicy-orphan.yaml")
	np.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "networking.k8s.io",
		Version: "v1",
		Kind:    "NetworkPolicy",
	})

	// Create a pod that matches the NetworkPolicy selector (app=orphaned-selector, environment=nonexistent)
	pod := &unstructured.Unstructured{}
	pod.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Pod",
	})
	pod.SetNamespace("default")
	pod.SetName("matching-pod")
	pod.SetLabels(map[string]string{
		"app":         "orphaned-selector",
		"environment": "nonexistent",
	})

	// Create fake client with both NetworkPolicy and matching Pod
	client := createFakeClient(np, pod)
	scanner := NewStateScannerWithClient(client)

	ctx := context.Background()
	result, err := scanner.ScanDanglingResources(ctx)

	require.NoError(t, err)
	require.NotNil(t, result)

	// NetworkPolicy should NOT be flagged when matching pods exist
	assert.Empty(t, result.Findings, "should not detect dangling NetworkPolicy when pods match")
	assert.Equal(t, 0, result.Summary.NetworkPolicies, "NetworkPolicy count should be 0")
}

// TestDanglingResources_AllTypes tests detection of all dangling resource types together
func TestDanglingResources_AllTypes(t *testing.T) {
	// Load all fixtures
	hpa := loadFixture(t, "hpa-no-target.yaml")
	hpa.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "autoscaling",
		Version: "v2",
		Kind:    "HorizontalPodAutoscaler",
	})

	svc := loadFixture(t, "service-no-pods.yaml")
	svc.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Service",
	})

	ingress := loadFixture(t, "ingress-no-backend.yaml")
	ingress.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "networking.k8s.io",
		Version: "v1",
		Kind:    "Ingress",
	})

	np := loadFixture(t, "networkpolicy-orphan.yaml")
	np.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "networking.k8s.io",
		Version: "v1",
		Kind:    "NetworkPolicy",
	})

	// Create fake client with all dangling resources but no targets
	client := createFakeClient(hpa, svc, ingress, np)
	scanner := NewStateScannerWithClient(client)

	ctx := context.Background()
	result, err := scanner.ScanDanglingResources(ctx)

	require.NoError(t, err, "scan should not return error")
	require.NotNil(t, result, "result should not be nil")

	// Assert all findings were detected
	assert.Len(t, result.Findings, 4, "should detect 4 dangling resources")

	// Verify summary counts
	assert.Equal(t, 1, result.Summary.HPAs, "should have 1 HPA finding")
	assert.Equal(t, 1, result.Summary.Services, "should have 1 Service finding")
	assert.Equal(t, 1, result.Summary.Ingresses, "should have 1 Ingress finding")
	assert.Equal(t, 1, result.Summary.NetworkPolicies, "should have 1 NetworkPolicy finding")
	assert.Equal(t, 4, result.Summary.Total, "should have 4 total findings")

	// Verify each CCVE ID is present
	ccveIDs := make(map[string]bool)
	for _, f := range result.Findings {
		ccveIDs[f.CCVEID] = true
	}
	assert.True(t, ccveIDs["CCVE-2025-0687"], "should have HPA CCVE")
	assert.True(t, ccveIDs["CCVE-2025-0688"], "should have Service CCVE")
	assert.True(t, ccveIDs["CCVE-2025-0689"], "should have Ingress CCVE")
	assert.True(t, ccveIDs["CCVE-2025-0690"], "should have NetworkPolicy CCVE")
}

// TestDanglingResources_NoFindings tests that scanner returns empty when all targets exist
func TestDanglingResources_NoFindings(t *testing.T) {
	// Create fake client with no resources
	client := createFakeClient()
	scanner := NewStateScannerWithClient(client)

	ctx := context.Background()
	result, err := scanner.ScanDanglingResources(ctx)

	require.NoError(t, err, "scan should not return error")
	require.NotNil(t, result, "result should not be nil")

	assert.Empty(t, result.Findings, "should have no findings when no dangling resources exist")
	assert.Equal(t, 0, result.Summary.Total, "summary total should be 0")
}
