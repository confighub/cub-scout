// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/confighub/cub-scout/pkg/agent"
)

var (
	bundleFormat  string
	bundleFailOn  string
	bundleSection string
)

var bundleCmd = &cobra.Command{
	Use:   "bundle",
	Short: "Work with debug bundles",
	Long: `Work with debug bundles for offline inspection and sharing.

Debug bundles are portable snapshots of debugging context that can be
inspected offline or shared across time and people.

A bundle contains:
- metadata.json: Bundle version, target, creation time, tool version
- session.json: Debug session data (if captured)
- drift.json: Drift findings (if captured)
- events.json: Timeline events (if captured)
- logs.json: Container logs (if captured)
- README.md: Human-readable summary

Bundles are pure packaging of existing facts — no new interpretation.

Commands:
  inspect  Show bundle metadata and contents summary
  replay   Re-render bundle contents with existing renderers

Examples:
  # Inspect a bundle
  cub-scout bundle inspect ./debug-bundle-2024-01-15

  # Replay bundle as ASCII
  cub-scout bundle replay ./debug-bundle-2024-01-15

  # Replay bundle as JSON
  cub-scout bundle replay ./debug-bundle-2024-01-15 --format json
`,
}

var bundleInspectCmd = &cobra.Command{
	Use:   "inspect <path>",
	Short: "Show bundle metadata and contents",
	Long: `Inspect a debug bundle and show its metadata and contents summary.

This command reads the bundle metadata and displays:
- Bundle format version
- cub-scout version that created the bundle
- Creation timestamp (captured, not current time)
- Target resource (kind, name, namespace)
- Optional label
- Contents summary (which files are present, counts)

Output is deterministic: same bundle always produces identical output.

Examples:
  # Inspect a bundle
  cub-scout bundle inspect ./debug-bundle-2024-01-15

  # Output as JSON
  cub-scout bundle inspect ./debug-bundle-2024-01-15 --format json
`,
	Args: cobra.ExactArgs(1),
	RunE: runBundleInspect,
}

var bundleReplayCmd = &cobra.Command{
	Use:   "replay <path>",
	Short: "Re-render bundle contents with existing renderers",
	Long: `Replay a debug bundle by re-running existing renderers against captured facts.

This command reads the bundle contents and renders them using the same
renderers as live commands. Useful for offline analysis or sharing.

Replay does NOT:
- Contact the cluster
- Consult git or filesystem (beyond the bundle)
- Generate new timestamps (uses captured times)

Supported sections:
- drift: Re-render drift findings (default)
- correlation: Re-render drift-debug correlation narrative

Output is deterministic: same bundle always produces identical output.

Exit codes (when --fail-on is used):
  0  No failure triggered (or no --fail-on specified)
  1  Operational error (bad arguments, read failure, etc.)
  2  Findings met the --fail-on severity threshold

Examples:
  # Replay drift findings as ASCII
  cub-scout bundle replay ./debug-bundle-2024-01-15

  # Replay as JSON
  cub-scout bundle replay ./debug-bundle-2024-01-15 --format json

  # Replay correlation narrative
  cub-scout bundle replay ./debug-bundle-2024-01-15 --section correlation

  # CI mode: fail on warning or higher
  cub-scout bundle replay ./debug-bundle-2024-01-15 --fail-on warning
`,
	Args: cobra.ExactArgs(1),
	RunE: runBundleReplay,
}

// BundleReplayExitOK indicates no failure triggered
const BundleReplayExitOK = 0

// BundleReplayExitError indicates operational error
const BundleReplayExitError = 1

// BundleReplayExitFailure indicates findings met threshold
const BundleReplayExitFailure = 2

func init() {
	rootCmd.AddCommand(bundleCmd)
	bundleCmd.AddCommand(bundleInspectCmd)
	bundleCmd.AddCommand(bundleReplayCmd)

	bundleInspectCmd.Flags().StringVar(&bundleFormat, "format", "ascii", "Output format: ascii, json")

	bundleReplayCmd.Flags().StringVar(&bundleFormat, "format", "ascii", "Output format: ascii, json")
	bundleReplayCmd.Flags().StringVar(&bundleFailOn, "fail-on", "", "Exit non-zero if max severity >= level (info, warning, critical)")
	bundleReplayCmd.Flags().StringVar(&bundleSection, "section", "drift", "Section to replay: drift, correlation")
}

func runBundleInspect(cmd *cobra.Command, args []string) error {
	bundlePath := args[0]

	// Read the bundle
	reader := agent.NewBundleReader()
	bundle, err := reader.Read(bundlePath)
	if err != nil {
		return fmt.Errorf("failed to read bundle: %w", err)
	}

	// Get summary
	summary := agent.Summarize(bundle)

	switch bundleFormat {
	case "json":
		return renderBundleInspectJSON(bundle, summary)
	case "ascii":
		return renderBundleInspectASCII(bundle, summary, bundlePath)
	default:
		return fmt.Errorf("unknown format: %s (valid: ascii, json)", bundleFormat)
	}
}

