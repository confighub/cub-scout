// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"strings"
	"testing"
)

func TestDriftComparator_CompareReplicas(t *testing.T) {
	comparator := &DriftComparator{
		options: DefaultDriftOptions(),
	}

	tests := []struct {
		name           string
		desired        map[string]interface{}
		live           map[string]interface{}
		wantFinding    bool
		wantSeverity   DriftSeverity
	}{
		{
			name: "no drift - same replicas",
			desired: map[string]interface{}{
				"kind":       "Deployment",
				"apiVersion": "apps/v1",
				"metadata":   map[string]interface{}{"name": "web", "namespace": "prod"},
				"spec":       map[string]interface{}{"replicas": 3},
			},
			live: map[string]interface{}{
				"kind":       "Deployment",
				"apiVersion": "apps/v1",
				"metadata":   map[string]interface{}{"name": "web", "namespace": "prod"},
				"spec":       map[string]interface{}{"replicas": 3},
			},
			wantFinding: false,
		},
		{
			name: "drift - scaled up (live > desired)",
			desired: map[string]interface{}{
				"kind":       "Deployment",
				"apiVersion": "apps/v1",
				"metadata":   map[string]interface{}{"name": "web", "namespace": "prod"},
				"spec":       map[string]interface{}{"replicas": 3},
			},
			live: map[string]interface{}{
				"kind":       "Deployment",
				"apiVersion": "apps/v1",
				"metadata":   map[string]interface{}{"name": "web", "namespace": "prod"},
				"spec":       map[string]interface{}{"replicas": 5},
			},
			wantFinding:  true,
			wantSeverity: DriftSeverityInfo, // Scale up is less concerning
		},
		{
			name: "drift - scaled down (live < desired)",
			desired: map[string]interface{}{
				"kind":       "Deployment",
				"apiVersion": "apps/v1",
				"metadata":   map[string]interface{}{"name": "web", "namespace": "prod"},
				"spec":       map[string]interface{}{"replicas": 5},
			},
			live: map[string]interface{}{
				"kind":       "Deployment",
				"apiVersion": "apps/v1",
				"metadata":   map[string]interface{}{"name": "web", "namespace": "prod"},
				"spec":       map[string]interface{}{"replicas": 2},
			},
			wantFinding:  true,
			wantSeverity: DriftSeverityWarning, // Scale down is more concerning
		},
		{
			name: "no drift - desired has no replicas",
			desired: map[string]interface{}{
				"kind":       "Deployment",
				"apiVersion": "apps/v1",
				"metadata":   map[string]interface{}{"name": "web", "namespace": "prod"},
				"spec":       map[string]interface{}{},
			},
			live: map[string]interface{}{
				"kind":       "Deployment",
				"apiVersion": "apps/v1",
				"metadata":   map[string]interface{}{"name": "web", "namespace": "prod"},
				"spec":       map[string]interface{}{"replicas": 3},
			},
			wantFinding: false, // No replicas in desired = not drift
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := comparator.compareResource(tt.desired, tt.live)

			// Filter for replica findings
			var replicaFindings []DriftFinding
			for _, f := range findings {
				if f.Path == "spec.replicas" {
					replicaFindings = append(replicaFindings, f)
				}
			}

			if tt.wantFinding && len(replicaFindings) == 0 {
				t.Errorf("expected replica finding, got none")
			}
			if !tt.wantFinding && len(replicaFindings) > 0 {
				t.Errorf("expected no replica finding, got %d", len(replicaFindings))
			}
			if tt.wantFinding && len(replicaFindings) > 0 {
				if replicaFindings[0].Severity != tt.wantSeverity {
					t.Errorf("severity = %s, want %s", replicaFindings[0].Severity, tt.wantSeverity)
				}
			}
		})
	}
}

