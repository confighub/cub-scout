package patterns

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/confighub/cub-scout/internal/gitctx"
	"github.com/confighub/cub-scout/internal/graph"
)

// DetectAll runs all registered patterns against the graph.
// Results are in deterministic order (sorted by pattern ID).
// This is a convenience wrapper that calls DetectAllWithGit with nil git context.
func DetectAll(g *graph.Graph) Result {
	return DetectAllWithGit(g, nil)
}

// DetectAllWithGit runs all registered patterns against the graph with optional git context.
// Results are in deterministic order (sorted by pattern ID).
// The gitCtx parameter may be nil (flag not provided) or invalid (flag provided but unusable).
func DetectAllWithGit(g *graph.Graph, gitCtx *gitctx.GitContext) Result {
	patterns := List()
	results := make([]PatternResult, 0, len(patterns))

	ctx := &DetectContext{
		Graph: g,
		Git:   gitCtx,
	}

	for _, p := range patterns {
		pr := runPattern(p, ctx)
		results = append(results, pr)
	}

	return Result{
		SchemaVersion: SchemaVersion,
		Patterns:      results,
	}
}

// DetectOne runs a single pattern against the graph.
// Returns nil if the pattern is not found.
// This is a convenience wrapper that calls DetectOneWithGit with nil git context.
func DetectOne(g *graph.Graph, patternID string) *PatternResult {
	return DetectOneWithGit(g, nil, patternID)
}

// DetectOneWithGit runs a single pattern against the graph with optional git context.
// Returns nil if the pattern is not found.
func DetectOneWithGit(g *graph.Graph, gitCtx *gitctx.GitContext, patternID string) *PatternResult {
	p := Get(patternID)
	if p == nil {
		return nil
	}

	ctx := &DetectContext{
		Graph: g,
		Git:   gitCtx,
	}

	pr := runPattern(*p, ctx)
	return &pr
}

// runPattern executes a single pattern and returns the result.
// Handles prerequisites, git context requirements, and detection.
func runPattern(p Pattern, ctx *DetectContext) PatternResult {
	// Check prerequisites first (v0.9+)
	if skipReason := checkPrerequisites(p, ctx.Graph); skipReason != "" {
		return PatternResult{
			ID:         p.ID,
			Name:       p.Name,
			Status:     StatusSkip,
			SkipReason: skipReason,
			Findings:   []Finding{},
		}
	}

	// Check git context requirements for git-aware patterns (v0.10+)
	// Git-aware patterns SKIP when git context is provided but invalid
	if p.PatternType == "git-aware" && ctx.Git != nil && !ctx.Git.Valid {
		return PatternResult{
			ID:         p.ID,
			Name:       p.Name,
			Status:     StatusSkip,
			SkipReason: ctx.Git.SkipReason,
			Findings:   []Finding{},
		}
	}

	// Run detection
	var findings []Finding
	var status Status

	if p.DetectWithGit != nil {
		// v0.10+ pattern with git context support
		findings, status = p.DetectWithGit(ctx)
	} else if p.Detect != nil {
		// v0.7+ pattern (graph-only)
		findings, status = p.Detect(ctx.Graph)
	} else {
		// No detection function (shouldn't happen)
		return PatternResult{
			ID:       p.ID,
			Name:     p.Name,
			Status:   StatusSkip,
			Findings: []Finding{},
		}
	}

	// Sort findings for deterministic output
	sortFindings(findings)

	return PatternResult{
		ID:       p.ID,
		Name:     p.Name,
		Status:   status,
		Findings: findings,
	}
}

// AnyFail returns true if any pattern has status Fail.
func AnyFail(r Result) bool {
	for _, pr := range r.Patterns {
		if pr.Status == StatusFail {
			return true
		}
	}
	return false
}

// checkPrerequisites evaluates pattern prerequisites against the graph.
// Returns empty string if all prerequisites are met, or a skip reason if not.
func checkPrerequisites(p Pattern, g *graph.Graph) string {
	if len(p.Prerequisites) == 0 {
		return ""
	}

	// Build a set of node kinds present in the graph
	presentKinds := make(map[string]bool)
	for _, node := range g.Nodes {
		presentKinds[node.Kind] = true
	}

	// Evaluate prerequisites in declared order
	for _, prereq := range p.Prerequisites {
		switch prereq.Type {
		case "requires_node_kind":
			// All listed kinds must be present
			for _, kind := range prereq.Kinds {
				if !presentKinds[kind] {
					return fmt.Sprintf("no %s nodes in graph", kind)
				}
			}
		case "requires_any_of_kinds":
			// At least one of the listed kinds must be present
			found := false
			for _, kind := range prereq.Kinds {
				if presentKinds[kind] {
					found = true
					break
				}
			}
			if !found {
				return fmt.Sprintf("no %s nodes in graph", strings.Join(prereq.Kinds, "/"))
			}
		}
	}

	return ""
}

// sortFindings sorts findings by (severity, resource, message) for deterministic output.
func sortFindings(findings []Finding) {
	// Define severity order for sorting
	severityOrder := map[Severity]int{
		SeverityError:   0,
		SeverityWarning: 1,
		SeverityInfo:    2,
	}

	sort.Slice(findings, func(i, j int) bool {
		// First by severity (error < warning < info)
		if findings[i].Severity != findings[j].Severity {
			return severityOrder[findings[i].Severity] < severityOrder[findings[j].Severity]
		}
		// Then by resource
		if findings[i].Resource != findings[j].Resource {
			return findings[i].Resource < findings[j].Resource
		}
		// Then by message
		return findings[i].Message < findings[j].Message
	})

	// Sort arrays within each finding for deterministic output
	for i := range findings {
		sort.Strings(findings[i].Evidence)
		sort.Strings(findings[i].Refs)
		// Sort remediation.links (steps maintain pattern-defined order)
		if findings[i].Remediation != nil {
			sort.Strings(findings[i].Remediation.Links)
		}
	}
}

