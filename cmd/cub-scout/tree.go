// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/confighub/cub-scout/internal/mapsvc"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var (
	treeJSON      bool   // deprecated: use --format json
	treeFormat    string // output format: ascii, json
	treeNamespace string
	treeAll       bool
	treeSpace     string // For ConfigHub tree
	treeEdge      string // For ConfigHub tree (clone/link)
	treeOwner     string // Filter by owner (Flux, ArgoCD, Helm, ConfigHub, Native)
	treeDepth     int    // Limit tree depth (0 = unlimited)
)

var treeCmd = &cobra.Command{
	Use:   "tree [type]",
	Short: "Show hierarchical views of resources",
	Long: `Show hierarchical views of cluster resources, Git repos, or ConfigHub units.

cub-scout tree provides different perspectives on your infrastructure:

  CLUSTER VIEWS (what the scout sees):
    runtime     Deployment -> ReplicaSet -> Pod trees (default)
    ownership   Resources grouped by GitOps owner (Flux, ArgoCD, Helm)
    composition Crossplane XR -> composed resources (managed by Crossplane)
    workloads   Same as 'cub-scout map workloads' (alias)

  GIT VIEWS:
    git         Git repository structure from detected sources
    patterns    Detected GitOps patterns (Arnie, Banko, Fluxy, D2)

  CONFIGHUB VIEWS (wraps 'cub unit tree'):
    config      ConfigHub Unit inheritance (--edge clone) or dependencies (--edge link)
    suggest     Suggested App model organization based on cluster workloads

Examples:
  # Show runtime hierarchy (Deployment -> ReplicaSet -> Pod)
  cub-scout tree
  cub-scout tree runtime

  # Show resources by GitOps owner
  cub-scout tree ownership

  # Show Crossplane composition trees (XR -> composed resources)
  cub-scout tree composition

  # Show Git repository structure
  cub-scout tree git

  # Show ConfigHub unit relationships (requires cub CLI)
  cub-scout tree config --space my-space
  cub-scout tree config --space "*" --edge link

The 'tree' command complements 'cub unit tree' in the ConfigHub CLI:
  - cub-scout tree: What's deployed in THIS cluster
  - cub unit tree:  How Units relate ACROSS your fleet
`,
	Args:      cobra.MaximumNArgs(1),
	ValidArgs: []string{"runtime", "ownership", "composition", "workloads", "git", "patterns", "config", "suggest"},
	RunE:      runTree,
}

func init() {
	rootCmd.AddCommand(treeCmd)

	treeCmd.Flags().StringVar(&treeFormat, "format", "ascii", "Output format: ascii, json, md")
	treeCmd.Flags().BoolVar(&treeJSON, "json", false, "Output as JSON (deprecated: use --format json)")
	treeCmd.Flags().StringVarP(&treeNamespace, "namespace", "n", "", "Filter by namespace")
	treeCmd.Flags().BoolVarP(&treeAll, "all", "A", false, "Show all resources including system namespaces")
	treeCmd.Flags().StringVar(&treeSpace, "space", "", "ConfigHub space for 'config' view (use '*' for all spaces)")
	treeCmd.Flags().StringVar(&treeEdge, "edge", "clone", "Edge type for 'config' view: clone (inheritance) or link (dependencies)")
	treeCmd.Flags().StringVar(&treeOwner, "owner", "", "Filter by owner: Flux, ArgoCD, Helm, ConfigHub, Native")
	treeCmd.Flags().IntVar(&treeDepth, "depth", 0, "Limit tree depth (0 = unlimited)")

	// Mark --json as deprecated
	_ = treeCmd.Flags().MarkDeprecated("json", "use --format json instead")
}

func runTree(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	viewType := "runtime"
	if len(args) > 0 {
		viewType = args[0]
	}

	switch viewType {
	case "runtime":
		return runTreeRuntime(ctx)
	case "ownership":
		return runTreeOwnership(ctx)
	case "composition":
		return runTreeComposition(ctx)
	case "workloads":
		return runTreeWorkloads()
	case "git":
		return runTreeGit(ctx)
	case "patterns":
		return runTreePatterns()
	case "config":
		return runTreeConfig()
	case "suggest":
		return runTreeSuggest(ctx)
	default:
		return fmt.Errorf("unknown tree type: %s (valid: runtime, ownership, composition, workloads, git, patterns, config, suggest)", viewType)
	}
}

// RuntimeTree represents a deployment with its children
type RuntimeTree struct {
	Name        string           `json:"name"`
	Namespace   string           `json:"namespace"`
	Kind        string           `json:"kind"`
	Owner       string           `json:"owner"`
	Status      string           `json:"status"`
	ReplicaSets []ReplicaSetNode `json:"replicaSets,omitempty"`
	Pods        []PodNode        `json:"pods,omitempty"` // For StatefulSets/DaemonSets
}

type ReplicaSetNode struct {
	Name   string    `json:"name"`
	Status string    `json:"status"`
	Pods   []PodNode `json:"pods"`
}

type PodNode struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Node   string `json:"node,omitempty"`
}

type treeGitRepoJSON struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	URL       string `json:"url"`
}

