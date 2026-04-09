// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/confighub/cub-scout/pkg/hub"
)

// ThreeWayPattern names the disagreement pattern detected.
type ThreeWayPattern string

const (
	// PatternAgreed means all three layers agree.
	PatternAgreed ThreeWayPattern = "agreed"

	// PatternChangeInProgress means ConfigHub differs from cluster but controller is syncing.
	PatternChangeInProgress ThreeWayPattern = "change-in-progress"

	// PatternSyncStale means ConfigHub differs from cluster but controller thinks it's synced.
	PatternSyncStale ThreeWayPattern = "sync-stale"

	// PatternRolloutPending means controller applied but cluster hasn't caught up.
	PatternRolloutPending ThreeWayPattern = "rollout-pending"

	// PatternMultiChange means all three layers disagree.
	PatternMultiChange ThreeWayPattern = "multi-change"

	// PatternUnknown means we couldn't determine a specific pattern.
	PatternUnknown ThreeWayPattern = "unknown"

	// PatternDisconnected means ConfigHub is not connected (can't compare).
	PatternDisconnected ThreeWayPattern = "disconnected"

	// PatternUnlinked means resource is not linked to ConfigHub unit.
	PatternUnlinked ThreeWayPattern = "unlinked"
)

// ThreeWayDisagreement represents detected disagreement between ConfigHub, controller, and cluster.
type ThreeWayDisagreement struct {
	Resource  string          `json:"resource"`
	Namespace string          `json:"namespace,omitempty"`
	Pattern   ThreeWayPattern `json:"pattern"`
	Meaning   string          `json:"meaning"`

	// State summaries for each layer
	ConfigHubState  string `json:"confighubState,omitempty"`  // What ConfigHub says (DRY/WET)
	ControllerState string `json:"controllerState,omitempty"` // What Argo/Flux says
	ClusterState    string `json:"clusterState,omitempty"`    // What cluster says (LIVE)

	// Sync details (when available)
	SyncStatus   string `json:"syncStatus,omitempty"`   // Argo: Synced/OutOfSync/Unknown
	HealthStatus string `json:"healthStatus,omitempty"` // Argo: Healthy/Degraded/Missing

	// Field-level mismatches
	FieldMismatches []ThreeWayFieldMismatch `json:"fieldMismatches,omitempty"`
}

// ThreeWayFieldMismatch shows divergence for a specific field.
type ThreeWayFieldMismatch struct {
	Field     string `json:"field"`
	ConfigHub string `json:"confighub,omitempty"`
	Cluster   string `json:"cluster"`
}

// AgreementState represents the overall convergence state across multiple resources.
type AgreementState string

const (
	// StateAgreed means all resources are aligned across all three layers.
	StateAgreed AgreementState = "agreed"

	// StateConverging means changes are in progress but expected to converge.
	StateConverging AgreementState = "converging"

	// StateDiverged means there are stale or unexplained disagreements.
	StateDiverged AgreementState = "diverged"

	// StatePartial means evidence is incomplete (disconnected or unlinked resources).
	StatePartial AgreementState = "partial"
)

// AgreementSummary provides a high-level convergence assessment across a scope.
type AgreementSummary struct {
	State   AgreementState `json:"state"`
	Summary string         `json:"summary"`
	Reasons []string       `json:"reasons,omitempty"`
	Sources SourceCoverage `json:"sources"`
}

// SourceCoverage tracks which layers have usable evidence.
type SourceCoverage struct {
	ConfigHub int `json:"confighub"` // Resources with DRY/WET evidence
	Deployer  int `json:"deployer"`  // Resources with controller evidence
	Cluster   int `json:"cluster"`   // Resources with LIVE evidence
	Total     int `json:"total"`     // Total resources in scope
}

// IsDisagreement returns true if the pattern indicates a disagreement.
func (d ThreeWayDisagreement) IsDisagreement() bool {
	return d.Pattern != PatternAgreed &&
		d.Pattern != PatternDisconnected &&
		d.Pattern != PatternUnlinked
}

