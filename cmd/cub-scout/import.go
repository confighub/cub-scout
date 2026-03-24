// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	importNamespace   string
	importDryRun      bool
	importYes         bool
	importJSON        bool
	importNoLog       bool
	importWizard      bool
	importConnect     bool
	importNoConnect   bool
	importFromBundle  string
	importAuditReason string
)

var listUnitSlugsForSpace = fetchUnitSlugsForSpace
var labelWorkloadForImport = labelWorkload

type cubTargetRef struct {
	Slug         string
	ProviderType string
	Toolchain    string
}

type gitOpsDelegationResult struct {
	ArgoWanted    bool
	FluxWanted    bool
	ArgoDelegated bool
	FluxDelegated bool
	ArgoReason    string
	FluxReason    string
}

type importAuditContext struct {
	ChangeSetSlug string
	Reason        string
}

func (r gitOpsDelegationResult) AnyDelegated() bool {
	return r.ArgoDelegated || r.FluxDelegated
}

// GitOpsReference identifies the GitOps resource that manages a workload
type GitOpsReference struct {
	Kind      string // "Kustomization", "HelmRelease", "Application", "HelmSecret"
	Name      string
	Namespace string
}

// WorkloadInfo represents a discovered workload
type WorkloadInfo struct {
	Kind        string
	Namespace   string
	Name        string
	UnitSlug    string // empty if not connected
	Owner       string // Flux, Argo, Helm, Native, etc.
	Ready       bool
	Replicas    int32
	Labels      map[string]string
	Annotations map[string]string

	// GitOps migration fields
	GitOpsRef         *GitOpsReference
	KustomizationPath string // Flux Kustomization spec.path
	ApplicationPath   string // Argo CD Application spec.source.path
	ExtractedConfig   string // YAML config extracted from GitOps source
	ConfigError       error  // Error if extraction failed

	// Source info (populated from ArgoCD/Flux Application)
	SourceRepo string // Git repository URL
	SourcePath string // Path within repository
}

// ImportResult is the JSON output structure
type ImportResult struct {
	Namespace  string          `json:"namespace"`
	Model      string          `json:"model"`
	Workloads  []WorkloadJSON  `json:"workloads"`
	Suggestion *SuggestionJSON `json:"suggestion"`
}

