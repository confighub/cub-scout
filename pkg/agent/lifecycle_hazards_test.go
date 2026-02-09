// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestLifecycleHazardScanner_HelmHookAmbiguity(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		wantRule    string
		wantHooks   []string
	}{
		{
			name: "single hook - no finding",
			annotations: map[string]string{
				"helm.sh/hook": "post-install",
			},
			wantRule: "",
		},
		{
			name: "comma-separated hooks - finding",
			annotations: map[string]string{
				"helm.sh/hook": "post-install,post-upgrade",
			},
			wantRule:  RuleHelmHookAmbiguity,
			wantHooks: []string{"post-install", "post-upgrade"},
		},
		{
			name: "mixed pre and post hooks",
			annotations: map[string]string{
				"helm.sh/hook": "pre-install,post-upgrade",
			},
			wantRule:  RuleHelmHookAmbiguity,
			wantHooks: []string{"post-upgrade", "pre-install"},
		},
		{
			name:        "no hook annotation",
			annotations: map[string]string{},
			wantRule:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{}
			obj.SetKind("Job")
			obj.SetName("test-job")
			obj.SetNamespace("default")
			obj.SetAnnotations(tt.annotations)

			scanner := NewLifecycleHazardScannerFromObjects(nil)
			result := scanner.ScanObjects([]*unstructured.Unstructured{obj})

			if tt.wantRule == "" {
				if len(result.Findings) > 0 {
					t.Errorf("Expected no findings, got %d", len(result.Findings))
				}
				return
			}

			if len(result.Findings) == 0 {
				t.Fatal("Expected finding, got none")
			}

			finding := result.Findings[0]
			if finding.Rule != tt.wantRule {
				t.Errorf("Rule = %q, want %q", finding.Rule, tt.wantRule)
			}

			if len(tt.wantHooks) > 0 {
				if len(finding.Hooks) != len(tt.wantHooks) {
					t.Errorf("Hooks = %v, want %v", finding.Hooks, tt.wantHooks)
				}
				for i, h := range tt.wantHooks {
					if i < len(finding.Hooks) && finding.Hooks[i] != h {
						t.Errorf("Hooks[%d] = %q, want %q", i, finding.Hooks[i], h)
					}
				}
			}
		})
	}
}

func TestLifecycleHazardScanner_PostSyncIdempotency(t *testing.T) {
	tests := []struct {
		name        string
		kind        string
		annotations map[string]string
		wantFinding bool
	}{
		{
			name: "Job with before-hook-creation - finding",
			kind: "Job",
			annotations: map[string]string{
				"helm.sh/hook":               "post-install",
				"helm.sh/hook-delete-policy": "before-hook-creation",
			},
			wantFinding: true,
		},
		{
			name: "Job with hook-succeeded - no finding",
			kind: "Job",
			annotations: map[string]string{
				"helm.sh/hook":               "post-install",
				"helm.sh/hook-delete-policy": "hook-succeeded",
			},
			wantFinding: false,
		},
		{
			name: "Job with pre-install hook - no finding",
			kind: "Job",
			annotations: map[string]string{
				"helm.sh/hook":               "pre-install",
				"helm.sh/hook-delete-policy": "before-hook-creation",
			},
			wantFinding: false,
		},
		{
			name: "Deployment (not Job) - no finding",
			kind: "Deployment",
			annotations: map[string]string{
				"helm.sh/hook":               "post-install",
				"helm.sh/hook-delete-policy": "before-hook-creation",
			},
			wantFinding: false,
		},
		{
			name: "Job without delete policy - no finding",
			kind: "Job",
			annotations: map[string]string{
				"helm.sh/hook": "post-install",
			},
			wantFinding: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{}
			obj.SetKind(tt.kind)
			obj.SetName("test-resource")
			obj.SetNamespace("default")
			obj.SetAnnotations(tt.annotations)

			scanner := NewLifecycleHazardScannerFromObjects(nil)
			result := scanner.ScanObjects([]*unstructured.Unstructured{obj})

			hasIdempotencyFinding := false
			for _, f := range result.Findings {
				if f.Rule == RulePostSyncIdempotency {
					hasIdempotencyFinding = true
					break
				}
			}

			if hasIdempotencyFinding != tt.wantFinding {
				t.Errorf("PostSync idempotency finding = %v, want %v", hasIdempotencyFinding, tt.wantFinding)
			}
		})
	}
}

func TestLifecycleHazardScanner_Summary(t *testing.T) {
	objects := []*unstructured.Unstructured{
		makeHookObject("Job", "job1", "default", map[string]string{
			"helm.sh/hook": "post-install,post-upgrade",
		}),
		makeHookObject("Job", "job2", "default", map[string]string{
			"helm.sh/hook":               "post-install",
			"helm.sh/hook-delete-policy": "before-hook-creation",
		}),
		makeHookObject("Job", "job3", "prod", map[string]string{
			"helm.sh/hook": "pre-install,post-upgrade",
		}),
	}

	scanner := NewLifecycleHazardScannerFromObjects(nil)
	result := scanner.ScanObjects(objects)

	// job1: ambiguity
	// job2: idempotency
	// job3: ambiguity
	if result.Summary.HelmHookAmbiguity != 2 {
		t.Errorf("HelmHookAmbiguity = %d, want 2", result.Summary.HelmHookAmbiguity)
	}
	if result.Summary.PostSyncIdempotency != 1 {
		t.Errorf("PostSyncIdempotency = %d, want 1", result.Summary.PostSyncIdempotency)
	}
	if result.Summary.Total != 3 {
		t.Errorf("Total = %d, want 3", result.Summary.Total)
	}
}

