// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var (
	debugNamespace      string
	debugFormat         string
	debugNonInteractive bool
)

var debugCmd = &cobra.Command{
	Use:   "debug [resource]",
	Short: "Guided GitOps debugging wizard",
	Long: `Guided GitOps debugging wizard for diagnosing pipeline issues.

This command walks you step-by-step through diagnosing why a workload
isn't working correctly. It shows:

  - Workload health (pod issues, restart counts)
  - Ownership chain (who manages this resource)
  - Pipeline health (Kustomization/HelmRelease/Application status)
  - Source health (GitRepository/OCIRepository status)
  - Root cause analysis with suggested fixes

The wizard includes inline explanations to help you understand
GitOps concepts like CrashLoopBackOff, reconciliation failures, etc.

Examples:
  # Interactive wizard - pick from unhealthy workloads
  cub-scout debug

  # Direct analysis of a specific workload
  cub-scout debug deployment/api-server -n production

  # Output as JSON (non-interactive)
  cub-scout debug deployment/api-server -n production --format json

  # Output as Markdown (non-interactive)
  cub-scout debug deployment/api-server -n production --format md
`,
	RunE: runDebug,
}

func init() {
	rootCmd.AddCommand(debugCmd)

	debugCmd.Flags().StringVarP(&debugNamespace, "namespace", "n", "", "Namespace of the resource (default: all namespaces)")
	debugCmd.Flags().StringVar(&debugFormat, "format", "ascii", "Output format: ascii, json, md")
	debugCmd.Flags().BoolVar(&debugNonInteractive, "non-interactive", false, "Run without interactive prompts (requires resource argument)")
}

func runDebug(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// TEST HOOK: Load debug session from JSON file for testing
	if debugJSON := os.Getenv("CUB_SCOUT_TEST_DEBUG_JSON"); debugJSON != "" {
		return loadAndRenderDebugFromJSON(debugJSON)
	}

	// If resource provided, run non-interactive analysis
	if len(args) > 0 {
		return runDebugNonInteractive(ctx, args[0], debugNamespace, debugFormat)
	}

	// If --non-interactive flag without resource, error
	if debugNonInteractive {
		return fmt.Errorf("--non-interactive requires a resource argument (e.g., deployment/api-server)")
	}

	// Run interactive TUI wizard
	return runDebugTUI(ctx)
}

// runDebugTUI launches the interactive debug wizard
func runDebugTUI(ctx context.Context) error {
	// Build Kubernetes clients
	cfg, err := buildConfig()
	if err != nil {
		return fmt.Errorf("failed to build kubernetes config: %w", err)
	}

	model, err := NewDebugModel(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize debug wizard: %w", err)
	}

	p := tea.NewProgram(model, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("debug wizard error: %w", err)
	}

	// Check if model exited with error
	if m, ok := finalModel.(DebugModel); ok && m.err != nil {
		return m.err
	}

	return nil
}

// runDebugNonInteractive analyzes a specific resource without TUI
func runDebugNonInteractive(ctx context.Context, resource, namespace, format string) error {
	// Parse resource kind/name
	kind, name, err := parseResourceArg(resource)
	if err != nil {
		return err
	}

	// Build Kubernetes clients
	cfg, err := buildConfig()
	if err != nil {
		return fmt.Errorf("failed to build kubernetes config: %w", err)
	}

	// Run debug analysis
	session, err := runDebugAnalysis(ctx, cfg, kind, name, namespace)
	if err != nil {
		return err
	}

	// Output based on format
	switch format {
	case "json":
		return outputDebugJSON(session)
	case "md":
		return outputDebugMarkdown(session)
	default:
		return outputDebugASCII(session)
	}
}

// parseResourceArg parses "kind/name" or "kind name" format
func parseResourceArg(resource string) (string, string, error) {
	// Try "kind/name" format first
	parts := splitResourceArg(resource)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid resource format: %s (expected kind/name)", resource)
	}
	return normalizeDebugKind(parts[0]), parts[1], nil
}

// splitResourceArg splits "kind/name" into parts
func splitResourceArg(resource string) []string {
	for _, sep := range []string{"/", " "} {
		if idx := indexString(resource, sep); idx > 0 {
			return []string{resource[:idx], resource[idx+1:]}
		}
	}
	return []string{resource}
}

// indexString returns the index of sep in s, or -1
func indexString(s, sep string) int {
	for i := 0; i < len(s); i++ {
		if s[i:i+len(sep)] == sep {
			return i
		}
	}
	return -1
}

// normalizeDebugKind converts short names to full kind names for debug command
func normalizeDebugKind(kind string) string {
	switch kind {
	case "deploy", "deployment", "deployments":
		return "Deployment"
	case "sts", "statefulset", "statefulsets":
		return "StatefulSet"
	case "ds", "daemonset", "daemonsets":
		return "DaemonSet"
	case "pod", "pods":
		return "Pod"
	case "svc", "service", "services":
		return "Service"
	case "ks", "kustomization", "kustomizations":
		return "Kustomization"
	case "hr", "helmrelease", "helmreleases":
		return "HelmRelease"
	case "app", "application", "applications":
		return "Application"
	default:
		return kind
	}
}

