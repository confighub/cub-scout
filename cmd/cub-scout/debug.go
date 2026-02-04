// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/confighub/cub-scout/pkg/agent"
)

var (
	debugNamespace      string
	debugFormat         string
	debugNonInteractive bool
	debugSaveBundleDir  string
	debugKustomizePath  string
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
	debugCmd.Flags().StringVar(&debugSaveBundleDir, "save-bundle", "", "Save debug bundle to directory (non-interactive only)")
	debugCmd.Flags().StringVar(&debugKustomizePath, "kustomize", "", "Kustomize overlay directory for attribution (requires --save-bundle)")
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

	// Save bundle if requested
	if debugSaveBundleDir != "" {
		if err := saveDebugBundle(ctx, cfg, session, debugSaveBundleDir); err != nil {
			return fmt.Errorf("failed to save bundle: %w", err)
		}
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

// saveDebugBundle creates and writes a debug bundle from the session
func saveDebugBundle(ctx context.Context, cfg *rest.Config, session *DebugSession, outputDir string) error {
	// Generate deterministic bundle directory name
	bundleDirName := generateBundleDirName(session.Target)
	bundlePath := filepath.Join(outputDir, bundleDirName)

	// Build the debug bundle
	bundle := buildDebugBundleFromSession(session)

	// Attempt to populate attribution if this is a Crossplane-managed resource
	if err := populateAttribution(ctx, cfg, session, bundle); err != nil {
		// Non-fatal: attribution is optional enrichment
		_ = err
	}

	// Enrich with kustomize overlay attribution if --kustomize provided
	if debugKustomizePath != "" {
		if err := populateKustomizeAttribution(session, bundle, debugKustomizePath); err != nil {
			// Non-fatal: kustomize attribution is optional enrichment
			_ = err
		}
	}

	// Write the bundle
	writer := agent.NewBundleWriter(BuildTag)
	if err := writer.Write(bundle, bundlePath); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Bundle saved to: %s\n", bundlePath)
	return nil
}

// populateKustomizeAttribution adds kustomize overlay ownership to the bundle
func populateKustomizeAttribution(session *DebugSession, bundle *agent.DebugBundle, kustomizePath string) error {
	// Build target ref
	targetRef := agent.AttributionRef{
		Kind:      session.Target.Kind,
		Name:      session.Target.Name,
		Namespace: session.Target.Namespace,
	}

	// Determine target node type:
	// - If bundle already has Crossplane attribution (has MR node), use NodeMR
	// - Otherwise use NodeK8sObject for generic targets
	targetNodeType := agent.NodeK8sObject
	if bundle.Attribution != nil {
		for _, n := range bundle.Attribution.Nodes {
			if n.Type == agent.NodeMR {
				targetNodeType = agent.NodeMR
				break
			}
		}
	}

	// Build kustomize attribution fragment
	kustomizeGraph, err := agent.BuildKustomizeOverlayOwnership(kustomizePath, targetRef, targetNodeType)
	if err != nil {
		return err
	}

	// Merge into existing attribution
	bundle.Attribution = agent.MergeAttributionGraphs(bundle.Attribution, kustomizeGraph)
	return nil
}

// generateBundleDirName creates a deterministic bundle directory name from target
func generateBundleDirName(target ResourceRef) string {
	// Format: <kind>-<namespace>-<name> or <kind>-<name> if no namespace
	name := strings.ToLower(target.Kind)
	if target.Namespace != "" {
		name += "-" + target.Namespace
	}
	name += "-" + target.Name
	return name
}

// buildDebugBundleFromSession converts a DebugSession to a DebugBundle
func buildDebugBundleFromSession(session *DebugSession) *agent.DebugBundle {
	bundle := &agent.DebugBundle{
		Metadata: agent.BundleMetadata{
			Target: agent.BundleTarget{
				Kind:      session.Target.Kind,
				Name:      session.Target.Name,
				Namespace: session.Target.Namespace,
			},
		},
	}

	// Populate session data
	bundle.Session = &agent.DebugSessionData{
		Target: agent.BundleTarget{
			Kind:      session.Target.Kind,
			Name:      session.Target.Name,
			Namespace: session.Target.Namespace,
		},
		StartedAt:   session.StartedAt,
		CompletedAt: session.CompletedAt,
	}

	// Convert workload health
	if session.WorkloadStatus != nil {
		bundle.Session.WorkloadHealth = &agent.WorkloadHealthSnapshot{
			Kind:                session.WorkloadStatus.Kind,
			Name:                session.WorkloadStatus.Name,
			Namespace:           session.WorkloadStatus.Namespace,
			Replicas:            session.WorkloadStatus.Replicas,
			ReadyReplicas:       session.WorkloadStatus.ReadyReplicas,
			UnavailableReplicas: session.WorkloadStatus.UnavailableReplicas,
		}
		for _, issue := range session.WorkloadStatus.PodIssues {
			bundle.Session.WorkloadHealth.PodIssues = append(bundle.Session.WorkloadHealth.PodIssues, agent.PodIssue{
				PodName:         issue.PodName,
				Phase:           issue.Phase,
				ContainerStatus: issue.ContainerStatus,
				Reason:          issue.Reason,
				Message:         issue.Message,
				RestartCount:    issue.RestartCount,
			})
		}
	}

	// Convert ownership chain
	if session.OwnershipChain != nil {
		bundle.Session.OwnershipChain = &agent.OwnershipSnapshot{
			Owner: session.OwnershipChain.Owner,
		}
		for _, link := range session.OwnershipChain.K8sChain {
			bundle.Session.OwnershipChain.K8sChain = append(bundle.Session.OwnershipChain.K8sChain, agent.ChainLink{
				Kind:      link.Kind,
				Name:      link.Name,
				Namespace: link.Namespace,
				Ready:     link.Ready,
				Status:    link.Status,
			})
		}
		for _, link := range session.OwnershipChain.GitOpsChain {
			bundle.Session.OwnershipChain.GitOpsChain = append(bundle.Session.OwnershipChain.GitOpsChain, agent.ChainLink{
				Kind:      link.Kind,
				Name:      link.Name,
				Namespace: link.Namespace,
				Ready:     link.Ready,
				Status:    link.Status,
			})
		}
	}

	// Convert deployer status
	if session.DeployerStatus != nil {
		bundle.Session.DeployerStatus = &agent.DeployerSnapshot{
			Kind:      session.DeployerStatus.Kind,
			Name:      session.DeployerStatus.Name,
			Namespace: session.DeployerStatus.Namespace,
			Ready:     session.DeployerStatus.Ready,
			Suspended: session.DeployerStatus.Suspended,
			Stage:     session.DeployerStatus.Stage,
			Reason:    session.DeployerStatus.Reason,
			Message:   session.DeployerStatus.Message,
		}
	}

	// Convert source status
	if session.SourceStatus != nil {
		bundle.Session.SourceStatus = &agent.SourceSnapshot{
			Kind:      session.SourceStatus.Kind,
			Name:      session.SourceStatus.Name,
			Namespace: session.SourceStatus.Namespace,
			Ready:     session.SourceStatus.Ready,
			URL:       session.SourceStatus.URL,
			Reason:    session.SourceStatus.Reason,
			Message:   session.SourceStatus.Message,
		}
	}

	// Convert root cause
	if session.RootCause != nil {
		bundle.Session.RootCause = &agent.RootCauseSnapshot{
			Category:       session.RootCause.Category,
			Stage:          session.RootCause.Stage,
			Confidence:     session.RootCause.Confidence,
			Summary:        session.RootCause.Summary,
			ProbableCauses: session.RootCause.ProbableCauses,
			SuggestedFixes: session.RootCause.SuggestedFixes,
		}
	}

	// Convert events if present
	if session.EventTimeline != nil {
		for _, event := range session.EventTimeline.AllEvents {
			bundle.Events = append(bundle.Events, agent.TimelineEvent{
				ResourceKind: event.ResourceKind,
				ResourceName: event.ResourceName,
				Severity:     event.Severity,
				K8sEvent: agent.K8sEvent{
					Type:    event.Type,
					Reason:  event.Reason,
					Message: event.Message,
					Count:   event.Count,
				},
				Explanation: event.Explanation,
				Suggestion:  event.Suggestion,
			})
		}
	}

	// Convert logs if present
	if session.ContainerLogs != nil {
		for _, log := range session.ContainerLogs {
			result := agent.ContainerLogResult{
				PodName:       log.PodName,
				ContainerName: log.ContainerName,
				Namespace:     log.Namespace,
				Lines:         log.Lines,
				TotalLines:    log.TotalLines,
				Truncated:     log.Truncated,
				Previous:      log.Previous,
				Error:         log.Error,
			}
			for _, p := range log.Patterns {
				result.Patterns = append(result.Patterns, agent.LogPattern{
					Type:        p.Type,
					Line:        p.Line,
					Match:       p.Match,
					Explanation: p.Explanation,
					Suggestion:  p.Suggestion,
				})
			}
			bundle.Logs = append(bundle.Logs, result)
		}
	}

	return bundle
}

// populateAttribution attempts to add Crossplane attribution to the bundle
func populateAttribution(ctx context.Context, cfg *rest.Config, session *DebugSession, bundle *agent.DebugBundle) error {
	// TEST HOOK: Load target object from file for testing
	if targetPath := os.Getenv("CUB_SCOUT_TEST_TARGET_OBJECT"); targetPath != "" {
		return populateAttributionFromFixture(targetPath, bundle)
	}

	// Skip if no config provided
	if cfg == nil {
		return nil
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return err
	}

	// Fetch target object
	target, err := fetchTargetObject(ctx, dynClient, session.Target)
	if err != nil {
		return err
	}

	// Build attribution graph
	bundleID := generateBundleDirName(session.Target)
	attribution := agent.AttributionGraphForTarget(target, []*unstructured.Unstructured{target}, bundleID)
	if attribution != nil && !attribution.IsEmpty() {
		bundle.Attribution = attribution
	}

	return nil
}

// fetchTargetObject fetches the target resource as unstructured
func fetchTargetObject(ctx context.Context, dynClient dynamic.Interface, target ResourceRef) (*unstructured.Unstructured, error) {
	gvr, err := agent.KindToGVR(target.Kind)
	if err != nil {
		return nil, err
	}

	if target.Namespace != "" {
		return dynClient.Resource(gvr).Namespace(target.Namespace).Get(ctx, target.Name, metav1.GetOptions{})
	}
	return dynClient.Resource(gvr).Get(ctx, target.Name, metav1.GetOptions{})
}

// populateAttributionFromFixture loads target from fixture file for testing
func populateAttributionFromFixture(targetPath string, bundle *agent.DebugBundle) error {
	data, err := os.ReadFile(targetPath)
	if err != nil {
		return err
	}

	target := &unstructured.Unstructured{}
	if err := target.UnmarshalJSON(data); err != nil {
		// Try YAML
		return fmt.Errorf("failed to parse target object: %w", err)
	}

	// Use the target alone as the object set
	bundleID := generateBundleDirName(ResourceRef{
		Kind:      target.GetKind(),
		Name:      target.GetName(),
		Namespace: target.GetNamespace(),
	})
	attribution := agent.AttributionGraphForTarget(target, []*unstructured.Unstructured{target}, bundleID)
	if attribution != nil && !attribution.IsEmpty() {
		bundle.Attribution = attribution
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
	icon := SymOK
	if !status.Ready {
		icon = SymError
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
	icon := SymOK
	if !status.Ready {
		icon = SymError
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