// WorkloadJSON is the JSON representation of a discovered workload
type WorkloadJSON struct {
	Kind              string            `json:"kind"`
	Namespace         string            `json:"namespace"`
	Name              string            `json:"name"`
	Owner             string            `json:"owner"`
	Connected         bool              `json:"connected"`
	UnitSlug          string            `json:"unitSlug,omitempty"`
	Ready             bool              `json:"ready"`
	Replicas          int32             `json:"replicas"`
	KustomizationPath string            `json:"kustomizationPath,omitempty"`
	ApplicationPath   string            `json:"applicationPath,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
}

// SuggestionJSON is the JSON representation of the import suggestion
type SuggestionJSON struct {
	App string     `json:"app"`
	Units    []UnitJSON `json:"units"`
}

// UnitJSON is the JSON representation of a suggested unit
type UnitJSON struct {
	Slug      string   `json:"slug"`
	App       string   `json:"app"`
	Variant   string   `json:"variant"`
	Workloads []string `json:"workloads"`
}

type importEvidenceJSON struct {
	Source     string `json:"source"`
	BundlePath string `json:"bundlePath,omitempty"`
}

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import workloads into ConfigHub",
	Long: `Import your cluster workloads into ConfigHub.

This command:
  1. Discovers workloads (Deployments, StatefulSets, DaemonSets)
  2. Suggests an App and Deployments structure
  3. Imports into ConfigHub

When ArgoCD/Flux workloads are found and matching GitOps targets are available
in the App Space, cub-scout delegates those workloads to:
  cub gitops discover + cub gitops import
and imports Helm/Native leftovers via the snapshot path.

Terminology: the API currently uses Space/Unit; see docs/reference/glossary.md
for the App/Deployment mapping.

That's it. One command.

Examples:
  # Import everything (discovers all namespaces)
  cub-scout import

  # Import one namespace
  cub-scout import -n argocd

  # Preview what would be created
  cub-scout import --dry-run

  # Skip confirmation (legacy: no auto-connect)
  cub-scout import -y

  # Skip confirmation + connect worker/targets now
  cub-scout import -y --connect

  # JSON output (for GUI integration)
  cub-scout import --json

  # Preview import proposal from an existing debug bundle
  cub-scout import --from-bundle ./debug-bundle --dry-run --json

  # Interactive TUI wizard (recommended)
  cub-scout import --wizard

  # Non-interactive import + immediate cluster connection
  cub-scout import --yes --connect
`,
	RunE: runImport,
}

func init() {
	importCmd.Flags().StringVarP(&importNamespace, "namespace", "n", "", "Namespace to import (discovers all if not specified)")
	importCmd.Flags().BoolVar(&importDryRun, "dry-run", false, "Preview without making changes")
	importCmd.Flags().BoolVarP(&importYes, "yes", "y", false, "Skip confirmation")
	importCmd.Flags().BoolVar(&importJSON, "json", false, "Output as JSON (for GUI/scripting)")
	importCmd.Flags().BoolVar(&importNoLog, "no-log", false, "Disable logging to file")
	importCmd.Flags().BoolVarP(&importWizard, "wizard", "w", false, "Launch interactive TUI wizard")
	importCmd.Flags().BoolVar(&importConnect, "connect", false, "After import, start worker and set targets")
	importCmd.Flags().BoolVar(&importNoConnect, "no-connect", false, "Do not start worker/targets after import")
	importCmd.Flags().StringVar(&importFromBundle, "from-bundle", "", "Import from a debug bundle directory instead of live cluster discovery")
	importCmd.Flags().StringVar(&importAuditReason, "audit-reason", "", "Record break-glass decision reason in connected audit history (max 512 chars)")

	rootCmd.AddCommand(importCmd)
}

func runImport(cmd *cobra.Command, args []string) error {
	if importConnect && importNoConnect {
		return fmt.Errorf("--connect and --no-connect cannot be used together")
	}

	// Wizard mode - launch interactive TUI
	if importWizard {
		if importFromBundle != "" {
			return fmt.Errorf("--wizard cannot be used with --from-bundle")
		}
		return RunImportWizard()
	}

	// JSON mode = dry-run (never change anything when outputting JSON)
	if importJSON {
		importDryRun = true
	}

	if _, err := normalizeImportAuditReason(importAuditReason); err != nil {
		return err
	}

	// Bundle import path bypasses live cluster discovery.
	if importFromBundle != "" {
		return runImportFromBundle(importFromBundle)
	}

	// Initialize logger (unless disabled or JSON mode)
	var logger *ImportLogger
	if !importNoLog && !importJSON {
		var err error
		logger, err = NewImportLogger("import")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not create log file: %v\n", err)
		}
	}
	defer func() {
		if logger != nil {
			logPath := logger.Close()
			if logPath != "" && !importJSON {
				fmt.Printf("\nLog: %s\n", logPath)
			}
		}
	}()

	if logger != nil {
		logger.Log("Starting import")
		if importNamespace != "" {
			logger.Log("Target namespace: %s", importNamespace)
		} else {
			logger.Log("Target: all namespaces (auto-discover)")
		}
		if importDryRun {
			logger.Log("Mode: dry-run")
		}
	}

	// Step 1: Discover workloads
	var allWorkloads []WorkloadInfo
	var namespaces []string

	if importNamespace != "" {
		// Single namespace
		namespaces = []string{importNamespace}
	} else {
		// Discover all namespaces with workloads
		var err error
		namespaces, err = discoverNamespacesWithWorkloads()
		if err != nil {
			return fmt.Errorf("discover namespaces: %w", err)
		}
	}

	if len(namespaces) == 0 {
		if logger != nil {
			logger.Log("No namespaces with workloads found")
		}
		if importJSON {
			return outputEmptyJSON()
		}
		fmt.Println("No workloads found.")
		return nil
	}

	if logger != nil {
		logger.Log("Found %d namespace(s): %s", len(namespaces), strings.Join(namespaces, ", "))
	}

	// Collect workloads from all namespaces
	for _, ns := range namespaces {
		workloads, err := discoverWorkloads(ns)
		if err != nil {
			if !importJSON {
				fmt.Fprintf(os.Stderr, "Warning: failed to scan namespace %s: %v\n", ns, err)
			}
			continue
		}
		allWorkloads = append(allWorkloads, workloads...)
	}

	if len(allWorkloads) == 0 {
		if logger != nil {
			logger.Log("No workloads found in any namespace")
		}
		if importJSON {
			return outputEmptyJSON()
		}
		fmt.Println("No workloads found.")
		return nil
	}

	// Log discovered workloads
	if logger != nil {
		logger.LogWorkloads(allWorkloads)
	}

	// Step 2: Generate suggestion
	proposal := SuggestFullProposal(nil, allWorkloads, "")

	// Log the proposal
	if logger != nil {
		logger.LogProposal(proposal)
	}

	// JSON output mode
	if importJSON {
		return outputProposalJSON(proposal, allWorkloads, namespaces)
	}

	// Step 3: Show what we found and what we'll create
	printDiscovery(namespaces, allWorkloads, proposal)

	if importDryRun {
		if logger != nil {
			logger.Log("Dry-run mode - no changes made")
			logger.LogResult(0, 0, nil)
		}
		fmt.Println("\n(dry-run mode - no changes made)")
		fmt.Println("Run without --dry-run to import.")
		return nil
	}

	// Step 4: Confirm
	if !importYes {
		fmt.Printf("\nImport this into ConfigHub? [y/N] ")
		if !confirm() {
			if logger != nil {
				logger.Log("User aborted import")
				logger.LogResult(0, 0, nil)
			}
			fmt.Println("Aborted.")
			return nil
		}
	}

	shouldConnect, showConnectHint := resolveImportConnectionMode(importYes, importConnect, importNoConnect)
	if showConnectHint {
		fmt.Println("\nTip: add --connect to start worker/targets now and avoid detached units.")
	}

	// Step 5: Apply
	if logger != nil {
		logger.Section("APPLYING")
		logger.Log("Creating App: %s", proposal.App)
	}

	// Try delegating Argo/Flux workloads to cub gitops import first.
	delegation, err := attemptGitOpsDelegation(proposal.App, allWorkloads, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: GitOps delegation failed, falling back to scout import: %v\n", err)
		if logger != nil {
			logger.Log("GitOps delegation failed, fallback to scout import: %v", err)
		}
	}

	if delegation.ArgoWanted || delegation.FluxWanted {
		printDelegationSummary(delegation)
	}

	// If a controller was delegated successfully, skip snapshot import for those workloads.
	scoutWorkloads := filterScoutWorkloadsAfterDelegation(allWorkloads, delegation)
	if len(scoutWorkloads) == 0 && delegation.AnyDelegated() {
		fmt.Println()
		fmt.Println("No Helm/Native leftovers to snapshot-import.")
		fmt.Printf("GitOps-managed workloads imported via cub gitops import into App '%s'.\n", proposal.App)
		return printSpaceSummary(proposal.App)
	}

	proposalToApply := proposal
	if len(scoutWorkloads) != len(allWorkloads) {
		proposalToApply = SuggestFullProposal(nil, scoutWorkloads, proposal.App)
	}

	return applyImportWithLogger(proposalToApply, scoutWorkloads, logger, shouldConnect, importAuditReason)
}

func runImportFromBundle(bundlePath string) error {
	if importNamespace != "" {
		return fmt.Errorf("--namespace cannot be used with --from-bundle")
	}

	proposal, workloads, namespaces, err := buildImportFromBundlePreview(bundlePath)
	if err != nil {
		return err
	}

	if len(workloads) == 0 {
		if importJSON {
			return outputEmptyJSON()
		}
		fmt.Println("No workloads found in bundle.")
		return nil
	}

	if importJSON {
		return outputProposalJSON(proposal, workloads, namespaces)
	}

	printDiscovery(namespaces, workloads, proposal)

	if importDryRun {
		fmt.Println("\n(dry-run mode - no changes made)")
		fmt.Println("Run without --dry-run to import.")
		return nil
	}

	if !importYes {
		fmt.Printf("\nImport this into ConfigHub? [y/N] ")
		if !confirm() {
			fmt.Println("Aborted.")
			return nil
		}
	}

	shouldConnect, showConnectHint := resolveImportConnectionMode(importYes, importConnect, importNoConnect)
	if showConnectHint {
		fmt.Println("\nTip: add --connect to start worker/targets now and avoid detached units.")
	}

	return applyImportWithLogger(proposal, workloads, nil, shouldConnect, importAuditReason)
}

// resolveImportConnectionMode determines whether import should auto-connect workers/targets
// and whether a non-interactive hint should be shown.
func resolveImportConnectionMode(importYes, importConnect, importNoConnect bool) (bool, bool) {
	shouldConnect := importConnect
	if !importYes && !importNoConnect && !importConnect {
		shouldConnect = true
	}
	if importNoConnect {
		shouldConnect = false
	}

	showConnectHint := importYes && !importConnect && !importNoConnect
	return shouldConnect, showConnectHint
}

func normalizeImportAuditReason(raw string) (string, error) {
	reason := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if reason == "" {
		return "", nil
	}
	if len([]rune(reason)) > 512 {
		return "", fmt.Errorf("--audit-reason exceeds 512 characters")
	}
	return reason, nil
}

func createImportAuditContext(proposal *FullProposal, workloads []WorkloadInfo, reason string, logger *ImportLogger) (*importAuditContext, error) {
	if proposal == nil || strings.TrimSpace(proposal.App) == "" {
		return nil, fmt.Errorf("cannot create break-glass audit context without app space")
	}

	changeSetSlug := fmt.Sprintf("break-glass-%s", time.Now().UTC().Format("20060102-150405"))
	description := buildBreakGlassChangesetDescription(reason, workloads)

	args := []string{
		"changeset", "create",
		"--space", proposal.App,
		changeSetSlug,
		"--description", description,
		"--label", "break-glass=true",
		"--label", "source=cub-scout-import",
		"--label", "workflow=break-glass",
		"--allow-exists",
		"--quiet",
	}

	namespaces := collectBreakGlassNamespaces(workloads)
	if len(namespaces) == 1 {
		args = append(args, "--label", fmt.Sprintf("namespace=%s", namespaces[0]))
	}

	cmd := exec.Command("cub", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("create break-glass audit changeset: %s", msg)
	}

	fmt.Printf("Audit: recorded break-glass decision in changeset %s\n", changeSetSlug)
	if logger != nil {
		logger.Log("Audit changeset created: %s (%s)", changeSetSlug, description)
	}

	return &importAuditContext{
		ChangeSetSlug: changeSetSlug,
		Reason:        reason,
	}, nil
}

func collectBreakGlassNamespaces(workloads []WorkloadInfo) []string {
	set := make(map[string]struct{})
	for _, workload := range workloads {
		ns := strings.TrimSpace(workload.Namespace)
		if ns == "" {
			continue
		}
		set[ns] = struct{}{}
	}
	namespaces := make([]string, 0, len(set))
	for ns := range set {
		namespaces = append(namespaces, ns)
	}
	sort.Strings(namespaces)
	return namespaces
}

func buildBreakGlassChangesetDescription(reason string, workloads []WorkloadInfo) string {
	namespaces := collectBreakGlassNamespaces(workloads)
	scope := "all namespaces"
	if len(namespaces) > 0 {
		scope = strings.Join(namespaces, ",")
	}

	text := fmt.Sprintf(
		"break-glass decision: accept unmanaged resources via cub-scout import; namespaces=%s; workloads=%d; reason=%s",
		scope,
		len(workloads),
		reason,
	)
	if len([]rune(text)) > 512 {
		runes := []rune(text)
		text = string(runes[:509]) + "..."
	}
	return text
}

func buildBreakGlassChangeDescription(reason string, unit UnitProposal) string {
	workloadCount := len(unit.Workloads)
	text := fmt.Sprintf(
		"break-glass accept: unit=%s workloads=%d reason=%s",
		strings.TrimSpace(unit.Slug),
		workloadCount,
		reason,
	)
	if len([]rune(text)) > 512 {
		runes := []rune(text)
		text = string(runes[:509]) + "..."
	}
	return text
}

func buildImportFromBundlePreview(bundlePath string) (*FullProposal, []WorkloadInfo, []string, error) {
	reader := agent.NewBundleReader()
	bundle, err := reader.Read(bundlePath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read bundle %q: %w", bundlePath, err)
	}

	workload, err := workloadFromBundle(bundle)
	if err != nil {
		return nil, nil, nil, err
	}

	workloads := []WorkloadInfo{workload}
	namespaces := collectNamespacesFromWorkloads(workloads, bundle.Metadata.Target.Namespace)
	proposal := SuggestFullProposal(nil, workloads, "")
	return proposal, workloads, namespaces, nil
}

func workloadFromBundle(bundle *agent.DebugBundle) (WorkloadInfo, error) {
	kind := strings.TrimSpace(bundle.Metadata.Target.Kind)
	name := strings.TrimSpace(bundle.Metadata.Target.Name)
	namespace := strings.TrimSpace(bundle.Metadata.Target.Namespace)

	workload := WorkloadInfo{
		Kind:      kind,
		Name:      name,
		Namespace: namespace,
		Owner:     "Native",
		Replicas:  0,
		Ready:     false,
	}

	if bundle.Session != nil {
		if health := bundle.Session.WorkloadHealth; health != nil {
			if strings.TrimSpace(health.Kind) != "" {
				workload.Kind = strings.TrimSpace(health.Kind)
			}
			if strings.TrimSpace(health.Name) != "" {
				workload.Name = strings.TrimSpace(health.Name)
			}
			if strings.TrimSpace(health.Namespace) != "" {
				workload.Namespace = strings.TrimSpace(health.Namespace)
			}
			workload.Replicas = health.Replicas
			workload.Ready = health.ReadyReplicas == health.Replicas
		}

		owner := inferOwnerFromBundleSession(bundle.Session)
		if owner != "" {
			workload.Owner = owner
		}

		workload.KustomizationPath, workload.ApplicationPath = inferPathsFromBundleSession(bundle.Session)
		if bundle.Session.SourceStatus != nil {
			workload.SourceRepo = strings.TrimSpace(bundle.Session.SourceStatus.URL)
		}
	}

	if workload.Kind == "" || workload.Name == "" {
		return WorkloadInfo{}, fmt.Errorf("bundle target is missing required kind/name")
	}

	return workload, nil
}

func inferOwnerFromBundleSession(session *agent.DebugSessionData) string {
	if session == nil {
		return "Native"
	}

	if session.OwnershipChain != nil {
		if owner := normalizeOwnerFromBundle(session.OwnershipChain.Owner); owner != "" {
			return owner
		}
		for _, link := range session.OwnershipChain.GitOpsChain {
			if owner := ownerFromDeployerKind(link.Kind); owner != "" {
				return owner
			}
		}
	}

	if session.DeployerStatus != nil {
		if owner := ownerFromDeployerKind(session.DeployerStatus.Kind); owner != "" {
			return owner
		}
	}

	return "Native"
}

func inferPathsFromBundleSession(session *agent.DebugSessionData) (string, string) {
	if session == nil || session.OwnershipChain == nil {
		return "", ""
	}

	var kustomizationPath, applicationPath string
	for _, link := range session.OwnershipChain.GitOpsChain {
		path := strings.TrimSpace(link.Path)
		if path == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(link.Kind)) {
		case "kustomization":
			if kustomizationPath == "" {
				kustomizationPath = path
			}
		case "application":
			if applicationPath == "" {
				applicationPath = path
			}
		}
	}
	return kustomizationPath, applicationPath
}

func normalizeOwnerFromBundle(owner string) string {
	switch strings.ToLower(strings.TrimSpace(owner)) {
	case "":
		return ""
	case "argocd", "argo", "argo cd":
		return "ArgoCD"
	case "flux":
		return "Flux"
	case "helm":
		return "Helm"
	case "native":
		return "Native"
	case "confighub", "config hub":
		return "ConfigHub"
	case "terraform":
		return "Terraform"
	case "crossplane":
		return "Crossplane"
	case "kro":
		return "kro"
	default:
		return strings.TrimSpace(owner)
	}
}

func ownerFromDeployerKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "application", "applicationset":
		return "ArgoCD"
	case "kustomization", "helmrelease":
		return "Flux"
	default:
		return ""
	}
}

func collectNamespacesFromWorkloads(workloads []WorkloadInfo, fallback string) []string {
	nsSet := make(map[string]struct{})
	for _, w := range workloads {
		ns := strings.TrimSpace(w.Namespace)
		if ns != "" {
			nsSet[ns] = struct{}{}
		}
	}

	if len(nsSet) == 0 {
		if fallback = strings.TrimSpace(fallback); fallback != "" {
			nsSet[fallback] = struct{}{}
		}
	}

	namespaces := make([]string, 0, len(nsSet))
	for ns := range nsSet {
		namespaces = append(namespaces, ns)
	}
	sort.Strings(namespaces)
	return namespaces
}

// discoverNamespacesWithWorkloads finds all namespaces that have Deployments, StatefulSets, or DaemonSets
func discoverNamespacesWithWorkloads() ([]string, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	nsSet := make(map[string]bool)

	// Check Deployments
	deps, err := clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, d := range deps.Items {
			// Skip system namespaces
			if !isSystemNamespace(d.Namespace) {
				nsSet[d.Namespace] = true
			}
		}
	}

	// Check StatefulSets
	sts, err := clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, s := range sts.Items {
			if !isSystemNamespace(s.Namespace) {
				nsSet[s.Namespace] = true
			}
		}
	}

	// Check DaemonSets
	ds, err := clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, d := range ds.Items {
			if !isSystemNamespace(d.Namespace) {
				nsSet[d.Namespace] = true
			}
		}
	}

	// Convert to sorted slice
	namespaces := make([]string, 0, len(nsSet))
	for ns := range nsSet {
		namespaces = append(namespaces, ns)
	}
	sort.Strings(namespaces)

	return namespaces, nil
}

// isSystemNamespace returns true for namespaces we should skip by default
func isSystemNamespace(ns string) bool {
	systemNamespaces := map[string]bool{
		"kube-system":        true,
		"kube-public":        true,
		"kube-node-lease":    true,
		"local-path-storage": true,
		"flux-system":        true, // Flux controllers
		"argocd":             true, // ArgoCD controllers
	}
	return systemNamespaces[ns]
}

func discoverWorkloads(namespace string) ([]WorkloadInfo, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("build kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		dynClient = nil // Non-fatal
	}

	ctx := context.Background()
	var workloads []WorkloadInfo

	// Deployments
	deployments, err := clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}

	for _, d := range deployments.Items {
		owner, gitopsRef := detectOwnerAndRef(d.Labels, d.Annotations)
		w := WorkloadInfo{
			Kind:        "Deployment",
			Namespace:   d.Namespace,
			Name:        d.Name,
			Replicas:    *d.Spec.Replicas,
			Ready:       d.Status.ReadyReplicas == *d.Spec.Replicas,
			Owner:       owner,
			Labels:      d.Labels,
			Annotations: d.Annotations,
			GitOpsRef:   gitopsRef,
		}

		if gitopsRef != nil {
			switch gitopsRef.Kind {
			case "Kustomization":
				w.KustomizationPath = getKustomizationPath(ctx, dynClient, gitopsRef.Name, gitopsRef.Namespace)
			case "Application":
				w.ApplicationPath = getApplicationPath(ctx, dynClient, gitopsRef.Name, gitopsRef.Namespace)
			}
		}

		if slug, ok := d.Labels["confighub.com/UnitSlug"]; ok {
			w.UnitSlug = slug
		} else if slug, ok := d.Annotations["confighub.com/UnitSlug"]; ok {
			w.UnitSlug = slug
		}

		workloads = append(workloads, w)
	}

	// StatefulSets
	statefulsets, err := clientset.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list statefulsets: %w", err)
	}

	for _, s := range statefulsets.Items {
		owner, gitopsRef := detectOwnerAndRef(s.Labels, s.Annotations)
		w := WorkloadInfo{
			Kind:        "StatefulSet",
			Namespace:   s.Namespace,
			Name:        s.Name,
			Replicas:    *s.Spec.Replicas,
			Ready:       s.Status.ReadyReplicas == *s.Spec.Replicas,
			Owner:       owner,
			Labels:      s.Labels,
			Annotations: s.Annotations,
			GitOpsRef:   gitopsRef,
		}

		if gitopsRef != nil {
			switch gitopsRef.Kind {
			case "Kustomization":
				w.KustomizationPath = getKustomizationPath(ctx, dynClient, gitopsRef.Name, gitopsRef.Namespace)
			case "Application":
				w.ApplicationPath = getApplicationPath(ctx, dynClient, gitopsRef.Name, gitopsRef.Namespace)
			}
		}

		if slug, ok := s.Labels["confighub.com/UnitSlug"]; ok {
			w.UnitSlug = slug
		} else if slug, ok := s.Annotations["confighub.com/UnitSlug"]; ok {
			w.UnitSlug = slug
		}

		workloads = append(workloads, w)
	}

	// DaemonSets
	daemonsets, err := clientset.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list daemonsets: %w", err)
	}

	for _, d := range daemonsets.Items {
		owner, gitopsRef := detectOwnerAndRef(d.Labels, d.Annotations)
		w := WorkloadInfo{
			Kind:        "DaemonSet",
			Namespace:   d.Namespace,
			Name:        d.Name,
			Replicas:    d.Status.DesiredNumberScheduled,
			Ready:       d.Status.NumberReady == d.Status.DesiredNumberScheduled,
			Owner:       owner,
			Labels:      d.Labels,
			Annotations: d.Annotations,
			GitOpsRef:   gitopsRef,
		}

		if gitopsRef != nil {
			switch gitopsRef.Kind {
			case "Kustomization":
				w.KustomizationPath = getKustomizationPath(ctx, dynClient, gitopsRef.Name, gitopsRef.Namespace)
			case "Application":
				w.ApplicationPath = getApplicationPath(ctx, dynClient, gitopsRef.Name, gitopsRef.Namespace)
			}
		}

		if slug, ok := d.Labels["confighub.com/UnitSlug"]; ok {
			w.UnitSlug = slug
		} else if slug, ok := d.Annotations["confighub.com/UnitSlug"]; ok {
			w.UnitSlug = slug
		}

		workloads = append(workloads, w)
	}

	return workloads, nil
}

// detectOwnerAndRef detects the owner and returns a GitOps reference
func detectOwnerAndRef(labels, annotations map[string]string) (string, *GitOpsReference) {
	// Flux Kustomization
	if name, ok := labels["kustomize.toolkit.fluxcd.io/name"]; ok {
		ns := labels["kustomize.toolkit.fluxcd.io/namespace"]
		if ns == "" {
			ns = "flux-system"
		}
		return "Flux", &GitOpsReference{Kind: "Kustomization", Name: name, Namespace: ns}
	}

	// Flux HelmRelease
	if name, ok := labels["helm.toolkit.fluxcd.io/name"]; ok {
		ns := labels["helm.toolkit.fluxcd.io/namespace"]
		if ns == "" {
			ns = "flux-system"
		}
		return "Flux", &GitOpsReference{Kind: "HelmRelease", Name: name, Namespace: ns}
	}

	// Argo CD
	if instance, ok := labels["argocd.argoproj.io/instance"]; ok {
		return "ArgoCD", &GitOpsReference{Kind: "Application", Name: instance, Namespace: "argocd"}
	}
	if trackingID, ok := annotations["argocd.argoproj.io/tracking-id"]; ok {
		// ArgoCD tracking-id formats:
		// 1. <app-name>:<group>/<kind>:<resource-ns>/<resource-name>
		//    Example: example.guestbook:apps/Deployment:guestbook/guestbook-ui
		// 2. <app-ns>.<app-name>:<group>/<kind>:<resource-ns>/<resource-name>
		//    Example: argocd.my-app:apps/Deployment:default/nginx
		//
		// The first segment before ":" is the app identifier (possibly with namespace prefix)
		// The second segment contains "/" (group/kind), which distinguishes it from app name
		if parts := strings.SplitN(trackingID, ":", 4); len(parts) >= 2 {
			appIdentifier := parts[0]
			// If parts[1] contains "/", it's the group/kind, meaning parts[0] is the full app identifier
			if strings.Contains(parts[1], "/") {
				// parts[0] is the app identifier (possibly namespace.name or just name)
				// Try to extract namespace and name from "namespace.name" format
				appNs := "argocd" // default namespace
				appName := appIdentifier
				if dotIdx := strings.Index(appIdentifier, "."); dotIdx > 0 {
					// Could be namespace.name format, but could also just be app name with dots
					// ArgoCD app names can contain dots, so we take the whole thing as name
					// and default to argocd namespace unless we can find the Application CR
					appName = appIdentifier
				}
				if appName != "" {
					return "ArgoCD", &GitOpsReference{Kind: "Application", Name: appName, Namespace: appNs}
				}
			} else {
				// Old format: parts[1] is the app name
				appNs := parts[0]
				appName := parts[1]
				if appNs == "" {
					appNs = "argocd"
				}
				if appName != "" {
					return "ArgoCD", &GitOpsReference{Kind: "Application", Name: appName, Namespace: appNs}
				}
			}
		}
		return "ArgoCD", nil
	}

	// Helm
	if labels["app.kubernetes.io/managed-by"] == "Helm" {
		releaseName := annotations["meta.helm.sh/release-name"]
		releaseNs := annotations["meta.helm.sh/release-namespace"]
		if releaseName != "" {
			return "Helm", &GitOpsReference{Kind: "HelmSecret", Name: releaseName, Namespace: releaseNs}
		}
		return "Helm", nil
	}

	// ConfigHub
	if _, ok := labels["confighub.com/UnitSlug"]; ok {
		return "ConfigHub", nil
	}
	if _, ok := annotations["confighub.com/UnitSlug"]; ok {
		return "ConfigHub", nil
	}

	return "Native", nil
}

// Flux Kustomization GVR
var kustomizationGVR = schema.GroupVersionResource{
	Group:    "kustomize.toolkit.fluxcd.io",
	Version:  "v1",
	Resource: "kustomizations",
}

// Argo CD Application GVR
var applicationGVR = schema.GroupVersionResource{
	Group:    "argoproj.io",
	Version:  "v1alpha1",
	Resource: "applications",
}

func getKustomizationPath(ctx context.Context, dynClient dynamic.Interface, name, namespace string) string {
	if dynClient == nil {
		return ""
	}

	kust, err := dynClient.Resource(kustomizationGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return ""
	}

	spec, ok := kust.Object["spec"].(map[string]interface{})
	if !ok {
		return ""
	}

	path, _ := spec["path"].(string)
	return path
}

func getApplicationPath(ctx context.Context, dynClient dynamic.Interface, name, namespace string) string {
	if dynClient == nil {
		return ""
	}

	app, err := dynClient.Resource(applicationGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return ""
	}

	spec, ok := app.Object["spec"].(map[string]interface{})
	if !ok {
		return ""
	}

	source, ok := spec["source"].(map[string]interface{})
	if !ok {
		return ""
	}

	path, _ := source["path"].(string)
	return path
}

func printDiscovery(namespaces []string, workloads []WorkloadInfo, proposal *FullProposal) {
	// Count by namespace
	byNs := make(map[string]int)
	for _, w := range workloads {
		byNs[w.Namespace]++
	}

	fmt.Println("┌─────────────────────────────────────────────────────────────┐")
	fmt.Println("│ DISCOVERED                                                  │")
	fmt.Println("└─────────────────────────────────────────────────────────────┘")

	for _, ns := range namespaces {
		if count, ok := byNs[ns]; ok {
			fmt.Printf("  %s (%d workloads)\n", ns, count)
		}
	}

	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────┐")
	fmt.Println("│ WILL CREATE                                                 │")
	fmt.Println("└─────────────────────────────────────────────────────────────┘")
	fmt.Printf("  App: %s\n\n", proposal.App)

	for _, unit := range proposal.Units {
		labels := []string{}
		for k, v := range unit.Labels {
			labels = append(labels, fmt.Sprintf("%s=%s", k, v))
		}
		fmt.Printf("  • %s\n", unit.Slug)
		if len(labels) > 0 {
			fmt.Printf("    labels: %s\n", strings.Join(labels, ", "))
		}
		fmt.Printf("    workloads: %d\n", len(unit.Workloads))
	}

	fmt.Printf("\n  Total: %d deployments\n", len(proposal.Units))
}

func printDelegationSummary(r gitOpsDelegationResult) {
	fmt.Println()
	fmt.Println("GitOps delegation:")
	if r.ArgoWanted {
		if r.ArgoDelegated {
			fmt.Println("  ✓ ArgoCD workloads -> cub gitops import")
		} else {
			fmt.Printf("  ○ ArgoCD workloads -> scout snapshot (%s)\n", r.ArgoReason)
		}
	}
	if r.FluxWanted {
		if r.FluxDelegated {
			fmt.Println("  ✓ Flux workloads -> cub gitops import")
		} else {
			fmt.Printf("  ○ Flux workloads -> scout snapshot (%s)\n", r.FluxReason)
		}
	}
}

func filterScoutWorkloadsAfterDelegation(workloads []WorkloadInfo, r gitOpsDelegationResult) []WorkloadInfo {
	filtered := make([]WorkloadInfo, 0, len(workloads))
	for _, w := range workloads {
		if w.Owner == "ArgoCD" && r.ArgoDelegated {
			continue
		}
		if w.Owner == "Flux" && r.FluxDelegated {
			continue
		}
		filtered = append(filtered, w)
	}
	return filtered
}

func attemptGitOpsDelegation(space string, workloads []WorkloadInfo, logger *ImportLogger) (gitOpsDelegationResult, error) {
	result := gitOpsDelegationResult{}
	needArgo := false
	needFlux := false
	for _, w := range workloads {
		if w.Owner == "ArgoCD" {
			needArgo = true
		}
		if w.Owner == "Flux" {
			needFlux = true
		}
	}
	result.ArgoWanted = needArgo
	result.FluxWanted = needFlux

	if !needArgo && !needFlux {
		return result, nil
	}

	if logger != nil {
		logger.Section("GITOPS DELEGATION")
		logger.Log("Trying cub gitops import for GitOps-managed workloads in space %s", space)
	}

	// Ensure space exists so target lookups and gitops commands can run.
	if _, err := CreateAppWithResult(space, true, nil); err != nil {
		msg := fmt.Sprintf("cannot ensure app space: %v", err)
		if needArgo {
			result.ArgoReason = msg
		}
		if needFlux {
			result.FluxReason = msg
		}
		return result, err
	}

	targets, err := loadCubTargets(space)
	if err != nil {
		msg := fmt.Sprintf("cannot list targets: %v", err)
		if needArgo {
			result.ArgoReason = msg
		}
		if needFlux {
			result.FluxReason = msg
		}
		return result, nil
	}

	k8sTarget, argoRendererTarget, fluxRendererTarget := selectGitOpsTargets(targets)
	if k8sTarget == "" {
		msg := "no Kubernetes discovery target in this App Space"
		if needArgo {
			result.ArgoReason = msg
		}
		if needFlux {
			result.FluxReason = msg
		}
		return result, nil
	}

	if needArgo {
		if argoRendererTarget == "" {
			result.ArgoReason = "no ArgoCD renderer target in this App Space"
		} else {
			argoNamespaces := gitOpsNamespacesForOwner(workloads, "ArgoCD", "argocd")
			if err := runGitOpsImportForNamespaces(space, k8sTarget, argoRendererTarget, argoNamespaces, "ArgoCD", logger); err != nil {
				result.ArgoReason = err.Error()
			} else {
				result.ArgoDelegated = true
			}
		}
	}

	if needFlux {
		if fluxRendererTarget == "" {
			result.FluxReason = "no Flux renderer target in this App Space"
		} else {
			fluxNamespaces := gitOpsNamespacesForOwner(workloads, "Flux", "flux-system")
			if err := runGitOpsImportForNamespaces(space, k8sTarget, fluxRendererTarget, fluxNamespaces, "Flux", logger); err != nil {
				result.FluxReason = err.Error()
			} else {
				result.FluxDelegated = true
			}
		}
	}

	if needArgo && !result.ArgoDelegated && result.ArgoReason == "" {
		result.ArgoReason = "delegation unavailable"
	}
	if needFlux && !result.FluxDelegated && result.FluxReason == "" {
		result.FluxReason = "delegation unavailable"
	}

	return result, nil
}

func loadCubTargets(space string) ([]cubTargetRef, error) {
	out, err := exec.Command("cub", "target", "list", "--space", space, "--json").Output()
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Target struct {
			Slug         string `json:"Slug"`
			ProviderType string `json:"ProviderType"`
			Toolchain    string `json:"ToolchainType"`
		} `json:"Target"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, err
	}
	targets := make([]cubTargetRef, 0, len(raw))
	for _, t := range raw {
		if t.Target.Slug == "" {
			continue
		}
		targets = append(targets, cubTargetRef{
			Slug:         t.Target.Slug,
			ProviderType: t.Target.ProviderType,
			Toolchain:    t.Target.Toolchain,
		})
	}
	return targets, nil
}

