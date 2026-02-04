// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"fmt"
	"strings"
)

// RenderBundleDiffASCII renders a bundle diff as ASCII text.
// This renderer consumes ONLY the BundleDiff struct — it does not
// peek back into the original bundles (ASCII = f(JSON) + g).
func RenderBundleDiffASCII(diff *BundleDiff) string {
	var b strings.Builder

	// Header
	b.WriteString("Bundle Diff\n")
	b.WriteString(strings.Repeat("─", 60))
	b.WriteString("\n\n")

	// From/To
	b.WriteString(fmt.Sprintf("From: %s (%s)\n",
		diff.From.ID,
		diff.From.CreatedAt.Format("2006-01-02 15:04:05 UTC")))
	b.WriteString(fmt.Sprintf("To:   %s (%s)\n",
		diff.To.ID,
		diff.To.CreatedAt.Format("2006-01-02 15:04:05 UTC")))
	b.WriteString(fmt.Sprintf("Join: %s\n", diff.JoinMode))
	b.WriteString("\n")

	// Summary
	b.WriteString("Summary\n")
	b.WriteString(strings.Repeat("─", 40))
	b.WriteString("\n")
	b.WriteString(renderDiffSummary(&diff.Summary))
	b.WriteString("\n")

	// Objects
	if len(diff.Objects) == 0 {
		b.WriteString("No objects to compare.\n")
	} else {
		b.WriteString("Objects\n")
		b.WriteString(strings.Repeat("─", 40))
		b.WriteString("\n")
		for _, obj := range diff.Objects {
			b.WriteString(renderObjectDiff(&obj))
		}
	}

	return b.String()
}

// renderDiffSummary renders the diff summary.
func renderDiffSummary(s *DiffSummary) string {
	var parts []string

	if s.Added > 0 {
		parts = append(parts, fmt.Sprintf("added=%d", s.Added))
	}
	if s.Removed > 0 {
		parts = append(parts, fmt.Sprintf("removed=%d", s.Removed))
	}
	if s.Changed > 0 {
		parts = append(parts, fmt.Sprintf("changed=%d", s.Changed))
	}
	if s.Unchanged > 0 {
		parts = append(parts, fmt.Sprintf("unchanged=%d", s.Unchanged))
	}
	if s.Ambiguous > 0 {
		parts = append(parts, fmt.Sprintf("ambiguous=%d", s.Ambiguous))
	}
	if s.Unjoinable > 0 {
		parts = append(parts, fmt.Sprintf("unjoinable=%d", s.Unjoinable))
	}

	if len(parts) == 0 {
		return "  (no objects)\n"
	}

	return "  " + strings.Join(parts, ", ") + "\n"
}

// renderObjectDiff renders a single object diff.
func renderObjectDiff(obj *ObjectDiff) string {
	var b strings.Builder

	// Object line: identity + status + section flags
	statusIcon := statusToIcon(obj.Status)
	sectionFlags := renderSectionFlags(obj.Sections)

	b.WriteString(fmt.Sprintf("  %s %s  %s", statusIcon, obj.Identity, string(obj.Status)))
	if sectionFlags != "" {
		b.WriteString(fmt.Sprintf("  [%s]", sectionFlags))
	}
	b.WriteString("\n")

	// Section details for changed/added/removed
	if obj.Status == StatusChanged || obj.Status == StatusAdded || obj.Status == StatusRemoved {
		for _, section := range obj.Sections {
			if section.Status != SectionUnchanged {
				b.WriteString(renderSectionDetail(&section))
			}
		}
	}

	return b.String()
}

// statusToIcon returns an icon for the object status.
func statusToIcon(status ObjectStatus) string {
	switch status {
	case StatusAdded:
		return "+"
	case StatusRemoved:
		return "-"
	case StatusChanged:
		return "~"
	case StatusUnchanged:
		return "="
	case StatusAmbiguous:
		return "?"
	case StatusUnjoinable:
		return "!"
	default:
		return " "
	}
}

// renderSectionFlags renders compact section status flags.
func renderSectionFlags(sections []SectionDiff) string {
	var flags []string
	for _, s := range sections {
		if s.Status != SectionUnchanged {
			flags = append(flags, fmt.Sprintf("%s:%s", s.Name, s.Status))
		}
	}
	return strings.Join(flags, " ")
}

// renderSectionDetail renders detail for a single section.
func renderSectionDetail(section *SectionDiff) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("      %s: %s", section.Name, section.Status))
	if section.Counts != nil {
		b.WriteString(fmt.Sprintf(" (%d → %d, delta %+d)",
			section.Counts.Before,
			section.Counts.After,
			section.Counts.Delta))
	}
	b.WriteString("\n")
	return b.String()
}
