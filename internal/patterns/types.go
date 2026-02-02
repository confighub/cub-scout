// Package patterns provides the pattern detection engine for cub-scout v0.7+.
//
// Patterns are deterministic checks that analyze the resource graph and report
// findings. Each pattern has a unique ID, description, and detection logic.
//
// This is a v0.7 contract surface that does not modify any v0.5 or v0.6 contracts.
package patterns

import (
	"github.com/confighub/cub-scout/internal/gitctx"
	"github.com/confighub/cub-scout/internal/graph"
)

// SchemaVersion is the patterns output schema version.
const SchemaVersion = "patterns.v1"

// Status represents the result of a pattern detection.
type Status string

const (
	// StatusPass indicates the pattern check passed.
	StatusPass Status = "pass"

	// StatusFail indicates the pattern check failed (findings detected).
	StatusFail Status = "fail"

	// StatusSkip indicates the pattern was skipped (not applicable).
	StatusSkip Status = "skip"
)

// DetectContext provides the execution context for pattern detection (v0.10+).
// This combines the resource graph with optional git repository context.
type DetectContext struct {
	// Graph is the resource graph (always present).
	Graph *graph.Graph

	// Git is the optional git repository context (nil when --git-root not provided).
	// Patterns should check for nil before using.
	Git *gitctx.GitContext
}

// Pattern represents a registered pattern.
type Pattern struct {
	// ID is the unique pattern identifier (e.g., "k8s.ownership_chain_complete").
	ID string `json:"id"`

	// Name is the human-readable pattern name.
	Name string `json:"name"`

	// Description explains what this pattern checks.
	Description string `json:"description"`

	// Category groups related patterns (e.g., "k8s", "gitops").
	Category string `json:"category"`

	// PatternType indicates whether this pattern requires git context (v0.10+).
	// Valid values: "" (graph-only), "hybrid", "git-aware"
	// Graph-only: runs without git context
	// Hybrid: runs without git context but enriched when available
	// Git-aware: SKIPs when git context unavailable
	PatternType string `json:"-"`

	// Prerequisites are conditions that must be met before detection runs (v0.9+).
	// If any prerequisite is unmet, the pattern is skipped with a reason.
	Prerequisites []Prerequisite `json:"-"`

	// Detect runs the pattern detection against a graph (v0.7+ patterns).
	// Returns findings and status. Errors are returned as findings with error severity.
	// For patterns that don't need git context, this is the only field needed.
	Detect func(g *graph.Graph) ([]Finding, Status) `json:"-"`

	// DetectWithGit runs pattern detection with optional git context (v0.10+ patterns).
	// If set, this is called instead of Detect. The git context may be nil.
	// Patterns should check ctx.Git != nil && ctx.Git.Valid before using git data.
	DetectWithGit func(ctx *DetectContext) ([]Finding, Status) `json:"-"`
}

// Prerequisite defines a condition that must be met before a pattern runs.
type Prerequisite struct {
	// Type is the prerequisite type (e.g., "requires_node_kind", "requires_any_of_kinds").
	Type string

	// Kinds is the list of node kinds required (for requires_node_kind or requires_any_of_kinds).
	Kinds []string
}

// Finding represents a single detection result from a pattern.
type Finding struct {
	// Pattern is the pattern ID that produced this finding.
	Pattern string `json:"pattern"`

	// Severity indicates the importance of this finding.
	Severity Severity `json:"severity"`

	// Message describes the finding.
	Message string `json:"message"`

	// Resource is the affected resource ID (optional).
	Resource string `json:"resource,omitempty"`

	// Evidence provides supporting details.
	Evidence []string `json:"evidence,omitempty"`

	// Confidence is a deterministic score (0.0-1.0) for the finding (v0.8+, optional).
	Confidence *float64 `json:"confidence,omitempty"`

	// Refs are stable identifiers for correlation across runs (v0.8+, optional).
	Refs []string `json:"refs,omitempty"`

	// Remediation provides actionable guidance (v0.8+, optional).
	Remediation *Remediation `json:"remediation,omitempty"`
}

// Remediation provides structured guidance for resolving a finding.
type Remediation struct {
	// Summary is a brief description of the fix (required if Remediation is present).
	Summary string `json:"summary"`

	// Steps are ordered action steps (optional).
	Steps []string `json:"steps,omitempty"`

	// Links are canonical documentation URLs (optional).
	Links []string `json:"links,omitempty"`
}

// Severity indicates the importance of a finding.
type Severity string

const (
	// SeverityInfo is informational (no action needed).
	SeverityInfo Severity = "info"

	// SeverityWarning suggests attention may be needed.
	SeverityWarning Severity = "warning"

	// SeverityError indicates a problem that should be addressed.
	SeverityError Severity = "error"
)

// Result represents the result of running all patterns.
type Result struct {
	// SchemaVersion identifies the output schema.
	SchemaVersion string `json:"schema_version"`

	// Patterns lists all pattern results in deterministic order.
	Patterns []PatternResult `json:"patterns"`
}

// PatternResult represents the result of a single pattern detection.
type PatternResult struct {
	// ID is the pattern identifier.
	ID string `json:"id"`

	// Name is the human-readable pattern name.
	Name string `json:"name"`

	// Status is the pattern result status.
	Status Status `json:"status"`

	// SkipReason explains why a pattern was skipped (v0.9+, optional).
	// Only present when status is "skip" and prerequisites were unmet.
	SkipReason string `json:"skip_reason,omitempty"`

	// Findings lists all findings from this pattern.
	Findings []Finding `json:"findings"`
}