func selectGitOpsTargets(targets []cubTargetRef) (k8sTarget, argoRenderer, fluxRenderer string) {
	for _, t := range targets {
		slug := strings.ToLower(t.Slug)
		provider := strings.ToLower(t.ProviderType)
		toolchain := strings.ToLower(t.Toolchain)
		all := slug + " " + provider + " " + toolchain

		if k8sTarget == "" && (strings.Contains(all, "kubernetes") || strings.Contains(all, "k8s")) {
			k8sTarget = t.Slug
		}
		if argoRenderer == "" &&
			(strings.Contains(all, "argocdrenderer") || (strings.Contains(all, "argocd") && strings.Contains(all, "renderer"))) {
			argoRenderer = t.Slug
		}
		if fluxRenderer == "" &&
			(strings.Contains(all, "fluxrenderer") || (strings.Contains(all, "flux") && strings.Contains(all, "renderer"))) {
			fluxRenderer = t.Slug
		}
	}
	return k8sTarget, argoRenderer, fluxRenderer
}

func gitOpsNamespacesForOwner(workloads []WorkloadInfo, owner, fallback string) []string {
	seen := make(map[string]bool)
	var namespaces []string
	for _, w := range workloads {
		if w.Owner != owner || w.GitOpsRef == nil {
			continue
		}
		ns := strings.TrimSpace(w.GitOpsRef.Namespace)
		if ns == "" {
			continue
		}
		if seen[ns] {
			continue
		}
		seen[ns] = true
		namespaces = append(namespaces, ns)
	}
	if len(namespaces) == 0 {
		namespaces = []string{fallback}
	}
	sort.Strings(namespaces)
	return namespaces
}

