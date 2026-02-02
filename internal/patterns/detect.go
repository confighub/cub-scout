package patterns

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/confighub/cub-scout/internal/graph"
)

// DetectAll runs all registered patterns against the graph.
// Results are in deterministic order (sorted by pattern ID).
func DetectAll(g *graph.Graph) Result {
	patterns := List()
	results := make([]PatternResult, 0, len(patterns))

	for _, p := range patterns {
		findings, status := p.Detect(g)

		// Sort findings for deterministic output
		sortFindings(findings)

		results = append(results, PatternResult{
			ID:       p.ID,
			Name:     p.Name,
			Status:   status,
			Findings: findings,
		})
	}

	return Result{
		SchemaVersion: SchemaVersion,
		Patterns:      results,
	}
}

// DetectOne runs a single pattern against the graph.
// Returns nil if the pattern is not found.
func DetectOne(g *graph.Graph, patternID string) *PatternResult {
	p := Get(patternID)
	if p == nil {
		return nil
	}

	findings, status := p.Detect(g)
	sortFindings(findings)

	return &PatternResult{
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

	// Also sort evidence within each finding
	for i := range findings {
		sort.Strings(findings[i].Evidence)
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

		// Always print findings block
		sb.WriteString(fmt.Sprintf("  findings (%d):\n", len(pr.Findings)))
		if len(pr.Findings) == 0 {
			sb.WriteString("    (none)\n")
		} else {
			for _, f := range pr.Findings {
				sb.WriteString(fmt.Sprintf("    - [%s] %s\n", f.Severity, f.Message))
				if f.Resource != "" {
					sb.WriteString(fmt.Sprintf("      resource: %s\n", f.Resource))
				}
				if len(f.Evidence) > 0 {
					sb.WriteString(fmt.Sprintf("      evidence (%d):\n", len(f.Evidence)))
					for _, e := range f.Evidence {
						sb.WriteString(fmt.Sprintf("        - %s\n", e))
					}
				}
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
		sb.WriteString(fmt.Sprintf("  findings (%d):\n", len(pr.Findings)))
		if len(pr.Findings) == 0 {
			sb.WriteString("    (none)\n")
		} else {
			for _, f := range pr.Findings {
				sb.WriteString(fmt.Sprintf("    - [%s] %s\n", f.Severity, f.Message))
				if f.Resource != "" {
					sb.WriteString(fmt.Sprintf("      resource: %s\n", f.Resource))
				}
				if len(f.Evidence) > 0 {
					sb.WriteString(fmt.Sprintf("      evidence (%d):\n", len(f.Evidence)))
					for _, e := range f.Evidence {
						sb.WriteString(fmt.Sprintf("        - %s\n", e))
					}
				}
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