func TestDriftComparator_CompareImages(t *testing.T) {
	comparator := &DriftComparator{
		options: DefaultDriftOptions(),
	}

	tests := []struct {
		name         string
		desired      map[string]interface{}
		live         map[string]interface{}
		wantFinding  bool
		wantSeverity DriftSeverity
	}{
		{
			name: "no drift - same image",
			desired: map[string]interface{}{
				"kind":       "Deployment",
				"apiVersion": "apps/v1",
				"metadata":   map[string]interface{}{"name": "web", "namespace": "prod"},
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{"name": "app", "image": "nginx:1.19"},
							},
						},
					},
				},
			},
			live: map[string]interface{}{
				"kind":       "Deployment",
				"apiVersion": "apps/v1",
				"metadata":   map[string]interface{}{"name": "web", "namespace": "prod"},
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{"name": "app", "image": "nginx:1.19"},
							},
						},
					},
				},
			},
			wantFinding: false,
		},
		{
			name: "drift - different tag (same repo)",
			desired: map[string]interface{}{
				"kind":       "Deployment",
				"apiVersion": "apps/v1",
				"metadata":   map[string]interface{}{"name": "web", "namespace": "prod"},
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{"name": "app", "image": "nginx:1.19"},
							},
						},
					},
				},
			},
			live: map[string]interface{}{
				"kind":       "Deployment",
				"apiVersion": "apps/v1",
				"metadata":   map[string]interface{}{"name": "web", "namespace": "prod"},
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{"name": "app", "image": "nginx:1.20"},
							},
						},
					},
				},
			},
			wantFinding:  true,
			wantSeverity: DriftSeverityWarning, // Same repo, different tag
		},
		{
			name: "drift - different repo",
			desired: map[string]interface{}{
				"kind":       "Deployment",
				"apiVersion": "apps/v1",
				"metadata":   map[string]interface{}{"name": "web", "namespace": "prod"},
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{"name": "app", "image": "nginx:1.19"},
							},
						},
					},
				},
			},
			live: map[string]interface{}{
				"kind":       "Deployment",
				"apiVersion": "apps/v1",
				"metadata":   map[string]interface{}{"name": "web", "namespace": "prod"},
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{"name": "app", "image": "apache:2.4"},
							},
						},
					},
				},
			},
			wantFinding:  true,
			wantSeverity: DriftSeverityCritical, // Different repo entirely
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := comparator.compareResource(tt.desired, tt.live)

			// Filter for image findings
			var imageFindings []DriftFinding
			for _, f := range findings {
				if f.Classification == DriftImage {
					imageFindings = append(imageFindings, f)
				}
			}

			if tt.wantFinding && len(imageFindings) == 0 {
				t.Errorf("expected image finding, got none")
			}
			if !tt.wantFinding && len(imageFindings) > 0 {
				t.Errorf("expected no image finding, got %d", len(imageFindings))
			}
			if tt.wantFinding && len(imageFindings) > 0 {
				if imageFindings[0].Severity != tt.wantSeverity {
					t.Errorf("severity = %s, want %s", imageFindings[0].Severity, tt.wantSeverity)
				}
			}
		})
	}
}

func TestDriftComparator_CompareFromResources(t *testing.T) {
	// Test comparing resources directly (no cluster)
	comparator := &DriftComparator{
		options: DefaultDriftOptions(),
	}

	desired := []map[string]interface{}{
		{
			"kind":       "Deployment",
			"apiVersion": "apps/v1",
			"metadata":   map[string]interface{}{"name": "web", "namespace": "prod"},
			"spec": map[string]interface{}{
				"replicas": 3,
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{"name": "app", "image": "nginx:1.19"},
						},
					},
				},
			},
		},
	}

	// Without cluster access, compare should return no findings (no live to compare)
	ctx := context.Background()
	findings, err := comparator.CompareFromResources(ctx, desired)
	if err != nil {
		t.Fatalf("CompareFromResources: %v", err)
	}

	// Since there's no cluster, we expect no findings
	if len(findings) != 0 {
		t.Errorf("expected 0 findings without cluster, got %d", len(findings))
	}
}

