package main

import (
	"fmt"
	"sort"
	"strings"
)

// ActionType classifies what kind of action a hint suggests.
// This helps AI agents and MCP clients distinguish read-only investigation
// from actions that require human approval or mutation.
type ActionType string

const (
	// ActionReadOnly is for hints that only read/observe (no side effects).
	// Most cub-scout commands fall into this category.
	ActionReadOnly ActionType = "read-only"

	// ActionMutating is for hints that would modify cluster or config state.
	// cub-scout itself is read-only, but hints might suggest external commands.
	ActionMutating ActionType = "mutating"

	// ActionWaiting is for hints that suggest waiting for convergence.
	// Example: "wait for Argo sync to complete"
	ActionWaiting ActionType = "waiting"

	// ActionHumanDecision is for hints that require human judgment.
	// Example: "decide whether to adopt this unmanaged resource"
	ActionHumanDecision ActionType = "human-decision"
)

// Hint represents a single navigation hint with rationale and priority.
// Priority is used for ranking: higher values are more urgent/relevant.
// Rationale explains why this hint is suggested (the "so what").
type Hint struct {
	Command      string     // The copyable cub-scout command
	Rationale    string     // Why this hint is suggested
	Priority     int        // Higher = more urgent; used for sorting
	ConfigHubURL string     // Optional: URL to open in ConfigHub GUI (only when connected with valid context)
	ActionType   ActionType // Classification for AI/MCP clients (default: read-only)
	Blocker      string     // Optional: machine-readable blocker key when a path is blocked
}

// StructuredHint is the JSON-serializable form of a hint for machine consumption.
// This is the contract for JSON output and MCP tool responses.
type StructuredHint struct {
	ActionType  ActionType `json:"actionType"`            // read-only, mutating, waiting, human-decision
	Reason      string     `json:"reason"`                // Short plain-English reason
	NextCommand string     `json:"nextCommand,omitempty"` // Exact command when applicable
	NextSurface string     `json:"nextSurface,omitempty"` // Optional URL or surface hint
	Blocker     string     `json:"blocker,omitempty"`     // Optional machine-readable blocker key
}

// ToStructured converts a Hint to its JSON-serializable form.
func (h Hint) ToStructured() StructuredHint {
	actionType := h.ActionType
	if actionType == "" {
		actionType = ActionReadOnly // Default for cub-scout (read-only tool)
	}
	return StructuredHint{
		ActionType:  actionType,
		Reason:      h.Rationale,
		NextCommand: h.Command,
		NextSurface: h.ConfigHubURL,
		Blocker:     h.Blocker,
	}
}

// HintsToStructured converts a slice of Hints to StructuredHints.
func HintsToStructured(hints []Hint) []StructuredHint {
	result := make([]StructuredHint, len(hints))
	for i, h := range hints {
		result[i] = h.ToStructured()
	}
	return result
}

// hintPriorityUrgent is for issues requiring immediate attention.
const hintPriorityUrgent = 100

// hintPriorityHigh is for important next steps.
const hintPriorityHigh = 80

// hintPriorityNormal is for standard exploration suggestions.
const hintPriorityNormal = 50

// hintPriorityLow is for general guidance.
const hintPriorityLow = 20

// hintPrioritySuppressed is for hints that should be included but ranked very low.
const hintPrioritySuppressed = 5

// HintMode represents the context in which hints are generated.
// Different modes adjust hint ranking to be more relevant for the audience.
type HintMode string

const (
	// HintModeDefault uses safe defaults with beginner-friendly hints included.
	HintModeDefault HintMode = "default"

	// HintModeBeginner emphasizes onboarding and exploration hints.
	HintModeBeginner HintMode = "beginner"

	// HintModeOperator is for experienced users - suppresses beginner hints,
	// emphasizes actionable operational next steps.
	HintModeOperator HintMode = "operator"

	// HintModeDemo is for demos and presentations - suppresses noisy hints,
	// emphasizes clear next steps that showcase capabilities.
	HintModeDemo HintMode = "demo"
)

