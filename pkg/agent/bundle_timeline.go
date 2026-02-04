// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package agent provides Bundle Timeline v1 for N-bundle time-series views.
//
// A timeline aligns multiple bundles from a catalog into a time-series
// showing how objects evolve across snapshots.
//
// Contract:
// - Timelines are computed from catalogs (explicit ordering)
// - No implicit ordering (never inferred from filesystem)
// - Join mode must be explicit (object_id only in PR3)
// - Gaps and missing points are first-class
// - Output is deterministic (series sorted by identity, points by bundle order)
package agent

import (
	"fmt"
	"path/filepath"
	"sort"
	"time"
)

// BundleTimelineSchemaVersion is the schema version for timelines.
const BundleTimelineSchemaVersion = "bundle-timeline.v1"

// BundleTimeline is the top-level timeline result.
// Schema: bundle-timeline.v1
type BundleTimeline struct {
	// SchemaVersion identifies the timeline format
	SchemaVersion string `json:"schema_version"`

	// CatalogRef identifies the source catalog
	CatalogRef TimelineCatalogRef `json:"catalog_ref"`

	// Ordering describes how bundles are ordered
	Ordering TimelineOrdering `json:"ordering"`

	// JoinMode specifies how objects were matched across bundles
	JoinMode JoinMode `json:"join_mode"`

	// Summary provides aggregate counts
	Summary TimelineSummary `json:"summary"`

	// Series lists per-object time series, sorted by identity
	Series []ObjectSeries `json:"series"`
}

// TimelineCatalogRef identifies the source catalog.
type TimelineCatalogRef struct {
	// Path is the catalog directory path
	Path string `json:"path"`

	// BundleCount is the number of bundles in the catalog
	BundleCount int `json:"bundle_count"`

	// SchemaVersion is the catalog's schema version
	SchemaVersion string `json:"schema_version"`
}

// TimelineOrdering describes how bundles are ordered.
type TimelineOrdering struct {
	// Mode is the ordering mode used
	Mode OrderMode `json:"mode"`

	// TieBreak describes tie-breaking rule
	TieBreak string `json:"tie_break"`
}

// TimelineSummary provides aggregate counts for the timeline.
type TimelineSummary struct {
	// BundleCount is the number of bundles
	BundleCount int `json:"bundle_count"`

	// SeriesCount is the number of object series
	SeriesCount int `json:"series_count"`

	// TotalPoints is the total number of points across all series
	TotalPoints int `json:"total_points"`

	// GapCount is the number of gaps (missing points) across all series
	GapCount int `json:"gap_count"`
}

// ObjectSeries represents the time series for a single object.
type ObjectSeries struct {
	// Identity is the join key (object_id)
	Identity string `json:"identity"`

	// Points are the data points, aligned to bundle order
	Points []TimelinePoint `json:"points"`
}

// PointStatus describes the status of an object at a point in time.
type PointStatus string

const (
	// PointPresent means the object was found in this bundle
	PointPresent PointStatus = "present"

	// PointMissing means the object was not found in this bundle (gap)
	PointMissing PointStatus = "missing"

	// PointAmbiguous means multiple matches were found
	PointAmbiguous PointStatus = "ambiguous"

	// PointUnjoinable means the object couldn't be joined
	PointUnjoinable PointStatus = "unjoinable"
)

// TimelinePoint represents a single point in an object's timeline.
type TimelinePoint struct {
	// BundleID identifies which bundle this point is from
	BundleID string `json:"bundle_id"`

	// CreatedAt is the bundle's creation time
	CreatedAt time.Time `json:"created_at"`

	// Status indicates whether the object was found
	Status PointStatus `json:"status"`

	// Sections contains per-section summaries (only if present)
	Sections *PointSections `json:"sections,omitempty"`
}

// PointSections contains section summaries for a timeline point.
type PointSections struct {
	// DriftCount is the number of drift findings for this object
	DriftCount int `json:"drift_count"`

	// HasCorrelation indicates if correlation data exists
	HasCorrelation bool `json:"has_correlation"`
}