// buildThreeWayDisagreement checks for three-way disagreement for a resource.
// Returns nil if no disagreement or if not connected/linked.
func buildThreeWayDisagreement(
	ctx context.Context,
	kind, name, namespace string,
	failureDetails *agent.FailureDetails,
) (*ThreeWayDisagreement, error) {
	// Check connected mode
	client := hub.NewClient()
	if err := client.RequireConnected(); err != nil {
		return &ThreeWayDisagreement{
			Resource:  kind + "/" + name,
			Namespace: namespace,
			Pattern:   PatternDisconnected,
			Meaning:   "Connect to ConfigHub to unlock three-way comparison.",
		}, nil
	}

	// Use existing compare infrastructure to get DRY/WET/LIVE
	result, err := buildCompareResourceResult(ctx, kind+"/"+name, namespace)
	if err != nil {
		return nil, fmt.Errorf("three-way comparison failed: %w", err)
	}

	// Not linked to ConfigHub unit
	if result.Dry == nil && result.Wet == nil {
		return &ThreeWayDisagreement{
			Resource:  kind + "/" + name,
			Namespace: namespace,
			Pattern:   PatternUnlinked,
			Meaning:   "Resource is not linked to a ConfigHub unit.",
		}, nil
	}

	// Extract sync state from failure details
	syncStatus := ""
	healthStatus := ""
	if failureDetails != nil {
		syncStatus = failureDetails.SyncStatus
		healthStatus = failureDetails.HealthStatus
	}

	// Classify the disagreement pattern
	return classifyThreeWayPattern(result, syncStatus, healthStatus), nil
}

// classifyThreeWayPattern determines the disagreement pattern from comparison result.
func classifyThreeWayPattern(
	result compareResourceResult,
	syncStatus, healthStatus string,
) *ThreeWayDisagreement {
	d := &ThreeWayDisagreement{
		Resource:     result.Resource,
		Namespace:    result.Namespace,
		SyncStatus:   syncStatus,
		HealthStatus: healthStatus,
	}

	// Build state summaries
	d.ConfigHubState = buildConfigHubStateSummary(result.Dry, result.Wet)
	d.ClusterState = buildClusterStateSummary(result.Live)
	d.ControllerState = buildControllerStateSummary(syncStatus, healthStatus)

	// Extract field mismatches
	d.FieldMismatches = extractFieldMismatches(result.Mismatches)

	// No mismatches = agreed
	if len(result.Mismatches) == 0 {
		d.Pattern = PatternAgreed
		d.Meaning = "ConfigHub, controller, and cluster state agree."
		return d
	}

	// Classify based on sync state and mismatch pattern
	syncLower := strings.ToLower(strings.TrimSpace(syncStatus))
	isSyncing := syncLower == "syncing" || syncLower == "progressing" || syncLower == "outofsync"
	isSynced := syncLower == "synced" || syncLower == ""

	// Check if ConfigHub (WET) differs from cluster (LIVE)
	hasConfigHubClusterMismatch := hasWetLiveMismatch(result.Mismatches)

	if hasConfigHubClusterMismatch {
		if isSyncing {
			d.Pattern = PatternChangeInProgress
			d.Meaning = "ConfigHub change is being applied. Controller is syncing - wait for completion."
		} else if isSynced {
			d.Pattern = PatternSyncStale
			d.Meaning = "ConfigHub and cluster disagree but controller reports synced. OCI content may be stale or controller applied different content."
		} else {
			d.Pattern = PatternUnknown
			d.Meaning = "ConfigHub and cluster disagree. Check controller status."
		}
		return d
	}

	// ConfigHub matches something but there are still mismatches (DRY vs WET vs LIVE)
	if hasDryWetMismatch(result.Mismatches) {
		d.Pattern = PatternRolloutPending
		d.Meaning = "Controller has applied changes but cluster hasn't fully rolled out yet."
		return d
	}

	// Multiple mismatches across all three
	if len(result.Mismatches) > 0 {
		d.Pattern = PatternMultiChange
		d.Meaning = "Multiple changes in flight or inconsistent state. Check all three layers."
	} else {
		d.Pattern = PatternAgreed
		d.Meaning = "All three layers agree."
	}

	return d
}

// buildConfigHubStateSummary creates a human-readable summary of ConfigHub state.
func buildConfigHubStateSummary(dry, wet *compareSideSummary) string {
	if wet != nil && len(wet.Images) > 0 {
		return fmt.Sprintf("image=%s", wet.Images[0])
	}
	if dry != nil && len(dry.Images) > 0 {
		return fmt.Sprintf("image=%s (intent)", dry.Images[0])
	}
	if wet != nil && wet.Replicas != nil {
		return fmt.Sprintf("replicas=%d", *wet.Replicas)
	}
	return "available"
}

// buildClusterStateSummary creates a human-readable summary of cluster state.
func buildClusterStateSummary(live compareSideSummary) string {
	if len(live.Images) > 0 {
		return fmt.Sprintf("image=%s", live.Images[0])
	}
	if live.Replicas != nil {
		return fmt.Sprintf("replicas=%d", *live.Replicas)
	}
	return "running"
}