func TestLifecycleHazardScanner_Deterministic(t *testing.T) {
	objects := []*unstructured.Unstructured{
		makeHookObject("Job", "z-job", "default", map[string]string{
			"helm.sh/hook": "post-install,post-upgrade",
		}),
		makeHookObject("Job", "a-job", "default", map[string]string{
			"helm.sh/hook": "pre-install,post-install",
		}),
		makeHookObject("Job", "m-job", "alpha", map[string]string{
			"helm.sh/hook": "post-upgrade,pre-upgrade",
		}),
	}

	scanner := NewLifecycleHazardScannerFromObjects(nil)

	// Run multiple times
	var outputs []string
	for i := 0; i < 3; i++ {
		result := scanner.ScanObjects(objects)
		output := RenderLifecycleHazardsASCII(result)
		outputs = append(outputs, output)
	}

	// All outputs should be identical
	for i := 1; i < len(outputs); i++ {
		if outputs[i] != outputs[0] {
			t.Errorf("Output %d differs from output 0", i)
		}
	}

	// Verify ordering: sorted by rule, then namespace, then resource
	result := scanner.ScanObjects(objects)
	if len(result.Findings) < 2 {
		t.Fatal("Expected at least 2 findings")
	}

	// All findings are helm-hook-ambiguity, so check namespace order
	for i := 1; i < len(result.Findings); i++ {
		prev := result.Findings[i-1]
		curr := result.Findings[i]
		if prev.Rule > curr.Rule {
			t.Errorf("Findings not sorted by rule: %s > %s", prev.Rule, curr.Rule)
		}
		if prev.Rule == curr.Rule && prev.Namespace > curr.Namespace {
			t.Errorf("Findings not sorted by namespace: %s > %s", prev.Namespace, curr.Namespace)
		}
	}
}

func TestLifecycleHazardScanner_NoFindings(t *testing.T) {
	objects := []*unstructured.Unstructured{
		makeHookObject("Job", "job1", "default", map[string]string{
			"helm.sh/hook": "post-install",
		}),
		makeHookObject("Deployment", "deploy1", "default", map[string]string{
			"some-annotation": "value",
		}),
	}

	scanner := NewLifecycleHazardScannerFromObjects(nil)
	result := scanner.ScanObjects(objects)

	if result.Summary.Total != 0 {
		t.Errorf("Total = %d, want 0", result.Summary.Total)
	}

	output := RenderLifecycleHazardsASCII(result)
	if !strings.Contains(output, "No lifecycle hazards detected") {
		t.Error("Output should indicate no hazards")
	}
}

func TestRenderLifecycleHazardsASCII(t *testing.T) {
	result := &LifecycleHazardResult{
		Findings: []LifecycleHazardFinding{
			{
				Rule:        RuleHelmHookAmbiguity,
				Resource:    "Job/db-migrate",
				Namespace:   "prod",
				Severity:    "warning",
				Hooks:       []string{"post-install", "post-upgrade"},
				MappedPhase: "PostSync",
				Risk:        "ArgoCD maps comma-separated Helm hooks to a single phase",
				Remediation: "Split into separate resources",
			},
		},
		Summary: LifecycleHazardSummary{
			HelmHookAmbiguity: 1,
			Total:             1,
		},
	}

	output := RenderLifecycleHazardsASCII(result)

	expectedStrings := []string{
		"GitOps Lifecycle Hazards",
		"Helm Hook Ambiguity (1)",
		"Job/db-migrate",
		"ns: prod",
		"Hooks: post-install, post-upgrade",
		"Phase: PostSync",
		"Risk:",
		"Fix:",
	}

	for _, s := range expectedStrings {
		if !strings.Contains(output, s) {
			t.Errorf("Output missing %q", s)
		}
	}
}

func TestParseHelmHooks(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"post-install", []string{"post-install"}},
		{"post-install,post-upgrade", []string{"post-install", "post-upgrade"}},
		{"post-install, post-upgrade", []string{"post-install", "post-upgrade"}},
		{" pre-install , post-install ", []string{"post-install", "pre-install"}},
		{"", nil},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseHelmHooks(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("parseHelmHooks(%q) = %v, want %v", tt.input, got, tt.want)
				return
			}
			for i, v := range tt.want {
				if got[i] != v {
					t.Errorf("parseHelmHooks(%q)[%d] = %q, want %q", tt.input, i, got[i], v)
				}
			}
		})
	}
}

