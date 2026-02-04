// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
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