// HintContext provides context for hint generation.
// This allows hints to be ranked differently based on the audience/situation.
type HintContext struct {
	Mode HintMode
}

// ResourcePhase represents the inferred operational phase of a resource.
// This is derived from deterministic signals in ExplainSummary.
type ResourcePhase string

const (
	// PhaseIncident indicates an active issue requiring investigation.
	// Triggered by: unhealthy status, detected drift, or critical risks.
	PhaseIncident ResourcePhase = "incident"

	// PhaseVerify indicates the resource needs confirmation of health.
	// Triggered by: healthy status but unknown drift or warnings present.
	PhaseVerify ResourcePhase = "verify"

	// PhaseCloseout indicates the resource is healthy and converged.
	// Triggered by: healthy status, no drift, no high/critical risks.
	PhaseCloseout ResourcePhase = "closeout"

	// PhaseDefault is used when phase cannot be determined.
	PhaseDefault ResourcePhase = "default"
)

// DefaultHintContext returns a HintContext with default settings.
func DefaultHintContext() HintContext {
	return HintContext{Mode: HintModeDefault}
}

// ParseHintMode parses a string into a HintMode.
// Returns an error if the string is not a valid mode.
func ParseHintMode(s string) (HintMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "default":
		return HintModeDefault, nil
	case "beginner":
		return HintModeBeginner, nil
	case "operator":
		return HintModeOperator, nil
	default:
		return HintModeDefault, fmt.Errorf("invalid hint mode %q (valid: default, beginner, operator)", s)
	}
}

// HintModeHelp returns help text for the --hint-mode flag.
func HintModeHelp() string {
	return "Hint ranking mode: default, beginner (emphasizes tutorials), operator (actionable hints)"
}

// isBeginnerMode returns true if quickstart-style hints should be prominent.
func (c HintContext) isBeginnerMode() bool {
	return c.Mode == HintModeDefault || c.Mode == HintModeBeginner
}

// isOperatorMode returns true if the context is operator or demo (non-beginner).
func (c HintContext) isOperatorMode() bool {
	return c.Mode == HintModeOperator || c.Mode == HintModeDemo
}

// deriveResourcePhase infers the operational phase from ExplainSummary facts.
// This is entirely deterministic: same inputs always produce same phase.
func deriveResourcePhase(summary ExplainSummary) ResourcePhase {
	health := strings.ToLower(strings.TrimSpace(summary.Health))
	drift := strings.ToLower(strings.TrimSpace(summary.Drift))
	risks := strings.ToLower(strings.TrimSpace(summary.Risks))
	deployedVia := strings.ToLower(strings.TrimSpace(summary.DeployedVia))

	// Check for incident indicators
	isUnhealthy := health == "unhealthy" || health == "unavailable" || health == "unknown" ||
		strings.Contains(health, "degraded") || strings.Contains(health, "error") ||
		strings.Contains(health, "failed") || strings.Contains(health, "crash")
	hasDrift := strings.Contains(drift, "detected") && !strings.Contains(drift, "none")
	hasCriticalRisks := strings.Contains(risks, "critical")
	hasHighRisks := strings.Contains(risks, "high")
	isPartialTrace := strings.Contains(deployedVia, "partial")

	// Incident phase: active issue requiring investigation
	if isUnhealthy || hasDrift || hasCriticalRisks {
		return PhaseIncident
	}

	// Check for healthy indicators
	isHealthy := health == "healthy" || strings.Contains(health, "ready") ||
		strings.Contains(health, "synced")
	noDrift := drift == "none detected" || drift == "none"
	noHighRisks := !hasCriticalRisks && !hasHighRisks
	hasWarnings := strings.Contains(risks, "warning")
	noWarnings := !hasWarnings
	hasFullTrace := !isPartialTrace && deployedVia != "unknown"

	// Closeout phase: everything looks good (no warnings either)
	if isHealthy && noDrift && noHighRisks && noWarnings && hasFullTrace {
		return PhaseCloseout
	}

	// Verify phase: healthy but needs confirmation
	if isHealthy {
		// Unknown drift, warnings present, or partial trace
		driftUnknown := drift == "unknown" || drift == ""
		if driftUnknown || hasWarnings || isPartialTrace {
			return PhaseVerify
		}
	}

	return PhaseDefault
}