func runGitOpsImportForNamespaces(space, k8sTarget, rendererTarget string, namespaces []string, label string, logger *ImportLogger) error {
	for _, ns := range namespaces {
		where := fmt.Sprintf("metadata.namespace = '%s'", ns)
		fmt.Printf("\nDelegating %s namespace '%s' via cub gitops import...\n", label, ns)
		if logger != nil {
			logger.Log("Delegating %s ns=%s using k8s=%s renderer=%s", label, ns, k8sTarget, rendererTarget)
		}

		discoverCmd := exec.Command("cub", "gitops", "discover",
			"--space", space,
			k8sTarget,
			"--where-resource", where,
		)
		discoverOutput, err := discoverCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("cub gitops discover failed for %s/%s: %s", label, ns, strings.TrimSpace(string(discoverOutput)))
		}

		importCmd := exec.Command("cub", "gitops", "import",
			"--space", space,
			k8sTarget, rendererTarget,
			"--where-resource", where,
			"--wait",
		)
		importOutput, err := importCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("cub gitops import failed for %s/%s: %s", label, ns, strings.TrimSpace(string(importOutput)))
		}
	}
	return nil
}

func applyImportWithLogger(proposal *FullProposal, workloads []WorkloadInfo, logger *ImportLogger, shouldConnect bool, auditReason string) error {
	normalizedReason, err := normalizeImportAuditReason(auditReason)
	if err != nil {
		return err
	}

	var auditCtx *importAuditContext

	// Index workloads
	workloadIndex := make(map[string]WorkloadInfo)
	for _, w := range workloads {
		key := canonicalWorkloadRef(fmt.Sprintf("%s/%s", w.Namespace, w.Name))
		if key == "" {
			continue
		}
		workloadIndex[key] = w
	}

	fmt.Println()

	// Create App
	fmt.Printf("Creating App: %s... ", proposal.App)
	result, err := CreateAppWithResult(proposal.App, true, nil)
	if err != nil {
		fmt.Println(SymError)
		if logger != nil {
			logger.Log("FAILED: App creation: %v", err)
			logger.LogResult(0, 1, err)
		}
		return fmt.Errorf("create space: %w", err)
	}
	if result.Created {
		fmt.Println(SymOK)
		if logger != nil {
			logger.Log("Created App: %s", proposal.App)
		}
	} else {
		fmt.Println("(exists)")
		if logger != nil {
			logger.Log("App already exists: %s", proposal.App)
		}
	}

	if normalizedReason != "" {
		auditCtx, err = createImportAuditContext(proposal, workloads, normalizedReason, logger)
		if err != nil {
			return err
		}
	}

	// Create Deployments
	created := 0
	failed := 0

	for _, unit := range proposal.Units {
		if len(unit.Workloads) == 0 {
			continue
		}

		fmt.Printf("Creating deployment: %s... ", unit.Slug)
		if logger != nil {
			logger.Log("Creating deployment: %s", unit.Slug)
		}

		// Get first workload's manifest
		workloadRef := canonicalWorkloadRef(unit.Workloads[0])
		w, ok := workloadIndex[workloadRef]
		if !ok {
			fmt.Println("✗ (workload not found)")
			if logger != nil {
				logger.Log("  FAILED: workload not found: %s", unit.Workloads[0])
			}
			failed++
			continue
		}

		manifest, err := fetchManifest(w.Kind, w.Namespace, w.Name)
		if err != nil {
			fmt.Printf("✗ (%v)\n", err)
			if logger != nil {
				logger.Log("  FAILED: fetch manifest: %v", err)
			}
			failed++
			continue
		}

		labels := []string{}
		for k, v := range unit.Labels {
			labels = append(labels, fmt.Sprintf("%s=%s", k, v))
		}

		if err := createUnitWithManifestSimple(proposal.App, unit, labels, manifest, auditCtx); err != nil {
			fmt.Printf("✗ (%v)\n", err)
			if logger != nil {
				logger.Log("  FAILED: create deployment: %v", err)
			}
			failed++
			continue
		}

		fmt.Println(SymOK)
		if logger != nil {
			logger.Log("  OK: created with labels %v", labels)
		}
		created++

		labelFailures := linkUnitWorkloadsToCluster(unit, workloadIndex, labelWorkloadForImport, logger)
		if labelFailures > 0 {
			fmt.Printf("  ! warning: %d workload link(s) failed for unit %s\n", labelFailures, unit.Slug)
		}
	}

	fmt.Println()

	// Log final result
	var finalErr error
	if failed > 0 {
		fmt.Printf("Done: %d created, %d failed\n", created, failed)
		finalErr = fmt.Errorf("%d deployments failed", failed)
	} else {
		fmt.Printf("Done: %d deployments created\n", created)
	}

	if logger != nil {
		logger.LogResult(created, failed, finalErr)
	}

	if finalErr != nil {
		return finalErr
	}

	if shouldConnect {
		fmt.Println()
		fmt.Println("Connecting this App to your cluster (worker + targets)...")
		if err := startWorkerAndSetTargets(proposal, logger); err != nil {
			return err
		}
		return nil
	}

	return printSpaceSummary(proposal.App)
}

