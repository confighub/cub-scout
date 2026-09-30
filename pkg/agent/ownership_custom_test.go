// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDetectOwnership_CustomDetectors(t *testing.T) {
	tmpDir := t.TempDir()
	detectors := filepath.Join(tmpDir, "detectors.yaml")
	if err := os.WriteFile(detectors, []byte(`
detectors:
  - name: internal-platform
    labels:
      - key: platform.company.com/managed-by
        value: "platform-controller"
    owner_name: "Internal Platform"
    owner_type: "custom"
`), 0o600); err != nil {
		t.Fatalf("write detectors file: %v", err)
	}

	t.Setenv("CUB_SCOUT_OWNERSHIP_DETECTORS", detectors)

	resource := newTestResource("default", "payments-api", map[string]string{
		"platform.company.com/managed-by": "platform-controller",
	}, nil)

	ownership := DetectOwnership(resource)
	if ownership.Type != "custom" {
		t.Fatalf("Type = %q, want %q", ownership.Type, "custom")
	}
	if ownership.Name != "Internal Platform" {
		t.Fatalf("Name = %q, want %q", ownership.Name, "Internal Platform")
	}
	if ownership.Source != "custom:internal-platform" {
		t.Fatalf("Source = %q, want %q", ownership.Source, "custom:internal-platform")
	}
}

func TestDetectOwnershipBuiltinNeverReadsMutableCustomDetectorConfig(t *testing.T) {
	tmpDir := t.TempDir()
	detectors := filepath.Join(tmpDir, "detectors.yaml")
	if err := os.WriteFile(detectors, []byte(`
detectors:
  - name: local-owner
    labels:
      - key: platform.example/managed-by
        value: controller
    owner_name: Initial Owner
    owner_type: custom
`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(customDetectorsEnvVar, detectors)
	resource := newTestResource("prod", "api", map[string]string{"platform.example/managed-by": "controller"}, nil)
	if got := DetectOwnershipBuiltin(resource); got.Type != OwnerUnknown {
		t.Fatalf("built-in-only owner = %#v, want unknown", got)
	}
	if got := DetectOwnership(resource); got.Type != OwnerCustom || got.Name != "Initial Owner" {
		t.Fatalf("normal detector = %#v, want initial custom detector", got)
	}
	if err := os.WriteFile(detectors, []byte(`
detectors:
  - name: local-owner
    labels:
      - key: platform.example/managed-by
        value: controller
    owner_name: Changed Owner
    owner_type: custom
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := DetectOwnershipBuiltin(resource); got.Type != OwnerUnknown {
		t.Fatalf("built-in-only owner changed after config mutation: %#v", got)
	}
	if got := DetectOwnership(resource); got.Type != OwnerCustom || got.Name != "Changed Owner" {
		t.Fatalf("normal detector did not observe config change: %#v", got)
	}
}

func TestDetectOwnership_CustomDetectorPrecedence(t *testing.T) {
	tmpDir := t.TempDir()
	detectors := filepath.Join(tmpDir, "detectors.yaml")
	if err := os.WriteFile(detectors, []byte(`
detectors:
  - name: first
    labels:
      - key: platform.company.com/managed-by
    owner_name: "First Controller"
    owner_type: "custom"
  - name: second
    labels:
      - key: platform.company.com/managed-by
    owner_name: "Second Controller"
    owner_type: "custom"
`), 0o600); err != nil {
		t.Fatalf("write detectors file: %v", err)
	}

	t.Setenv("CUB_SCOUT_OWNERSHIP_DETECTORS", detectors)

	resource := newTestResource("default", "edge", map[string]string{
		"platform.company.com/managed-by": "anything",
	}, nil)

	ownership := DetectOwnership(resource)
	if ownership.Type != "custom" {
		t.Fatalf("Type = %q, want %q", ownership.Type, "custom")
	}
	if ownership.Name != "First Controller" {
		t.Fatalf("Name = %q, want %q", ownership.Name, "First Controller")
	}
	if ownership.Source != "custom:first" {
		t.Fatalf("Source = %q, want %q", ownership.Source, "custom:first")
	}
}

func TestDetectOwnership_CustomDetectorInvalidConfigFallsBack(t *testing.T) {
	tmpDir := t.TempDir()
	detectors := filepath.Join(tmpDir, "detectors.yaml")
	if err := os.WriteFile(detectors, []byte("detectors: ["), 0o600); err != nil {
		t.Fatalf("write detectors file: %v", err)
	}

	t.Setenv("CUB_SCOUT_OWNERSHIP_DETECTORS", detectors)

	resource := newTestResourceWithOwners("default", "payments-api", []metav1.OwnerReference{
		{
			Kind: "Deployment",
			Name: "payments-api",
			UID:  "abc-123",
		},
	})

	warnOut := captureStderr(t, func() {
		ownership := DetectOwnership(resource)
		if ownership.Type != OwnerKubernetes {
			t.Fatalf("Type = %q, want %q", ownership.Type, OwnerKubernetes)
		}
	})

	if !strings.Contains(strings.ToLower(warnOut), "custom ownership detector") {
		t.Fatalf("expected warning for invalid custom detector config, got %q", warnOut)
	}
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	os.Stderr = w

	done := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(r)
		done <- string(data)
	}()

	fn()

	_ = w.Close()
	os.Stderr = oldStderr

	return <-done
}
