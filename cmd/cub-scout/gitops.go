// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/confighub/cub-scout/pkg/agent"
)

var (
	gitopsNamespace string
	gitopsJSON      bool
)

var gitopsCmd = &cobra.Command{
	Use:   "gitops",
	Short: "GitOps status and diagnostics",
	Long: `GitOps status and diagnostics for Flux and Argo CD.

Shows the health of your GitOps pipeline including:
- Detected backend (Flux, Argo CD)
- Transport mode (OCI, Git, Helm)
- Deployer status (Kustomization, HelmRelease, Application)
- Source status (OCIRepository, GitRepository)
- Failure stage and reason when things go wrong

Examples:
  # Show GitOps status for all namespaces
  cub-scout gitops status

  # Show GitOps status for a specific namespace
  cub-scout gitops status -n flux-system

  # Output as JSON
  cub-scout gitops status --json
`,
}

var gitopsStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show GitOps pipeline status",
	Long: `Show the status of GitOps deployers and sources in the cluster.

This command helps diagnose delegated apply issues by showing:
- Backend: Which GitOps tool is managing resources (Flux, Argo CD)
- Transport: How manifests are delivered (OCI, Git, Helm)
- ConfigHub target: If resources are managed by ConfigHub
- Failing stage: Where in the pipeline things are breaking (source, build, apply, sync)
- Reason/Message: The specific error causing the failure

Examples:
  # Quick health check
  cub-scout gitops status

  # Check specific namespace
  cub-scout gitops status -n production

  # Machine-readable output
  cub-scout gitops status --json
`,
	RunE: runGitOpsStatus,
}

func init() {
	rootCmd.AddCommand(gitopsCmd)
	gitopsCmd.AddCommand(gitopsStatusCmd)

	gitopsStatusCmd.Flags().StringVarP(&gitopsNamespace, "namespace", "n", "", "Namespace to scan (default: all namespaces)")
	gitopsStatusCmd.Flags().BoolVar(&gitopsJSON, "json", false, "Output as JSON")
}

// GitOpsSummary holds the summary of GitOps status for output
type GitOpsSummary struct {
	// Backend is the detected GitOps backend: flux, argocd, worker, none
	Backend string `json:"backend"`

	// Transport is the source transport: oci, git, helm, unknown
	Transport string `json:"transport"`

	// ConfigHubTarget is present if ConfigHub worker is detected
	ConfigHubTarget *agent.ConfigHubTarget `json:"confighubTarget,omitempty"`

	// Deployers are the detected deployer resources
	Deployers []DeployerStatus `json:"deployers,omitempty"`

	// Sources are the detected source resources
	Sources []SourceStatus `json:"sources,omitempty"`

	// HealthyCount is the number of healthy deployers
	HealthyCount int `json:"healthyCount"`

	// FailedCount is the number of failed deployers
	FailedCount int `json:"failedCount"`
}

// DeployerStatus holds the status of a single deployer
type DeployerStatus struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Ready     bool   `json:"ready"`
	Suspended bool   `json:"suspended,omitempty"`

	// Stage indicates where failure occurred: source, build, apply, sync, healthy
	Stage string `json:"stage"`

	// Reason is the condition reason
	Reason string `json:"reason,omitempty"`

	// Message is the human-readable error message
	Message string `json:"message,omitempty"`

	// SourceRef is the key of the source this deployer references
	SourceRef string `json:"sourceRef,omitempty"`

	// Argo-specific fields
	SyncStatus   string `json:"syncStatus,omitempty"`
	HealthStatus string `json:"healthStatus,omitempty"`

	// Revision information
	LastAppliedRevision   string `json:"lastAppliedRevision,omitempty"`
	LastAttemptedRevision string `json:"lastAttemptedRevision,omitempty"`
}

// IsHealthy returns true if the deployer is healthy
func (d DeployerStatus) IsHealthy() bool {
	return d.Ready && d.Stage == string(agent.StageHealthy) && !d.Suspended
}

