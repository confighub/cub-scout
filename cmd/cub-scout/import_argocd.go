// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"
)

var (
	argoImportApp         string
	argoImportNamespace   string
	argoImportSpace       string
	argoImportDryRun      bool
	argoImportYes         bool
	argoImportShowYAML    bool
	argoImportRaw         bool // Keep raw YAML with all runtime fields (default: clean)
	argoImportList        bool
	argoImportDisableSync bool // Disable auto-sync after import
	argoImportDeleteApp   bool // Delete Application after import
	argoImportTestUpdate  bool // Test ConfigHub pipeline with annotation update
	argoImportTestRollout bool // Test ConfigHub pipeline with rollout restart
)

// ArgoApplication represents an ArgoCD Application CR
type ArgoApplication struct {
	Name      string
	Namespace string
	Project   string
	Source    ArgoSource
	Destination ArgoDestination
	SyncStatus  string
	HealthStatus string
}

// ArgoSource represents the source configuration in an ArgoCD Application
type ArgoSource struct {
	RepoURL        string
	Path           string
	TargetRevision string
	Chart          string // For Helm charts
	Helm           *ArgoHelmSource
}

// ArgoHelmSource represents Helm-specific source configuration
type ArgoHelmSource struct {
	ReleaseName string
	Values      string
	ValueFiles  []string
}

// ArgoDestination represents the deployment target
type ArgoDestination struct {
	Server    string
	Namespace string
	Name      string // Cluster name
}

// ManagedResource represents a resource managed by an ArgoCD Application
type ManagedResource struct {
	APIVersion string
	Kind       string
	Namespace  string
	Name       string
	Status     string
	Health     string
	YAML       string // The actual resource YAML
}

// ArgoStatusResource represents a resource from .status.resources
type ArgoStatusResource struct {
	Group     string
	Version   string
	Kind      string
	Namespace string
	Name      string
	Status    string // Sync status
	Health    string
}

var importArgoCmd = &cobra.Command{
	Use:   "import-argocd [application-name]",
	Short: "Import an ArgoCD Application into ConfigHub",
	Long: `Import an ArgoCD Application's managed resources into ConfigHub as a Unit.

This command:
  1. Reads the ArgoCD Application to find its destination namespace
  2. Discovers all resources managed by the Application (Deployments, Services, etc.)
  3. Creates a ConfigHub Unit containing the workload manifests
  4. Extracts labels from the Git path (e.g., overlays/prod → variant=prod)

The ArgoCD Application CR itself is NOT imported as a Unit - it's Argo's
orchestration mechanism. ConfigHub uses its own orchestration (Hub → Space → Unit).

For App-of-Apps patterns, import the child Applications individually to get
the actual workload resources.

Examples:
  # List available ArgoCD Applications
  cub-agent import-argocd --list

  # Import a specific ArgoCD Application
  cub-agent import-argocd guestbook

  # Preview what would be imported (dry-run)
  cub-agent import-argocd guestbook --dry-run

  # Show YAML content that would be imported
  cub-agent import-argocd guestbook --show-yaml

  # Import and disable ArgoCD auto-sync (keep App for reference)
  cub-agent import-argocd guestbook --disable-sync

  # Import and delete the ArgoCD Application (hand off to ConfigHub)
  cub-agent import-argocd guestbook --delete-app
`,
	Args: cobra.MaximumNArgs(1),
	RunE: runImportArgoCD,
}

func init() {
	importArgoCmd.Flags().StringVar(&argoImportNamespace, "argocd-namespace", "argocd", "Namespace where ArgoCD is installed")
	importArgoCmd.Flags().StringVar(&argoImportSpace, "space", "", "ConfigHub space to import into (auto-inferred if not specified)")
	importArgoCmd.Flags().BoolVar(&argoImportDryRun, "dry-run", false, "Preview what would be imported without making changes")
	importArgoCmd.Flags().BoolVar(&argoImportShowYAML, "show-yaml", false, "Show YAML content that would be imported (implies --dry-run)")
	importArgoCmd.Flags().BoolVar(&argoImportRaw, "raw", false, "Keep raw YAML with all runtime fields (default: clean)")
	importArgoCmd.Flags().BoolVar(&argoImportList, "list", false, "List available ArgoCD Applications")
	importArgoCmd.Flags().BoolVarP(&argoImportYes, "yes", "y", false, "Skip confirmation prompts")
	importArgoCmd.Flags().BoolVar(&argoImportDisableSync, "disable-sync", false, "Disable auto-sync on the ArgoCD Application after import")
	importArgoCmd.Flags().BoolVar(&argoImportDeleteApp, "delete-app", false, "Delete the ArgoCD Application after import (keeps resources)")
	importArgoCmd.Flags().BoolVar(&argoImportTestUpdate, "test-update", false, "Test ConfigHub pipeline by adding an annotation to verify it can update resources")
	importArgoCmd.Flags().BoolVar(&argoImportTestRollout, "test-rollout", false, "Test ConfigHub pipeline by triggering a rollout restart")

	rootCmd.AddCommand(importArgoCmd)
}

