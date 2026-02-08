// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/confighub/cub-scout/pkg/agent"
)

var (
	driftFile      string
	driftNamespace string
	driftFormat    string
	driftFailOn    string
)

// Exit codes for drift command (CI contract)
const (
	// DriftExitOK indicates no failure triggered
	DriftExitOK = 0

	// DriftExitError indicates operational error (bad args, file read, etc.)
	DriftExitError = 1

	// DriftExitFailure indicates findings met the --fail-on threshold
	DriftExitFailure = 2
)

var driftCmd = &cobra.Command{
	Use:   "drift",
	Short: "Detect drift between desired and live state",
	Long: `Detect differences between desired state (from file/git) and live state (from cluster).

This command compares what should exist (desired) against what actually exists (live)
and reports any differences as drift findings.

v0.14.3 supports:
- spec.replicas comparison
- container image comparison

Examples:
  # Compare a YAML file against the cluster
  cub-scout drift --file manifests/deployment.yaml

  # Compare with namespace filter
  cub-scout drift --file manifests/ -n production

  # Output as JSON (for CI/automation)
  cub-scout drift --file manifests/deployment.yaml --format json

  # Fail CI if any warning or critical drift found
  cub-scout drift --file manifests/ --fail-on warning

  # Fail CI only on critical drift
  cub-scout drift --file manifests/ --fail-on critical

Output formats:
  ascii  Human-readable output (default)
  json   Machine-readable JSON (v0.14.3 schema)

Exit codes:
  0  No failure triggered (or no --fail-on specified)
  1  Operational error (bad arguments, file read failure, etc.)
  2  Findings met the --fail-on severity threshold

CI/Automation note:
  Exit status is determined solely by JSON facts (severity field) and the
  --fail-on flag. ASCII output does not affect CI behavior.
`,
	RunE: runDrift,
}

func init() {
	rootCmd.AddCommand(driftCmd)

	driftCmd.Flags().StringVar(&driftFile, "file", "", "YAML file or directory containing desired state (required)")
	driftCmd.Flags().StringVarP(&driftNamespace, "namespace", "n", "", "Namespace to compare (default: all namespaces)")
	driftCmd.Flags().StringVar(&driftFormat, "format", "ascii", "Output format: ascii, json")
	driftCmd.Flags().StringVar(&driftFailOn, "fail-on", "", "Exit non-zero if max severity >= level (info, warning, critical)")

	if err := driftCmd.MarkFlagRequired("file"); err != nil {
		panic(err)
	}
}

func runDrift(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Validate --fail-on if provided
	if driftFailOn != "" {
		if !isValidSeverityLevel(driftFailOn) {
			fmt.Fprintf(os.Stderr, "Error: invalid --fail-on value %q (valid: info, warning, critical)\n", driftFailOn)
			os.Exit(DriftExitError)
		}
	}

	var report agent.DriftReport

	// TEST HOOK: Load drift data from JSON file to bypass cluster access in tests.
	if driftJSON := os.Getenv("CUB_SCOUT_TEST_DRIFT_JSON"); driftJSON != "" {
		var err error
		report, err = loadDriftFromJSON(driftJSON)
		if err != nil {
			return err
		}
	} else {
		// Build Kubernetes clients
		kubeClient, dynamicClient, err := buildDriftClients()
		if err != nil {
			return fmt.Errorf("failed to create Kubernetes clients: %w", err)
		}

		// Configure comparator options
		opts := agent.DefaultDriftOptions()
		opts.Namespace = driftNamespace

		// Create comparator
		comparator := agent.NewDriftComparator(kubeClient, dynamicClient, opts)

		// Run comparison
		findings, err := comparator.CompareFromFile(ctx, driftFile)
		if err != nil {
			return fmt.Errorf("drift comparison failed: %w", err)
		}

		// Build report
		report = buildDriftReport(ctx, findings, driftFile, driftNamespace)
	}

	// Output based on format
	var outputErr error
	switch driftFormat {
	case "json":
		outputErr = outputDriftJSON(report)
	default:
		outputErr = outputDriftASCII(report)
	}

	if outputErr != nil {
		return outputErr
	}

	// Check exit condition based on --fail-on (JSON facts only)
	exitCode := computeDriftExitCode(report, driftFailOn)
	if exitCode != DriftExitOK {
		os.Exit(exitCode)
	}

	return nil
}

