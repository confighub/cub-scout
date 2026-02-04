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
	bundleFormat string
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

func init() {
	rootCmd.AddCommand(bundleCmd)
	bundleCmd.AddCommand(bundleInspectCmd)

	bundleInspectCmd.Flags().StringVar(&bundleFormat, "format", "ascii", "Output format: ascii, json")
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