func runImportArgoCD(cmd *cobra.Command, args []string) error {
	// Build Kubernetes clients first (needed for both list and import)
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	ctx := context.Background()

	// Handle --list flag
	if argoImportList {
		return listArgoApplicationsCmd(ctx, dynamicClient, argoImportNamespace)
	}

	// Application name is required for import
	if len(args) == 0 {
		return fmt.Errorf("application name required. Use --list to see available applications")
	}
	appName := args[0]

	// Validate mutually exclusive flags
	if argoImportDisableSync && argoImportDeleteApp {
		return fmt.Errorf("--disable-sync and --delete-app are mutually exclusive")
	}

	// Check cub CLI is available and authenticated (only for actual import)
	if !argoImportDryRun && !argoImportShowYAML {
		if err := checkCubAuth(); err != nil {
			return err
		}
	}

	fmt.Printf("ConfigHub ArgoCD Import\n")
	fmt.Printf("=======================\n\n")

	// Step 1: Read the ArgoCD Application CR
	fmt.Printf("Step 1: Reading ArgoCD Application '%s' from namespace '%s'...\n", appName, argoImportNamespace)

	app, _, err := getArgoApplication(ctx, dynamicClient, argoImportNamespace, appName)
	if err != nil {
		return fmt.Errorf("failed to get ArgoCD Application: %w", err)
	}

	fmt.Printf("  ✓ Found Application: %s\n", app.Name)
	fmt.Printf("    Project: %s\n", app.Project)
	fmt.Printf("    Source: %s (path: %s, revision: %s)\n", app.Source.RepoURL, app.Source.Path, app.Source.TargetRevision)
	fmt.Printf("    Destination: %s (namespace: %s)\n", app.Destination.Server, app.Destination.Namespace)
	fmt.Printf("    Sync: %s, Health: %s\n", app.SyncStatus, app.HealthStatus)
	fmt.Println()

	// Step 2: Get destination namespace from Application spec
	destNamespace := app.Destination.Namespace
	if destNamespace == "" {
		return fmt.Errorf("Application has no destination namespace specified")
	}
	fmt.Printf("Step 2: Destination namespace: %s\n\n", destNamespace)

	// Step 3: Find all resources managed by this Application
	// First check .status.resources to see what ArgoCD is tracking
	statusResources, err := getStatusResources(ctx, dynamicClient, argoImportNamespace, appName)
	if err != nil {
		fmt.Printf("  ⚠ Could not get status resources: %v\n", err)
	}

	// Check if this is an App of Apps pattern
	appOfApps := isAppOfApps(statusResources)
	var childApps []string

	if appOfApps {
		fmt.Printf("Step 3: Detected App of Apps pattern\n")
		childApps = getChildApplicationNames(statusResources)
		fmt.Printf("  ✓ This Application manages %d child Applications:\n", len(childApps))
		for _, name := range childApps {
			fmt.Printf("    → %s\n", name)
		}
		fmt.Println()
		fmt.Printf("  Note: App of Apps manages Application CRs, not workload resources.\n")
		fmt.Printf("  To import the actual workloads, import the child Applications individually.\n")
	} else {
		fmt.Printf("Step 3: Finding resources managed by '%s' in namespace '%s'...\n", appName, destNamespace)
	}

	// For non-App-of-Apps, get the actual managed workload resources
	var managedResources []ManagedResource
	if !appOfApps {
		managedResources, err = getManagedResources(ctx, clientset, dynamicClient, appName, destNamespace)
		if err != nil {
			return fmt.Errorf("failed to get managed resources: %w", err)
		}

		if len(managedResources) == 0 {
			fmt.Printf("  ⚠ No managed resources found in namespace %s\n", destNamespace)
			fmt.Println("  (The Application may not be synced yet)")
		} else {
			fmt.Printf("  ✓ Found %d managed resources:\n", len(managedResources))
			for _, r := range managedResources {
				healthIcon := "✓"
				if r.Health != "Healthy" && r.Health != "" {
					healthIcon = "⚠"
				}
				fmt.Printf("    %s %s/%s (%s)\n", healthIcon, r.Kind, r.Name, r.Health)
			}
		}
	}
	fmt.Println()

	// Step 4: Extract labels from Git path
	fmt.Printf("Step 4: Extracting labels from Git path...\n")
	extractedLabels := extractLabelsFromPath(app.Source.Path)
	if len(extractedLabels) > 0 {
		fmt.Printf("  ✓ Extracted labels from path '%s':\n", app.Source.Path)
		for k, v := range extractedLabels {
			fmt.Printf("    %s=%s\n", k, v)
		}
	} else {
		fmt.Printf("  - No labels extracted from path: %s\n", app.Source.Path)
	}
	// Always set app label to the ArgoCD Application name if not already set
	if _, hasApp := extractedLabels["app"]; !hasApp {
		extractedLabels["app"] = appName
	}
	fmt.Println()

	// Determine space
	space := argoImportSpace
	if space == "" {
		space, err = getCurrentSpace()
		if err != nil {
			// Suggest using app name as space
			space = appName
			fmt.Printf("  Using app name as space: %s\n", space)
		}
	}

	// Summary
	fmt.Printf("Import Summary\n")
	fmt.Printf("--------------\n")
	fmt.Printf("Space: %s\n", space)
	if appOfApps {
		fmt.Printf("Type: App of Apps (manages %d child Applications)\n", len(childApps))
		fmt.Println()
		fmt.Printf("⚠ App of Apps detected - this Application manages other Applications,\n")
		fmt.Printf("  not workload resources. Import the child Applications instead:\n")
		for _, child := range childApps {
			fmt.Printf("  → cub-agent import-argocd %s\n", child)
		}
		fmt.Println()
		fmt.Println("No unit will be created for the App of Apps parent.")
		return nil
	}

	if len(managedResources) == 0 {
		fmt.Printf("⚠ No managed resources found - nothing to import.\n")
		return nil
	}

	fmt.Printf("Unit to create: %s\n", appName)
	fmt.Printf("  Resources: %d (", len(managedResources))
	kinds := make(map[string]int)
	for _, r := range managedResources {
		kinds[r.Kind]++
	}
	first := true
	for k, v := range kinds {
		if !first {
			fmt.Printf(", ")
		}
		fmt.Printf("%d %s", v, k)
		first = false
	}
	fmt.Printf(")\n")
	if len(extractedLabels) > 0 {
		fmt.Printf("  Labels: ")
		first = true
		for k, v := range extractedLabels {
			if k == "is_base" {
				continue // Don't show internal flag
			}
			if !first {
				fmt.Printf(", ")
			}
			fmt.Printf("%s=%s", k, v)
			first = false
		}
		fmt.Println()
	}
	fmt.Println()

	// Show YAML content if requested (implies dry-run)
	if argoImportShowYAML {
		if argoImportRaw {
			fmt.Println("=== YAML Preview (raw) ===")
		} else {
			fmt.Println("=== YAML Preview (cleaned) ===")
		}
		fmt.Println()

		fmt.Printf("--- Unit: %s ---\n", appName)
		for i, r := range managedResources {
			if i > 0 {
				fmt.Println("---")
			}
			displayYAML := r.YAML
			if !argoImportRaw {
				if cleaned, err := cleanResourceYAML(r.YAML); err == nil {
					displayYAML = cleaned
				}
			}
			fmt.Println(displayYAML)
		}
		fmt.Println()

		// Show what cleanup action would happen
		if argoImportDisableSync {
			fmt.Printf("After import: Would disable auto-sync on ArgoCD Application '%s'\n", appName)
		} else if argoImportDeleteApp {
			fmt.Printf("After import: Would delete ArgoCD Application '%s' (resources preserved)\n", appName)
		}

		fmt.Println("Dry-run mode: no changes will be made.")
		fmt.Println("\nTo import, run without --dry-run and --show-yaml")
		return nil
	}

	if argoImportDryRun {
		// Show what cleanup action would happen
		if argoImportDisableSync {
			fmt.Printf("\nAfter import: Would disable auto-sync on ArgoCD Application '%s'\n", appName)
		} else if argoImportDeleteApp {
			fmt.Printf("\nAfter import: Would delete ArgoCD Application '%s' (resources preserved)\n", appName)
		}

		fmt.Println("\nDry-run mode: no changes will be made.")
		fmt.Println("To import, run without --dry-run")
		return nil
	}

	// Confirm
	if !argoImportYes {
		fmt.Printf("Proceed with import? [y/N] ")
		if !confirm() {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Create space if needed
	if err := ensureSpace(space); err != nil {
		return fmt.Errorf("failed to ensure space: %w", err)
	}

	fmt.Println()

	// Create the unit with workload resources
	fmt.Printf("Creating unit: %s\n", appName)

	// Combine all managed resources into a single YAML
	var workloadYAML strings.Builder
	for i, r := range managedResources {
		if i > 0 {
			workloadYAML.WriteString("---\n")
		}
		resourceYAML := r.YAML
		if !argoImportRaw {
			if cleaned, err := cleanResourceYAML(r.YAML); err == nil {
				resourceYAML = cleaned
			}
		}
		workloadYAML.WriteString(resourceYAML)
		workloadYAML.WriteString("\n")
	}

	// Build labels string for unit creation
	var labelParts []string
	for k, v := range extractedLabels {
		if k == "is_base" {
			continue // Skip internal flag
		}
		labelParts = append(labelParts, fmt.Sprintf("%s=%s", k, v))
	}
	labelsArg := strings.Join(labelParts, ",")

	if err := createUnitWithConfigAndLabels(space, appName, workloadYAML.String(), labelsArg); err != nil {
		fmt.Printf("  ✗ Failed: %v\n", err)
		return fmt.Errorf("failed to create unit: %w", err)
	}
	fmt.Printf("  ✓ Created unit: %s (%d resources)\n", appName, len(managedResources))

	fmt.Println()
	fmt.Printf("Import complete: 1 unit created\n")
	fmt.Println()

	// Handle ArgoCD Application cleanup if requested
	if argoImportDisableSync {
		fmt.Println("Disabling auto-sync on ArgoCD Application...")
		if err := disableArgoAutoSync(ctx, dynamicClient, argoImportNamespace, appName); err != nil {
			fmt.Printf("  ✗ Failed to disable auto-sync: %v\n", err)
			fmt.Println("  You may need to manually disable sync in ArgoCD.")
		} else {
			fmt.Printf("  ✓ Auto-sync disabled on '%s'\n", appName)
			fmt.Println("  The Application is preserved for reference but won't sync automatically.")
		}
		fmt.Println()
	} else if argoImportDeleteApp {
		fmt.Println("Deleting ArgoCD Application (resources will be preserved)...")
		if err := deleteArgoApplication(ctx, dynamicClient, argoImportNamespace, appName); err != nil {
			fmt.Printf("  ✗ Failed to delete Application: %v\n", err)
			fmt.Println("  You may need to manually delete it in ArgoCD.")
		} else {
			fmt.Printf("  ✓ ArgoCD Application '%s' deleted\n", appName)
			fmt.Println("  The managed resources are preserved and now under ConfigHub control.")
		}
		fmt.Println()
	}

	// Handle ConfigHub pipeline tests if requested
	if argoImportTestUpdate {
		fmt.Println("Testing ConfigHub pipeline (annotation update)...")
		result, err := testAnnotationUpdate(space, appName)
		if err != nil {
			fmt.Printf("  ✗ Test failed: %v\n", err)
			fmt.Println("  The import succeeded, but ConfigHub couldn't update the resource.")
			fmt.Println("  Check that a worker is connected and the target is configured.")
		} else {
			fmt.Printf("  ✓ %s\n", result.Message)
			fmt.Println("  ConfigHub can successfully update resources on this target.")
		}
		fmt.Println()
	}

	if argoImportTestRollout {
		fmt.Println("Testing ConfigHub pipeline (rollout restart)...")
		result, err := testRolloutRestart(space, appName)
		if err != nil {
			fmt.Printf("  ✗ Test failed: %v\n", err)
			fmt.Println("  The import succeeded, but ConfigHub couldn't trigger a rollout.")
			fmt.Println("  Check that a worker is connected and the target is configured.")
		} else {
			fmt.Printf("  ✓ %s\n", result.Message)
			fmt.Println("  ConfigHub can successfully trigger rollouts on this target.")
			fmt.Println("  Note: Pods will restart. Watch with: kubectl get pods -n <namespace> -w")
		}
		fmt.Println()
	}

	fmt.Println("View your imported unit:")
	fmt.Printf("  cub unit get %s --space %s\n", appName, space)

	return nil
}

// getArgoApplication reads an ArgoCD Application CR and returns parsed info + raw YAML
func getArgoApplication(ctx context.Context, client dynamic.Interface, namespace, name string) (*ArgoApplication, string, error) {
	gvr := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}

	app, err := client.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, "", err
	}

	// Extract fields from unstructured
	spec, _, _ := unstructured.NestedMap(app.Object, "spec")
	status, _, _ := unstructured.NestedMap(app.Object, "status")

	// Parse source
	source, _, _ := unstructured.NestedMap(spec, "source")
	repoURL, _, _ := unstructured.NestedString(source, "repoURL")
	path, _, _ := unstructured.NestedString(source, "path")
	targetRevision, _, _ := unstructured.NestedString(source, "targetRevision")
	chart, _, _ := unstructured.NestedString(source, "chart")

	// Parse destination
	dest, _, _ := unstructured.NestedMap(spec, "destination")
	server, _, _ := unstructured.NestedString(dest, "server")
	destNamespace, _, _ := unstructured.NestedString(dest, "namespace")
	destName, _, _ := unstructured.NestedString(dest, "name")

	// Parse status
	syncStatus := "Unknown"
	healthStatus := "Unknown"
	if sync, ok, _ := unstructured.NestedMap(status, "sync"); ok {
		if s, ok, _ := unstructured.NestedString(sync, "status"); ok {
			syncStatus = s
		}
	}
	if health, ok, _ := unstructured.NestedMap(status, "health"); ok {
		if h, ok, _ := unstructured.NestedString(health, "status"); ok {
			healthStatus = h
		}
	}

	project, _, _ := unstructured.NestedString(spec, "project")

	argoApp := &ArgoApplication{
		Name:         name,
		Namespace:    namespace,
		Project:      project,
		SyncStatus:   syncStatus,
		HealthStatus: healthStatus,
		Source: ArgoSource{
			RepoURL:        repoURL,
			Path:           path,
			TargetRevision: targetRevision,
			Chart:          chart,
		},
		Destination: ArgoDestination{
			Server:    server,
			Namespace: destNamespace,
			Name:      destName,
		},
	}

	// Get raw YAML
	yamlBytes, err := yaml.Marshal(app.Object)
	if err != nil {
		return argoApp, "", err
	}

	return argoApp, string(yamlBytes), nil
}