func buildDriftClients() (kubernetes.Interface, dynamic.Interface, error) {
	config, err := buildConfig()
	if err != nil {
		return nil, nil, err
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, fmt.Errorf("create kubernetes client: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, nil, fmt.Errorf("create dynamic client: %w", err)
	}

	return kubeClient, dynamicClient, nil
}

func buildDriftReport(ctx context.Context, findings []agent.DriftFinding, file, namespace string) agent.DriftReport {
	var ns *string
	if namespace != "" {
		ns = &namespace
	}

	// Get cluster name from context
	cluster := getCurrentClusterName()

	report := agent.DriftReport{
		Command: "drift",
		Context: agent.DriftContext{
			Cluster:   cluster,
			Namespace: ns,
			DesiredSource: agent.DriftSource{
				Type: "file",
				Ref:  file,
			},
			LiveSource: agent.DriftSource{
				Type: "cluster",
				Ref:  cluster,
			},
		},
		Findings: findings,
	}

	// Build summary
	report.Summary = agent.BuildDriftSummary(findings)

	return report
}

func outputDriftJSON(report agent.DriftReport) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func outputDriftASCII(report agent.DriftReport) error {
	// ASCII renderer implements f(JSON) + g.
	// See pkg/agent/drift_render.go for implementation.
	output := agent.RenderDriftASCII(report)
	fmt.Print(output)
	return nil
}

func loadDriftFromJSON(filename string) (agent.DriftReport, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return agent.DriftReport{}, fmt.Errorf("read test fixture: %w", err)
	}

	var report agent.DriftReport
	if err := json.Unmarshal(data, &report); err != nil {
		return agent.DriftReport{}, fmt.Errorf("parse test fixture: %w", err)
	}

	return report, nil
}

// isValidSeverityLevel checks if a severity level string is valid.
func isValidSeverityLevel(level string) bool {
	switch level {
	case "info", "warning", "critical":
		return true
	}
	return false
}

// computeDriftExitCode determines the exit code based on findings and --fail-on.
// This uses JSON facts (severity field) only - ASCII output does not affect this.
// Contract: Leak Test compliance - exit behavior depends only on structural facts.
func computeDriftExitCode(report agent.DriftReport, failOn string) int {
	// No --fail-on specified = pure reporting mode, always exit 0
	if failOn == "" {
		return DriftExitOK
	}

	// No findings = no failure
	if len(report.Findings) == 0 {
		return DriftExitOK
	}

	// Compute max severity from findings (JSON facts)
	maxSeverity := getMaxSeverity(report.Findings)

	// Compare max severity against threshold
	if severityMeetsThreshold(maxSeverity, failOn) {
		return DriftExitFailure
	}

	return DriftExitOK
}

// getMaxSeverity returns the highest severity from findings.
// Severity ranking: critical > warning > info
func getMaxSeverity(findings []agent.DriftFinding) agent.DriftSeverity {
	var max agent.DriftSeverity = agent.DriftSeverityInfo

	for _, f := range findings {
		if severityRank(f.Severity) > severityRank(max) {
			max = f.Severity
		}
	}

	return max
}

// severityRank returns a numeric rank for severity comparison.
// Higher rank = more severe.
func severityRank(s agent.DriftSeverity) int {
	switch s {
	case agent.DriftSeverityCritical:
		return 3
	case agent.DriftSeverityWarning:
		return 2
	case agent.DriftSeverityInfo:
		return 1
	}
	return 0
}

// severityMeetsThreshold returns true if maxSeverity >= threshold.
func severityMeetsThreshold(maxSeverity agent.DriftSeverity, threshold string) bool {
	thresholdRank := 0
	switch threshold {
	case "critical":
		thresholdRank = 3
	case "warning":
		thresholdRank = 2
	case "info":
		thresholdRank = 1
	}

	return severityRank(maxSeverity) >= thresholdRank
}

func getCurrentClusterName() string {
	// Try to get from kubeconfig context
	config, err := buildConfig()
	if err != nil {
		return "unknown"
	}

	// The config doesn't directly expose context name, use a simple approach
	if config.Host != "" {
		// Extract cluster identifier from host
		return extractClusterFromHost(config.Host)
	}

	return "unknown"
}

func extractClusterFromHost(host string) string {
	// Simple extraction - in practice you'd want to use the actual context name
	// For now, return a sanitized version of the host
	if host == "" {
		return "unknown"
	}
	// Remove protocol and port for display
	name := host
	if idx := len("https://"); len(name) > idx && name[:idx] == "https://" {
		name = name[idx:]
	}
	if idx := len("http://"); len(name) > idx && name[:idx] == "http://" {
		name = name[idx:]
	}
	// Truncate at port or path
	for _, sep := range []string{":", "/"} {
		if idx := len(name); idx > 0 {
			for i, c := range name {
				if string(c) == sep {
					name = name[:i]
					break
				}
			}
		}
	}
	return name
}