// SourceStatus holds the status of a single source
type SourceStatus struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Ready     bool   `json:"ready"`

	// Reason is the condition reason
	Reason string `json:"reason,omitempty"`

	// Message is the human-readable error message
	Message string `json:"message,omitempty"`

	// URL is the source URL (OCI, Git, Helm)
	URL string `json:"url,omitempty"`

	// ArtifactDigest is the current artifact digest
	ArtifactDigest string `json:"artifactDigest,omitempty"`
}

// IsHealthy returns true if the source is healthy
func (s SourceStatus) IsHealthy() bool {
	return s.Ready
}

// HasFailures returns true if there are any failures in the summary
func (g GitOpsSummary) HasFailures() bool {
	for _, d := range g.Deployers {
		if !d.IsHealthy() {
			return true
		}
	}
	for _, s := range g.Sources {
		if !s.IsHealthy() {
			return true
		}
	}
	return false
}

// GetFailedDeployerCount returns the number of failed deployers
func (g GitOpsSummary) GetFailedDeployerCount() int {
	count := 0
	for _, d := range g.Deployers {
		if !d.IsHealthy() {
			count++
		}
	}
	return count
}

// GetFailedSourceCount returns the number of failed sources
func (g GitOpsSummary) GetFailedSourceCount() int {
	count := 0
	for _, s := range g.Sources {
		if !s.IsHealthy() {
			count++
		}
	}
	return count
}