// getStatusResources extracts the managed resources from .status.resources
func getStatusResources(ctx context.Context, client dynamic.Interface, namespace, name string) ([]ArgoStatusResource, error) {
	gvr := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}

	app, err := client.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	status, _, _ := unstructured.NestedMap(app.Object, "status")
	resourcesRaw, found, _ := unstructured.NestedSlice(status, "resources")
	if !found {
		return nil, nil
	}

	var resources []ArgoStatusResource
	for _, r := range resourcesRaw {
		rMap, ok := r.(map[string]interface{})
		if !ok {
			continue
		}

		res := ArgoStatusResource{}
		res.Group, _, _ = unstructured.NestedString(rMap, "group")
		res.Version, _, _ = unstructured.NestedString(rMap, "version")
		res.Kind, _, _ = unstructured.NestedString(rMap, "kind")
		res.Namespace, _, _ = unstructured.NestedString(rMap, "namespace")
		res.Name, _, _ = unstructured.NestedString(rMap, "name")
		res.Status, _, _ = unstructured.NestedString(rMap, "status")
		if health, found, _ := unstructured.NestedMap(rMap, "health"); found {
			res.Health, _, _ = unstructured.NestedString(health, "status")
		}

		resources = append(resources, res)
	}

	return resources, nil
}

