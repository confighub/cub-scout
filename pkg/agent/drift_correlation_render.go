// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"fmt"
	"sort"
	"strings"
)

// CorrelationNarrative generates human-readable narrative explaining
// the relationship between drift findings and failure signals.
//
// This implements the "g" portion of the f(JSON)+g model:
// - Correlation facts come from DriftCorrelation (JSON-derived)
// - Narrative adds explanation, context, and actionable insights
//
// Contract: This function does not invent facts. All statements
// are derived from the correlation structure.
type CorrelationNarrative struct {
	// Summary is a one-line summary of the correlation
	Summary string

	// Explanation provides context for the correlation
	Explanation string

	// Insight is an actionable observation
	Insight string

	// DriftPaths lists the drift paths found (reference to facts)
	DriftPaths []string

	// FailureSignals lists the failure types detected
	FailureSignals []string
}

// RenderCorrelationNarrative generates narrative for a drift-debug correlation.
// The narrative explains what the correlation means without inventing new facts.
func RenderCorrelationNarrative(corr DriftCorrelation) CorrelationNarrative {
	narrative := CorrelationNarrative{}

	// Extract drift paths for reference
	for _, f := range corr.Findings {
		narrative.DriftPaths = append(narrative.DriftPaths, f.Path)
	}

	// Extract failure signal types
	narrative.FailureSignals = extractFailureSignalTypes(corr.Events, corr.Logs)

	// Generate narrative based on correlation state
	switch {
	case corr.HasDrift && corr.HasFailure:
		narrative.Summary = fmt.Sprintf("Object %s has both drift and failure signals", corr.ObjectID)
		narrative.Explanation = formatDriftAndFailureExplanation(corr)
		narrative.Insight = "Drift may have caused or contributed to the failure. Review the drift paths and failure signals together."

	case corr.HasDrift && !corr.HasFailure:
		narrative.Summary = fmt.Sprintf("Object %s has drift but no failure signals", corr.ObjectID)
		narrative.Explanation = formatDriftOnlyExplanation(corr)
		narrative.Insight = "Drift exists but the object is currently healthy. This may be intentional (e.g., manual scaling) or a potential future issue."

	case !corr.HasDrift && corr.HasFailure:
		narrative.Summary = fmt.Sprintf("Object %s has failure signals but no drift", corr.ObjectID)
		narrative.Explanation = formatFailureOnlyExplanation(corr)
		narrative.Insight = "Failure is not caused by configuration drift. Investigate runtime issues, dependencies, or external factors."

	default:
		narrative.Summary = fmt.Sprintf("Object %s has no drift or failure signals", corr.ObjectID)
		narrative.Explanation = "The object matches its desired state and shows no failure signals."
		narrative.Insight = "Object is healthy and correctly configured."
	}

	return narrative
}

// RenderCorrelationASCII renders the correlation narrative as ASCII text.
func RenderCorrelationASCII(corr DriftCorrelation) string {
	narrative := RenderCorrelationNarrative(corr)

	var b strings.Builder

	// Header
	b.WriteString("Drift-Debug Correlation\n")
	b.WriteString(strings.Repeat("─", 50))
	b.WriteString("\n\n")

	// Summary with icon
	var icon string
	if corr.HasDrift && corr.HasFailure {
		icon = "⚠"
	} else if corr.HasFailure {
		icon = "✗"
	} else if corr.HasDrift {
		icon = "△"
	} else {
		icon = "✓"
	}
	b.WriteString(fmt.Sprintf("%s %s\n\n", icon, narrative.Summary))

	// Explanation
	b.WriteString(narrative.Explanation)
	b.WriteString("\n\n")

	// Drift paths (if any)
	if len(narrative.DriftPaths) > 0 {
		b.WriteString("Drift at:\n")
		for _, path := range narrative.DriftPaths {
			b.WriteString(fmt.Sprintf("  • %s\n", path))
		}
		b.WriteString("\n")
	}

	// Failure signals (if any)
	if len(narrative.FailureSignals) > 0 {
		b.WriteString("Failure signals:\n")
		for _, signal := range narrative.FailureSignals {
			b.WriteString(fmt.Sprintf("  • %s\n", signal))
		}
		b.WriteString("\n")
	}

	// Insight
	b.WriteString("💡 ")
	b.WriteString(narrative.Insight)
	b.WriteString("\n")

	return b.String()
}

// RenderCorrelationSummaryLine returns a one-line summary for the correlation.
// Useful for inline display in debug output.
func RenderCorrelationSummaryLine(corr DriftCorrelation) string {
	switch {
	case corr.HasDrift && corr.HasFailure:
		return fmt.Sprintf("⚠ %d drift finding(s) + %d failure signal(s) — may be related",
			len(corr.Findings), countFailureSignals(corr.Events, corr.Logs))
	case corr.HasDrift && !corr.HasFailure:
		return fmt.Sprintf("△ %d drift finding(s), no failures — drift may be intentional",
			len(corr.Findings))
	case !corr.HasDrift && corr.HasFailure:
		return fmt.Sprintf("✗ %d failure signal(s), no drift — runtime issue",
			countFailureSignals(corr.Events, corr.Logs))
	default:
		return "✓ No drift or failures"
	}
}