// renderFinding renders a single finding to the string builder.
func renderFinding(sb *strings.Builder, f Finding, indent string) {
	sb.WriteString(fmt.Sprintf("%s- [%s] %s\n", indent, f.Severity, f.Message))
	if f.Resource != "" {
		sb.WriteString(fmt.Sprintf("%s  resource: %s\n", indent, f.Resource))
	}
	if len(f.Evidence) > 0 {
		sb.WriteString(fmt.Sprintf("%s  evidence (%d):\n", indent, len(f.Evidence)))
		for _, e := range f.Evidence {
			sb.WriteString(fmt.Sprintf("%s    - %s\n", indent, e))
		}
	}
	// Render remediation block only if present (v0.8+)
	if f.Remediation != nil && f.Remediation.Summary != "" {
		sb.WriteString(fmt.Sprintf("%s  Remediation: %s\n", indent, f.Remediation.Summary))
		if len(f.Remediation.Steps) > 0 {
			for _, step := range f.Remediation.Steps {
				sb.WriteString(fmt.Sprintf("%s    - %s\n", indent, step))
			}
		}
		if len(f.Remediation.Links) > 0 {
			sb.WriteString(fmt.Sprintf("%s  Links:\n", indent))
			for _, link := range f.Remediation.Links {
				sb.WriteString(fmt.Sprintf("%s    - %s\n", indent, link))
			}
		}
	}
}

// RenderText renders the result as deterministic text output.
func RenderText(r Result) string {
	var sb strings.Builder

	sb.WriteString("PATTERNS DETECT\n")
	sb.WriteString(fmt.Sprintf("Schema: %s\n", r.SchemaVersion))
	sb.WriteString("\n")

	for _, pr := range r.Patterns {
		// Status indicator
		indicator := "[PASS]"
		if pr.Status == StatusFail {
			indicator = "[FAIL]"
		} else if pr.Status == StatusSkip {
			indicator = "[SKIP]"
		}

		sb.WriteString(fmt.Sprintf("%s %s\n", indicator, pr.ID))
		sb.WriteString(fmt.Sprintf("  %s\n", pr.Name))

		// Skip reason (v0.9+) - only when status=skip and prereqs unmet
		if pr.SkipReason != "" {
			sb.WriteString(fmt.Sprintf("  skip_reason: %s\n", pr.SkipReason))
		}

		// Always print findings block
		sb.WriteString(fmt.Sprintf("  findings (%d):\n", len(pr.Findings)))
		if len(pr.Findings) == 0 {
			sb.WriteString("    (none)\n")
		} else {
			for _, f := range pr.Findings {
				renderFinding(&sb, f, "    ")
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// RenderJSON renders the result as deterministic JSON.
func RenderJSON(r Result) (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}
	return string(data), nil
}

// RenderExplain renders a detailed explanation for a single pattern.
func RenderExplain(p *Pattern, pr *PatternResult) string {
	var sb strings.Builder

	sb.WriteString("PATTERN EXPLAIN\n")
	sb.WriteString(fmt.Sprintf("ID: %s\n", p.ID))
	sb.WriteString(fmt.Sprintf("Name: %s\n", p.Name))
	sb.WriteString(fmt.Sprintf("Category: %s\n", p.Category))
	sb.WriteString("\n")

	sb.WriteString("Description:\n")
	sb.WriteString(fmt.Sprintf("  %s\n", p.Description))
	sb.WriteString("\n")

	if pr != nil {
		// Status indicator
		indicator := "[PASS]"
		if pr.Status == StatusFail {
			indicator = "[FAIL]"
		} else if pr.Status == StatusSkip {
			indicator = "[SKIP]"
		}

		sb.WriteString(fmt.Sprintf("Result: %s\n", indicator))

		// Skip reason (v0.9+) - only when status=skip and prereqs unmet
		if pr.SkipReason != "" {
			sb.WriteString(fmt.Sprintf("  skip_reason: %s\n", pr.SkipReason))
		}

		sb.WriteString(fmt.Sprintf("  findings (%d):\n", len(pr.Findings)))
		if len(pr.Findings) == 0 {
			sb.WriteString("    (none)\n")
		} else {
			for _, f := range pr.Findings {
				renderFinding(&sb, f, "    ")
			}
		}
	}

	return sb.String()
}

// RenderList renders the pattern list as text.
func RenderList() string {
	var sb strings.Builder

	sb.WriteString("PATTERNS LIST\n")
	sb.WriteString(fmt.Sprintf("Schema: %s\n", SchemaVersion))
	sb.WriteString("\n")

	patterns := List()
	sb.WriteString(fmt.Sprintf("Registered patterns (%d):\n", len(patterns)))

	if len(patterns) == 0 {
		sb.WriteString("  (none)\n")
	} else {
		for _, p := range patterns {
			sb.WriteString(fmt.Sprintf("  - %s\n", p.ID))
			sb.WriteString(fmt.Sprintf("    %s\n", p.Name))
		}
	}

	return sb.String()
}