func TestGetMappedPhase(t *testing.T) {
	tests := []struct {
		hooks []string
		want  string
	}{
		{[]string{"post-install"}, "PostSync"},
		{[]string{"pre-install"}, "PreSync"},
		{[]string{"post-install", "post-upgrade"}, "PostSync"},
		{[]string{"pre-install", "pre-upgrade"}, "PreSync"},
		{[]string{"pre-install", "post-upgrade"}, "Mixed (PreSync+PostSync)"},
		{[]string{"test"}, "Unknown"},
	}

	for _, tt := range tests {
		t.Run(strings.Join(tt.hooks, ","), func(t *testing.T) {
			got := getMappedPhase(tt.hooks)
			if got != tt.want {
				t.Errorf("getMappedPhase(%v) = %q, want %q", tt.hooks, got, tt.want)
			}
		})
	}
}

func makeHookObject(kind, name, namespace string, annotations map[string]string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetKind(kind)
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.SetAnnotations(annotations)
	return obj
}

func newLifecycleFakeClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{
		{Group: "batch", Version: "v1", Resource: "jobs"}:        "JobList",
		{Group: "batch", Version: "v1", Resource: "cronjobs"}:    "CronJobList",
		{Group: "", Version: "v1", Resource: "pods"}:             "PodList",
		{Group: "", Version: "v1", Resource: "configmaps"}:       "ConfigMapList",
		{Group: "", Version: "v1", Resource: "secrets"}:          "SecretList",
		{Group: "", Version: "v1", Resource: "serviceaccounts"}:  "ServiceAccountList",
		{Group: "", Version: "v1", Resource: "services"}:         "ServiceList",
		{Group: "apps", Version: "v1", Resource: "deployments"}:  "DeploymentList",
		{Group: "apps", Version: "v1", Resource: "statefulsets"}: "StatefulSetList",
		{Group: "apps", Version: "v1", Resource: "daemonsets"}:   "DaemonSetList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objects...)
}

func TestLifecycleHazardScanner_CollectClusterObjects(t *testing.T) {
	jobHook := &unstructured.Unstructured{}
	jobHook.SetAPIVersion("batch/v1")
	jobHook.SetKind("Job")
	jobHook.SetName("db-migrate")
	jobHook.SetNamespace("prod")
	jobHook.SetAnnotations(map[string]string{
		HelmHookAnnotation: "post-install",
	})

	deployNoHook := &unstructured.Unstructured{}
	deployNoHook.SetAPIVersion("apps/v1")
	deployNoHook.SetKind("Deployment")
	deployNoHook.SetName("api")
	deployNoHook.SetNamespace("prod")

	cmHook := &unstructured.Unstructured{}
	cmHook.SetAPIVersion("v1")
	cmHook.SetKind("ConfigMap")
	cmHook.SetName("hook-cm")
	cmHook.SetNamespace("dev")
	cmHook.SetAnnotations(map[string]string{
		HelmHookAnnotation: "pre-install,post-upgrade",
	})

	client := newLifecycleFakeClient(jobHook, deployNoHook, cmHook)
	scanner := &LifecycleHazardScanner{client: client}

	objects, err := scanner.collectClusterObjects(context.Background())
	if err != nil {
		t.Fatalf("collectClusterObjects failed: %v", err)
	}

	if len(objects) != 2 {
		t.Fatalf("collectClusterObjects len = %d, want 2", len(objects))
	}

	names := map[string]bool{}
	for _, obj := range objects {
		names[obj.GetName()] = true
	}
	if !names["db-migrate"] || !names["hook-cm"] {
		t.Fatalf("collectClusterObjects missing expected hook objects: got names=%v", names)
	}
}

func TestLifecycleHazardScanner_CollectNamespaceObjects(t *testing.T) {
	jobProd := &unstructured.Unstructured{}
	jobProd.SetAPIVersion("batch/v1")
	jobProd.SetKind("Job")
	jobProd.SetName("job-prod")
	jobProd.SetNamespace("prod")
	jobProd.SetAnnotations(map[string]string{
		HelmHookAnnotation: "post-install",
	})

	jobDev := &unstructured.Unstructured{}
	jobDev.SetAPIVersion("batch/v1")
	jobDev.SetKind("Job")
	jobDev.SetName("job-dev")
	jobDev.SetNamespace("dev")
	jobDev.SetAnnotations(map[string]string{
		HelmHookAnnotation: "post-upgrade",
	})

	client := newLifecycleFakeClient(jobProd, jobDev)
	scanner := &LifecycleHazardScanner{client: client}

	objects, err := scanner.collectNamespaceObjects(context.Background(), "prod")
	if err != nil {
		t.Fatalf("collectNamespaceObjects failed: %v", err)
	}

	if len(objects) != 1 {
		t.Fatalf("collectNamespaceObjects len = %d, want 1", len(objects))
	}
	if objects[0].GetName() != "job-prod" {
		t.Fatalf("collectNamespaceObjects[0].name = %q, want %q", objects[0].GetName(), "job-prod")
	}
}
