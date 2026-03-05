package scan

import (
	"fmt"
	"time"
)

// NormalizedSchemaVersion is the schema version for normalized findings output.
const NormalizedSchemaVersion = "scan.normalized.v1"

// NormalizedFinding is the universal finding shape across all scanner tracks.
// Each field maps deterministically from the source finding type — no inference.
type NormalizedFinding struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Category    string `json:"category"`
	Track       string `json:"track"` // "kyverno", "state", "timing-bomb", "unresolved", "dangling", "static", "lifecycle"
	Severity    string `json:"severity"`
	Resource    string `json:"resource"`
	Namespace   string `json:"namespace"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
	Command     string `json:"command,omitempty"`
}

// NormalizedResult wraps all normalized findings with metadata.
type NormalizedResult struct {
	SchemaVersion string              `json:"schema_version"`
	ScannedAt     time.Time           `json:"scanned_at"`
	Findings      []NormalizedFinding `json:"findings"`
	Summary       NormalizedSummary   `json:"summary"`
}

// NormalizedSummary counts findings by severity.
type NormalizedSummary struct {
	Critical int `json:"critical"`
	Warning  int `json:"warning"`
	Info     int `json:"info"`
	Total    int `json:"total"`
}

// Normalize converts a CombinedResult into a NormalizedResult.
// The mapping is deterministic: same input always produces same output.
func Normalize(combined *CombinedResult) *NormalizedResult {
	if combined == nil {
		return &NormalizedResult{
			SchemaVersion: NormalizedSchemaVersion,
			Findings:      []NormalizedFinding{},
		}
	}

	var findings []NormalizedFinding

	// Kyverno findings
	if combined.Kyverno != nil {
		for _, f := range combined.Kyverno.Findings {
			id := f.PolicyID
			if id == "" {
				id = f.ID
			}
			findings = append(findings, NormalizedFinding{
				ID:        id,
				Title:     f.PolicyName,
				Category:  f.Category,
				Track:     "kyverno",
				Severity:  f.Severity,
				Resource:  f.Resource,
				Namespace: f.Namespace,
				Message:   f.Message,
			})
		}
	}

	// State findings (stuck reconciliations)
	if combined.State != nil {
		for _, f := range combined.State.Findings {
			findings = append(findings, NormalizedFinding{
				ID:          f.CCVEID,
				Title:       fmt.Sprintf("%s/%s stuck", f.Kind, f.Name),
				Category:    f.Category,
				Track:       "state",
				Severity:    f.Severity,
				Resource:    fmt.Sprintf("%s/%s", f.Kind, f.Name),
				Namespace:   f.Namespace,
				Message:     f.Message,
				Remediation: f.Remediation,
				Command:     f.Command,
			})
		}

		// Runtime findings (pod-level failures from scan --state)
		for _, f := range combined.State.RuntimeFindings {
			findings = append(findings, NormalizedFinding{
				ID:          f.CCVEID,
				Title:       fmt.Sprintf("Pod/%s %s", f.Name, f.FailureType),
				Category:    f.Category,
				Track:       "runtime",
				Severity:    f.Severity,
				Resource:    fmt.Sprintf("%s/%s", f.Kind, f.Name),
				Namespace:   f.Namespace,
				Message:     f.Message,
				Remediation: f.Remediation,
				Command:     f.Command,
			})
		}
	}

	// Timing bomb findings
	if combined.TimingBombs != nil {
		for _, f := range combined.TimingBombs.Findings {
			findings = append(findings, NormalizedFinding{
				ID:          f.CCVEID,
				Title:       fmt.Sprintf("%s/%s expires %s", f.Kind, f.Name, f.ExpiresIn),
				Category:    f.Category,
				Track:       "timing-bomb",
				Severity:    f.Severity,
				Resource:    fmt.Sprintf("%s/%s", f.Kind, f.Name),
				Namespace:   f.Namespace,
				Message:     f.Message,
				Remediation: f.Remediation,
				Command:     f.Command,
			})
		}
	}

	// Unresolved findings
	if combined.Unresolved != nil {
		for _, f := range combined.Unresolved.Findings {
			findings = append(findings, NormalizedFinding{
				ID:        f.CCVEID,
				Title:     fmt.Sprintf("%s unresolved %s", f.Source, f.FindingType),
				Category:  f.Category,
				Track:     "unresolved",
				Severity:  f.Severity,
				Resource:  fmt.Sprintf("%s/%s", f.Kind, f.Name),
				Namespace: f.Namespace,
				Message:   f.Message,
				Command:   f.Command,
			})
		}
	}

	// Dangling findings
	if combined.Dangling != nil {
		for _, f := range combined.Dangling.Findings {
			findings = append(findings, NormalizedFinding{
				ID:          f.CCVEID,
				Title:       fmt.Sprintf("Dangling %s/%s", f.Kind, f.Name),
				Category:    f.Category,
				Track:       "dangling",
				Severity:    f.Severity,
				Resource:    fmt.Sprintf("%s/%s", f.Kind, f.Name),
				Namespace:   f.Namespace,
				Message:     f.Message,
				Remediation: f.Remediation,
				Command:     f.Command,
			})
		}
	}

	// Static findings
	if combined.Static != nil {
		for _, f := range combined.Static.Findings {
			findings = append(findings, NormalizedFinding{
				ID:          f.CCVEID,
				Title:       f.Name,
				Category:    f.Category,
				Track:       "static",
				Severity:    f.Severity,
				Resource:    fmt.Sprintf("%s/%s", f.Kind, f.ResourceName),
				Namespace:   f.Namespace,
				Message:     f.Message,
				Remediation: f.Remediation,
			})
		}
	}

	// Lifecycle hazard findings
	if combined.LifecycleHazards != nil {
		for _, f := range combined.LifecycleHazards.Findings {
			findings = append(findings, NormalizedFinding{
				ID:          f.Rule,
				Title:       f.Risk,
				Category:    "LIFECYCLE",
				Track:       "lifecycle",
				Severity:    f.Severity,
				Resource:    f.Resource,
				Namespace:   f.Namespace,
				Message:     f.Risk,
				Remediation: f.Remediation,
			})
		}
	}

	// Build summary
	summary := NormalizedSummary{Total: len(findings)}
	for _, f := range findings {
		switch f.Severity {
		case "critical":
			summary.Critical++
		case "warning":
			summary.Warning++
		default:
			summary.Info++
		}
	}

	// Determine scannedAt from available results
	scannedAt := time.Now()
	if combined.State != nil && !combined.State.ScannedAt.IsZero() {
		scannedAt = combined.State.ScannedAt
	} else if combined.Kyverno != nil && !combined.Kyverno.ScannedAt.IsZero() {
		scannedAt = combined.Kyverno.ScannedAt
	} else if combined.Static != nil && !combined.Static.ScannedAt.IsZero() {
		scannedAt = combined.Static.ScannedAt
	}

	return &NormalizedResult{
		SchemaVersion: NormalizedSchemaVersion,
		ScannedAt:     scannedAt,
		Findings:      findings,
		Summary:       summary,
	}
}