// BundleInspectOutput is the JSON output for bundle inspect.
// All fields are derived from the bundle — no new facts.
type BundleInspectOutput struct {
	// Path is the bundle path that was inspected
	Path string `json:"path"`

	// FormatVersion is the bundle format version
	FormatVersion string `json:"formatVersion"`

	// CubScoutVersion is the tool version that created the bundle
	CubScoutVersion string `json:"cubScoutVersion"`

	// CreatedAt is when the bundle was created (ISO 8601)
	CreatedAt string `json:"createdAt"`

	// Label is the optional user-provided label
	Label string `json:"label,omitempty"`

	// Target describes what was debugged
	Target agent.BundleTarget `json:"target"`

	// Contents describes what's in the bundle
	Contents BundleInspectContents `json:"contents"`
}

// BundleInspectContents provides counts for bundle contents.
type BundleInspectContents struct {
	HasSession bool `json:"hasSession"`
	HasDrift   bool `json:"hasDrift"`
	HasEvents  bool `json:"hasEvents"`
	HasLogs    bool `json:"hasLogs"`

	// Counts are only present when the corresponding Has* is true
	DriftCount int `json:"driftCount,omitempty"`
	EventCount int `json:"eventCount,omitempty"`
	LogCount   int `json:"logCount,omitempty"`
}

