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

	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var (
	treeJSON      bool
	treeNamespace string
	treeAll       bool
	treeSpace     string // For ConfigHub tree
	treeEdge      string // For ConfigHub tree (clone/link)
)

var treeCmd = &cobra.Command{
	Use:   "tree [type]",
	Short: "Show hierarchical views of resources",
	Long: `Show hierarchical views of cluster resources, Git repos, or ConfigHub units.

cub-scout tree provides different perspectives on your infrastructure:

  CLUSTER VIEWS (what the scout sees):
    runtime     Deployment → ReplicaSet → Pod trees (default)
    ownership   Resources grouped by GitOps owner (Flux, ArgoCD, Helm)
    workloads   Same as 'cub-scout map workloads' (alias)

  GIT VIEWS:
    git         Git repository structure from detected sources
    patterns    Detected GitOps patterns (Arnie, Banko, Fluxy, D2)

  CONFIGHUB VIEWS (wraps 'cub unit tree'):
    config      ConfigHub Unit inheritance (--edge clone) or dependencies (--edge link)
    suggest     Suggested Hub/AppSpace organization based on cluster workloads

Examples:
  # Show runtime hierarchy (Deployment → ReplicaSet → Pod)
  cub-scout tree
  cub-scout tree runtime

  # Show resources by GitOps owner
  cub-scout tree ownership

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
	ValidArgs: []string{"runtime", "ownership", "workloads", "git", "patterns", "config", "suggest"},
	RunE:      runTree,
}

func init() {
	rootCmd.AddCommand(treeCmd)

	treeCmd.Flags().BoolVar(&treeJSON, "json", false, "Output as JSON")
	treeCmd.Flags().StringVarP(&treeNamespace, "namespace", "n", "", "Filter by namespace")
	treeCmd.Flags().BoolVarP(&treeAll, "all", "A", false, "Show all resources including system namespaces")
	treeCmd.Flags().StringVar(&treeSpace, "space", "", "ConfigHub space for 'config' view (use '*' for all spaces)")
	treeCmd.Flags().StringVar(&treeEdge, "edge", "clone", "Edge type for 'config' view: clone (inheritance) or link (dependencies)")
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
		return fmt.Errorf("unknown tree type: %s (valid: runtime, ownership, workloads, git, patterns, config, suggest)", viewType)
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

func runTreeRuntime(ctx context.Context) error {

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

func runTreeOwnership(ctx context.Context) error {

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

	// Group by owner
	byOwner := make(map[string][]RuntimeTree)
	for _, deploy := range deploys.Items {
		ns := deploy.GetNamespace()
		if !treeAll && isSystemNamespace(ns) {
			continue
		}

		owner, _ := detectOwnership(&deploy)

		tree := RuntimeTree{
			Name:      deploy.GetName(),
			Namespace: ns,
			Kind:      "Deployment",
			Owner:     owner,
		}
		byOwner[owner] = append(byOwner[owner], tree)
	}

	if treeJSON {
		return json.NewEncoder(os.Stdout).Encode(byOwner)
	}

	// Print by owner
	fmt.Printf("%sOwnership Hierarchy%s\n", colorBold, colorReset)
	fmt.Println(strings.Repeat("─", 60))

	// Order: Flux, ArgoCD, Helm, ConfigHub, Native
	owners := []string{"Flux", "ArgoCD", "Helm", "ConfigHub", "Native"}
	for _, owner := range owners {
		resources := byOwner[owner]
		if len(resources) == 0 {
			continue
		}

		ownerColor := getOwnerColor(owner)
		fmt.Printf("%s%s%s (%d)\n", ownerColor, owner, colorReset, len(resources))

		// Sort by namespace then name
		sort.Slice(resources, func(i, j int) bool {
			if resources[i].Namespace != resources[j].Namespace {
				return resources[i].Namespace < resources[j].Namespace
			}
			return resources[i].Name < resources[j].Name
		})

		for i, r := range resources {
			connector := "├──"
			if i == len(resources)-1 {
				connector = "└──"
			}
			fmt.Printf("  %s %s/%s\n", connector, r.Namespace, r.Name)
		}
		fmt.Println()
	}

	return nil
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
	if err != nil {
		fmt.Printf("%sNote:%s Could not list GitRepositories (Flux may not be installed)\n", colorYellow, colorReset)
	}

	// Try to get ArgoCD Applications
	argoAppGVR := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}

	argoApps, err := dynClient.Resource(argoAppGVR).Namespace("").List(ctx, v1.ListOptions{})
	if err != nil {
		fmt.Printf("%sNote:%s Could not list ArgoCD Applications (ArgoCD may not be installed)\n", colorYellow, colorReset)
	}

	if treeJSON {
		result := map[string]interface{}{
			"gitRepositories":  []interface{}{},
			"argoApplications": []interface{}{},
		}
		if gitRepos != nil {
			repos := []map[string]string{}
			for _, r := range gitRepos.Items {
				spec, _ := r.Object["spec"].(map[string]interface{})
				url, _ := spec["url"].(string)
				repos = append(repos, map[string]string{
					"name":      r.GetName(),
					"namespace": r.GetNamespace(),
					"url":       url,
				})
			}
			result["gitRepositories"] = repos
		}
		if argoApps != nil {
			apps := []map[string]string{}
			for _, a := range argoApps.Items {
				spec, _ := a.Object["spec"].(map[string]interface{})
				source, _ := spec["source"].(map[string]interface{})
				repoURL, _ := source["repoURL"].(string)
				path, _ := source["path"].(string)
				apps = append(apps, map[string]string{
					"name":      a.GetName(),
					"namespace": a.GetNamespace(),
					"repoURL":   repoURL,
					"path":      path,
				})
			}
			result["argoApplications"] = apps
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	}

	fmt.Printf("%sGit Source Hierarchy%s\n", colorBold, colorReset)
	fmt.Println(strings.Repeat("─", 60))

	// Print Flux GitRepositories
	if gitRepos != nil && len(gitRepos.Items) > 0 {
		fmt.Printf("\n%sFlux GitRepositories%s (%d)\n", colorCyan, colorReset, len(gitRepos.Items))
		for i, r := range gitRepos.Items {
			connector := "├──"
			if i == len(gitRepos.Items)-1 {
				connector = "└──"
			}
			spec, _ := r.Object["spec"].(map[string]interface{})
			url, _ := spec["url"].(string)
			ref, _ := spec["ref"].(map[string]interface{})
			branch, _ := ref["branch"].(string)
			if branch == "" {
				branch = "main"
			}
			fmt.Printf("  %s %s/%s\n", connector, r.GetNamespace(), r.GetName())
			fmt.Printf("      %s → %s\n", colorDim, url)
			fmt.Printf("      branch: %s%s\n", branch, colorReset)
		}
	}

	// Print ArgoCD Applications
	if argoApps != nil && len(argoApps.Items) > 0 {
		fmt.Printf("\n%sArgoCD Applications%s (%d)\n", colorPurple, colorReset, len(argoApps.Items))

		// Group by repo URL
		byRepo := make(map[string][]map[string]string)
		for _, a := range argoApps.Items {
			spec, _ := a.Object["spec"].(map[string]interface{})
			source, _ := spec["source"].(map[string]interface{})
			repoURL, _ := source["repoURL"].(string)
			path, _ := source["path"].(string)

			byRepo[repoURL] = append(byRepo[repoURL], map[string]string{
				"name":      a.GetName(),
				"namespace": a.GetNamespace(),
				"path":      path,
			})
		}

		repoIdx := 0
		for repoURL, apps := range byRepo {
			repoConnector := "├──"
			if repoIdx == len(byRepo)-1 {
				repoConnector = "└──"
			}
			fmt.Printf("  %s %s\n", repoConnector, repoURL)

			for i, app := range apps {
				appConnector := "│   ├──"
				if i == len(apps)-1 {
					appConnector = "│   └──"
				}
				if repoIdx == len(byRepo)-1 {
					appConnector = strings.Replace(appConnector, "│", " ", 1)
				}
				path := app["path"]
				if path == "" {
					path = "."
				}
				fmt.Printf("  %s %s (%s)\n", appConnector, app["name"], path)
			}
			repoIdx++
		}
	}

	if (gitRepos == nil || len(gitRepos.Items) == 0) && (argoApps == nil || len(argoApps.Items) == 0) {
		fmt.Printf("\n%sNo Git sources found.%s\n", colorDim, colorReset)
		fmt.Println("Install Flux or ArgoCD to see Git source hierarchy.")
	}

	return nil
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
	fmt.Printf("%sHub/AppSpace Suggestion%s\n", colorBold, colorReset)
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