func TestIsWorkloadKind(t *testing.T) {
	tests := []struct {
		kind string
		want bool
	}{
		{"Deployment", true},
		{"StatefulSet", true},
		{"DaemonSet", true},
		{"ReplicaSet", true},
		{"Job", true},
		{"CronJob", true},
		{"Pod", false},
		{"Service", false},
		{"ConfigMap", false},
		{"Secret", false},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			got := isWorkloadKind(tt.kind)
			if got != tt.want {
				t.Errorf("isWorkloadKind(%s) = %v, want %v", tt.kind, got, tt.want)
			}
		})
	}
}

func TestClassifyImageSeverity(t *testing.T) {
	tests := []struct {
		desired  string
		live     string
		want     DriftSeverity
	}{
		// Same repo, different tag -> warning
		{"nginx:1.19", "nginx:1.20", DriftSeverityWarning},
		{"nginx:latest", "nginx:1.20", DriftSeverityWarning},
		{"gcr.io/project/app:v1", "gcr.io/project/app:v2", DriftSeverityWarning},
		// Different repo -> critical
		{"nginx:1.19", "apache:2.4", DriftSeverityCritical},
		{"gcr.io/project/app:v1", "gcr.io/other/app:v1", DriftSeverityCritical},
	}

	for _, tt := range tests {
		t.Run(tt.desired+"_vs_"+tt.live, func(t *testing.T) {
			got := classifyImageSeverity(tt.desired, tt.live)
			if got != tt.want {
				t.Errorf("classifyImageSeverity(%s, %s) = %s, want %s",
					tt.desired, tt.live, got, tt.want)
			}
		})
	}
}

func TestClassifyReplicaSeverity(t *testing.T) {
	tests := []struct {
		desired int
		live    int
		want    DriftSeverity
	}{
		// Live < desired (scaled down) -> warning
		{5, 2, DriftSeverityWarning},
		{3, 1, DriftSeverityWarning},
		// Live > desired (scaled up) -> info
		{2, 5, DriftSeverityInfo},
		{1, 3, DriftSeverityInfo},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := classifyReplicaSeverity(tt.desired, tt.live)
			if got != tt.want {
				t.Errorf("classifyReplicaSeverity(%d, %d) = %s, want %s",
					tt.desired, tt.live, got, tt.want)
			}
		})
	}
}

