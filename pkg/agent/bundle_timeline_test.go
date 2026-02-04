// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Helper to create a catalog with test bundles
func createTestCatalogWithBundles(t *testing.T, tmpDir string, bundles []testBundleSpec) string {
	t.Helper()

	catalogDir := filepath.Join(tmpDir, "catalog")

	// Initialize catalog
	writer := NewCatalogWriter()
	if err := writer.Init(catalogDir); err != nil {
		t.Fatalf("Failed to init catalog: %v", err)
	}

	// Create and add each bundle
	bundleWriter := NewBundleWriter("test")
	for _, spec := range bundles {
		bundleDir := filepath.Join(tmpDir, "src-"+spec.id)
		bundle := &DebugBundle{
			Metadata: BundleMetadata{
				FormatVersion: BundleFormatVersion,
				Label:         spec.id,
				Target: BundleTarget{
					Kind:      "Deployment",
					Name:      "api",
					Namespace: "prod",
				},
			},
			DriftFindings: spec.driftFindings,
		}

		if spec.hasCorrelation {
			bundle.Session = &DebugSessionData{
				Target: bundle.Metadata.Target,
				RootCause: &RootCauseSnapshot{
					Category: "test",
					Summary:  "test correlation",
				},
			}
		}

		if err := bundleWriter.Write(bundle, bundleDir); err != nil {
			t.Fatalf("Failed to write bundle %s: %v", spec.id, err)
		}

		// Patch the metadata.json to set the desired createdAt
		// (BundleWriter.Write overwrites createdAt with time.Now())
		patchBundleCreatedAt(t, bundleDir, spec.createdAt)

		entry := CatalogEntry{ID: spec.id}
		if spec.sequence != nil {
			entry.Sequence = spec.sequence
		}

		if err := writer.Add(catalogDir, bundleDir, entry); err != nil {
			t.Fatalf("Failed to add bundle %s: %v", spec.id, err)
		}
	}

	return catalogDir
}

// patchBundleCreatedAt overwrites the createdAt in a bundle's metadata.json
func patchBundleCreatedAt(t *testing.T, bundleDir string, createdAt time.Time) {
	t.Helper()

	metadataPath := filepath.Join(bundleDir, "metadata.json")

	// Read existing metadata
	reader := NewBundleReader()
	bundle, err := reader.Read(bundleDir)
	if err != nil {
		t.Fatalf("Failed to read bundle for patching: %v", err)
	}

	// Update createdAt
	bundle.Metadata.CreatedAt = createdAt

	// Write back
	data, err := json.MarshalIndent(bundle.Metadata, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal metadata: %v", err)
	}

	if err := os.WriteFile(metadataPath, data, 0644); err != nil {
		t.Fatalf("Failed to write patched metadata: %v", err)
	}
}

type testBundleSpec struct {
	id             string
	createdAt      time.Time
	sequence       *int
	driftFindings  []DriftFinding
	hasCorrelation bool
}

func intPtr(i int) *int {
	return &i
}

func TestTimelineSchemaVersion(t *testing.T) {
	tmpDir := t.TempDir()

	bundles := []testBundleSpec{
		{id: "bundle-1", createdAt: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)},
	}
	catalogDir := createTestCatalogWithBundles(t, tmpDir, bundles)

	timeline, err := BuildTimeline(catalogDir, OrderManifest, JoinObjectID)
	if err != nil {
		t.Fatalf("BuildTimeline failed: %v", err)
	}

	if timeline.SchemaVersion != BundleTimelineSchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", timeline.SchemaVersion, BundleTimelineSchemaVersion)
	}
}