func fetchManifest(kind, namespace, name string) ([]byte, error) {
	cmd := exec.Command("kubectl", "get", strings.ToLower(kind), name, "-n", namespace, "-o", "yaml")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// #112: Propagate actual error message instead of generic "exit status 1"
		errMsg := strings.TrimSpace(string(output))
		if errMsg == "" {
			errMsg = err.Error()
		}
		return nil, fmt.Errorf("kubectl get %s %s -n %s: %s", kind, name, namespace, errMsg)
	}

	// Strip server-side fields that interfere with kubectl apply's three-way merge
	// These fields are set by Kubernetes and shouldn't be in source YAML
	return stripServerSideFields(output)
}

// stripServerSideFields removes Kubernetes server-side fields from YAML
// This ensures clean YAML that works properly with kubectl apply
func stripServerSideFields(yamlData []byte) ([]byte, error) {
	lines := strings.Split(string(yamlData), "\n")
	var result []string
	skipUntilDedent := -1
	inStatus := false

	for _, line := range lines {
		// Calculate indentation
		trimmed := strings.TrimLeft(line, " ")
		indent := len(line) - len(trimmed)

		// Track if we're in a section to skip
		if skipUntilDedent >= 0 {
			if indent <= skipUntilDedent && trimmed != "" {
				skipUntilDedent = -1
			} else {
				continue
			}
		}

		// Skip status section entirely
		if strings.HasPrefix(trimmed, "status:") && indent == 0 {
			inStatus = true
			continue
		}
		if inStatus {
			if indent == 0 && trimmed != "" && !strings.HasPrefix(trimmed, " ") {
				inStatus = false
			} else {
				continue
			}
		}

		// Skip managedFields section
		if strings.HasPrefix(trimmed, "managedFields:") {
			skipUntilDedent = indent
			continue
		}

		// Skip specific server-side metadata fields
		if strings.HasPrefix(trimmed, "creationTimestamp:") ||
			strings.HasPrefix(trimmed, "resourceVersion:") ||
			strings.HasPrefix(trimmed, "uid:") ||
			strings.HasPrefix(trimmed, "generation:") ||
			strings.HasPrefix(trimmed, "selfLink:") {
			continue
		}

		// Skip kubectl.kubernetes.io/last-applied-configuration annotation
		// It can span multiple lines due to the JSON content
		if strings.Contains(trimmed, "kubectl.kubernetes.io/last-applied-configuration") {
			// Skip this line and any continuation lines (higher indent)
			skipUntilDedent = indent
			continue
		}

		result = append(result, line)
	}

	return []byte(strings.Join(result, "\n")), nil
}