// TestDriftComparator_CompareEnvVars tests environment variable drift detection.
func TestDriftComparator_CompareEnvVars(t *testing.T) {
	comparator := &DriftComparator{
		options: DriftOptions{
			IncludeEnv: true,
		},
	}

	tests := []struct {
		name          string
		desired       map[string]interface{}
		live          map[string]interface{}
		wantFindings  int
		wantPaths     []string
		checkDesired  map[string]interface{} // path -> expected desired value
		checkLive     map[string]interface{} // path -> expected live value
	}{
		{
			name: "no drift - same env vars",
			desired: makeDeploymentWithEnv("web", "prod", []map[string]interface{}{
				{"name": "LOG_LEVEL", "value": "info"},
				{"name": "PORT", "value": "8080"},
			}),
			live: makeDeploymentWithEnv("web", "prod", []map[string]interface{}{
				{"name": "LOG_LEVEL", "value": "info"},
				{"name": "PORT", "value": "8080"},
			}),
			wantFindings: 0,
		},
		{
			name: "drift - changed value",
			desired: makeDeploymentWithEnv("web", "prod", []map[string]interface{}{
				{"name": "LOG_LEVEL", "value": "info"},
			}),
			live: makeDeploymentWithEnv("web", "prod", []map[string]interface{}{
				{"name": "LOG_LEVEL", "value": "debug"},
			}),
			wantFindings: 1,
			wantPaths:    []string{"spec.template.spec.containers[name=app].env[name=LOG_LEVEL]"},
			checkDesired: map[string]interface{}{
				"spec.template.spec.containers[name=app].env[name=LOG_LEVEL]": "info",
			},
			checkLive: map[string]interface{}{
				"spec.template.spec.containers[name=app].env[name=LOG_LEVEL]": "debug",
			},
		},
		{
			name: "drift - added var (in live, not in desired)",
			desired: makeDeploymentWithEnv("web", "prod", []map[string]interface{}{
				{"name": "LOG_LEVEL", "value": "info"},
			}),
			live: makeDeploymentWithEnv("web", "prod", []map[string]interface{}{
				{"name": "LOG_LEVEL", "value": "info"},
				{"name": "DEBUG", "value": "true"},
			}),
			wantFindings: 1,
			wantPaths:    []string{"spec.template.spec.containers[name=app].env[name=DEBUG]"},
			checkDesired: map[string]interface{}{
				"spec.template.spec.containers[name=app].env[name=DEBUG]": nil,
			},
			checkLive: map[string]interface{}{
				"spec.template.spec.containers[name=app].env[name=DEBUG]": "true",
			},
		},
		{
			name: "drift - removed var (in desired, not in live)",
			desired: makeDeploymentWithEnv("web", "prod", []map[string]interface{}{
				{"name": "LOG_LEVEL", "value": "info"},
				{"name": "FEATURE_FLAG", "value": "enabled"},
			}),
			live: makeDeploymentWithEnv("web", "prod", []map[string]interface{}{
				{"name": "LOG_LEVEL", "value": "info"},
			}),
			wantFindings: 1,
			wantPaths:    []string{"spec.template.spec.containers[name=app].env[name=FEATURE_FLAG]"},
			checkDesired: map[string]interface{}{
				"spec.template.spec.containers[name=app].env[name=FEATURE_FLAG]": "enabled",
			},
			checkLive: map[string]interface{}{
				"spec.template.spec.containers[name=app].env[name=FEATURE_FLAG]": nil,
			},
		},
		{
			name: "drift - multiple changes",
			desired: makeDeploymentWithEnv("web", "prod", []map[string]interface{}{
				{"name": "A_VAR", "value": "a"},
				{"name": "B_VAR", "value": "b"},
				{"name": "C_VAR", "value": "c"},
			}),
			live: makeDeploymentWithEnv("web", "prod", []map[string]interface{}{
				{"name": "A_VAR", "value": "a"},       // same
				{"name": "B_VAR", "value": "changed"}, // changed
				{"name": "D_VAR", "value": "d"},       // added
				// C_VAR removed
			}),
			wantFindings: 3, // B changed, C removed, D added
		},
		{
			name: "no drift - empty env in both",
			desired: makeDeploymentWithEnv("web", "prod", nil),
			live:    makeDeploymentWithEnv("web", "prod", nil),
			wantFindings: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := comparator.compareResource(tt.desired, tt.live)

			// Filter for env findings
			var envFindings []DriftFinding
			for _, f := range findings {
				if f.Classification == DriftConfig {
					envFindings = append(envFindings, f)
				}
			}

			if len(envFindings) != tt.wantFindings {
				t.Errorf("got %d env findings, want %d", len(envFindings), tt.wantFindings)
				for _, f := range envFindings {
					t.Logf("  finding: %s (desired=%v, live=%v)", f.Path, f.Desired, f.Live)
				}
			}

			// Check specific paths if specified
			if tt.wantPaths != nil {
				for _, wantPath := range tt.wantPaths {
					found := false
					for _, f := range envFindings {
						if f.Path == wantPath {
							found = true
							// Check desired/live values if specified
							if tt.checkDesired != nil {
								if expected, ok := tt.checkDesired[wantPath]; ok {
									if f.Desired != expected {
										t.Errorf("path %s: desired = %v, want %v", wantPath, f.Desired, expected)
									}
								}
							}
							if tt.checkLive != nil {
								if expected, ok := tt.checkLive[wantPath]; ok {
									if f.Live != expected {
										t.Errorf("path %s: live = %v, want %v", wantPath, f.Live, expected)
									}
								}
							}
							break
						}
					}
					if !found {
						t.Errorf("expected finding for path %s, not found", wantPath)
					}
				}
			}

			// Verify all findings have warning severity and config classification
			for _, f := range envFindings {
				if f.Severity != DriftSeverityWarning {
					t.Errorf("finding %s: severity = %s, want warning", f.Path, f.Severity)
				}
				if f.Classification != DriftConfig {
					t.Errorf("finding %s: classification = %s, want config", f.Path, f.Classification)
				}
			}
		})
	}
}

