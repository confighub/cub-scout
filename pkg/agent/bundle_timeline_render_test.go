// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"strings"
	"testing"
	"time"
)

func TestRenderBundleTimelineDeterministic(t *testing.T) {
	timeline := &BundleTimeline{
		SchemaVersion: BundleTimelineSchemaVersion,
		CatalogRef: TimelineCatalogRef{
			Path:          "./test-catalog",
			BundleCount:   2,
			SchemaVersion: CatalogSchemaVersion,
		},
		Ordering: TimelineOrdering{
			Mode:     OrderCreatedAt,
			TieBreak: "id",
		},
		JoinMode: JoinObjectID,
		Summary: TimelineSummary{
			BundleCount: 2,
			SeriesCount: 1,
			TotalPoints: 2,
			GapCount:    0,
		},
		Series: []ObjectSeries{
			{
				Identity: "Deployment:prod/api",
				Points: []TimelinePoint{
					{
						BundleID:  "before",
						CreatedAt: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
						Status:    PointPresent,
						Sections:  &PointSections{DriftCount: 2, HasCorrelation: false},
					},
					{
						BundleID:  "after",
						CreatedAt: time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC),
						Status:    PointPresent,
						Sections:  &PointSections{DriftCount: 5, HasCorrelation: true},
					},
				},
			},
		},
	}

	// Render multiple times and verify identical output
	first := RenderBundleTimelineASCII(timeline)
	for i := 0; i < 5; i++ {
		output := RenderBundleTimelineASCII(timeline)
		if output != first {
			t.Errorf("Iteration %d: output differs from first render", i)
		}
	}
}

func TestRenderBundleTimelineHeader(t *testing.T) {
	timeline := &BundleTimeline{
		SchemaVersion: BundleTimelineSchemaVersion,
		CatalogRef: TimelineCatalogRef{
			Path:        "./my-catalog",
			BundleCount: 3,
		},
		Ordering: TimelineOrdering{
			Mode:     OrderManifest,
			TieBreak: "id",
		},
		JoinMode: JoinObjectID,
		Summary: TimelineSummary{
			BundleCount: 3,
			SeriesCount: 2,
			TotalPoints: 5,
			GapCount:    1,
		},
		Series: []ObjectSeries{},
	}

	output := RenderBundleTimelineASCII(timeline)

	if !strings.Contains(output, "Bundle Timeline") {
		t.Error("Output missing 'Bundle Timeline' header")
	}
	if !strings.Contains(output, "Catalog:") {
		t.Error("Output missing 'Catalog:' line")
	}
	if !strings.Contains(output, "./my-catalog") {
		t.Error("Output missing catalog path")
	}
	if !strings.Contains(output, "Bundles:  3") {
		t.Error("Output missing bundle count")
	}
	if !strings.Contains(output, "Order:    manifest") {
		t.Error("Output missing order mode")
	}
	if !strings.Contains(output, "Join:     object_id") {
		t.Error("Output missing join mode")
	}
}

func TestRenderBundleTimelineSummary(t *testing.T) {
	timeline := &BundleTimeline{
		SchemaVersion: BundleTimelineSchemaVersion,
		CatalogRef:    TimelineCatalogRef{Path: "./catalog"},
		Ordering:      TimelineOrdering{Mode: OrderManifest, TieBreak: "id"},
		JoinMode:      JoinObjectID,
		Summary: TimelineSummary{
			BundleCount: 5,
			SeriesCount: 10,
			TotalPoints: 45,
			GapCount:    5,
		},
		Series: []ObjectSeries{},
	}

	output := RenderBundleTimelineASCII(timeline)

	if !strings.Contains(output, "Summary") {
		t.Error("Output missing 'Summary' section")
	}
	if !strings.Contains(output, "Series:  10 object(s)") {
		t.Error("Output missing series count")
	}
	if !strings.Contains(output, "45 present") {
		t.Error("Output missing points count")
	}
	if !strings.Contains(output, "5 gaps") {
		t.Error("Output missing gaps count")
	}
}

func TestRenderBundleTimelineNoObjects(t *testing.T) {
	timeline := &BundleTimeline{
		SchemaVersion: BundleTimelineSchemaVersion,
		CatalogRef:    TimelineCatalogRef{Path: "./empty"},
		Ordering:      TimelineOrdering{Mode: OrderManifest, TieBreak: "id"},
		JoinMode:      JoinObjectID,
		Summary:       TimelineSummary{},
		Series:        []ObjectSeries{},
	}

	output := RenderBundleTimelineASCII(timeline)

	if !strings.Contains(output, "No objects in timeline") {
		t.Error("Output should indicate no objects")
	}
}

func TestRenderBundleTimelinePointStatus(t *testing.T) {
	timeline := &BundleTimeline{
		SchemaVersion: BundleTimelineSchemaVersion,
		CatalogRef:    TimelineCatalogRef{Path: "./catalog", BundleCount: 3},
		Ordering:      TimelineOrdering{Mode: OrderManifest, TieBreak: "id"},
		JoinMode:      JoinObjectID,
		Summary:       TimelineSummary{BundleCount: 3, SeriesCount: 1},
		Series: []ObjectSeries{
			{
				Identity: "Deployment:prod/api",
				Points: []TimelinePoint{
					{BundleID: "b1", Status: PointPresent, Sections: &PointSections{DriftCount: 2}},
					{BundleID: "b2", Status: PointMissing},
					{BundleID: "b3", Status: PointPresent, Sections: &PointSections{DriftCount: 0, HasCorrelation: true}},
				},
			},
		},
	}

	output := RenderBundleTimelineASCII(timeline)

	// Check that present points show drift count
	if !strings.Contains(output, "D:2") {
		t.Error("Output missing drift count for present point")
	}

	// Check that missing points show gap indicator
	if !strings.Contains(output, "·") {
		t.Error("Output missing gap indicator for missing point")
	}

	// Check correlation indicator
	if !strings.Contains(output, "C") {
		t.Error("Output missing correlation indicator")
	}
}

func TestRenderBundleTimelineBundleHeaders(t *testing.T) {
	timeline := &BundleTimeline{
		SchemaVersion: BundleTimelineSchemaVersion,
		CatalogRef:    TimelineCatalogRef{Path: "./catalog", BundleCount: 2},
		Ordering:      TimelineOrdering{Mode: OrderManifest, TieBreak: "id"},
		JoinMode:      JoinObjectID,
		Summary:       TimelineSummary{BundleCount: 2, SeriesCount: 1},
		Series: []ObjectSeries{
			{
				Identity: "Deployment:prod/api",
				Points: []TimelinePoint{
					{BundleID: "before-fix", Status: PointPresent, Sections: &PointSections{}},
					{BundleID: "after-fix", Status: PointPresent, Sections: &PointSections{}},
				},
			},
		},
	}

	output := RenderBundleTimelineASCII(timeline)

	if !strings.Contains(output, "before-fix") {
		t.Error("Output missing first bundle ID in header")
	}
	if !strings.Contains(output, "after-fix") {
		t.Error("Output missing second bundle ID in header")
	}
}
