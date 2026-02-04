// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package agent provides drift detection rendering for v0.14.3.
//
// ASCII renderer implements f(JSON) + g.
// Structural facts must already exist in DriftReport.
// Narrative semantics (ordering, grouping, labels) must not introduce new facts.
// See docs/semantic-contract.md and .github/SEMANTIC-SAFETY.md.
//
// Contract: R1 (facts from JSON), R3 (narrative is additive), R4 (no meaning by placement).
package agent

import (
	"fmt"
	"sort"
	"strings"
)

// RenderDriftASCII renders a DriftReport as human-readable ASCII.
//
// This implements f(JSON) + g where:
//   - f: Every DriftFinding is rendered exactly once with all structural facts
//   - g: Grouping, ordering, icons, and labels are narrative semantics
//
// Ordering rules (documented for reviewability):
//  1. Group by classification
//  2. Order groups by max severity in group (critical > warning > info)
//  3. Within groups, order findings by: severity desc, object_id asc, path asc
func RenderDriftASCII(report DriftReport) string {
	var sb strings.Builder

	// Header (narrative: context summary)
	sb.WriteString("Drift Report\n")
	sb.WriteString("============\n\n")
	sb.WriteString(fmt.Sprintf("Cluster: %s\n", report.Context.Cluster))
	sb.WriteString(fmt.Sprintf("Source:  %s (%s)\n", report.Context.DesiredSource.Ref, report.Context.DesiredSource.Type))
	if report.Context.Namespace != nil {
		sb.WriteString(fmt.Sprintf("Namespace: %s\n", *report.Context.Namespace))
	}
	sb.WriteString("\n")

	// No findings case
	if len(report.Findings) == 0 {
		sb.WriteString("No drift detected.\n")
		return sb.String()
	}

	// Group findings by classification (structural fact)
	groups := groupByClassification(report.Findings)

	// Order groups by max severity (derived from JSON facts)
	orderedClassifications := orderGroupsBySeverity(groups)

	// Render each group
	for i, classification := range orderedClassifications {
		findings := groups[classification]

		// Group heading (narrative: icon + label)
		icon := classificationIcon(classification)
		label := classificationLabel(classification)
		sb.WriteString(fmt.Sprintf("%s %s\n", icon, label))

		// Sort findings within group: severity desc, object_id asc, path asc
		sortFindingsForDisplay(findings)

		// Render each finding in group
		for j, f := range findings {
			isLast := j == len(findings)-1
			renderFinding(&sb, f, isLast)
		}

		// Spacing between groups (narrative)
		if i < len(orderedClassifications)-1 {
			sb.WriteString("\n")
		}
	}

	// Summary footer (structural facts from JSON)
	sb.WriteString("\n")
	sb.WriteString(renderSummary(report.Summary))

	return sb.String()
}

// groupByClassification groups findings by their classification field.
func groupByClassification(findings []DriftFinding) map[DriftClassification][]DriftFinding {
	groups := make(map[DriftClassification][]DriftFinding)
	for _, f := range findings {
		groups[f.Classification] = append(groups[f.Classification], f)
	}
	return groups
}

// orderGroupsBySeverity orders classifications by max severity in each group.
// This derives ordering from JSON facts (severity field), not narrative invention.
func orderGroupsBySeverity(groups map[DriftClassification][]DriftFinding) []DriftClassification {
	type classWithSeverity struct {
		classification DriftClassification
		maxSeverity    int
	}

	var items []classWithSeverity
	for class, findings := range groups {
		maxSev := 0
		for _, f := range findings {
			if sev := severityRankForSort(f.Severity); sev > maxSev {
				maxSev = sev
			}
		}
		items = append(items, classWithSeverity{class, maxSev})
	}

	// Sort: max severity desc, then classification name for stability
	sort.Slice(items, func(i, j int) bool {
		if items[i].maxSeverity != items[j].maxSeverity {
			return items[i].maxSeverity > items[j].maxSeverity
		}
		return items[i].classification < items[j].classification
	})

	result := make([]DriftClassification, len(items))
	for i, item := range items {
		result[i] = item.classification
	}
	return result
}