// TestDriftComparator_EnvVars_Determinism verifies that reordered env lists produce identical output.
func TestDriftComparator_EnvVars_Determinism(t *testing.T) {
	comparator := &DriftComparator{
		options: DriftOptions{
			IncludeEnv: true,
		},
	}

	// Desired has vars in one order
	desired := makeDeploymentWithEnv("web", "prod", []map[string]interface{}{
		{"name": "ZEBRA", "value": "z"},
		{"name": "ALPHA", "value": "a"},
		{"name": "MIKE", "value": "m"},
	})

	// Live has different values, in different order
	live := makeDeploymentWithEnv("web", "prod", []map[string]interface{}{
		{"name": "MIKE", "value": "changed_m"},
		{"name": "ALPHA", "value": "changed_a"},
		{"name": "ZEBRA", "value": "changed_z"},
	})

	// Run comparison twice
	findings1 := comparator.compareResource(desired, live)
	findings2 := comparator.compareResource(desired, live)

	// Filter for env findings
	var env1, env2 []DriftFinding
	for _, f := range findings1 {
		if f.Classification == DriftConfig {
			env1 = append(env1, f)
		}
	}
	for _, f := range findings2 {
		if f.Classification == DriftConfig {
			env2 = append(env2, f)
		}
	}

	if len(env1) != len(env2) {
		t.Fatalf("non-deterministic: got %d findings first, %d second", len(env1), len(env2))
	}

	// Verify findings are in same order (alphabetical by var name)
	for i := range env1 {
		if env1[i].Path != env2[i].Path {
			t.Errorf("non-deterministic order: position %d has %s vs %s", i, env1[i].Path, env2[i].Path)
		}
	}

	// Verify alphabetical order (ALPHA, MIKE, ZEBRA)
	expectedOrder := []string{
		"spec.template.spec.containers[name=app].env[name=ALPHA]",
		"spec.template.spec.containers[name=app].env[name=MIKE]",
		"spec.template.spec.containers[name=app].env[name=ZEBRA]",
	}
	for i, expected := range expectedOrder {
		if i >= len(env1) {
			t.Errorf("missing finding at position %d", i)
			continue
		}
		if env1[i].Path != expected {
			t.Errorf("position %d: got %s, want %s", i, env1[i].Path, expected)
		}
	}
}

// TestDriftComparator_EnvVars_MultipleContainers tests env comparison across multiple containers.
func TestDriftComparator_EnvVars_MultipleContainers(t *testing.T) {
	comparator := &DriftComparator{
		options: DriftOptions{
			IncludeEnv: true,
		},
	}

	desired := map[string]interface{}{
		"kind":       "Deployment",
		"apiVersion": "apps/v1",
		"metadata":   map[string]interface{}{"name": "web", "namespace": "prod"},
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "nginx:1.19",
							"env": []interface{}{
								map[string]interface{}{"name": "LOG_LEVEL", "value": "info"},
							},
						},
						map[string]interface{}{
							"name":  "sidecar",
							"image": "envoy:1.0",
							"env": []interface{}{
								map[string]interface{}{"name": "PROXY_PORT", "value": "8080"},
							},
						},
					},
				},
			},
		},
	}

	live := map[string]interface{}{
		"kind":       "Deployment",
		"apiVersion": "apps/v1",
		"metadata":   map[string]interface{}{"name": "web", "namespace": "prod"},
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "nginx:1.19",
							"env": []interface{}{
								map[string]interface{}{"name": "LOG_LEVEL", "value": "debug"}, // changed
							},
						},
						map[string]interface{}{
							"name":  "sidecar",
							"image": "envoy:1.0",
							"env": []interface{}{
								map[string]interface{}{"name": "PROXY_PORT", "value": "9090"}, // changed
							},
						},
					},
				},
			},
		},
	}

	findings := comparator.compareResource(desired, live)

	// Filter for env findings
	var envFindings []DriftFinding
	for _, f := range findings {
		if f.Classification == DriftConfig {
			envFindings = append(envFindings, f)
		}
	}

	if len(envFindings) != 2 {
		t.Errorf("got %d env findings, want 2", len(envFindings))
	}

	// Check that both containers are represented
	foundApp := false
	foundSidecar := false
	for _, f := range envFindings {
		if f.Path == "spec.template.spec.containers[name=app].env[name=LOG_LEVEL]" {
			foundApp = true
		}
		if f.Path == "spec.template.spec.containers[name=sidecar].env[name=PROXY_PORT]" {
			foundSidecar = true
		}
	}

	if !foundApp {
		t.Error("missing finding for app container")
	}
	if !foundSidecar {
		t.Error("missing finding for sidecar container")
	}
}