// loadAndRenderDebugFromJSON loads a debug session from JSON for testing
func loadAndRenderDebugFromJSON(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read debug JSON: %w", err)
	}

	var session DebugSession
	if err := json.Unmarshal(data, &session); err != nil {
		return fmt.Errorf("failed to parse debug JSON: %w", err)
	}

	// Render based on format
	switch debugFormat {
	case "json":
		return outputDebugJSON(&session)
	case "md":
		return outputDebugMarkdown(&session)
	default:
		return outputDebugASCII(&session)
	}
}

// outputDebugJSON outputs the session as JSON
func outputDebugJSON(session *DebugSession) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(session)
}

// outputDebugMarkdown outputs the session as Markdown
func outputDebugMarkdown(session *DebugSession) error {
	// Wrap ASCII output in code block for now
	fmt.Println("# Debug Analysis")
	fmt.Println()
	fmt.Println("```")
	if err := outputDebugASCII(session); err != nil {
		return err
	}
	fmt.Println("```")
	return nil
}

// outputDebugASCII outputs the session as colored ASCII
func outputDebugASCII(session *DebugSession) error {
	// Header
	fmt.Printf("DEBUG: %s/%s", session.Target.Kind, session.Target.Name)
	if session.Target.Namespace != "" {
		fmt.Printf(" in %s", session.Target.Namespace)
	}
	fmt.Println()
	fmt.Println()

	// Workload status
	if session.WorkloadStatus != nil {
		renderWorkloadStatus(session.WorkloadStatus)
	}

	// Ownership
	if session.OwnershipChain != nil {
		renderOwnership(session.OwnershipChain)
	}

	// Pipeline health
	if session.DeployerStatus != nil {
		renderDeployerStatus(session.DeployerStatus)
	}

	// Source health
	if session.SourceStatus != nil {
		renderSourceStatus(session.SourceStatus)
	}

	// Root cause
	if session.RootCause != nil {
		renderRootCause(session.RootCause)
	}

	return nil
}

// Placeholder render functions - will be implemented in debug_views.go
func renderWorkloadStatus(status *WorkloadHealthStatus) {
	fmt.Printf("Workload: %s/%s (%d/%d ready)\n",
		status.Kind, status.Name,
		status.ReadyReplicas, status.Replicas)
	for _, issue := range status.PodIssues {
		fmt.Printf("  - %s: %s\n", issue.PodName, issue.ContainerStatus)
	}
	fmt.Println()
}

func renderOwnership(chain *OwnershipChainResult) {
	fmt.Printf("Owner: %s\n", chain.Owner)
	if chain.OwnerDetails != nil {
		fmt.Printf("  Managed by: %s/%s\n", chain.OwnerDetails.Type, chain.OwnerDetails.Name)
	}
	fmt.Println()
}

func renderDeployerStatus(status *DeployerStatus) {
	icon := "✓"
	if !status.Ready {
		icon = "✗"
	}
	fmt.Printf("Pipeline: %s %s/%s\n", icon, status.Kind, status.Name)
	if status.Stage != "" && status.Stage != "healthy" {
		fmt.Printf("  Stage: %s\n", status.Stage)
		if status.Reason != "" {
			fmt.Printf("  Reason: %s\n", status.Reason)
		}
		if status.Message != "" {
			fmt.Printf("  Message: %s\n", status.Message)
		}
	}
	fmt.Println()
}

func renderSourceStatus(status *SourceStatus) {
	icon := "✓"
	if !status.Ready {
		icon = "✗"
	}
	fmt.Printf("Source: %s %s/%s\n", icon, status.Kind, status.Name)
	if status.URL != "" {
		fmt.Printf("  URL: %s\n", status.URL)
	}
	if !status.Ready {
		if status.Reason != "" {
			fmt.Printf("  Reason: %s\n", status.Reason)
		}
		if status.Message != "" {
			fmt.Printf("  Message: %s\n", status.Message)
		}
	}
	fmt.Println()
}

func renderRootCause(analysis *RootCauseAnalysis) {
	fmt.Println("─────────────────────────────────────────────────────────────")
	fmt.Println("ROOT CAUSE ANALYSIS")
	fmt.Println()
	fmt.Printf("Category: %s\n", analysis.Category)
	fmt.Printf("Stage: %s\n", analysis.Stage)
	fmt.Println()
	fmt.Printf("Summary: %s\n", analysis.Summary)
	fmt.Println()

	if len(analysis.ProbableCauses) > 0 {
		fmt.Println("Probable Causes:")
		for _, cause := range analysis.ProbableCauses {
			fmt.Printf("  - %s\n", cause)
		}
		fmt.Println()
	}

	if len(analysis.SuggestedFixes) > 0 {
		fmt.Println("Suggested Fixes:")
		for _, fix := range analysis.SuggestedFixes {
			fmt.Printf("  %s\n", fix)
		}
		fmt.Println()
	}
}