func TestTimelineSingleBundle(t *testing.T) {
	tmpDir := t.TempDir()

	bundles := []testBundleSpec{
		{
			id:        "snapshot-1",
			createdAt: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
			driftFindings: []DriftFinding{
				{ID: "drift-1", ObjectID: "Deployment:prod/api", Path: "spec.replicas"},
				{ID: "drift-2", ObjectID: "Deployment:prod/api", Path: "spec.image"},
			},
		},
	}
	catalogDir := createTestCatalogWithBundles(t, tmpDir, bundles)

	timeline, err := BuildTimeline(catalogDir, OrderManifest, JoinObjectID)
	if err != nil {
		t.Fatalf("BuildTimeline failed: %v", err)
	}

	// Verify summary
	if timeline.Summary.BundleCount != 1 {
		t.Errorf("BundleCount = %d, want 1", timeline.Summary.BundleCount)
	}
	if timeline.Summary.SeriesCount != 1 {
		t.Errorf("SeriesCount = %d, want 1", timeline.Summary.SeriesCount)
	}
	if timeline.Summary.TotalPoints != 1 {
		t.Errorf("TotalPoints = %d, want 1", timeline.Summary.TotalPoints)
	}
	if timeline.Summary.GapCount != 0 {
		t.Errorf("GapCount = %d, want 0", timeline.Summary.GapCount)
	}

	// Verify series
	if len(timeline.Series) != 1 {
		t.Fatalf("len(Series) = %d, want 1", len(timeline.Series))
	}

	series := timeline.Series[0]
	if series.Identity != "Deployment:prod/api" {
		t.Errorf("Identity = %q, want Deployment:prod/api", series.Identity)
	}

	if len(series.Points) != 1 {
		t.Fatalf("len(Points) = %d, want 1", len(series.Points))
	}

	point := series.Points[0]
	if point.Status != PointPresent {
		t.Errorf("Status = %v, want present", point.Status)
	}
	if point.Sections == nil {
		t.Fatal("Sections is nil")
	}
	if point.Sections.DriftCount != 2 {
		t.Errorf("DriftCount = %d, want 2", point.Sections.DriftCount)
	}
}

func TestTimelineMultipleBundles_Gaps(t *testing.T) {
	tmpDir := t.TempDir()

	// Bundle 1: api object
	// Bundle 2: api and worker objects
	// Bundle 3: worker object only (api disappears)
	bundles := []testBundleSpec{
		{
			id:        "before",
			createdAt: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
			driftFindings: []DriftFinding{
				{ID: "drift-1", ObjectID: "Deployment:prod/api", Path: "spec.replicas"},
			},
		},
		{
			id:        "during",
			createdAt: time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC),
			driftFindings: []DriftFinding{
				{ID: "drift-2", ObjectID: "Deployment:prod/api", Path: "spec.replicas"},
				{ID: "drift-3", ObjectID: "Deployment:prod/worker", Path: "spec.replicas"},
			},
		},
		{
			id:        "after",
			createdAt: time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
			driftFindings: []DriftFinding{
				{ID: "drift-4", ObjectID: "Deployment:prod/worker", Path: "spec.replicas"},
			},
		},
	}
	catalogDir := createTestCatalogWithBundles(t, tmpDir, bundles)

	timeline, err := BuildTimeline(catalogDir, OrderManifest, JoinObjectID)
	if err != nil {
		t.Fatalf("BuildTimeline failed: %v", err)
	}

	// Should have 2 series: api and worker
	if timeline.Summary.SeriesCount != 2 {
		t.Errorf("SeriesCount = %d, want 2", timeline.Summary.SeriesCount)
	}

	// Should have gaps:
	// - api: present, present, missing (1 gap)
	// - worker: missing, present, present (1 gap)
	// Total: 2 gaps
	if timeline.Summary.GapCount != 2 {
		t.Errorf("GapCount = %d, want 2", timeline.Summary.GapCount)
	}

	// Verify series ordering (deterministic by identity)
	if len(timeline.Series) != 2 {
		t.Fatalf("len(Series) = %d, want 2", len(timeline.Series))
	}
	if timeline.Series[0].Identity != "Deployment:prod/api" {
		t.Errorf("Series[0].Identity = %q, want Deployment:prod/api", timeline.Series[0].Identity)
	}
	if timeline.Series[1].Identity != "Deployment:prod/worker" {
		t.Errorf("Series[1].Identity = %q, want Deployment:prod/worker", timeline.Series[1].Identity)
	}

	// Verify api series: present, present, missing
	apiSeries := timeline.Series[0]
	if apiSeries.Points[0].Status != PointPresent {
		t.Errorf("api point 0 status = %v, want present", apiSeries.Points[0].Status)
	}
	if apiSeries.Points[1].Status != PointPresent {
		t.Errorf("api point 1 status = %v, want present", apiSeries.Points[1].Status)
	}
	if apiSeries.Points[2].Status != PointMissing {
		t.Errorf("api point 2 status = %v, want missing", apiSeries.Points[2].Status)
	}

	// Verify worker series: missing, present, present
	workerSeries := timeline.Series[1]
	if workerSeries.Points[0].Status != PointMissing {
		t.Errorf("worker point 0 status = %v, want missing", workerSeries.Points[0].Status)
	}
	if workerSeries.Points[1].Status != PointPresent {
		t.Errorf("worker point 1 status = %v, want present", workerSeries.Points[1].Status)
	}
	if workerSeries.Points[2].Status != PointPresent {
		t.Errorf("worker point 2 status = %v, want present", workerSeries.Points[2].Status)
	}
}

