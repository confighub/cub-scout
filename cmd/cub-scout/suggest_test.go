// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import "testing"

func TestExtractVariantFromPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		// Standard Flux patterns
		{"simple staging", "./staging", "staging"},
		{"simple production", "./production", "prod"},
		{"simple prod", "./prod", "prod"},
		{"simple dev", "./dev", "dev"},
		{"simple development", "./development", "dev"},

		// Nested paths
		{"nested staging", "./apps/staging/podinfo", "staging"},
		{"nested production", "./clusters/production/apps", "prod"},
		{"nested prod", "./clusters/prod/apps", "prod"},
		{"deeply nested", "./infrastructure/staging/overlays", "staging"},

		// Without leading ./
		{"no dot slash", "staging", "staging"},
		{"nested no dot slash", "apps/staging/podinfo", "staging"},

		// Edge cases
		{"empty path", "", ""},
		{"just dot", ".", ""},
		{"just slash", "/", ""},
		{"no variant in path", "./apps/base/podinfo", ""},
		{"root path", "./", ""},

		// Mixed case (should normalize)
		{"uppercase staging", "./STAGING", "staging"},
		{"mixed case production", "./Production", "prod"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractVariantFromPath(tt.path)
			if result != tt.expected {
				t.Errorf("extractVariantFromPath(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestParseNamespacePattern(t *testing.T) {
	tests := []struct {
		name        string
		namespace   string
		expectedApp string
		expectedVar string
	}{
		// Suffix patterns
		{"app-prod suffix", "myapp-prod", "myapp", "prod"},
		{"app-staging suffix", "myapp-staging", "myapp", "staging"},
		{"app-dev suffix", "myapp-dev", "myapp", "dev"},
		{"app-production suffix", "myapp-production", "myapp", "prod"},

		// Prefix patterns
		{"prod-app prefix", "prod-myapp", "myapp", "prod"},
		{"staging-app prefix", "staging-myapp", "myapp", "staging"},

		// No pattern
		{"no pattern", "myapp", "myapp", ""},
		{"system namespace", "kube-system", "kube-system", ""},

		// Complex names
		{"multi-segment", "payment-api-prod", "payment-api", "prod"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, variant := parseNamespacePattern(tt.namespace)
			if app != tt.expectedApp {
				t.Errorf("parseNamespacePattern(%q) app = %q, want %q", tt.namespace, app, tt.expectedApp)
			}
			if variant != tt.expectedVar {
				t.Errorf("parseNamespacePattern(%q) variant = %q, want %q", tt.namespace, variant, tt.expectedVar)
			}
		})
	}
}

func TestInferAppAndVariant(t *testing.T) {
	tests := []struct {
		name        string
		workload    WorkloadInfo
		expectedApp string
		expectedVar string
	}{
		{
			name: "kustomization path takes priority",
			workload: WorkloadInfo{
				Name:              "myapp",
				Namespace:         "default",
				KustomizationPath: "./staging",
				Labels:            map[string]string{"app": "myapp"},
			},
			expectedApp: "myapp",
			expectedVar: "staging",
		},
		{
			name: "application path takes priority (Argo)",
			workload: WorkloadInfo{
				Name:            "myapp",
				Namespace:       "default",
				ApplicationPath: "apps/production/myapp",
				Labels:          map[string]string{"app": "myapp"},
			},
			expectedApp: "myapp",
			expectedVar: "prod",
		},
		{
			name: "kustomization path preferred over application path",
			workload: WorkloadInfo{
				Name:              "myapp",
				Namespace:         "default",
				KustomizationPath: "./staging",
				ApplicationPath:   "apps/prod", // Should be ignored
				Labels:            map[string]string{"app": "myapp"},
			},
			expectedApp: "myapp",
			expectedVar: "staging",
		},
		{
			name: "argo overlays path pattern",
			workload: WorkloadInfo{
				Name:            "cart",
				Namespace:       "checkout",
				ApplicationPath: "tenants/checkout/cart/overlays/dev",
				Labels:          map[string]string{"app": "cart"},
			},
			expectedApp: "cart",
			expectedVar: "dev",
		},
		{
			name: "k8s labels when no gitops path",
			workload: WorkloadInfo{
				Name:      "myapp",
				Namespace: "default",
				Labels: map[string]string{
					"app.kubernetes.io/name":     "myapp",
					"app.kubernetes.io/instance": "myapp-prod",
				},
			},
			expectedApp: "myapp",
			expectedVar: "prod",
		},
		{
			name: "namespace pattern fallback",
			workload: WorkloadInfo{
				Name:      "some-deployment",
				Namespace: "myapp-staging",
				Labels:    map[string]string{},
			},
			expectedApp: "myapp",
			expectedVar: "staging",
		},
		{
			name: "kustomization path overrides namespace pattern",
			workload: WorkloadInfo{
				Name:              "some-deployment",
				Namespace:         "myapp-staging",
				KustomizationPath: "./production",
				Labels:            map[string]string{"app": "myapp"},
			},
			expectedApp: "myapp",
			expectedVar: "prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, variant := inferAppAndVariant(tt.workload)
			if app != tt.expectedApp {
				t.Errorf("inferAppAndVariant() app = %q, want %q", app, tt.expectedApp)
			}
			if variant != tt.expectedVar {
				t.Errorf("inferAppAndVariant() variant = %q, want %q", variant, tt.expectedVar)
			}
		})
	}
}