func createUnitWithManifestSimple(space string, unit UnitProposal, labels []string, manifest []byte, auditCtx *importAuditContext) error {
	slug := strings.TrimSpace(unit.Slug)
	args := []string{"unit", "create", "--space", space}
	if auditCtx != nil && strings.TrimSpace(auditCtx.ChangeSetSlug) != "" {
		args = append(args, "--changeset", auditCtx.ChangeSetSlug)
		if changeDesc := buildBreakGlassChangeDescription(auditCtx.Reason, unit); changeDesc != "" {
			args = append(args, "--change-desc", changeDesc)
		}
	}
	for _, l := range labels {
		args = append(args, "--label", l)
	}
	args = append(args, slug, "-")

	cmd := exec.Command("cub", args...)
	cmd.Stdin = bytes.NewReader(manifest)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if already exists
		if strings.Contains(string(output), "already exists") {
			return nil
		}
		// #112: Provide actionable error with context
		errMsg := strings.TrimSpace(string(output))
		if errMsg == "" {
			errMsg = err.Error()
		}
		// Check for common auth issues and provide remediation hints
		if strings.Contains(errMsg, "401") ||
			strings.Contains(errMsg, "unauthorized") ||
			strings.Contains(errMsg, "token") ||
			strings.Contains(errMsg, "expired") {
			return fmt.Errorf("%s\n\nHint: Your ConfigHub token may be expired. Run: cub auth login", errMsg)
		}
		return fmt.Errorf("cub unit create %s: %s", slug, errMsg)
	}
	return nil
}

func confirm() bool {
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}