func TestTimelineOrderCreatedAt(t *testing.T) {
	tmpDir := t.TempDir()

	// Add bundles in reverse chronological order to catalog
	bundles := []testBundleSpec{
		{
			id:        "third",
			createdAt: time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
			driftFindings: []DriftFinding{
				{ID: "drift-3", ObjectID: "Deployment:prod/api", Path: "spec.replicas", Desired: 3, Live: 3},
			},
		},
		{
			id:        "first",
			createdAt: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
			driftFindings: []DriftFinding{
				{ID: "drift-1", ObjectID: "Deployment:prod/api", Path: "spec.replicas", Desired: 1, Live: 1},
			},
		},
		{
			id:        "second",
			createdAt: time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC),
			driftFindings: []DriftFinding{
				{ID: "drift-2", ObjectID: "Deployment:prod/api", Path: "spec.replicas", Desired: 2, Live: 2},
			},
		},
	}
	catalogDir := createTestCatalogWithBundles(t, tmpDir, bundles)

	// With manifest order, should be: third, first, second (catalog order)
	manifestTimeline, _ := BuildTimeline(catalogDir, OrderManifest, JoinObjectID)
	if manifestTimeline.Series[0].Points[0].BundleID != "third" {
		t.Errorf("Manifest order: first point bundle = %q, want third", manifestTimeline.Series[0].Points[0].BundleID)
	}

	// With created_at order, should be: first, second, third (chronological)
	createdAtTimeline, _ := BuildTimeline(catalogDir, OrderCreatedAt, JoinObjectID)
	if createdAtTimeline.Series[0].Points[0].BundleID != "first" {
		t.Errorf("CreatedAt order: first point bundle = %q, want first", createdAtTimeline.Series[0].Points[0].BundleID)
	}
	if createdAtTimeline.Series[0].Points[1].BundleID != "second" {
		t.Errorf("CreatedAt order: second point bundle = %q, want second", createdAtTimeline.Series[0].Points[1].BundleID)
	}
	if createdAtTimeline.Series[0].Points[2].BundleID != "third" {
		t.Errorf("CreatedAt order: third point bundle = %q, want third", createdAtTimeline.Series[0].Points[2].BundleID)
	}
}

func TestTimelineOrderSequence(t *testing.T) {
	tmpDir := t.TempDir()

	// Add bundles with explicit sequence
	bundles := []testBundleSpec{
		{
			id:        "seq-3",
			createdAt: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
			sequence:  intPtr(3),
			driftFindings: []DriftFinding{
				{ID: "drift-3", ObjectID: "Deployment:prod/api", Path: "spec.replicas"},
			},
		},
		{
			id:        "seq-1",
			createdAt: time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC),
			sequence:  intPtr(1),
			driftFindings: []DriftFinding{
				{ID: "drift-1", ObjectID: "Deployment:prod/api", Path: "spec.replicas"},
			},
		},
		{
			id:        "seq-2",
			createdAt: time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
			sequence:  intPtr(2),
			driftFindings: []DriftFinding{
				{ID: "drift-2", ObjectID: "Deployment:prod/api", Path: "spec.replicas"},
			},
		},
	}
	catalogDir := createTestCatalogWithBundles(t, tmpDir, bundles)

	// With sequence order, should be: seq-1, seq-2, seq-3
	timeline, _ := BuildTimeline(catalogDir, OrderSequence, JoinObjectID)

	if timeline.Series[0].Points[0].BundleID != "seq-1" {
		t.Errorf("Sequence order: first point bundle = %q, want seq-1", timeline.Series[0].Points[0].BundleID)
	}
	if timeline.Series[0].Points[1].BundleID != "seq-2" {
		t.Errorf("Sequence order: second point bundle = %q, want seq-2", timeline.Series[0].Points[1].BundleID)
	}
	if timeline.Series[0].Points[2].BundleID != "seq-3" {
		t.Errorf("Sequence order: third point bundle = %q, want seq-3", timeline.Series[0].Points[2].BundleID)
	}
}