// isArgoOwned returns true if the owner string indicates ArgoCD management.
// Uses exact matches for known ArgoCD display values to avoid misclassifying
// custom owners that happen to contain "argo" (e.g., "Argo Platform", "argo-rollouts").
func isArgoOwned(owner string) bool {
	o := strings.ToLower(strings.TrimSpace(owner))
	// Match only the actual ArgoCD display values used by ownership detection
	return o == "argocd" || o == "argo cd" || o == "argo-cd"
}

// sortHints sorts hints by priority (descending) for deterministic output.
func sortHints(hints []Hint) {
	sort.SliceStable(hints, func(i, j int) bool {
		return hints[i].Priority > hints[j].Priority
	})
}

// hintsToStrings converts Hint slice to legacy string format for rendering.
// Format: "Rationale: command"
func hintsToStrings(hints []Hint) []string {
	result := make([]string, 0, len(hints))
	for _, h := range hints {
		result = append(result, h.Rationale+": "+h.Command)
	}
	return result
}

func withKubeRecoveryHint(err error, command string) error {
	if err == nil {
		return nil
	}
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		cmd = "cub-scout"
	}
	return fmt.Errorf("%w\n\nRecovery:\n  1) kubectl config current-context\n  2) kubectl get ns\n  3) %s --help\n  4) cub-scout quickstart", err, cmd)
}

func mapListTryNextHints(entries []MapEntry, byOwner map[string]int, namespace string) []string {
	hints := mapListHints(entries, byOwner, namespace)
	sortHints(hints)
	if len(hints) > 3 {
		hints = hints[:3]
	}
	return hintsToStrings(hints)
}

// mapListHints generates structured hints for map list output.
func mapListHints(entries []MapEntry, byOwner map[string]int, namespace string) []Hint {
	hints := make([]Hint, 0, 4)
	nsFlag := commandNamespaceFlag(namespace)

	nativeCount := byOwner["Native"]
	if nativeCount > 0 {
		// Unmanaged resources are a governance concern - prioritize based on count
		priority := hintPriorityHigh
		if nativeCount >= 10 {
			priority = hintPriorityUrgent
		}
		rationale := fmt.Sprintf("Found %d unmanaged resource", nativeCount)
		if nativeCount != 1 {
			rationale += "s"
		}
		rationale += " - these lack GitOps ownership and may drift"
		hints = append(hints, Hint{
			Command:   fmt.Sprintf("cub-scout map orphans%s", nsFlag),
			Rationale: rationale,
			Priority:  priority,
		})
	}

	if kind, name, ns, ok := pickExplainResource(entries); ok {
		useNS := chooseNamespace(namespace, ns)
		hints = append(hints, Hint{
			Command:   fmt.Sprintf("cub-scout explain %s/%s%s", strings.ToLower(kind), name, commandNamespaceFlag(useNS)),
			Rationale: "Trace one resource end-to-end to see its full ownership chain",
			Priority:  hintPriorityNormal,
		})
	}

	hints = append(hints, Hint{
		Command:   fmt.Sprintf("cub-scout doctor%s", nsFlag),
		Rationale: "Get a one-command health summary across ownership, drift, and risks",
		Priority:  hintPriorityLow,
	})

	return hints
}

func doctorTryNextHints(summary DoctorSummary) []string {
	return doctorTryNextHintsWithContext(summary, DefaultHintContext())
}

// doctorTryNextHintsWithContext generates hints with explicit context control.
func doctorTryNextHintsWithContext(summary DoctorSummary, ctx HintContext) []string {
	hints := doctorHintsWithContext(summary, ctx)
	sortHints(hints)
	if len(hints) > 3 {
		hints = hints[:3]
	}
	return hintsToStrings(hints)
}