// TestDriftComparator_CompareResources tests resource requests/limits drift detection.
func TestDriftComparator_CompareResources(t *testing.T) {
	comparator := &DriftComparator{
		options: DriftOptions{
			IncludeResources: true,
		},
	}

	tests := []struct {
		name         string
		desired      map[string]interface{}
		live         map[string]interface{}
		wantFindings int
		wantPaths    []string
		wantSeverity DriftSeverity
	}{
		{
			name:         "no drift - same resources",
			desired:      makeDeploymentWithResources("web", "prod", "100m", "256Mi", "200m", "512Mi"),
			live:         makeDeploymentWithResources("web", "prod", "100m", "256Mi", "200m", "512Mi"),
			wantFindings: 0,
		},
		{
			name:         "drift - cpu request changed",
			desired:      makeDeploymentWithResources("web", "prod", "100m", "256Mi", "200m", "512Mi"),
			live:         makeDeploymentWithResources("web", "prod", "200m", "256Mi", "200m", "512Mi"),
			wantFindings: 1,
			wantPaths:    []string{"spec.template.spec.containers[name=app].resources.requests.cpu"},
			wantSeverity: DriftSeverityWarning,
		},
		{
			name:         "drift - memory limit changed",
			desired:      makeDeploymentWithResources("web", "prod", "100m", "256Mi", "200m", "512Mi"),
			live:         makeDeploymentWithResources("web", "prod", "100m", "256Mi", "200m", "1Gi"),
			wantFindings: 1,
			wantPaths:    []string{"spec.template.spec.containers[name=app].resources.limits.memory"},
			wantSeverity: DriftSeverityWarning,
		},
		{
			name:         "drift - multiple resource changes",
			desired:      makeDeploymentWithResources("web", "prod", "100m", "256Mi", "200m", "512Mi"),
			live:         makeDeploymentWithResources("web", "prod", "200m", "512Mi", "400m", "1Gi"),
			wantFindings: 4, // cpu req, mem req, cpu lim, mem lim
		},
		{
			name:         "drift - resource added (not in desired)",
			desired:      makeDeploymentWithResources("web", "prod", "", "", "", ""),
			live:         makeDeploymentWithResources("web", "prod", "100m", "256Mi", "", ""),
			wantFindings: 2, // cpu req, mem req added
			wantSeverity: DriftSeverityWarning,
		},
		{
			name:         "drift - resource removed (not in live)",
			desired:      makeDeploymentWithResources("web", "prod", "100m", "256Mi", "", ""),
			live:         makeDeploymentWithResources("web", "prod", "", "", "", ""),
			wantFindings: 2, // cpu req, mem req removed
			wantSeverity: DriftSeverityWarning,
		},
		{
			name:         "critical - invalid config (limits < requests)",
			desired:      makeDeploymentWithResources("web", "prod", "100m", "256Mi", "200m", "512Mi"),
			live:         makeDeploymentWithResources("web", "prod", "500m", "256Mi", "200m", "512Mi"), // req > lim
			wantFindings: 1,
			wantPaths:    []string{"spec.template.spec.containers[name=app].resources.requests.cpu"},
			wantSeverity: DriftSeverityCritical,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := comparator.compareResource(tt.desired, tt.live)

			// Filter for resource findings (capacity classification)
			var resourceFindings []DriftFinding
			for _, f := range findings {
				if f.Classification == DriftCapacity && strings.Contains(f.Path, "resources") {
					resourceFindings = append(resourceFindings, f)
				}
			}

			if len(resourceFindings) != tt.wantFindings {
				t.Errorf("got %d resource findings, want %d", len(resourceFindings), tt.wantFindings)
				for _, f := range resourceFindings {
					t.Logf("  finding: %s (desired=%v, live=%v, severity=%s)", f.Path, f.Desired, f.Live, f.Severity)
				}
			}

			// Check specific paths if specified
			if tt.wantPaths != nil {
				for _, wantPath := range tt.wantPaths {
					found := false
					for _, f := range resourceFindings {
						if f.Path == wantPath {
							found = true
							if tt.wantSeverity != "" && f.Severity != tt.wantSeverity {
								t.Errorf("path %s: severity = %s, want %s", wantPath, f.Severity, tt.wantSeverity)
							}
							break
						}
					}
					if !found {
						t.Errorf("expected finding for path %s, not found", wantPath)
					}
				}
			}
		})
	}
}