// isAppOfApps checks if the Application manages other Application CRs (App of Apps pattern)
func isAppOfApps(resources []ArgoStatusResource) bool {
	for _, r := range resources {
		if r.Group == "argoproj.io" && r.Kind == "Application" {
			return true
		}
	}
	return false
}

// getChildApplicationNames returns the names of child Applications (for App of Apps)
func getChildApplicationNames(resources []ArgoStatusResource) []string {
	var names []string
	for _, r := range resources {
		if r.Group == "argoproj.io" && r.Kind == "Application" {
			names = append(names, r.Name)
		}
	}
	return names
}

// extractLabelsFromPath extracts ConfigHub labels from a Git path.
// Common patterns:
//   - overlays/prod → variant=prod
//   - overlays/staging → variant=staging
//   - apps/payment-api/overlays/prod → app=payment-api, variant=prod
//   - tenants/checkout/cart/overlays/dev → app=cart, variant=dev
func extractLabelsFromPath(path string) map[string]string {
	labels := make(map[string]string)
	if path == "" {
		return labels
	}

	parts := strings.Split(path, "/")

	// Look for overlays or environments pattern
	for i, part := range parts {
		switch part {
		case "overlays", "envs", "environments":
			// Next part is the variant
			if i+1 < len(parts) {
				labels["variant"] = parts[i+1]
			}
		case "prod", "production":
			labels["variant"] = "prod"
		case "staging", "stage":
			labels["variant"] = "staging"
		case "dev", "development":
			labels["variant"] = "dev"
		case "base":
			// This might indicate a base template - note it
			labels["is_base"] = "true"
		}
	}

	// Try to extract app name from path structure
	// Pattern: apps/{app-name}/... or {app-name}/overlays/...
	for i, part := range parts {
		if part == "apps" && i+1 < len(parts) {
			// Next part after "apps" is the app name
			appName := parts[i+1]
			if appName != "overlays" && appName != "base" {
				labels["app"] = appName
			}
			break
		}
	}

	return labels
}

// isArgoManagedResource checks if a resource is managed by the given ArgoCD Application
// ArgoCD uses either a label (argocd.argoproj.io/instance) or an annotation (argocd.argoproj.io/tracking-id)
func isArgoManagedResource(labels, annotations map[string]string, appName string) bool {
	// Check label first (some ArgoCD configs use this)
	if instance, ok := labels["argocd.argoproj.io/instance"]; ok {
		return instance == appName
	}

	// Check tracking-id annotation (format: "appName:group/Kind:namespace/name")
	if trackingID, ok := annotations["argocd.argoproj.io/tracking-id"]; ok {
		// tracking-id starts with "appName:"
		return strings.HasPrefix(trackingID, appName+":")
	}

	return false
}

// getManagedResources finds all resources in the destination namespace that are managed by the Application
func getManagedResources(ctx context.Context, clientset *kubernetes.Clientset, dynamicClient dynamic.Interface, appName, namespace string) ([]ManagedResource, error) {
	var resources []ManagedResource

	// Get Deployments - list all and filter by ArgoCD ownership
	deployments, err := clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, d := range deployments.Items {
			if !isArgoManagedResource(d.Labels, d.Annotations, appName) {
				continue
			}
			// Set TypeMeta for proper YAML output
			d.APIVersion = "apps/v1"
			d.Kind = "Deployment"
			yamlBytes, _ := yaml.Marshal(d)
			health := "Progressing"
			if d.Status.ReadyReplicas == *d.Spec.Replicas {
				health = "Healthy"
			}
			resources = append(resources, ManagedResource{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Namespace:  d.Namespace,
				Name:       d.Name,
				Health:     health,
				YAML:       string(yamlBytes),
			})
		}
	}

	// Get StatefulSets
	statefulsets, err := clientset.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, s := range statefulsets.Items {
			if !isArgoManagedResource(s.Labels, s.Annotations, appName) {
				continue
			}
			s.APIVersion = "apps/v1"
			s.Kind = "StatefulSet"
			yamlBytes, _ := yaml.Marshal(s)
			health := "Progressing"
			if s.Status.ReadyReplicas == *s.Spec.Replicas {
				health = "Healthy"
			}
			resources = append(resources, ManagedResource{
				APIVersion: "apps/v1",
				Kind:       "StatefulSet",
				Namespace:  s.Namespace,
				Name:       s.Name,
				Health:     health,
				YAML:       string(yamlBytes),
			})
		}
	}

	// Get Services
	services, err := clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, s := range services.Items {
			if !isArgoManagedResource(s.Labels, s.Annotations, appName) {
				continue
			}
			s.APIVersion = "v1"
			s.Kind = "Service"
			yamlBytes, _ := yaml.Marshal(s)
			resources = append(resources, ManagedResource{
				APIVersion: "v1",
				Kind:       "Service",
				Namespace:  s.Namespace,
				Name:       s.Name,
				Health:     "Healthy",
				YAML:       string(yamlBytes),
			})
		}
	}

	// Get ConfigMaps
	configmaps, err := clientset.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, c := range configmaps.Items {
			if !isArgoManagedResource(c.Labels, c.Annotations, appName) {
				continue
			}
			c.APIVersion = "v1"
			c.Kind = "ConfigMap"
			yamlBytes, _ := yaml.Marshal(c)
			resources = append(resources, ManagedResource{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Namespace:  c.Namespace,
				Name:       c.Name,
				Health:     "Healthy",
				YAML:       string(yamlBytes),
			})
		}
	}

	// Get Secrets (excluding service account tokens)
	secrets, err := clientset.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, s := range secrets.Items {
			if s.Type == "kubernetes.io/service-account-token" {
				continue
			}
			if !isArgoManagedResource(s.Labels, s.Annotations, appName) {
				continue
			}
			s.APIVersion = "v1"
			s.Kind = "Secret"
			yamlBytes, _ := yaml.Marshal(s)
			resources = append(resources, ManagedResource{
				APIVersion: "v1",
				Kind:       "Secret",
				Namespace:  s.Namespace,
				Name:       s.Name,
				Health:     "Healthy",
				YAML:       string(yamlBytes),
			})
		}
	}

	// Get Ingresses
	ingresses, err := clientset.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, i := range ingresses.Items {
			if !isArgoManagedResource(i.Labels, i.Annotations, appName) {
				continue
			}
			i.APIVersion = "networking.k8s.io/v1"
			i.Kind = "Ingress"
			yamlBytes, _ := yaml.Marshal(i)
			resources = append(resources, ManagedResource{
				APIVersion: "networking.k8s.io/v1",
				Kind:       "Ingress",
				Namespace:  i.Namespace,
				Name:       i.Name,
				Health:     "Healthy",
				YAML:       string(yamlBytes),
			})
		}
	}

	// Get DaemonSets
	daemonsets, err := clientset.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, d := range daemonsets.Items {
			if !isArgoManagedResource(d.Labels, d.Annotations, appName) {
				continue
			}
			d.APIVersion = "apps/v1"
			d.Kind = "DaemonSet"
			yamlBytes, _ := yaml.Marshal(d)
			health := "Progressing"
			if d.Status.NumberReady == d.Status.DesiredNumberScheduled {
				health = "Healthy"
			}
			resources = append(resources, ManagedResource{
				APIVersion: "apps/v1",
				Kind:       "DaemonSet",
				Namespace:  d.Namespace,
				Name:       d.Name,
				Health:     health,
				YAML:       string(yamlBytes),
			})
		}
	}

	return resources, nil
}

