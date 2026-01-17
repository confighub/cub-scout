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

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	importNamespace string
	importDryRun    bool
	importYes       bool
	importJSON      bool
	importNoLog     bool
	importWizard    bool
)

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
	AppSpace string     `json:"appSpace"`
	Units    []UnitJSON `json:"units"`
}

// UnitJSON is the JSON representation of a suggested unit
type UnitJSON struct {
	Slug      string   `json:"slug"`
	App       string   `json:"app"`
	Variant   string   `json:"variant"`
	Workloads []string `json:"workloads"`
}

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import workloads into ConfigHub",
	Long: `Import your cluster workloads into ConfigHub.

This command:
  1. Discovers workloads (Deployments, StatefulSets, DaemonSets)
  2. Suggests an App Space and Units structure
  3. Creates everything in ConfigHub

That's it. One command.

Examples:
  # Import everything (discovers all namespaces)
  cub-scout import

  # Import one namespace
  cub-scout import -n argocd

  # Preview what would be created
  cub-scout import --dry-run

  # Skip confirmation
  cub-scout import -y

  # JSON output (for GUI integration)
  cub-scout import --json

  # Interactive TUI wizard (recommended)
  cub-scout import --wizard
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

	rootCmd.AddCommand(importCmd)
}

func runImport(cmd *cobra.Command, args []string) error {
	// Wizard mode - launch interactive TUI
	if importWizard {
		return RunImportWizard()
	}

	// JSON mode = dry-run (never change anything when outputting JSON)
	if importJSON {
		importDryRun = true
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
		fmt.Printf("\nImport %d units into App Space '%s'? [y/N] ", len(proposal.Units), proposal.AppSpace)
		if !confirm() {
			if logger != nil {
				logger.Log("User aborted import")
				logger.LogResult(0, 0, nil)
			}
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Step 5: Apply
	if logger != nil {
		logger.Section("APPLYING")
		logger.Log("Creating App Space: %s", proposal.AppSpace)
	}
	return applyImportWithLogger(proposal, allWorkloads, logger)
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
	fmt.Printf("  App Space: %s\n\n", proposal.AppSpace)

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

	fmt.Printf("\n  Total: %d units\n", len(proposal.Units))
}

func applyImportWithLogger(proposal *FullProposal, workloads []WorkloadInfo, logger *ImportLogger) error {
	// Index workloads
	workloadIndex := make(map[string]WorkloadInfo)
	for _, w := range workloads {
		key := fmt.Sprintf("%s/%s", w.Namespace, w.Name)
		workloadIndex[key] = w
	}

	fmt.Println()

	// Create App Space
	fmt.Printf("Creating App Space: %s... ", proposal.AppSpace)
	result, err := CreateAppSpaceWithResult(proposal.AppSpace, true, nil)
	if err != nil {
		fmt.Println("✗")
		if logger != nil {
			logger.Log("FAILED: App Space creation: %v", err)
			logger.LogResult(0, 1, err)
		}
		return fmt.Errorf("create space: %w", err)
	}
	if result.Created {
		fmt.Println("✓")
		if logger != nil {
			logger.Log("Created App Space: %s", proposal.AppSpace)
		}
	} else {
		fmt.Println("(exists)")
		if logger != nil {
			logger.Log("App Space already exists: %s", proposal.AppSpace)
		}
	}

	// Create Units
	created := 0
	failed := 0

	for _, unit := range proposal.Units {
		if len(unit.Workloads) == 0 {
			continue
		}

		fmt.Printf("Creating unit: %s... ", unit.Slug)
		if logger != nil {
			logger.Log("Creating unit: %s", unit.Slug)
		}

		// Get first workload's manifest
		w, ok := workloadIndex[unit.Workloads[0]]
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

		if err := createUnitWithManifestSimple(proposal.AppSpace, unit.Slug, labels, manifest); err != nil {
			fmt.Printf("✗ (%v)\n", err)
			if logger != nil {
				logger.Log("  FAILED: create unit: %v", err)
			}
			failed++
			continue
		}

		fmt.Println("✓")
		if logger != nil {
			logger.Log("  OK: created with labels %v", labels)
		}
		created++
	}

	fmt.Println()

	// Log final result
	var finalErr error
	if failed > 0 {
		fmt.Printf("Done: %d created, %d failed\n", created, failed)
		finalErr = fmt.Errorf("%d units failed", failed)
	} else {
		fmt.Printf("Done: %d units created\n", created)
	}

	if logger != nil {
		logger.LogResult(created, failed, finalErr)
	}

	if finalErr != nil {
		return finalErr
	}

	fmt.Println()
	fmt.Println("View your units:")
	fmt.Printf("  cub unit list --space %s\n", proposal.AppSpace)

	// Ask to start worker and set targets (skip if -y was used)
	if !importYes {
		fmt.Printf("\nStart worker and set targets? [Y/n] ")
		if confirmDefault(true) {
			fmt.Println()
			return startWorkerAndSetTargets(proposal, logger)
		}
	}

	return nil
}

func fetchManifest(kind, namespace, name string) ([]byte, error) {
	cmd := exec.Command("kubectl", "get", strings.ToLower(kind), name, "-n", namespace, "-o", "yaml")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
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

func createUnitWithManifestSimple(space, slug string, labels []string, manifest []byte) error {
	args := []string{"unit", "create", "--space", space}
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
		return fmt.Errorf("%s", strings.TrimSpace(string(output)))
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

// confirmDefault returns the default if user just presses enter
func confirmDefault(defaultYes bool) bool {
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return defaultYes
	}
	response = strings.ToLower(strings.TrimSpace(response))
	if response == "" {
		return defaultYes
	}
	return response == "y" || response == "yes"
}

// startWorkerAndSetTargets starts worker, waits for targets, sets them on units
func startWorkerAndSetTargets(proposal *FullProposal, logger *ImportLogger) error {
	if logger != nil {
		logger.Section("STARTING WORKER")
		logger.Log("Space: %s", proposal.AppSpace)
	}

	// Get current kubectl context for target matching
	ctxCmd := exec.Command("kubectl", "config", "current-context")
	ctxOut, err := ctxCmd.Output()
	if err != nil {
		return fmt.Errorf("get kubectl context: %w", err)
	}
	kubeContext := strings.TrimSpace(string(ctxOut))

	fmt.Printf("Starting worker for space '%s'...\n", proposal.AppSpace)

	// Start worker in background with output to devnull
	workerCmd := exec.Command("cub", "worker", "run", "dev", "--space", proposal.AppSpace)
	devNull, _ := os.Open(os.DevNull)
	workerCmd.Stdout = devNull
	workerCmd.Stderr = devNull
	if err := workerCmd.Start(); err != nil {
		devNull.Close()
		return fmt.Errorf("start worker: %w", err)
	}
	devNull.Close()

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
		checkCmd := exec.Command("cub", "target", "list", "--space", proposal.AppSpace, "-o", "json")
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
		fmt.Printf("  cub unit set-target <unit> <target> --space %s\n", proposal.AppSpace)
	} else {
		// Set target on all units
		fmt.Printf("Setting target '%s' on units...\n", targetSlug)
		if logger != nil {
			logger.Log("Target found: %s", targetSlug)
		}

		for _, unit := range proposal.Units {
			setCmd := exec.Command("cub", "unit", "set-target", unit.Slug, targetSlug, "--space", proposal.AppSpace)
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

	// Show next steps
	fmt.Println()
	fmt.Println("Next: Sync via your deployer to apply changes:")
	fmt.Println("  ArgoCD:  argocd app sync <app-name>")
	fmt.Println("  Flux:    flux reconcile kustomization <name>")
	fmt.Println()
	fmt.Println("Worker is running. Press Ctrl+C to stop.")
	fmt.Println()

	if logger != nil {
		logger.Log("Worker running in foreground")
	}

	// Now attach to worker output
	workerCmd.Wait()

	return nil
}

func outputEmptyJSON() error {
	result := map[string]interface{}{
		"namespaces": []string{},
		"workloads":  []WorkloadJSON{},
		"proposal":   nil,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func outputProposalJSON(proposal *FullProposal, workloads []WorkloadInfo, namespaces []string) error {
	wJSON := make([]WorkloadJSON, 0, len(workloads))
	for _, w := range workloads {
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
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
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
		return fmt.Errorf("%s", string(output))
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
		return fmt.Errorf("%s", string(output))
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
		return fmt.Errorf("%s", string(output))
	}
	return nil
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