// TestDriftComparator_Resources_Determinism verifies deterministic output for resources.
func TestDriftComparator_Resources_Determinism(t *testing.T) {
	comparator := &DriftComparator{
		options: DriftOptions{
			IncludeResources: true,
		},
	}

	desired := makeDeploymentWithResources("web", "prod", "100m", "256Mi", "200m", "512Mi")
	live := makeDeploymentWithResources("web", "prod", "200m", "512Mi", "400m", "1Gi")

	// Run twice
	findings1 := comparator.compareResource(desired, live)
	findings2 := comparator.compareResource(desired, live)

	// Filter for resource findings
	var res1, res2 []DriftFinding
	for _, f := range findings1 {
		if f.Classification == DriftCapacity && strings.Contains(f.Path, "resources") {
			res1 = append(res1, f)
		}
	}
	for _, f := range findings2 {
		if f.Classification == DriftCapacity && strings.Contains(f.Path, "resources") {
			res2 = append(res2, f)
		}
	}

	if len(res1) != len(res2) {
		t.Fatalf("non-deterministic: got %d findings first, %d second", len(res1), len(res2))
	}

	// Verify same order (cpu before memory, requests before limits)
	for i := range res1 {
		if res1[i].Path != res2[i].Path {
			t.Errorf("non-deterministic order: position %d has %s vs %s", i, res1[i].Path, res2[i].Path)
		}
	}

	// Verify expected order: requests.cpu, requests.memory, limits.cpu, limits.memory
	expectedPaths := []string{
		"spec.template.spec.containers[name=app].resources.requests.cpu",
		"spec.template.spec.containers[name=app].resources.requests.memory",
		"spec.template.spec.containers[name=app].resources.limits.cpu",
		"spec.template.spec.containers[name=app].resources.limits.memory",
	}
	for i, expected := range expectedPaths {
		if i >= len(res1) {
			t.Errorf("missing finding at position %d", i)
			continue
		}
		if res1[i].Path != expected {
			t.Errorf("position %d: got %s, want %s", i, res1[i].Path, expected)
		}
	}
}