// ArgoAppDetails holds detailed info about an ArgoCD Application for the TUI
type ArgoAppDetails struct {
	Name         string
	Namespace    string
	Project      string
	RepoURL      string
	Path         string
	DestServer   string
	DestNS       string
	SyncStatus   string
	HealthStatus string
	IsAppOfApps  bool     // True if this app manages other Application CRs
	ChildApps    []string // Names of child applications (if App of Apps)
}

// listArgoApplicationsDetailed returns detailed info about all ArgoCD Applications
func listArgoApplicationsDetailed(ctx context.Context, client dynamic.Interface, namespace string) ([]ArgoAppDetails, error) {
	gvr := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}

	list, err := client.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var apps []ArgoAppDetails
	for _, item := range list.Items {
		app := ArgoAppDetails{
			Name:      item.GetName(),
			Namespace: item.GetNamespace(),
		}

		// Get spec
		spec, _, _ := unstructured.NestedMap(item.Object, "spec")
		app.Project, _, _ = unstructured.NestedString(spec, "project")

		// Source info
		source, _, _ := unstructured.NestedMap(spec, "source")
		app.RepoURL, _, _ = unstructured.NestedString(source, "repoURL")
		app.Path, _, _ = unstructured.NestedString(source, "path")

		// Destination info
		dest, _, _ := unstructured.NestedMap(spec, "destination")
		app.DestServer, _, _ = unstructured.NestedString(dest, "server")
		app.DestNS, _, _ = unstructured.NestedString(dest, "namespace")

		// Status
		status, _, _ := unstructured.NestedMap(item.Object, "status")
		if sync, ok, _ := unstructured.NestedMap(status, "sync"); ok {
			app.SyncStatus, _, _ = unstructured.NestedString(sync, "status")
		}
		if health, ok, _ := unstructured.NestedMap(status, "health"); ok {
			app.HealthStatus, _, _ = unstructured.NestedString(health, "status")
		}

		// Check if App of Apps by looking for Application resources in status.resources
		if resources, ok, _ := unstructured.NestedSlice(status, "resources"); ok {
			for _, r := range resources {
				if res, ok := r.(map[string]interface{}); ok {
					kind, _, _ := unstructured.NestedString(res, "kind")
					group, _, _ := unstructured.NestedString(res, "group")
					if kind == "Application" && group == "argoproj.io" {
						app.IsAppOfApps = true
						name, _, _ := unstructured.NestedString(res, "name")
						if name != "" {
							app.ChildApps = append(app.ChildApps, name)
						}
					}
				}
			}
		}

		apps = append(apps, app)
	}

	return apps, nil
}

// verifyArgoInstalled checks if ArgoCD CRDs are installed
func verifyArgoInstalled(ctx context.Context, client dynamic.Interface) error {
	gvr := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}

	_, err := client.Resource(gvr).Namespace("argocd").List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return fmt.Errorf("ArgoCD not installed or not accessible: %w", err)
	}
	return nil
}

// listArgoApplicationsCmd lists all ArgoCD Applications with details
func listArgoApplicationsCmd(ctx context.Context, client dynamic.Interface, namespace string) error {
	gvr := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}

	list, err := client.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list Applications: %w", err)
	}

	if len(list.Items) == 0 {
		fmt.Printf("No ArgoCD Applications found in namespace '%s'\n", namespace)
		return nil
	}

	fmt.Printf("ArgoCD Applications in namespace '%s'\n", namespace)
	fmt.Println("======================================")
	fmt.Println()
	fmt.Printf("%-25s %-12s %-12s %s\n", "NAME", "SYNC", "HEALTH", "DESTINATION")
	fmt.Printf("%-25s %-12s %-12s %s\n", "----", "----", "------", "-----------")

	for _, item := range list.Items {
		name := item.GetName()

		// Get status
		status, _, _ := unstructured.NestedMap(item.Object, "status")
		syncStatus := "Unknown"
		healthStatus := "Unknown"
		if sync, ok, _ := unstructured.NestedMap(status, "sync"); ok {
			if s, ok, _ := unstructured.NestedString(sync, "status"); ok {
				syncStatus = s
			}
		}
		if health, ok, _ := unstructured.NestedMap(status, "health"); ok {
			if h, ok, _ := unstructured.NestedString(health, "status"); ok {
				healthStatus = h
			}
		}

		// Get destination
		spec, _, _ := unstructured.NestedMap(item.Object, "spec")
		dest, _, _ := unstructured.NestedMap(spec, "destination")
		destNamespace, _, _ := unstructured.NestedString(dest, "namespace")
		destServer, _, _ := unstructured.NestedString(dest, "server")
		if destServer == "https://kubernetes.default.svc" {
			destServer = "local"
		}

		destination := fmt.Sprintf("%s:%s", destServer, destNamespace)
		if len(destination) > 40 {
			destination = destination[:37] + "..."
		}

		fmt.Printf("%-25s %-12s %-12s %s\n", name, syncStatus, healthStatus, destination)
	}

	fmt.Println()
	fmt.Println("To import an application:")
	fmt.Println("  cub-agent import-argocd <application-name> --dry-run")

	return nil
}

// cleanResourceYAML removes runtime fields from a resource YAML to produce
// clean, minimal declarative YAML suitable for ConfigHub units.
func cleanResourceYAML(yamlStr string) (string, error) {
	var obj map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlStr), &obj); err != nil {
		return yamlStr, err // Return original on error
	}

	// Remove status block entirely
	delete(obj, "status")

	// Clean metadata
	cleanMetadata(obj)

	// Clean spec based on resource kind
	if spec, ok := obj["spec"].(map[string]interface{}); ok {
		kind, _ := obj["kind"].(string)
		cleanSpec(spec, kind)
	}

	cleanedYAML, err := yaml.Marshal(obj)
	if err != nil {
		return yamlStr, err
	}
	return string(cleanedYAML), nil
}

