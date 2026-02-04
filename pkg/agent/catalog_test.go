// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCatalogWriter_Init(t *testing.T) {
	tmpDir := t.TempDir()
	catalogDir := filepath.Join(tmpDir, "test-catalog")

	writer := NewCatalogWriter()
	if err := writer.Init(catalogDir); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Verify catalog.json exists
	catalogPath := filepath.Join(catalogDir, "catalog.json")
	if _, err := os.Stat(catalogPath); os.IsNotExist(err) {
		t.Error("catalog.json should exist")
	}

	// Verify bundles directory exists
	bundlesDir := filepath.Join(catalogDir, "bundles")
	if _, err := os.Stat(bundlesDir); os.IsNotExist(err) {
		t.Error("bundles directory should exist")
	}

	// Read and verify catalog contents
	reader := NewCatalogReader()
	catalog, err := reader.Read(catalogDir)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if catalog.SchemaVersion != CatalogSchemaVersion {
		t.Errorf("SchemaVersion = %s, want %s", catalog.SchemaVersion, CatalogSchemaVersion)
	}
	if len(catalog.Bundles) != 0 {
		t.Errorf("Bundles should be empty, got %d", len(catalog.Bundles))
	}
}

func TestCatalogWriter_Add(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a catalog
	catalogDir := filepath.Join(tmpDir, "catalog")
	catalogWriter := NewCatalogWriter()
	if err := catalogWriter.Init(catalogDir); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Create a test bundle
	bundleDir := filepath.Join(tmpDir, "test-bundle")
	bundleWriter := NewBundleWriter("v0.15.0-test")
	bundle := &DebugBundle{
		Metadata: BundleMetadata{
			Target: BundleTarget{
				Kind:      "Deployment",
				Name:      "api",
				Namespace: "prod",
			},
		},
		DriftFindings: []DriftFinding{
			{ID: "drift:1", Path: "spec.replicas"},
		},
	}
	if err := bundleWriter.Write(bundle, bundleDir); err != nil {
		t.Fatalf("Write bundle failed: %v", err)
	}

	// Add bundle to catalog
	entry := CatalogEntry{
		ID: "test-bundle-1",
		Labels: map[string]string{
			"source": "test",
		},
	}
	if err := catalogWriter.Add(catalogDir, bundleDir, entry); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Read catalog and verify
	reader := NewCatalogReader()
	catalog, err := reader.Read(catalogDir)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if len(catalog.Bundles) != 1 {
		t.Fatalf("Expected 1 bundle, got %d", len(catalog.Bundles))
	}

	addedEntry := catalog.Bundles[0]
	if addedEntry.ID != "test-bundle-1" {
		t.Errorf("ID = %s, want test-bundle-1", addedEntry.ID)
	}
	if addedEntry.Path != "bundles/test-bundle-1" {
		t.Errorf("Path = %s, want bundles/test-bundle-1", addedEntry.Path)
	}
	if addedEntry.Labels["source"] != "test" {
		t.Errorf("Labels[source] = %s, want test", addedEntry.Labels["source"])
	}

	// Verify bundle was copied
	copiedPath := filepath.Join(catalogDir, addedEntry.Path)
	if _, err := os.Stat(copiedPath); os.IsNotExist(err) {
		t.Error("Bundle should be copied to catalog directory")
	}
}

func TestCatalogWriter_Add_DuplicateID(t *testing.T) {
	tmpDir := t.TempDir()

	// Create catalog and bundle
	catalogDir := filepath.Join(tmpDir, "catalog")
	catalogWriter := NewCatalogWriter()
	catalogWriter.Init(catalogDir)

	bundleDir := filepath.Join(tmpDir, "bundle")
	bundleWriter := NewBundleWriter("v0.15.0-test")
	bundle := &DebugBundle{
		Metadata: BundleMetadata{
			Target: BundleTarget{Kind: "Deployment", Name: "api"},
		},
	}
	bundleWriter.Write(bundle, bundleDir)

	// Add first bundle
	entry1 := CatalogEntry{ID: "same-id"}
	if err := catalogWriter.Add(catalogDir, bundleDir, entry1); err != nil {
		t.Fatalf("First add failed: %v", err)
	}

	// Create another bundle directory for second add
	bundleDir2 := filepath.Join(tmpDir, "bundle2")
	bundleWriter.Write(bundle, bundleDir2)

	// Try to add with same ID - should fail
	entry2 := CatalogEntry{ID: "same-id"}
	err := catalogWriter.Add(catalogDir, bundleDir2, entry2)
	if err == nil {
		t.Error("Expected error for duplicate ID")
	}
}