type treeGitAppJSON struct {
	Name                      string `json:"name"`
	Namespace                 string `json:"namespace"`
	RepoURL                   string `json:"repoURL"`
	Path                      string `json:"path"`
	GeneratedByApplicationSet string `json:"generatedByApplicationSet,omitempty"`
	// ApplicationSetLinkStatus is "resolved", "orphan", or "unknown" when
	// generatedByApplicationSet is present.
	ApplicationSetLinkStatus string   `json:"applicationSetLinkStatus,omitempty"`
	ParentApplication        string   `json:"parentApplication,omitempty"`
	LineageConfidence        string   `json:"lineageConfidence,omitempty"` // "explicit" (ownerRef) or "inferred" (label/annotation)
	ChildApplications        []string `json:"childApplications,omitempty"`
}

type treeGitApplicationSetJSON struct {
	Name                  string   `json:"name"`
	Namespace             string   `json:"namespace"`
	RepoURL               string   `json:"repoURL,omitempty"`
	Path                  string   `json:"path,omitempty"`
	GeneratorTypes        []string `json:"generatorTypes,omitempty"`
	GeneratedApplications []string `json:"generatedApplications,omitempty"`
}

type treeGitJSON struct {
	GitRepositories  []treeGitRepoJSON           `json:"gitRepositories"`
	ArgoApplications []treeGitAppJSON            `json:"argoApplications"`
	ApplicationSets  []treeGitApplicationSetJSON `json:"applicationSets"`
}