// doctorHints generates structured hints for doctor output.
func doctorHints(summary DoctorSummary) []Hint {
	return doctorHintsWithContext(summary, DefaultHintContext())
}

// doctorHintsWithContext generates structured hints with explicit context control.
// The context affects hint ranking:
// - Beginner/default: quickstart is prominent, general exploration hints included
// - Operator/demo: quickstart is suppressed, actionable hints prioritized
func doctorHintsWithContext(summary DoctorSummary, ctx HintContext) []Hint {
	hints := make([]Hint, 0, 5)
	nsFlag := commandNamespaceFlag(summary.Namespace)

	// Top issue gets highest priority if critical
	if len(summary.TopIssues) > 0 {
		issue := summary.TopIssues[0]
		if kind, name, ok := parseKindName(issue.Resource); ok {
			useNS := chooseNamespace(summary.Namespace, issue.Namespace)
			priority := hintPriorityHigh
			rationale := fmt.Sprintf("Investigate your top issue (%s)", issue.Severity)
			if strings.EqualFold(issue.Severity, "CRITICAL") {
				priority = hintPriorityUrgent
				rationale = fmt.Sprintf("CRITICAL issue on %s - investigate immediately", issue.Resource)
			} else if strings.EqualFold(issue.Severity, "HIGH") {
				rationale = fmt.Sprintf("HIGH severity issue on %s - needs attention", issue.Resource)
			}
			hints = append(hints, Hint{
				Command:   fmt.Sprintf("cub-scout explain %s/%s%s", strings.ToLower(kind), name, commandNamespaceFlag(useNS)),
				Rationale: rationale,
				Priority:  priority,
			})
		}
	}

	// Unmanaged resources hint
	if summary.Ownership.Unmanaged > 0 {
		priority := hintPriorityHigh
		if summary.Ownership.Unmanaged >= 10 {
			priority = hintPriorityUrgent - 10 // Just below critical issues
		}
		rationale := fmt.Sprintf("%d resource", summary.Ownership.Unmanaged)
		if summary.Ownership.Unmanaged != 1 {
			rationale += "s"
		}
		rationale += " without GitOps ownership - review for governance gaps"
		hints = append(hints, Hint{
			Command:   fmt.Sprintf("cub-scout map orphans%s", nsFlag),
			Rationale: rationale,
			Priority:  priority,
		})
	}

	// Import hint for native-heavy clusters (operator/demo contexts or high unmanaged count)
	// This helps operators see how to bring unmanaged resources under GitOps control.
	if summary.Ownership.Unmanaged > 0 {
		unmanagedRatio := float64(summary.Ownership.Unmanaged) / float64(max(summary.Resources.Total, 1))
		isNativeHeavy := summary.Ownership.Unmanaged >= 5 || unmanagedRatio > 0.3

		if isNativeHeavy {
			priority := hintPriorityNormal
			// Boost priority in operator mode or when the cluster is mostly unmanaged
			if ctx.isOperatorMode() || unmanagedRatio > 0.5 {
				priority = hintPriorityHigh - 5
			}
			hints = append(hints, Hint{
				Command:   fmt.Sprintf("cub-scout import --dry-run%s", nsFlag),
				Rationale: "Preview how to bring unmanaged resources under GitOps control",
				Priority:  priority,
			})
		}
	}

	// Quickstart hint - priority depends on context
	quickstartPriority := hintPriorityLow
	if ctx.isOperatorMode() {
		// Suppress quickstart in operator/demo mode - experienced users don't need the walkthrough
		quickstartPriority = hintPrioritySuppressed
	} else if ctx.Mode == HintModeBeginner {
		// Boost in explicit beginner mode
		quickstartPriority = hintPriorityNormal
	}
	hints = append(hints, Hint{
		Command:   fmt.Sprintf("cub-scout quickstart%s --yes", nsFlag),
		Rationale: "Run the guided walkthrough to explore your cluster step-by-step",
		Priority:  quickstartPriority,
	})

	return hints
}