func TestCatalogReader_Validate(t *testing.T) {
	tmpDir := t.TempDir()

	// Create catalog with a bundle
	catalogDir := filepath.Join(tmpDir, "catalog")
	catalogWriter := NewCatalogWriter()
	catalogWriter.Init(catalogDir)

	bundleDir := filepath.Join(tmpDir, "bundle")
	bundleWriter := NewBundleWriter("v0.15.0-test")
	bundle := &DebugBundle{
		Metadata: BundleMetadata{
			Target: BundleTarget{Kind: "Deployment", Name: "api"},
		},
	}
	bundleWriter.Write(bundle, bundleDir)

	entry := CatalogEntry{ID: "valid-bundle"}
	catalogWriter.Add(catalogDir, bundleDir, entry)

	// Validate - should pass
	reader := NewCatalogReader()
	catalog, _ := reader.Read(catalogDir)
	errors := reader.Validate(catalogDir, catalog)

	if len(errors) != 0 {
		t.Errorf("Expected no validation errors, got %d: %v", len(errors), errors)
	}
}

func TestCatalogReader_Validate_MissingBundle(t *testing.T) {
	tmpDir := t.TempDir()

	// Create catalog manually with non-existent bundle path
	catalogDir := filepath.Join(tmpDir, "catalog")
	os.MkdirAll(catalogDir, 0755)

	catalog := &Catalog{
		SchemaVersion: CatalogSchemaVersion,
		Bundles: []CatalogEntry{
			{
				ID:        "missing-bundle",
				Path:      "bundles/does-not-exist",
				CreatedAt: time.Now(),
			},
		},
	}

	writer := NewCatalogWriter()
	writer.writeCatalog(filepath.Join(catalogDir, "catalog.json"), catalog)

	// Validate - should fail
	reader := NewCatalogReader()
	readCatalog, _ := reader.Read(catalogDir)
	errors := reader.Validate(catalogDir, readCatalog)

	if len(errors) == 0 {
		t.Error("Expected validation errors for missing bundle")
	}

	// Check for path error
	hasPathError := false
	for _, err := range errors {
		if err.Field == "bundles[0].path" {
			hasPathError = true
			break
		}
	}
	if !hasPathError {
		t.Error("Expected path validation error")
	}
}

func TestCatalogReader_Validate_DuplicateID(t *testing.T) {
	tmpDir := t.TempDir()
	catalogDir := filepath.Join(tmpDir, "catalog")
	os.MkdirAll(catalogDir, 0755)

	catalog := &Catalog{
		SchemaVersion: CatalogSchemaVersion,
		Bundles: []CatalogEntry{
			{ID: "same-id", Path: "bundles/a", CreatedAt: time.Now()},
			{ID: "same-id", Path: "bundles/b", CreatedAt: time.Now()},
		},
	}

	writer := NewCatalogWriter()
	writer.writeCatalog(filepath.Join(catalogDir, "catalog.json"), catalog)

	reader := NewCatalogReader()
	readCatalog, _ := reader.Read(catalogDir)
	errors := reader.Validate(catalogDir, readCatalog)

	hasDuplicateError := false
	for _, err := range errors {
		if err.Field == "bundles[1].id" {
			hasDuplicateError = true
			break
		}
	}
	if !hasDuplicateError {
		t.Error("Expected duplicate ID validation error")
	}
}

func TestOrderBundles_Manifest(t *testing.T) {
	entries := []CatalogEntry{
		{ID: "c", CreatedAt: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)},
		{ID: "a", CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "b", CreatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
	}

	result := OrderBundles(entries, OrderManifest)

	// Should preserve original order
	if result[0].ID != "c" || result[1].ID != "a" || result[2].ID != "b" {
		t.Errorf("Manifest order should preserve input: got %s, %s, %s",
			result[0].ID, result[1].ID, result[2].ID)
	}
}

func TestOrderBundles_CreatedAt(t *testing.T) {
	entries := []CatalogEntry{
		{ID: "c", CreatedAt: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)},
		{ID: "a", CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "b", CreatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
	}

	result := OrderBundles(entries, OrderCreatedAt)

	// Should be sorted by created_at
	if result[0].ID != "a" || result[1].ID != "b" || result[2].ID != "c" {
		t.Errorf("CreatedAt order should sort by time: got %s, %s, %s",
			result[0].ID, result[1].ID, result[2].ID)
	}
}

func TestOrderBundles_CreatedAt_TieBreak(t *testing.T) {
	sameTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []CatalogEntry{
		{ID: "z", CreatedAt: sameTime},
		{ID: "a", CreatedAt: sameTime},
		{ID: "m", CreatedAt: sameTime},
	}

	result := OrderBundles(entries, OrderCreatedAt)

	// Should tie-break by ID (lexicographic)
	if result[0].ID != "a" || result[1].ID != "m" || result[2].ID != "z" {
		t.Errorf("Tie-break should use ID: got %s, %s, %s",
			result[0].ID, result[1].ID, result[2].ID)
	}
}

func TestOrderBundles_Sequence(t *testing.T) {
	seq1, seq2, seq3 := 3, 1, 2
	entries := []CatalogEntry{
		{ID: "a", Sequence: &seq1},
		{ID: "b", Sequence: &seq2},
		{ID: "c", Sequence: &seq3},
	}

	result := OrderBundles(entries, OrderSequence)

	// Should be sorted by sequence
	if result[0].ID != "b" || result[1].ID != "c" || result[2].ID != "a" {
		t.Errorf("Sequence order should sort by sequence: got %s, %s, %s",
			result[0].ID, result[1].ID, result[2].ID)
	}
}

