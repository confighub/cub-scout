// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"
	"time"
)

// Helper to create a test bundle with drift findings
func makeTestBundle(targetName string, createdAt time.Time, driftIDs ...string) *DebugBundle {
	bundle := &DebugBundle{
		Metadata: BundleMetadata{
			FormatVersion:   BundleFormatVersion,
			CubScoutVersion: "test",
			CreatedAt:       createdAt,
			Target: BundleTarget{
				Kind:      "Deployment",
				Name:      targetName,
				Namespace: "default",
			},
		},
	}

	for _, id := range driftIDs {
		bundle.DriftFindings = append(bundle.DriftFindings, DriftFinding{
			ID:       id,
			ObjectID: "Deployment:default/" + targetName,
			Object: DriftObjectID{
				Kind:      "Deployment",
				Name:      targetName,
				Namespace: "default",
			},
			Path:           "spec.replicas",
			Desired:        3,
			Live:           2,
			Classification: DriftCapacity,
			Severity:       DriftSeverityWarning,
		})
	}

	return bundle
}

func TestDiffSchemaVersion(t *testing.T) {
	t1 := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC)

	bundleA := makeTestBundle("api", t1)
	bundleB := makeTestBundle("api", t2)

	diff, err := DiffBundles(bundleA, bundleB, JoinObjectID)
	if err != nil {
		t.Fatalf("DiffBundles failed: %v", err)
	}

	if diff.SchemaVersion != BundleDiffSchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", diff.SchemaVersion, BundleDiffSchemaVersion)
	}
}

func TestDiffAddedRemoved(t *testing.T) {
	t1 := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC)

	// Bundle A has obj-1, obj-2
	bundleA := &DebugBundle{
		Metadata: BundleMetadata{
			FormatVersion: BundleFormatVersion,
			CreatedAt:     t1,
			Target:        BundleTarget{Kind: "Deployment", Name: "api", Namespace: "default"},
		},
		DriftFindings: []DriftFinding{
			{ID: "drift-1", ObjectID: "Deployment:default/api", Object: DriftObjectID{Kind: "Deployment", Name: "api", Namespace: "default"}, Path: "spec.replicas"},
			{ID: "drift-2", ObjectID: "Deployment:default/worker", Object: DriftObjectID{Kind: "Deployment", Name: "worker", Namespace: "default"}, Path: "spec.replicas"},
		},
	}

	// Bundle B has obj-1, obj-3 (obj-2 removed, obj-3 added)
	bundleB := &DebugBundle{
		Metadata: BundleMetadata{
			FormatVersion: BundleFormatVersion,
			CreatedAt:     t2,
			Target:        BundleTarget{Kind: "Deployment", Name: "api", Namespace: "default"},
		},
		DriftFindings: []DriftFinding{
			{ID: "drift-1", ObjectID: "Deployment:default/api", Object: DriftObjectID{Kind: "Deployment", Name: "api", Namespace: "default"}, Path: "spec.replicas"},
			{ID: "drift-3", ObjectID: "Deployment:default/frontend", Object: DriftObjectID{Kind: "Deployment", Name: "frontend", Namespace: "default"}, Path: "spec.image"},
		},
	}

	diff, err := DiffBundles(bundleA, bundleB, JoinObjectID)
	if err != nil {
		t.Fatalf("DiffBundles failed: %v", err)
	}

	// Check summary counts
	if diff.Summary.Added != 1 {
		t.Errorf("Summary.Added = %d, want 1", diff.Summary.Added)
	}
	if diff.Summary.Removed != 1 {
		t.Errorf("Summary.Removed = %d, want 1", diff.Summary.Removed)
	}
	if diff.Summary.Unchanged != 1 {
		t.Errorf("Summary.Unchanged = %d, want 1", diff.Summary.Unchanged)
	}

	// Verify objects are sorted by identity
	if len(diff.Objects) != 3 {
		t.Fatalf("len(Objects) = %d, want 3", len(diff.Objects))
	}

	// Objects should be: api (unchanged), frontend (added), worker (removed)
	expectedOrder := []string{"Deployment:default/api", "Deployment:default/frontend", "Deployment:default/worker"}
	for i, obj := range diff.Objects {
		if obj.Identity != expectedOrder[i] {
			t.Errorf("Objects[%d].Identity = %q, want %q", i, obj.Identity, expectedOrder[i])
		}
	}

	// Check statuses
	statusMap := make(map[string]ObjectStatus)
	for _, obj := range diff.Objects {
		statusMap[obj.Identity] = obj.Status
	}

	if statusMap["Deployment:default/api"] != StatusUnchanged {
		t.Errorf("api status = %v, want unchanged", statusMap["Deployment:default/api"])
	}
	if statusMap["Deployment:default/frontend"] != StatusAdded {
		t.Errorf("frontend status = %v, want added", statusMap["Deployment:default/frontend"])
	}
	if statusMap["Deployment:default/worker"] != StatusRemoved {
		t.Errorf("worker status = %v, want removed", statusMap["Deployment:default/worker"])
	}
}