// cleanMetadata removes runtime fields and controller-specific annotations
func cleanMetadata(obj map[string]interface{}) {
	metadata, ok := obj["metadata"].(map[string]interface{})
	if !ok {
		return
	}

	// Remove runtime fields
	delete(metadata, "managedFields")
	delete(metadata, "resourceVersion")
	delete(metadata, "uid")
	delete(metadata, "generation")
	delete(metadata, "creationTimestamp")
	delete(metadata, "selfLink")
	delete(metadata, "ownerReferences")

	// Clean annotations - remove controller bookkeeping
	if annotations, ok := metadata["annotations"].(map[string]interface{}); ok {
		controllerAnnotationPrefixes := []string{
			"argocd.argoproj.io/",
			"kubectl.kubernetes.io/",
			"deployment.kubernetes.io/",
			"kubernetes.io/",
			"meta.helm.sh/",
			"helm.sh/",
			"fluxcd.io/",
			"kustomize.toolkit.fluxcd.io/",
			"config.k8s.io/", // ConfigHub inventory ownership
		}
		controllerAnnotations := []string{
			"kubectl.kubernetes.io/last-applied-configuration",
			"deployment.kubernetes.io/revision",
			"deprecated.daemonset.template.generation",
		}

		for key := range annotations {
			// Check exact matches
			for _, ann := range controllerAnnotations {
				if key == ann {
					delete(annotations, key)
					break
				}
			}
			// Check prefix matches
			for _, prefix := range controllerAnnotationPrefixes {
				if strings.HasPrefix(key, prefix) {
					delete(annotations, key)
					break
				}
			}
		}
		if len(annotations) == 0 {
			delete(metadata, "annotations")
		}
	}

	// Clean labels - remove controller bookkeeping labels
	if labels, ok := metadata["labels"].(map[string]interface{}); ok {
		controllerLabelPrefixes := []string{
			"app.kubernetes.io/managed-by",
			"helm.sh/",
			"argocd.argoproj.io/",
			"confighub.com/", // ConfigHub unit labels
			"cli-utils.sigs.k8s.io/", // ConfigHub inventory labels
		}
		for key := range labels {
			for _, prefix := range controllerLabelPrefixes {
				if strings.HasPrefix(key, prefix) {
					delete(labels, key)
					break
				}
			}
		}
		if len(labels) == 0 {
			delete(metadata, "labels")
		}
	}
}

// cleanSpec removes defaulted fields from the spec
func cleanSpec(spec map[string]interface{}, kind string) {
	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet":
		cleanWorkloadSpec(spec)
	case "Service":
		cleanServiceSpec(spec)
	case "ConfigMap", "Secret":
		// These are usually minimal, nothing special to clean
	}
}

// cleanWorkloadSpec removes defaulted fields from Deployment/StatefulSet/DaemonSet specs
func cleanWorkloadSpec(spec map[string]interface{}) {
	// Remove defaulted top-level fields
	if progressDeadline, ok := spec["progressDeadlineSeconds"].(int64); ok && progressDeadline == 600 {
		delete(spec, "progressDeadlineSeconds")
	}
	if progressDeadline, ok := spec["progressDeadlineSeconds"].(int); ok && progressDeadline == 600 {
		delete(spec, "progressDeadlineSeconds")
	}
	delete(spec, "revisionHistoryLimit") // Usually defaulted

	// Clean strategy if it's the default RollingUpdate
	if strategy, ok := spec["strategy"].(map[string]interface{}); ok {
		strategyType, _ := strategy["type"].(string)
		if strategyType == "RollingUpdate" || strategyType == "" {
			// Check if rollingUpdate is default (25%/25%)
			if ru, ok := strategy["rollingUpdate"].(map[string]interface{}); ok {
				maxSurge, _ := ru["maxSurge"].(string)
				maxUnavail, _ := ru["maxUnavailable"].(string)
				if (maxSurge == "25%" || maxSurge == "") && (maxUnavail == "25%" || maxUnavail == "") {
					delete(spec, "strategy")
				}
			} else if len(strategy) <= 1 {
				delete(spec, "strategy")
			}
		}
	}

	// Clean pod template
	if template, ok := spec["template"].(map[string]interface{}); ok {
		// Clean template metadata
		cleanMetadata(template)

		if podSpec, ok := template["spec"].(map[string]interface{}); ok {
			cleanPodSpec(podSpec)
		}
	}
}

// cleanPodSpec removes defaulted fields from a pod spec
func cleanPodSpec(podSpec map[string]interface{}) {
	// Remove defaulted fields
	if dnsPolicy, ok := podSpec["dnsPolicy"].(string); ok && dnsPolicy == "ClusterFirst" {
		delete(podSpec, "dnsPolicy")
	}
	if restartPolicy, ok := podSpec["restartPolicy"].(string); ok && restartPolicy == "Always" {
		delete(podSpec, "restartPolicy")
	}
	if schedulerName, ok := podSpec["schedulerName"].(string); ok && schedulerName == "default-scheduler" {
		delete(podSpec, "schedulerName")
	}
	delete(podSpec, "terminationGracePeriodSeconds") // Usually defaulted to 30

	// Remove empty securityContext
	if sc, ok := podSpec["securityContext"].(map[string]interface{}); ok && len(sc) == 0 {
		delete(podSpec, "securityContext")
	}

	// Remove serviceAccountName if it's "default"
	if sa, ok := podSpec["serviceAccountName"].(string); ok && sa == "default" {
		delete(podSpec, "serviceAccountName")
	}
	delete(podSpec, "serviceAccount") // Deprecated field

	// Clean containers
	if containers, ok := podSpec["containers"].([]interface{}); ok {
		for _, c := range containers {
			if container, ok := c.(map[string]interface{}); ok {
				cleanContainer(container)
			}
		}
	}

	// Clean initContainers
	if initContainers, ok := podSpec["initContainers"].([]interface{}); ok {
		for _, c := range initContainers {
			if container, ok := c.(map[string]interface{}); ok {
				cleanContainer(container)
			}
		}
	}
}