func TestOrderBundles_Sequence_NilLast(t *testing.T) {
	seq1 := 1
	entries := []CatalogEntry{
		{ID: "no-seq"},
		{ID: "has-seq", Sequence: &seq1},
	}

	result := OrderBundles(entries, OrderSequence)

	// Entry with sequence should come first
	if result[0].ID != "has-seq" {
		t.Error("Entry with sequence should come before entry without")
	}
}

func TestOrderBundles_Input(t *testing.T) {
	entries := []CatalogEntry{
		{ID: "z"},
		{ID: "a"},
		{ID: "m"},
	}

	result := OrderBundles(entries, OrderInput)

	// Should preserve order (same as manifest)
	if result[0].ID != "z" || result[1].ID != "a" || result[2].ID != "m" {
		t.Errorf("Input order should preserve input: got %s, %s, %s",
			result[0].ID, result[1].ID, result[2].ID)
	}
}

func TestOrderBundles_DoesNotMutate(t *testing.T) {
	entries := []CatalogEntry{
		{ID: "c", CreatedAt: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)},
		{ID: "a", CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
	}

	originalFirst := entries[0].ID
	OrderBundles(entries, OrderCreatedAt)

	// Original should be unchanged
	if entries[0].ID != originalFirst {
		t.Error("OrderBundles should not mutate input")
	}
}

func TestSummarizeCatalog(t *testing.T) {
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)

	catalog := &Catalog{
		SchemaVersion: CatalogSchemaVersion,
		Bundles: []CatalogEntry{
			{
				ID:        "a",
				CreatedAt: t1,
				Scope:     &CatalogScope{Cluster: "prod", Namespace: "api", Target: "Deployment/api"},
				Labels:    map[string]string{"source": "ci"},
			},
			{
				ID:        "b",
				CreatedAt: t2,
				Scope:     &CatalogScope{Cluster: "prod", Namespace: "api", Target: "Deployment/api"},
				Labels:    map[string]string{"source": "ci", "ticket": "INC-123"},
			},
			{
				ID:        "c",
				CreatedAt: t3,
				Labels:    map[string]string{"source": "manual"},
			},
		},
	}

	summary := SummarizeCatalog(catalog)

	if summary.BundleCount != 3 {
		t.Errorf("BundleCount = %d, want 3", summary.BundleCount)
	}

	if summary.OldestBundle == nil || !summary.OldestBundle.Equal(t1) {
		t.Errorf("OldestBundle = %v, want %v", summary.OldestBundle, t1)
	}

	if summary.NewestBundle == nil || !summary.NewestBundle.Equal(t2) {
		t.Errorf("NewestBundle = %v, want %v", summary.NewestBundle, t2)
	}

	if len(summary.Scopes) != 1 {
		t.Errorf("Scopes count = %d, want 1", len(summary.Scopes))
	}

	if summary.Labels["source"] != 3 {
		t.Errorf("Labels[source] = %d, want 3", summary.Labels["source"])
	}
	if summary.Labels["ticket"] != 1 {
		t.Errorf("Labels[ticket] = %d, want 1", summary.Labels["ticket"])
	}
}

func TestCatalogRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	catalogDir := filepath.Join(tmpDir, "roundtrip-catalog")

	// Create catalog
	writer := NewCatalogWriter()
	writer.Init(catalogDir)

	// Create and add multiple bundles
	bundleWriter := NewBundleWriter("v0.15.0-test")

	for i, id := range []string{"bundle-1", "bundle-2", "bundle-3"} {
		bundleDir := filepath.Join(tmpDir, id)
		bundle := &DebugBundle{
			Metadata: BundleMetadata{
				Target: BundleTarget{Kind: "Deployment", Name: "api"},
			},
		}
		bundleWriter.Write(bundle, bundleDir)

		seq := i + 1
		entry := CatalogEntry{
			ID:       id,
			Sequence: &seq,
			Scope:    &CatalogScope{Cluster: "prod", Target: "Deployment/api"},
			Labels:   map[string]string{"index": string(rune('0' + i))},
		}
		writer.Add(catalogDir, bundleDir, entry)
	}

	// Read back and verify
	reader := NewCatalogReader()
	catalog, err := reader.Read(catalogDir)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if len(catalog.Bundles) != 3 {
		t.Fatalf("Expected 3 bundles, got %d", len(catalog.Bundles))
	}

	// Validate
	errors := reader.Validate(catalogDir, catalog)
	if len(errors) != 0 {
		t.Errorf("Validation errors: %v", errors)
	}

	// Test ordering
	ordered := OrderBundles(catalog.Bundles, OrderSequence)
	if ordered[0].ID != "bundle-1" || ordered[1].ID != "bundle-2" || ordered[2].ID != "bundle-3" {
		t.Error("Sequence ordering failed")
	}
}
