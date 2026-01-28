// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// encodeRelease encodes a helmRelease to the format stored in k8s secrets
func encodeRelease(t *testing.T, release *helmRelease) []byte {
	t.Helper()

	// JSON marshal
	jsonData, err := json.Marshal(release)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}

	// Gzip compress
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	if _, err := gzWriter.Write(jsonData); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gzWriter.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	// Base64 encode
	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	return []byte(encoded)
}

func TestHelmTracerTraceRelease(t *testing.T) {
	ctx := context.Background()

	// Create a test release
	release := &helmRelease{
		Name:      "nginx",
		Namespace: "default",
		Version:   3,
		Info: helmReleaseInfo{
			FirstDeployed: time.Now().Add(-24 * time.Hour),
			LastDeployed:  time.Now().Add(-1 * time.Hour),
			Status:        "deployed",
			Description:   "Install complete",
		},
		Chart: helmChart{
			Metadata: helmChartMetadata{
				Name:       "nginx",
				Version:    "15.3.2",
				AppVersion: "1.25.3",
				Sources:    []string{"https://github.com/bitnami/charts"},
			},
		},
		Manifest: `---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
  namespace: default
---
apiVersion: v1
kind: Service
metadata:
  name: nginx
  namespace: default
`,
	}

	// Create fake k8s client with the release secret
	fakeClient := fake.NewSimpleClientset(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sh.helm.release.v1.nginx.v3",
				Namespace: "default",
				Labels: map[string]string{
					"owner": "helm",
					"name":  "nginx",
				},
			},
			Data: map[string][]byte{
				"release": encodeRelease(t, release),
			},
		},
	)

	tracer := NewHelmTracer(fakeClient)

	// Test TraceRelease
	result, err := tracer.TraceRelease(ctx, "nginx", "default")
	if err != nil {
		t.Fatalf("TraceRelease error: %v", err)
	}

	if result.Tool != "helm" {
		t.Errorf("Tool = %q, want %q", result.Tool, "helm")
	}

	if !result.FullyManaged {
		t.Errorf("FullyManaged = false, want true")
	}

	// Should have 2 links when tracing a Release: HelmChart, Release
	// (no redundant resource link when tracing release directly)
	if len(result.Chain) != 2 {
		t.Errorf("Chain length = %d, want 2", len(result.Chain))
	}

	// First link should be HelmChart
	if result.Chain[0].Kind != "HelmChart" {
		t.Errorf("Chain[0].Kind = %q, want %q", result.Chain[0].Kind, "HelmChart")
	}
	if result.Chain[0].Name != "nginx" {
		t.Errorf("Chain[0].Name = %q, want %q", result.Chain[0].Name, "nginx")
	}

	// Second link should be Release
	if result.Chain[1].Kind != "Release" {
		t.Errorf("Chain[1].Kind = %q, want %q", result.Chain[1].Kind, "Release")
	}
	if !result.Chain[1].Ready {
		t.Errorf("Chain[1].Ready = false, want true (status is deployed)")
	}
}

func TestHelmTracerTraceDeployment(t *testing.T) {
	ctx := context.Background()

	release := &helmRelease{
		Name:      "redis",
		Namespace: "cache",
		Version:   1,
		Info: helmReleaseInfo{
			Status:      "deployed",
			Description: "Install complete",
		},
		Chart: helmChart{
			Metadata: helmChartMetadata{
				Name:    "redis",
				Version: "18.0.0",
			},
		},
		Manifest: `---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis-master
  namespace: cache
`,
	}

	fakeClient := fake.NewSimpleClientset(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sh.helm.release.v1.redis.v1",
				Namespace: "cache",
				Labels: map[string]string{
					"owner": "helm",
					"name":  "redis",
				},
			},
			Data: map[string][]byte{
				"release": encodeRelease(t, release),
			},
		},
	)

	tracer := NewHelmTracer(fakeClient)

	// Test Trace for a Deployment
	result, err := tracer.Trace(ctx, "Deployment", "redis-master", "cache")
	if err != nil {
		t.Fatalf("Trace error: %v", err)
	}

	if result.Error != "" {
		t.Errorf("Unexpected error: %s", result.Error)
	}

	if !result.FullyManaged {
		t.Errorf("FullyManaged = false, want true")
	}

	// Chain should include Deployment
	foundDeployment := false
	for _, link := range result.Chain {
		if link.Kind == "Deployment" && link.Name == "redis-master" {
			foundDeployment = true
			break
		}
	}
	if !foundDeployment {
		t.Error("Deployment not found in trace chain")
	}
}

