// Package remedy provides automated remediation for CCVE findings.
package remedy

import (
	"context"
	"time"
)

// RemedyType matches CCVE remedy.type field
type RemedyType string

const (
	ConfigFix       RemedyType = "config_fix"
	TriggerAction   RemedyType = "trigger_action"
	DeleteResource  RemedyType = "delete_resource"
	Restart         RemedyType = "restart"
	Upgrade         RemedyType = "upgrade"
	SourceFix       RemedyType = "source_fix"
	ExternalAction  RemedyType = "external_action"
	DiagnoseThenFix RemedyType = "diagnose_then_fix"
	ConfigChange    RemedyType = "config_change"
)

// AutoFixableTypes are remedy types that can be fully automated
var AutoFixableTypes = []RemedyType{
	ConfigFix,
	TriggerAction,
	Restart,
	DeleteResource, // Needs confirmation but is automatable
}

// Executor executes a remedy for a CCVE finding
type Executor interface {
	// Type returns the remedy type this executor handles
	Type() RemedyType

	// CanExecute checks if this executor can handle the finding
	CanExecute(finding *Finding) bool

	// DryRun shows what would be changed without applying
	DryRun(ctx context.Context, finding *Finding) (*RemedyPlan, error)

	// Execute applies the remedy
	Execute(ctx context.Context, finding *Finding, opts *ExecuteOptions) (*RemedyResult, error)
}

// Finding represents a detected CCVE issue
type Finding struct {
	CCVE       string            // e.g., "CCVE-2025-0687"
	Resource   ResourceRef       // What resource has the issue
	Namespace  string            // Namespace of the resource
	Details    map[string]string // Issue-specific details
	RemedyType RemedyType        // From CCVE remedy.type
	Commands   []string          // From CCVE remediation.commands
	Steps      []string          // From CCVE remediation.steps
}

// ResourceRef identifies a K8s resource
type ResourceRef struct {
	APIVersion string
	Kind       string
	Name       string
	Namespace  string
}

// String returns a kubectl-style resource reference
func (r ResourceRef) String() string {
	if r.Namespace != "" {
		return r.Kind + "/" + r.Name + " -n " + r.Namespace
	}
	return r.Kind + "/" + r.Name
}

// RemedyPlan describes what the executor will do
type RemedyPlan struct {
	Finding    *Finding
	Actions    []PlannedAction
	Reversible bool
	RiskLevel  RiskLevel
}

// RiskLevel indicates how dangerous an action is
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// PlannedAction is one step in the remedy
type PlannedAction struct {
	Description string // Human-readable description
	Command     string // kubectl command to execute
	DiffBefore  string // Current state (YAML)
	DiffAfter   string // Expected state after (YAML or description)
}

// ExecuteOptions controls execution behavior
type ExecuteOptions struct {
	DryRun   bool          // Show what would be done without doing it
	Force    bool          // Skip confirmation prompts
	Timeout  time.Duration // Max time for execution
	Rollback bool          // Create rollback point before execution
}

// DefaultExecuteOptions returns sensible defaults
func DefaultExecuteOptions() *ExecuteOptions {
	return &ExecuteOptions{
		DryRun:   true, // Safe by default
		Force:    false,
		Timeout:  30 * time.Second,
		Rollback: true,
	}
}

// RemedyResult is the outcome of execution
type RemedyResult struct {
	Success     bool
	Message     string
	Actions     []ActionResult
	RollbackCmd string // Command to undo if needed
}

// ActionResult is the outcome of a single action
type ActionResult struct {
	Action  PlannedAction
	Success bool
	Output  string
	Error   string
}