func renderBundleInspectJSON(bundle *agent.DebugBundle, summary agent.BundleSummary) error {
	output := BundleInspectOutput{
		FormatVersion:   bundle.Metadata.FormatVersion,
		CubScoutVersion: bundle.Metadata.CubScoutVersion,
		CreatedAt:       bundle.Metadata.CreatedAt.Format("2006-01-02T15:04:05Z"),
		Label:           bundle.Metadata.Label,
		Target:          bundle.Metadata.Target,
		Contents: BundleInspectContents{
			HasSession: summary.SessionPresent,
			HasDrift:   summary.DriftCount > 0,
			HasEvents:  summary.EventCount > 0,
			HasLogs:    summary.LogCount > 0,
			DriftCount: summary.DriftCount,
			EventCount: summary.EventCount,
			LogCount:   summary.LogCount,
		},
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

func renderBundleInspectASCII(bundle *agent.DebugBundle, summary agent.BundleSummary, path string) error {
	var b strings.Builder

	// Header
	b.WriteString("Debug Bundle Inspection\n")
	b.WriteString(strings.Repeat("─", 50))
	b.WriteString("\n\n")

	// Metadata section
	b.WriteString("Metadata\n")
	b.WriteString(fmt.Sprintf("  Path:            %s\n", path))
	b.WriteString(fmt.Sprintf("  Format version:  %s\n", bundle.Metadata.FormatVersion))
	b.WriteString(fmt.Sprintf("  Created by:      cub-scout %s\n", bundle.Metadata.CubScoutVersion))
	b.WriteString(fmt.Sprintf("  Created at:      %s\n", bundle.Metadata.CreatedAt.Format("2006-01-02 15:04:05 UTC")))
	if bundle.Metadata.Label != "" {
		b.WriteString(fmt.Sprintf("  Label:           %s\n", bundle.Metadata.Label))
	}
	b.WriteString("\n")

	// Target section
	b.WriteString("Target\n")
	b.WriteString(fmt.Sprintf("  Kind:            %s\n", bundle.Metadata.Target.Kind))
	b.WriteString(fmt.Sprintf("  Name:            %s\n", bundle.Metadata.Target.Name))
	if bundle.Metadata.Target.Namespace != "" {
		b.WriteString(fmt.Sprintf("  Namespace:       %s\n", bundle.Metadata.Target.Namespace))
	}
	if bundle.Metadata.Target.Cluster != "" {
		b.WriteString(fmt.Sprintf("  Cluster:         %s\n", bundle.Metadata.Target.Cluster))
	}
	b.WriteString("\n")

	// Contents section
	b.WriteString("Contents\n")
	b.WriteString(contentsLine("session.json", summary.SessionPresent, 0))
	b.WriteString(contentsLine("drift.json", summary.DriftCount > 0, summary.DriftCount))
	b.WriteString(contentsLine("events.json", summary.EventCount > 0, summary.EventCount))
	b.WriteString(contentsLine("logs.json", summary.LogCount > 0, summary.LogCount))
	b.WriteString("\n")

	// Summary counts
	b.WriteString("Summary\n")
	counts := []string{}
	if summary.SessionPresent {
		counts = append(counts, "session")
	}
	if summary.DriftCount > 0 {
		counts = append(counts, fmt.Sprintf("%d drift finding(s)", summary.DriftCount))
	}
	if summary.EventCount > 0 {
		counts = append(counts, fmt.Sprintf("%d event(s)", summary.EventCount))
	}
	if summary.LogCount > 0 {
		counts = append(counts, fmt.Sprintf("%d log result(s)", summary.LogCount))
	}
	if len(counts) == 0 {
		counts = append(counts, "metadata only")
	}
	// Sort for determinism (session always first since it's a keyword, rest alphabetically)
	sort.Strings(counts)
	b.WriteString(fmt.Sprintf("  Contains: %s\n", strings.Join(counts, ", ")))

	fmt.Print(b.String())
	return nil
}

func contentsLine(filename string, present bool, count int) string {
	status := "✗"
	if present {
		status = "✓"
	}
	if count > 0 {
		return fmt.Sprintf("  %s %-15s (%d items)\n", status, filename, count)
	}
	return fmt.Sprintf("  %s %-15s\n", status, filename)
}

func runBundleReplay(cmd *cobra.Command, args []string) error {
	bundlePath := args[0]

	// Validate --fail-on if provided
	if bundleFailOn != "" {
		if !isValidReplaySeverity(bundleFailOn) {
			fmt.Fprintf(os.Stderr, "Error: invalid --fail-on value %q (valid: info, warning, critical)\n", bundleFailOn)
			os.Exit(BundleReplayExitError)
		}
	}

	// Validate --section
	if bundleSection != "drift" && bundleSection != "correlation" {
		fmt.Fprintf(os.Stderr, "Error: invalid --section value %q (valid: drift, correlation)\n", bundleSection)
		os.Exit(BundleReplayExitError)
	}

	// Read the bundle
	reader := agent.NewBundleReader()
	bundle, err := reader.Read(bundlePath)
	if err != nil {
		return fmt.Errorf("failed to read bundle: %w", err)
	}

	// Dispatch to section-specific replay
	switch bundleSection {
	case "drift":
		return replayDrift(bundle)
	case "correlation":
		return replayCorrelation(bundle)
	default:
		return fmt.Errorf("unknown section: %s", bundleSection)
	}
}

func replayDrift(bundle *agent.DebugBundle) error {
	// Check if bundle has drift findings
	if len(bundle.DriftFindings) == 0 {
		fmt.Println("No drift findings in bundle.")
		return nil
	}

	// Build a DriftReport from bundle contents
	report := agent.DriftReport{
		Command:  "bundle replay",
		Findings: bundle.DriftFindings,
		Context: agent.DriftContext{
			DesiredSource: agent.DriftSource{
				Type: "bundle",
				Ref:  bundle.Metadata.Target.Name,
			},
			LiveSource: agent.DriftSource{
				Type: "bundle",
				Ref:  "(captured)",
			},
		},
		Summary: buildDriftSummaryFromFindings(bundle.DriftFindings),
	}

	// Output based on format
	switch bundleFormat {
	case "json":
		return outputReplayDriftJSON(report)
	case "ascii":
		return outputReplayDriftASCII(report)
	default:
		return fmt.Errorf("unknown format: %s (valid: ascii, json)", bundleFormat)
	}
}

func outputReplayDriftJSON(report agent.DriftReport) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	// Check exit condition based on --fail-on (JSON facts only)
	exitCode := computeReplayExitCode(report.Findings, bundleFailOn)
	if exitCode != BundleReplayExitOK {
		os.Exit(exitCode)
	}

	return nil
}

func outputReplayDriftASCII(report agent.DriftReport) error {
	// Use the same renderer as live drift command
	output := agent.RenderDriftASCII(report)
	fmt.Print(output)

	// Check exit condition based on --fail-on (JSON facts only)
	exitCode := computeReplayExitCode(report.Findings, bundleFailOn)
	if exitCode != BundleReplayExitOK {
		os.Exit(exitCode)
	}

	return nil
}

func replayCorrelation(bundle *agent.DebugBundle) error {
	// Check if bundle has drift findings
	if len(bundle.DriftFindings) == 0 && len(bundle.Events) == 0 {
		fmt.Println("No drift findings or events in bundle for correlation.")
		return nil
	}

	// Build correlation from bundle contents
	corr := agent.DriftCorrelation{
		ObjectID:   fmt.Sprintf("%s:%s/%s", bundle.Metadata.Target.Kind, bundle.Metadata.Target.Namespace, bundle.Metadata.Target.Name),
		Findings:   bundle.DriftFindings,
		Events:     bundle.Events,
		Logs:       bundle.Logs,
		HasDrift:   len(bundle.DriftFindings) > 0,
		HasFailure: hasFailureSignalsInBundle(bundle.Events, bundle.Logs),
	}

	// Output based on format
	switch bundleFormat {
	case "json":
		return outputReplayCorrelationJSON(corr)
	case "ascii":
		return outputReplayCorrelationASCII(corr)
	default:
		return fmt.Errorf("unknown format: %s (valid: ascii, json)", bundleFormat)
	}
}

func outputReplayCorrelationJSON(corr agent.DriftCorrelation) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(corr)
}