func runGitOpsStatus(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// TEST HOOK: Load status data from JSON file to bypass cluster access in tests.
	if statusJSONFile := os.Getenv("CUB_SCOUT_TEST_GITOPS_JSON"); statusJSONFile != "" {
		return loadAndRenderGitOpsStatusFromJSON(statusJSONFile)
	}

	// Build k8s config
	cfg, err := buildConfig()
	if err != nil {
		return fmt.Errorf("failed to build kubernetes config: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	// Detect backend
	detector := agent.NewApplyBackendDetector(dynClient)
	backendInfo, err := detector.DetectBackend(ctx, gitopsNamespace)
	if err != nil {
		return fmt.Errorf("failed to detect GitOps backend: %w", err)
	}

	// Build summary
	summary := buildGitOpsSummary(ctx, dynClient, backendInfo)

	// Output
	if gitopsJSON {
		return outputGitOpsStatusJSON(summary)
	}
	return outputGitOpsStatusHuman(summary)
}

// buildGitOpsSummary builds a GitOpsSummary from the detected backend info
func buildGitOpsSummary(ctx context.Context, client dynamic.Interface, info *agent.ApplyBackendInfo) GitOpsSummary {
	summary := GitOpsSummary{
		Backend:         string(info.Backend),
		Transport:       string(info.Transport),
		ConfigHubTarget: info.ConfigHubTarget,
		Deployers:       []DeployerStatus{},
		Sources:         []SourceStatus{},
	}

	// Process sources first to build source status map
	sourceStatus := make(map[string]SourceStatus)
	for _, src := range info.Sources {
		status := SourceStatus{
			Kind:      src.Kind,
			Name:      src.Name,
			Namespace: src.Namespace,
			Ready:     true, // Default, will be updated if we can fetch the resource
		}

		// Try to fetch and extract failure details
		resource := fetchSourceResource(ctx, client, src)
		if resource != nil {
			details := agent.ExtractFluxSourceFailure(resource)
			status.Ready = details.Ready
			status.Reason = details.Reason
			status.Message = details.Message
			status.URL = details.SourceURL
			status.ArtifactDigest = details.ArtifactDigest
		}

		sourceStatus[src.Key()] = status
		summary.Sources = append(summary.Sources, status)
	}

	// Process deployers
	for _, dep := range info.Deployers {
		status := DeployerStatus{
			Kind:      dep.Kind,
			Name:      dep.Name,
			Namespace: dep.Namespace,
			Ready:     true, // Default, will be updated
			Stage:     string(agent.StageHealthy),
		}

		if dep.SourceRef != nil {
			status.SourceRef = dep.SourceRef.Key()
		}

		// Fetch and extract failure details based on deployer type
		resource := fetchDeployerResource(ctx, client, dep)
		if resource != nil {
			switch dep.Kind {
			case "Kustomization", "HelmRelease":
				details := agent.ExtractFluxDeployerFailure(resource)
				status.Ready = details.Ready
				status.Suspended = details.Suspended
				status.Stage = string(details.Stage)
				status.Reason = details.Reason
				status.Message = details.Message
				status.LastAppliedRevision = details.LastAppliedRevision
				status.LastAttemptedRevision = details.LastAttemptedRevision
			case "Application":
				details := agent.ExtractArgoFailure(resource)
				status.Ready = details.Ready
				status.Stage = string(details.Stage)
				status.Reason = details.Reason
				status.Message = details.Message
				status.SyncStatus = details.SyncStatus
				status.HealthStatus = details.HealthStatus
				status.LastAttemptedRevision = details.LastAttemptedRevision
			}
		}

		if status.IsHealthy() {
			summary.HealthyCount++
		} else {
			summary.FailedCount++
		}

		summary.Deployers = append(summary.Deployers, status)
	}

	return summary
}

// fetchSourceResource fetches a source resource from the cluster
func fetchSourceResource(ctx context.Context, client dynamic.Interface, ref agent.SourceRef) *unstructured.Unstructured {
	gvr := kindToGVR(ref.Kind)
	if gvr.Resource == "" {
		return nil
	}

	resource, err := client.Resource(gvr).Namespace(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return nil
	}
	return resource
}

// fetchDeployerResource fetches a deployer resource from the cluster
func fetchDeployerResource(ctx context.Context, client dynamic.Interface, ref agent.DeployerRef) *unstructured.Unstructured {
	gvr := kindToGVR(ref.Kind)
	if gvr.Resource == "" {
		return nil
	}

	resource, err := client.Resource(gvr).Namespace(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return nil
	}
	return resource
}


// outputGitOpsStatusJSON outputs the summary as JSON
func outputGitOpsStatusJSON(summary GitOpsSummary) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(summary)
}

// outputGitOpsStatusHuman outputs the summary in human-readable format
func outputGitOpsStatusHuman(summary GitOpsSummary) error {
	fmt.Printf("\n")
	fmt.Printf("%s%sGITOPS STATUS%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("════════════════════════════════════════════════════════════════════\n\n")

	// Backend and Transport
	backendColor := colorWhite
	switch summary.Backend {
	case "flux":
		backendColor = colorCyan
	case "argocd":
		backendColor = colorPurple
	case "worker":
		backendColor = colorBlue
	case "none":
		backendColor = colorDim
	}

	transportColor := colorWhite
	switch summary.Transport {
	case "oci":
		transportColor = colorBlue
	case "git":
		transportColor = colorGreen
	case "helm":
		transportColor = colorYellow
	}

	fmt.Printf("  %sBackend:%s   %s%s%s\n", colorDim, colorReset, backendColor, strings.ToUpper(summary.Backend), colorReset)
	fmt.Printf("  %sTransport:%s %s%s%s\n", colorDim, colorReset, transportColor, strings.ToUpper(summary.Transport), colorReset)

	// ConfigHub target if present
	if summary.ConfigHubTarget != nil {
		fmt.Printf("  %sConfigHub:%s Space=%s%s%s Target=%s%s%s\n",
			colorDim, colorReset,
			colorBlue, summary.ConfigHubTarget.Space, colorReset,
			colorBlue, summary.ConfigHubTarget.Target, colorReset)
	}

	fmt.Printf("\n")

	// Summary counts
	if summary.Backend == "none" {
		fmt.Printf("  %sNo GitOps backend detected in cluster.%s\n", colorYellow, colorReset)
		fmt.Printf("  %sInstall Flux or Argo CD to enable GitOps.%s\n\n", colorDim, colorReset)
		return nil
	}

	totalDeployers := len(summary.Deployers)
	if totalDeployers == 0 {
		fmt.Printf("  %sNo deployers found.%s\n\n", colorDim, colorReset)
		return nil
	}

	// Health summary
	if summary.HasFailures() {
		fmt.Printf("  %s%s⚠ %d/%d deployers failing%s\n\n",
			colorBold, colorYellow, summary.FailedCount, totalDeployers, colorReset)
	} else {
		fmt.Printf("  %s%s✓ All %d deployers healthy%s\n\n",
			colorBold, colorGreen, totalDeployers, colorReset)
	}

	// Sources section
	if len(summary.Sources) > 0 {
		fmt.Printf("%sSOURCES%s\n", colorBold, colorReset)
		fmt.Printf("────────────────────────────────────────────────────────────────────\n")
		for _, src := range summary.Sources {
			outputSourceStatus(src)
		}
		fmt.Printf("\n")
	}

	// Deployers section
	if len(summary.Deployers) > 0 {
		fmt.Printf("%sDEPLOYERS%s\n", colorBold, colorReset)
		fmt.Printf("────────────────────────────────────────────────────────────────────\n")
		for _, dep := range summary.Deployers {
			outputDeployerStatus(dep)
		}
		fmt.Printf("\n")
	}

	// Next steps for failures
	if summary.HasFailures() {
		fmt.Printf("%sNEXT STEPS:%s\n", colorBold, colorReset)

		// Find first failing deployer for trace suggestion
		for _, dep := range summary.Deployers {
			if !dep.IsHealthy() {
				fmt.Printf("→ Trace failing resource:  cub-scout trace %s/%s -n %s\n",
					strings.ToLower(dep.Kind), dep.Name, dep.Namespace)
				break
			}
		}

		// Check for source failures
		for _, src := range summary.Sources {
			if !src.IsHealthy() {
				if strings.Contains(strings.ToLower(src.Reason), "auth") {
					fmt.Printf("→ Check credentials:       kubectl get secret -n %s\n", src.Namespace)
				}
				break
			}
		}

		fmt.Printf("→ Scan for issues:         cub-scout scan --state\n")
		fmt.Printf("\n")
	}

	return nil
}

// outputSourceStatus outputs a single source status
func outputSourceStatus(src SourceStatus) {
	// Status icon
	icon := SymOK
	iconColor := colorGreen
	if !src.Ready {
		icon = SymError
		iconColor = colorRed
	}

	// Kind color
	kindColor := colorPurple
	if src.Kind == "GitRepository" {
		kindColor = colorGreen
	} else if src.Kind == "HelmRepository" {
		kindColor = colorYellow
	}

	fmt.Printf("  %s%s%s %s%s%s/%s%s%s\n",
		iconColor, icon, colorReset,
		kindColor, src.Kind, colorReset,
		colorBold, src.Name, colorReset)

	fmt.Printf("      %sNamespace:%s %s\n", colorDim, colorReset, src.Namespace)

	if src.URL != "" {
		fmt.Printf("      %sURL:%s %s%s%s\n", colorDim, colorReset, colorBlue, src.URL, colorReset)
	}

	if !src.Ready {
		if src.Reason != "" {
			fmt.Printf("      %sReason:%s %s%s%s\n", colorDim, colorReset, colorRed, src.Reason, colorReset)
		}
		if src.Message != "" {
			msg := src.Message
			if len(msg) > 60 {
				msg = msg[:57] + "..."
			}
			fmt.Printf("      %sMessage:%s %s\n", colorDim, colorReset, msg)
		}
	} else if src.ArtifactDigest != "" {
		digest := src.ArtifactDigest
		if len(digest) > 20 {
			digest = digest[:20] + "..."
		}
		fmt.Printf("      %sDigest:%s %s\n", colorDim, colorReset, digest)
	}

	fmt.Printf("\n")
}

// outputDeployerStatus outputs a single deployer status
func outputDeployerStatus(dep DeployerStatus) {
	// Status icon
	icon := SymOK
	iconColor := colorGreen
	if !dep.Ready {
		icon = SymError
		iconColor = colorRed
	}
	if dep.Suspended {
		icon = SymPaused
		iconColor = colorYellow
	}

	// Kind color
	kindColor := colorCyan
	if dep.Kind == "Application" {
		kindColor = colorPurple
	} else if dep.Kind == "HelmRelease" {
		kindColor = colorYellow
	}

	fmt.Printf("  %s%s%s %s%s%s/%s%s%s\n",
		iconColor, icon, colorReset,
		kindColor, dep.Kind, colorReset,
		colorBold, dep.Name, colorReset)

	fmt.Printf("      %sNamespace:%s %s\n", colorDim, colorReset, dep.Namespace)

	// Show stage for non-healthy deployers
	if !dep.IsHealthy() {
		stageColor := colorYellow
		switch dep.Stage {
		case string(agent.StageSource):
			stageColor = colorPurple
		case string(agent.StageBuild):
			stageColor = colorYellow
		case string(agent.StageApply):
			stageColor = colorRed
		case string(agent.StageSync):
			stageColor = colorRed
		}
		fmt.Printf("      %sStage:%s %s%s%s\n", colorDim, colorReset, stageColor, dep.Stage, colorReset)
	}

	// Argo-specific status
	if dep.Kind == "Application" {
		if dep.SyncStatus != "" {
			syncColor := colorGreen
			if dep.SyncStatus != "Synced" {
				syncColor = colorYellow
			}
			fmt.Printf("      %sSync:%s %s%s%s\n", colorDim, colorReset, syncColor, dep.SyncStatus, colorReset)
		}
		if dep.HealthStatus != "" {
			healthColor := colorGreen
			if dep.HealthStatus != "Healthy" {
				healthColor = colorRed
			}
			fmt.Printf("      %sHealth:%s %s%s%s\n", colorDim, colorReset, healthColor, dep.HealthStatus, colorReset)
		}
	}

	// Error details
	if !dep.Ready && !dep.Suspended {
		if dep.Reason != "" {
			fmt.Printf("      %sReason:%s %s%s%s\n", colorDim, colorReset, colorRed, dep.Reason, colorReset)
		}
		if dep.Message != "" {
			msg := dep.Message
			if len(msg) > 60 {
				msg = msg[:57] + "..."
			}
			fmt.Printf("      %sMessage:%s %s\n", colorDim, colorReset, msg)
		}
	}

	// Revision mismatch warning
	if dep.LastAppliedRevision != "" && dep.LastAttemptedRevision != "" &&
		dep.LastAppliedRevision != dep.LastAttemptedRevision {
		fmt.Printf("      %s⚠ Revision mismatch:%s applied=%s, attempted=%s\n",
			colorYellow, colorReset,
			truncate(dep.LastAppliedRevision, 12),
			truncate(dep.LastAttemptedRevision, 12))
	}

	// Suspended indicator
	if dep.Suspended {
		fmt.Printf("      %s(suspended)%s\n", colorDim, colorReset)
	}

	fmt.Printf("\n")
}

// loadAndRenderGitOpsStatusFromJSON loads status from JSON file for testing
func loadAndRenderGitOpsStatusFromJSON(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read gitops JSON: %w", err)
	}

	var summary GitOpsSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return fmt.Errorf("failed to parse gitops JSON: %w", err)
	}

	if gitopsJSON {
		return outputGitOpsStatusJSON(summary)
	}
	return outputGitOpsStatusHuman(summary)
}