// buildControllerStateSummary creates a human-readable summary of controller state.
func buildControllerStateSummary(syncStatus, healthStatus string) string {
	parts := make([]string, 0, 2)
	if syncStatus != "" {
		parts = append(parts, syncStatus)
	}
	if healthStatus != "" {
		parts = append(parts, healthStatus)
	}
	if len(parts) == 0 {
		return "unknown"
	}
	return strings.Join(parts, ", ")
}

// extractFieldMismatches converts compareFieldMismatch to ThreeWayFieldMismatch.
func extractFieldMismatches(mismatches []compareFieldMismatch) []ThreeWayFieldMismatch {
	result := make([]ThreeWayFieldMismatch, 0, len(mismatches))
	for _, m := range mismatches {
		// Use WET as ConfigHub state (rendered), fall back to DRY
		confighub := m.Wet
		if confighub == "" {
			confighub = m.Dry
		}
		result = append(result, ThreeWayFieldMismatch{
			Field:     m.Field,
			ConfigHub: confighub,
			Cluster:   m.Live,
		})
	}
	return result
}

// hasWetLiveMismatch checks if there's a mismatch between WET (ConfigHub rendered) and LIVE.
func hasWetLiveMismatch(mismatches []compareFieldMismatch) bool {
	for _, m := range mismatches {
		if m.Wet != "" && m.Live != "" && m.Wet != m.Live {
			return true
		}
	}
	return false
}

// hasDryWetMismatch checks if there's a mismatch between DRY (intent) and WET (rendered).
func hasDryWetMismatch(mismatches []compareFieldMismatch) bool {
	for _, m := range mismatches {
		if m.Dry != "" && m.Wet != "" && m.Dry != m.Wet {
			return true
		}
	}
	return false
}

// formatThreeWaySection formats the three-way status for ASCII output.
func formatThreeWaySection(d *ThreeWayDisagreement) string {
	if d == nil {
		return ""
	}

	// Don't show section for non-disagreements
	if !d.IsDisagreement() {
		return ""
	}

	var b strings.Builder
	b.WriteString("\nTHREE-WAY STATUS:\n")

	// Show warning symbol and pattern
	b.WriteString(fmt.Sprintf("  %s %s\n", Yellow("\u26a0"), patternToLabel(d.Pattern)))

	// Show state comparison
	if d.ConfigHubState != "" {
		b.WriteString(fmt.Sprintf("    ConfigHub: %s\n", d.ConfigHubState))
	}
	if d.ControllerState != "" && d.ControllerState != "unknown" {
		b.WriteString(fmt.Sprintf("    Controller: %s\n", d.ControllerState))
	}
	if d.ClusterState != "" {
		b.WriteString(fmt.Sprintf("    Cluster: %s\n", d.ClusterState))
	}

	// Show field-level mismatches if any
	if len(d.FieldMismatches) > 0 {
		b.WriteString("  Mismatched fields:\n")
		for _, m := range d.FieldMismatches {
			b.WriteString(fmt.Sprintf("    - %s: ConfigHub=%s, Cluster=%s\n", m.Field, m.ConfigHub, m.Cluster))
		}
	}

	// Show meaning/recommendation
	b.WriteString(fmt.Sprintf("  -> %s\n", d.Meaning))

	return b.String()
}