func runTreeRuntime(ctx context.Context) error {
	// TEST HOOK: Load tree data from JSON file to bypass cluster access in tests.
	// In production this env var is never set, so real K8s collection is always used.
	if treeJSON := os.Getenv("CUB_SCOUT_TEST_TREE_JSON"); treeJSON != "" {
		return loadAndRenderTreeFromJSON(treeJSON)
	}

	cfg, err := buildConfig()
	if err != nil {
		return fmt.Errorf("failed to build config: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	// Get Deployments
	deployGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	deploys, err := dynClient.Resource(deployGVR).Namespace(treeNamespace).List(ctx, v1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list deployments: %w", err)
	}

	// Get ReplicaSets
	rsGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}
	replicaSets, err := dynClient.Resource(rsGVR).Namespace(treeNamespace).List(ctx, v1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list replicasets: %w", err)
	}

	// Get Pods
	podGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	pods, err := dynClient.Resource(podGVR).Namespace(treeNamespace).List(ctx, v1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	// Build index: RS name -> pods
	rsToPods := make(map[string][]PodNode)
	for _, pod := range pods.Items {
		ns := pod.GetNamespace()
		if !treeAll && isSystemNamespace(ns) {
			continue
		}
		for _, ownerRef := range pod.GetOwnerReferences() {
			if ownerRef.Kind == "ReplicaSet" {
				key := ns + "/" + ownerRef.Name
				phase := "Unknown"
				if status, ok := pod.Object["status"].(map[string]interface{}); ok {
					if p, ok := status["phase"].(string); ok {
						phase = p
					}
				}
				rsToPods[key] = append(rsToPods[key], PodNode{
					Name:   pod.GetName(),
					Status: phase,
				})
			}
		}
	}

	// Build index: Deployment name -> ReplicaSets
	deployToRS := make(map[string][]string)
	rsStatus := make(map[string]string)
	for _, rs := range replicaSets.Items {
		ns := rs.GetNamespace()
		if !treeAll && isSystemNamespace(ns) {
			continue
		}
		for _, ownerRef := range rs.GetOwnerReferences() {
			if ownerRef.Kind == "Deployment" {
				key := ns + "/" + ownerRef.Name
				rsKey := ns + "/" + rs.GetName()
				deployToRS[key] = append(deployToRS[key], rsKey)

				// Get RS status
				replicas := int64(0)
				readyReplicas := int64(0)
				if spec, ok := rs.Object["spec"].(map[string]interface{}); ok {
					if r, ok := spec["replicas"].(int64); ok {
						replicas = r
					}
				}
				if status, ok := rs.Object["status"].(map[string]interface{}); ok {
					if r, ok := status["readyReplicas"].(int64); ok {
						readyReplicas = r
					}
				}
				rsStatus[rsKey] = fmt.Sprintf("%d/%d", readyReplicas, replicas)
			}
		}
	}

	// Build trees
	var trees []RuntimeTree
	for _, deploy := range deploys.Items {
		ns := deploy.GetNamespace()
		if !treeAll && isSystemNamespace(ns) {
			continue
		}

		name := deploy.GetName()
		owner, _ := detectOwnership(&deploy)

		// Get deployment status
		status := "Unknown"
		if statusMap, ok := deploy.Object["status"].(map[string]interface{}); ok {
			ready := int64(0)
			replicas := int64(0)
			if r, ok := statusMap["readyReplicas"].(int64); ok {
				ready = r
			}
			if r, ok := statusMap["replicas"].(int64); ok {
				replicas = r
			}
			status = fmt.Sprintf("%d/%d ready", ready, replicas)
		}

		tree := RuntimeTree{
			Name:      name,
			Namespace: ns,
			Kind:      "Deployment",
			Owner:     owner,
			Status:    status,
		}

		// Add ReplicaSets
		key := ns + "/" + name
		for _, rsKey := range deployToRS[key] {
			rsName := strings.TrimPrefix(rsKey, ns+"/")
			rsNode := ReplicaSetNode{
				Name:   rsName,
				Status: rsStatus[rsKey],
				Pods:   rsToPods[rsKey],
			}
			tree.ReplicaSets = append(tree.ReplicaSets, rsNode)
		}

		trees = append(trees, tree)
	}

	// Sort by namespace then name
	sort.Slice(trees, func(i, j int) bool {
		if trees[i].Namespace != trees[j].Namespace {
			return trees[i].Namespace < trees[j].Namespace
		}
		return trees[i].Name < trees[j].Name
	})

	if treeJSON {
		return json.NewEncoder(os.Stdout).Encode(trees)
	}

	// Print tree
	fmt.Printf("%sRuntime Hierarchy%s (%d Deployments)\n", colorBold, colorReset, len(trees))
	fmt.Println(strings.Repeat("─", 60))

	for _, tree := range trees {
		ownerColor := getOwnerColor(tree.Owner)
		fmt.Printf("├── %s%s%s/%s [%s%s%s] %s\n",
			colorBold, tree.Namespace, colorReset,
			tree.Name,
			ownerColor, tree.Owner, colorReset,
			tree.Status)

		for i, rs := range tree.ReplicaSets {
			rsPrefix := "│   ├──"
			podPrefix := "│   │   "
			if i == len(tree.ReplicaSets)-1 {
				rsPrefix = "│   └──"
				podPrefix = "│       "
			}

			fmt.Printf("%s ReplicaSet %s [%s]\n", rsPrefix, rs.Name, rs.Status)

			for j, pod := range rs.Pods {
				podConnector := "├──"
				if j == len(rs.Pods)-1 {
					podConnector = "└──"
				}
				statusIcon := getStatusIcon(pod.Status)
				fmt.Printf("%s%s Pod %s %s\n", podPrefix, podConnector, pod.Name, statusIcon)
			}
		}
	}

	return nil
}

// loadAndRenderTreeFromJSON loads tree data from a JSON file and renders it.
// This is a test hook to enable golden tests without cluster access.
func loadAndRenderTreeFromJSON(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read tree JSON: %w", err)
	}

	var trees []RuntimeTree
	if err := json.Unmarshal(data, &trees); err != nil {
		return fmt.Errorf("failed to parse tree JSON: %w", err)
	}

	// Sort by namespace then name for determinism
	sort.Slice(trees, func(i, j int) bool {
		if trees[i].Namespace != trees[j].Namespace {
			return trees[i].Namespace < trees[j].Namespace
		}
		return trees[i].Name < trees[j].Name
	})

	// Resolve effective format
	effectiveFormat := treeFormat
	if treeJSON && effectiveFormat == "ascii" {
		effectiveFormat = "json"
	}

	switch effectiveFormat {
	case "json":
		return json.NewEncoder(os.Stdout).Encode(trees)

	case "md":
		// Markdown output: thin wrapper over ASCII (no colors)
		fmt.Println("## Runtime Hierarchy")
		fmt.Println()
		fmt.Println("```")
		fmt.Printf("Runtime Hierarchy (%d Deployments)\n", len(trees))
		fmt.Println(strings.Repeat("─", 60))

		for _, tree := range trees {
			fmt.Printf("├── %s/%s [%s] %s\n",
				tree.Namespace, tree.Name, tree.Owner, tree.Status)

			for i, rs := range tree.ReplicaSets {
				rsPrefix := "│   ├──"
				podPrefix := "│   │   "
				if i == len(tree.ReplicaSets)-1 {
					rsPrefix = "│   └──"
					podPrefix = "│       "
				}

				fmt.Printf("%s ReplicaSet %s [%s]\n", rsPrefix, rs.Name, rs.Status)

				for j, pod := range rs.Pods {
					podConnector := "├──"
					if j == len(rs.Pods)-1 {
						podConnector = "└──"
					}
					statusIcon := getStatusIconNoColor(pod.Status)
					fmt.Printf("%s%s Pod %s %s\n", podPrefix, podConnector, pod.Name, statusIcon)
				}
			}
		}
		fmt.Println("```")
		return nil

	default: // "ascii"
		// Print tree with colors
		fmt.Printf("%sRuntime Hierarchy%s (%d Deployments)\n", colorBold, colorReset, len(trees))
		fmt.Println(strings.Repeat("─", 60))

		for _, tree := range trees {
			ownerColor := getOwnerColor(tree.Owner)
			fmt.Printf("├── %s%s%s/%s [%s%s%s] %s\n",
				colorBold, tree.Namespace, colorReset,
				tree.Name,
				ownerColor, tree.Owner, colorReset,
				tree.Status)

			for i, rs := range tree.ReplicaSets {
				rsPrefix := "│   ├──"
				podPrefix := "│   │   "
				if i == len(tree.ReplicaSets)-1 {
					rsPrefix = "│   └──"
					podPrefix = "│       "
				}

				fmt.Printf("%s ReplicaSet %s [%s]\n", rsPrefix, rs.Name, rs.Status)

				for j, pod := range rs.Pods {
					podConnector := "├──"
					if j == len(rs.Pods)-1 {
						podConnector = "└──"
					}
					statusIcon := getStatusIcon(pod.Status)
					fmt.Printf("%s%s Pod %s %s\n", podPrefix, podConnector, pod.Name, statusIcon)
				}
			}
		}
		return nil
	}
}

func runTreeOwnership(ctx context.Context) error {
	// Resolve effective format (--format takes precedence over deprecated --json)
	effectiveFormat := treeFormat
	if treeJSON && effectiveFormat == "ascii" {
		effectiveFormat = "json"
	}

	cfg, err := buildConfig()
	if err != nil {
		return fmt.Errorf("failed to build config: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	// Get current context name for cluster field
	clusterName := getCurrentContext()
	argoLineage := buildArgoLineageIndex(ctx, dynClient)

	// Convert to mapsvc.Entry for shared renderer
	var entries []mapsvc.Entry
	forEachCanonicalWorkload(ctx, dynClient, treeNamespace, func(workload *unstructured.Unstructured) {
		ns := workload.GetNamespace()
		if !treeAll && isSystemNamespace(ns) {
			return
		}

		owner, ownerName := detectOwnership(workload)
		ownerNamespace := ""
		lineage := ""
		if owner == "ArgoCD" {
			if appName, appNamespace, appLineage := resolveArgoOwnershipLineage(workload, argoLineage); appName != "" {
				ownerName = appName
				ownerNamespace = appNamespace
				lineage = appLineage
			}
		}

		// Build ownerDetails map from ownerName
		var ownerDetails map[string]string
		if ownerName != "" && ownerName != "-" {
			// Format: "kind/name" based on owner type
			kindPrefix := ownerKindPrefix(owner)
			ownerDetails = map[string]string{
				"name": kindPrefix + "/" + ownerName,
			}
			if ownerNamespace != "" {
				ownerDetails["namespace"] = ownerNamespace
			}
			if lineage != "" {
				ownerDetails["lineage"] = lineage
			}
		}

		entries = append(entries, mapsvc.Entry{
			Name:         workload.GetName(),
			Namespace:    ns,
			Kind:         workload.GetKind(),
			Owner:        owner,
			OwnerDetails: ownerDetails,
		})
	})

	// Build opts for shared renderer
	opts := mapsvc.OwnershipTreeOpts{
		ShowKind:     false,
		ShowOwnerRef: true,
		FilterOwner:  treeOwner,
		MaxDepth:     treeDepth,
	}

	// Output based on format
	switch effectiveFormat {
	case "json":
		// v0.14 JSON schema output
		var namespace *string
		if treeNamespace != "" {
			namespace = &treeNamespace
		}

		output := mapsvc.BuildOwnershipTreeJSON(entries, opts, clusterName, namespace)

		// Use MarshalIndent for deterministic, readable output
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)

	case "md":
		// Markdown output: thin wrapper over ASCII (no colors)
		fmt.Println("## Ownership Hierarchy")
		fmt.Println()
		fmt.Println("```")

		// Use shared renderer for canonical output structure
		lines := mapsvc.BuildOwnershipTreeLines(entries, opts)

		// Output without ANSI colors
		for _, line := range lines {
			if line.IsHeader {
				fmt.Printf("%s (%d)\n", line.Owner, line.Count)
			} else if line.Connector == "" {
				fmt.Println()
			} else if strings.HasPrefix(line.Name, "(+") {
				// Depth truncation indicator
				fmt.Printf("  %s %s\n", line.Connector, line.Name)
			} else {
				fmt.Printf("  %s %s/%s", line.Connector, line.Namespace, line.Name)
				if line.OwnerRef != "" {
					fmt.Printf(" -> %s", line.OwnerRef)
				}
				fmt.Println()
			}
		}

		fmt.Println("```")

	default: // "ascii"
		// Print header
		fmt.Printf("%sOwnership Hierarchy%s\n", colorBold, colorReset)
		fmt.Println(strings.Repeat("─", 60))

		// Use shared renderer for canonical output structure
		lines := mapsvc.BuildOwnershipTreeLines(entries, opts)

		// Apply CLI colors to each line
		for _, line := range lines {
			if line.IsHeader {
				ownerColor := getOwnerColor(line.Owner)
				fmt.Printf("%s%s%s (%d)\n", ownerColor, line.Owner, colorReset, line.Count)
			} else if line.Connector == "" {
				fmt.Println()
			} else if strings.HasPrefix(line.Name, "(+") {
				// Depth truncation indicator
				fmt.Printf("  %s %s%s%s\n", line.Connector, colorDim, line.Name, colorReset)
			} else {
				fmt.Printf("  %s %s/%s", line.Connector, line.Namespace, line.Name)
				if line.OwnerRef != "" {
					fmt.Printf(" %s-> %s%s", colorDim, line.OwnerRef, colorReset)
				}
				fmt.Println()
			}
		}
	}

	return nil
}

// ownerKindPrefix returns the deployer kind prefix for owner type.
func ownerKindPrefix(owner string) string {
	switch owner {
	case "Flux":
		return "kustomization" // Could also be helmrelease
	case "ArgoCD":
		return "application"
	case "Helm":
		return "release"
	case "ConfigHub":
		return "unit"
	default:
		return ""
	}
}

func runTreeWorkloads() error {
	// This is an alias for 'map workloads'
	fmt.Println("Tip: 'cub-scout tree workloads' is an alias for 'cub-scout map workloads'")
	fmt.Println()

	// Run map workloads
	mapCmd.SetArgs([]string{"workloads"})
	return mapCmd.Execute()
}

func runTreeGit(ctx context.Context) error {
	jsonOutput := treeJSON || strings.EqualFold(treeFormat, "json")
	cfg, err := buildConfig()
	if err != nil {
		return fmt.Errorf("failed to build config: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	// Try to get GitRepositories (Flux)
	gitRepoGVR := schema.GroupVersionResource{
		Group:    "source.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "gitrepositories",
	}

	gitRepos, err := dynClient.Resource(gitRepoGVR).Namespace("").List(ctx, v1.ListOptions{})
	warnings := []string{}
	if err != nil {
		warnings = append(warnings, "Could not list GitRepositories (Flux may not be installed)")
		gitRepos = nil
	}

	// Try to get ArgoCD Applications
	argoAppGVR := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}

	argoApps, err := dynClient.Resource(argoAppGVR).Namespace("").List(ctx, v1.ListOptions{})
	if err != nil {
		warnings = append(warnings, "Could not list ArgoCD Applications (ArgoCD may not be installed)")
		argoApps = nil
	}

	argoAppSetGVR := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applicationsets",
	}
	argoAppSets, err := dynClient.Resource(argoAppSetGVR).Namespace("").List(ctx, v1.ListOptions{})
	if err != nil {
		warnings = append(warnings, "Could not list ArgoCD ApplicationSets (ApplicationSet CRD may not be installed)")
		argoAppSets = nil
	}

	result := buildTreeGitJSON(gitRepos, argoApps, argoAppSets)
	if jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(result)
	}

	fmt.Printf("%sGit Source Hierarchy%s\n", colorBold, colorReset)
	fmt.Println(strings.Repeat("─", 60))
	for _, warning := range warnings {
		fmt.Printf("%sNote:%s %s\n", colorYellow, colorReset, warning)
	}

	// Print Flux GitRepositories
	if len(result.GitRepositories) > 0 {
		fmt.Printf("\n%sFlux GitRepositories%s (%d)\n", colorCyan, colorReset, len(result.GitRepositories))
		for i, r := range result.GitRepositories {
			connector := "├──"
			if i == len(result.GitRepositories)-1 {
				connector = "└──"
			}
			fmt.Printf("  %s %s/%s\n", connector, r.Namespace, r.Name)
			fmt.Printf("      %s-> %s%s\n", colorDim, r.URL, colorReset)
		}
	}

	// Print ArgoCD Applications
	if len(result.ArgoApplications) > 0 {
		fmt.Printf("\n%sArgoCD Applications%s (%d)\n", colorPurple, colorReset, len(result.ArgoApplications))

		byRepo := make(map[string][]treeGitAppJSON)
		repoKeys := make(map[string]struct{})
		for _, app := range result.ArgoApplications {
			key := app.RepoURL
			if key == "" {
				key = "(unknown repo)"
			}
			repoKeys[key] = struct{}{}
			byRepo[key] = append(byRepo[key], app)
		}

		repos := make([]string, 0, len(repoKeys))
		for key := range repoKeys {
			repos = append(repos, key)
		}
		sort.Strings(repos)

		for repoIdx, repoURL := range repos {
			repoConnector := "├──"
			if repoIdx == len(repos)-1 {
				repoConnector = "└──"
			}
			fmt.Printf("  %s %s\n", repoConnector, repoURL)

			apps := byRepo[repoURL]
			for i, app := range apps {
				appConnector := "│   ├──"
				if i == len(apps)-1 {
					appConnector = "│   └──"
				}
				if repoIdx == len(repos)-1 {
					appConnector = strings.Replace(appConnector, "│", " ", 1)
				}
				path := app.Path
				if path == "" {
					path = "."
				}
				fmt.Printf("  %s %s (%s)", appConnector, app.Name, path)

				notes := []string{}
				if app.GeneratedByApplicationSet != "" {
					note := "generated by applicationset/" + app.GeneratedByApplicationSet
					switch app.ApplicationSetLinkStatus {
					case "orphan":
						note = "orphan applicationset/" + app.GeneratedByApplicationSet + " (missing from cluster)"
					case "unknown":
						note = "unresolved applicationset/" + app.GeneratedByApplicationSet + " (inferred link only)"
					}
					if app.LineageConfidence != "" && app.ApplicationSetLinkStatus != "unknown" {
						note += " (" + app.LineageConfidence + ")"
					}
					notes = append(notes, note)
				}
				if app.ParentApplication != "" {
					note := "child of application/" + app.ParentApplication
					if app.LineageConfidence != "" {
						note += " (" + app.LineageConfidence + ")"
					}
					notes = append(notes, note)
				}
				if len(app.ChildApplications) > 0 {
					notes = append(notes, fmt.Sprintf("parent of %d apps", len(app.ChildApplications)))
				}
				if len(notes) > 0 {
					fmt.Printf(" %s[%s]%s", colorDim, strings.Join(notes, "; "), colorReset)
				}
				fmt.Println()
			}
		}
	}

	if len(result.ApplicationSets) > 0 {
		fmt.Printf("\n%sArgoCD ApplicationSets%s (%d)\n", colorPurple, colorReset, len(result.ApplicationSets))
		for i, appSet := range result.ApplicationSets {
			connector := "├──"
			if i == len(result.ApplicationSets)-1 {
				connector = "└──"
			}

			path := appSet.Path
			if path == "" {
				path = "."
			}

			fmt.Printf("  %s %s/%s (%s)\n", connector, appSet.Namespace, appSet.Name, path)
			if len(appSet.GeneratorTypes) > 0 {
				fmt.Printf("      generators: %s\n", strings.Join(appSet.GeneratorTypes, ","))
			}
			if appSet.RepoURL != "" {
				fmt.Printf("      %s-> %s%s\n", colorDim, appSet.RepoURL, colorReset)
			}
			if len(appSet.GeneratedApplications) == 0 {
				fmt.Println("      generates: (none detected)")
				continue
			}

			for _, appName := range appSet.GeneratedApplications {
				fmt.Printf("      -> %s\n", appName)
			}
		}
	}

	if len(result.GitRepositories) == 0 && len(result.ArgoApplications) == 0 && len(result.ApplicationSets) == 0 {
		fmt.Printf("\n%sNo Git sources found.%s\n", colorDim, colorReset)
		fmt.Println("Install Flux or ArgoCD to see Git source hierarchy.")
	}

	return nil
}

func buildTreeGitJSON(gitRepos, argoApps, argoAppSets *unstructured.UnstructuredList) treeGitJSON {
	result := treeGitJSON{
		GitRepositories:  make([]treeGitRepoJSON, 0),
		ArgoApplications: make([]treeGitAppJSON, 0),
		ApplicationSets:  make([]treeGitApplicationSetJSON, 0),
	}

	if gitRepos != nil {
		for _, r := range gitRepos.Items {
			spec, _ := r.Object["spec"].(map[string]interface{})
			url, _ := spec["url"].(string)
			result.GitRepositories = append(result.GitRepositories, treeGitRepoJSON{
				Name:      r.GetName(),
				Namespace: r.GetNamespace(),
				URL:       url,
			})
		}
		sort.Slice(result.GitRepositories, func(i, j int) bool {
			left := result.GitRepositories[i]
			right := result.GitRepositories[j]
			if left.Namespace != right.Namespace {
				return left.Namespace < right.Namespace
			}
			return left.Name < right.Name
		})
	}

	appSetByNamespacedName := map[string]int{}
	appSetByName := map[string]int{}
	if argoAppSets != nil {
		for _, appSet := range argoAppSets.Items {
			spec, _ := appSet.Object["spec"].(map[string]interface{})
			repoURL, path := parseAppSetTemplateSource(spec)
			result.ApplicationSets = append(result.ApplicationSets, treeGitApplicationSetJSON{
				Name:           appSet.GetName(),
				Namespace:      appSet.GetNamespace(),
				RepoURL:        repoURL,
				Path:           path,
				GeneratorTypes: detectApplicationSetGeneratorTypes(spec),
			})
		}
		sort.Slice(result.ApplicationSets, func(i, j int) bool {
			left := result.ApplicationSets[i]
			right := result.ApplicationSets[j]
			if left.Namespace != right.Namespace {
				return left.Namespace < right.Namespace
			}
			return left.Name < right.Name
		})
		for idx, appSet := range result.ApplicationSets {
			key := appSet.Namespace + "/" + appSet.Name
			appSetByNamespacedName[key] = idx
			if _, exists := appSetByName[appSet.Name]; !exists {
				appSetByName[appSet.Name] = idx
			}
		}
	}

	if argoApps != nil {
		for _, app := range argoApps.Items {
			spec, _ := app.Object["spec"].(map[string]interface{})
			source, _ := spec["source"].(map[string]interface{})
			repoURL, _ := source["repoURL"].(string)
			path, _ := source["path"].(string)

			generatedByAppSet, appSetConf := resolveGeneratedByApplicationSetWithConfidence(&app)
			parentApp, parentConf := resolveParentApplicationWithConfidence(&app)
			appSetStatus, appSetIndex, appSetResolved := classifyApplicationSetLink(
				app.GetNamespace(),
				generatedByAppSet,
				appSetConf,
				appSetByNamespacedName,
				appSetByName,
			)

			confidence := parentConf
			if confidence == "" {
				confidence = appSetConf
			} else if appSetConf == "explicit" {
				confidence = "explicit"
			}

			result.ArgoApplications = append(result.ArgoApplications, treeGitAppJSON{
				Name:                      app.GetName(),
				Namespace:                 app.GetNamespace(),
				RepoURL:                   repoURL,
				Path:                      path,
				GeneratedByApplicationSet: generatedByAppSet,
				ApplicationSetLinkStatus:  appSetStatus,
				ParentApplication:         parentApp,
				LineageConfidence:         confidence,
			})

			if generatedByAppSet == "" || len(result.ApplicationSets) == 0 || !appSetResolved {
				continue
			}
			result.ApplicationSets[appSetIndex].GeneratedApplications = append(result.ApplicationSets[appSetIndex].GeneratedApplications, app.GetName())
		}
		sort.Slice(result.ArgoApplications, func(i, j int) bool {
			left := result.ArgoApplications[i]
			right := result.ArgoApplications[j]
			if left.Namespace != right.Namespace {
				return left.Namespace < right.Namespace
			}
			return left.Name < right.Name
		})
	}

	for idx := range result.ApplicationSets {
		sort.Strings(result.ApplicationSets[idx].GeneratedApplications)
	}

	// Build parent → children index for App-of-Apps (#194)
	parentIndex := map[string]int{} // name → index in ArgoApplications
	for i, app := range result.ArgoApplications {
		parentIndex[app.Name] = i
	}
	for _, app := range result.ArgoApplications {
		if app.ParentApplication == "" {
			continue
		}
		if idx, ok := parentIndex[app.ParentApplication]; ok {
			result.ArgoApplications[idx].ChildApplications = append(
				result.ArgoApplications[idx].ChildApplications, app.Name,
			)
		}
	}
	for idx := range result.ArgoApplications {
		sort.Strings(result.ArgoApplications[idx].ChildApplications)
	}

	return result
}

func parseAppSetTemplateSource(spec map[string]interface{}) (repoURL, path string) {
	template, _ := spec["template"].(map[string]interface{})
	templateSpec, _ := template["spec"].(map[string]interface{})
	source, _ := templateSpec["source"].(map[string]interface{})
	repoURL, _ = source["repoURL"].(string)
	path, _ = source["path"].(string)
	return repoURL, path
}

func detectApplicationSetGeneratorTypes(spec map[string]interface{}) []string {
	generators, _ := spec["generators"].([]interface{})
	if len(generators) == 0 {
		return nil
	}

	known := []string{"list", "clusters", "git", "matrix", "merge", "pullRequest", "scmProvider", "clusterDecisionResource", "plugin"}
	seen := map[string]struct{}{}
	for _, g := range generators {
		generatorMap, ok := g.(map[string]interface{})
		if !ok {
			continue
		}
		for _, key := range known {
			if _, ok := generatorMap[key]; ok {
				seen[key] = struct{}{}
			}
		}
	}

	if len(seen) == 0 {
		return []string{"unknown"}
	}

	out := make([]string, 0, len(seen))
	for key := range seen {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func resolveGeneratedByApplicationSet(app *unstructured.Unstructured) string {
	for _, ownerRef := range app.GetOwnerReferences() {
		if strings.EqualFold(ownerRef.Kind, "ApplicationSet") {
			name := strings.TrimSpace(ownerRef.Name)
			if name != "" {
				return name
			}
		}
	}

	labels := app.GetLabels()
	if labels != nil {
		if name := strings.TrimSpace(labels["argocd.argoproj.io/application-set-name"]); name != "" {
			return name
		}
	}

	annotations := app.GetAnnotations()
	if annotations != nil {
		if name := strings.TrimSpace(annotations["argocd.argoproj.io/application-set-name"]); name != "" {
			return name
		}
		if name := strings.TrimSpace(annotations["cub-scout.io/generated-by-applicationset"]); name != "" {
			return name
		}
	}

	return ""
}

// resolveGeneratedByApplicationSetWithConfidence returns (name, confidence)
// where confidence is "explicit" for ownerReference, "inferred" for label/annotation.
func resolveGeneratedByApplicationSetWithConfidence(app *unstructured.Unstructured) (string, string) {
	for _, ownerRef := range app.GetOwnerReferences() {
		if strings.EqualFold(ownerRef.Kind, "ApplicationSet") {
			name := strings.TrimSpace(ownerRef.Name)
			if name != "" {
				return name, "explicit"
			}
		}
	}

	labels := app.GetLabels()
	if labels != nil {
		if name := strings.TrimSpace(labels["argocd.argoproj.io/application-set-name"]); name != "" {
			return name, "inferred"
		}
	}

	annotations := app.GetAnnotations()
	if annotations != nil {
		if name := strings.TrimSpace(annotations["argocd.argoproj.io/application-set-name"]); name != "" {
			return name, "inferred"
		}
		if name := strings.TrimSpace(annotations["cub-scout.io/generated-by-applicationset"]); name != "" {
			return name, "inferred"
		}
	}

	return "", ""
}

func classifyApplicationSetLink(
	appNamespace, generatedByAppSet, appSetConfidence string,
	appSetByNamespacedName, appSetByName map[string]int,
) (status string, appSetIndex int, resolved bool) {
	generatedByAppSet = strings.TrimSpace(generatedByAppSet)
	if generatedByAppSet == "" {
		return "", -1, false
	}

	nsKey := appNamespace + "/" + generatedByAppSet
	if idx, ok := appSetByNamespacedName[nsKey]; ok {
		return "resolved", idx, true
	}
	if idx, ok := appSetByName[generatedByAppSet]; ok {
		return "resolved", idx, true
	}

	if appSetConfidence == "explicit" {
		return "orphan", -1, false
	}
	return "unknown", -1, false
}

// resolveParentApplicationWithConfidence returns (name, confidence)
// where confidence is "explicit" for ownerReference, "inferred" for label/annotation.
func resolveParentApplicationWithConfidence(app *unstructured.Unstructured) (string, string) {
	self := strings.TrimSpace(app.GetName())

	for _, ownerRef := range app.GetOwnerReferences() {
		if strings.EqualFold(ownerRef.Kind, "Application") {
			name := strings.TrimSpace(ownerRef.Name)
			if name != "" && name != self {
				return name, "explicit"
			}
		}
	}

	annotations := app.GetAnnotations()
	if annotations != nil {
		if name := strings.TrimSpace(annotations["cub-scout.io/parent-application"]); name != "" && name != self {
			return name, "inferred"
		}
		if name := strings.TrimSpace(annotations["argocd.argoproj.io/managed-by"]); name != "" && name != self {
			return name, "inferred"
		}
	}

	labels := app.GetLabels()
	if labels != nil {
		if name := strings.TrimSpace(labels["app.kubernetes.io/part-of"]); name != "" && name != self {
			return name, "inferred"
		}
	}

	return "", ""
}

func runTreePatterns() error {
	// Run the patterns command
	return runMapPatterns(mapPatternsCmd, []string{})
}

func runTreeConfig() error {
	// Check if cub CLI is available
	_, err := exec.LookPath("cub")
	if err != nil {
		return fmt.Errorf("'cub' CLI not found. Install with: brew install confighub/tap/cub")
	}

	// Build command args
	args := []string{"unit", "tree"}

	if treeSpace != "" {
		args = append(args, "--space", treeSpace)
	}

	if treeEdge != "" {
		args = append(args, "--edge", treeEdge)
	}

	if treeJSON {
		args = append(args, "--json")
	}

	fmt.Printf("%sConfigHub Unit Tree%s (via 'cub unit tree')\n", colorBold, colorReset)
	fmt.Println(strings.Repeat("─", 60))

	if treeEdge == "clone" {
		fmt.Printf("Showing configuration inheritance (clone relationships)\n")
	} else if treeEdge == "link" {
		fmt.Printf("Showing dependencies (link relationships)\n")
	}
	fmt.Println()

	// Execute cub unit tree
	cmd := exec.Command("cub", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func getOwnerColor(owner string) string {
	switch owner {
	case "Flux":
		return colorCyan
	case "ArgoCD":
		return colorPurple
	case "Helm":
		return colorYellow
	case "ConfigHub":
		return colorGreen
	default:
		return colorDim
	}
}

func getStatusIcon(status string) string {
	switch status {
	case "Running":
		return colorGreen + "✓ Running" + colorReset
	case "Pending":
		return colorYellow + "⏳ Pending" + colorReset
	case "Failed":
		return colorRed + "✗ Failed" + colorReset
	case "Succeeded":
		return colorGreen + "✓ Succeeded" + colorReset
	default:
		return colorDim + "? " + status + colorReset
	}
}

func getStatusIconNoColor(status string) string {
	switch status {
	case "Running":
		return "✓ Running"
	case "Pending":
		return "⏳ Pending"
	case "Failed":
		return "✗ Failed"
	case "Succeeded":
		return "✓ Succeeded"
	default:
		return "? " + status
	}
}

func runTreeSuggest(ctx context.Context) error {

	cfg, err := buildConfig()
	if err != nil {
		return fmt.Errorf("failed to build config: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	// Get Deployments
	deployGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	deploys, err := dynClient.Resource(deployGVR).Namespace(treeNamespace).List(ctx, v1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list deployments: %w", err)
	}

	// Build workload info list
	var workloads []WorkloadInfo
	for _, deploy := range deploys.Items {
		ns := deploy.GetNamespace()
		if !treeAll && isSystemNamespace(ns) {
			continue
		}

		owner, _ := detectOwnership(&deploy)
		labels := deploy.GetLabels()
		annotations := deploy.GetAnnotations()

		// Extract Kustomization or Application path for variant detection
		var kustomizationPath, applicationPath string
		if path, ok := annotations["kustomize.toolkit.fluxcd.io/path"]; ok {
			kustomizationPath = path
		}
		if path, ok := annotations["argocd.argoproj.io/source-path"]; ok {
			applicationPath = path
		}

		workloads = append(workloads, WorkloadInfo{
			Name:              deploy.GetName(),
			Namespace:         ns,
			Kind:              "Deployment",
			Owner:             owner,
			Labels:            labels,
			Annotations:       annotations,
			KustomizationPath: kustomizationPath,
			ApplicationPath:   applicationPath,
		})
	}

	if treeJSON {
		suggestion := SuggestHubAppSpaceStructure(workloads, treeSpace)
		return json.NewEncoder(os.Stdout).Encode(suggestion)
	}

	// Print suggestion
	fmt.Printf("%sApp Model Suggestion%s\n", colorBold, colorReset)
	fmt.Println(strings.Repeat("─", 60))
	fmt.Println()
	fmt.Println("Based on cluster workloads, here's a suggested ConfigHub structure:")
	fmt.Println()

	suggestion := SuggestHubAppSpaceStructure(workloads, treeSpace)
	suggestion.Print()

	fmt.Println()
	fmt.Println(strings.Repeat("─", 60))
	fmt.Printf("\n%sNext steps:%s\n", colorBold, colorReset)
	fmt.Println("  1. Review the suggested structure above")
	fmt.Println("  2. Import workloads: cub-scout import -n <namespace>")
	fmt.Println("  3. View in ConfigHub: cub unit tree --space <space>")
	fmt.Println()
	fmt.Println("For fleet-wide queries after import:")
	fmt.Println("  cub unit list --space \"*\"                    # All units")
	fmt.Println("  cub unit tree --space \"*\" --edge clone       # Inheritance")
	fmt.Println("  cub unit tree --space \"*\" --edge link        # Dependencies")

	return nil
}

// isSystemNamespace is defined in import.go