func TestDiffUnchangedWhenSame(t *testing.T) {
	t1 := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC)

	// Create identical bundles (except timestamp)
	bundleA := makeTestBundle("api", t1, "drift-1", "drift-2")
	bundleB := makeTestBundle("api", t2, "drift-1", "drift-2")

	diff, err := DiffBundles(bundleA, bundleB, JoinObjectID)
	if err != nil {
		t.Fatalf("DiffBundles failed: %v", err)
	}

	if diff.Summary.Unchanged != 1 {
		t.Errorf("Summary.Unchanged = %d, want 1", diff.Summary.Unchanged)
	}
	if diff.Summary.Changed != 0 {
		t.Errorf("Summary.Changed = %d, want 0", diff.Summary.Changed)
	}
}

func TestDiffChangedWhenDriftDiffers(t *testing.T) {
	t1 := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC)

	// Bundle A has 2 drift findings
	bundleA := makeTestBundle("api", t1, "drift-1", "drift-2")

	// Bundle B has 3 drift findings (one more)
	bundleB := makeTestBundle("api", t2, "drift-1", "drift-2", "drift-3")

	diff, err := DiffBundles(bundleA, bundleB, JoinObjectID)
	if err != nil {
		t.Fatalf("DiffBundles failed: %v", err)
	}

	if diff.Summary.Changed != 1 {
		t.Errorf("Summary.Changed = %d, want 1", diff.Summary.Changed)
	}

	// Check drift section shows changed
	if len(diff.Objects) == 0 {
		t.Fatal("No objects in diff")
	}

	obj := diff.Objects[0]
	if obj.Status != StatusChanged {
		t.Errorf("Object status = %v, want changed", obj.Status)
	}

	// Find drift section
	var driftSection *SectionDiff
	for i := range obj.Sections {
		if obj.Sections[i].Name == "drift" {
			driftSection = &obj.Sections[i]
			break
		}
	}

	if driftSection == nil {
		t.Fatal("Drift section not found")
	}

	if driftSection.Status != SectionChanged {
		t.Errorf("Drift section status = %v, want changed", driftSection.Status)
	}

	if driftSection.Counts == nil {
		t.Fatal("Drift section counts is nil")
	}

	if driftSection.Counts.Before != 2 {
		t.Errorf("Drift counts.Before = %d, want 2", driftSection.Counts.Before)
	}
	if driftSection.Counts.After != 3 {
		t.Errorf("Drift counts.After = %d, want 3", driftSection.Counts.After)
	}
	if driftSection.Counts.Delta != 1 {
		t.Errorf("Drift counts.Delta = %d, want 1", driftSection.Counts.Delta)
	}
}

func TestDiffChangedWhenCorrelationDiffers(t *testing.T) {
	t1 := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC)

	// Bundle A with correlation
	bundleA := makeTestBundle("api", t1)
	bundleA.Session = &DebugSessionData{
		Target: bundleA.Metadata.Target,
		RootCause: &RootCauseSnapshot{
			Category:   "deployment",
			Stage:      "rollout",
			Confidence: "high",
			Summary:    "Rollout stuck",
		},
	}

	// Bundle B with different correlation
	bundleB := makeTestBundle("api", t2)
	bundleB.Session = &DebugSessionData{
		Target: bundleB.Metadata.Target,
		RootCause: &RootCauseSnapshot{
			Category:   "deployment",
			Stage:      "rollout",
			Confidence: "high",
			Summary:    "Rollout completed", // Different!
		},
	}

	diff, err := DiffBundles(bundleA, bundleB, JoinObjectID)
	if err != nil {
		t.Fatalf("DiffBundles failed: %v", err)
	}

	if diff.Summary.Changed != 1 {
		t.Errorf("Summary.Changed = %d, want 1", diff.Summary.Changed)
	}

	// Find correlation section
	obj := diff.Objects[0]
	var corrSection *SectionDiff
	for i := range obj.Sections {
		if obj.Sections[i].Name == "correlation" {
			corrSection = &obj.Sections[i]
			break
		}
	}

	if corrSection == nil {
		t.Fatal("Correlation section not found")
	}

	if corrSection.Status != SectionChanged {
		t.Errorf("Correlation section status = %v, want changed", corrSection.Status)
	}
}