// BuildTimeline constructs a timeline from a catalog.
func BuildTimeline(catalogDir string, orderMode OrderMode, joinMode JoinMode) (*BundleTimeline, error) {
	// Validate join mode
	if joinMode == JoinComposite {
		return nil, fmt.Errorf("join mode 'composite' not yet implemented")
	}
	if joinMode == JoinNone {
		return nil, fmt.Errorf("join mode 'none' not yet implemented")
	}
	if joinMode == "" {
		joinMode = JoinObjectID
	}

	// Read catalog
	reader := NewCatalogReader()
	catalog, err := reader.Read(catalogDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read catalog: %w", err)
	}

	// Order bundles
	orderedEntries := OrderBundles(catalog.Bundles, orderMode)

	// Load all bundles
	bundleReader := NewBundleReader()
	bundles := make([]*DebugBundle, len(orderedEntries))
	for i, entry := range orderedEntries {
		bundlePath := filepath.Join(catalogDir, entry.Path)
		bundle, err := bundleReader.Read(bundlePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read bundle %s: %w", entry.ID, err)
		}
		bundles[i] = bundle
	}

	// Build object maps for each bundle
	bundleObjects := make([]map[string]*timelineObjectData, len(bundles))
	allIdentities := make(map[string]bool)

	for i, bundle := range bundles {
		objMap := buildTimelineObjectMap(bundle)
		bundleObjects[i] = objMap
		for id := range objMap {
			allIdentities[id] = true
		}
	}

	// Sort identities for deterministic output
	sortedIdentities := make([]string, 0, len(allIdentities))
	for id := range allIdentities {
		sortedIdentities = append(sortedIdentities, id)
	}
	sort.Strings(sortedIdentities)

	// Build series
	series := make([]ObjectSeries, 0, len(sortedIdentities))
	totalPoints := 0
	gapCount := 0

	for _, identity := range sortedIdentities {
		objSeries := ObjectSeries{
			Identity: identity,
			Points:   make([]TimelinePoint, len(orderedEntries)),
		}

		for i, entry := range orderedEntries {
			point := TimelinePoint{
				BundleID:  entry.ID,
				CreatedAt: entry.CreatedAt,
			}

			objData, found := bundleObjects[i][identity]
			if found {
				point.Status = PointPresent
				point.Sections = &PointSections{
					DriftCount:     objData.driftCount,
					HasCorrelation: objData.hasCorrelation,
				}
				totalPoints++
			} else {
				point.Status = PointMissing
				gapCount++
			}

			objSeries.Points[i] = point
		}

		series = append(series, objSeries)
	}

	// Build timeline
	timeline := &BundleTimeline{
		SchemaVersion: BundleTimelineSchemaVersion,
		CatalogRef: TimelineCatalogRef{
			Path:          catalogDir,
			BundleCount:   len(orderedEntries),
			SchemaVersion: catalog.SchemaVersion,
		},
		Ordering: TimelineOrdering{
			Mode:     orderMode,
			TieBreak: "id",
		},
		JoinMode: joinMode,
		Summary: TimelineSummary{
			BundleCount: len(orderedEntries),
			SeriesCount: len(series),
			TotalPoints: totalPoints,
			GapCount:    gapCount,
		},
		Series: series,
	}

	return timeline, nil
}

// timelineObjectData holds summary data for an object at a point.
type timelineObjectData struct {
	driftCount     int
	hasCorrelation bool
}

// buildTimelineObjectMap extracts object summaries from a bundle.
func buildTimelineObjectMap(bundle *DebugBundle) map[string]*timelineObjectData {
	objects := make(map[string]*timelineObjectData)

	// Group drift findings by object_id
	driftByObject := make(map[string]int)
	for _, df := range bundle.DriftFindings {
		id := df.ObjectID
		if id == "" {
			id = fmt.Sprintf("%s:%s/%s",
				df.Object.Kind,
				df.Object.Namespace,
				df.Object.Name)
		}
		driftByObject[id]++
	}

	// Create entries for all objects with drift
	for id, count := range driftByObject {
		objects[id] = &timelineObjectData{
			driftCount: count,
		}
	}

	// Add the target object from session if present
	if bundle.Session != nil {
		targetID := fmt.Sprintf("%s:%s/%s",
			bundle.Session.Target.Kind,
			bundle.Session.Target.Namespace,
			bundle.Session.Target.Name)

		if objects[targetID] == nil {
			objects[targetID] = &timelineObjectData{}
		}

		if bundle.Session.RootCause != nil {
			objects[targetID].hasCorrelation = true
		}
	}

	return objects
}