// startWorkerAndSetTargets starts worker, waits for targets, sets them on units
func startWorkerAndSetTargets(proposal *FullProposal, logger *ImportLogger) error {
	if logger != nil {
		logger.Section("STARTING WORKER")
		logger.Log("Space: %s", proposal.App)
	}

	// Get current kubectl context for target matching
	ctxCmd := exec.Command("kubectl", "config", "current-context")
	ctxOut, err := ctxCmd.Output()
	if err != nil {
		return fmt.Errorf("get kubectl context: %w", err)
	}
	kubeContext := strings.TrimSpace(string(ctxOut))

	fmt.Printf("Starting worker for App '%s'...\n", proposal.App)

	// Start worker in background with output to devnull.
	// Command exits while worker keeps running.
	workerCmd := exec.Command("cub", "worker", "run", "dev", "--space", proposal.App)
	devNull, _ := os.Open(os.DevNull)
	workerCmd.Stdout = devNull
	workerCmd.Stderr = devNull
	if err := workerCmd.Start(); err != nil {
		devNull.Close()
		return fmt.Errorf("start worker: %w", err)
	}
	devNull.Close()
	go func() { _ = workerCmd.Wait() }()

	if logger != nil {
		logger.Log("Worker started (PID %d)", workerCmd.Process.Pid)
	}

	// Wait for targets to be created (poll for up to 30 seconds)
	fmt.Print("Waiting for targets to register")
	var targetSlug string
	expectedTarget := fmt.Sprintf("dev-kubernetes-yaml-%s", kubeContext)

	for i := 0; i < 30; i++ {
		time.Sleep(1 * time.Second)
		fmt.Print(".")

		// Check if target exists
		checkCmd := exec.Command("cub", "target", "list", "--space", proposal.App, "-o", "json")
		out, err := checkCmd.Output()
		if err != nil {
			continue
		}

		// Look for our expected target
		if strings.Contains(string(out), expectedTarget) {
			targetSlug = expectedTarget
			break
		}
	}
	fmt.Println()

	if targetSlug == "" {
		fmt.Println("⚠ Targets not ready yet. Set target manually:")
		fmt.Printf("  cub unit set-target <unit> <target> --space %s\n", proposal.App)
	} else {
		// Set target on all units
		fmt.Printf("Setting target '%s' on deployments...\n", targetSlug)
		if logger != nil {
			logger.Log("Target found: %s", targetSlug)
		}

		for _, unit := range proposal.Units {
			setCmd := exec.Command("cub", "unit", "set-target", unit.Slug, targetSlug, "--space", proposal.App)
			if err := setCmd.Run(); err != nil {
				fmt.Printf("  ⚠ %s: failed to set target\n", unit.Slug)
				if logger != nil {
					logger.Log("  FAILED: set-target %s: %v", unit.Slug, err)
				}
			} else {
				fmt.Printf("  ✓ %s → %s\n", unit.Slug, targetSlug)
				if logger != nil {
					logger.Log("  OK: %s → %s", unit.Slug, targetSlug)
				}
			}
		}
	}

	if err := printSpaceSummary(proposal.App); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("Next: Sync via your deployer to apply changes:")
	fmt.Println("  ArgoCD:  argocd app sync <app-name>")
	fmt.Println("  Flux:    flux reconcile kustomization <name>")
	fmt.Println()
	fmt.Printf("Worker running in background (PID %d)\n", workerCmd.Process.Pid)

	if logger != nil {
		logger.Log("Worker running in background")
	}

	return nil
}

func printSpaceSummary(space string) error {
	fmt.Println()
	fmt.Println("Imported units now visible in ConfigHub:")
	listCmd := exec.Command("cub", "unit", "list", "--space", space)
	listCmd.Stdout = os.Stdout
	listCmd.Stderr = os.Stderr
	if err := listCmd.Run(); err != nil {
		fmt.Printf("  (could not list units: %v)\n", err)
	}

	if url := getSpaceURL(space); url != "" {
		fmt.Println()
		fmt.Println("View in browser:")
		fmt.Printf("  %s\n", url)
	}
	return nil
}

func getSpaceURL(spaceSlug string) string {
	ctxCmd := exec.Command("cub", "context", "get", "--json")
	ctxOutput, err := ctxCmd.Output()
	if err != nil {
		return ""
	}

	var ctx CubContext
	if err := json.Unmarshal(ctxOutput, &ctx); err != nil {
		return ""
	}
	serverURL := strings.TrimSpace(ctx.Coordinate.ServerURL)
	if serverURL == "" {
		return ""
	}

	spaceCmd := exec.Command("cub", "space", "list", "--json")
	spaceOutput, err := spaceCmd.Output()
	if err != nil {
		return fmt.Sprintf("%s/spaces/%s", serverURL, spaceSlug)
	}

	var spaces []CubSpaceData
	if err := json.Unmarshal(spaceOutput, &spaces); err != nil {
		return fmt.Sprintf("%s/spaces/%s", serverURL, spaceSlug)
	}

	for _, s := range spaces {
		if s.Space.Slug == spaceSlug {
			return fmt.Sprintf("%s/spaces/%s", serverURL, s.Space.SpaceID)
		}
	}

	return fmt.Sprintf("%s/spaces/%s", serverURL, spaceSlug)
}