func TestDiffDeterministicObjectOrdering(t *testing.T) {
	t1 := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC)

	// Create bundles with objects in random order
	bundleA := &DebugBundle{
		Metadata: BundleMetadata{
			FormatVersion: BundleFormatVersion,
			CreatedAt:     t1,
			Target:        BundleTarget{Kind: "Deployment", Name: "api", Namespace: "default"},
		},
		DriftFindings: []DriftFinding{
			{ID: "drift-z", ObjectID: "Deployment:default/zebra", Object: DriftObjectID{Kind: "Deployment", Name: "zebra", Namespace: "default"}},
			{ID: "drift-a", ObjectID: "Deployment:default/alpha", Object: DriftObjectID{Kind: "Deployment", Name: "alpha", Namespace: "default"}},
			{ID: "drift-m", ObjectID: "Deployment:default/middle", Object: DriftObjectID{Kind: "Deployment", Name: "middle", Namespace: "default"}},
		},
	}

	bundleB := &DebugBundle{
		Metadata: BundleMetadata{
			FormatVersion: BundleFormatVersion,
			CreatedAt:     t2,
			Target:        BundleTarget{Kind: "Deployment", Name: "api", Namespace: "default"},
		},
		DriftFindings: []DriftFinding{
			{ID: "drift-z", ObjectID: "Deployment:default/zebra", Object: DriftObjectID{Kind: "Deployment", Name: "zebra", Namespace: "default"}},
			{ID: "drift-a", ObjectID: "Deployment:default/alpha", Object: DriftObjectID{Kind: "Deployment", Name: "alpha", Namespace: "default"}},
			{ID: "drift-m", ObjectID: "Deployment:default/middle", Object: DriftObjectID{Kind: "Deployment", Name: "middle", Namespace: "default"}},
		},
	}

	// Run diff multiple times and verify order is always the same
	for i := 0; i < 5; i++ {
		diff, err := DiffBundles(bundleA, bundleB, JoinObjectID)
		if err != nil {
			t.Fatalf("DiffBundles failed: %v", err)
		}

		expectedOrder := []string{
			"Deployment:default/alpha",
			"Deployment:default/middle",
			"Deployment:default/zebra",
		}

		if len(diff.Objects) != len(expectedOrder) {
			t.Fatalf("len(Objects) = %d, want %d", len(diff.Objects), len(expectedOrder))
		}

		for j, obj := range diff.Objects {
			if obj.Identity != expectedOrder[j] {
				t.Errorf("Iteration %d: Objects[%d].Identity = %q, want %q", i, j, obj.Identity, expectedOrder[j])
			}
		}
	}
}

func TestDiffDeterministicSectionOrdering(t *testing.T) {
	t1 := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC)

	bundleA := makeTestBundle("api", t1, "drift-1")
	bundleB := makeTestBundle("api", t2, "drift-1")

	diff, err := DiffBundles(bundleA, bundleB, JoinObjectID)
	if err != nil {
		t.Fatalf("DiffBundles failed: %v", err)
	}

	if len(diff.Objects) == 0 {
		t.Fatal("No objects in diff")
	}

	obj := diff.Objects[0]

	// Sections must always be in fixed order: drift, correlation
	expectedSectionOrder := []string{"drift", "correlation"}

	if len(obj.Sections) != len(expectedSectionOrder) {
		t.Fatalf("len(Sections) = %d, want %d", len(obj.Sections), len(expectedSectionOrder))
	}

	for i, section := range obj.Sections {
		if section.Name != expectedSectionOrder[i] {
			t.Errorf("Sections[%d].Name = %q, want %q", i, section.Name, expectedSectionOrder[i])
		}
	}
}

func TestDiffJoinCompositeNotImplemented(t *testing.T) {
	t1 := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	bundleA := makeTestBundle("api", t1)
	bundleB := makeTestBundle("api", t1)

	_, err := DiffBundles(bundleA, bundleB, JoinComposite)
	if err == nil {
		t.Fatal("Expected error for composite join, got nil")
	}

	expected := "join mode 'composite' not yet implemented"
	if err.Error() != expected {
		t.Errorf("Error = %q, want %q", err.Error(), expected)
	}
}

func TestDiffJoinNoneNotImplemented(t *testing.T) {
	t1 := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	bundleA := makeTestBundle("api", t1)
	bundleB := makeTestBundle("api", t1)

	_, err := DiffBundles(bundleA, bundleB, JoinNone)
	if err == nil {
		t.Fatal("Expected error for none join, got nil")
	}

	expected := "join mode 'none' not yet implemented"
	if err.Error() != expected {
		t.Errorf("Error = %q, want %q", err.Error(), expected)
	}
}
