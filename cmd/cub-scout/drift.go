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

Output formats:
  ascii  Human-readable output (default)
  json   Machine-readable JSON (v0.14.3 schema)
`,
	RunE: runDrift,
}

func init() {
	rootCmd.AddCommand(driftCmd)

	driftCmd.Flags().StringVar(&driftFile, "file", "", "YAML file or directory containing desired state (required)")
	driftCmd.Flags().StringVarP(&driftNamespace, "namespace", "n", "", "Namespace to compare (default: all namespaces)")
	driftCmd.Flags().StringVar(&driftFormat, "format", "ascii", "Output format: ascii, json")

	driftCmd.MarkFlagRequired("file")
}

func runDrift(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// TEST HOOK: Load drift data from JSON file to bypass cluster access in tests.
	if driftJSON := os.Getenv("CUB_SCOUT_TEST_DRIFT_JSON"); driftJSON != "" {
		return loadAndRenderDriftFromJSON(driftJSON)
	}

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
	report := buildDriftReport(ctx, findings, driftFile, driftNamespace)

	// Output based on format
	switch driftFormat {
	case "json":
		return outputDriftJSON(report)
	default:
		return outputDriftASCII(report)
	}
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
	// v0.14.3 PR2: JSON only, ASCII placeholder
	// ASCII rendering will be implemented in PR4
	fmt.Printf("Drift Report\n")
	fmt.Printf("============\n\n")
	fmt.Printf("Cluster: %s\n", report.Context.Cluster)
	fmt.Printf("Source:  %s (%s)\n", report.Context.DesiredSource.Ref, report.Context.DesiredSource.Type)
	fmt.Printf("\n")

	if len(report.Findings) == 0 {
		fmt.Printf("No drift detected.\n")
		return nil
	}

	fmt.Printf("Found %d drift finding(s):\n\n", len(report.Findings))

	for _, f := range report.Findings {
		severity := string(f.Severity)
		switch f.Severity {
		case agent.DriftSeverityCritical:
			severity = "[CRITICAL]"
		case agent.DriftSeverityWarning:
			severity = "[WARNING]"
		case agent.DriftSeverityInfo:
			severity = "[INFO]"
		}

		fmt.Printf("%s %s\n", severity, f.ObjectID)
		fmt.Printf("  Path:     %s\n", f.Path)
		fmt.Printf("  Desired:  %v\n", f.Desired)
		fmt.Printf("  Live:     %v\n", f.Live)
		fmt.Printf("  Class:    %s\n", f.Classification)
		fmt.Printf("\n")
	}

	// Summary
	fmt.Printf("Summary: %d finding(s) across %d object(s)\n",
		report.Summary.TotalFindings, report.Summary.AffectedObjects)

	return nil
}

func loadAndRenderDriftFromJSON(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read test fixture: %w", err)
	}

	var report agent.DriftReport
	if err := json.Unmarshal(data, &report); err != nil {
		return fmt.Errorf("parse test fixture: %w", err)
	}

	switch driftFormat {
	case "json":
		return outputDriftJSON(report)
	default:
		return outputDriftASCII(report)
	}
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
