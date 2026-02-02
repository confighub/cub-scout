// Package patterns provides the pattern detection engine for cub-scout v0.7+.
//
// Patterns are deterministic checks that analyze the resource graph and report
// findings. Each pattern has a unique ID, description, and detection logic.
//
// This is a v0.7 contract surface that does not modify any v0.5 or v0.6 contracts.
package patterns

import "github.com/confighub/cub-scout/internal/graph"

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

	// Detect runs the pattern detection against a graph.
	// Returns findings and status. Errors are returned as findings with error severity.
	Detect func(g *graph.Graph) ([]Finding, Status) `json:"-"`
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

	// Findings lists all findings from this pattern.
	Findings []Finding `json:"findings"`
}