func TestHelmTracerNoReleaseFound(t *testing.T) {
	ctx := context.Background()

	// Empty cluster - no Helm releases
	fakeClient := fake.NewSimpleClientset()
	tracer := NewHelmTracer(fakeClient)

	result, err := tracer.Trace(ctx, "Deployment", "orphan", "default")
	if err != nil {
		t.Fatalf("Trace error: %v", err)
	}

	if result.Error == "" {
		t.Error("Expected error message for missing release")
	}

	if result.FullyManaged {
		t.Error("FullyManaged should be false for orphan resource")
	}
}

func TestHelmTracerFailedRelease(t *testing.T) {
	ctx := context.Background()

	release := &helmRelease{
		Name:      "broken",
		Namespace: "default",
		Version:   1,
		Info: helmReleaseInfo{
			Status:      "failed",
			Description: "install failed: timeout",
		},
		Chart: helmChart{
			Metadata: helmChartMetadata{
				Name:    "broken-chart",
				Version: "1.0.0",
			},
		},
		Manifest: `---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: broken-app
  namespace: default
`,
	}

	fakeClient := fake.NewSimpleClientset(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sh.helm.release.v1.broken.v1",
				Namespace: "default",
				Labels: map[string]string{
					"owner": "helm",
					"name":  "broken",
				},
			},
			Data: map[string][]byte{
				"release": encodeRelease(t, release),
			},
		},
	)

	tracer := NewHelmTracer(fakeClient)

	result, err := tracer.TraceRelease(ctx, "broken", "default")
	if err != nil {
		t.Fatalf("TraceRelease error: %v", err)
	}

	if result.FullyManaged {
		t.Error("FullyManaged should be false for failed release")
	}

	// Find the Release link
	for _, link := range result.Chain {
		if link.Kind == "Release" {
			if link.Ready {
				t.Error("Release.Ready should be false for failed status")
			}
			if link.Status != "failed" {
				t.Errorf("Release.Status = %q, want %q", link.Status, "failed")
			}
		}
	}
}

func TestHelmTracerAvailable(t *testing.T) {
	// With client
	fakeClient := fake.NewSimpleClientset()
	tracer := NewHelmTracer(fakeClient)
	if !tracer.Available() {
		t.Error("Available() = false with client, want true")
	}

	// Without client
	tracer2 := NewHelmTracer(nil)
	if tracer2.Available() {
		t.Error("Available() = true without client, want false")
	}
}

func TestHelmTracerToolName(t *testing.T) {
	tracer := NewHelmTracer(nil)
	if tracer.ToolName() != "helm" {
		t.Errorf("ToolName() = %q, want %q", tracer.ToolName(), "helm")
	}
}

// ============================================================================
// History Feature Tests (Task 4: Helm history extraction)
// ============================================================================

