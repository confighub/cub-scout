// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"fmt"
	"strings"
)

// RenderAttributionReportASCII renders an attribution report as ASCII.
// The output is deterministic: same report always produces identical output.
func RenderAttributionReportASCII(report *AttributionReport) string {
	if report == nil {
		return "No attribution report available.\n"
	}

	var b strings.Builder

	// Header
	b.WriteString("Attribution Report\n")
	b.WriteString(strings.Repeat("─", 60))
	b.WriteString("\n\n")

	// Summary
	b.WriteString("Summary\n")
	b.WriteString(fmt.Sprintf("  Total objects:  %d\n", report.Summary.TotalItems))
	b.WriteString(fmt.Sprintf("  Owned:          %d\n", report.Summary.OwnedCount))
	if report.Summary.UnattributedCount > 0 {
		b.WriteString(fmt.Sprintf("  Unattributed:   %d\n", report.Summary.UnattributedCount))
	}
	if report.Summary.AmbiguousCount > 0 {
		b.WriteString(fmt.Sprintf("  Ambiguous:      %d\n", report.Summary.AmbiguousCount))
	}
	b.WriteString("\n")

	// Items
	if len(report.Items) == 0 {
		b.WriteString("No objects in report.\n")
		return b.String()
	}

	b.WriteString("Objects\n")
	b.WriteString(strings.Repeat("─", 60))
	b.WriteString("\n")

	for _, item := range report.Items {
		renderReportItem(&b, item)
	}

	return b.String()
}

// renderReportItem renders a single report item.
func renderReportItem(b *strings.Builder, item AttributionReportItem) {
	// Object header
	ref := formatAttributionRef(item.Ref)
	b.WriteString(fmt.Sprintf("\n%s\n", ref))

	// Reason with appropriate indicator
	switch item.Reason {
	case ReasonOwnedViaOwnerRef:
		b.WriteString("  Status: Owned (via owner reference)\n")
	case ReasonOwnedViaLabel:
		b.WriteString("  Status: Owned (via label)\n")
	case ReasonOwnedViaKustomize:
		b.WriteString("  Status: Owned (via kustomize)\n")
	case ReasonUnattributed:
		b.WriteString("  Status: Unattributed\n")
	case ReasonAmbiguous:
		b.WriteString("  Status: Ambiguous (multiple owners with equal score)\n")
	default:
		b.WriteString(fmt.Sprintf("  Status: %s\n", item.Reason))
	}

	// Best owner (if present)
	if item.BestOwner != nil {
		ownerRef := formatAttributionRef(item.BestOwner.Ref)
		b.WriteString(fmt.Sprintf("  Owner:  %s\n", ownerRef))
		b.WriteString(fmt.Sprintf("          type=%s, score=%d, evidence=%s\n",
			item.BestOwner.NodeType, item.BestOwner.Score, item.BestOwner.Evidence))
	}

	// Additional candidates (if more than one)
	if len(item.OwnerCandidates) > 1 {
		b.WriteString("  Other candidates:\n")
		for i, c := range item.OwnerCandidates {
			if i == 0 {
				continue // Skip best owner (already shown)
			}
			candRef := formatAttributionRef(c.Ref)
			b.WriteString(fmt.Sprintf("    - %s (score=%d, %s)\n", candRef, c.Score, c.Evidence))
		}
	}
}

// RenderAttributionReportJSON is provided for consistency but typically
// the caller just uses json.Marshal directly. This is here for future
// extension if we need custom JSON handling.
func RenderAttributionReportJSON(report *AttributionReport) ([]byte, error) {
	// For now, just delegate to standard encoding
	// This function exists for API symmetry with ASCII
	return nil, fmt.Errorf("use json.Marshal directly for JSON output")
}
