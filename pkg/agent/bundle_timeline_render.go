// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"fmt"
	"strings"
)

// RenderBundleTimelineASCII renders a timeline as ASCII text.
// This renderer consumes ONLY the BundleTimeline struct — it does not
// peek back into the original bundles or catalog (ASCII = f(JSON) + g).
func RenderBundleTimelineASCII(timeline *BundleTimeline) string {
	var b strings.Builder

	// Header
	b.WriteString("Bundle Timeline\n")
	b.WriteString(strings.Repeat("─", 70))
	b.WriteString("\n\n")

	// Catalog info
	b.WriteString(fmt.Sprintf("Catalog:  %s\n", timeline.CatalogRef.Path))
	b.WriteString(fmt.Sprintf("Bundles:  %d\n", timeline.CatalogRef.BundleCount))
	b.WriteString(fmt.Sprintf("Order:    %s (tie-break: %s)\n", timeline.Ordering.Mode, timeline.Ordering.TieBreak))
	b.WriteString(fmt.Sprintf("Join:     %s\n", timeline.JoinMode))
	b.WriteString("\n")

	// Summary
	b.WriteString("Summary\n")
	b.WriteString(strings.Repeat("─", 40))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  Series:  %d object(s)\n", timeline.Summary.SeriesCount))
	b.WriteString(fmt.Sprintf("  Points:  %d present, %d gaps\n", timeline.Summary.TotalPoints, timeline.Summary.GapCount))
	b.WriteString("\n")

	// Bundle timeline header
	if len(timeline.Series) == 0 {
		b.WriteString("No objects in timeline.\n")
		return b.String()
	}

	// Build bundle ID header row
	if timeline.Summary.BundleCount > 0 && len(timeline.Series) > 0 {
		b.WriteString("Timeline\n")
		b.WriteString(strings.Repeat("─", 40))
		b.WriteString("\n\n")

		// Render bundle IDs as column headers
		b.WriteString(renderTimelineHeader(timeline))
		b.WriteString("\n")

		// Render each series
		for _, series := range timeline.Series {
			b.WriteString(renderSeriesRow(series, timeline.Summary.BundleCount))
		}
	}

	return b.String()
}

// renderTimelineHeader renders the bundle ID header row.
func renderTimelineHeader(timeline *BundleTimeline) string {
	var b strings.Builder

	// Object identity column (fixed width)
	b.WriteString(fmt.Sprintf("%-40s", "Object"))

	// Bundle columns
	if len(timeline.Series) > 0 {
		for _, point := range timeline.Series[0].Points {
			// Truncate bundle ID if needed
			id := point.BundleID
			if len(id) > 12 {
				id = id[:12]
			}
			b.WriteString(fmt.Sprintf(" %-12s", id))
		}
	}

	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", 40))

	if len(timeline.Series) > 0 {
		for range timeline.Series[0].Points {
			b.WriteString(" ")
			b.WriteString(strings.Repeat("─", 12))
		}
	}

	return b.String()
}

// renderSeriesRow renders a single object series as a row.
func renderSeriesRow(series ObjectSeries, bundleCount int) string {
	var b strings.Builder

	// Object identity (truncate if needed)
	identity := series.Identity
	if len(identity) > 38 {
		identity = identity[:35] + "..."
	}
	b.WriteString(fmt.Sprintf("%-40s", identity))

	// Points
	for _, point := range series.Points {
		b.WriteString(" ")
		b.WriteString(renderPointCell(point))
	}

	b.WriteString("\n")
	return b.String()
}

// renderPointCell renders a single timeline point as a cell.
func renderPointCell(point TimelinePoint) string {
	switch point.Status {
	case PointPresent:
		if point.Sections != nil {
			// Show drift count and correlation indicator
			corr := " "
			if point.Sections.HasCorrelation {
				corr = "C"
			}
			return fmt.Sprintf("%-12s", fmt.Sprintf("D:%d %s", point.Sections.DriftCount, corr))
		}
		return fmt.Sprintf("%-12s", "present")

	case PointMissing:
		return fmt.Sprintf("%-12s", "·")

	case PointAmbiguous:
		return fmt.Sprintf("%-12s", "?")

	case PointUnjoinable:
		return fmt.Sprintf("%-12s", "!")

	default:
		return fmt.Sprintf("%-12s", "-")
	}
}