// formatDriftAndFailureExplanation formats explanation for drift + failure case.
func formatDriftAndFailureExplanation(corr DriftCorrelation) string {
	var parts []string

	// Describe drift
	driftCount := len(corr.Findings)
	if driftCount == 1 {
		parts = append(parts, fmt.Sprintf("Found 1 configuration difference at: %s", corr.Findings[0].Path))
	} else {
		paths := make([]string, 0, min(3, len(corr.Findings)))
		for i := 0; i < len(corr.Findings) && i < 3; i++ {
			paths = append(paths, corr.Findings[i].Path)
		}
		pathList := strings.Join(paths, ", ")
		if driftCount > 3 {
			pathList += fmt.Sprintf(" (+%d more)", driftCount-3)
		}
		parts = append(parts, fmt.Sprintf("Found %d configuration differences at: %s", driftCount, pathList))
	}

	// Describe failures
	failureCount := countFailureSignals(corr.Events, corr.Logs)
	warningEvents := countWarningEvents(corr.Events)
	logErrors := countLogErrors(corr.Logs)

	failureParts := []string{}
	if warningEvents > 0 {
		failureParts = append(failureParts, fmt.Sprintf("%d warning event(s)", warningEvents))
	}
	if logErrors > 0 {
		failureParts = append(failureParts, fmt.Sprintf("%d log error(s)", logErrors))
	}
	if len(failureParts) > 0 {
		parts = append(parts, fmt.Sprintf("Detected %d failure signal(s): %s", failureCount, strings.Join(failureParts, ", ")))
	}

	return strings.Join(parts, ". ") + "."
}

// formatDriftOnlyExplanation formats explanation for drift-only case.
func formatDriftOnlyExplanation(corr DriftCorrelation) string {
	driftCount := len(corr.Findings)

	// Categorize drift by classification
	classifications := make(map[DriftClassification]int)
	for _, f := range corr.Findings {
		classifications[f.Classification]++
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("Found %d configuration difference(s)", driftCount))

	// Add classification breakdown if diverse (sorted for determinism)
	if len(classifications) > 1 {
		classStrs := []string{}
		for class, count := range classifications {
			classStrs = append(classStrs, fmt.Sprintf("%d %s", count, class))
		}
		sort.Strings(classStrs)
		parts = append(parts, fmt.Sprintf("Categories: %s", strings.Join(classStrs, ", ")))
	}

	parts = append(parts, "No warning events or error patterns detected in logs/events")

	return strings.Join(parts, ". ") + "."
}

// formatFailureOnlyExplanation formats explanation for failure-only case.
func formatFailureOnlyExplanation(corr DriftCorrelation) string {
	failureCount := countFailureSignals(corr.Events, corr.Logs)
	signalTypes := extractFailureSignalTypes(corr.Events, corr.Logs)

	var parts []string
	parts = append(parts, fmt.Sprintf("Detected %d failure signal(s)", failureCount))

	if len(signalTypes) > 0 {
		parts = append(parts, fmt.Sprintf("Types: %s", strings.Join(signalTypes, ", ")))
	}

	parts = append(parts, "Object configuration matches desired state (no drift)")

	return strings.Join(parts, ". ") + "."
}

// extractFailureSignalTypes returns a deduplicated list of failure signal types.
func extractFailureSignalTypes(events []TimelineEvent, logs []ContainerLogResult) []string {
	types := make(map[string]bool)

	for _, e := range events {
		if e.Type == "Warning" {
			types[e.Reason] = true
		}
		if e.Severity == "error" || e.Severity == "warning" {
			types[e.Reason] = true
		}
	}

	for _, l := range logs {
		if l.Error != "" {
			types["log-error"] = true
		}
		for _, p := range l.Patterns {
			if p.Type == "error" || p.Type == "panic" || p.Type == "fatal" {
				types[p.Type] = true
			}
		}
	}

	result := make([]string, 0, len(types))
	for t := range types {
		result = append(result, t)
	}
	sort.Strings(result) // Deterministic ordering
	return result
}

// countFailureSignals counts total failure signals from events and logs.
func countFailureSignals(events []TimelineEvent, logs []ContainerLogResult) int {
	count := 0
	count += countWarningEvents(events)
	count += countLogErrors(logs)
	return count
}

// countWarningEvents counts warning/error events.
func countWarningEvents(events []TimelineEvent) int {
	count := 0
	for _, e := range events {
		if e.Type == "Warning" || e.Severity == "error" || e.Severity == "warning" {
			count++
		}
	}
	return count
}

// countLogErrors counts error patterns in logs.
func countLogErrors(logs []ContainerLogResult) int {
	count := 0
	for _, l := range logs {
		if l.Error != "" {
			count++
		}
		for _, p := range l.Patterns {
			if p.Type == "error" || p.Type == "panic" || p.Type == "fatal" {
				count++
			}
		}
	}
	return count
}