// cleanContainer removes defaulted fields from a container spec
func cleanContainer(container map[string]interface{}) {
	// Remove defaulted imagePullPolicy
	if policy, ok := container["imagePullPolicy"].(string); ok {
		image, _ := container["image"].(string)
		// Default is IfNotPresent for non-latest, Always for latest
		if policy == "IfNotPresent" && !strings.HasSuffix(image, ":latest") {
			delete(container, "imagePullPolicy")
		}
		if policy == "Always" && strings.HasSuffix(image, ":latest") {
			delete(container, "imagePullPolicy")
		}
	}

	// Remove defaulted terminationMessagePath/Policy
	if tmp, ok := container["terminationMessagePath"].(string); ok && tmp == "/dev/termination-log" {
		delete(container, "terminationMessagePath")
	}
	if tmp, ok := container["terminationMessagePolicy"].(string); ok && tmp == "File" {
		delete(container, "terminationMessagePolicy")
	}

	// Remove empty resources
	if res, ok := container["resources"].(map[string]interface{}); ok && len(res) == 0 {
		delete(container, "resources")
	}

	// Remove empty securityContext
	if sc, ok := container["securityContext"].(map[string]interface{}); ok && len(sc) == 0 {
		delete(container, "securityContext")
	}

	// Clean ports - remove defaulted protocol
	if ports, ok := container["ports"].([]interface{}); ok {
		for _, p := range ports {
			if port, ok := p.(map[string]interface{}); ok {
				if proto, ok := port["protocol"].(string); ok && proto == "TCP" {
					delete(port, "protocol")
				}
			}
		}
	}

	// Clean env - remove empty valueFrom
	if envVars, ok := container["env"].([]interface{}); ok {
		for _, e := range envVars {
			if env, ok := e.(map[string]interface{}); ok {
				if vf, ok := env["valueFrom"].(map[string]interface{}); ok && len(vf) == 0 {
					delete(env, "valueFrom")
				}
			}
		}
	}

	// Clean volumeMounts - remove defaulted readOnly: false
	if mounts, ok := container["volumeMounts"].([]interface{}); ok {
		for _, m := range mounts {
			if mount, ok := m.(map[string]interface{}); ok {
				if ro, ok := mount["readOnly"].(bool); ok && !ro {
					delete(mount, "readOnly")
				}
			}
		}
	}
}

// cleanServiceSpec removes defaulted fields from Service specs
func cleanServiceSpec(spec map[string]interface{}) {
	// Remove defaulted sessionAffinity
	if sa, ok := spec["sessionAffinity"].(string); ok && sa == "None" {
		delete(spec, "sessionAffinity")
	}

	// Remove defaulted type
	if svcType, ok := spec["type"].(string); ok && svcType == "ClusterIP" {
		delete(spec, "type")
	}

	// Remove clusterIP/clusterIPs (assigned at runtime)
	delete(spec, "clusterIP")
	delete(spec, "clusterIPs")
	delete(spec, "ipFamilies")
	delete(spec, "ipFamilyPolicy")

	// Clean ports - remove defaulted protocol and targetPort if same as port
	if ports, ok := spec["ports"].([]interface{}); ok {
		for _, p := range ports {
			if port, ok := p.(map[string]interface{}); ok {
				if proto, ok := port["protocol"].(string); ok && proto == "TCP" {
					delete(port, "protocol")
				}
				// Remove targetPort if it equals port
				portNum := port["port"]
				targetPort := port["targetPort"]
				if portNum == targetPort {
					delete(port, "targetPort")
				}
			}
		}
	}
}

// disableArgoAutoSync removes the automated sync policy from an ArgoCD Application.
// This keeps the Application for reference but stops it from automatically syncing.
func disableArgoAutoSync(ctx context.Context, client dynamic.Interface, namespace, name string) error {
	gvr := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}

	// Use strategic merge patch to remove the automated sync policy
	// Setting automated to null effectively disables auto-sync
	patchData := []byte(`{"spec":{"syncPolicy":{"automated":null}}}`)

	_, err := client.Resource(gvr).Namespace(namespace).Patch(
		ctx,
		name,
		types.MergePatchType,
		patchData,
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to disable auto-sync: %w", err)
	}

	return nil
}

// deleteArgoApplication deletes an ArgoCD Application entirely.
// This removes ArgoCD control over the resources (resources themselves are NOT deleted).
func deleteArgoApplication(ctx context.Context, client dynamic.Interface, namespace, name string) error {
	gvr := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}

	// Use non-cascading delete to avoid deleting the managed resources
	// ArgoCD defaults to cascading delete, so we explicitly set it to orphan
	deletePolicy := metav1.DeletePropagationOrphan
	err := client.Resource(gvr).Namespace(namespace).Delete(
		ctx,
		name,
		metav1.DeleteOptions{
			PropagationPolicy: &deletePolicy,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to delete Application: %w", err)
	}

	return nil
}

// TestUpdateResult holds the result of a ConfigHub pipeline test
type TestUpdateResult struct {
	Success      bool
	UnitSlug     string
	ResourceName string
	Annotation   string
	Message      string
}

