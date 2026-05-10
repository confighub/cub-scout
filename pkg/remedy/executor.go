// Package remedy generates suggested-patch evidence for risk findings.
//
// cub-scout never applies a remedy. The Suggest method returns a structured
// description of *what a fix would look like*, including the kubectl command
// that would carry it out — but cub-scout does not run that command. Pilot,
// ConfigHub, an operator, or another tool is responsible for any apply.
//
// See issues #410 and #428 for the architectural decision behind this split.
package remedy

import (
	"context"
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

// AutoFixableTypes are remedy types for which an automated fix can be
// described. cub-scout only describes them; a downstream tool decides
// whether and how to apply.
var AutoFixableTypes = []RemedyType{
	ConfigFix,
	TriggerAction,
	Restart,
	DeleteResource,
}

// Suggester produces a suggested-patch description for a risk finding.
//
// Implementations must be read-only: no kubectl apply, no kubectl delete,
// no exec of mutating shell commands. Reading current resource state for
// the purpose of describing a diff is allowed.
type Suggester interface {
	// Type returns the remedy type this suggester handles.
	Type() RemedyType

	// CanSuggest reports whether this suggester can describe a fix for the
	// given finding.
	CanSuggest(finding *Finding) bool

	// Suggest produces a structured description of the fix that would
	// resolve the finding. It MUST NOT mutate cluster state.
	Suggest(ctx context.Context, finding *Finding) (*SuggestedRemedy, error)
}

// Finding represents a detected risk issue
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

// SuggestedRemedy is a structured description of a fix that *would*
// resolve a risk finding. It is read-only evidence — cub-scout produces
// it; cub-scout does not apply it.
type SuggestedRemedy struct {
	Finding    *Finding
	Actions    []SuggestedAction
	Reversible bool
	RiskLevel  RiskLevel
}

// RiskLevel indicates how dangerous applying the suggestion would be.
// cub-scout exposes this as evidence; the decision to act on it belongs
// to the caller.
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// SuggestedAction is one step of a suggested remedy. The Command field
// records the kubectl invocation that *would* perform the step — cub-scout
// does not execute it.
type SuggestedAction struct {
	Description string // Human-readable description
	Command     string // kubectl command that would carry out the step
	DiffBefore  string // Current state (YAML), if observable
	DiffAfter   string // Expected state after, if computable ahead of apply
}
