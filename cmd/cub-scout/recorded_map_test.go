// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func loadRecordedMapTestSnapshot(t *testing.T, raw []byte) recordedObjectSnapshot {
	t.Helper()
	snapshot, err := loadRecordedObjectSnapshot(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("load recorded snapshot: %v", err)
	}
	return snapshot
}

func scaleRecordedMapScope() RecordedMapScope {
	return RecordedMapScope{APIVersion: "apps/v1", Kind: "Deployment", NamespacePrefix: "team-"}
}

func TestBuildRecordedMapReportPinnedScaleDeployments(t *testing.T) {
	path := filepath.Join("..", "..", "evals", "fixtures", "scale", "cluster", "deployments.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := loadRecordedMapTestSnapshot(t, raw)
	got, err := buildRecordedMapReport(snapshot, scaleRecordedMapScope())
	if err != nil {
		t.Fatal(err)
	}

	if got.Schema != "map-list-recorded.v1" || got.SelectedCount != 300 || got.ExcludedFromScope != 2 {
		t.Fatalf("schema/scope counts = %q/%d/%d, want map-list-recorded.v1/300/2", got.Schema, got.SelectedCount, got.ExcludedFromScope)
	}
	wantOwners := map[string]int{"Flux": 120, "ArgoCD": 90, "Helm": 45, "ConfigHub": 33, "Native": 12}
	if !reflect.DeepEqual(got.OwnerCounts, wantOwners) {
		t.Fatalf("owner counts = %#v, want %#v", got.OwnerCounts, wantOwners)
	}
	if got.Provenance.SHA256 != snapshot.Provenance.SHA256 || got.Provenance.Bytes != len(raw) ||
		got.Provenance.Documents != 1 || got.Provenance.ObjectCount != 302 ||
		got.Provenance.CaptureTime != "unknown" || got.Provenance.CaptureCompleteness != "unknown" {
		t.Fatalf("provenance = %#v", got.Provenance)
	}
	if got.Scope.APIVersion != "apps/v1" || got.Scope.Kind != "Deployment" || got.Scope.Namespace != nil || got.Scope.NamespacePrefix != "team-" {
		t.Fatalf("scope = %#v", got.Scope)
	}

	wantNative := []string{
		"team-02/auth", "team-03/auth", "team-05/auth", "team-05/cron", "team-05/notify",
		"team-11/api", "team-11/notify", "team-12/web", "team-18/web", "team-19/cron",
		"team-25/search", "team-30/cache",
	}
	var gotNative []string
	for i, resource := range got.Resources {
		if resource.APIVersion != "apps/v1" || resource.Kind != "Deployment" || !strings.HasPrefix(resource.Namespace, "team-") {
			t.Fatalf("resource outside requested scope: %#v", resource)
		}
		if i > 0 && recordedMapResourceLess(resource, got.Resources[i-1]) {
			t.Fatalf("resources are not sorted at index %d: %#v before %#v", i, got.Resources[i-1], resource)
		}
		if resource.Owner == "Native" {
			if resource.OwnershipDetection == nil || resource.OwnershipDetection.Status != "no_known_marker" {
				t.Fatalf("Native resource lacks no-marker evidence: %#v", resource)
			}
			gotNative = append(gotNative, resource.Namespace+"/"+resource.Name)
		}
	}
	if !reflect.DeepEqual(gotNative, wantNative) {
		t.Fatalf("Native/no-marker identities = %#v, want %#v", gotNative, wantNative)
	}

	manifestPath := filepath.Join("..", "..", "evals", "fixtures", "scale", "recording-2026-09-30.json")
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Files map[string]string `json:"files"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if want := manifest.Files["evals/fixtures/scale/cluster/deployments.yaml"]; want == "" || got.Provenance.SHA256 != want {
		t.Fatalf("input SHA256 = %q, recording manifest has %q", got.Provenance.SHA256, want)
	}

	wire, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"health", "omissions", "clusterName", "observation", "capturedAt"} {
		if bytes.Contains(wire, []byte(`"`+forbidden+`"`)) {
			t.Fatalf("recorded report includes unsupported field %q: %s", forbidden, wire)
		}
	}
}

func TestBuildRecordedMapReportRejectsDuplicateOutsideScope(t *testing.T) {
	const raw = `apiVersion: v1
kind: List
items:
- apiVersion: apps/v1
  kind: Deployment
  metadata: {name: api, namespace: outside}
- apiVersion: apps/v1
  kind: Deployment
  metadata: {name: api, namespace: outside}
`
	snapshot := loadRecordedMapTestSnapshot(t, []byte(raw))
	_, err := buildRecordedMapReport(snapshot, scaleRecordedMapScope())
	if err == nil || !strings.Contains(err.Error(), "duplicate full identity") {
		t.Fatalf("duplicate outside requested prefix must fail before filtering, got %v", err)
	}
}

func TestBuildRecordedMapReportScopeValidationAndEmptySelection(t *testing.T) {
	snapshot := loadRecordedMapTestSnapshot(t, []byte(`apiVersion: v1
kind: List
items:
- apiVersion: apps/v1
  kind: Deployment
  metadata: {name: api, namespace: team-a}
`))
	namespace := "team-a"
	if _, err := buildRecordedMapReport(snapshot, RecordedMapScope{APIVersion: "apps/v1", Kind: "Deployment", Namespace: &namespace, NamespacePrefix: "team-"}); err == nil {
		t.Fatal("scope accepted exact namespace together with a prefix")
	}
	emptyNamespace := ""
	got, err := buildRecordedMapReport(snapshot, RecordedMapScope{APIVersion: "apps/v1", Kind: "Deployment", Namespace: &emptyNamespace})
	if err != nil {
		t.Fatal(err)
	}
	if got.SelectedCount != 0 || got.ExcludedFromScope != 1 || len(got.Resources) != 0 || len(got.OwnerCounts) != 0 {
		t.Fatalf("empty exact-namespace selection = %#v", got)
	}
	if got.Scope.Namespace == nil || *got.Scope.Namespace != "" {
		t.Fatalf("empty exact namespace was not retained: %#v", got.Scope)
	}
	*got.Scope.Namespace = "changed"
	if namespace != "team-a" || emptyNamespace != "" {
		t.Fatal("report scope shares mutable namespace pointers with callers")
	}
}

func TestBuildRecordedMapReportUsesBuiltinDetectionOnly(t *testing.T) {
	const raw = `apiVersion: v1
kind: List
items:
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: api
    namespace: team-a
    labels: {platform.example/managed-by: local-controller}
`
	snapshot := loadRecordedMapTestSnapshot(t, []byte(raw))
	scope := scaleRecordedMapScope()
	scope.NamespacePrefix = "team-a"
	withoutConfig, err := buildRecordedMapReport(snapshot, scope)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "detectors.yaml")
	if err := os.WriteFile(path, []byte(`detectors:
- name: local-owner
  labels:
  - key: platform.example/managed-by
    value: local-controller
  owner_name: Ambient Custom Owner
  owner_type: custom
`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CUB_SCOUT_OWNERSHIP_DETECTORS", path)
	withConfig, err := buildRecordedMapReport(snapshot, scope)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(withoutConfig, withConfig) || withConfig.Resources[0].Owner != "Native" {
		t.Fatalf("ambient detector changed builtin-only result: before=%#v after=%#v", withoutConfig, withConfig)
	}
}

func TestBuildRecordedMapReportHashAndOrderingAreDeterministic(t *testing.T) {
	const raw = `apiVersion: v1
kind: List
items:
- apiVersion: apps/v1
  kind: Deployment
  metadata: {name: z, namespace: team-z}
- apiVersion: apps/v1
  kind: Deployment
  metadata: {name: a, namespace: team-a}
`
	scope := scaleRecordedMapScope()
	snapshot := loadRecordedMapTestSnapshot(t, []byte(raw))
	first, err := buildRecordedMapReport(snapshot, scope)
	if err != nil {
		t.Fatal(err)
	}
	second, err := buildRecordedMapReport(snapshot, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Resources) != 2 || first.Resources[0].Namespace != "team-a" || first.Resources[1].Namespace != "team-z" {
		t.Fatalf("resource order = %#v", first.Resources)
	}
	first.Resources[0].Owner = "mutated"
	if second.Resources[0].Owner != "Native" {
		t.Fatalf("separate report shares resource data: %#v", second.Resources[0])
	}
	changed := loadRecordedMapTestSnapshot(t, append([]byte("# different source bytes\n"), []byte(raw)...))
	third, err := buildRecordedMapReport(changed, scope)
	if err != nil {
		t.Fatal(err)
	}
	if third.Provenance.SHA256 == first.Provenance.SHA256 || third.Provenance.Bytes == first.Provenance.Bytes {
		t.Fatalf("changed source bytes were not reflected in provenance: first=%#v third=%#v", first.Provenance, third.Provenance)
	}
}

func TestBuildRecordedMapReportRejectsMalformedAndEmptySnapshots(t *testing.T) {
	for _, raw := range [][]byte{nil, []byte("apiVersion: apps/v1\nkind: Deployment\nmetadata: ["), []byte("# no objects\n")} {
		if _, err := loadRecordedObjectSnapshot(bytes.NewReader(raw)); err == nil {
			t.Fatalf("loader accepted malformed/empty recording %q", raw)
		}
	}
	if _, err := buildRecordedMapReport(recordedObjectSnapshot{}, scaleRecordedMapScope()); err == nil {
		t.Fatal("model accepted empty snapshot")
	}
}

func TestBuildRecordedMapReportDistinguishesKubernetesOwnerReferenceFromNative(t *testing.T) {
	const raw = `apiVersion: v1
kind: List
items:
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: api
    namespace: team-a
    ownerReferences:
    - apiVersion: apps/v1
      kind: ReplicaSet
      name: api-123
      uid: recorded-uid
`
	snapshot := loadRecordedMapTestSnapshot(t, []byte(raw))
	scope := scaleRecordedMapScope()
	scope.NamespacePrefix = "team-a"
	got, err := buildRecordedMapReport(snapshot, scope)
	if err != nil {
		t.Fatal(err)
	}
	if got.Resources[0].Owner != "Kubernetes" || got.Resources[0].OwnershipDetection.Status != "detected" {
		t.Fatalf("built-in owner reference was presented as Native: %#v", got.Resources[0])
	}
}

func TestBuildRecordedMapReportDoesNotMutateSnapshotObjects(t *testing.T) {
	const raw = `apiVersion: v1
kind: List
items:
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: api
    namespace: team-a
    labels: {app: api}
`
	snapshot := loadRecordedMapTestSnapshot(t, []byte(raw))
	before := snapshot.Objects[0].DeepCopy()
	if _, err := buildRecordedMapReport(snapshot, scaleRecordedMapScope()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot.Objects[0].Object, before.Object) {
		t.Fatal("recorded map projection mutated the immutable snapshot")
	}
}

func recordedMapResourceLess(a, b RecordedMapResource) bool {
	if a.APIVersion != b.APIVersion {
		return a.APIVersion < b.APIVersion
	}
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if a.Namespace != b.Namespace {
		return a.Namespace < b.Namespace
	}
	return a.Name < b.Name
}
