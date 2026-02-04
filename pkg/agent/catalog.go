// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package agent provides Catalog v1 for indexing multiple debug bundles.
//
// A catalog is a file-backed manifest that indexes multiple bundles
// for multi-bundle operations (diffs, timelines). Catalogs are:
//   - Explicit: ordering is declared, not inferred
//   - Deterministic: same catalog + same bundles = same results
//   - Portable: directory-based, no database dependencies
//
// Contract:
//   - Catalogs do not modify bundles
//   - Ordering is never inferred from filesystem
//   - All grouping requires explicit scope or labels
package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// CatalogSchemaVersion is the current catalog schema version.
const CatalogSchemaVersion = "catalog.v1"

// Catalog represents a collection of debug bundles.
// This is the root structure for catalog.json.
type Catalog struct {
	// SchemaVersion identifies the catalog format version
	SchemaVersion string `json:"schema_version"`

	// Bundles is the list of bundle entries
	// Order is authoritative when using --order manifest
	Bundles []CatalogEntry `json:"bundles"`
}

// CatalogEntry represents a single bundle in the catalog.
type CatalogEntry struct {
	// ID is a stable, unique identifier for this bundle within the catalog
	// Required. Used for tie-breaking and references.
	ID string `json:"id"`

	// Path is the relative path from the catalog directory to the bundle
	// Required. Must point to a valid bundle directory.
	Path string `json:"path"`

	// CreatedAt is copied from the bundle's metadata.createdAt
	// Required. Used for --order created_at.
	CreatedAt time.Time `json:"created_at"`

	// Digest is an optional SHA256 hash of the bundle contents
	// Used for integrity verification.
	Digest string `json:"digest,omitempty"`

	// Scope enables "same thing" grouping across bundles
	// Recommended for multi-bundle operations.
	Scope *CatalogScope `json:"scope,omitempty"`

	// Labels are arbitrary key/value metadata
	// Optional. For user organization.
	Labels map[string]string `json:"labels,omitempty"`

	// Sequence is an explicit ordering integer
	// Optional. Used for --order sequence.
	Sequence *int `json:"sequence,omitempty"`
}

// CatalogScope identifies what a bundle is "about".
// Used for grouping bundles that represent the same logical target.
type CatalogScope struct {
	// Cluster is the cluster name or environment
	Cluster string `json:"cluster,omitempty"`

	// Namespace is the Kubernetes namespace
	Namespace string `json:"namespace,omitempty"`

	// Target identifies the workload (e.g., "Deployment/api")
	Target string `json:"target,omitempty"`
}

// OrderMode specifies how bundles should be ordered.
type OrderMode string

const (
	// OrderManifest uses the order in catalog.json (default)
	OrderManifest OrderMode = "manifest"

	// OrderCreatedAt sorts by created_at, tie-break by id
	OrderCreatedAt OrderMode = "created_at"

	// OrderSequence sorts by sequence field, tie-break by id
	OrderSequence OrderMode = "sequence"

	// OrderInput preserves CLI argument order
	OrderInput OrderMode = "input"
)

// CatalogWriter writes catalogs to disk.
type CatalogWriter struct{}

// NewCatalogWriter creates a new catalog writer.
func NewCatalogWriter() *CatalogWriter {
	return &CatalogWriter{}
}

// Init creates a new empty catalog at the specified path.
func (w *CatalogWriter) Init(catalogDir string) error {
	// Create catalog directory
	if err := os.MkdirAll(catalogDir, 0755); err != nil {
		return fmt.Errorf("failed to create catalog directory: %w", err)
	}

	// Create bundles subdirectory
	bundlesDir := filepath.Join(catalogDir, "bundles")
	if err := os.MkdirAll(bundlesDir, 0755); err != nil {
		return fmt.Errorf("failed to create bundles directory: %w", err)
	}

	// Create empty catalog.json
	catalog := &Catalog{
		SchemaVersion: CatalogSchemaVersion,
		Bundles:       []CatalogEntry{},
	}

	catalogPath := filepath.Join(catalogDir, "catalog.json")
	return w.writeCatalog(catalogPath, catalog)
}