func TestHelmReleaseHistory(t *testing.T) {
	ctx := context.Background()

	// Create multiple versions of the same release
	releaseV1 := &helmRelease{
		Name:      "nginx",
		Namespace: "default",
		Version:   1,
		Info: helmReleaseInfo{
			FirstDeployed: time.Date(2026, 1, 25, 10, 0, 0, 0, time.UTC),
			LastDeployed:  time.Date(2026, 1, 25, 10, 0, 0, 0, time.UTC),
			Status:        "superseded",
			Description:   "Install complete",
		},
		Chart: helmChart{
			Metadata: helmChartMetadata{
				Name:    "nginx",
				Version: "15.0.0",
			},
		},
	}

	releaseV2 := &helmRelease{
		Name:      "nginx",
		Namespace: "default",
		Version:   2,
		Info: helmReleaseInfo{
			FirstDeployed: time.Date(2026, 1, 25, 10, 0, 0, 0, time.UTC),
			LastDeployed:  time.Date(2026, 1, 26, 14, 0, 0, 0, time.UTC),
			Status:        "superseded",
			Description:   "Upgrade complete",
		},
		Chart: helmChart{
			Metadata: helmChartMetadata{
				Name:    "nginx",
				Version: "15.1.0",
			},
		},
	}

	releaseV3 := &helmRelease{
		Name:      "nginx",
		Namespace: "default",
		Version:   3,
		Info: helmReleaseInfo{
			FirstDeployed: time.Date(2026, 1, 25, 10, 0, 0, 0, time.UTC),
			LastDeployed:  time.Date(2026, 1, 28, 9, 0, 0, 0, time.UTC),
			Status:        "deployed",
			Description:   "Upgrade complete",
		},
		Chart: helmChart{
			Metadata: helmChartMetadata{
				Name:    "nginx",
				Version: "15.3.0",
			},
		},
	}

	fakeClient := fake.NewSimpleClientset(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sh.helm.release.v1.nginx.v1",
				Namespace: "default",
				Labels:    map[string]string{"owner": "helm", "name": "nginx"},
			},
			Data: map[string][]byte{"release": encodeRelease(t, releaseV1)},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sh.helm.release.v1.nginx.v2",
				Namespace: "default",
				Labels:    map[string]string{"owner": "helm", "name": "nginx"},
			},
			Data: map[string][]byte{"release": encodeRelease(t, releaseV2)},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sh.helm.release.v1.nginx.v3",
				Namespace: "default",
				Labels:    map[string]string{"owner": "helm", "name": "nginx"},
			},
			Data: map[string][]byte{"release": encodeRelease(t, releaseV3)},
		},
	)

	tracer := NewHelmTracer(fakeClient)

	history, err := tracer.GetReleaseHistory(ctx, "nginx", "default")
	if err != nil {
		t.Fatalf("GetReleaseHistory error: %v", err)
	}

	// Should have 3 history entries
	if len(history) != 3 {
		t.Fatalf("Expected 3 history entries, got %d", len(history))
	}

	// History should be sorted by version descending (most recent first)
	if history[0].Revision != "v3" {
		t.Errorf("History[0].Revision = %q, want %q", history[0].Revision, "v3")
	}
	if history[0].Status != "deployed" {
		t.Errorf("History[0].Status = %q, want %q", history[0].Status, "deployed")
	}

	if history[1].Revision != "v2" {
		t.Errorf("History[1].Revision = %q, want %q", history[1].Revision, "v2")
	}
	if history[1].Status != "superseded" {
		t.Errorf("History[1].Status = %q, want %q", history[1].Status, "superseded")
	}

	if history[2].Revision != "v1" {
		t.Errorf("History[2].Revision = %q, want %q", history[2].Revision, "v1")
	}
}

func TestHelmHistorySingleRelease(t *testing.T) {
	ctx := context.Background()

	release := &helmRelease{
		Name:      "redis",
		Namespace: "cache",
		Version:   1,
		Info: helmReleaseInfo{
			FirstDeployed: time.Date(2026, 1, 28, 10, 0, 0, 0, time.UTC),
			LastDeployed:  time.Date(2026, 1, 28, 10, 0, 0, 0, time.UTC),
			Status:        "deployed",
			Description:   "Install complete",
		},
		Chart: helmChart{
			Metadata: helmChartMetadata{
				Name:    "redis",
				Version: "18.0.0",
			},
		},
	}

	fakeClient := fake.NewSimpleClientset(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sh.helm.release.v1.redis.v1",
				Namespace: "cache",
				Labels:    map[string]string{"owner": "helm", "name": "redis"},
			},
			Data: map[string][]byte{"release": encodeRelease(t, release)},
		},
	)

	tracer := NewHelmTracer(fakeClient)

	history, err := tracer.GetReleaseHistory(ctx, "redis", "cache")
	if err != nil {
		t.Fatalf("GetReleaseHistory error: %v", err)
	}

	if len(history) != 1 {
		t.Fatalf("Expected 1 history entry, got %d", len(history))
	}

	if history[0].Revision != "v1" {
		t.Errorf("History[0].Revision = %q, want %q", history[0].Revision, "v1")
	}
	if history[0].Status != "deployed" {
		t.Errorf("History[0].Status = %q, want %q", history[0].Status, "deployed")
	}
	if history[0].Message != "Install complete" {
		t.Errorf("History[0].Message = %q, want %q", history[0].Message, "Install complete")
	}
}

func TestHelmHistoryNoRelease(t *testing.T) {
	ctx := context.Background()

	fakeClient := fake.NewSimpleClientset()
	tracer := NewHelmTracer(fakeClient)

	history, err := tracer.GetReleaseHistory(ctx, "nonexistent", "default")
	if err != nil {
		t.Fatalf("GetReleaseHistory error: %v", err)
	}

	if len(history) != 0 {
		t.Errorf("Expected empty history, got %d entries", len(history))
	}
}