func outputReplayCorrelationASCII(corr agent.DriftCorrelation) error {
	output := agent.RenderCorrelationASCII(corr)
	fmt.Print(output)
	return nil
}

// hasFailureSignalsInBundle detects failure signals from events and logs.
// This mirrors the logic in drift_correlation.go but works on bundle data.
func hasFailureSignalsInBundle(events []agent.TimelineEvent, logs []agent.ContainerLogResult) bool {
	// Check events for warnings
	for _, e := range events {
		if e.Type == "Warning" || e.Severity == "error" || e.Severity == "warning" {
			return true
		}
	}

	// Check logs for errors
	for _, l := range logs {
		if l.Error != "" {
			return true
		}
		for _, p := range l.Patterns {
			if p.Type == "error" || p.Type == "panic" || p.Type == "fatal" {
				return true
			}
		}
	}

	return false
}

// isValidReplaySeverity checks if a severity level string is valid.
func isValidReplaySeverity(level string) bool {
	switch level {
	case "info", "warning", "critical":
		return true
	}
	return false
}

// computeReplayExitCode determines exit code based on findings and --fail-on.
func computeReplayExitCode(findings []agent.DriftFinding, failOn string) int {
	// No --fail-on specified = pure reporting mode, always exit 0
	if failOn == "" {
		return BundleReplayExitOK
	}

	// No findings = no failure
	if len(findings) == 0 {
		return BundleReplayExitOK
	}

	// Compute max severity from findings (JSON facts)
	maxSeverity := getReplayMaxSeverity(findings)

	// Compare max severity against threshold
	if replaySeverityMeetsThreshold(maxSeverity, failOn) {
		return BundleReplayExitFailure
	}

	return BundleReplayExitOK
}

// getReplayMaxSeverity returns the highest severity from findings.
func getReplayMaxSeverity(findings []agent.DriftFinding) agent.DriftSeverity {
	var max agent.DriftSeverity = agent.DriftSeverityInfo
	for _, f := range findings {
		if replaySeverityRank(f.Severity) > replaySeverityRank(max) {
			max = f.Severity
		}
	}
	return max
}

// replaySeverityRank returns numeric rank for severity comparison.
func replaySeverityRank(s agent.DriftSeverity) int {
	switch s {
	case agent.DriftSeverityCritical:
		return 3
	case agent.DriftSeverityWarning:
		return 2
	case agent.DriftSeverityInfo:
		return 1
	default:
		return 0
	}
}

// replaySeverityMeetsThreshold checks if max severity meets or exceeds threshold.
func replaySeverityMeetsThreshold(max agent.DriftSeverity, threshold string) bool {
	thresholdRank := 0
	switch threshold {
	case "critical":
		thresholdRank = 3
	case "warning":
		thresholdRank = 2
	case "info":
		thresholdRank = 1
	}
	return replaySeverityRank(max) >= thresholdRank
}

// buildDriftSummaryFromFindings builds a DriftSummary from findings.
func buildDriftSummaryFromFindings(findings []agent.DriftFinding) agent.DriftSummary {
	// Count by severity
	severityCounts := make(map[agent.DriftSeverity]int)
	classificationCounts := make(map[agent.DriftClassification]int)

	for _, f := range findings {
		severityCounts[f.Severity]++
		classificationCounts[f.Classification]++
	}

	// Build severity counts in deterministic order
	bySeverity := []agent.DriftSeverityCount{}
	for _, sev := range []agent.DriftSeverity{agent.DriftSeverityCritical, agent.DriftSeverityWarning, agent.DriftSeverityInfo} {
		if count, ok := severityCounts[sev]; ok && count > 0 {
			bySeverity = append(bySeverity, agent.DriftSeverityCount{Severity: sev, Count: count})
		}
	}

	// Build classification counts in deterministic order (use canonical order)
	byClassification := []agent.DriftClassificationCount{}
	for _, class := range agent.CanonicalClassificationOrder {
		if count, ok := classificationCounts[class]; ok && count > 0 {
			byClassification = append(byClassification, agent.DriftClassificationCount{Classification: class, Count: count})
		}
	}

	return agent.DriftSummary{
		TotalFindings:    len(findings),
		BySeverity:       bySeverity,
		ByClassification: byClassification,
	}
}
