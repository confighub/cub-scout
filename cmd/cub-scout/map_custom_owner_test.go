// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestProcessResource_CustomOwnershipDisplay(t *testing.T) {
	detectors := writeCustomDetectorsFile(t, `
detectors:
  - name: internal-platform
    labels:
      - key: platform.company.com/managed-by
        value: "platform-controller"
    owner_name: "Internal Platform"
    owner_type: "custom"
`)
	t.Setenv("CUB_SCOUT_OWNERSHIP_DETECTORS", detectors)

	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("apps/v1")
	obj.SetKind("Deployment")
	obj.SetNamespace("default")
	obj.SetName("payments-api")
	obj.SetLabels(map[string]string{
		"platform.company.com/managed-by": "platform-controller",
	})

	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	entries := processResourceWithLookup(obj, gvr, "local", nil, map[string]int{}, mapApplicationSetLookup{
		byNamespacedName: map[string]int{},
		byName:           map[string]int{},
	})

	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(entries))
	}
	if entries[0].Owner != "Internal Platform" {
		t.Fatalf("entry owner = %q, want %q", entries[0].Owner, "Internal Platform")
	}
	if got := entries[0].OwnerDetails["name"]; got != "Internal Platform" {
		t.Fatalf("ownerDetails.name = %q, want %q", got, "Internal Platform")
	}
}

func TestProcessResource_OwnershipDetectionEvidenceUsesCanonicalType(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	makeEntry := func(name string, ownerRef bool) MapEntry {
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion("apps/v1")
		obj.SetKind("Deployment")
		obj.SetNamespace("default")
		obj.SetName(name)
		if ownerRef {
			obj.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "app-rs"}})
		}
		entries := processResourceWithLookup(obj, gvr, "local", nil, map[string]int{}, mapApplicationSetLookup{
			byNamespacedName: map[string]int{}, byName: map[string]int{},
		})
		return entries[0]
	}

	knownKubernetes := makeEntry("owned", true)
	if knownKubernetes.Owner != "Native" {
		t.Fatalf("display owner = %q, want legacy Native label", knownKubernetes.Owner)
	}
	if knownKubernetes.OwnershipDetection.Status != "detected" || knownKubernetes.OwnershipDetection.Source != "ownerRef" {
		t.Fatalf("canonical Kubernetes ownership source lost: %+v", knownKubernetes.OwnershipDetection)
	}
	markerFree := makeEntry("bare", false)
	if markerFree.Owner != "Native" || markerFree.OwnershipDetection.Status != "no_known_marker" {
		t.Fatalf("marker-free Native entry should be distinct from known ownerRef: owner=%q evidence=%+v", markerFree.Owner, markerFree.OwnershipDetection)
	}
}

func TestDetectOwnershipHelper_UsesAgentDetectors(t *testing.T) {
	detectors := writeCustomDetectorsFile(t, `
detectors:
  - name: pulumi
    annotations:
      - key: pulumi.com/stack
    owner_name: "Pulumi"
    owner_type: "custom"
`)
	t.Setenv("CUB_SCOUT_OWNERSHIP_DETECTORS", detectors)

	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("apps/v1")
	obj.SetKind("Deployment")
	obj.SetNamespace("default")
	obj.SetName("api")
	obj.SetAnnotations(map[string]string{
		"pulumi.com/stack": "platform/prod",
	})

	owner, managedBy := detectOwnership(obj)
	if owner != "Pulumi" {
		t.Fatalf("owner = %q, want %q", owner, "Pulumi")
	}
	if managedBy != "Pulumi" {
		t.Fatalf("managedBy = %q, want %q", managedBy, "Pulumi")
	}
}

func TestProcessResource_KroOwnershipDisplay(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("apps/v1")
	obj.SetKind("Deployment")
	obj.SetNamespace("default")
	obj.SetName("checkout-api")
	obj.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion: "apps.kro.run/v1alpha1",
			Kind:       "WebApp",
			Name:       "checkout",
		},
	})

	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	entries := processResourceWithLookup(obj, gvr, "local", nil, map[string]int{}, mapApplicationSetLookup{
		byNamespacedName: map[string]int{},
		byName:           map[string]int{},
	})
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(entries))
	}
	if entries[0].Owner != "kro" {
		t.Fatalf("entry owner = %q, want %q", entries[0].Owner, "kro")
	}
	if got := entries[0].OwnerDetails["subType"]; got != "instance" {
		t.Fatalf("ownerDetails.subType = %q, want %q", got, "instance")
	}
}

func writeCustomDetectorsFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "detectors.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write detectors file: %v", err)
	}
	return path
}