// formatThreeWayMarkdown formats the three-way status for markdown output.
func formatThreeWayMarkdown(d *ThreeWayDisagreement) string {
	if d == nil {
		return ""
	}

	// Don't show section for non-disagreements
	if !d.IsDisagreement() {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n### Three-Way Status\n\n")
	b.WriteString(fmt.Sprintf("**%s**\n\n", patternToLabel(d.Pattern)))

	// Show state comparison
	if d.ConfigHubState != "" {
		b.WriteString(fmt.Sprintf("- **ConfigHub:** %s\n", d.ConfigHubState))
	}
	if d.ControllerState != "" && d.ControllerState != "unknown" {
		b.WriteString(fmt.Sprintf("- **Controller:** %s\n", d.ControllerState))
	}
	if d.ClusterState != "" {
		b.WriteString(fmt.Sprintf("- **Cluster:** %s\n", d.ClusterState))
	}

	// Show field-level mismatches if any
	if len(d.FieldMismatches) > 0 {
		b.WriteString("\n**Mismatched fields:**\n")
		for _, m := range d.FieldMismatches {
			b.WriteString(fmt.Sprintf("- `%s`: ConfigHub=%s, Cluster=%s\n", m.Field, m.ConfigHub, m.Cluster))
		}
	}

	// Show meaning/recommendation
	b.WriteString(fmt.Sprintf("\n> %s\n", d.Meaning))

	return b.String()
}

// patternToLabel returns a human-readable label for the pattern.
func patternToLabel(p ThreeWayPattern) string {
	switch p {
	case PatternAgreed:
		return "All layers agree"
	case PatternChangeInProgress:
		return "Change in progress"
	case PatternSyncStale:
		return "Sync/state mismatch"
	case PatternRolloutPending:
		return "Rollout pending"
	case PatternMultiChange:
		return "Multiple disagreements"
	case PatternDisconnected:
		return "Not connected to ConfigHub"
	case PatternUnlinked:
		return "Not linked to ConfigHub unit"
	default:
		return "Unknown state"
	}
}

// deriveAgreementSummary computes the overall agreement state from per-resource patterns.
// This is the single source of truth for convergence assessment.
func deriveAgreementSummary(patternCounts map[ThreeWayPattern]int, sources SourceCoverage) AgreementSummary {
	total := sources.Total
	if total == 0 {
		return AgreementSummary{
			State:   StatePartial,
			Summary: "No resources in scope",
			Sources: sources,
		}
	}

	// Count by category
	agreed := patternCounts[PatternAgreed]
	converging := patternCounts[PatternChangeInProgress] + patternCounts[PatternRolloutPending]
	diverged := patternCounts[PatternSyncStale] + patternCounts[PatternMultiChange] + patternCounts[PatternUnknown]
	partial := patternCounts[PatternDisconnected] + patternCounts[PatternUnlinked]

	var reasons []string

	// Determine overall state (priority: partial > diverged > converging > agreed)
	var state AgreementState
	var summary string

	switch {
	case partial > 0 && partial >= total/2:
		// Majority missing evidence
		state = StatePartial
		summary = fmt.Sprintf("%d/%d resources have incomplete evidence", partial, total)
		if patternCounts[PatternDisconnected] > 0 {
			reasons = append(reasons, fmt.Sprintf("%d disconnected from ConfigHub", patternCounts[PatternDisconnected]))
		}
		if patternCounts[PatternUnlinked] > 0 {
			reasons = append(reasons, fmt.Sprintf("%d not linked to ConfigHub units", patternCounts[PatternUnlinked]))
		}

	case diverged > 0:
		// Any true divergence takes priority
		state = StateDiverged
		summary = fmt.Sprintf("%d/%d resources diverged", diverged, total)
		if patternCounts[PatternSyncStale] > 0 {
			reasons = append(reasons, fmt.Sprintf("%d have stale sync (controller claims synced but cluster differs)", patternCounts[PatternSyncStale]))
		}
		if patternCounts[PatternMultiChange] > 0 {
			reasons = append(reasons, fmt.Sprintf("%d have multiple disagreements", patternCounts[PatternMultiChange]))
		}
		if patternCounts[PatternUnknown] > 0 {
			reasons = append(reasons, fmt.Sprintf("%d have unknown divergence", patternCounts[PatternUnknown]))
		}

	case converging > 0:
		// Changes in progress
		state = StateConverging
		summary = fmt.Sprintf("%d/%d resources converging", converging, total)
		if patternCounts[PatternChangeInProgress] > 0 {
			reasons = append(reasons, fmt.Sprintf("%d changes in progress (controller syncing)", patternCounts[PatternChangeInProgress]))
		}
		if patternCounts[PatternRolloutPending] > 0 {
			reasons = append(reasons, fmt.Sprintf("%d rollouts pending (applied, awaiting cluster)", patternCounts[PatternRolloutPending]))
		}

	default:
		// All agreed
		state = StateAgreed
		summary = fmt.Sprintf("All %d resources agree", agreed)
	}

	// Add partial info as secondary reason if not the primary state
	if state != StatePartial && partial > 0 {
		reasons = append(reasons, fmt.Sprintf("%d resources have incomplete evidence", partial))
	}

	return AgreementSummary{
		State:   state,
		Summary: summary,
		Reasons: reasons,
		Sources: sources,
	}
}

// stateToIcon returns a compact icon for the agreement state.
func stateToIcon(s AgreementState) string {
	switch s {
	case StateAgreed:
		return "\u2713" // ✓
	case StateConverging:
		return "\u2192" // →
	case StateDiverged:
		return "\u2717" // ✗
	case StatePartial:
		return "?"
	default:
		return "?"
	}
}

// stateToLabel returns a human-readable label for the agreement state.
func stateToLabel(s AgreementState) string {
	switch s {
	case StateAgreed:
		return "AGREED"
	case StateConverging:
		return "CONVERGING"
	case StateDiverged:
		return "DIVERGED"
	case StatePartial:
		return "PARTIAL"
	default:
		return "UNKNOWN"
	}
}