func explainTryNextHints(summary ExplainSummary) []string {
	return explainTryNextHintsWithContext(summary, DefaultHintContext())
}

// explainTryNextHintsWithContext generates hints with explicit context control.
func explainTryNextHintsWithContext(summary ExplainSummary, ctx HintContext) []string {
	hints := explainHintsWithContext(summary, ctx)
	sortHints(hints)
	if len(hints) > 3 {
		hints = hints[:3]
	}
	return hintsToStrings(hints)
}

// explainConfigHubHint returns a ConfigHub URL hint if available.
// Returns nil if no ConfigHub URL is available for this resource.
func explainConfigHubHint(summary ExplainSummary) *Hint {
	url := strings.TrimSpace(summary.ConfigHubURL)
	if url == "" {
		return nil
	}
	return &Hint{
		ConfigHubURL: url,
		Rationale:    "Review this unit in ConfigHub for audit trail and policy management",
		Priority:     hintPriorityNormal,
	}
}

// explainHints generates structured hints for explain output.
func explainHints(summary ExplainSummary) []Hint {
	return explainHintsWithContext(summary, DefaultHintContext())
}

// explainHintsWithContext generates structured hints with explicit context control.
// For Argo-managed resources, hints are phase-aware based on health/drift/risk signals.
func explainHintsWithContext(summary ExplainSummary, ctx HintContext) []Hint {
	hints := make([]Hint, 0, 5)
	ns := strings.TrimSpace(summary.Namespace)
	nsFlag := commandNamespaceFlag(ns)

	owner := strings.TrimSpace(summary.Owner)
	unknownOwner := strings.HasPrefix(strings.ToLower(owner), "unknown")

	if unknownOwner {
		// Unknown ownership is a governance concern - help find related issues
		hints = append(hints, Hint{
			Command:   fmt.Sprintf("cub-scout map orphans%s", nsFlag),
			Rationale: "This resource has no recognized owner - find other unmanaged resources",
			Priority:  hintPriorityHigh,
		})
		hints = append(hints, Hint{
			Command:   fmt.Sprintf("cub-scout map issues%s", nsFlag),
			Rationale: "Check for health issues that may indicate why ownership is unclear",
			Priority:  hintPriorityNormal,
		})

		// In operator mode, suggest import for unknown-owner resources
		if ctx.isOperatorMode() {
			hints = append(hints, Hint{
				Command:   fmt.Sprintf("cub-scout import --dry-run%s", nsFlag),
				Rationale: "Preview how to bring this resource under GitOps control",
				Priority:  hintPriorityNormal - 5,
			})
		}
	} else if isArgoOwned(owner) {
		// Argo-managed resource: use phase-aware hints
		hints = append(hints, explainArgoPhaseHints(summary, ctx)...)
	} else {
		// Other known owner (Flux, Helm) - use standard ownership chain hints
		hints = append(hints, explainKnownOwnerHints(summary, ctx)...)
	}

	// Doctor hint - lower priority in operator mode since they likely already ran it
	doctorPriority := hintPriorityLow
	if ctx.isOperatorMode() {
		doctorPriority = hintPrioritySuppressed
	}
	hints = append(hints, Hint{
		Command:   fmt.Sprintf("cub-scout doctor%s", nsFlag),
		Rationale: "Get a broad health summary to contextualize this resource",
		Priority:  doctorPriority,
	})

	return hints
}