func TestTimelineDeterministicSeriesOrder(t *testing.T) {
	tmpDir := t.TempDir()

	bundles := []testBundleSpec{
		{
			id:        "snapshot",
			createdAt: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
			driftFindings: []DriftFinding{
				{ID: "drift-z", ObjectID: "Deployment:prod/zebra", Path: "spec.replicas"},
				{ID: "drift-a", ObjectID: "Deployment:prod/alpha", Path: "spec.replicas"},
				{ID: "drift-m", ObjectID: "Deployment:prod/middle", Path: "spec.replicas"},
			},
		},
	}
	catalogDir := createTestCatalogWithBundles(t, tmpDir, bundles)

	// Build multiple times and verify order is deterministic
	for i := 0; i < 5; i++ {
		timeline, _ := BuildTimeline(catalogDir, OrderManifest, JoinObjectID)

		expectedOrder := []string{
			"Deployment:prod/alpha",
			"Deployment:prod/middle",
			"Deployment:prod/zebra",
		}

		if len(timeline.Series) != len(expectedOrder) {
			t.Fatalf("Iteration %d: len(Series) = %d, want %d", i, len(timeline.Series), len(expectedOrder))
		}

		for j, series := range timeline.Series {
			if series.Identity != expectedOrder[j] {
				t.Errorf("Iteration %d: Series[%d].Identity = %q, want %q", i, j, series.Identity, expectedOrder[j])
			}
		}
	}
}

func TestTimelineWithCorrelation(t *testing.T) {
	tmpDir := t.TempDir()

	bundles := []testBundleSpec{
		{
			id:             "with-corr",
			createdAt:      time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
			hasCorrelation: true,
		},
	}
	catalogDir := createTestCatalogWithBundles(t, tmpDir, bundles)

	timeline, err := BuildTimeline(catalogDir, OrderManifest, JoinObjectID)
	if err != nil {
		t.Fatalf("BuildTimeline failed: %v", err)
	}

	if len(timeline.Series) != 1 {
		t.Fatalf("len(Series) = %d, want 1", len(timeline.Series))
	}

	point := timeline.Series[0].Points[0]
	if point.Sections == nil {
		t.Fatal("Sections is nil")
	}
	if !point.Sections.HasCorrelation {
		t.Error("HasCorrelation should be true")
	}
}

func TestTimelineJoinCompositeNotImplemented(t *testing.T) {
	tmpDir := t.TempDir()
	bundles := []testBundleSpec{
		{id: "test", createdAt: time.Now()},
	}
	catalogDir := createTestCatalogWithBundles(t, tmpDir, bundles)

	_, err := BuildTimeline(catalogDir, OrderManifest, JoinComposite)
	if err == nil {
		t.Fatal("Expected error for composite join")
	}

	expected := "join mode 'composite' not yet implemented"
	if err.Error() != expected {
		t.Errorf("Error = %q, want %q", err.Error(), expected)
	}
}

func TestTimelineJoinNoneNotImplemented(t *testing.T) {
	tmpDir := t.TempDir()
	bundles := []testBundleSpec{
		{id: "test", createdAt: time.Now()},
	}
	catalogDir := createTestCatalogWithBundles(t, tmpDir, bundles)

	_, err := BuildTimeline(catalogDir, OrderManifest, JoinNone)
	if err == nil {
		t.Fatal("Expected error for none join")
	}

	expected := "join mode 'none' not yet implemented"
	if err.Error() != expected {
		t.Errorf("Error = %q, want %q", err.Error(), expected)
	}
}

func TestTimelineCatalogRef(t *testing.T) {
	tmpDir := t.TempDir()

	bundles := []testBundleSpec{
		{id: "b1", createdAt: time.Now()},
		{id: "b2", createdAt: time.Now()},
	}
	catalogDir := createTestCatalogWithBundles(t, tmpDir, bundles)

	timeline, _ := BuildTimeline(catalogDir, OrderManifest, JoinObjectID)

	if timeline.CatalogRef.Path != catalogDir {
		t.Errorf("CatalogRef.Path = %q, want %q", timeline.CatalogRef.Path, catalogDir)
	}
	if timeline.CatalogRef.BundleCount != 2 {
		t.Errorf("CatalogRef.BundleCount = %d, want 2", timeline.CatalogRef.BundleCount)
	}
	if timeline.CatalogRef.SchemaVersion != CatalogSchemaVersion {
		t.Errorf("CatalogRef.SchemaVersion = %q, want %q", timeline.CatalogRef.SchemaVersion, CatalogSchemaVersion)
	}
}

func TestTimelineOrdering(t *testing.T) {
	tmpDir := t.TempDir()

	bundles := []testBundleSpec{
		{id: "test", createdAt: time.Now()},
	}
	catalogDir := createTestCatalogWithBundles(t, tmpDir, bundles)

	timeline, _ := BuildTimeline(catalogDir, OrderCreatedAt, JoinObjectID)

	if timeline.Ordering.Mode != OrderCreatedAt {
		t.Errorf("Ordering.Mode = %v, want created_at", timeline.Ordering.Mode)
	}
	if timeline.Ordering.TieBreak != "id" {
		t.Errorf("Ordering.TieBreak = %q, want id", timeline.Ordering.TieBreak)
	}
}