// TestDriftComparator_Resources_MultipleContainers tests resource comparison across containers.
func TestDriftComparator_Resources_MultipleContainers(t *testing.T) {
	comparator := &DriftComparator{
		options: DriftOptions{
			IncludeResources: true,
		},
	}

	desired := map[string]interface{}{
		"kind":       "Deployment",
		"apiVersion": "apps/v1",
		"metadata":   map[string]interface{}{"name": "web", "namespace": "prod"},
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "nginx:1.19",
							"resources": map[string]interface{}{
								"requests": map[string]interface{}{"cpu": "100m"},
							},
						},
						map[string]interface{}{
							"name":  "sidecar",
							"image": "envoy:1.0",
							"resources": map[string]interface{}{
								"requests": map[string]interface{}{"cpu": "50m"},
							},
						},
					},
				},
			},
		},
	}

	live := map[string]interface{}{
		"kind":       "Deployment",
		"apiVersion": "apps/v1",
		"metadata":   map[string]interface{}{"name": "web", "namespace": "prod"},
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "nginx:1.19",
							"resources": map[string]interface{}{
								"requests": map[string]interface{}{"cpu": "200m"}, // changed
							},
						},
						map[string]interface{}{
							"name":  "sidecar",
							"image": "envoy:1.0",
							"resources": map[string]interface{}{
								"requests": map[string]interface{}{"cpu": "100m"}, // changed
							},
						},
					},
				},
			},
		},
	}

	findings := comparator.compareResource(desired, live)

	// Filter for resource findings
	var resourceFindings []DriftFinding
	for _, f := range findings {
		if f.Classification == DriftCapacity && strings.Contains(f.Path, "resources") {
			resourceFindings = append(resourceFindings, f)
		}
	}

	if len(resourceFindings) != 2 {
		t.Errorf("got %d resource findings, want 2", len(resourceFindings))
	}

	// Check both containers are represented
	foundApp := false
	foundSidecar := false
	for _, f := range resourceFindings {
		if strings.Contains(f.Path, "name=app") {
			foundApp = true
		}
		if strings.Contains(f.Path, "name=sidecar") {
			foundSidecar = true
		}
	}

	if !foundApp {
		t.Error("missing finding for app container")
	}
	if !foundSidecar {
		t.Error("missing finding for sidecar container")
	}
}

// TestParseResourceQuantity tests the resource quantity parser.
func TestParseResourceQuantity(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"100m", 0.1},
		{"500m", 0.5},
		{"1", 1},
		{"2", 2},
		{"256Mi", 256 * 1024 * 1024},
		{"1Gi", 1024 * 1024 * 1024},
		{"512Ki", 512 * 1024},
		{"1G", 1000 * 1000 * 1000},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseResourceQuantity(tt.input)
			if got != tt.want {
				t.Errorf("parseResourceQuantity(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// makeDeploymentWithResources creates a test deployment with resource requests/limits.
func makeDeploymentWithResources(name, namespace, cpuReq, memReq, cpuLim, memLim string) map[string]interface{} {
	resources := map[string]interface{}{}

	if cpuReq != "" || memReq != "" {
		requests := map[string]interface{}{}
		if cpuReq != "" {
			requests["cpu"] = cpuReq
		}
		if memReq != "" {
			requests["memory"] = memReq
		}
		resources["requests"] = requests
	}

	if cpuLim != "" || memLim != "" {
		limits := map[string]interface{}{}
		if cpuLim != "" {
			limits["cpu"] = cpuLim
		}
		if memLim != "" {
			limits["memory"] = memLim
		}
		resources["limits"] = limits
	}

	container := map[string]interface{}{
		"name":  "app",
		"image": "nginx:1.19",
	}
	if len(resources) > 0 {
		container["resources"] = resources
	}

	return map[string]interface{}{
		"kind":       "Deployment",
		"apiVersion": "apps/v1",
		"metadata":   map[string]interface{}{"name": name, "namespace": namespace},
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{container},
				},
			},
		},
	}
}

// makeDeploymentWithEnv creates a test deployment with the given env vars.
func makeDeploymentWithEnv(name, namespace string, env []map[string]interface{}) map[string]interface{} {
	var envList []interface{}
	if env != nil {
		for _, e := range env {
			envList = append(envList, e)
		}
	}

	containers := []interface{}{
		map[string]interface{}{
			"name":  "app",
			"image": "nginx:1.19",
		},
	}

	if envList != nil {
		containers[0].(map[string]interface{})["env"] = envList
	}

	return map[string]interface{}{
		"kind":       "Deployment",
		"apiVersion": "apps/v1",
		"metadata":   map[string]interface{}{"name": name, "namespace": namespace},
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": containers,
				},
			},
		},
	}
}