// explainArgoPhaseHints generates phase-aware hints for Argo-managed resources.
// The phase is derived from health, drift, and risk signals in the summary.
func explainArgoPhaseHints(summary ExplainSummary, ctx HintContext) []Hint {
	hints := make([]Hint, 0, 4)
	ns := strings.TrimSpace(summary.Namespace)
	nsFlag := commandNamespaceFlag(ns)
	owner := strings.TrimSpace(summary.Owner)
	phase := deriveResourcePhase(summary)

	kind, name, hasResource := parseKindName(summary.Resource)

	switch phase {
	case PhaseIncident:
		// Active issue - prioritize investigation commands
		if hasResource {
			hints = append(hints, Hint{
				Command:   fmt.Sprintf("cub-scout trace %s/%s%s --explain", strings.ToLower(kind), name, nsFlag),
				Rationale: "Investigate: trace the Argo ownership chain to find the root cause",
				Priority:  hintPriorityUrgent,
			})
		}
		// Suggest checking issues across the namespace
		hints = append(hints, Hint{
			Command:   fmt.Sprintf("cub-scout map issues%s", nsFlag),
			Rationale: "See all health issues in this scope to understand the blast radius",
			Priority:  hintPriorityHigh,
		})
		// Remind that kubectl changes will be reverted
		hints = append(hints, Hint{
			Command:   fmt.Sprintf("cub-scout gitops status%s", nsFlag),
			Rationale: "This resource is Argo-managed; direct kubectl changes will be reverted - check GitOps pipeline status",
			Priority:  hintPriorityHigh - 5,
		})

	case PhaseVerify:
		// Resource looks OK but needs confirmation
		if hasResource {
			hints = append(hints, Hint{
				Command:   fmt.Sprintf("cub-scout trace %s/%s%s --explain", strings.ToLower(kind), name, nsFlag),
				Rationale: "Verify: confirm the Argo sync chain is complete and healthy",
				Priority:  hintPriorityHigh,
			})
		}
		hints = append(hints, Hint{
			Command:   fmt.Sprintf("cub-scout gitops status%s", nsFlag),
			Rationale: "Confirm GitOps pipeline is synced before closing out",
			Priority:  hintPriorityHigh - 5,
		})
		hints = append(hints, Hint{
			Command:   fmt.Sprintf("cub-scout map list%s -q \"owner=%s\"", nsFlag, owner),
			Rationale: fmt.Sprintf("Check other %s resources to confirm no remaining issues", owner),
			Priority:  hintPriorityNormal,
		})

	case PhaseCloseout:
		// Everything looks good - suggest read-only verification
		hints = append(hints, Hint{
			Command:   fmt.Sprintf("cub-scout gitops status%s", nsFlag),
			Rationale: "Closeout: GitOps and cluster agree - confirm no further action needed (read-only)",
			Priority:  hintPriorityHigh,
		})
		hints = append(hints, Hint{
			Command:   fmt.Sprintf("cub-scout map list%s -q \"owner=%s\"", nsFlag, owner),
			Rationale: fmt.Sprintf("Review all %s resources to confirm convergence (read-only)", owner),
			Priority:  hintPriorityNormal,
		})
		if hasResource {
			hints = append(hints, Hint{
				Command:   fmt.Sprintf("cub-scout trace %s/%s%s --history", strings.ToLower(kind), name, nsFlag),
				Rationale: "Show deployment history for audit trail (read-only)",
				Priority:  hintPriorityNormal - 5,
			})
		}

	default:
		// Default Argo hints (same as before)
		hints = append(hints, explainKnownOwnerHints(summary, ctx)...)
	}

	return hints
}

// explainKnownOwnerHints generates standard hints for known (non-Argo) owners.
func explainKnownOwnerHints(summary ExplainSummary, ctx HintContext) []Hint {
	hints := make([]Hint, 0, 2)
	ns := strings.TrimSpace(summary.Namespace)
	nsFlag := commandNamespaceFlag(ns)
	owner := strings.TrimSpace(summary.Owner)

	healthUnknown := strings.EqualFold(strings.TrimSpace(summary.Health), "unavailable") ||
		strings.EqualFold(strings.TrimSpace(summary.Health), "unknown")

	if kind, name, ok := parseKindName(summary.Resource); ok {
		priority := hintPriorityHigh
		rationale := fmt.Sprintf("Trace the full %s ownership chain from source to runtime", owner)
		if healthUnknown {
			priority = hintPriorityUrgent
			rationale = "Health status unknown - trace the chain to find the root cause"
		}
		hints = append(hints, Hint{
			Command:   fmt.Sprintf("cub-scout trace %s/%s%s --explain", strings.ToLower(kind), name, nsFlag),
			Rationale: rationale,
			Priority:  priority,
		})
	}
	hints = append(hints, Hint{
		Command:   fmt.Sprintf("cub-scout map list%s -q \"owner=%s\"", nsFlag, owner),
		Rationale: fmt.Sprintf("See all %s-managed resources in this scope", owner),
		Priority:  hintPriorityNormal,
	})

	return hints
}