// Add adds a bundle to the catalog.
// The bundle is copied to the catalog's bundles directory.
func (w *CatalogWriter) Add(catalogDir string, bundlePath string, entry CatalogEntry) error {
	// Read existing catalog
	catalogPath := filepath.Join(catalogDir, "catalog.json")
	catalog, err := w.readCatalog(catalogPath)
	if err != nil {
		return fmt.Errorf("failed to read catalog: %w", err)
	}

	// Validate bundle exists and read its metadata
	bundleReader := NewBundleReader()
	bundle, err := bundleReader.Read(bundlePath)
	if err != nil {
		return fmt.Errorf("failed to read bundle: %w", err)
	}

	// Use bundle's created_at if not provided
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = bundle.Metadata.CreatedAt
	}

	// Generate ID if not provided
	if entry.ID == "" {
		entry.ID = generateBundleID(bundle)
	}

	// Check for duplicate ID
	for _, existing := range catalog.Bundles {
		if existing.ID == entry.ID {
			return fmt.Errorf("bundle with ID %q already exists in catalog", entry.ID)
		}
	}

	// Copy bundle to catalog's bundles directory
	destPath := filepath.Join(catalogDir, "bundles", entry.ID)
	if err := copyBundle(bundlePath, destPath); err != nil {
		return fmt.Errorf("failed to copy bundle: %w", err)
	}

	// Set relative path
	entry.Path = filepath.Join("bundles", entry.ID)

	// Add to catalog
	catalog.Bundles = append(catalog.Bundles, entry)

	// Write updated catalog
	return w.writeCatalog(catalogPath, catalog)
}

// writeCatalog writes a catalog to disk.
func (w *CatalogWriter) writeCatalog(path string, catalog *Catalog) error {
	data, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal catalog: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// readCatalog reads a catalog from disk.
func (w *CatalogWriter) readCatalog(path string) (*Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var catalog Catalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("failed to parse catalog: %w", err)
	}

	return &catalog, nil
}

// generateBundleID generates a unique ID for a bundle.
func generateBundleID(bundle *DebugBundle) string {
	// Use target + timestamp for uniqueness
	target := bundle.Metadata.Target
	ts := bundle.Metadata.CreatedAt.Format("20060102-150405")
	return fmt.Sprintf("%s-%s-%s", target.Kind, target.Name, ts)
}

// copyBundle copies a bundle directory to a new location.
func copyBundle(src, dst string) error {
	// Create destination directory
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	// Copy all files
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			// Skip subdirectories for now (bundles are flat)
			continue
		}

		data, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", entry.Name(), err)
		}

		if err := os.WriteFile(dstPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", entry.Name(), err)
		}
	}

	return nil
}

// CatalogReader reads and validates catalogs.
type CatalogReader struct{}

// NewCatalogReader creates a new catalog reader.
func NewCatalogReader() *CatalogReader {
	return &CatalogReader{}
}

// Read reads a catalog from the specified directory.
func (r *CatalogReader) Read(catalogDir string) (*Catalog, error) {
	catalogPath := filepath.Join(catalogDir, "catalog.json")

	data, err := os.ReadFile(catalogPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read catalog.json: %w", err)
	}

	var catalog Catalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("failed to parse catalog.json: %w", err)
	}

	return &catalog, nil
}