// testAnnotationUpdate tests the ConfigHub pipeline by adding an annotation to a resource.
// This verifies the full pipeline: unit update → apply → target resource updated.
func testAnnotationUpdate(space, unitSlug string) (*TestUpdateResult, error) {
	result := &TestUpdateResult{
		UnitSlug: unitSlug,
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	annotationKey := "confighub.com/test-update"
	annotationValue := timestamp

	result.Annotation = fmt.Sprintf("%s=%s", annotationKey, annotationValue)

	// Step 1: Get current unit config
	getCmd := exec.Command("cub", "unit", "get", unitSlug, "--space", space, "--json", "--quiet")
	var getOut bytes.Buffer
	getCmd.Stdout = &getOut
	getCmd.Stderr = &getOut
	if err := getCmd.Run(); err != nil {
		return result, fmt.Errorf("failed to get unit config: %s", getOut.String())
	}

	// Step 2: Get the unit's config data
	configCmd := exec.Command("cub", "unit", "livedata", unitSlug, "--space", space)
	var configOut bytes.Buffer
	configCmd.Stdout = &configOut
	configCmd.Stderr = &configOut
	if err := configCmd.Run(); err != nil {
		return result, fmt.Errorf("failed to get unit livedata: %s", configOut.String())
	}

	configYAML := configOut.String()
	if configYAML == "" {
		return result, fmt.Errorf("unit has no config data")
	}

	// Step 3: Parse and modify the YAML to add annotation
	modifiedYAML, resourceName, err := addAnnotationToYAML(configYAML, annotationKey, annotationValue)
	if err != nil {
		return result, fmt.Errorf("failed to modify YAML: %w", err)
	}
	result.ResourceName = resourceName

	// Step 4: Update the unit with modified config
	updateCmd := exec.Command("cub", "unit", "update", unitSlug, "-", "--space", space, "--change-desc", "Test update: added ConfigHub annotation")
	updateCmd.Stdin = strings.NewReader(modifiedYAML)
	var updateOut bytes.Buffer
	updateCmd.Stdout = &updateOut
	updateCmd.Stderr = &updateOut
	if err := updateCmd.Run(); err != nil {
		return result, fmt.Errorf("failed to update unit: %s", updateOut.String())
	}

	// Step 5: Apply the unit to push changes to target
	applyCmd := exec.Command("cub", "unit", "apply", unitSlug, "--space", space, "--wait")
	var applyOut bytes.Buffer
	applyCmd.Stdout = &applyOut
	applyCmd.Stderr = &applyOut
	if err := applyCmd.Run(); err != nil {
		return result, fmt.Errorf("failed to apply unit: %s", applyOut.String())
	}

	result.Success = true
	result.Message = fmt.Sprintf("Successfully added annotation %s to %s", result.Annotation, resourceName)
	return result, nil
}

// testRolloutRestart tests the ConfigHub pipeline by triggering a rollout restart.
// This modifies the pod template annotation to trigger Kubernetes to do a rolling update.
func testRolloutRestart(space, unitSlug string) (*TestUpdateResult, error) {
	result := &TestUpdateResult{
		UnitSlug: unitSlug,
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	annotationKey := "kubectl.kubernetes.io/restartedAt"
	annotationValue := timestamp

	result.Annotation = fmt.Sprintf("%s=%s", annotationKey, annotationValue)

	// Step 1: Get the unit's config data (with retry - livedata may not be available immediately)
	// Keep trying until we get livedata or user cancels. ConfigHub apply timeout is 10 minutes.
	var configYAML string
	startTime := time.Now()
	for {
		configCmd := exec.Command("cub", "unit", "livedata", unitSlug, "--space", space)
		var configOut bytes.Buffer
		configCmd.Stdout = &configOut
		configCmd.Stderr = &configOut
		if err := configCmd.Run(); err == nil && configOut.Len() > 0 {
			configYAML = configOut.String()
			break
		}
		// After 2 minutes, fail with helpful message (worker may not be running)
		elapsed := time.Since(startTime)
		if elapsed > 2*time.Minute {
			return result, fmt.Errorf("timed out waiting for livedata after %v - ensure worker is running and connected", elapsed.Round(time.Second))
		}
		// Wait before retry - livedata appears after worker applies unit
		time.Sleep(3 * time.Second)
	}

	// Step 2: Parse and modify the YAML to add restart annotation to pod template
	modifiedYAML, resourceName, err := addRolloutAnnotationToYAML(configYAML, annotationKey, annotationValue)
	if err != nil {
		return result, fmt.Errorf("failed to modify YAML for rollout: %w", err)
	}
	result.ResourceName = resourceName

	// Step 3: Update the unit with modified config
	updateCmd := exec.Command("cub", "unit", "update", unitSlug, "-", "--space", space, "--change-desc", "Test rollout: triggered restart")
	updateCmd.Stdin = strings.NewReader(modifiedYAML)
	var updateOut bytes.Buffer
	updateCmd.Stdout = &updateOut
	updateCmd.Stderr = &updateOut
	if err := updateCmd.Run(); err != nil {
		return result, fmt.Errorf("failed to update unit: %s", updateOut.String())
	}

	// Step 4: Apply the unit to push changes to target
	applyCmd := exec.Command("cub", "unit", "apply", unitSlug, "--space", space, "--wait")
	var applyOut bytes.Buffer
	applyCmd.Stdout = &applyOut
	applyCmd.Stderr = &applyOut
	if err := applyCmd.Run(); err != nil {
		return result, fmt.Errorf("failed to apply unit: %s", applyOut.String())
	}

	result.Success = true
	result.Message = fmt.Sprintf("Successfully triggered rollout restart for %s", resourceName)
	return result, nil
}

// addAnnotationToYAML adds an annotation to the first resource in a YAML document.
// Returns the modified YAML and the name of the resource that was modified.
func addAnnotationToYAML(yamlStr, key, value string) (string, string, error) {
	// Split multi-document YAML
	docs := strings.Split(yamlStr, "\n---")
	if len(docs) == 0 {
		return "", "", fmt.Errorf("no YAML documents found")
	}

	// Find first Deployment or other workload resource
	var modifiedDocs []string
	var resourceName string
	modified := false

	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}

		var obj map[string]interface{}
		if err := yaml.Unmarshal([]byte(doc), &obj); err != nil {
			modifiedDocs = append(modifiedDocs, doc)
			continue
		}

		kind, _, _ := unstructured.NestedString(obj, "kind")

		// Only modify the first Deployment we find
		if !modified && kind == "Deployment" {
			metadata, ok := obj["metadata"].(map[string]interface{})
			if !ok {
				metadata = make(map[string]interface{})
				obj["metadata"] = metadata
			}

			annotations, ok := metadata["annotations"].(map[string]interface{})
			if !ok {
				annotations = make(map[string]interface{})
				metadata["annotations"] = annotations
			}

			annotations[key] = value
			modified = true
			resourceName, _, _ = unstructured.NestedString(obj, "metadata", "name")
		}

		modifiedYAML, err := yaml.Marshal(obj)
		if err != nil {
			modifiedDocs = append(modifiedDocs, doc)
		} else {
			modifiedDocs = append(modifiedDocs, string(modifiedYAML))
		}
	}

	if !modified {
		return "", "", fmt.Errorf("no Deployment found to annotate")
	}

	return strings.Join(modifiedDocs, "---\n"), resourceName, nil
}

// addRolloutAnnotationToYAML adds a restart annotation to the pod template spec.
// This triggers a rolling update when applied.
func addRolloutAnnotationToYAML(yamlStr, key, value string) (string, string, error) {
	// Split multi-document YAML
	docs := strings.Split(yamlStr, "\n---")
	if len(docs) == 0 {
		return "", "", fmt.Errorf("no YAML documents found")
	}

	var modifiedDocs []string
	var resourceName string
	modified := false

	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}

		var obj map[string]interface{}
		if err := yaml.Unmarshal([]byte(doc), &obj); err != nil {
			modifiedDocs = append(modifiedDocs, doc)
			continue
		}

		kind, _, _ := unstructured.NestedString(obj, "kind")

		// Only modify the first Deployment we find
		if !modified && kind == "Deployment" {
			// Navigate to spec.template.metadata.annotations
			spec, ok := obj["spec"].(map[string]interface{})
			if !ok {
				modifiedDocs = append(modifiedDocs, doc)
				continue
			}

			template, ok := spec["template"].(map[string]interface{})
			if !ok {
				template = make(map[string]interface{})
				spec["template"] = template
			}

			templateMeta, ok := template["metadata"].(map[string]interface{})
			if !ok {
				templateMeta = make(map[string]interface{})
				template["metadata"] = templateMeta
			}

			annotations, ok := templateMeta["annotations"].(map[string]interface{})
			if !ok {
				annotations = make(map[string]interface{})
				templateMeta["annotations"] = annotations
			}

			annotations[key] = value
			modified = true
			resourceName, _, _ = unstructured.NestedString(obj, "metadata", "name")
		}

		modifiedYAML, err := yaml.Marshal(obj)
		if err != nil {
			modifiedDocs = append(modifiedDocs, doc)
		} else {
			modifiedDocs = append(modifiedDocs, string(modifiedYAML))
		}
	}

	if !modified {
		return "", "", fmt.Errorf("no Deployment found to trigger rollout")
	}

	return strings.Join(modifiedDocs, "---\n"), resourceName, nil
}
