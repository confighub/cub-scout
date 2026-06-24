// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

// symbols.go defines the canonical visual vocabulary for cub-scout.
//
// All CLI and TUI output MUST use these symbols. This ensures:
// - Consistent visual language across all views
// - Predictable snapshot testing
// - CLI ↔ TUI symmetry
//
// Symbol Categories:
// - Status: health/readiness indicators (✓, ✗, ⚠, ⏸)
// - Activity: running/stopped states (●, ○)
// - Tree: hierarchy visualization (▼, ▶, ├─, └─, │)
// - Navigation: directional indicators (→, ←)

// =============================================================================
// Status Symbols — health and readiness
// =============================================================================

const (
	// SymOK indicates success, healthy, ready, or passing state.
	// Use for: healthy pods, passing checks, successful operations
	SymOK = "✓"

	// SymError indicates failure, unhealthy, or error state.
	// Use for: failed pods, errors, broken deployers
	SymError = "✗"

	// SymWarning indicates degraded, risky, or attention-needed state.
	// Use for: drift detected, suspended resources, potential issues
	SymWarning = "⚠"

	// SymPaused indicates intentionally stopped or suspended state.
	// Use for: suspended Kustomizations, paused reconciliation
	SymPaused = "⏸"

	// SymUnknown indicates unknown or indeterminate state.
	// Use for: resources without status, missing data
	SymUnknown = "?"
)

// =============================================================================
// Activity Symbols — running vs stopped
// =============================================================================

const (
	// SymActive indicates active, running, or enabled state.
	// Use for: active context, running workers, enabled features
	SymActive = "●"

	// SymInactive indicates inactive, stopped, or disabled state.
	// Use for: inactive context, stopped workers, disabled features
	SymInactive = "○"
)

// =============================================================================
// Tree Symbols — hierarchy and structure
// =============================================================================

const (
	// SymExpanded indicates an expanded/open tree node.
	SymExpanded = "▼"

	// SymCollapsed indicates a collapsed/closed tree node.
	SymCollapsed = "▶"

	// SymBranch indicates a tree branch (has siblings below).
	SymBranch = "├─"

	// SymLast indicates the last item in a tree level.
	SymLast = "└─"

	// SymPipe is a vertical continuation line in tree views.
	SymPipe = "│ "

	// SymIndent is spacing for tree indentation.
	SymIndent = "  "
)

// =============================================================================
// Navigation Symbols — directional indicators
// =============================================================================

const (
	// SymArrowRight indicates direction, flow, or mapping.
	// Use for: ownership (A → B owns), data flow, transitions
	SymArrowRight = "→"

	// SymArrowLeft indicates reverse direction or source.
	SymArrowLeft = "←"

	// SymBullet is a neutral list item marker.
	SymBullet = "•"
)

// =============================================================================
// Semantic Symbol Functions — use these for consistent meaning
// =============================================================================

// StatusSymbol returns the appropriate symbol for a boolean status.
// true = OK (✓), false = Error (✗)
func StatusSymbol(ok bool) string {
	if ok {
		return SymOK
	}
	return SymError
}

// ReadySymbol returns the symbol for readiness state.
// ready=true → ✓, ready=false → ✗
func ReadySymbol(ready bool) string {
	return StatusSymbol(ready)
}

// HealthSymbol returns the symbol for health state.
// healthy=true → ✓, healthy=false → ✗
func HealthSymbol(healthy bool) string {
	return StatusSymbol(healthy)
}

// ActiveSymbol returns the symbol for activity state.
// active=true → ●, active=false → ○
func ActiveSymbol(active bool) string {
	if active {
		return SymActive
	}
	return SymInactive
}

// SuspendedSymbol returns ⏸ if suspended, ✓ if not.
func SuspendedSymbol(suspended bool) string {
	if suspended {
		return SymPaused
	}
	return SymOK
}

// TreeSymbol returns SymBranch or SymLast based on position.
func TreeSymbol(isLast bool) string {
	if isLast {
		return SymLast
	}
	return SymBranch
}

// ExpandSymbol returns SymExpanded or SymCollapsed based on state.
func ExpandSymbol(expanded bool) string {
	if expanded {
		return SymExpanded
	}
	return SymCollapsed
}

// =============================================================================
// Severity Ordering — canonical sort order for issues
// =============================================================================

// SeverityOrder defines the canonical ordering for severity levels.
// Lower number = higher priority (shown first).
var SeverityOrder = map[string]int{
	"critical": 0,
	"high":     1,
	"medium":   2,
	"warning":  3,
	"low":      4,
	"info":     5,
	"unknown":  6,
}

// CompareSeverity returns true if a should sort before b.
func CompareSeverity(a, b string) bool {
	orderA, okA := SeverityOrder[a]
	orderB, okB := SeverityOrder[b]
	if !okA {
		orderA = 99 // unknown goes last
	}
	if !okB {
		orderB = 99
	}
	return orderA < orderB
}

// =============================================================================
// Owner Ordering — canonical sort order for ownership types
// =============================================================================

// OwnerOrder defines the canonical ordering for owner types.
// GitOps tools first, then Helm, then Native (unmanaged) last.
var OwnerOrder = map[string]int{
	"Flux":       0,
	"ArgoCD":     1,
	"Sveltos":    2,
	"Modelplane": 3,
	"Crossplane": 4,
	"kro":        5,
	"Helm":       6,
	"Terraform":  7,
	"ConfigHub":  8,
	"Native":     9,
	"Unknown":    10,
}

// CompareOwner returns true if a should sort before b.
func CompareOwner(a, b string) bool {
	orderA, okA := OwnerOrder[a]
	orderB, okB := OwnerOrder[b]
	if !okA {
		orderA = 99
	}
	if !okB {
		orderB = 99
	}
	return orderA < orderB
}