// Validate validates a catalog and its bundle references.
func (r *CatalogReader) Validate(catalogDir string, catalog *Catalog) []ValidationError {
	var errors []ValidationError

	// Check schema version
	if catalog.SchemaVersion != CatalogSchemaVersion {
		errors = append(errors, ValidationError{
			Field:   "schema_version",
			Message: fmt.Sprintf("unsupported schema version %q (expected %q)", catalog.SchemaVersion, CatalogSchemaVersion),
		})
	}

	// Check each bundle entry
	seenIDs := make(map[string]bool)
	for i, entry := range catalog.Bundles {
		prefix := fmt.Sprintf("bundles[%d]", i)

		// Check required fields
		if entry.ID == "" {
			errors = append(errors, ValidationError{
				Field:   prefix + ".id",
				Message: "id is required",
			})
		} else if seenIDs[entry.ID] {
			errors = append(errors, ValidationError{
				Field:   prefix + ".id",
				Message: fmt.Sprintf("duplicate id %q", entry.ID),
			})
		} else {
			seenIDs[entry.ID] = true
		}

		if entry.Path == "" {
			errors = append(errors, ValidationError{
				Field:   prefix + ".path",
				Message: "path is required",
			})
		} else {
			// Check path exists
			bundlePath := filepath.Join(catalogDir, entry.Path)
			if _, err := os.Stat(bundlePath); os.IsNotExist(err) {
				errors = append(errors, ValidationError{
					Field:   prefix + ".path",
					Message: fmt.Sprintf("bundle path does not exist: %s", entry.Path),
				})
			} else {
				// Validate bundle metadata matches catalog entry
				bundleReader := NewBundleReader()
				bundle, err := bundleReader.Read(bundlePath)
				if err != nil {
					errors = append(errors, ValidationError{
						Field:   prefix + ".path",
						Message: fmt.Sprintf("failed to read bundle: %v", err),
					})
				} else {
					// Check created_at matches
					if !entry.CreatedAt.Equal(bundle.Metadata.CreatedAt) {
						errors = append(errors, ValidationError{
							Field:   prefix + ".created_at",
							Message: fmt.Sprintf("created_at mismatch: catalog has %v, bundle has %v",
								entry.CreatedAt.Format(time.RFC3339),
								bundle.Metadata.CreatedAt.Format(time.RFC3339)),
						})
					}
				}
			}
		}

		if entry.CreatedAt.IsZero() {
			errors = append(errors, ValidationError{
				Field:   prefix + ".created_at",
				Message: "created_at is required",
			})
		}
	}

	return errors
}

// ValidationError represents a validation error.
type ValidationError struct {
	Field   string
	Message string
}

// Error implements the error interface.
func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// OrderBundles orders bundle entries according to the specified mode.
// Returns a new slice (does not modify input).
func OrderBundles(entries []CatalogEntry, mode OrderMode) []CatalogEntry {
	result := make([]CatalogEntry, len(entries))
	copy(result, entries)

	switch mode {
	case OrderManifest, OrderInput:
		// Keep original order
		return result

	case OrderCreatedAt:
		sort.SliceStable(result, func(i, j int) bool {
			if result[i].CreatedAt.Equal(result[j].CreatedAt) {
				return result[i].ID < result[j].ID // tie-break by id
			}
			return result[i].CreatedAt.Before(result[j].CreatedAt)
		})

	case OrderSequence:
		sort.SliceStable(result, func(i, j int) bool {
			// Entries without sequence go last
			if result[i].Sequence == nil && result[j].Sequence == nil {
				return result[i].ID < result[j].ID
			}
			if result[i].Sequence == nil {
				return false
			}
			if result[j].Sequence == nil {
				return true
			}
			if *result[i].Sequence == *result[j].Sequence {
				return result[i].ID < result[j].ID // tie-break by id
			}
			return *result[i].Sequence < *result[j].Sequence
		})
	}

	return result
}

// CatalogSummary provides a summary of a catalog for display.
type CatalogSummary struct {
	SchemaVersion string
	BundleCount   int
	OldestBundle  *time.Time
	NewestBundle  *time.Time
	Scopes        []string
	Labels        map[string]int
}

// Summarize returns a summary of a catalog.
func SummarizeCatalog(catalog *Catalog) CatalogSummary {
	summary := CatalogSummary{
		SchemaVersion: catalog.SchemaVersion,
		BundleCount:   len(catalog.Bundles),
		Labels:        make(map[string]int),
	}

	scopeSet := make(map[string]bool)

	for _, entry := range catalog.Bundles {
		// Track oldest/newest
		if summary.OldestBundle == nil || entry.CreatedAt.Before(*summary.OldestBundle) {
			t := entry.CreatedAt
			summary.OldestBundle = &t
		}
		if summary.NewestBundle == nil || entry.CreatedAt.After(*summary.NewestBundle) {
			t := entry.CreatedAt
			summary.NewestBundle = &t
		}

		// Track scopes
		if entry.Scope != nil {
			scopeKey := fmt.Sprintf("%s/%s/%s", entry.Scope.Cluster, entry.Scope.Namespace, entry.Scope.Target)
			scopeSet[scopeKey] = true
		}

		// Track labels
		for k := range entry.Labels {
			summary.Labels[k]++
		}
	}

	// Convert scope set to sorted list
	for scope := range scopeSet {
		summary.Scopes = append(summary.Scopes, scope)
	}
	sort.Strings(summary.Scopes)

	return summary
}