// sortFindingsForDisplay sorts findings for human-friendly display.
// Order: severity desc, object_id asc, path asc (same as JSON canonical order).
func sortFindingsForDisplay(findings []DriftFinding) {
	sort.Slice(findings, func(i, j int) bool {
		// Primary: severity (descending)
		si := severityRankForSort(findings[i].Severity)
		sj := severityRankForSort(findings[j].Severity)
		if si != sj {
			return si > sj
		}
		// Secondary: object_id (ascending)
		if findings[i].ObjectID != findings[j].ObjectID {
			return findings[i].ObjectID < findings[j].ObjectID
		}
		// Tertiary: path (ascending)
		return findings[i].Path < findings[j].Path
	})
}

// severityRankForSort returns a numeric rank for sorting.
func severityRankForSort(s DriftSeverity) int {
	switch s {
	case DriftSeverityCritical:
		return 3
	case DriftSeverityWarning:
		return 2
	case DriftSeverityInfo:
		return 1
	}
	return 0
}

// renderFinding renders a single finding with tree connectors.
func renderFinding(sb *strings.Builder, f DriftFinding, isLast bool) {
	// Tree connector (narrative: visual hierarchy)
	connector := "├─"
	if isLast {
		connector = "└─"
	}
	indent := "│  "
	if isLast {
		indent = "   "
	}

	// Severity indicator (narrative: derived from JSON severity field)
	sevIcon := severityIcon(f.Severity)

	// Object identity (structural: from JSON)
	sb.WriteString(fmt.Sprintf("%s %s %s\n", connector, sevIcon, f.ObjectID))

	// Path (structural: from JSON)
	sb.WriteString(fmt.Sprintf("%s   path: %s\n", indent, f.Path))

	// Desired value (structural: from JSON)
	sb.WriteString(fmt.Sprintf("%s   desired: %v\n", indent, formatValue(f.Desired)))

	// Live value (structural: from JSON)
	sb.WriteString(fmt.Sprintf("%s   live:    %v\n", indent, formatValue(f.Live)))
}

// renderSummary renders the summary section.
func renderSummary(summary DriftSummary) string {
	var sb strings.Builder
	sb.WriteString("Summary\n")
	sb.WriteString("-------\n")
	sb.WriteString(fmt.Sprintf("Total findings: %d\n", summary.TotalFindings))
	sb.WriteString(fmt.Sprintf("Affected objects: %d\n", summary.AffectedObjects))

	// Severity breakdown (from JSON)
	if len(summary.BySeverity) > 0 {
		sb.WriteString("By severity: ")
		parts := make([]string, 0, len(summary.BySeverity))
		for _, s := range summary.BySeverity {
			parts = append(parts, fmt.Sprintf("%s=%d", s.Severity, s.Count))
		}
		sb.WriteString(strings.Join(parts, ", "))
		sb.WriteString("\n")
	}

	return sb.String()
}

// Narrative semantics helpers (g layer)

// classificationIcon returns an icon for the classification (narrative).
// Icons are narrative semantics - they make the output human-friendly
// but do not introduce new structural facts.
func classificationIcon(c DriftClassification) string {
	switch c {
	case DriftCapacity:
		return "[Capacity]"
	case DriftImage:
		return "[Image]"
	case DriftConfig:
		return "[Config]"
	case DriftResource:
		return "[Resources]"
	case DriftRollout:
		return "[Rollout]"
	case DriftHealth:
		return "[Health]"
	case DriftLabel:
		return "[Labels]"
	case DriftAnnotation:
		return "[Annotations]"
	default:
		return "[Other]"
	}
}

// classificationLabel returns a human-friendly label for the classification.
func classificationLabel(c DriftClassification) string {
	switch c {
	case DriftCapacity:
		return "Capacity Drift"
	case DriftImage:
		return "Image Drift"
	case DriftConfig:
		return "Configuration Drift"
	case DriftResource:
		return "Resource Drift"
	case DriftRollout:
		return "Rollout Drift"
	case DriftHealth:
		return "Health Check Drift"
	case DriftLabel:
		return "Label Drift"
	case DriftAnnotation:
		return "Annotation Drift"
	default:
		return "Other Drift"
	}
}

// severityIcon returns an icon for severity (narrative).
// This is derived from the JSON severity field, not invented.
func severityIcon(s DriftSeverity) string {
	switch s {
	case DriftSeverityCritical:
		return "[CRITICAL]"
	case DriftSeverityWarning:
		return "[WARNING]"
	case DriftSeverityInfo:
		return "[INFO]"
	}
	return ""
}

// formatValue formats a value for display.
func formatValue(v interface{}) string {
	if v == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%v", v)
}
