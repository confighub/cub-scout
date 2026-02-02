// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/confighub/cub-scout/internal/graph"
	"github.com/confighub/cub-scout/internal/patterns"
	"github.com/spf13/cobra"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

var patternsCmd = &cobra.Command{
	Use:   "patterns",
	Short: "Pattern detection engine (v0.7)",
	Long: `Pattern detection engine for analyzing resource graphs.

This is a v0.7 contract surface. It does not modify any v0.5 or v0.6 contracts.

Patterns are deterministic checks that analyze the resource graph and report
findings. Each pattern has a unique ID, description, and detection logic.

Available subcommands:
  list      List all registered patterns
  detect    Run pattern detection against the cluster
  explain   Explain a specific pattern with results`,
}

var patternsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered patterns",
	Long: `List all registered patterns with their IDs and names.

Output is deterministic: patterns are listed in sorted order by ID.`,
	RunE: runPatternsList,
}

var patternsDetectCmd = &cobra.Command{
	Use:   "detect",
	Short: "Run pattern detection",
	Long: `Run all registered patterns against the resource graph.

The command builds a graph from the cluster (or uses --empty for testing)
and runs all patterns against it. Results are reported with status and findings.

Exit codes:
  0  All patterns passed
  2  Usage error (invalid arguments)
  4  One or more patterns failed

Examples:
  cub-scout patterns detect           # Run all patterns against cluster
  cub-scout patterns detect -n prod   # Run against specific namespace
  cub-scout patterns detect --empty   # Run against empty graph (for testing)
  cub-scout patterns detect --json    # Output as JSON`,
	RunE:              runPatternsDetect,
	SilenceUsage:      true,
	SilenceErrors:     true,
}

var patternsExplainCmd = &cobra.Command{
	Use:   "explain <pattern-id>",
	Short: "Explain a specific pattern",
	Long: `Explain a specific pattern and show its detection results.

Shows the pattern's description, category, and current detection results
against the resource graph.

Exit codes:
  0  Success
  2  Usage error (missing argument)
  3  Unknown pattern ID
  4  Pattern failed

Examples:
  cub-scout patterns explain k8s.ownership_chain_complete
  cub-scout patterns explain gitops.controller_presence -n prod`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			fmt.Fprintln(os.Stderr, "Error: requires exactly 1 argument: <pattern-id>")
			os.Exit(2)
		}
		return nil
	},
	RunE:              runPatternsExplain,
	SilenceUsage:      true,
	SilenceErrors:     true,
	DisableFlagsInUseLine: true,
}

// Flag variables for patterns commands (scoped to avoid conflicts)
var (
	patternsDetectNamespace string
	patternsDetectEmpty     bool
	patternsDetectJSON      bool
	patternsExplainNamespace string
	patternsExplainEmpty    bool
)

func init() {
	rootCmd.AddCommand(patternsCmd)
	patternsCmd.AddCommand(patternsListCmd)
	patternsCmd.AddCommand(patternsDetectCmd)
	patternsCmd.AddCommand(patternsExplainCmd)

	// Detect flags
	patternsDetectCmd.Flags().StringVarP(&patternsDetectNamespace, "namespace", "n", "", "Namespace to collect (empty = all namespaces)")
	patternsDetectCmd.Flags().BoolVar(&patternsDetectEmpty, "empty", false, "Use empty graph (skip cluster collection)")
	patternsDetectCmd.Flags().BoolVar(&patternsDetectJSON, "json", false, "Output as JSON")

	// Explain flags
	patternsExplainCmd.Flags().StringVarP(&patternsExplainNamespace, "namespace", "n", "", "Namespace to collect (empty = all namespaces)")
	patternsExplainCmd.Flags().BoolVar(&patternsExplainEmpty, "empty", false, "Use empty graph (skip cluster collection)")
}

func runPatternsList(cmd *cobra.Command, args []string) error {
	output := patterns.RenderList()
	fmt.Print(output)
	return nil
}

func runPatternsDetect(cmd *cobra.Command, args []string) error {
	// Build graph
	g, err := buildPatternsGraph(patternsDetectNamespace, patternsDetectEmpty)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Run detection
	result := patterns.DetectAll(g)

	// Render output
	if patternsDetectJSON {
		output, err := patterns.RenderJSON(result)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(output)
	} else {
		output := patterns.RenderText(result)
		fmt.Print(output)
	}

	// Exit with code 4 if any failures
	if patterns.AnyFail(result) {
		os.Exit(4)
	}

	return nil
}

func runPatternsExplain(cmd *cobra.Command, args []string) error {
	patternID := args[0]

	// Get pattern
	p := patterns.Get(patternID)
	if p == nil {
		fmt.Fprintf(os.Stderr, "Error: unknown pattern: %s\n", patternID)
		fmt.Fprintln(os.Stderr, "\nAvailable patterns:")
		for _, id := range patterns.IDs() {
			fmt.Fprintf(os.Stderr, "  - %s\n", id)
		}
		os.Exit(3)
	}

	// Build graph
	g, err := buildPatternsGraph(patternsExplainNamespace, patternsExplainEmpty)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Run single pattern
	pr := patterns.DetectOne(g, patternID)

	// Render output
	output := patterns.RenderExplain(p, pr)
	fmt.Print(output)

	// Exit with code 4 if pattern failed
	if pr != nil && pr.Status == patterns.StatusFail {
		os.Exit(4)
	}

	return nil
}

// buildPatternsGraph builds a graph for pattern detection.
// Collects K8s ownership chain + GitOps CRDs.
func buildPatternsGraph(namespace string, empty bool) (*graph.Graph, error) {
	// Get cluster name
	cluster := os.Getenv("CUB_SCOUT_TEST_CLUSTER")
	if cluster == "" {
		cluster = getCurrentContext()
	}

	// Create graph
	g := graph.NewGraph(cluster)

	// Skip collection if empty flag or test mode
	if empty || os.Getenv("CUB_SCOUT_TEST_TIME") != "" {
		// Override GeneratedAt for deterministic testing
		if testTime := os.Getenv("CUB_SCOUT_TEST_TIME"); testTime != "" {
			if t, err := time.Parse(time.RFC3339, testTime); err == nil {
				g.GeneratedAt = t
			}
		}
		return g, nil
	}

	// Build k8s config
	cfg, err := buildConfig()
	if err != nil {
		return g, nil // Return empty graph if no cluster access
	}

	ctx := context.Background()

	// Collect K8s ownership chain
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	collector := graph.NewCollector(client, cluster)
	if err := collector.CollectOwnershipChain(ctx, g, namespace); err != nil {
		// Log but don't fail - best effort
		fmt.Fprintf(os.Stderr, "Warning: failed to collect ownership chain: %v\n", err)
	}

	// Collect GitOps CRDs
	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		// Log but don't fail - best effort
		fmt.Fprintf(os.Stderr, "Warning: failed to create dynamic client: %v\n", err)
	} else {
		gitopsCollector := graph.NewGitOpsCollector(dynClient, cluster)
		if err := gitopsCollector.CollectGitOpsCRDs(ctx, g, namespace); err != nil {
			// Best effort - errors are expected when CRDs don't exist
			_ = err
		}
	}

	return g, nil
}
