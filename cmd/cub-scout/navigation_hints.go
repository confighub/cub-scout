package main

import (
	"fmt"
	"sort"
	"strings"
)

// Hint represents a single navigation hint with rationale and priority.
// Priority is used for ranking: higher values are more urgent/relevant.
// Rationale explains why this hint is suggested (the "so what").
type Hint struct {
	Command      string // The copyable cub-scout command
	Rationale    string // Why this hint is suggested
	Priority     int    // Higher = more urgent; used for sorting
	ConfigHubURL string // Optional: URL to open in ConfigHub GUI (only when connected with valid context)
}

// hintPriorityUrgent is for issues requiring immediate attention.
const hintPriorityUrgent = 100

// hintPriorityHigh is for important next steps.
const hintPriorityHigh = 80

// hintPriorityNormal is for standard exploration suggestions.
const hintPriorityNormal = 50

// hintPriorityLow is for general guidance.
const hintPriorityLow = 20

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
	hints := doctorHints(summary)
	sortHints(hints)
	if len(hints) > 3 {
		hints = hints[:3]
	}
	return hintsToStrings(hints)
}

// doctorHints generates structured hints for doctor output.
func doctorHints(summary DoctorSummary) []Hint {
	hints := make([]Hint, 0, 4)
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

	hints = append(hints, Hint{
		Command:   fmt.Sprintf("cub-scout quickstart%s --yes", nsFlag),
		Rationale: "Run the guided walkthrough to explore your cluster step-by-step",
		Priority:  hintPriorityLow,
	})

	return hints
}

func explainTryNextHints(summary ExplainSummary) []string {
	hints := explainHints(summary)
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
	hints := make([]Hint, 0, 4)
	ns := strings.TrimSpace(summary.Namespace)
	nsFlag := commandNamespaceFlag(ns)

	owner := strings.TrimSpace(summary.Owner)
	unknownOwner := strings.HasPrefix(strings.ToLower(owner), "unknown")
	healthUnknown := strings.EqualFold(strings.TrimSpace(summary.Health), "unavailable") ||
		strings.EqualFold(strings.TrimSpace(summary.Health), "unknown")

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
	} else {
		// Known owner - help explore the ownership chain
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
	}

	hints = append(hints, Hint{
		Command:   fmt.Sprintf("cub-scout doctor%s", nsFlag),
		Rationale: "Get a broad health summary to contextualize this resource",
		Priority:  hintPriorityLow,
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