func renderTryNextSection(hints []string) string {
	return renderTryNextSectionWithMode(hints, DefaultPresentationMode)
}

// renderTryNextSectionWithMode renders the TRY NEXT section with mode-specific framing.
func renderTryNextSectionWithMode(hints []string, mode PresentationMode) string {
	if len(hints) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(TryNextHeading(mode))
	b.WriteString("\n")
	for _, hint := range hints {
		b.WriteString("  - ")
		b.WriteString(hint)
		b.WriteString("\n")
	}
	return b.String()
}

// renderConfigHubSection renders a ConfigHub GUI URL suggestion.
// This is separate from TRY NEXT (which is for CLI commands).
func renderConfigHubSection(hint *Hint) string {
	if hint == nil || hint.ConfigHubURL == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("\nOPEN IN CONFIGHUB:\n")
	b.WriteString("  ")
	b.WriteString(hint.Rationale)
	b.WriteString("\n  -> ")
	b.WriteString(hint.ConfigHubURL)
	b.WriteString("\n")
	return b.String()
}

func renderTryNextMarkdown(hints []string) string {
	return renderTryNextMarkdownWithMode(hints, DefaultPresentationMode)
}

// renderTryNextMarkdownWithMode renders the TRY NEXT section in markdown with mode-specific framing.
func renderTryNextMarkdownWithMode(hints []string, mode PresentationMode) string {
	if len(hints) == 0 {
		return ""
	}

	var b strings.Builder
	switch mode {
	case PresentationAI:
		b.WriteString("\n### RECOMMENDED ACTIONS\n\n")
	default:
		b.WriteString("\n### Try Next\n\n")
	}
	for _, hint := range hints {
		b.WriteString("- `")
		b.WriteString(extractCommand(hint))
		b.WriteString("`")
		b.WriteString("\n")
	}
	return b.String()
}

func extractCommand(hint string) string {
	idx := strings.Index(hint, "cub-scout ")
	if idx < 0 {
		return hint
	}
	return strings.TrimSpace(hint[idx:])
}

func commandNamespaceFlag(namespace string) string {
	ns := strings.TrimSpace(namespace)
	if ns == "" || strings.EqualFold(ns, "all") || ns == "-" {
		return ""
	}
	return " -n " + ns
}

func chooseNamespace(preferred, fallback string) string {
	p := strings.TrimSpace(preferred)
	if p != "" && !strings.EqualFold(p, "all") {
		return p
	}
	return strings.TrimSpace(fallback)
}

func parseKindName(resource string) (kind, name string, ok bool) {
	parts := strings.SplitN(strings.TrimSpace(resource), "/", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func pickExplainResource(entries []MapEntry) (kind, name, namespace string, ok bool) {
	workloadKinds := map[string]struct{}{
		"Deployment":  {},
		"StatefulSet": {},
		"DaemonSet":   {},
		"Application": {},
		"Pod":         {},
	}

	for _, e := range entries {
		if e.Namespace == "" {
			continue
		}
		if _, preferred := workloadKinds[e.Kind]; preferred {
			return e.Kind, e.Name, e.Namespace, true
		}
	}
	for _, e := range entries {
		if e.Namespace == "" {
			continue
		}
		if e.Kind != "" && e.Name != "" {
			return e.Kind, e.Name, e.Namespace, true
		}
	}
	return "", "", "", false
}