func outputEmptyJSON() error {
	result := map[string]interface{}{
		"namespaces": []string{},
		"workloads":  []WorkloadJSON{},
		"proposal":   nil,
		"evidence":   buildImportEvidenceJSON(),
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func linkUnitWorkloadsToCluster(unit UnitProposal, workloadIndex map[string]WorkloadInfo, labelFn func(kind, namespace, name, unitSlug string) error, logger *ImportLogger) int {
	if len(unit.Workloads) == 0 {
		return 0
	}

	labelFailures := 0
	for _, ref := range unit.Workloads {
		workloadRef := canonicalWorkloadRef(ref)
		workload, ok := workloadIndex[workloadRef]
		if !ok {
			labelFailures++
			if logger != nil {
				logger.Log("  WARN: label skip, workload ref not found: %s", ref)
			}
			continue
		}
		if err := labelFn(workload.Kind, workload.Namespace, workload.Name, unit.Slug); err != nil {
			labelFailures++
			if logger != nil {
				logger.Log("  WARN: label workload failed %s/%s (%s): %v", workload.Namespace, workload.Name, unit.Slug, err)
			}
		}
	}
	return labelFailures
}

func outputProposalJSON(proposal *FullProposal, workloads []WorkloadInfo, namespaces []string) error {
	resolvedWorkloads := resolveConnectedWorkloadsFromProposal(proposal, workloads)

	wJSON := make([]WorkloadJSON, 0, len(resolvedWorkloads))
	for _, w := range resolvedWorkloads {
		wJSON = append(wJSON, WorkloadJSON{
			Kind:              w.Kind,
			Namespace:         w.Namespace,
			Name:              w.Name,
			Owner:             w.Owner,
			Connected:         w.UnitSlug != "",
			UnitSlug:          w.UnitSlug,
			Ready:             w.Ready,
			Replicas:          w.Replicas,
			KustomizationPath: w.KustomizationPath,
			ApplicationPath:   w.ApplicationPath,
			Labels:            w.Labels,
		})
	}

	result := map[string]interface{}{
		"namespaces": namespaces,
		"workloads":  wJSON,
		"proposal":   proposal,
		"evidence":   buildImportEvidenceJSON(),
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func resolveConnectedWorkloadsFromProposal(proposal *FullProposal, workloads []WorkloadInfo) []WorkloadInfo {
	if proposal == nil || strings.TrimSpace(proposal.App) == "" || len(proposal.Units) == 0 || len(workloads) == 0 {
		return workloads
	}

	needsFallback := false
	for _, w := range workloads {
		if strings.TrimSpace(w.UnitSlug) == "" {
			needsFallback = true
			break
		}
	}
	if !needsFallback {
		return workloads
	}

	existingUnitSlugs, err := listUnitSlugsForSpace(proposal.App)
	if err != nil || len(existingUnitSlugs) == 0 {
		return workloads
	}

	workloadToUnit := make(map[string]string)
	for _, unit := range proposal.Units {
		if !existingUnitSlugs[unit.Slug] {
			continue
		}
		for _, ref := range unit.Workloads {
			key := canonicalWorkloadRef(ref)
			if key == "" {
				continue
			}
			if _, exists := workloadToUnit[key]; !exists {
				workloadToUnit[key] = unit.Slug
			}
		}
	}
	if len(workloadToUnit) == 0 {
		return workloads
	}

	updated := make([]WorkloadInfo, len(workloads))
	copy(updated, workloads)
	for i := range updated {
		if strings.TrimSpace(updated[i].UnitSlug) != "" {
			continue
		}
		key := canonicalWorkloadRef(fmt.Sprintf("%s/%s", updated[i].Namespace, updated[i].Name))
		if key == "" {
			continue
		}
		if slug := workloadToUnit[key]; slug != "" {
			updated[i].UnitSlug = slug
		}
	}

	return updated
}

func fetchUnitSlugsForSpace(space string) (map[string]bool, error) {
	space = strings.TrimSpace(space)
	if space == "" {
		return map[string]bool{}, nil
	}

	cmd := exec.Command("cub", "unit", "list", "--space", space, "--json", "--quiet")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parseUnitSlugsFromListJSON(output)
}

func parseUnitSlugsFromListJSON(raw []byte) (map[string]bool, error) {
	var decoded interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}

	entries := extractUnitListEntries(decoded)
	slugs := make(map[string]bool)
	for _, entry := range entries {
		if slug := extractUnitSlug(entry); slug != "" {
			slugs[slug] = true
		}
	}
	return slugs, nil
}

func extractUnitListEntries(decoded interface{}) []interface{} {
	switch typed := decoded.(type) {
	case []interface{}:
		return typed
	case map[string]interface{}:
		for _, key := range []string{"units", "Units", "items", "Items", "data", "Data"} {
			if items, ok := typed[key].([]interface{}); ok {
				return items
			}
		}
		return []interface{}{typed}
	default:
		return nil
	}
}

func extractUnitSlug(entry interface{}) string {
	m, ok := entry.(map[string]interface{})
	if !ok {
		return ""
	}

	if unit, ok := m["Unit"].(map[string]interface{}); ok {
		if slug := readUnitSlug(unit); slug != "" {
			return slug
		}
	}
	if unit, ok := m["unit"].(map[string]interface{}); ok {
		if slug := readUnitSlug(unit); slug != "" {
			return slug
		}
	}

	return readUnitSlug(m)
}

func readUnitSlug(m map[string]interface{}) string {
	for _, key := range []string{"Slug", "slug"} {
		if value, ok := m[key].(string); ok {
			value = strings.TrimSpace(value)
			if value != "" {
				return value
			}
		}
	}
	return ""
}

func canonicalWorkloadRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}

	parts := strings.Split(ref, "/")
	switch len(parts) {
	case 2:
		if parts[0] == "" || parts[1] == "" {
			return ""
		}
		return parts[0] + "/" + parts[1]
	case 3:
		if parts[1] == "" || parts[2] == "" {
			return ""
		}
		return parts[1] + "/" + parts[2]
	default:
		return ""
	}
}

func buildImportEvidenceJSON() importEvidenceJSON {
	if strings.TrimSpace(importFromBundle) != "" {
		return importEvidenceJSON{
			Source:     "bundle",
			BundlePath: importFromBundle,
		}
	}
	return importEvidenceJSON{
		Source: "cluster",
	}
}

// createUnitWithConfig creates a unit with initial configuration from stdin
func createUnitWithConfig(space, unitSlug, config string) error {
	if config == "" {
		return createUnit(space, unitSlug)
	}

	cmd := exec.Command("cub", "unit", "create", unitSlug, "-", "--space", space)
	cmd.Stdin = strings.NewReader(config)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "already exists") {
			return nil
		}
		// #112: Provide actionable error with context
		return formatCubError("unit create", unitSlug, string(output), err)
	}
	return nil
}

// createUnitWithConfigAndLabels creates a unit with initial configuration and labels
func createUnitWithConfigAndLabels(space, unitSlug, config, labels string) error {
	if config == "" {
		return createUnit(space, unitSlug)
	}

	args := []string{"unit", "create", unitSlug, "-", "--space", space}
	if labels != "" {
		args = append(args, "--labels", labels)
	}

	cmd := exec.Command("cub", args...)
	cmd.Stdin = strings.NewReader(config)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "already exists") {
			return nil
		}
		// #112: Provide actionable error with context
		return formatCubError("unit create", unitSlug, string(output), err)
	}
	return nil
}

// createUnit creates a unit in ConfigHub
func createUnit(space, unitSlug string) error {
	cmd := exec.Command("cub", "unit", "create", unitSlug, "--space", space)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "already exists") {
			return nil
		}
		// #112: Provide actionable error with context
		return formatCubError("unit create", unitSlug, string(output), err)
	}
	return nil
}

// formatCubError formats a cub CLI error with actionable context
// #112: Ensure error messages are helpful, not just "exit status 1"
func formatCubError(command, resource, output string, originalErr error) error {
	errMsg := strings.TrimSpace(output)
	if errMsg == "" {
		errMsg = originalErr.Error()
	}

	// Check for common auth issues and provide remediation hints
	lowerMsg := strings.ToLower(errMsg)
	if strings.Contains(lowerMsg, "401") ||
		strings.Contains(lowerMsg, "unauthorized") ||
		strings.Contains(lowerMsg, "token") ||
		strings.Contains(lowerMsg, "expired") ||
		strings.Contains(lowerMsg, "not authenticated") {
		return fmt.Errorf("%s\n\n→ Hint: Your ConfigHub token may be expired.\n  Run: cub auth login", errMsg)
	}

	// Check for network errors
	if strings.Contains(lowerMsg, "connection refused") ||
		strings.Contains(lowerMsg, "no such host") ||
		strings.Contains(lowerMsg, "network") {
		return fmt.Errorf("%s\n\n→ Hint: Cannot reach ConfigHub API.\n  Check your network connection and ConfigHub status.", errMsg)
	}

	// Check for permission errors
	if strings.Contains(lowerMsg, "403") ||
		strings.Contains(lowerMsg, "forbidden") ||
		strings.Contains(lowerMsg, "permission") {
		return fmt.Errorf("%s\n\n→ Hint: You may not have permission for this operation.\n  Check your space access in ConfigHub.", errMsg)
	}

	return fmt.Errorf("cub %s %s: %s", command, resource, errMsg)
}

// labelWorkload applies a ConfigHub label to a workload
func labelWorkload(kind, namespace, name, unitSlug string) error {
	resource := strings.ToLower(kind)
	label := fmt.Sprintf("confighub.com/UnitSlug=%s", unitSlug)

	cmd := exec.Command("kubectl", "label", resource, name,
		"-n", namespace, label, "--overwrite")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", string(output))
	}
	return nil
}

// checkCubAuth verifies the cub CLI is authenticated
func checkCubAuth() error {
	cmd := exec.Command("cub", "auth", "status", "--quiet")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("not authenticated with ConfigHub. Run 'cub auth login' first.\n%s", string(output))
	}
	return nil
}

// getCurrentSpace returns the currently selected ConfigHub space
func getCurrentSpace() (string, error) {
	cmd := exec.Command("cub", "context", "get", "--json", "--quiet")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get current context: %s", string(output))
	}

	// Parse JSON output to get space
	var ctx struct {
		Space string `json:"space"`
	}
	if err := json.Unmarshal(output, &ctx); err != nil {
		return "", err
	}
	if ctx.Space == "" {
		return "", fmt.Errorf("no space selected")
	}
	return ctx.Space, nil
}

// ensureSpace creates the space if it doesn't exist
func ensureSpace(space string) error {
	// Try to select the space first
	cmd := exec.Command("cub", "context", "set", "--space", space)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	// If space doesn't exist, create it
	if strings.Contains(string(output), "not found") || strings.Contains(string(output), "does not exist") {
		createCmd := exec.Command("cub", "space", "create", space)
		createOutput, createErr := createCmd.CombinedOutput()
		if createErr != nil {
			return fmt.Errorf("failed to create space: %s", string(createOutput))
		}
		// Select the newly created space
		if err := exec.Command("cub", "context", "set", "--space", space).Run(); err != nil {
			return fmt.Errorf("space created but failed to set context: %w", err)
		}
	}
	return nil
}
