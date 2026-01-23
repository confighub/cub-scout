// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Types, styles, and constants are in hierarchy_types.go

// extractClusterName extracts a canonical cluster name from various context formats
func extractClusterName(contextName string) string {
	// Handle empty/unknown
	if contextName == "" || contextName == "unknown" {
		return contextName
	}

	// AWS EKS: arn:aws:eks:region:account:cluster/name
	if strings.HasPrefix(contextName, "arn:aws:eks:") {
		if idx := strings.LastIndex(contextName, "/"); idx != -1 {
			return contextName[idx+1:]
		}
	}

	// GKE: gke_project_zone_cluster
	if strings.HasPrefix(contextName, "gke_") {
		parts := strings.Split(contextName, "_")
		if len(parts) >= 4 {
			return parts[len(parts)-1]
		}
	}

	// kind: kind-name
	if strings.HasPrefix(contextName, "kind-") {
		return strings.TrimPrefix(contextName, "kind-")
	}

	// Default: use the context name as-is
	return contextName
}

// matchesCluster checks if a target cluster matches the current cluster
func matchesCluster(targetCluster, currentCluster string) bool {
	if targetCluster == "" || currentCluster == "" {
		return false
	}

	// Exact match
	if targetCluster == currentCluster {
		return true
	}

	// Partial match (for different naming conventions)
	if strings.Contains(strings.ToLower(targetCluster), strings.ToLower(currentCluster)) {
		return true
	}
	if strings.Contains(strings.ToLower(currentCluster), strings.ToLower(targetCluster)) {
		return true
	}

	return false
}

// Commands
func loadDataCmd() tea.Msg {
	nodes, currentOrg, currentOrgInt, currentSpace, spacesToLoad, err := loadConfigHubData()
	if err != nil {
		return errMsg{err: err}
	}
	return dataLoadedMsg{nodes: nodes, currentOrg: currentOrg, currentOrgInt: currentOrgInt, currentSpace: currentSpace, spacesToLoad: spacesToLoad}
}

// loadSpaceDataCmd loads units, targets, and workers for a space in the background
func loadSpaceDataCmd(spaceSlug string) tea.Cmd {
	return func() tea.Msg {
		units, _ := loadUnitsForSpace(spaceSlug)
		targets, _ := loadTargetsForSpace(spaceSlug)
		workers, _ := loadWorkersForSpace(spaceSlug)
		return spaceDataLoadedMsg{
			spaceSlug: spaceSlug,
			units:     units,
			targets:   targets,
			workers:   workers,
		}
	}
}

// loadPanelDataCmd fetches cluster workloads for the WET↔LIVE panel view
func loadPanelDataCmd(unitSlugs []string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// Build kubernetes config
		cfg, err := buildConfig()
		if err != nil {
			return panelDataLoadedMsg{err: fmt.Errorf("build kubernetes config: %w", err)}
		}

		dynClient, err := dynamic.NewForConfig(cfg)
		if err != nil {
			return panelDataLoadedMsg{err: fmt.Errorf("create dynamic client: %w", err)}
		}

		clusterName := os.Getenv("CLUSTER_NAME")
		if clusterName == "" {
			clusterName = "default"
		}

		var workloads []MapEntry
		correlation := make(map[string][]MapEntry)
		var orphans []MapEntry

		// Build unit slug lookup set for correlation
		unitSet := make(map[string]bool)
		for _, slug := range unitSlugs {
			unitSet[slug] = true
		}

		// Workload resources to fetch
		workloadGVRs := []schema.GroupVersionResource{
			{Group: "apps", Version: "v1", Resource: "deployments"},
			{Group: "apps", Version: "v1", Resource: "statefulsets"},
			{Group: "apps", Version: "v1", Resource: "daemonsets"},
		}

		for _, gvr := range workloadGVRs {
			list, err := dynClient.Resource(gvr).List(ctx, metav1.ListOptions{})
			if err != nil {
				continue
			}
			for i := range list.Items {
				item := &list.Items[i]
				labels := item.GetLabels()
				annotations := item.GetAnnotations()

				// Detect ownership
				ownership := agent.DetectOwnership(item)

				entry := MapEntry{
					ID:          fmt.Sprintf("%s/%s/%s/%s/%s", clusterName, item.GetNamespace(), gvr.Group, item.GetKind(), item.GetName()),
					ClusterName: clusterName,
					Namespace:   item.GetNamespace(),
					Kind:        item.GetKind(),
					Name:        item.GetName(),
					APIVersion:  item.GetAPIVersion(),
					Owner:       ownership.Type,
					Labels:      labels,
					Status:      detectStatus(item),
					CreatedAt:   item.GetCreationTimestamp().Time,
					UpdatedAt:   item.GetCreationTimestamp().Time,
				}

				// Extract ConfigHub details
				if ownership.Type == agent.OwnerConfigHub {
					entry.OwnerDetails = map[string]string{}
					if space := annotations["confighub.com/SpaceName"]; space != "" {
						entry.OwnerDetails["space"] = space
					}
					if unit := labels["confighub.com/UnitSlug"]; unit != "" {
						entry.OwnerDetails["unit"] = unit
						// Correlate with ConfigHub units
						correlation[unit] = append(correlation[unit], entry)
					}
					if rev := annotations["confighub.com/RevisionNum"]; rev != "" {
						entry.OwnerDetails["revision"] = rev
					}
				} else {
					// Check if workload has ConfigHub label but detected as different owner
					if unitSlug := labels["confighub.com/UnitSlug"]; unitSlug != "" && unitSet[unitSlug] {
						entry.OwnerDetails = map[string]string{"unit": unitSlug}
						correlation[unitSlug] = append(correlation[unitSlug], entry)
					} else if ownership.Type == "" || ownership.Type == agent.OwnerUnknown {
						// This is an orphan (not managed by GitOps or ConfigHub)
						// "Native" resources are those with no recognized owner
						orphans = append(orphans, entry)
					}
				}

				workloads = append(workloads, entry)
			}
		}

		return panelDataLoadedMsg{
			workloads:   workloads,
			correlation: correlation,
			orphans:     orphans,
		}
	}
}

// loadSuggestDataCmd fetches cluster workloads and generates unit suggestions
func loadSuggestDataCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// Build kubernetes config
		cfg, err := buildConfig()
		if err != nil {
			return suggestDataLoadedMsg{err: fmt.Errorf("build kubernetes config: %w", err)}
		}

		dynClient, err := dynamic.NewForConfig(cfg)
		if err != nil {
			return suggestDataLoadedMsg{err: fmt.Errorf("create dynamic client: %w", err)}
		}

		var workloads []WorkloadInfo

		// Workload resources to fetch
		workloadGVRs := []schema.GroupVersionResource{
			{Group: "apps", Version: "v1", Resource: "deployments"},
			{Group: "apps", Version: "v1", Resource: "statefulsets"},
			{Group: "apps", Version: "v1", Resource: "daemonsets"},
		}

		for _, gvr := range workloadGVRs {
			list, err := dynClient.Resource(gvr).List(ctx, metav1.ListOptions{})
			if err != nil {
				continue
			}
			for i := range list.Items {
				item := &list.Items[i]
				labels := item.GetLabels()

				// Detect ownership - skip workloads already managed by ConfigHub
				ownership := agent.DetectOwnership(item)
				if ownership.Type == agent.OwnerConfigHub {
					continue // Already in ConfigHub, don't suggest
				}

				workloads = append(workloads, WorkloadInfo{
					Kind:      item.GetKind(),
					Name:      item.GetName(),
					Namespace: item.GetNamespace(),
					Owner:     ownership.Type,
					Labels:    labels,
					Ready:     detectStatus(item) == "Ready",
				})
			}
		}

		// Generate suggestion using existing logic
		suggestion := SuggestHubAppSpaceStructure(workloads, "")

		return suggestDataLoadedMsg{
			proposal: &suggestion,
		}
	}
}

func loadConfigHubData() ([]*TreeNode, string, string, string, []string, error) {
	// Get context
	ctxJSON, err := runCubCommand("context", "get", "--json")
	if err != nil {
		return nil, "", "", "", nil, fmt.Errorf("failed to get context: %w", err)
	}
	var ctx CubContext
	if err := json.Unmarshal(ctxJSON, &ctx); err != nil {
		return nil, "", "", "", nil, fmt.Errorf("failed to parse context: %w", err)
	}

	// Get current space from context settings (for auto-focus)
	currentSpace := ctx.Settings.DefaultSpace

	// Get organizations
	orgsJSON, err := runCubCommand("organization", "list", "--json")
	if err != nil {
		return nil, "", "", "", nil, fmt.Errorf("failed to list organizations: %w", err)
	}
	var orgs []CubOrganization
	if err := json.Unmarshal(orgsJSON, &orgs); err != nil {
		return nil, "", "", "", nil, fmt.Errorf("failed to parse organizations: %w", err)
	}

	// Check for empty orgs - likely not logged in
	if len(orgs) == 0 {
		return nil, "", "", "", nil, fmt.Errorf("no organizations found - try running: cub auth login")
	}

	// Get all spaces
	spacesJSON, err := runCubCommand("space", "list", "--json")
	if err != nil {
		return nil, "", "", "", nil, fmt.Errorf("failed to list spaces: %w", err)
	}
	var spaces []CubSpaceData
	if err := json.Unmarshal(spacesJSON, &spaces); err != nil {
		return nil, "", "", "", nil, fmt.Errorf("failed to parse spaces: %w", err)
	}

	// Build tree
	var nodes []*TreeNode
	var spacesToLoad []string // spaces that need background loading
	currentOrg := ctx.Coordinate.OrganizationID

	// Find internal org ID
	var currentOrgInt string
	for _, org := range orgs {
		if org.ExternalID == currentOrg || org.Slug == currentOrg {
			currentOrgInt = org.OrganizationID
			break
		}
	}

	for _, org := range orgs {
		isCurrentOrg := org.ExternalID == currentOrg || org.Slug == currentOrg || org.OrganizationID == currentOrg

		orgNode := &TreeNode{
			ID:       org.OrganizationID,
			Name:     org.DisplayName,
			Type:     "org",
			Expanded: isCurrentOrg,
			Data:     org,
			OrgID:    org.ExternalID,
		}

		// Only show space/unit counts for current org (we can only fetch data for it)
		if isCurrentOrg {
			spaceCount := 0
			unitCount := 0
			for _, space := range spaces {
				if space.Space.OrganizationID == org.OrganizationID {
					spaceCount++
					unitCount += space.TotalUnitCount
				}
			}
			orgNode.Info = fmt.Sprintf("%d spaces, %d units", spaceCount, unitCount)
		}
		// Non-current orgs get no Info - we can't fetch their data without switching context

		// Only show spaces for current org (others need auth switch)
		if isCurrentOrg {
			// Add spaces as children
			for _, space := range spaces {
				if space.Space.OrganizationID != org.OrganizationID {
					continue
				}

				status := ""
				if space.TotalBridgeWorkerCount > 0 {
					status = "ok"
				} else if space.TotalUnitCount > 0 {
					status = "warn"
				}

				// Count targets
				targetCount := 0
				for _, count := range space.TargetCountByType {
					targetCount += count
				}

				spaceNode := &TreeNode{
					ID:       space.Space.Slug,
					Name:     space.Space.Slug,
					Type:     "space",
					Status:   status,
					Info:     fmt.Sprintf("units:%d targets:%d workers:%d", space.TotalUnitCount, targetCount, space.TotalBridgeWorkerCount),
					Parent:   orgNode,
					Expanded: space.Space.Slug == ctx.Settings.DefaultSpace,
					Data:     space,
					OrgID:    org.ExternalID,
				}

				// Add group nodes for Units, Targets, Workers
				unitsGroup := &TreeNode{
					ID:       space.Space.Slug + "/units",
					Name:     "Units",
					Type:     "group",
					Info:     fmt.Sprintf("(%d)", space.TotalUnitCount),
					Parent:   spaceNode,
					Expanded: space.Space.Slug == ctx.Settings.DefaultSpace,
					OrgID:    org.ExternalID,
				}

				targetsGroup := &TreeNode{
					ID:       space.Space.Slug + "/targets",
					Name:     "Targets",
					Type:     "group",
					Info:     fmt.Sprintf("(%d)", targetCount),
					Parent:   spaceNode,
					Expanded: false,
					OrgID:    org.ExternalID,
				}

				workersGroup := &TreeNode{
					ID:       space.Space.Slug + "/workers",
					Name:     "Workers",
					Type:     "group",
					Info:     fmt.Sprintf("(%d)", space.TotalBridgeWorkerCount),
					Parent:   spaceNode,
					Expanded: false,
					OrgID:    org.ExternalID,
				}

				// Queue this space for background loading (don't block initial render)
				spacesToLoad = append(spacesToLoad, space.Space.Slug)

				spaceNode.Children = []*TreeNode{unitsGroup, targetsGroup, workersGroup}
				orgNode.Children = append(orgNode.Children, spaceNode)
			}
		}

		nodes = append(nodes, orgNode)
	}

	return nodes, currentOrg, currentOrgInt, currentSpace, spacesToLoad, nil
}

func loadUnitsForSpace(spaceSlug string) ([]CubUnitData, error) {
	unitsJSON, err := runCubCommand("unit", "list", "--space", spaceSlug, "--json")
	if err != nil {
		return nil, err
	}
	var units []CubUnitData
	if err := json.Unmarshal(unitsJSON, &units); err != nil {
		return nil, err
	}
	return units, nil
}

func loadTargetsForSpace(spaceSlug string) ([]CubTargetData, error) {
	targetsJSON, err := runCubCommand("target", "list", "--space", spaceSlug, "--json")
	if err != nil {
		return nil, err
	}
	var targets []CubTargetData
	if err := json.Unmarshal(targetsJSON, &targets); err != nil {
		return nil, err
	}
	return targets, nil
}

func loadWorkersForSpace(spaceSlug string) ([]CubWorkerData, error) {
	workersJSON, err := runCubCommand("worker", "list", "--space", spaceSlug, "--json")
	if err != nil {
		return nil, err
	}
	var workers []CubWorkerData
	if err := json.Unmarshal(workersJSON, &workers); err != nil {
		return nil, err
	}
	return workers, nil
}

func runCubCommand(args ...string) ([]byte, error) {
	cmd := exec.Command("cub", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return output, nil
}

// Import wizard commands
func loadNamespacesCmd() tea.Cmd {
	return func() tea.Msg {
		// Get namespace names
		out, err := exec.Command("kubectl", "get", "namespaces", "-o", "jsonpath={.items[*].metadata.name}").Output()
		if err != nil {
			return namespacesLoadedMsg{err: err}
		}
		nsNames := strings.Fields(string(out))

		// Get workload counts and owner info for each namespace
		var namespaces []namespaceInfo
		for _, ns := range nsNames {
			info := namespaceInfo{Name: ns}

			// Get all workloads with their labels in one call per type
			// Deployments
			if out, err := exec.Command("kubectl", "get", "deployments", "-n", ns, "-o", "json").Output(); err == nil {
				countWorkloadsWithOwners(out, &info, &info.Deployments)
			}

			// StatefulSets
			if out, err := exec.Command("kubectl", "get", "statefulsets", "-n", ns, "-o", "json").Output(); err == nil {
				countWorkloadsWithOwners(out, &info, &info.StatefulSet)
			}

			// DaemonSets
			if out, err := exec.Command("kubectl", "get", "daemonsets", "-n", ns, "-o", "json").Output(); err == nil {
				countWorkloadsWithOwners(out, &info, &info.DaemonSets)
			}

			namespaces = append(namespaces, info)
		}

		return namespacesLoadedMsg{namespaces: namespaces}
	}
}

func loadArgoAppsCmd() tea.Cmd {
	return func() tea.Msg {
		// Get kubernetes config using clientcmd
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		configOverrides := &clientcmd.ConfigOverrides{}
		kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

		config, err := kubeConfig.ClientConfig()
		if err != nil {
			return argoAppsLoadedMsg{err: fmt.Errorf("failed to get kubeconfig: %w", err)}
		}

		dynamicClient, err := dynamic.NewForConfig(config)
		if err != nil {
			return argoAppsLoadedMsg{err: fmt.Errorf("failed to create dynamic client: %w", err)}
		}

		ctx := context.Background()

		// First check if ArgoCD is installed
		if err := verifyArgoInstalled(ctx, dynamicClient); err != nil {
			return argoAppsLoadedMsg{err: fmt.Errorf("ArgoCD not found: %w", err)}
		}

		// Get ArgoCD applications from argocd namespace
		apps, err := listArgoApplicationsDetailed(ctx, dynamicClient, "argocd")
		if err != nil {
			return argoAppsLoadedMsg{err: fmt.Errorf("failed to list ArgoCD Applications: %w", err)}
		}

		// Convert to TUI type
		var appInfos []argoAppInfo
		for _, app := range apps {
			appInfos = append(appInfos, argoAppInfo{
				Name:         app.Name,
				Namespace:    app.Namespace,
				Project:      app.Project,
				RepoURL:      app.RepoURL,
				Path:         app.Path,
				DestServer:   app.DestServer,
				DestNS:       app.DestNS,
				SyncStatus:   app.SyncStatus,
				HealthStatus: app.HealthStatus,
				IsAppOfApps:  app.IsAppOfApps,
				ChildApps:    app.ChildApps,
			})
		}

		return argoAppsLoadedMsg{apps: appInfos}
	}
}

func loadArgoResourcesCmd(app argoAppInfo) tea.Cmd {
	return func() tea.Msg {
		// Get kubernetes config using clientcmd
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		configOverrides := &clientcmd.ConfigOverrides{}
		kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

		config, err := kubeConfig.ClientConfig()
		if err != nil {
			return argoResourcesLoadedMsg{err: fmt.Errorf("failed to get kubeconfig: %w", err)}
		}

		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			return argoResourcesLoadedMsg{err: fmt.Errorf("failed to create clientset: %w", err)}
		}

		dynamicClient, err := dynamic.NewForConfig(config)
		if err != nil {
			return argoResourcesLoadedMsg{err: fmt.Errorf("failed to create dynamic client: %w", err)}
		}

		ctx := context.Background()

		// Get managed resources for this ArgoCD Application
		resources, err := getManagedResources(ctx, clientset, dynamicClient, app.Name, app.DestNS)
		if err != nil {
			return argoResourcesLoadedMsg{err: fmt.Errorf("failed to get managed resources: %w", err)}
		}

		// Also get the Application YAML itself (cleaned)
		argoApp, appYAML, err := getArgoApplication(ctx, dynamicClient, app.Namespace, app.Name)
		if err != nil {
			return argoResourcesLoadedMsg{err: fmt.Errorf("failed to get Application: %w", err)}
		}

		// Clean the application YAML
		cleanedAppYAML, _ := cleanResourceYAML(appYAML)
		_ = argoApp // We have the raw YAML, don't need the struct

		return argoResourcesLoadedMsg{
			app:       &app,
			resources: resources,
			appYAML:   cleanedAppYAML,
		}
	}
}

func disableArgoSyncCmd(namespace, name string) tea.Cmd {
	return func() tea.Msg {
		// Get kubernetes config using clientcmd
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		configOverrides := &clientcmd.ConfigOverrides{}
		kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

		config, err := kubeConfig.ClientConfig()
		if err != nil {
			return argoSyncDisabledMsg{appName: name, err: fmt.Errorf("failed to get kubeconfig: %w", err)}
		}

		dynamicClient, err := dynamic.NewForConfig(config)
		if err != nil {
			return argoSyncDisabledMsg{appName: name, err: fmt.Errorf("failed to create dynamic client: %w", err)}
		}

		ctx := context.Background()
		if err := disableArgoAutoSync(ctx, dynamicClient, namespace, name); err != nil {
			return argoSyncDisabledMsg{appName: name, err: err}
		}

		return argoSyncDisabledMsg{appName: name}
	}
}

func deleteArgoAppCmd(namespace, name string) tea.Cmd {
	return func() tea.Msg {
		// Get kubernetes config using clientcmd
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		configOverrides := &clientcmd.ConfigOverrides{}
		kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

		config, err := kubeConfig.ClientConfig()
		if err != nil {
			return argoAppDeletedMsg{appName: name, err: fmt.Errorf("failed to get kubeconfig: %w", err)}
		}

		dynamicClient, err := dynamic.NewForConfig(config)
		if err != nil {
			return argoAppDeletedMsg{appName: name, err: fmt.Errorf("failed to create dynamic client: %w", err)}
		}

		ctx := context.Background()
		if err := deleteArgoApplication(ctx, dynamicClient, namespace, name); err != nil {
			return argoAppDeletedMsg{appName: name, err: err}
		}

		return argoAppDeletedMsg{appName: name}
	}
}

// applyUnitCmd applies a unit to its target. This is called AFTER ArgoCD cleanup
// to ensure ArgoCD's selfHeal doesn't revert the changes.
// It first cleans stale ConfigHub inventory annotations from the target resources
// to prevent ownership conflicts from previous imports.
func applyUnitCmd(space, unitSlug string, workloads []WorkloadInfo) tea.Cmd {
	return func() tea.Msg {
		// Clean stale ConfigHub inventory annotations from live resources
		// This prevents conflicts when re-importing resources that were previously managed
		// Errors are intentionally ignored: cleanup is best-effort, missing annotations are fine
		for _, w := range workloads {
			// Remove stale ownership annotation (best-effort)
			_ = exec.Command("kubectl", "annotate", strings.ToLower(w.Kind), w.Name,
				"-n", w.Namespace, "config.k8s.io/owning-inventory-", "--overwrite").Run() //nolint:errcheck // best-effort cleanup
			// Remove stale ConfigHub label (best-effort)
			_ = exec.Command("kubectl", "label", strings.ToLower(w.Kind), w.Name,
				"-n", w.Namespace, "confighub.com/UnitSlug-", "--overwrite").Run() //nolint:errcheck // best-effort cleanup
			// Remove stale inventory ID label (best-effort)
			_ = exec.Command("kubectl", "label", strings.ToLower(w.Kind), w.Name,
				"-n", w.Namespace, "cli-utils.sigs.k8s.io/inventory-id-", "--overwrite").Run() //nolint:errcheck // best-effort cleanup
		}

		cmd := exec.Command("cub", "unit", "apply", unitSlug, "--space", space)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			return unitAppliedMsg{unitSlug: unitSlug, err: fmt.Errorf("apply failed: %s", out.String())}
		}
		return unitAppliedMsg{unitSlug: unitSlug}
	}
}

func testPipelineCmd(space, unitSlug string) tea.Cmd {
	return func() tea.Msg {
		// Use rollout restart which adds annotation to pod template,
		// triggering a rolling update - this is the most comprehensive test
		result, err := testRolloutRestart(space, unitSlug)
		return testUpdateCompleteMsg{result: result, err: err}
	}
}

// countWorkloadsWithOwners parses kubectl JSON output and counts workloads by owner
func countWorkloadsWithOwners(jsonData []byte, info *namespaceInfo, kindCount *int) {
	var result struct {
		Items []struct {
			Metadata struct {
				Labels      map[string]string `json:"labels"`
				Annotations map[string]string `json:"annotations"`
			} `json:"metadata"`
		} `json:"items"`
	}

	if err := json.Unmarshal(jsonData, &result); err != nil {
		return
	}

	*kindCount = len(result.Items)

	for _, item := range result.Items {
		labels := item.Metadata.Labels
		annotations := item.Metadata.Annotations

		// Check for ConfigHub first
		if labels["confighub.com/UnitSlug"] != "" || annotations["confighub.com/UnitSlug"] != "" {
			info.ConfigHubCount++
			continue
		}

		// Check for Flux
		if labels["kustomize.toolkit.fluxcd.io/name"] != "" ||
			labels["helm.toolkit.fluxcd.io/name"] != "" ||
			labels["kustomize.toolkit.fluxcd.io/namespace"] != "" {
			info.FluxCount++
			continue
		}

		// Check for Argo CD
		if labels["argocd.argoproj.io/instance"] != "" ||
			labels["app.kubernetes.io/instance"] != "" && labels["app.kubernetes.io/managed-by"] == "Helm" && strings.Contains(labels["app.kubernetes.io/instance"], "argocd") {
			info.ArgoCount++
			continue
		}

		// Check for Helm
		if labels["app.kubernetes.io/managed-by"] == "Helm" {
			info.HelmCount++
			continue
		}

		// Native (kubectl)
		info.NativeCount++
	}
}

func discoverWorkloadsCmd(namespace string) tea.Cmd {
	return func() tea.Msg {
		workloads, err := discoverWorkloads(namespace)
		if err != nil {
			return workloadsDiscoveredMsg{err: err}
		}
		return workloadsDiscoveredMsg{workloads: workloads}
	}
}

func importWorkloadsCmd(space string, workloads []WorkloadInfo) tea.Cmd {
	return func() tea.Msg {
		success, failed := 0, 0
		for _, w := range workloads {
			// Use createUnitWithConfig if we have extracted config
			if err := createUnitWithConfig(space, w.Name, w.ExtractedConfig); err != nil {
				failed++
				continue
			}
			if err := labelWorkload(w.Kind, w.Namespace, w.Name, w.Name); err != nil {
				failed++
				continue
			}
			success++
		}
		return importCompleteMsg{success: success, failed: failed}
	}
}

// importArgoWorkloadsCmd imports ArgoCD workloads as a combined unit.
// Creates a single {appName}-workload unit with all resources, matching the CLI behavior.
// Also links to target and applies so livedata becomes available.
func importArgoWorkloadsCmd(space, appName, target string, workloads []WorkloadInfo) tea.Cmd {
	return func() tea.Msg {
		if len(workloads) == 0 {
			return importCompleteMsg{success: 0, failed: 0}
		}

		// Combine all workload configs into a single YAML
		var combinedYAML strings.Builder
		for i, w := range workloads {
			if i > 0 {
				combinedYAML.WriteString("---\n")
			}
			combinedYAML.WriteString(w.ExtractedConfig)
			if !strings.HasSuffix(w.ExtractedConfig, "\n") {
				combinedYAML.WriteString("\n")
			}
		}

		// Create single unit with {appName}-workload slug
		unitSlug := fmt.Sprintf("%s-workload", appName)
		if err := createUnitWithConfig(space, unitSlug, combinedYAML.String()); err != nil {
			return importCompleteMsg{success: 0, failed: len(workloads)}
		}

		// Link unit to target (required for apply and livedata)
		// NOTE: We do NOT apply here - apply happens after ArgoCD cleanup step
		// because ArgoCD selfHeal would revert our changes if sync is still enabled
		var applyErr error
		if target != "" {
			setTargetCmd := exec.Command("cub", "unit", "set-target", unitSlug, target, "--space", space)
			var setTargetOut bytes.Buffer
			setTargetCmd.Stdout = &setTargetOut
			setTargetCmd.Stderr = &setTargetOut
			if err := setTargetCmd.Run(); err != nil {
				applyErr = fmt.Errorf("set-target failed: %s", setTargetOut.String())
			}
			// Apply will be done after ArgoCD cleanup step
		} else {
			applyErr = fmt.Errorf("no target specified - unit created but not applied")
		}

		// Label all workloads with the combined unit slug
		for _, w := range workloads {
			// Best-effort labeling: don't fail the whole import for labeling issues
			_ = labelWorkload(w.Kind, w.Namespace, w.Name, unitSlug) //nolint:errcheck // best-effort labeling
		}

		return importCompleteMsg{success: len(workloads), failed: 0, applyError: applyErr}
	}
}

// importWorkloadsWithSuggestionCmd imports workloads using the smart suggestion structure
// Groups workloads by app/variant and creates one unit per variant with the suggested slug
func importWorkloadsWithSuggestionCmd(space string, suggestion *ImportSuggestion, selected []WorkloadInfo) tea.Cmd {
	return func() tea.Msg {
		if suggestion == nil {
			// Fall back to basic import if no suggestion
			return importWorkloadsCmd(space, selected)()
		}

		// Build a set of selected workload names for quick lookup
		selectedSet := make(map[string]WorkloadInfo)
		for _, w := range selected {
			selectedSet[w.Namespace+"/"+w.Name] = w
		}

		success, failed := 0, 0

		// Iterate through the suggestion structure
		for _, app := range suggestion.Apps {
			for _, variant := range app.Variants {
				// Collect selected workloads in this variant
				var variantWorkloads []WorkloadInfo
				for _, w := range variant.Workloads {
					if sw, ok := selectedSet[w.Namespace+"/"+w.Name]; ok {
						variantWorkloads = append(variantWorkloads, sw)
					}
				}

				if len(variantWorkloads) == 0 {
					continue // No selected workloads in this variant
				}

				// Create unit with the suggested slug
				// Use the first workload's config for the unit if available
				var config string
				for _, w := range variantWorkloads {
					if w.ExtractedConfig != "" {
						config = w.ExtractedConfig
						break
					}
				}

				if err := createUnitWithConfig(space, variant.UnitSlug, config); err != nil {
					failed += len(variantWorkloads)
					continue
				}

				// Label all workloads in this variant with the same unit slug
				for _, w := range variantWorkloads {
					if err := labelWorkload(w.Kind, w.Namespace, w.Name, variant.UnitSlug); err != nil {
						failed++
						continue
					}
					success++
				}
			}
		}

		return importCompleteMsg{success: success, failed: failed}
	}
}

// extractConfigCmd extracts GitOps configuration for selected workloads
func extractConfigCmd(workloads []WorkloadInfo) tea.Cmd {
	return func() tea.Msg {
		// Make a copy since we're modifying the slice
		updated := make([]WorkloadInfo, len(workloads))
		copy(updated, workloads)

		successCount := 0
		for i := range updated {
			w := &updated[i]
			if w.GitOpsRef == nil {
				continue // No GitOps source to extract from
			}
			if err := ExtractGitOpsConfig(w); err == nil {
				successCount++
			}
			// ConfigError is set by ExtractGitOpsConfig on failure
		}
		return configExtractedMsg{workloads: updated, success: successCount}
	}
}

// loadExistingSpacesCmd fetches the list of existing spaces for the setup wizard
func loadExistingSpacesCmd() tea.Cmd {
	return func() tea.Msg {
		spacesJSON, err := runCubCommand("space", "list", "--json")
		if err != nil {
			return spacesLoadedMsg{err: err}
		}
		var spaces []CubSpaceData
		if err := json.Unmarshal(spacesJSON, &spaces); err != nil {
			return spacesLoadedMsg{err: err}
		}
		var slugs []string
		for _, s := range spaces {
			slugs = append(slugs, s.Space.Slug)
		}
		return spacesLoadedMsg{spaces: slugs}
	}
}

// createSpaceCmd creates a new ConfigHub space
func createSpaceCmd(spaceName string) tea.Cmd {
	return func() tea.Msg {
		_, err := runCubCommand("space", "create", spaceName, "--set-context")
		if err != nil {
			return spaceCreatedMsg{err: fmt.Errorf("failed to create space: %w", err)}
		}
		return spaceCreatedMsg{space: spaceName}
	}
}

// loadWorkersForSpaceCmd fetches existing workers in a space
func loadWorkersForSpaceCmd(space string) tea.Cmd {
	return func() tea.Msg {
		workersJSON, err := runCubCommand("worker", "list", "--space", space, "--json")
		if err != nil {
			return workersLoadedMsg{err: err}
		}
		var rawWorkers []struct {
			BridgeWorker struct {
				Slug      string `json:"Slug"`
				Condition string `json:"Condition"`
			} `json:"BridgeWorker"`
		}
		if err := json.Unmarshal(workersJSON, &rawWorkers); err != nil {
			return workersLoadedMsg{err: err}
		}
		var workers []workerInfo
		for _, w := range rawWorkers {
			workers = append(workers, workerInfo{
				Slug:      w.BridgeWorker.Slug,
				Condition: w.BridgeWorker.Condition,
			})
		}
		return workersLoadedMsg{workers: workers}
	}
}

// createWorkerCmd creates a new ConfigHub worker
func createWorkerCmd(space, workerName string) tea.Cmd {
	return func() tea.Msg {
		_, err := runCubCommand("worker", "create", workerName, "--space", space)
		if err != nil {
			return workerCreatedMsg{err: fmt.Errorf("failed to create worker: %w", err)}
		}
		return workerCreatedMsg{worker: workerName}
	}
}

// waitForTargetCmd polls until an auto-created target is found
// Target slug pattern: {worker}-kubernetes-yaml-{kube-context}
func waitForTargetCmd(space, workerName, kubeContext string) tea.Cmd {
	return func() tea.Msg {
		// Build expected target slug: worker-kubernetes-yaml-context
		// Convert context name to slug format (replace / with -)
		contextSlug := strings.ReplaceAll(kubeContext, "/", "-")
		expectedSlug := fmt.Sprintf("%s-kubernetes-yaml-%s", workerName, contextSlug)

		// Poll for up to 60 seconds (30 attempts, 2 seconds apart)
		for i := 0; i < 30; i++ {
			// Check if target exists
			targetsJSON, err := runCubCommand("target", "list", "--space", space, "--json")
			if err == nil {
				var targets []struct {
					Target struct {
						Slug         string `json:"Slug"`
						ProviderType string `json:"ProviderType"`
					} `json:"Target"`
					Parameters map[string]interface{} `json:"Parameters"`
				}
				if err := json.Unmarshal(targetsJSON, &targets); err == nil {
					// First, look for exact match
					for _, t := range targets {
						if t.Target.Slug == expectedSlug {
							return targetFoundMsg{target: expectedSlug}
						}
					}
					// Fallback: look for any Kubernetes target matching our context
					for _, t := range targets {
						if t.Target.ProviderType == "Kubernetes" {
							// Check if this target's KubeContext matches
							if params := t.Parameters; params != nil {
								if kc, ok := params["KubeContext"].(string); ok && kc == kubeContext {
									return targetFoundMsg{target: t.Target.Slug}
								}
							}
						}
					}
				}
			}
			// Wait 2 seconds before next poll
			time.Sleep(2 * time.Second)
		}
		return targetFoundMsg{err: fmt.Errorf("no Kubernetes target found for context %s within 60 seconds", kubeContext)}
	}
}

// startWorkerCmd starts the worker process in the background
func startWorkerCmd(space, workerName string) tea.Cmd {
	return func() tea.Msg {
		// Start worker in background (detached process)
		cmd := exec.Command("cub", "worker", "run", workerName, "--space", space)
		cmd.Stdout = nil
		cmd.Stderr = nil
		cmd.Stdin = nil

		if err := cmd.Start(); err != nil {
			return workerStartedMsg{err: fmt.Errorf("failed to start worker: %w", err)}
		}

		// Don't wait for the process - let it run in background
		// We must call Wait() to release process resources, but we don't need the result
		go func() {
			_ = cmd.Wait() //nolint:errcheck // background process, exit status irrelevant
		}()

		return workerStartedMsg{worker: workerName}
	}
}

// waitForWorkerReadyCmd polls until the worker is Ready (or timeout)
func waitForWorkerReadyCmd(space, workerName string) tea.Cmd {
	return func() tea.Msg {
		// Poll for up to 30 seconds (15 attempts, 2 seconds apart)
		for i := 0; i < 15; i++ {
			workersJSON, err := runCubCommand("worker", "list", "--space", space, "--json")
			if err == nil {
				var rawWorkers []struct {
					BridgeWorker struct {
						Slug      string `json:"Slug"`
						Condition string `json:"Condition"`
					} `json:"BridgeWorker"`
				}
				if err := json.Unmarshal(workersJSON, &rawWorkers); err == nil {
					for _, w := range rawWorkers {
						if w.BridgeWorker.Slug == workerName && w.BridgeWorker.Condition == "Ready" {
							return workerReadyMsg{worker: workerName}
						}
					}
				}
			}
			// Wait 2 seconds before next poll
			time.Sleep(2 * time.Second)
		}
		return workerReadyMsg{err: fmt.Errorf("worker did not become ready within 30 seconds")}
	}
}

// runCommandCmd executes a shell command and returns the output
func runCommandCmd(command string) tea.Cmd {
	return func() tea.Msg {
		// Parse command into parts
		parts := strings.Fields(command)
		if len(parts) == 0 {
			return cmdCompleteMsg{err: fmt.Errorf("empty command")}
		}

		// Special handling for common commands
		cmdName := parts[0]
		var args []string
		if len(parts) > 1 {
			args = parts[1:]
		}

		// Execute command
		cmd := exec.Command(cmdName, args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return cmdCompleteMsg{
				output: string(output),
				err:    fmt.Errorf("%s: %w", cmdName, err),
			}
		}
		return cmdCompleteMsg{output: string(output)}
	}
}

// getCurrentKubeContext returns the current kubectl context name
func getCurrentKubeContext() string {
	cmd := exec.Command("kubectl", "config", "current-context")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Create wizard commands

// loadCreateUnitsCmd loads units in a space for cloning (includes toolchain type)
func loadCreateUnitsCmd(space string) tea.Cmd {
	return func() tea.Msg {
		unitsJSON, err := runCubCommand("unit", "list", "--space", space, "--json")
		if err != nil {
			return createUnitsLoadedMsg{err: err}
		}
		var rawUnits []struct {
			Unit struct {
				Slug          string `json:"Slug"`
				ToolchainType string `json:"ToolchainType"`
			} `json:"Unit"`
		}
		if err := json.Unmarshal(unitsJSON, &rawUnits); err != nil {
			return createUnitsLoadedMsg{err: err}
		}
		var units []createUnitInfo
		for _, u := range rawUnits {
			if u.Unit.Slug != "" {
				toolchain := u.Unit.ToolchainType
				if toolchain == "" {
					toolchain = "Kubernetes/YAML" // Default
				}
				units = append(units, createUnitInfo{
					Slug:      u.Unit.Slug,
					Toolchain: toolchain,
				})
			}
		}
		return createUnitsLoadedMsg{units: units}
	}
}

// loadCreateTargetsCmd loads targets in a space filtered by toolchain type
func loadCreateTargetsCmd(space, toolchain string) tea.Cmd {
	return func() tea.Msg {
		targetsJSON, err := runCubCommand("target", "list", "--space", space, "--json")
		if err != nil {
			return createTargetsLoadedMsg{err: err}
		}
		var rawTargets []struct {
			Target struct {
				Slug          string `json:"Slug"`
				ToolchainType string `json:"ToolchainType"`
			} `json:"Target"`
		}
		if err := json.Unmarshal(targetsJSON, &rawTargets); err != nil {
			return createTargetsLoadedMsg{err: err}
		}
		var targets []string
		for _, t := range rawTargets {
			if t.Target.Slug != "" {
				// Filter targets by matching toolchain type
				targetToolchain := t.Target.ToolchainType
				if targetToolchain == "" {
					targetToolchain = "Kubernetes/YAML" // Default
				}
				if targetToolchain == toolchain {
					targets = append(targets, t.Target.Slug)
				}
			}
		}
		return createTargetsLoadedMsg{targets: targets}
	}
}

// loadCreateWorkersCmd loads workers in a space for target creation
func loadCreateWorkersCmd(space string) tea.Cmd {
	return func() tea.Msg {
		workersJSON, err := runCubCommand("worker", "list", "--space", space, "--json")
		if err != nil {
			return createWorkersLoadedMsg{err: err}
		}
		var rawWorkers []struct {
			BridgeWorker struct {
				Slug string `json:"Slug"`
			} `json:"BridgeWorker"`
		}
		if err := json.Unmarshal(workersJSON, &rawWorkers); err != nil {
			return createWorkersLoadedMsg{err: err}
		}
		var workers []string
		for _, w := range rawWorkers {
			if w.BridgeWorker.Slug != "" {
				workers = append(workers, w.BridgeWorker.Slug)
			}
		}
		return createWorkersLoadedMsg{workers: workers}
	}
}

// doCreateSpaceCmd creates a new space
func doCreateSpaceCmd(name string) tea.Cmd {
	return func() tea.Msg {
		_, err := runCubCommand("space", "create", name, "--set-context")
		if err != nil {
			return createResourceMsg{resourceType: "space", name: name, err: fmt.Errorf("failed to create space: %w", err)}
		}
		return createResourceMsg{resourceType: "space", name: name, space: ""}
	}
}

// doCreateUnitCmd creates a new unit
func doCreateUnitCmd(space, name, cloneFrom, target string) tea.Cmd {
	return func() tea.Msg {
		args := []string{"unit", "create", name, "--space", space}

		if cloneFrom != "" {
			// Clone from existing unit
			args = append(args, "--upstream-unit", cloneFrom)
		} else {
			// Create with minimal placeholder config from stdin
			args = append(args, "-")
		}

		if target != "" {
			args = append(args, "--target", target)
		}

		if cloneFrom != "" {
			// No stdin needed for clone
			_, err := runCubCommand(args...)
			if err != nil {
				return createResourceMsg{resourceType: "unit", name: name, space: space, err: fmt.Errorf("failed to create unit: %w", err)}
			}
		} else {
			// Need to pipe config via stdin
			cmd := exec.Command("cub", args...)
			cmd.Stdin = strings.NewReader(`# Empty unit configuration
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: placeholder
data:
  placeholder: "true"
`)
			output, err := cmd.CombinedOutput()
			if err != nil {
				return createResourceMsg{resourceType: "unit", name: name, space: space, err: fmt.Errorf("failed to create unit: %s", string(output))}
			}
		}
		return createResourceMsg{resourceType: "unit", name: name, space: space}
	}
}

// doCreateTargetCmd creates a new target
func doCreateTargetCmd(space, name, worker, provider string) tea.Cmd {
	return func() tea.Msg {
		args := []string{"target", "create", name, "{}", "--space", space, "--provider", provider}
		if worker != "" {
			args = append(args, worker)
		}
		_, err := runCubCommand(args...)
		if err != nil {
			return createResourceMsg{resourceType: "target", name: name, space: space, err: fmt.Errorf("failed to create target: %w", err)}
		}
		return createResourceMsg{resourceType: "target", name: name, space: space}
	}
}

// doDeleteSpaceCmd deletes a space
func doDeleteSpaceCmd(name string) tea.Cmd {
	return func() tea.Msg {
		_, err := runCubCommand("space", "delete", name, "--recursive")
		if err != nil {
			return deleteResourceMsg{resourceType: "space", name: name, err: fmt.Errorf("failed to delete space: %w", err)}
		}
		return deleteResourceMsg{resourceType: "space", name: name, space: ""}
	}
}

// doDeleteUnitCmd deletes a unit
func doDeleteUnitCmd(space, name string) tea.Cmd {
	return func() tea.Msg {
		_, err := runCubCommand("unit", "delete", name, "--space", space)
		if err != nil {
			return deleteResourceMsg{resourceType: "unit", name: name, space: space, err: fmt.Errorf("failed to delete unit: %w", err)}
		}
		return deleteResourceMsg{resourceType: "unit", name: name, space: space}
	}
}

// doDeleteTargetCmd deletes a target
func doDeleteTargetCmd(space, name string) tea.Cmd {
	return func() tea.Msg {
		_, err := runCubCommand("target", "delete", name, "--space", space)
		if err != nil {
			return deleteResourceMsg{resourceType: "target", name: name, space: space, err: fmt.Errorf("failed to delete target: %w", err)}
		}
		return deleteResourceMsg{resourceType: "target", name: name, space: space}
	}
}

// buildUnitInfo creates a detailed info string for a unit
func buildUnitInfo(unit CubUnitData) string {
	var parts []string

	// Target connection
	if unit.Target.Slug != "" && unit.Target.Slug != "-" {
		parts = append(parts, fmt.Sprintf("→ %s", unit.Target.Slug))
	}

	// Revision info (show if head != live, indicating pending changes)
	if unit.Unit.HeadRevisionNum > 0 {
		if unit.Unit.LiveRevisionNum > 0 && unit.Unit.HeadRevisionNum != unit.Unit.LiveRevisionNum {
			parts = append(parts, fmt.Sprintf("rev:%d→%d", unit.Unit.LiveRevisionNum, unit.Unit.HeadRevisionNum))
		} else {
			parts = append(parts, fmt.Sprintf("rev:%d", unit.Unit.HeadRevisionNum))
		}
	}

	// Error status (accessibility: always show text with error icon)
	if unit.UnitStatus.Status == "Error" {
		parts = append(parts, "error")
	}

	// Sync status (if not synced)
	if unit.UnitStatus.SyncStatus == "OutOfSync" {
		parts = append(parts, "out-of-sync")
	}

	// Drift status (if drifted)
	if unit.UnitStatus.Drift == "Drifted" {
		parts = append(parts, "drifted")
	}

	// Worker (if assigned)
	if unit.BridgeWorker.Slug != "" {
		parts = append(parts, fmt.Sprintf("worker:%s", unit.BridgeWorker.Slug))
	}

	return strings.Join(parts, "  ")
}

// buildUnitDetailChildren creates detail child nodes for an expanded unit
func buildUnitDetailChildren(unit CubUnitData, parent *TreeNode) []*TreeNode {
	var children []*TreeNode

	// Target info
	if unit.Target.Slug != "" && unit.Target.Slug != "-" {
		targetInfo := unit.Target.Slug
		if unit.Target.ProviderType != "" {
			targetInfo += fmt.Sprintf(" (%s)", unit.Target.ProviderType)
		}
		children = append(children, &TreeNode{
			ID:     parent.ID + "/target",
			Name:   "Target",
			Type:   "detail",
			Info:   targetInfo,
			Parent: parent,
		})
	}

	// Revision info
	revInfo := fmt.Sprintf("head: %d", unit.Unit.HeadRevisionNum)
	if unit.Unit.LiveRevisionNum > 0 {
		revInfo += fmt.Sprintf(", live: %d", unit.Unit.LiveRevisionNum)
		if unit.Unit.HeadRevisionNum != unit.Unit.LiveRevisionNum {
			revInfo += fmt.Sprintf(" (%d pending)", unit.Unit.HeadRevisionNum-unit.Unit.LiveRevisionNum)
		}
	} else {
		revInfo += " (not yet live)"
	}
	children = append(children, &TreeNode{
		ID:     parent.ID + "/revision",
		Name:   "Revision",
		Type:   "detail",
		Info:   revInfo,
		Parent: parent,
	})

	// Status info
	statusInfo := unit.UnitStatus.Status
	if unit.UnitStatus.SyncStatus != "" && unit.UnitStatus.SyncStatus != "InSync" {
		statusInfo += fmt.Sprintf(", sync: %s", unit.UnitStatus.SyncStatus)
	}
	if unit.UnitStatus.Drift != "" && unit.UnitStatus.Drift != "NotDrifted" && unit.UnitStatus.Drift != "N/A" {
		statusInfo += fmt.Sprintf(", drift: %s", unit.UnitStatus.Drift)
	}
	statusNode := &TreeNode{
		ID:     parent.ID + "/status",
		Name:   "Status",
		Type:   "detail",
		Info:   statusInfo,
		Parent: parent,
	}
	statusNode.Status = unit.DeriveStatus()
	children = append(children, statusNode)

	// Last action info (if available)
	if unit.UnitStatus.Action != "" {
		actionInfo := unit.UnitStatus.Action
		if unit.UnitStatus.ActionResult != "" {
			actionInfo += fmt.Sprintf(" → %s", unit.UnitStatus.ActionResult)
		}
		children = append(children, &TreeNode{
			ID:     parent.ID + "/action",
			Name:   "Last Action",
			Type:   "detail",
			Info:   actionInfo,
			Parent: parent,
		})
	}

	// Worker info
	if unit.BridgeWorker.Slug != "" {
		workerInfo := unit.BridgeWorker.Slug
		if unit.BridgeWorker.Condition != "" {
			workerInfo += fmt.Sprintf(" (%s)", unit.BridgeWorker.Condition)
		}
		if unit.BridgeWorker.IPAddress != "" {
			workerInfo += fmt.Sprintf(" @ %s", unit.BridgeWorker.IPAddress)
		}
		workerNode := &TreeNode{
			ID:     parent.ID + "/worker",
			Name:   "Worker",
			Type:   "detail",
			Info:   workerInfo,
			Parent: parent,
		}
		if unit.BridgeWorker.Condition == "Ready" {
			workerNode.Status = "ok"
		} else {
			workerNode.Status = "warn"
		}
		children = append(children, workerNode)
	}

	// Space info
	if unit.Space.Slug != "" {
		children = append(children, &TreeNode{
			ID:     parent.ID + "/space",
			Name:   "Space",
			Type:   "detail",
			Info:   unit.Space.Slug,
			Parent: parent,
		})
	}

	return children
}

// updateSearchMatches finds all nodes matching the search query
func (m *Model) updateSearchMatches() {
	m.searchMatches = nil
	m.searchIndex = 0

	if m.searchQuery == "" {
		return
	}

	query := strings.ToLower(m.searchQuery)
	for i, node := range m.flatList {
		// Match against name and info
		if strings.Contains(strings.ToLower(node.Name), query) ||
			strings.Contains(strings.ToLower(node.Info), query) {
			m.searchMatches = append(m.searchMatches, i)
		}
	}

	// If we have matches, jump to the first one at or after cursor
	if len(m.searchMatches) > 0 {
		for i, matchIdx := range m.searchMatches {
			if matchIdx >= m.cursor {
				m.searchIndex = i
				m.cursor = matchIdx
				return
			}
		}
		// No match at or after cursor, wrap to first
		m.searchIndex = 0
		m.cursor = m.searchMatches[0]
	}
}

// nextSearchMatch moves to the next search match
func (m *Model) nextSearchMatch() {
	if len(m.searchMatches) == 0 {
		return
	}
	m.searchIndex = (m.searchIndex + 1) % len(m.searchMatches)
	m.cursor = m.searchMatches[m.searchIndex]
}

// prevSearchMatch moves to the previous search match
func (m *Model) prevSearchMatch() {
	if len(m.searchMatches) == 0 {
		return
	}
	m.searchIndex--
	if m.searchIndex < 0 {
		m.searchIndex = len(m.searchMatches) - 1
	}
	m.cursor = m.searchMatches[m.searchIndex]
}

// isSearchMatch returns true if the given flatList index is a search match
func (m *Model) isSearchMatch(idx int) bool {
	for _, matchIdx := range m.searchMatches {
		if matchIdx == idx {
			return true
		}
	}
	return false
}

// nodeMatchesQuery returns true if this node directly matches the search query
func (m *Model) nodeMatchesQuery(node *TreeNode) bool {
	if m.searchQuery == "" {
		return true
	}
	query := strings.ToLower(m.searchQuery)
	return strings.Contains(strings.ToLower(node.Name), query) ||
		strings.Contains(strings.ToLower(node.Info), query)
}

// nodeOrDescendantMatches returns true if this node or any descendant matches
// (checks ALL descendants, not just expanded, to preserve hierarchy context)
// Uses the matchCache to avoid recomputation
func (m *Model) nodeOrDescendantMatches(node *TreeNode) bool {
	if m.searchQuery == "" {
		return true
	}

	// Check cache first
	if m.matchCache != nil {
		if result, ok := m.matchCache[node]; ok {
			return result
		}
	}

	// Check if this node directly matches
	if m.nodeMatchesQuery(node) {
		if m.matchCache != nil {
			m.matchCache[node] = true
		}
		return true
	}

	// Check ALL children (not just expanded) to preserve hierarchy
	// Parent nodes stay visible if any descendant matches
	for _, child := range node.Children {
		if m.nodeOrDescendantMatches(child) {
			if m.matchCache != nil {
				m.matchCache[node] = true
			}
			return true
		}
	}

	if m.matchCache != nil {
		m.matchCache[node] = false
	}
	return false
}

// clearMatchCache clears the match cache (call when tree structure changes)
func (m *Model) clearMatchCache() {
	m.matchCache = make(map[*TreeNode]bool)
}

// openSpaceInBrowserCmd opens the space in the web browser
func openSpaceInBrowserCmd(spaceID string) tea.Cmd {
	return func() tea.Msg {
		// Get current context to find server URL
		listCmd := exec.Command("cub", "context", "list", "--json")
		listOutput, err := listCmd.Output()
		if err != nil {
			return statusUpdateMsg{msg: "Failed to get context info"}
		}

		var contexts []struct {
			Current    bool `json:"current"`
			Coordinate struct {
				ServerURL string `json:"serverURL"`
			} `json:"coordinate"`
		}
		if err := json.Unmarshal(listOutput, &contexts); err != nil {
			return statusUpdateMsg{msg: "Failed to parse context"}
		}

		// Find current context
		var serverURL string
		for _, ctx := range contexts {
			if ctx.Current {
				serverURL = ctx.Coordinate.ServerURL
				break
			}
		}
		if serverURL == "" {
			return statusUpdateMsg{msg: "No active context found"}
		}

		// Build URL: {serverURL}/spaces/{spaceID}
		url := fmt.Sprintf("%s/spaces/%s", serverURL, spaceID)

		// Open in browser (macOS) - best-effort, user sees URL in status anyway
		_ = exec.Command("open", url).Start() //nolint:errcheck // best-effort browser open

		return statusUpdateMsg{msg: fmt.Sprintf("Opening %s", url)}
	}
}

// statusUpdateMsg is used to update the status message
type statusUpdateMsg struct {
	msg string
}

func switchOrgCmd(orgID string) tea.Cmd {
	return func() tea.Msg {
		// First, list all contexts to find one with the target org
		listCmd := exec.Command("cub", "context", "list", "--json")
		listOutput, err := listCmd.Output()
		if err != nil {
			return authCompleteMsg{success: false, orgID: orgID}
		}

		// Parse contexts to find one matching the target org
		var contexts []struct {
			Name       string `json:"name"`
			Coordinate struct {
				ServerURL      string `json:"serverURL"`
				OrganizationID string `json:"organizationID"`
			} `json:"coordinate"`
		}
		if err := json.Unmarshal(listOutput, &contexts); err != nil {
			return authCompleteMsg{success: false, orgID: orgID}
		}

		// Find a context that has the target org (by ExternalID)
		var targetContext string
		for _, ctx := range contexts {
			if ctx.Coordinate.OrganizationID == orgID {
				targetContext = ctx.Name
				break
			}
		}

		if targetContext == "" {
			// No existing context for this org
			return authCompleteMsg{success: false, orgID: orgID}
		}

		// Switch to the found context
		useCmd := exec.Command("cub", "context", "use", targetContext)
		if err := useCmd.Run(); err != nil {
			return authCompleteMsg{success: false, orgID: orgID}
		}
		return authCompleteMsg{success: true, orgID: orgID}
	}
}

// Initialize model
func initialModel() Model {
	return initialModelWithContext("")
}

// initialModelWithContext creates a model with optional app context
// If appContext is provided, starts in Maps view filtered to that app
func initialModelWithContext(appContext string) Model {
	vp := viewport.New(40, 20) // Will be resized on WindowSizeMsg
	vp.MouseWheelEnabled = true

	// Initialize spinner with dot style
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))

	// Get current kubectl context for cluster filtering
	contextName := getCurrentContext()
	clusterName := extractClusterName(contextName)

	m := Model{
		keymap:         defaultKeyMap(),
		loading:        true,
		detailsPane:    vp,
		spinner:        s,
		contextName:    contextName,
		currentCluster: clusterName,
		showAllUnits:   false, // Default to showing only current cluster's units
	}

	// If context provided, start in Maps mode
	if appContext != "" {
		m.mapsMode = true
		m.searchQuery = appContext // Pre-fill search with app name
	} else {
		// Restore from snapshot if available (only when no explicit context)
		if snap := loadHubSnapshot(); snap != nil {
			m.cursor = snap.Cursor
			m.mapsMode = snap.MapsMode
			m.pendingSnapshot = snap // Save for expanded paths restoration after data loads
		}
	}

	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, loadDataCmd)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle auth prompt
		if m.authPrompt {
			switch msg.String() {
			case "y", "Y", "enter":
				m.authPrompt = false
				m.loading = true
				m.statusMsg = fmt.Sprintf("Switching to %s...", m.authOrgName)
				return m, switchOrgCmd(m.authOrgID)
			case "n", "N", "esc", "q":
				m.authPrompt = false
				m.authOrgName = ""
				m.authOrgID = ""
				return m, nil
			}
			return m, nil
		}

		// Handle import wizard mode
		if m.importMode {
			return m.updateImportWizard(msg)
		}

		// Handle create wizard mode
		if m.createMode {
			return m.updateCreateWizard(msg)
		}

		// Handle delete wizard mode
		if m.deleteMode {
			return m.updateDeleteWizard(msg)
		}

		// Handle org selection mode
		if m.orgSelectMode {
			orgs := m.getOrgList()
			switch msg.String() {
			case "esc", "O":
				m.orgSelectMode = false
				return m, nil
			case "j", "down":
				if m.orgSelectCursor < len(orgs)-1 {
					m.orgSelectCursor++
				}
				return m, nil
			case "k", "up":
				if m.orgSelectCursor > 0 {
					m.orgSelectCursor--
				}
				return m, nil
			case "enter":
				if m.orgSelectCursor < len(orgs) {
					selectedOrg := orgs[m.orgSelectCursor]
					if !m.isCurrentOrg(selectedOrg) {
						// Trigger org switch via auth prompt
						m.orgSelectMode = false
						m.authPrompt = true
						m.authOrgName = selectedOrg.DisplayName
						m.authOrgID = selectedOrg.ExternalID
					} else {
						m.orgSelectMode = false
						m.statusMsg = "Already in this organization"
					}
				}
				return m, nil
			}
			return m, nil
		}

		// Handle help overlay mode - dismiss on any key
		if m.helpMode {
			m.helpMode = false
			return m, nil
		}

		// Handle activity view mode - dismiss on any key
		if m.activityMode {
			m.activityMode = false
			return m, nil
		}

		// Handle maps view mode - dismiss on any key
		if m.mapsMode {
			m.mapsMode = false
			return m, nil
		}

		// Handle panel view mode - dismiss on escape or any key
		if m.panelMode {
			m.panelMode = false
			return m, nil
		}

		// Handle suggest view mode - dismiss on escape
		if m.suggestMode {
			m.suggestMode = false
			return m, nil
		}

		// Handle details pane focus mode - route keys to viewport
		if m.detailsFocused && !m.searchMode {
			switch msg.String() {
			case "j", "down":
				m.detailsPane.LineDown(1)
				return m, nil
			case "k", "up":
				m.detailsPane.LineUp(1)
				return m, nil
			case "d", "ctrl+d":
				m.detailsPane.HalfPageDown()
				return m, nil
			case "u", "ctrl+u":
				m.detailsPane.HalfPageUp()
				return m, nil
			case "g":
				m.detailsPane.GotoTop()
				return m, nil
			case "G":
				m.detailsPane.GotoBottom()
				return m, nil
			case "q":
				saveHubSnapshot(&m)
				return m, tea.Quit
			case "tab":
				m.detailsFocused = false
				return m, nil
			case "esc":
				m.detailsFocused = false
				return m, nil
			case "O", ":", "L", "?":
				// Global keys - fall through to main handler
				m.detailsFocused = false
			default:
				return m, nil
			}
		}

		if m.searchMode {
			switch msg.String() {
			case "esc":
				m.searchMode = false
				m.searchQuery = ""
				m.searchMatches = nil
				m.filterActive = false
				m.rebuildFlatList() // Rebuild to show all nodes again
				return m, nil
			case "enter":
				m.searchMode = false
				// Keep searchQuery, matches, and filter for n/N navigation
				return m, nil
			case "backspace":
				if len(m.searchQuery) > 0 {
					m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
					m.rebuildFlatList() // Rebuild to update filter
				}
				return m, nil
			default:
				if len(msg.String()) == 1 {
					m.searchQuery += msg.String()
					// Enable filter mode automatically when typing a search
					m.filterActive = true
					m.rebuildFlatList() // Rebuild to apply filter
				}
				return m, nil
			}
		}

		// Command palette mode
		if m.cmdMode {
			switch msg.String() {
			case "esc":
				m.cmdMode = false
				m.cmdInput = ""
				return m, nil
			case "enter":
				if m.cmdInput != "" {
					// Save to history
					m.cmdHistory = append([]string{m.cmdInput}, m.cmdHistory...)
					if len(m.cmdHistory) > 20 {
						m.cmdHistory = m.cmdHistory[:20]
					}
					// Execute command
					cmd := m.cmdInput
					m.cmdMode = false
					m.cmdRunning = true
					m.cmdShowOutput = true
					m.statusMsg = "Running: " + cmd
					return m, runCommandCmd(cmd)
				}
				m.cmdMode = false
				return m, nil
			case "backspace":
				if len(m.cmdInput) > 0 {
					m.cmdInput = m.cmdInput[:len(m.cmdInput)-1]
				}
				return m, nil
			case "up":
				// Navigate history
				if len(m.cmdHistory) > 0 && m.cmdHistoryIdx < len(m.cmdHistory)-1 {
					m.cmdHistoryIdx++
					m.cmdInput = m.cmdHistory[m.cmdHistoryIdx]
				}
				return m, nil
			case "down":
				if m.cmdHistoryIdx > 0 {
					m.cmdHistoryIdx--
					m.cmdInput = m.cmdHistory[m.cmdHistoryIdx]
				} else if m.cmdHistoryIdx == 0 {
					m.cmdHistoryIdx = -1
					m.cmdInput = ""
				}
				return m, nil
			default:
				if len(msg.String()) == 1 {
					m.cmdInput += msg.String()
				}
				return m, nil
			}
		}

		// Handle Esc to dismiss command output
		if msg.String() == "esc" && m.cmdShowOutput {
			m.cmdShowOutput = false
			m.cmdOutput = ""
			return m, nil
		}

		// Handle Esc to clear search when not in search mode
		if msg.String() == "esc" && m.searchQuery != "" {
			m.searchQuery = ""
			m.searchMatches = nil
			m.filterActive = false
			m.rebuildFlatList() // Rebuild to show all nodes again
			return m, nil
		}

		switch {
		case key.Matches(msg, m.keymap.Quit):
			saveHubSnapshot(&m)
			return m, tea.Quit

		case key.Matches(msg, m.keymap.Up):
			if m.cursor > 0 {
				m.cursor--
			}

		case key.Matches(msg, m.keymap.Down):
			if m.cursor < len(m.flatList)-1 {
				m.cursor++
			}

		case key.Matches(msg, m.keymap.Left):
			// Collapse current node or go to parent
			if m.cursor < len(m.flatList) {
				node := m.flatList[m.cursor]
				if node.Expanded && len(node.Children) > 0 {
					node.Expanded = false
					m.rebuildFlatList()
				} else if node.Parent != nil {
					for i, n := range m.flatList {
						if n == node.Parent {
							m.cursor = i
							break
						}
					}
				}
			}

		case key.Matches(msg, m.keymap.Right):
			// Right arrow: Expand node only (no details loading)
			if m.cursor < len(m.flatList) {
				node := m.flatList[m.cursor]

				// Check if this is an org that's not the current one
				if node.Type == "org" {
					orgData, ok := node.Data.(CubOrganization)
					if ok {
						isCurrentOrg := m.isCurrentOrg(orgData)
						if !isCurrentOrg {
							// Prompt to switch org
							m.authPrompt = true
							m.authOrgName = orgData.DisplayName
							m.authOrgID = orgData.ExternalID
							return m, nil
						}
					}
				}

				// Expand node
				if !node.Expanded {
					// Load data on-demand for groups
					if node.Type == "group" && len(node.Children) == 0 {
						// Find the space slug from parent
						if node.Parent != nil && node.Parent.Type == "space" {
							spaceSlug := node.Parent.ID
							m.loadGroupData(node, spaceSlug)
						}
					}
					// Load detail children for units
					if node.Type == "unit" && len(node.Children) == 0 {
						if unitData, ok := node.Data.(CubUnitData); ok {
							node.Children = buildUnitDetailChildren(unitData, node)
						}
					}
					node.Expanded = true
					m.rebuildFlatList()
				}
			}

		case key.Matches(msg, m.keymap.Enter):
			// Enter: Load entity details into right pane
			if m.cursor < len(m.flatList) {
				node := m.flatList[m.cursor]

				// Check if this is an org that's not the current one
				if node.Type == "org" {
					orgData, ok := node.Data.(CubOrganization)
					if ok {
						isCurrentOrg := m.isCurrentOrg(orgData)
						if !isCurrentOrg {
							// Prompt to switch org
							m.authPrompt = true
							m.authOrgName = orgData.DisplayName
							m.authOrgID = orgData.ExternalID
							return m, nil
						}
					}
				}

				// Load details for this node
				m.detailsLoading = true
				m.detailsError = nil
				m.detailsNode = node
				return m, loadEntityDetailsCmd(node)
			}

		case key.Matches(msg, m.keymap.Tab):
			// Tab: Switch focus to details pane
			m.detailsFocused = true
			return m, nil

		case key.Matches(msg, m.keymap.Search):
			m.searchMode = true
			m.searchQuery = ""
			m.searchMatches = nil

		case key.Matches(msg, m.keymap.NextMatch):
			m.nextSearchMatch()

		case key.Matches(msg, m.keymap.PrevMatch):
			m.prevSearchMatch()

		case key.Matches(msg, m.keymap.ToggleFilter):
			// Toggle filter mode (only useful when there's a search query)
			if m.searchQuery != "" {
				m.filterActive = !m.filterActive
				m.rebuildFlatList()
			}

		case key.Matches(msg, m.keymap.Command):
			// Enter command mode
			m.cmdMode = true
			m.cmdInput = ""
			m.cmdHistoryIdx = -1
			return m, nil

		case key.Matches(msg, m.keymap.SwitchOrg):
			// Enter org selection mode
			orgs := m.getOrgList()
			if len(orgs) > 1 {
				m.orgSelectMode = true
				// Set cursor to current org
				for i, org := range orgs {
					if m.isCurrentOrg(org) {
						m.orgSelectCursor = i
						break
					}
				}
			} else if len(orgs) == 1 {
				m.statusMsg = "Only one organization available"
			} else {
				m.statusMsg = "No organizations loaded - press r to refresh"
			}
			return m, nil

		case key.Matches(msg, m.keymap.Refresh):
			m.loading = true
			m.statusMsg = "Refreshing..."
			return m, loadDataCmd

		case key.Matches(msg, m.keymap.Help):
			m.helpMode = true
			return m, nil

		case key.Matches(msg, m.keymap.Activity):
			// Toggle between "this cluster" and "all units"
			m.showAllUnits = !m.showAllUnits
			// Rebuild flat list with new filter
			m.rebuildFlatList()
			// Show status message
			if m.showAllUnits {
				m.statusMsg = "Showing all units"
			} else {
				m.statusMsg = fmt.Sprintf("Showing units on cluster: %s", m.currentCluster)
			}
			return m, nil

		case key.Matches(msg, m.keymap.Maps):
			m.mapsMode = true
			return m, nil

		case key.Matches(msg, m.keymap.Panel):
			// Toggle panel view and load cluster data
			m.panelMode = true
			m.panelLoading = true
			// Collect unit slugs for correlation
			var unitSlugs []string
			for _, node := range m.nodes {
				if node.Type == "organization" {
					for _, spaceNode := range node.Children {
						if spaceNode.Type == "space" {
							for _, groupNode := range spaceNode.Children {
								if groupNode.Type == "units" {
									for _, unitNode := range groupNode.Children {
										if unitData, ok := unitNode.Data.(CubUnitData); ok {
											unitSlugs = append(unitSlugs, unitData.Unit.Slug)
										}
									}
								}
							}
						}
					}
				}
			}
			return m, loadPanelDataCmd(unitSlugs)

		case key.Matches(msg, m.keymap.Suggest):
			// Toggle suggest view and load cluster data
			m.suggestMode = true
			m.suggestLoading = true
			return m, loadSuggestDataCmd()

		case key.Matches(msg, m.keymap.HubView):
			// Toggle Hub/AppSpace view mode
			m.hubViewMode = !m.hubViewMode
			m.rebuildFlatList()
			if m.hubViewMode {
				m.statusMsg = "Hub/AppSpace view enabled"
			} else {
				m.statusMsg = "Standard view"
			}
			return m, nil

		case key.Matches(msg, m.keymap.LocalCluster):
			// Switch to local cluster TUI
			m.launchLocalCluster = true
			saveHubSnapshot(&m)
			return m, tea.Quit

		case key.Matches(msg, m.keymap.Import):
			// Launch the new import wizard (exits hierarchy, runs wizard, returns)
			m.launchImportWizard = true
			saveHubSnapshot(&m)
			return m, tea.Quit

		case key.Matches(msg, m.keymap.Create):
			// Start the create wizard
			m.createMode = true
			m.createStep = createStepSelectType
			m.createType = ""
			m.createName = ""
			m.createSpace = m.getSpaceFromSelection()
			m.createCloneFrom = ""
			m.createTarget = ""
			m.createWorker = ""
			m.createProvider = "Kubernetes"
			m.createCursor = 0
			m.createLoading = false
			m.createError = nil
			return m, nil

		case key.Matches(msg, m.keymap.Delete):
			// Start the delete wizard if on a deletable node
			if m.cursor < len(m.flatList) {
				node := m.flatList[m.cursor]
				switch node.Type {
				case "space":
					m.deleteMode = true
					m.deleteStep = deleteStepConfirm
					m.deleteType = "space"
					m.deleteName = node.ID
					m.deleteSpace = ""
					m.deleteCursor = 1 // Default to "No" for safety
					m.deleteLoading = false
					m.deleteError = nil
					return m, nil
				case "unit":
					m.deleteMode = true
					m.deleteStep = deleteStepConfirm
					m.deleteType = "unit"
					m.deleteName = node.ID
					m.deleteSpace = m.getSpaceFromNode(node)
					m.deleteCursor = 1 // Default to "No" for safety
					m.deleteLoading = false
					m.deleteError = nil
					return m, nil
				case "target":
					m.deleteMode = true
					m.deleteStep = deleteStepConfirm
					m.deleteType = "target"
					m.deleteName = node.ID
					m.deleteSpace = m.getSpaceFromNode(node)
					m.deleteCursor = 1 // Default to "No" for safety
					m.deleteLoading = false
					m.deleteError = nil
					return m, nil
				default:
					// Can't delete orgs, groups, workers, or details
					m.statusMsg = "Cannot delete this type of resource"
				}
			}

		case key.Matches(msg, m.keymap.OpenWeb):
			// Open the current space in the web browser
			if m.cursor < len(m.flatList) {
				node := m.flatList[m.cursor]
				spaceID := m.getSpaceIDFromNode(node)
				if spaceID != "" {
					return m, openSpaceInBrowserCmd(spaceID)
				}
				m.statusMsg = "No space selected"
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

		// Calculate pane widths (50/50 split, minus borders)
		rightWidth := (m.width / 2) - 4 // Account for borders and gap
		viewportHeight := m.height - 8  // Account for header, breadcrumb, help text

		// Resize viewport
		m.detailsPane.Width = rightWidth
		m.detailsPane.Height = viewportHeight

		// Set initial org summary if no content yet
		if m.detailsContent == "" && len(m.nodes) > 0 {
			m.detailsContent = m.buildOrgSummary()
			m.detailsPane.SetContent(m.detailsContent)
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case dataLoadedMsg:
		m.nodes = msg.nodes
		m.currentOrg = msg.currentOrg
		m.currentOrgInt = msg.currentOrgInt
		m.loading = false
		m.statusMsg = ""

		// Restore expanded paths from snapshot if pending
		if m.pendingSnapshot != nil && len(m.pendingSnapshot.ExpandedPaths) > 0 {
			expandedSet := make(map[string]bool)
			for _, p := range m.pendingSnapshot.ExpandedPaths {
				expandedSet[p] = true
			}
			var restoreExpanded func(nodes []*TreeNode, path string)
			restoreExpanded = func(nodes []*TreeNode, path string) {
				for _, n := range nodes {
					nodePath := path + "/" + n.Name
					if expandedSet[nodePath] {
						n.Expanded = true
						restoreExpanded(n.Children, nodePath)
					}
				}
			}
			restoreExpanded(m.nodes, "")
			m.pendingSnapshot = nil // Clear after restoration
		}

		m.rebuildFlatList()

		// Auto-focus on current space if set
		if msg.currentSpace != "" {
			// Find and expand to the current space
			for i, node := range m.flatList {
				if node.Type == "space" && node.ID == msg.currentSpace {
					// Expand parent org first
					if node.Parent != nil {
						node.Parent.Expanded = true
						m.rebuildFlatList()
					}
					// Find new position after rebuild
					for j, n := range m.flatList {
						if n.Type == "space" && n.ID == msg.currentSpace {
							m.cursor = j
							// Show space summary in details
							m.detailsContent = m.buildSpaceSummary(n)
							m.detailsPane.SetContent(m.detailsContent)
							break
						}
					}
					break
				}
				// Also match if it's the org with this as default
				_ = i
			}
		}

		// Set initial org summary in details pane (fallback if no current space)
		if m.detailsContent == "" {
			m.detailsContent = m.buildOrgSummary()
			m.detailsPane.SetContent(m.detailsContent)
		}

		// Trigger background loading for all spaces
		if len(msg.spacesToLoad) > 0 {
			var cmds []tea.Cmd
			for _, spaceSlug := range msg.spacesToLoad {
				cmds = append(cmds, loadSpaceDataCmd(spaceSlug))
			}
			return m, tea.Batch(cmds...)
		}

	case spaceDataLoadedMsg:
		// Update the tree in place without resetting cursor or expanded state
		if msg.err == nil {
			m.updateSpaceData(msg.spaceSlug, msg.units, msg.targets, msg.workers)
			m.rebuildFlatList()
		}

	case panelDataLoadedMsg:
		m.panelLoading = false
		if msg.err != nil {
			m.panelError = msg.err
		} else {
			m.panelWorkloads = msg.workloads
			m.panelCorrelation = msg.correlation
			m.panelOrphans = msg.orphans
			m.panelError = nil
		}

	case suggestDataLoadedMsg:
		m.suggestLoading = false
		if msg.err != nil {
			m.suggestError = msg.err
		} else {
			m.suggestProposal = msg.proposal
			m.suggestError = nil
		}

	case detailsLoadedMsg:
		m.detailsLoading = false
		if msg.err != nil {
			m.detailsError = msg.err
			m.detailsContent = fmt.Sprintf("Error: %v", msg.err)
		} else {
			m.detailsNode = msg.node
			// Apply syntax highlighting to JSON
			m.detailsContent = m.formatJSONWithSyntaxHighlight(msg.content)
		}
		m.detailsPane.SetContent(m.detailsContent)
		m.detailsPane.GotoTop()

	case authCompleteMsg:
		if msg.success {
			m.statusMsg = "Switched org, reloading..."
			return m, loadDataCmd
		} else {
			m.loading = false
			m.statusMsg = "Failed to switch organization"
		}

	case statusUpdateMsg:
		m.statusMsg = msg.msg

	case errMsg:
		m.err = msg.err
		m.loading = false

	// Import wizard messages
	case namespacesLoadedMsg:
		m.importLoading = false
		if msg.err != nil {
			m.importError = msg.err
			return m, nil
		}
		m.importNamespaces = msg.namespaces

	case argoAppsLoadedMsg:
		m.importLoading = false
		if msg.err != nil {
			m.importError = msg.err
			return m, nil
		}
		m.importArgoApps = msg.apps

	case argoResourcesLoadedMsg:
		m.importLoading = false
		if msg.err != nil {
			m.importError = msg.err
			return m, nil
		}
		m.importSelectedArgo = msg.app
		m.importArgoResources = msg.resources
		// Convert ArgoCD resources to WorkloadInfo for selection step
		// This reuses the existing workload selection UI
		m.importWorkloads = nil
		for _, res := range msg.resources {
			// Clean the YAML to remove runtime fields (same as CLI does)
			cleanedYAML := res.YAML
			if cleaned, err := cleanResourceYAML(res.YAML); err == nil {
				cleanedYAML = cleaned
			}
			w := WorkloadInfo{
				Name:            res.Name,
				Kind:            res.Kind,
				Namespace:       res.Namespace,
				Owner:           "ArgoCD", // We know it's managed by ArgoCD
				ExtractedConfig: cleanedYAML,
			}
			// Set GitOps source from ArgoCD Application
			if msg.app != nil {
				w.SourceRepo = msg.app.RepoURL
				w.SourcePath = msg.app.Path
				w.GitOpsRef = &GitOpsReference{
					Kind:      "Application",
					Name:      msg.app.Name,
					Namespace: msg.app.Namespace,
				}
			}
			m.importWorkloads = append(m.importWorkloads, w)
		}
		m.importSelected = make([]bool, len(m.importWorkloads))
		// Default: select all resources
		for i := range m.importSelected {
			m.importSelected[i] = true
		}
		m.importCursor = 0
		// Move to setup step (reuses existing flow)
		m.importStep = importStepSetup
		m.importLoading = true
		return m, loadExistingSpacesCmd()

	case workloadsDiscoveredMsg:
		m.importLoading = false
		m.importStep = importStepSelection
		if msg.err != nil {
			m.importError = msg.err
			return m, nil
		}
		m.importWorkloads = msg.workloads
		m.importSelected = make([]bool, len(msg.workloads))
		m.importCursor = 0

		// Generate smart suggestions from labels and namespace patterns
		suggestion := SuggestStructure(msg.workloads, m.importSpace)
		m.importSuggestion = &suggestion
		m.importGroupedView = true // Default to grouped view

	case importCompleteMsg:
		m.importProgress = msg.success
		m.importTotal = msg.success + msg.failed
		m.importApplyError = msg.applyError // May be nil if apply succeeded
		// For ArgoCD imports, transition to cleanup step to offer disable/delete
		if m.importSource == importSourceArgoCD && m.importSelectedArgo != nil {
			m.importStep = importStepArgoCleanup
			m.importCursor = argoCleanupKeepAsIs // Default to keep as-is
		} else {
			// For non-ArgoCD imports, offer test step
			m.importStep = importStepTest
			m.importCursor = testOptionSkip // Default to skip
		}

	case argoSyncDisabledMsg:
		m.importLoading = false
		if msg.err != nil {
			m.importError = msg.err
			// Cleanup failed - go to test step but warn that ArgoCD still controls resources
			m.importStep = importStepTest
			m.importCursor = testOptionSkip // Default to skip
			return m, nil
		}
		// Cleanup succeeded - now apply the unit (ArgoCD won't revert it anymore)
		if m.importSelectedArgo != nil {
			unitSlug := fmt.Sprintf("%s-workload", m.importSelectedArgo.Name)
			m.importLoading = true
			return m, applyUnitCmd(m.importSpace, unitSlug, m.getSelectedWorkloads())
		}
		// Fallback: go to test step
		m.importStep = importStepTest
		m.importCursor = testOptionSkip

	case argoAppDeletedMsg:
		m.importLoading = false
		if msg.err != nil {
			m.importError = msg.err
			// Cleanup failed - go to test step but warn
			m.importStep = importStepTest
			m.importCursor = testOptionSkip
			return m, nil
		}
		// Cleanup succeeded - now apply the unit
		if m.importSelectedArgo != nil {
			unitSlug := fmt.Sprintf("%s-workload", m.importSelectedArgo.Name)
			m.importLoading = true
			return m, applyUnitCmd(m.importSpace, unitSlug, m.getSelectedWorkloads())
		}
		// Fallback: go to test step
		m.importStep = importStepTest
		m.importCursor = testOptionSkip

	case unitAppliedMsg:
		m.importLoading = false
		if msg.err != nil {
			m.importError = msg.err
		}
		// TODO: Re-enable test step after fixing apply issues
		// Skip test step for now - go directly to complete
		m.importStep = importStepComplete

	case testUpdateCompleteMsg:
		m.importLoading = false
		m.importTestResult = msg.result
		if msg.err != nil {
			m.importError = msg.err
		}
		m.importStep = importStepComplete

	case configExtractedMsg:
		m.importLoading = false
		m.importExtractDone = true
		m.importExtractSuccess = msg.success
		// Update workloads with extracted config
		// msg.workloads contains only selected workloads, so we need to match by name
		for _, extracted := range msg.workloads {
			for i := range m.importWorkloads {
				if m.importWorkloads[i].Name == extracted.Name && m.importWorkloads[i].Namespace == extracted.Namespace {
					m.importWorkloads[i].ExtractedConfig = extracted.ExtractedConfig
					m.importWorkloads[i].ConfigError = extracted.ConfigError
					break
				}
			}
		}
		m.importCursor = 0

	// Setup wizard messages
	case spacesLoadedMsg:
		m.importLoading = false
		if msg.err != nil {
			m.importError = msg.err
			return m, nil
		}
		m.importExistingSpaces = msg.spaces

	case spaceCreatedMsg:
		m.importLoading = false
		if msg.err != nil {
			m.importError = msg.err
			return m, nil
		}
		m.importSpace = msg.space
		// After creating space, move to worker creation (worker needed before target)
		m.importStep = importStepCreateWorker
		m.importCursor = 0

	case workersLoadedMsg:
		m.importLoading = false
		if msg.err != nil {
			m.importError = msg.err
			return m, nil
		}
		m.importExistingWorkers = msg.workers
		// Find a running worker (Condition == "Ready")
		for _, w := range msg.workers {
			if w.Condition == "Ready" {
				m.importSelectedWorker = w.Slug
				m.importNewWorkerName = w.Slug // Use existing worker
				// We have a running worker - wait for target to be auto-created
				m.importStep = importStepWaitTarget
				m.statusMsg = "Waiting for target to be created..."
				kubeContext := getCurrentKubeContext()
				return m, waitForTargetCmd(m.importSpace, w.Slug, kubeContext)
			}
		}
		// No running worker found - need to create one
		m.importStep = importStepCreateWorker

	case workerCreatedMsg:
		m.importLoading = false
		if msg.err != nil {
			m.importError = msg.err
			return m, nil
		}
		// Worker created - now start it in background
		m.importLoading = true
		m.statusMsg = "Starting worker..."
		return m, startWorkerCmd(m.importSpace, m.importNewWorkerName)

	case workerStartedMsg:
		if msg.err != nil {
			m.importLoading = false
			m.importError = msg.err
			return m, nil
		}
		// Worker process started - wait for it to become ready
		m.statusMsg = "Waiting for worker to be ready..."
		return m, waitForWorkerReadyCmd(m.importSpace, m.importNewWorkerName)

	case workerReadyMsg:
		m.statusMsg = ""
		if msg.err != nil {
			// Worker didn't become ready in time - still proceed but skip target
			m.importError = nil // Clear this - we'll show manual instructions
			// For ArgoCD imports, we already have workloads loaded - skip discovery
			if m.importSource == importSourceArgoCD && len(m.importWorkloads) > 0 {
				m.importStep = importStepSelection
				m.importCursor = 0
				m.importLoading = false
				return m, nil
			}
			m.importStep = importStepDiscovering
			m.importLoading = true
			return m, discoverWorkloadsCmd(m.importNamespace)
		}
		// Worker is ready! Now wait for target to be auto-created
		m.importSelectedWorker = msg.worker // Mark that we have a running worker
		m.importStep = importStepWaitTarget
		m.statusMsg = "Waiting for target to be created..."
		kubeContext := getCurrentKubeContext()
		return m, waitForTargetCmd(m.importSpace, m.importNewWorkerName, kubeContext)

	case targetFoundMsg:
		m.importLoading = false
		m.statusMsg = ""
		if msg.err != nil {
			// Target wasn't found - show error
			m.importError = fmt.Errorf("target not found: %w", msg.err)
			return m, nil
		}
		// Target found! Store the auto-created target name
		if msg.target != "" {
			m.importNewTargetName = msg.target
		} else {
			// This shouldn't happen, but guard against it
			m.importError = fmt.Errorf("target lookup returned empty result")
			return m, nil
		}
		// For ArgoCD imports, we already have workloads loaded - skip discovery
		if m.importSource == importSourceArgoCD && len(m.importWorkloads) > 0 {
			m.importStep = importStepSelection
			m.importCursor = 0
			return m, nil
		}
		// For Kubernetes imports, discover workloads in the namespace
		m.importStep = importStepDiscovering
		m.importLoading = true
		return m, discoverWorkloadsCmd(m.importNamespace)

	// Command palette messages
	case cmdCompleteMsg:
		m.cmdRunning = false
		if msg.err != nil {
			m.cmdOutput = fmt.Sprintf("Error: %v\n%s", msg.err, msg.output)
		} else {
			m.cmdOutput = msg.output
		}
		m.statusMsg = ""
		return m, nil

	case workersStatusMsg:
		m.workersLoaded = true
		if msg.err == nil {
			m.workers = msg.workers
		}
		return m, nil

	// Create wizard messages
	case createUnitsLoadedMsg:
		m.createLoading = false
		if msg.err != nil {
			m.createError = msg.err
			return m, nil
		}
		m.createUnits = msg.units

	case createTargetsLoadedMsg:
		m.createLoading = false
		if msg.err != nil {
			m.createError = msg.err
			return m, nil
		}
		m.createTargets = msg.targets

	case createWorkersLoadedMsg:
		m.createLoading = false
		if msg.err != nil {
			m.createError = msg.err
			return m, nil
		}
		m.createWorkers = msg.workers

	case createResourceMsg:
		m.createLoading = false
		// Remove pending action (whether success or failure)
		m.removePendingAction(msg.resourceType, msg.name)
		if msg.err != nil {
			// Failed - show error and rebuild to remove the pending node
			m.statusMsg = fmt.Sprintf("Failed to create %s '%s': %v", msg.resourceType, msg.name, msg.err)
			m.rebuildFlatList()
			return m, nil
		}
		// Success - insert the real node into the tree
		m.insertCreatedNode(msg.resourceType, msg.name, msg.space)
		m.statusMsg = fmt.Sprintf("%s '%s' created successfully", msg.resourceType, msg.name)
		m.rebuildFlatList()
		return m, nil

	case deleteResourceMsg:
		m.deleteLoading = false
		// Remove pending action (whether success or failure)
		m.removePendingAction(msg.resourceType, msg.name)
		if msg.err != nil {
			// Failed - show error and rebuild to restore the node
			m.statusMsg = fmt.Sprintf("Failed to delete %s '%s': %v", msg.resourceType, msg.name, msg.err)
			m.rebuildFlatList()
			return m, nil
		}
		// Success - remove the node from the tree structure
		m.removeNodeFromTree(msg.resourceType, msg.name, msg.space)
		m.statusMsg = fmt.Sprintf("%s '%s' deleted successfully", msg.resourceType, msg.name)
		m.rebuildFlatList()
		return m, nil
	}

	return m, nil
}

func (m *Model) loadGroupData(groupNode *TreeNode, spaceSlug string) {
	switch {
	case strings.HasSuffix(groupNode.ID, "/units"):
		units, err := loadUnitsForSpace(spaceSlug)
		if err == nil {
			for _, unit := range units {
				unitNode := &TreeNode{
					ID:     unit.Unit.Slug,
					Name:   unit.Unit.Slug,
					Type:   "unit",
					Parent: groupNode,
					Data:   unit,
					OrgID:  groupNode.OrgID,
				}
				unitNode.Status = unit.DeriveStatus()
				unitNode.Info = buildUnitInfo(unit)
				groupNode.Children = append(groupNode.Children, unitNode)
			}
		}

	case strings.HasSuffix(groupNode.ID, "/targets"):
		targets, err := loadTargetsForSpace(spaceSlug)
		if err == nil {
			for _, target := range targets {
				targetNode := &TreeNode{
					ID:     target.Target.Slug,
					Name:   target.Target.Slug,
					Type:   "target",
					Status: "ok",
					Info:   target.Target.ProviderType,
					Parent: groupNode,
					Data:   target,
					OrgID:  groupNode.OrgID,
				}
				groupNode.Children = append(groupNode.Children, targetNode)
			}
		}

	case strings.HasSuffix(groupNode.ID, "/workers"):
		workers, err := loadWorkersForSpace(spaceSlug)
		if err == nil {
			for _, worker := range workers {
				workerNode := &TreeNode{
					ID:     worker.BridgeWorker.Slug,
					Name:   worker.BridgeWorker.Slug,
					Type:   "worker",
					Status: worker.DeriveStatus(),
					Info:   worker.BridgeWorker.Condition,
					Parent: groupNode,
					Data:   worker,
					OrgID:  groupNode.OrgID,
				}
				groupNode.Children = append(groupNode.Children, workerNode)
			}
		}
	}
}

// Import wizard methods
func (m *Model) updateImportWizard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "o":
		// Open space in browser on complete step
		if m.importStep == importStepComplete {
			url := m.getSpaceURL()
			if url != "" {
				_ = exec.Command("open", url).Start() //nolint:errcheck // best-effort browser open
				m.statusMsg = fmt.Sprintf("Opening %s", url)
			}
			return m, nil
		}
	case "esc":
		// Handle Esc based on current state
		if m.importViewingConfig {
			// Exit config view, return to list
			m.importViewingConfig = false
			return m, nil
		}
		if m.importStep == importStepExtractConfig {
			// Go back to selection step
			m.importStep = importStepSelection
			m.importExtractDone = false
			m.importCursor = 0
			return m, nil
		}
		// Default: exit import mode
		m.importMode = false
		m.resetImportState()
		return m, nil
	case "up", "k":
		if m.importCursor > 0 {
			m.importCursor--
			// Clear App of Apps warning when navigating
			if m.importStep == importStepArgoApps {
				m.importError = nil
			}
		}
	case "down", "j":
		max := m.getImportMaxCursor()
		if m.importCursor < max {
			m.importCursor++
			// Clear App of Apps warning when navigating
			if m.importStep == importStepArgoApps {
				m.importError = nil
			}
		}
	case " ":
		if m.importStep == importStepSelection {
			if m.importGroupedView && m.importSuggestion != nil {
				// In grouped view, cursor maps to a flat list of unit headers + workloads
				// We need to determine what the cursor is pointing at and toggle accordingly
				m.toggleGroupedSelection()
			} else if m.importCursor < len(m.importSelected) {
				m.importSelected[m.importCursor] = !m.importSelected[m.importCursor]
			}
		}
	case "a":
		if m.importStep == importStepNamespace {
			// Toggle show all namespaces
			m.importShowAllNS = !m.importShowAllNS
			m.importCursor = 0 // Reset cursor when toggling
		} else if m.importStep == importStepSelection {
			for i := range m.importSelected {
				m.importSelected[i] = true
			}
		}
	case "g":
		// Toggle grouped/flat view in selection step
		if m.importStep == importStepSelection {
			m.importGroupedView = !m.importGroupedView
			m.importCursor = 0 // Reset cursor when switching views
		}
	case "v":
		// View full config in ExtractConfig step
		if m.importStep == importStepExtractConfig && !m.importViewingConfig {
			selected := m.getSelectedWorkloads()
			if m.importCursor < len(selected) {
				m.importViewingConfig = true
				// Find the actual index in importWorkloads
				for i, w := range m.importWorkloads {
					if m.importSelected[i] && w.Name == selected[m.importCursor].Name {
						m.importViewConfigIdx = i
						break
					}
				}
			}
		}
	case "backspace":
		// Handle text input for space/worker names
		if m.importStep == importStepCreateSpace && len(m.importNewSpaceName) > 0 {
			m.importNewSpaceName = m.importNewSpaceName[:len(m.importNewSpaceName)-1]
		} else if m.importStep == importStepCreateWorker && len(m.importNewWorkerName) > 0 {
			m.importNewWorkerName = m.importNewWorkerName[:len(m.importNewWorkerName)-1]
		}
		// Note: Targets are auto-named, no user input needed
	case "enter":
		return m.advanceImportStep()
	default:
		// Handle text input for space/worker names
		if len(msg.String()) == 1 && msg.String() != " " {
			if m.importStep == importStepCreateSpace {
				m.importNewSpaceName += msg.String()
			} else if m.importStep == importStepCreateWorker {
				m.importNewWorkerName += msg.String()
			}
			// Note: Targets are auto-named, no user input needed
		}
	}
	return m, nil
}

func (m *Model) getImportMaxCursor() int {
	switch m.importStep {
	case importStepSource:
		return 1 // 0 = Kubernetes, 1 = ArgoCD
	case importStepArgoApps:
		if len(m.importArgoApps) == 0 {
			return 0
		}
		return len(m.importArgoApps) - 1
	case importStepNamespace:
		filtered := getFilteredNamespaces(m.importNamespaces, m.importShowAllNS)
		if len(filtered) == 0 {
			return 0
		}
		return len(filtered) - 1
	case importStepSetup:
		// Options: 0 = use existing space, 1 = create new space
		return 1 + len(m.importExistingSpaces) - 1 // "Create new" + existing spaces
	case importStepSelection:
		if m.importGroupedView && m.importSuggestion != nil {
			// Count lines in grouped view: unit headers + workloads
			return m.getGroupedViewLineCount() - 1
		}
		return len(m.importWorkloads) - 1
	case importStepUnitStructure:
		return 1 // 0 = combined, 1 = individual
	case importStepExtractConfig:
		selected := m.getSelectedWorkloads()
		if len(selected) > 0 {
			return len(selected) - 1
		}
		return 0
	case importStepArgoCleanup:
		return 2 // 0 = disable sync, 1 = delete app, 2 = keep as-is
	case importStepTest:
		return 2 // 0 = annotation, 1 = rollout, 2 = skip
	default:
		return 0
	}
}

// getGroupedViewLineCount counts total navigable lines in grouped view
func (m *Model) getGroupedViewLineCount() int {
	if m.importSuggestion == nil {
		return 0
	}

	// Build workload lookup for checking which are new
	workloadIdx := make(map[string]int)
	for i, w := range m.importWorkloads {
		workloadIdx[w.Namespace+"/"+w.Name] = i
	}

	count := 0
	for _, app := range m.importSuggestion.Apps {
		for _, variant := range app.Variants {
			hasNewWorkloads := false
			for _, w := range variant.Workloads {
				if idx, ok := workloadIdx[w.Namespace+"/"+w.Name]; ok {
					if m.importWorkloads[idx].UnitSlug == "" {
						hasNewWorkloads = true
						count++ // Count workload line
					}
				}
			}
			if hasNewWorkloads {
				count++ // Count unit header line
			}
		}
	}
	return count
}

// toggleGroupedSelection toggles selection for the item at cursor in grouped view
func (m *Model) toggleGroupedSelection() {
	if m.importSuggestion == nil {
		return
	}

	// Build workload lookup
	workloadIdx := make(map[string]int)
	for i, w := range m.importWorkloads {
		workloadIdx[w.Namespace+"/"+w.Name] = i
	}

	lineNum := 0
	for _, app := range m.importSuggestion.Apps {
		for _, variant := range app.Variants {
			// Collect new workloads in this variant
			var variantWorkloadIndices []int
			for _, w := range variant.Workloads {
				if idx, ok := workloadIdx[w.Namespace+"/"+w.Name]; ok {
					if m.importWorkloads[idx].UnitSlug == "" {
						variantWorkloadIndices = append(variantWorkloadIndices, idx)
					}
				}
			}

			if len(variantWorkloadIndices) == 0 {
				continue
			}

			// Unit header line
			if lineNum == m.importCursor {
				// Toggle all workloads in this variant
				// Determine current state (all selected = toggle off, otherwise toggle on)
				allSelected := true
				for _, idx := range variantWorkloadIndices {
					if !m.importSelected[idx] {
						allSelected = false
						break
					}
				}
				for _, idx := range variantWorkloadIndices {
					m.importSelected[idx] = !allSelected
				}
				return
			}
			lineNum++

			// Workload lines
			for _, idx := range variantWorkloadIndices {
				if lineNum == m.importCursor {
					m.importSelected[idx] = !m.importSelected[idx]
					return
				}
				lineNum++
			}
		}
	}
}

func (m *Model) advanceImportStep() (tea.Model, tea.Cmd) {
	switch m.importStep {
	case importStepSource:
		// cursor 0 = Kubernetes namespace, cursor 1 = ArgoCD
		if m.importCursor == 0 {
			m.importSource = importSourceKubernetes
			m.importStep = importStepNamespace
			m.importCursor = 0
			m.importLoading = true
			return m, loadNamespacesCmd()
		} else {
			m.importSource = importSourceArgoCD
			m.importStep = importStepArgoApps
			m.importCursor = 0
			m.importLoading = true
			return m, loadArgoAppsCmd()
		}

	case importStepArgoApps:
		if m.importCursor < len(m.importArgoApps) {
			selectedApp := m.importArgoApps[m.importCursor]
			// Warn about App of Apps
			if selectedApp.IsAppOfApps {
				m.importError = fmt.Errorf("'%s' is an App of Apps that manages %d child Applications.\n\nApp of Apps don't contain workloads directly - import the child apps instead:\n  %s",
					selectedApp.Name, len(selectedApp.ChildApps), strings.Join(selectedApp.ChildApps, ", "))
				return m, nil
			}
			m.importLoading = true
			// Default the names to the ArgoCD app name
			m.importNewSpaceName = selectedApp.Name
			m.importNewTargetName = selectedApp.DestNS
			m.importNewWorkerName = selectedApp.Name + "-worker"
			m.importNamespace = selectedApp.DestNS // For display
			return m, loadArgoResourcesCmd(selectedApp)
		}

	case importStepNamespace:
		filtered := getFilteredNamespaces(m.importNamespaces, m.importShowAllNS)
		if m.importCursor < len(filtered) {
			m.importNamespace = filtered[m.importCursor].Name
			// Move to setup step to choose/create space
			m.importStep = importStepSetup
			m.importCursor = 0
			m.importLoading = true
			// Default the new space/target/worker names to the namespace
			m.importNewSpaceName = m.importNamespace
			m.importNewTargetName = m.importNamespace
			m.importNewWorkerName = m.importNamespace + "-worker"
			return m, loadExistingSpacesCmd()
		}

	case importStepSetup:
		// cursor 0 = "Create new space", cursor > 0 = select existing space
		if m.importCursor == 0 {
			// Create new space - move to space creation step
			m.importCreateNewSpace = true
			m.importStep = importStepCreateSpace
			m.importCursor = 0
		} else {
			// Use existing space - check if it has a running worker
			m.importCreateNewSpace = false
			selectedIdx := m.importCursor - 1
			if selectedIdx < len(m.importExistingSpaces) {
				m.importSpace = m.importExistingSpaces[selectedIdx]
			}
			// Load workers for this space to see if any are running
			m.importLoading = true
			return m, loadWorkersForSpaceCmd(m.importSpace)
		}

	case importStepCreateSpace:
		// Create the new space
		if m.importNewSpaceName != "" {
			m.importLoading = true
			return m, createSpaceCmd(m.importNewSpaceName)
		}

	case importStepCreateWorker:
		// Create the worker
		if m.importNewWorkerName != "" {
			m.importLoading = true
			return m, createWorkerCmd(m.importSpace, m.importNewWorkerName)
		}

	// Note: importStepWaitTarget is handled automatically via message flow
	// (workerReadyMsg → waitForTargetCmd → targetFoundMsg)
	// No user interaction needed

	case importStepSelection:
		selected := m.getSelectedWorkloads()
		if len(selected) > 0 {
			// For ArgoCD imports, always use combined structure (1 Unit per Application)
			// per Hub/AppSpace/Unit model - skip the unit structure step
			if m.importSource == importSourceArgoCD {
				m.importUnitStructure = unitStructureCombined
				m.importStep = importStepExtractConfig
				m.importExtractDone = true
				m.importExtractSuccess = len(selected)
				m.importTotal = len(selected)
				m.importCursor = 0
				return m, nil
			}
			// For Kubernetes imports, extract GitOps configs first
			m.importStep = importStepExtractConfig
			m.importLoading = true
			m.importExtractDone = false
			m.importTotal = len(selected)
			return m, extractConfigCmd(selected)
		}

	case importStepExtractConfig:
		// User confirmed extracted configs, proceed to import
		selected := m.getSelectedWorkloads()
		if len(selected) > 0 {
			m.importStep = importStepImporting
			m.importTotal = len(selected)
			// ArgoCD imports: use chosen unit structure
			if m.importSource == importSourceArgoCD && m.importSelectedArgo != nil {
				if m.importUnitStructure == unitStructureCombined {
					return m, importArgoWorkloadsCmd(m.importSpace, m.importSelectedArgo.Name, m.importNewTargetName, selected)
				}
				// Individual units - use standard import
				return m, importWorkloadsCmd(m.importSpace, selected)
			}
			// Use suggestion-based import if in grouped mode, otherwise use flat import
			if m.importGroupedView && m.importSuggestion != nil {
				return m, importWorkloadsWithSuggestionCmd(m.importSpace, m.importSuggestion, selected)
			}
			return m, importWorkloadsCmd(m.importSpace, selected)
		}

	case importStepArgoCleanup:
		// Handle ArgoCD cleanup choice
		m.importArgoCleanup = m.importCursor
		if m.importSelectedArgo != nil {
			switch m.importCursor {
			case argoCleanupDisableSync:
				m.importLoading = true
				return m, disableArgoSyncCmd(m.importSelectedArgo.Namespace, m.importSelectedArgo.Name)
			case argoCleanupDeleteApp:
				m.importLoading = true
				return m, deleteArgoAppCmd(m.importSelectedArgo.Namespace, m.importSelectedArgo.Name)
			case argoCleanupKeepAsIs:
				// Skip cleanup but still apply the unit
				// Note: ArgoCD may revert changes if selfHeal is enabled
				unitSlug := fmt.Sprintf("%s-workload", m.importSelectedArgo.Name)
				m.importLoading = true
				return m, applyUnitCmd(m.importSpace, unitSlug, m.getSelectedWorkloads())
			}
		}
		m.importStep = importStepTest
		m.importCursor = testOptionSkip
		return m, nil

	case importStepTest:
		// Handle test pipeline choice
		m.importTestChoice = m.importCursor
		// Get the workload unit slug for testing
		var unitSlug string
		if m.importSource == importSourceArgoCD && m.importSelectedArgo != nil {
			unitSlug = fmt.Sprintf("%s-workload", m.importSelectedArgo.Name)
		} else if len(m.importWorkloads) > 0 {
			// Use the first selected workload
			for i, selected := range m.importSelected {
				if selected {
					unitSlug = m.importWorkloads[i].Name
					break
				}
			}
		}
		switch m.importCursor {
		case testOptionTest:
			if unitSlug != "" {
				m.importLoading = true
				m.importTestRan = true
				return m, testPipelineCmd(m.importSpace, unitSlug)
			}
			m.importStep = importStepComplete
		case testOptionSkip:
			m.importStep = importStepComplete
		}
		return m, nil

	case importStepComplete:
		// Enter closes the wizard
		m.importMode = false
		m.resetImportState()
		return m, loadDataCmd
	}
	return m, nil
}

func (m *Model) getSpaceFromSelection() string {
	if m.cursor >= len(m.flatList) {
		return ""
	}
	return m.getSpaceFromNode(m.flatList[m.cursor])
}

func (m *Model) getSpaceFromNode(node *TreeNode) string {
	for node != nil {
		if node.Type == "space" {
			return node.ID
		}
		node = node.Parent
	}
	return ""
}

func (m *Model) getSpaceIDFromNode(node *TreeNode) string {
	// Walk nodes to find the space node and get its Data
	for node != nil {
		if node.Type == "space" {
			if spaceData, ok := node.Data.(CubSpaceData); ok {
				return spaceData.Space.SpaceID
			}
			break
		}
		node = node.Parent
	}
	return ""
}

// getSpaceURL returns the URL to open the current import space in browser
func (m *Model) getSpaceURL() string {
	// Get server URL from current context
	ctxCmd := exec.Command("cub", "context", "get", "--json")
	ctxOutput, err := ctxCmd.Output()
	if err != nil {
		return ""
	}

	var ctx struct {
		Coordinate struct {
			ServerURL string `json:"serverURL"`
		} `json:"coordinate"`
	}
	if err := json.Unmarshal(ctxOutput, &ctx); err != nil {
		return ""
	}

	serverURL := ctx.Coordinate.ServerURL
	if serverURL == "" {
		return ""
	}

	// Get space ID from space slug
	spaceCmd := exec.Command("cub", "space", "list", "--json")
	spaceOutput, err := spaceCmd.Output()
	if err != nil {
		return fmt.Sprintf("%s/spaces/%s", serverURL, m.importSpace)
	}

	var spaces []CubSpaceData
	if err := json.Unmarshal(spaceOutput, &spaces); err != nil {
		return fmt.Sprintf("%s/spaces/%s", serverURL, m.importSpace)
	}

	for _, s := range spaces {
		if s.Space.Slug == m.importSpace {
			return fmt.Sprintf("%s/spaces/%s", serverURL, s.Space.SpaceID)
		}
	}

	return fmt.Sprintf("%s/spaces/%s", serverURL, m.importSpace)
}

func (m *Model) resetImportState() {
	m.importStep = 0
	m.importNamespaces = nil
	m.importShowAllNS = false
	m.importNamespace = ""
	m.importWorkloads = nil
	m.importSelected = nil
	m.importCursor = 0
	m.importLoading = false
	m.importError = nil
	m.importApplyError = nil
	m.importProgress = 0
	m.importTotal = 0
	// Reset config extraction state
	m.importExtractDone = false
	m.importExtractSuccess = 0
	m.importViewingConfig = false
	m.importViewConfigIdx = 0
	// Reset setup wizard state
	m.importCreateNewSpace = false
	m.importNewSpaceName = ""
	m.importCreateTarget = false
	m.importNewWorkerName = ""
	m.importNewTargetName = ""
	m.importTargetParams = ""
	m.importSetupChoice = 0
	m.importExistingSpaces = nil
	m.importExistingWorkers = nil
	m.importSelectedWorker = ""
	// Reset ArgoCD import state
	m.importSource = importSourceKubernetes
	m.importArgoApps = nil
	m.importSelectedArgo = nil
	m.importArgoResources = nil
	m.importArgoCleanup = argoCleanupKeepAsIs
	m.importUnitStructure = unitStructureCombined
	// Reset test state
	m.importTestChoice = testOptionSkip
	m.importTestResult = nil
	m.importTestRan = false
}

func (m *Model) getSelectedWorkloads() []WorkloadInfo {
	var selected []WorkloadInfo
	for i, w := range m.importWorkloads {
		if i < len(m.importSelected) && m.importSelected[i] {
			selected = append(selected, w)
		}
	}
	return selected
}

// Create wizard methods
func (m *Model) updateCreateWizard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.createMode = false
		m.resetCreateState()
		return m, nil
	case "up", "k":
		if m.createCursor > 0 {
			m.createCursor--
		}
	case "down", "j":
		max := m.getCreateMaxCursor()
		if m.createCursor < max {
			m.createCursor++
		}
	case "backspace":
		if m.createStep == createStepEnterName && len(m.createName) > 0 {
			m.createName = m.createName[:len(m.createName)-1]
		}
	case "enter":
		return m.advanceCreateStep()
	default:
		// Handle text input for name entry
		if len(msg.String()) == 1 && msg.String() != " " {
			if m.createStep == createStepEnterName {
				m.createName += msg.String()
			}
		}
	}
	return m, nil
}

func (m *Model) getCreateMaxCursor() int {
	switch m.createStep {
	case createStepSelectType:
		return 2 // Space, Unit, Target
	case createStepUnitMethod:
		return 1 // Clone, Empty
	case createStepSelectSource:
		if len(m.createUnits) == 0 {
			return 0
		}
		return len(m.createUnits) - 1
	case createStepSelectTarget:
		return len(m.createTargets) // Includes "(none)" option
	case createStepSelectWorker:
		return len(m.createWorkers) // Includes "(no worker)" option
	case createStepSelectProvider:
		return 2 // Kubernetes, Terraform, FluxOCIWriter
	case createStepConfirm:
		return 1 // Yes, No
	default:
		return 0
	}
}

func (m *Model) advanceCreateStep() (tea.Model, tea.Cmd) {
	switch m.createStep {
	case createStepSelectType:
		switch m.createCursor {
		case 0:
			m.createType = "space"
			m.createStep = createStepEnterName
		case 1:
			m.createType = "unit"
			if m.createSpace == "" {
				m.createError = fmt.Errorf("no space selected - navigate to a space first or create one")
				return m, nil
			}
			m.createStep = createStepEnterName
		case 2:
			m.createType = "target"
			if m.createSpace == "" {
				m.createError = fmt.Errorf("no space selected - navigate to a space first or create one")
				return m, nil
			}
			m.createStep = createStepEnterName
		}
		m.createCursor = 0

	case createStepEnterName:
		if m.createName == "" {
			m.createError = fmt.Errorf("name is required")
			return m, nil
		}
		m.createError = nil
		switch m.createType {
		case "space":
			m.createStep = createStepConfirm
		case "unit":
			m.createStep = createStepUnitMethod
		case "target":
			// Load workers for target creation
			m.createLoading = true
			m.createStep = createStepSelectWorker
			return m, loadCreateWorkersCmd(m.createSpace)
		}
		m.createCursor = 0

	case createStepUnitMethod:
		if m.createCursor == 0 {
			// Clone from existing
			m.createLoading = true
			m.createStep = createStepSelectSource
			return m, loadCreateUnitsCmd(m.createSpace)
		} else {
			// Empty config - use default toolchain and skip to target selection
			m.createCloneFrom = ""
			m.createToolchain = "Kubernetes/YAML" // Default toolchain for empty units
			m.createLoading = true
			m.createStep = createStepSelectTarget
			return m, loadCreateTargetsCmd(m.createSpace, m.createToolchain)
		}

	case createStepSelectSource:
		if len(m.createUnits) > 0 && m.createCursor < len(m.createUnits) {
			m.createCloneFrom = m.createUnits[m.createCursor].Slug
			m.createToolchain = m.createUnits[m.createCursor].Toolchain
		} else {
			// No units available, use default toolchain
			m.createToolchain = "Kubernetes/YAML"
		}
		// Move to target selection (filtered by toolchain)
		m.createLoading = true
		m.createStep = createStepSelectTarget
		m.createCursor = 0
		return m, loadCreateTargetsCmd(m.createSpace, m.createToolchain)

	case createStepSelectTarget:
		if m.createCursor == 0 {
			m.createTarget = "" // (none)
		} else if m.createCursor-1 < len(m.createTargets) {
			m.createTarget = m.createTargets[m.createCursor-1]
		}
		m.createStep = createStepConfirm
		m.createCursor = 0

	case createStepSelectWorker:
		if m.createCursor == 0 {
			m.createWorker = "" // (no worker)
		} else if m.createCursor-1 < len(m.createWorkers) {
			m.createWorker = m.createWorkers[m.createCursor-1]
		}
		m.createStep = createStepSelectProvider
		m.createCursor = 0

	case createStepSelectProvider:
		switch m.createCursor {
		case 0:
			m.createProvider = "Kubernetes"
		case 1:
			m.createProvider = "Terraform"
		case 2:
			m.createProvider = "FluxOCIWriter"
		}
		m.createStep = createStepConfirm
		m.createCursor = 0

	case createStepConfirm:
		if m.createCursor == 0 {
			// Yes - create the resource
			// Add pending action for optimistic UI
			parentID := m.createSpace
			if m.createType == "space" {
				parentID = m.currentOrg
			}
			m.addPendingAction("creating", m.createType, m.createName, parentID)

			// Exit create mode immediately for optimistic UI feedback
			m.createMode = false
			m.statusMsg = fmt.Sprintf("Creating %s '%s'...", m.createType, m.createName)
			m.rebuildFlatList()

			// Store values before reset for the command
			createType := m.createType
			createName := m.createName
			createSpace := m.createSpace
			createCloneFrom := m.createCloneFrom
			createTarget := m.createTarget
			createWorker := m.createWorker
			createProvider := m.createProvider

			m.resetCreateState()

			switch createType {
			case "space":
				return m, doCreateSpaceCmd(createName)
			case "unit":
				return m, doCreateUnitCmd(createSpace, createName, createCloneFrom, createTarget)
			case "target":
				return m, doCreateTargetCmd(createSpace, createName, createWorker, createProvider)
			}
		} else {
			// No - cancel
			m.createMode = false
			m.resetCreateState()
		}

	case createStepComplete:
		m.createMode = false
		m.resetCreateState()
		return m, loadDataCmd
	}
	return m, nil
}

func (m *Model) resetCreateState() {
	m.createStep = createStepSelectType
	m.createType = ""
	m.createName = ""
	m.createCloneFrom = ""
	m.createTarget = ""
	m.createWorker = ""
	m.createProvider = "Kubernetes"
	m.createToolchain = ""
	m.createCursor = 0
	m.createLoading = false
	m.createError = nil
	m.createUnits = nil
	m.createTargets = nil
	m.createWorkers = nil
}

// Delete wizard methods
func (m *Model) updateDeleteWizard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.deleteMode = false
		m.resetDeleteState()
		return m, nil
	case "up", "k":
		if m.deleteCursor > 0 {
			m.deleteCursor--
		}
	case "down", "j":
		if m.deleteCursor < 1 {
			m.deleteCursor++
		}
	case "enter":
		return m.advanceDeleteStep()
	case "y", "Y":
		// Quick confirm with 'y'
		if m.deleteStep == deleteStepConfirm {
			m.deleteCursor = 0
			return m.advanceDeleteStep()
		}
	case "n", "N":
		// Quick cancel with 'n'
		if m.deleteStep == deleteStepConfirm {
			m.deleteMode = false
			m.resetDeleteState()
			return m, nil
		}
	}
	return m, nil
}

func (m *Model) advanceDeleteStep() (tea.Model, tea.Cmd) {
	switch m.deleteStep {
	case deleteStepConfirm:
		if m.deleteCursor == 0 {
			// Yes - delete the resource
			// Add pending action for optimistic UI (hides item immediately)
			parentID := m.deleteSpace
			if m.deleteType == "space" {
				parentID = m.currentOrg
			}
			m.addPendingAction("deleting", m.deleteType, m.deleteName, parentID)

			// Exit delete mode immediately for optimistic UI feedback
			m.deleteMode = false
			m.statusMsg = fmt.Sprintf("Deleting %s '%s'...", m.deleteType, m.deleteName)
			m.rebuildFlatList()

			// Store values before reset for the command
			deleteType := m.deleteType
			deleteName := m.deleteName
			deleteSpace := m.deleteSpace

			m.resetDeleteState()

			switch deleteType {
			case "space":
				return m, doDeleteSpaceCmd(deleteName)
			case "unit":
				return m, doDeleteUnitCmd(deleteSpace, deleteName)
			case "target":
				return m, doDeleteTargetCmd(deleteSpace, deleteName)
			}
		} else {
			// No - cancel
			m.deleteMode = false
			m.resetDeleteState()
		}

	case deleteStepComplete:
		m.deleteMode = false
		m.resetDeleteState()
		return m, loadDataCmd
	}
	return m, nil
}

func (m *Model) resetDeleteState() {
	m.deleteStep = deleteStepConfirm
	m.deleteType = ""
	m.deleteName = ""
	m.deleteSpace = ""
	m.deleteCursor = 1 // Default to "No"
	m.deleteLoading = false
	m.deleteError = nil
}

func (m Model) renderDeleteWizard() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render(" DELETE RESOURCE "))
	b.WriteString("\n\n")

	// Show error if any
	if m.deleteError != nil {
		b.WriteString(statusErr.Render("Error: " + m.deleteError.Error()))
		b.WriteString("\n\n")
	}

	// Show loading state
	if m.deleteLoading {
		b.WriteString(dimStyle.Render("Deleting..."))
		b.WriteString("\n")
		return b.String()
	}

	switch m.deleteStep {
	case deleteStepConfirm:
		// Show what will be deleted
		b.WriteString(statusErr.Render("⚠ WARNING: This action cannot be undone!"))
		b.WriteString("\n\n")

		typeLabel := titleCase(m.deleteType)
		b.WriteString(fmt.Sprintf("Delete %s: ", typeLabel))
		b.WriteString(activeStyle.Render(m.deleteName))
		b.WriteString("\n")

		if m.deleteSpace != "" {
			b.WriteString(dimStyle.Render("Space: " + m.deleteSpace))
			b.WriteString("\n")
		}

		if m.deleteType == "space" {
			b.WriteString("\n")
			b.WriteString(statusWarn.Render("This will delete all units and targets in the space."))
			b.WriteString("\n")
		}

		b.WriteString("\n")
		b.WriteString("Are you sure?\n\n")

		// Yes/No options
		options := []string{"Yes, delete", "No, cancel"}
		for i, opt := range options {
			cursor := "  "
			if i == m.deleteCursor {
				cursor = activeStyle.Render("> ")
			}
			if i == 0 {
				b.WriteString(cursor + statusErr.Render(opt) + "\n")
			} else {
				b.WriteString(cursor + opt + "\n")
			}
		}

		b.WriteString("\n")
		b.WriteString(dimStyle.Render("Tip: Press 'y' to confirm, 'n' or Esc to cancel"))

	case deleteStepComplete:
		b.WriteString(statusOK.Render("✓ "))
		b.WriteString(fmt.Sprintf("%s '%s' deleted successfully", titleCase(m.deleteType), m.deleteName))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("Press Enter to continue"))
	}

	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("↑↓ navigate  y/n confirm  esc cancel"))

	return b.String()
}

func (m Model) renderOrgSelector() string {
	var b strings.Builder

	// Header
	b.WriteString(headerStyle.Render(" SWITCH ORGANIZATION "))
	b.WriteString("\n\n")

	orgs := m.getOrgList()

	// Instructions
	b.WriteString(dimStyle.Render("Select an organization to switch to:"))
	b.WriteString("\n\n")

	// List orgs
	for i, org := range orgs {
		isCurrent := m.isCurrentOrg(org)

		// Cursor indicator
		cursor := "  "
		if i == m.orgSelectCursor {
			cursor = activeStyle.Render("> ")
		}

		// Org name
		name := org.DisplayName
		if name == "" {
			name = org.Slug
		}

		// Current indicator
		currentMark := ""
		if isCurrent {
			currentMark = statusOK.Render(" ✓ current")
		}

		// Slug/ID info
		info := dimStyle.Render(fmt.Sprintf(" (%s)", org.Slug))

		if i == m.orgSelectCursor {
			b.WriteString(cursor + activeStyle.Render(name) + info + currentMark)
		} else {
			b.WriteString(cursor + name + info + currentMark)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("This allows you to compare the TUI view with the GUI at app.confighub.com"))
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("↑↓ navigate  Enter select  Esc cancel"))

	return b.String()
}

func (m Model) renderHelpOverlay() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("141"))
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	cmdStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("81"))

	b.WriteString(titleStyle.Render("╭────────────────────────────────────────────────────────────────╮"))
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("│") + "  " + titleStyle.Render("CONFIGHUB HIERARCHY HELP") + strings.Repeat(" ", 37) + titleStyle.Render("│"))
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("╰────────────────────────────────────────────────────────────────╯"))
	b.WriteString("\n\n")

	b.WriteString(sectionStyle.Render("NAVIGATION"))
	b.WriteString("\n")
	b.WriteString("  " + keyStyle.Render("↑/k ↓/j") + "    " + descStyle.Render("Move up/down"))
	b.WriteString("\n")
	b.WriteString("  " + keyStyle.Render("←/h →/l") + "    " + descStyle.Render("Collapse/expand node"))
	b.WriteString("\n")
	b.WriteString("  " + keyStyle.Render("Enter") + "      " + descStyle.Render("Load details in right pane"))
	b.WriteString("\n")
	b.WriteString("  " + keyStyle.Render("Tab") + "        " + descStyle.Render("Switch focus to details pane"))
	b.WriteString("\n\n")

	b.WriteString(sectionStyle.Render("SEARCH & FILTER"))
	b.WriteString("\n")
	b.WriteString("  " + keyStyle.Render("/") + "          " + descStyle.Render("Start search"))
	b.WriteString("\n")
	b.WriteString("  " + keyStyle.Render("n/N") + "        " + descStyle.Render("Next/previous match"))
	b.WriteString("\n")
	b.WriteString("  " + keyStyle.Render("f") + "          " + descStyle.Render("Toggle filter mode"))
	b.WriteString("\n\n")

	b.WriteString(sectionStyle.Render("ACTIONS"))
	b.WriteString("\n")
	b.WriteString("  " + keyStyle.Render("a") + "          " + descStyle.Render("Activity view (recent changes)"))
	b.WriteString("\n")
	b.WriteString("  " + keyStyle.Render("B") + "          " + descStyle.Render("Toggle Hub/AppSpace view"))
	b.WriteString("\n")
	b.WriteString("  " + keyStyle.Render("M") + "          " + descStyle.Render("Three Maps view (GitOps + ConfigHub + Repos)"))
	b.WriteString("\n")
	b.WriteString("  " + keyStyle.Render("P") + "          " + descStyle.Render("Panel view (WET↔LIVE side-by-side)"))
	b.WriteString("\n")
	b.WriteString("  " + keyStyle.Render("c") + "          " + descStyle.Render("Create new resource"))
	b.WriteString("\n")
	b.WriteString("  " + keyStyle.Render("d/x") + "        " + descStyle.Render("Delete selected resource"))
	b.WriteString("\n")
	b.WriteString("  " + keyStyle.Render("i") + "          " + descStyle.Render("Import workloads from Kubernetes"))
	b.WriteString("\n")
	b.WriteString("  " + keyStyle.Render("o") + "          " + descStyle.Render("Open in browser"))
	b.WriteString("\n")
	b.WriteString("  " + keyStyle.Render("O") + "          " + descStyle.Render("Switch organization"))
	b.WriteString("\n")
	b.WriteString("  " + keyStyle.Render("r") + "          " + descStyle.Render("Refresh data"))
	b.WriteString("\n\n")

	b.WriteString(sectionStyle.Render("PATTERNS") + " " + descStyle.Render("(press B to see)"))
	b.WriteString("\n")
	b.WriteString("  " + cmdStyle.Render("Banko") + "       " + descStyle.Render("Flux cluster-per-dir, versioned platform"))
	b.WriteString("\n")
	b.WriteString("  " + cmdStyle.Render("Arnie") + "       " + descStyle.Render("ArgoCD folders-per-env, promotion=cp"))
	b.WriteString("\n")
	b.WriteString("  " + cmdStyle.Render("TraderX") + "     " + descStyle.Render("Multi-region base/infra Hub"))
	b.WriteString("\n")
	b.WriteString("  " + cmdStyle.Render("KubeCon") + "     " + descStyle.Render("Platform team + App teams"))
	b.WriteString("\n")
	b.WriteString("  " + descStyle.Render("See: docs/map/reference/hub-appspace-examples.md"))
	b.WriteString("\n\n")

	b.WriteString(sectionStyle.Render("QUERY EXAMPLES") + " " + descStyle.Render("(type after :)"))
	b.WriteString("\n")
	b.WriteString("  " + cmdStyle.Render("owner=Native") + "            " + descStyle.Render("Orphaned resources"))
	b.WriteString("\n")
	b.WriteString("  " + cmdStyle.Render("owner=Flux OR owner=ArgoCD") + " " + descStyle.Render("GitOps managed"))
	b.WriteString("\n")
	b.WriteString("  " + cmdStyle.Render("namespace=prod*") + "         " + descStyle.Render("Prod namespaces"))
	b.WriteString("\n")
	b.WriteString("  " + cmdStyle.Render("labels[app]=nginx") + "       " + descStyle.Render("By label"))
	b.WriteString("\n\n")

	b.WriteString(sectionStyle.Render("COMMAND PALETTE"))
	b.WriteString("\n")
	b.WriteString("  " + keyStyle.Render(":") + "          " + descStyle.Render("Open command input (queries or commands)"))
	b.WriteString("\n")
	b.WriteString("  " + keyStyle.Render("L") + "          " + descStyle.Render("Switch to local cluster TUI"))
	b.WriteString("\n\n")

	b.WriteString(sectionStyle.Render("CUB-AGENT COMMANDS") + " " + descStyle.Render("(use : to run)"))
	b.WriteString("\n")
	b.WriteString("  " + cmdStyle.Render("cub-scout map") + "           " + descStyle.Render("Local cluster TUI"))
	b.WriteString("\n")
	b.WriteString("  " + cmdStyle.Render("cub-scout map orphans") + "   " + descStyle.Render("List orphaned resources"))
	b.WriteString("\n")
	b.WriteString("  " + cmdStyle.Render("cub-scout map crashes") + "   " + descStyle.Render("List crashing resources"))
	b.WriteString("\n")
	b.WriteString("  " + cmdStyle.Render("cub-scout scan") + "          " + descStyle.Render("Scan for CCVEs"))
	b.WriteString("\n")
	b.WriteString("  " + cmdStyle.Render("cub-scout trace") + "         " + descStyle.Render("Trace ownership"))
	b.WriteString("\n\n")

	b.WriteString(sectionStyle.Render("QUIT"))
	b.WriteString("\n")
	b.WriteString("  " + keyStyle.Render("q") + "          " + descStyle.Render("Quit"))
	b.WriteString("\n")
	b.WriteString("  " + keyStyle.Render("?") + "          " + descStyle.Render("Show this help"))
	b.WriteString("\n\n")

	b.WriteString(descStyle.Render("Press any key to close"))

	return b.String()
}

func (m Model) renderActivityView() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("141"))
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	syncedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	notSyncedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

	b.WriteString(titleStyle.Render("╭────────────────────────────────────────────────────────────────╮"))
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("│") + "  " + titleStyle.Render("🕐 RECENT ACTIVITY") + strings.Repeat(" ", 43) + titleStyle.Render("│"))
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("╰────────────────────────────────────────────────────────────────╯"))
	b.WriteString("\n\n")

	// Collect recent units sorted by revision
	b.WriteString(sectionStyle.Render("RECENT UNIT UPDATES"))
	b.WriteString("\n")

	type unitActivity struct {
		space    string
		unit     string
		revision int
		synced   bool
	}
	var activities []unitActivity

	// Walk through tree to find units
	for _, node := range m.flatList {
		if node.Type == "unit" {
			if unitData, ok := node.Data.(CubUnitData); ok {
				spaceName := ""
				if node.Parent != nil && node.Parent.Parent != nil && node.Parent.Parent.Type == "space" {
					spaceName = node.Parent.Parent.ID
				}
				activities = append(activities, unitActivity{
					space:    spaceName,
					unit:     unitData.Unit.Slug,
					revision: unitData.Unit.HeadRevisionNum,
					synced:   true, // We'd need target data for real sync status
				})
			}
		}
	}

	// Sort by revision (highest first) and show top 10
	for i := 0; i < len(activities)-1; i++ {
		for j := i + 1; j < len(activities); j++ {
			if activities[j].revision > activities[i].revision {
				activities[i], activities[j] = activities[j], activities[i]
			}
		}
	}

	if len(activities) == 0 {
		b.WriteString("  " + dimStyle.Render("No units found. Navigate to a space to load units."))
		b.WriteString("\n")
	} else {
		count := len(activities)
		if count > 10 {
			count = 10
		}
		for i := 0; i < count; i++ {
			act := activities[i]
			status := syncedStyle.Render("✓")
			if !act.synced {
				status = notSyncedStyle.Render("⚠")
			}
			b.WriteString(fmt.Sprintf("  %s  %-20s  %s  rev %d",
				status,
				nameStyle.Render(act.unit),
				dimStyle.Render(act.space),
				act.revision))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")

	// Workers status
	b.WriteString(sectionStyle.Render("WORKER STATUS"))
	b.WriteString("\n")

	workersFound := false
	for _, node := range m.flatList {
		if node.Type == "worker" {
			workersFound = true
			workerName := node.ID
			status := dimStyle.Render("Unknown")
			if workerData, ok := node.Data.(CubWorkerData); ok {
				workerName = workerData.BridgeWorker.Slug
				switch workerData.BridgeWorker.Condition {
				case "Ready":
					status = syncedStyle.Render("● Ready")
				case "Disconnected":
					status = errorStyle.Render("○ Disconnected")
				default:
					status = notSyncedStyle.Render("◐ " + workerData.BridgeWorker.Condition)
				}
			}
			b.WriteString(fmt.Sprintf("  %s  %s", status, nameStyle.Render(workerName)))
			b.WriteString("\n")
		}
	}
	if !workersFound {
		b.WriteString("  " + dimStyle.Render("No workers found. Expand a space to see workers."))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Targets status
	b.WriteString(sectionStyle.Render("TARGET STATUS"))
	b.WriteString("\n")

	targetsFound := false
	for _, node := range m.flatList {
		if node.Type == "target" {
			targetsFound = true
			targetName := node.ID
			status := dimStyle.Render("Unknown")
			if targetData, ok := node.Data.(CubTargetData); ok {
				if targetData.Target.Slug != "" {
					targetName = targetData.Target.Slug
				}
				// Targets don't have a condition field, show provider instead
				provider := targetData.Target.ProviderType
				if provider == "" {
					provider = "Kubernetes"
				}
				status = dimStyle.Render(provider)
			}
			b.WriteString(fmt.Sprintf("  ●  %-20s  %s", nameStyle.Render(targetName), status))
			b.WriteString("\n")
		}
	}
	if !targetsFound {
		b.WriteString("  " + dimStyle.Render("No targets found. Expand a space to see targets."))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Tip
	b.WriteString(dimStyle.Render("Tip: Press 'r' to refresh, '/' to search, ':' for commands"))
	b.WriteString("\n\n")
	b.WriteString(dimStyle.Render("Press any key to close"))

	return b.String()
}

// renderMapsView shows the THREE MAPS integrated view
func (m Model) renderMapsView() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("141"))
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	cyanStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("51"))
	purpleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("141"))

	// Header
	b.WriteString(titleStyle.Render("╭────────────────────────────────────────────────────────────────╮"))
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("│") + "  " + titleStyle.Render("🗺️  THREE MAPS") + "             " + dimStyle.Render("GitOps · ConfigHub · Repos") + "  " + titleStyle.Render("│"))
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("╰────────────────────────────────────────────────────────────────╯"))
	b.WriteString("\n\n")

	// MAP 1: GitOps Resource Trees
	b.WriteString(sectionStyle.Render("MAP 1: GITOPS RESOURCE TREES"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  Use 'L' to switch to local cluster TUI for full GitOps tree view"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  Or run: cub-scout map workloads, cub-scout map deployers"))
	b.WriteString("\n\n")

	// MAP 2a: ConfigHub Basic Hierarchy
	b.WriteString(sectionStyle.Render("MAP 2a: CONFIGHUB BASIC HIERARCHY"))
	b.WriteString("\n")

	// Walk through tree to show org → space → units/workers/targets
	orgCount := 0
	spaceCount := 0
	unitCount := 0
	workerCount := 0
	targetCount := 0

	for _, node := range m.nodes {
		if node.Type == "organization" {
			orgCount++
			b.WriteString(fmt.Sprintf("  %s Org: %s\n", titleStyle.Render(""), nameStyle.Render(node.Name)))

			for _, spaceNode := range node.Children {
				if spaceNode.Type == "space" {
					spaceCount++
					units := 0
					workers := 0
					targets := 0

					for _, child := range spaceNode.Children {
						switch child.Type {
						case "units":
							units = len(child.Children)
							unitCount += units
						case "workers":
							workers = len(child.Children)
							workerCount += workers
						case "targets":
							targets = len(child.Children)
							targetCount += targets
						}
					}

					b.WriteString(fmt.Sprintf("  └── Space: %s\n", nameStyle.Render(spaceNode.Name)))
					b.WriteString(fmt.Sprintf("      ├── Units: %d\n", units))
					b.WriteString(fmt.Sprintf("      ├── Workers: %d\n", workers))
					b.WriteString(fmt.Sprintf("      └── Targets: %d\n", targets))
				}
			}
		}
	}

	if orgCount == 0 {
		b.WriteString("  " + dimStyle.Render("Loading hierarchy data..."))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// MAP 2b: Hub/AppSpace Model
	b.WriteString(sectionStyle.Render("MAP 2b: HUB/APPSPACE MODEL") + " " + dimStyle.Render("(Platform + App Teams)"))
	b.WriteString("\n")

	// Categorize spaces into platform vs app spaces
	var platformSpaces []string
	var appSpaces []string

	for _, node := range m.nodes {
		if node.Type == "organization" {
			for _, spaceNode := range node.Children {
				if spaceNode.Type == "space" {
					spaceName := spaceNode.Name
					// Platform spaces typically start with platform-, infra-, hub-, shared-
					if strings.HasPrefix(spaceName, "platform-") ||
						strings.HasPrefix(spaceName, "infra-") ||
						strings.HasPrefix(spaceName, "hub-") ||
						strings.HasPrefix(spaceName, "shared-") {
						platformSpaces = append(platformSpaces, spaceName)
					} else {
						appSpaces = append(appSpaces, spaceName)
					}
				}
			}
		}
	}

	b.WriteString(fmt.Sprintf("  ├── %s\n", purpleStyle.Render("Hub (Platform)")))
	if len(platformSpaces) > 0 {
		for _, s := range platformSpaces {
			b.WriteString(fmt.Sprintf("  │   └── %s\n", s))
		}
	} else {
		b.WriteString("  │   └── " + dimStyle.Render("(no platform spaces)") + "\n")
	}

	b.WriteString("  │\n")
	b.WriteString(fmt.Sprintf("  └── %s\n", cyanStyle.Render("App Spaces (Teams)")))
	if len(appSpaces) > 0 {
		shown := 5
		if len(appSpaces) < shown {
			shown = len(appSpaces)
		}
		for i := 0; i < shown; i++ {
			b.WriteString(fmt.Sprintf("      ├── %s\n", nameStyle.Render(appSpaces[i])))
		}
		if len(appSpaces) > shown {
			b.WriteString(fmt.Sprintf("      └── %s\n", dimStyle.Render(fmt.Sprintf("... and %d more", len(appSpaces)-shown))))
		}
	} else {
		b.WriteString("      └── " + dimStyle.Render("(no app spaces)") + "\n")
	}

	b.WriteString("\n")
	b.WriteString("  " + dimStyle.Render("Queryable: space, labels (app, env, team), owner") + "\n")
	b.WriteString("\n")

	// MAP 3: Repo Structure
	b.WriteString(sectionStyle.Render("MAP 3: REPO STRUCTURE → DEPLOYMENTS"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  Use 'L' to switch to local cluster TUI for git sources"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  Or run: cub-scout map deployers"))
	b.WriteString("\n\n")

	// Summary
	b.WriteString(dimStyle.Render("────────────────────────────────────────────────────────────────"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("Total: %d orgs · %d spaces · %d units · %d workers · %d targets",
		orgCount, spaceCount, unitCount, workerCount, targetCount))
	b.WriteString("\n\n")

	b.WriteString(dimStyle.Render("Press any key to close"))

	return b.String()
}

// renderPanelView shows WET (ConfigHub) vs LIVE (Cluster) side-by-side
func (m Model) renderPanelView() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("141"))
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82"))

	// Header
	b.WriteString(titleStyle.Render("╭────────────────────────────────────────────────────────────────╮"))
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("│") + "  " + titleStyle.Render("📊  PANEL VIEW") + "           " + dimStyle.Render("WET (ConfigHub) ↔ LIVE (Cluster)") + "  " + titleStyle.Render("│"))
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("╰────────────────────────────────────────────────────────────────╯"))
	b.WriteString("\n\n")

	// Loading state
	if m.panelLoading {
		b.WriteString(dimStyle.Render("  Loading cluster data..."))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("Press any key to close"))
		return b.String()
	}

	// Error state
	if m.panelError != nil {
		b.WriteString(errStyle.Render("  Error loading cluster data: " + m.panelError.Error()))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("Press any key to close"))
		return b.String()
	}

	// Calculate pane width for side-by-side display
	paneWidth := 32
	if m.width > 80 {
		paneWidth = (m.width - 10) / 2
	}

	// Collect units from the tree
	var units []CubUnitData
	for _, node := range m.nodes {
		if node.Type == "organization" {
			for _, spaceNode := range node.Children {
				if spaceNode.Type == "space" {
					for _, groupNode := range spaceNode.Children {
						if groupNode.Type == "units" {
							for _, unitNode := range groupNode.Children {
								if unitData, ok := unitNode.Data.(CubUnitData); ok {
									units = append(units, unitData)
								}
							}
						}
					}
				}
			}
		}
	}

	// WET column header
	wetHeader := sectionStyle.Render("WET (ConfigHub)")
	liveHeader := sectionStyle.Render("LIVE (Cluster)")
	b.WriteString(fmt.Sprintf("  %-*s  │  %s\n", paneWidth, wetHeader, liveHeader))
	b.WriteString(fmt.Sprintf("  %s──┼──%s\n", strings.Repeat("─", paneWidth), strings.Repeat("─", paneWidth)))

	// Show each unit with its correlated workloads
	for _, unit := range units {
		unitSlug := unit.Unit.Slug
		unitRev := unit.Unit.HeadRevisionNum

		// WET side: unit info
		wetLine := fmt.Sprintf("%s (Rev %d)", nameStyle.Render(unitSlug), unitRev)

		// LIVE side: correlated workloads
		liveWorkloads, hasLive := m.panelCorrelation[unitSlug]
		var liveLine string
		if hasLive && len(liveWorkloads) > 0 {
			// Show first workload with status
			w := liveWorkloads[0]
			statusIcon := "✓"
			statusStyle := okStyle
			if w.Status == "NotReady" || w.Status == "Pending" {
				statusIcon = "⚠"
				statusStyle = warnStyle
			} else if w.Status == "Failed" {
				statusIcon = "✗"
				statusStyle = errStyle
			}

			// Check revision match
			revMatch := ""
			if revStr, ok := w.OwnerDetails["revision"]; ok {
				if revNum, err := strconv.Atoi(revStr); err == nil {
					if revNum == unitRev {
						revMatch = okStyle.Render(" ✓")
					} else if revNum < unitRev {
						revMatch = warnStyle.Render(fmt.Sprintf(" (Rev %d behind)", unitRev-revNum))
					}
				}
			}

			liveLine = fmt.Sprintf("%s %s/%s%s", statusStyle.Render(statusIcon), w.Kind, w.Name, revMatch)
			if len(liveWorkloads) > 1 {
				liveLine += dimStyle.Render(fmt.Sprintf(" +%d more", len(liveWorkloads)-1))
			}
		} else {
			liveLine = dimStyle.Render("—")
		}

		b.WriteString(fmt.Sprintf("  %-*s  │  %s\n", paneWidth, wetLine, liveLine))
	}

	// Show orphans section if any
	if len(m.panelOrphans) > 0 {
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  %-*s  │  %s\n", paneWidth, "", warnStyle.Render("ORPHANS (not in ConfigHub)")))
		b.WriteString(fmt.Sprintf("  %s──┼──%s\n", strings.Repeat("─", paneWidth), strings.Repeat("─", paneWidth)))

		for i, orphan := range m.panelOrphans {
			if i >= 5 { // Limit to first 5 orphans
				b.WriteString(fmt.Sprintf("  %-*s  │  %s\n", paneWidth, "", dimStyle.Render(fmt.Sprintf("  ... and %d more", len(m.panelOrphans)-5))))
				break
			}
			wetLine := dimStyle.Render("—")
			liveLine := warnStyle.Render("🔴 ") + fmt.Sprintf("%s/%s", orphan.Kind, orphan.Name)
			if orphan.Namespace != "" {
				liveLine += dimStyle.Render(fmt.Sprintf(" (%s)", orphan.Namespace))
			}
			b.WriteString(fmt.Sprintf("  %-*s  │  %s\n", paneWidth, wetLine, liveLine))
		}
	}

	// Summary
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("────────────────────────────────────────────────────────────────"))
	b.WriteString("\n")
	synced := len(m.panelCorrelation)
	orphanCount := len(m.panelOrphans)
	totalLive := len(m.panelWorkloads)
	summary := fmt.Sprintf("WET: %d units │ LIVE: %d workloads │ Correlated: %d │ Orphans: %d",
		len(units), totalLive, synced, orphanCount)
	b.WriteString(summary)
	b.WriteString("\n\n")

	b.WriteString(dimStyle.Render("[i] Import orphan  [r] Refresh  [Esc] Close"))

	return b.String()
}

// renderSuggestView shows suggested ConfigHub units from cluster workloads
func (m Model) renderSuggestView() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("141"))
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	cyanStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("87"))

	// Header
	b.WriteString(titleStyle.Render("╭────────────────────────────────────────────────────────────────╮"))
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("│") + "  " + titleStyle.Render("💡  SUGGEST UNITS") + "         " + dimStyle.Render("Recommend ConfigHub Units") + "     " + titleStyle.Render("│"))
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("╰────────────────────────────────────────────────────────────────╯"))
	b.WriteString("\n\n")

	// Loading state
	if m.suggestLoading {
		b.WriteString(dimStyle.Render("  Scanning cluster for workloads..."))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("Press Esc to close"))
		return b.String()
	}

	// Error state
	if m.suggestError != nil {
		b.WriteString(errStyle.Render("  Error scanning cluster: " + m.suggestError.Error()))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("Press Esc to close"))
		return b.String()
	}

	// No proposal state
	if m.suggestProposal == nil || len(m.suggestProposal.Units) == 0 {
		b.WriteString(dimStyle.Render("  No workloads found to suggest."))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("All cluster workloads are either:"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("  • Already managed by ConfigHub"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("  • In system namespaces (kube-system, etc.)"))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("Press Esc to close"))
		return b.String()
	}

	// Show suggestion
	proposal := m.suggestProposal

	b.WriteString(sectionStyle.Render("Suggested App Space: ") + nameStyle.Render(proposal.AppSpace))
	b.WriteString("\n\n")

	b.WriteString(sectionStyle.Render("Suggested Units:"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("────────────────────────────────────────────────────────────────"))
	b.WriteString("\n\n")

	// Show each suggested unit
	for i, unit := range proposal.Units {
		if i >= 15 { // Limit display
			remaining := len(proposal.Units) - 15
			b.WriteString(dimStyle.Render(fmt.Sprintf("  ... and %d more units", remaining)))
			b.WriteString("\n")
			break
		}

		// Unit slug with icon
		cursor := "  "
		if i == m.suggestCursor {
			cursor = cyanStyle.Render("▶ ")
		}
		b.WriteString(cursor + nameStyle.Render(unit.Slug))
		b.WriteString("\n")

		// Workloads in this unit
		for j, wl := range unit.Workloads {
			if j >= 3 { // Limit workloads shown per unit
				remaining := len(unit.Workloads) - 3
				b.WriteString(dimStyle.Render(fmt.Sprintf("      ... +%d more", remaining)))
				b.WriteString("\n")
				break
			}
			ownerLabel := ""
			if wl.Owner != "" && wl.Owner != "Unknown" {
				switch wl.Owner {
				case "Flux":
					ownerLabel = cyanStyle.Render(" [Flux]")
				case "ArgoCD":
					ownerLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("141")).Render(" [ArgoCD]")
				case "Helm":
					ownerLabel = warnStyle.Render(" [Helm]")
				default:
					ownerLabel = dimStyle.Render(fmt.Sprintf(" [%s]", wl.Owner))
				}
			}
			b.WriteString(dimStyle.Render(fmt.Sprintf("    └─ %s/%s (%s)%s", wl.Kind, wl.Name, wl.Namespace, ownerLabel)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Summary
	b.WriteString(dimStyle.Render("────────────────────────────────────────────────────────────────"))
	b.WriteString("\n")
	totalWorkloads := 0
	for _, u := range proposal.Units {
		totalWorkloads += len(u.Workloads)
	}
	summary := fmt.Sprintf("Suggested: %d units from %d workloads", len(proposal.Units), totalWorkloads)
	b.WriteString(summary)
	b.WriteString("\n\n")

	b.WriteString(dimStyle.Render("[i] Import selected  [I] Import all  [r] Refresh  [Esc] Close"))

	return b.String()
}

func (m Model) renderCreateWizard() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render(" CREATE RESOURCE "))
	b.WriteString("\n\n")

	// Show error if any
	if m.createError != nil {
		b.WriteString(statusErr.Render("Error: " + m.createError.Error()))
		b.WriteString("\n\n")
	}

	// Show loading state
	if m.createLoading {
		b.WriteString(dimStyle.Render("Loading..."))
		b.WriteString("\n")
		return b.String()
	}

	switch m.createStep {
	case createStepSelectType:
		b.WriteString("What would you like to create?\n\n")
		options := []string{"Space", "Unit", "Target"}
		for i, opt := range options {
			cursor := "  "
			if i == m.createCursor {
				cursor = activeStyle.Render("> ")
			}
			b.WriteString(cursor + opt + "\n")
		}

	case createStepEnterName:
		typeLabel := titleCase(m.createType)
		if m.createSpace != "" && m.createType != "space" {
			b.WriteString(dimStyle.Render("Space: " + m.createSpace))
			b.WriteString("\n\n")
		}
		b.WriteString(fmt.Sprintf("Enter %s name:\n\n", typeLabel))
		b.WriteString(promptStyle.Render("> "))
		b.WriteString(m.createName)
		b.WriteString(activeStyle.Render("_"))

	case createStepUnitMethod:
		b.WriteString(dimStyle.Render("Creating unit: " + m.createName))
		b.WriteString("\n\n")
		b.WriteString("How would you like to create this unit?\n\n")
		options := []string{"Clone from existing unit", "Start with empty config"}
		for i, opt := range options {
			cursor := "  "
			if i == m.createCursor {
				cursor = activeStyle.Render("> ")
			}
			b.WriteString(cursor + opt + "\n")
		}

	case createStepSelectSource:
		b.WriteString(dimStyle.Render("Creating unit: " + m.createName))
		b.WriteString("\n\n")
		b.WriteString("Select unit to clone:\n\n")
		if len(m.createUnits) == 0 {
			b.WriteString(dimStyle.Render("(no units available - press Enter to skip)"))
		} else {
			for i, unit := range m.createUnits {
				cursor := "  "
				if i == m.createCursor {
					cursor = activeStyle.Render("> ")
				}
				// Show unit slug with toolchain type
				b.WriteString(fmt.Sprintf("%s%s %s\n", cursor, unit.Slug, dimStyle.Render("("+unit.Toolchain+")")))
			}
		}

	case createStepSelectTarget:
		b.WriteString(dimStyle.Render("Creating unit: " + m.createName))
		b.WriteString("\n")
		if m.createCloneFrom != "" {
			b.WriteString(dimStyle.Render("Clone from: " + m.createCloneFrom))
			b.WriteString("\n")
		}
		b.WriteString(dimStyle.Render("Toolchain: " + m.createToolchain))
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf("Assign to target (optional, %s only):\n\n", m.createToolchain))
		// First option is always "(none)"
		cursor := "  "
		if m.createCursor == 0 {
			cursor = activeStyle.Render("> ")
		}
		b.WriteString(cursor + "(none)\n")
		if len(m.createTargets) == 0 {
			b.WriteString(dimStyle.Render("  (no compatible targets found)"))
			b.WriteString("\n")
		} else {
			for i, target := range m.createTargets {
				cursor := "  "
				if i+1 == m.createCursor {
					cursor = activeStyle.Render("> ")
				}
				b.WriteString(cursor + target + "\n")
			}
		}

	case createStepSelectWorker:
		b.WriteString(dimStyle.Render("Creating target: " + m.createName))
		b.WriteString("\n\n")
		b.WriteString("Select worker:\n\n")
		// First option is always "(no worker)"
		cursor := "  "
		if m.createCursor == 0 {
			cursor = activeStyle.Render("> ")
		}
		b.WriteString(cursor + "(no worker)\n")
		for i, worker := range m.createWorkers {
			cursor := "  "
			if i+1 == m.createCursor {
				cursor = activeStyle.Render("> ")
			}
			b.WriteString(cursor + worker + "\n")
		}

	case createStepSelectProvider:
		b.WriteString(dimStyle.Render("Creating target: " + m.createName))
		b.WriteString("\n")
		if m.createWorker != "" {
			b.WriteString(dimStyle.Render("Worker: " + m.createWorker))
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString("Select provider type:\n\n")
		options := []string{"Kubernetes", "Terraform", "FluxOCIWriter"}
		for i, opt := range options {
			cursor := "  "
			if i == m.createCursor {
				cursor = activeStyle.Render("> ")
			}
			b.WriteString(cursor + opt + "\n")
		}

	case createStepConfirm:
		b.WriteString("Confirm creation:\n\n")
		b.WriteString(fmt.Sprintf("  Type:     %s\n", m.createType))
		b.WriteString(fmt.Sprintf("  Name:     %s\n", m.createName))
		if m.createSpace != "" && m.createType != "space" {
			b.WriteString(fmt.Sprintf("  Space:    %s\n", m.createSpace))
		}
		if m.createCloneFrom != "" {
			b.WriteString(fmt.Sprintf("  Clone:    %s\n", m.createCloneFrom))
		}
		if m.createTarget != "" {
			b.WriteString(fmt.Sprintf("  Target:   %s\n", m.createTarget))
		}
		if m.createWorker != "" {
			b.WriteString(fmt.Sprintf("  Worker:   %s\n", m.createWorker))
		}
		if m.createType == "target" {
			b.WriteString(fmt.Sprintf("  Provider: %s\n", m.createProvider))
		}
		b.WriteString("\n")
		options := []string{"Yes, create", "No, cancel"}
		for i, opt := range options {
			cursor := "  "
			if i == m.createCursor {
				cursor = activeStyle.Render("> ")
			}
			b.WriteString(cursor + opt + "\n")
		}

	case createStepCreating:
		b.WriteString(dimStyle.Render("Creating " + m.createType + "..."))

	case createStepComplete:
		b.WriteString(statusOK.Render(iconCheckOK + " " + titleCase(m.createType) + " created successfully!"))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("Press Enter to continue"))
	}

	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("↑↓ navigate  enter select  esc cancel"))

	return b.String()
}

func (m Model) renderImportWizard() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render(" IMPORT WIZARD "))
	b.WriteString("\n\n")

	// Show progress indicator based on import source
	hasRunningWorker := m.importSelectedWorker != ""
	showTargetStep := hasRunningWorker || m.importStep == importStepWaitTarget
	var steps []string
	if m.importSource == importSourceArgoCD {
		// ArgoCD flow: Source → ArgoApp → Space → Worker → (Target) → Units → Cleanup → Test → Done
		if showTargetStep {
			steps = []string{"Source", "ArgoApp", "Space", "Worker", "Target", "Units", "Cleanup", "Test", "Done"}
		} else {
			steps = []string{"Source", "ArgoApp", "Space", "Worker", "Units", "Cleanup", "Test", "Done"}
		}
	} else {
		// Kubernetes flow: Source → Namespace → Space → Worker → (Target) → Units → Test → Done
		if showTargetStep {
			steps = []string{"Source", "Namespace", "Space", "Worker", "Target", "Units", "Test", "Done"}
		} else {
			steps = []string{"Source", "Namespace", "Space", "Worker", "Units", "Test", "Done"}
		}
	}
	currentStepIdx := 0
	switch m.importStep {
	case importStepSource:
		currentStepIdx = 0
	case importStepNamespace, importStepArgoApps:
		currentStepIdx = 1
	case importStepSetup, importStepCreateSpace:
		currentStepIdx = 2
	case importStepCreateWorker:
		currentStepIdx = 3
	case importStepWaitTarget:
		currentStepIdx = 4
	case importStepDiscovering, importStepSelection, importStepUnitStructure:
		if showTargetStep {
			currentStepIdx = 5
		} else {
			currentStepIdx = 4
		}
	case importStepExtractConfig:
		if showTargetStep {
			currentStepIdx = 5 // Same as selection - part of "Units" step
		} else {
			currentStepIdx = 4
		}
	case importStepImporting, importStepArgoCleanup:
		if showTargetStep {
			currentStepIdx = 6
		} else {
			currentStepIdx = 5
		}
	case importStepTest:
		if m.importSource == importSourceArgoCD {
			if showTargetStep {
				currentStepIdx = 7
			} else {
				currentStepIdx = 6
			}
		} else {
			if showTargetStep {
				currentStepIdx = 6
			} else {
				currentStepIdx = 5
			}
		}
	case importStepComplete:
		if m.importSource == importSourceArgoCD {
			if showTargetStep {
				currentStepIdx = 8
			} else {
				currentStepIdx = 7
			}
		} else {
			if showTargetStep {
				currentStepIdx = 7
			} else {
				currentStepIdx = 6
			}
		}
	}
	var stepIndicator strings.Builder
	for i, step := range steps {
		if i == currentStepIdx {
			stepIndicator.WriteString(activeStyle.Render(fmt.Sprintf("[%s]", step)))
		} else if i < currentStepIdx {
			stepIndicator.WriteString(statusOK.Render(fmt.Sprintf("✓ %s", step)))
		} else {
			stepIndicator.WriteString(dimStyle.Render(step))
		}
		if i < len(steps)-1 {
			stepIndicator.WriteString(dimStyle.Render(" → "))
		}
	}
	b.WriteString(stepIndicator.String())
	b.WriteString("\n\n")

	switch m.importStep {
	case importStepSource:
		b.WriteString("Select the import source:\n\n")
		options := []struct {
			name string
			desc string
		}{
			{"Kubernetes Namespace", "Import workloads from a namespace (native, Helm, Flux)"},
			{"ArgoCD Application", "Import resources managed by ArgoCD"},
		}
		for i, opt := range options {
			cursor := dimStyle.Render("  ")
			if i == m.importCursor {
				cursor = activeStyle.Render("▸ ")
			}
			b.WriteString(fmt.Sprintf("%s%s\n", cursor, groupStyle.Render(opt.name)))
			b.WriteString(fmt.Sprintf("   %s\n", dimStyle.Render(opt.desc)))
		}
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("↑↓ navigate  Enter select  Esc cancel"))

	case importStepArgoApps:
		b.WriteString("Select an ArgoCD Application to import:\n\n")
		if m.importLoading {
			b.WriteString(dimStyle.Render("Loading ArgoCD Applications...\n\n"))
		} else if m.importError != nil && len(m.importArgoApps) == 0 {
			// Only show error as blocking if we have no apps to show
			b.WriteString(statusErr.Render(fmt.Sprintf("Error: %v\n\n", m.importError)))
			b.WriteString(dimStyle.Render("Press Esc to go back"))
		} else if len(m.importArgoApps) == 0 {
			b.WriteString(dimStyle.Render("No ArgoCD Applications found.\n"))
			b.WriteString(dimStyle.Render("Make sure ArgoCD is installed and Applications exist.\n\n"))
			b.WriteString(dimStyle.Render("Press Esc to go back"))
		} else {
			// Show warning if there's an error (e.g., App of Apps selected)
			if m.importError != nil {
				b.WriteString(statusWarn.Render("⚠ ") + statusWarn.Render(fmt.Sprintf("%v", m.importError)) + "\n\n")
			}
			// Show column headers
			b.WriteString(dimStyle.Render(fmt.Sprintf("   %-25s %-10s %-10s %s", "NAME", "SYNC", "HEALTH", "DESTINATION")) + "\n")
			b.WriteString(dimStyle.Render(fmt.Sprintf("   %-25s %-10s %-10s %s", "----", "----", "------", "-----------")) + "\n")
			for i, app := range m.importArgoApps {
				cursor := dimStyle.Render("  ")
				if i == m.importCursor {
					cursor = activeStyle.Render("▸ ")
				}
				// Color sync status
				syncStyle := dimStyle
				switch app.SyncStatus {
				case "Synced":
					syncStyle = statusOK
				case "OutOfSync":
					syncStyle = statusWarn
				case "Unknown":
					syncStyle = statusErr
				}
				// Color health status
				healthStyle := dimStyle
				switch app.HealthStatus {
				case "Healthy":
					healthStyle = statusOK
				case "Progressing":
					healthStyle = statusWarn
				case "Degraded", "Missing":
					healthStyle = statusErr
				}
				// Shorten server if local
				destServer := app.DestServer
				if destServer == "https://kubernetes.default.svc" {
					destServer = "local"
				}
				dest := fmt.Sprintf("%s:%s", destServer, app.DestNS)
				if len(dest) > 30 {
					dest = dest[:27] + "..."
				}
				// Use padRight to maintain column alignment with styled text
				displayName := app.Name
				if app.IsAppOfApps {
					displayName = app.Name + " " + statusWarn.Render("[AoA]")
				}
				name := padRight(displayName, 25)
				sync := padRight(syncStyle.Render(app.SyncStatus), 10)
				health := padRight(healthStyle.Render(app.HealthStatus), 10)
				b.WriteString(fmt.Sprintf("%s%s %s %s %s\n",
					cursor,
					name,
					sync,
					health,
					dimStyle.Render(dest),
				))
			}
			b.WriteString("\n")
			b.WriteString(dimStyle.Render("↑↓ navigate  Enter select  Esc cancel"))
		}

	case importStepNamespace:
		b.WriteString("Select a Kubernetes namespace to import:\n\n")
		if len(m.importNamespaces) == 0 {
			// No namespaces available - show helpful message
			if m.importError != nil {
				b.WriteString(statusErr.Render("Could not list namespaces from cluster.\n"))
				b.WriteString(dimStyle.Render("Check that kubectl is configured and you have access.\n\n"))
			} else if m.importLoading {
				b.WriteString(dimStyle.Render("Loading namespaces...\n\n"))
			} else {
				b.WriteString(dimStyle.Render("No namespaces found in the cluster.\n\n"))
			}
			b.WriteString(dimStyle.Render("Press Esc to cancel"))
		} else {
			// Get filtered namespaces
			filtered := getFilteredNamespaces(m.importNamespaces, m.importShowAllNS)
			hiddenCount := countHiddenNamespaces(m.importNamespaces)

			if len(filtered) == 0 && !m.importShowAllNS {
				b.WriteString(dimStyle.Render("No namespaces with workloads found.\n"))
				b.WriteString(dimStyle.Render(fmt.Sprintf("(%d empty/system namespaces hidden)\n\n", hiddenCount)))
				b.WriteString(dimStyle.Render("Press 'a' to show all namespaces"))
			} else {
				for i, ns := range filtered {
					cursor := dimStyle.Render("  ")
					if i == m.importCursor {
						cursor = activeStyle.Render("▸ ")
					}
					// Build workload count info
					total := ns.Deployments + ns.StatefulSet + ns.DaemonSets
					var details string
					if total == 0 {
						details = dimStyle.Render("(no workloads)")
					} else {
						// Build owner breakdown with colors (per TUI design guidelines)
						// Native=214 (yellow), Flux=81 (cyan), Argo=141 (purple), Helm=208 (orange), ConfigHub=82 (green)
						var ownerParts []string
						if ns.ArgoCount > 0 {
							ownerParts = append(ownerParts, lipgloss.NewStyle().Foreground(lipgloss.Color("141")).Render(fmt.Sprintf("%d Argo", ns.ArgoCount)))
						}
						if ns.FluxCount > 0 {
							ownerParts = append(ownerParts, lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Render(fmt.Sprintf("%d Flux", ns.FluxCount)))
						}
						if ns.HelmCount > 0 {
							ownerParts = append(ownerParts, lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Render(fmt.Sprintf("%d Helm", ns.HelmCount)))
						}
						if ns.ConfigHubCount > 0 {
							ownerParts = append(ownerParts, lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Render(fmt.Sprintf("%d ConfigHub", ns.ConfigHubCount)))
						}
						if ns.NativeCount > 0 {
							ownerParts = append(ownerParts, lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render(fmt.Sprintf("%d Native", ns.NativeCount)))
						}

						if len(ownerParts) > 0 {
							details = strings.Join(ownerParts, dimStyle.Render(", "))
						} else {
							details = dimStyle.Render(fmt.Sprintf("%d workloads", total))
						}
					}
					b.WriteString(fmt.Sprintf("%s%-20s %s\n", cursor, ns.Name, details))
				}
				b.WriteString("\n")
				// Show hidden count and toggle hint
				if !m.importShowAllNS && hiddenCount > 0 {
					b.WriteString(dimStyle.Render(fmt.Sprintf("(%d empty/system namespaces hidden)\n", hiddenCount)))
				}
				if m.importShowAllNS {
					b.WriteString(dimStyle.Render("↑↓ navigate  Enter select  a hide empty/system  Esc cancel"))
				} else {
					b.WriteString(dimStyle.Render("↑↓ navigate  Enter select  a show all  Esc cancel"))
				}
			}
		}

	case importStepSetup:
		if m.importSource == importSourceArgoCD && m.importSelectedArgo != nil {
			b.WriteString(fmt.Sprintf("ArgoCD Application: %s\n", activeStyle.Render(m.importSelectedArgo.Name)))
			b.WriteString(fmt.Sprintf("Target Namespace: %s\n", dimStyle.Render(m.importSelectedArgo.DestNS)))
			b.WriteString(fmt.Sprintf("Resources: %d\n\n", len(m.importWorkloads)))
		} else {
			b.WriteString(fmt.Sprintf("Namespace: %s\n\n", activeStyle.Render(m.importNamespace)))
		}
		b.WriteString("Choose where to import:\n\n")

		// Option 0: Create new space
		cursor := dimStyle.Render("  ")
		if m.importCursor == 0 {
			cursor = activeStyle.Render("▸ ")
		}
		b.WriteString(fmt.Sprintf("%s%s %s\n", cursor, groupStyle.Render("+ Create new space"), dimStyle.Render("(creates space, target, and units)")))

		// Options 1+: Existing spaces
		for i, space := range m.importExistingSpaces {
			cursor = dimStyle.Render("  ")
			if m.importCursor == i+1 {
				cursor = activeStyle.Render("▸ ")
			}
			b.WriteString(fmt.Sprintf("%s%s\n", cursor, space))
		}
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("↑↓ navigate  Enter select  Esc cancel"))

	case importStepCreateSpace:
		b.WriteString(fmt.Sprintf("Namespace: %s\n\n", activeStyle.Render(m.importNamespace)))
		b.WriteString("Create a new ConfigHub space:\n\n")
		b.WriteString("Space name: ")
		b.WriteString(activeStyle.Render(m.importNewSpaceName))
		b.WriteString("█\n\n")
		b.WriteString(dimStyle.Render("This will create:"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("  1. Space: ") + m.importNewSpaceName + "\n")
		b.WriteString(dimStyle.Render("  2. Worker: ") + m.importNewWorkerName + "\n")
		b.WriteString(dimStyle.Render("  3. Units for each workload\n"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("Type to edit name  Enter create  Esc cancel"))

	case importStepCreateWorker:
		b.WriteString(fmt.Sprintf("Namespace: %s\n", activeStyle.Render(m.importNamespace)))
		b.WriteString(fmt.Sprintf("Space: %s\n\n", activeStyle.Render(m.importSpace)))

		if m.importLoading && m.statusMsg != "" {
			// Show loading status when starting/waiting for worker
			b.WriteString(m.statusMsg + "\n\n")
			if strings.Contains(m.statusMsg, "Starting") {
				b.WriteString(dimStyle.Render("The worker will run in background to connect to your cluster.\n"))
			} else if strings.Contains(m.statusMsg, "Waiting") {
				b.WriteString(dimStyle.Render("Waiting for the worker to register its capabilities...\n"))
				b.WriteString(dimStyle.Render("(This usually takes a few seconds)\n"))
			}
		} else {
			b.WriteString("Create a worker for this space:\n\n")
			b.WriteString("Worker name: ")
			b.WriteString(activeStyle.Render(m.importNewWorkerName))
			b.WriteString("█\n\n")
			b.WriteString(dimStyle.Render("The worker connects ConfigHub to your Kubernetes cluster.\n"))
			b.WriteString(dimStyle.Render("It will be started automatically after creation.\n\n"))
			b.WriteString(dimStyle.Render("Type to edit name  Enter create  Esc cancel"))
		}

	case importStepWaitTarget:
		b.WriteString(fmt.Sprintf("Namespace: %s\n", activeStyle.Render(m.importNamespace)))
		b.WriteString(fmt.Sprintf("Space: %s  Worker: %s\n\n", activeStyle.Render(m.importSpace), activeStyle.Render(m.importNewWorkerName)))
		b.WriteString("Waiting for target to be auto-created...\n\n")
		kubeCtx := getCurrentKubeContext()
		expectedSlug := fmt.Sprintf("%s-kubernetes-yaml-%s", m.importNewWorkerName, strings.ReplaceAll(kubeCtx, "/", "-"))
		b.WriteString(dimStyle.Render("Expected target: ") + expectedSlug + "\n")
		b.WriteString(dimStyle.Render("Kubernetes context: ") + kubeCtx + "\n\n")
		b.WriteString(dimStyle.Render("Targets are automatically created when the worker discovers your cluster.\n"))
		b.WriteString(dimStyle.Render("This usually takes a few seconds...\n"))

	case importStepDiscovering:
		b.WriteString(fmt.Sprintf("Namespace: %s\n", activeStyle.Render(m.importNamespace)))
		b.WriteString(fmt.Sprintf("Space: %s\n", activeStyle.Render(m.importSpace)))
		b.WriteString(fmt.Sprintf("Target: %s\n\n", activeStyle.Render(m.importNewTargetName)))
		b.WriteString("Discovering workloads...\n")

	case importStepSelection:
		b.WriteString(fmt.Sprintf("Namespace: %s  Space: %s  Target: %s\n\n",
			activeStyle.Render(m.importNamespace),
			activeStyle.Render(m.importSpace),
			activeStyle.Render(m.importNewTargetName)))

		// Filter to show only new (unconnected) workloads
		var newWorkloads []int
		for i, w := range m.importWorkloads {
			if w.UnitSlug == "" {
				newWorkloads = append(newWorkloads, i)
			}
		}

		if len(newWorkloads) == 0 {
			b.WriteString("No new workloads to import.\n")
			b.WriteString(dimStyle.Render("All workloads are already connected to ConfigHub.\n\n"))
			b.WriteString(dimStyle.Render("Press Esc to return"))
		} else if m.importGroupedView && m.importSuggestion != nil {
			// Grouped view - show workloads organized by app/variant with suggested unit slugs
			viewIndicator := dimStyle.Render("[g] flat view")
			b.WriteString(fmt.Sprintf("Select workloads to import as Units (%d available):  %s\n\n", len(newWorkloads), viewIndicator))

			// Build a map of workload name to index for selection lookup
			workloadIdx := make(map[string]int)
			for i, w := range m.importWorkloads {
				workloadIdx[w.Namespace+"/"+w.Name] = i
			}

			// Calculate selected count
			selected := 0
			for _, idx := range newWorkloads {
				if m.importSelected[idx] {
					selected++
				}
			}

			// Render the suggested structure as a tree
			lineNum := 0
			for _, app := range m.importSuggestion.Apps {
				for _, variant := range app.Variants {
					// Check if any workload in this variant is new (unconnected)
					hasNewWorkloads := false
					variantSelectedCount := 0
					variantTotalCount := 0
					for _, w := range variant.Workloads {
						if idx, ok := workloadIdx[w.Namespace+"/"+w.Name]; ok {
							if w.UnitSlug == "" {
								hasNewWorkloads = true
								variantTotalCount++
								if m.importSelected[idx] {
									variantSelectedCount++
								}
							}
						}
					}
					if !hasNewWorkloads {
						continue
					}

					// Unit header line
					cursor := dimStyle.Render("  ")
					if lineNum == m.importCursor {
						cursor = activeStyle.Render("▸ ")
					}

					// Show unit slug with selection status
					unitCheck := dimStyle.Render("[ ]")
					if variantSelectedCount == variantTotalCount {
						unitCheck = activeStyle.Render("[x]")
					} else if variantSelectedCount > 0 {
						unitCheck = statusWarn.Render("[~]") // Partial selection
					}

					unitInfo := fmt.Sprintf("unit: %s", variant.UnitSlug)
					b.WriteString(fmt.Sprintf("%s%s %s  %s\n",
						cursor, unitCheck,
						groupStyle.Render(app.Name),
						dimStyle.Render(unitInfo)))
					lineNum++

					// Workloads under this unit
					for _, w := range variant.Workloads {
						if w.UnitSlug != "" {
							continue // Skip already connected
						}
						idx, ok := workloadIdx[w.Namespace+"/"+w.Name]
						if !ok {
							continue
						}

						cursor = dimStyle.Render("    ") // Indented
						if lineNum == m.importCursor {
							cursor = activeStyle.Render("  ▸ ")
						}

						check := dimStyle.Render("[ ]")
						if m.importSelected[idx] {
							check = activeStyle.Render("[x]")
						}

						kindShort := strings.ToLower(w.Kind)
						if len(kindShort) > 6 {
							kindShort = kindShort[:6]
						}
						b.WriteString(fmt.Sprintf("%s%s %s %s\n", padRight(cursor, 2), padRight(check, 3), padRight(kindShort, 6), w.Name))
						lineNum++
					}
				}
			}

			// Show labels source hint
			b.WriteString("\n")
			b.WriteString(dimStyle.Render("Labels from: app.kubernetes.io/name, namespace patterns\n"))
			b.WriteString(fmt.Sprintf("Selected: %d/%d\n\n", selected, len(newWorkloads)))
			b.WriteString(dimStyle.Render("Space toggle  a select all  g flat view  Enter import  Esc cancel"))
		} else {
			// Flat view - original behavior
			viewIndicator := dimStyle.Render("[g] grouped view")
			b.WriteString(fmt.Sprintf("Select workloads to import as Units (%d available):  %s\n\n", len(newWorkloads), viewIndicator))

			// Determine cursor position in filtered list
			filteredCursor := 0
			for i, idx := range newWorkloads {
				if idx == m.importCursor {
					filteredCursor = i
					break
				}
			}

			for i, idx := range newWorkloads {
				w := m.importWorkloads[idx]
				cursor := dimStyle.Render("  ")
				if i == filteredCursor {
					cursor = activeStyle.Render("▸ ")
				}
				check := dimStyle.Render("[ ]")
				if m.importSelected[idx] {
					check = activeStyle.Render("[x]")
				}
				kindShort := w.Kind
				if len(kindShort) > 6 {
					kindShort = kindShort[:6]
				}
				b.WriteString(fmt.Sprintf("%s%s %s %s %s\n", padRight(cursor, 2), padRight(check, 3), padRight(kindShort, 6), padRight(w.Name, 20), dimStyle.Render(w.Owner)))
			}
			b.WriteString("\n")
			selected := 0
			for _, idx := range newWorkloads {
				if m.importSelected[idx] {
					selected++
				}
			}
			b.WriteString(fmt.Sprintf("Selected: %d/%d\n\n", selected, len(newWorkloads)))
			b.WriteString(dimStyle.Render("Space toggle  a select all  g grouped view  Enter import  Esc cancel"))
		}

	case importStepUnitStructure:
		// Show unit structure choice for ArgoCD imports
		b.WriteString(fmt.Sprintf("Namespace: %s  Space: %s\n\n",
			activeStyle.Render(m.importNamespace),
			activeStyle.Render(m.importSpace)))

		selected := m.getSelectedWorkloads()
		b.WriteString("How should the resources be organized?\n\n")

		// Combined unit option
		cursor := dimStyle.Render("  ")
		if m.importCursor == unitStructureCombined {
			cursor = activeStyle.Render("▸ ")
		}
		check := dimStyle.Render("○")
		if m.importCursor == unitStructureCombined {
			check = activeStyle.Render("●")
		}
		combinedSlug := ""
		if m.importSelectedArgo != nil {
			combinedSlug = fmt.Sprintf("%s-workload", m.importSelectedArgo.Name)
		}
		b.WriteString(fmt.Sprintf("%s%s %s\n", cursor, check, activeStyle.Render("Combined unit")))
		b.WriteString(fmt.Sprintf("     %s\n", dimStyle.Render(fmt.Sprintf("All %d resources → %s", len(selected), combinedSlug))))
		b.WriteString("\n")

		// Individual units option
		cursor = dimStyle.Render("  ")
		if m.importCursor == unitStructureIndividual {
			cursor = activeStyle.Render("▸ ")
		}
		check = dimStyle.Render("○")
		if m.importCursor == unitStructureIndividual {
			check = activeStyle.Render("●")
		}
		b.WriteString(fmt.Sprintf("%s%s %s\n", cursor, check, "Individual units"))
		b.WriteString(fmt.Sprintf("     %s\n", dimStyle.Render(fmt.Sprintf("Each resource → separate unit (%d units)", len(selected)))))

		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("↑/↓ select  Enter confirm  Esc cancel"))

	case importStepExtractConfig:
		b.WriteString(fmt.Sprintf("Namespace: %s  Space: %s\n\n",
			activeStyle.Render(m.importNamespace),
			activeStyle.Render(m.importSpace)))

		if m.importLoading {
			b.WriteString("Extracting GitOps configuration...\n")
		} else if m.importViewingConfig {
			// Show full config view for a single workload
			idx := m.importViewConfigIdx
			if idx < len(m.importWorkloads) {
				w := m.importWorkloads[idx]
				b.WriteString(groupStyle.Render(fmt.Sprintf("Configuration for: %s", w.Name)))
				b.WriteString("\n\n")
				if w.ExtractedConfig != "" {
					// Show the config with syntax highlighting hint
					lines := strings.Split(w.ExtractedConfig, "\n")
					maxLines := 20 // Limit display
					for i, line := range lines {
						if i >= maxLines {
							b.WriteString(dimStyle.Render(fmt.Sprintf("... (%d more lines)\n", len(lines)-maxLines)))
							break
						}
						if strings.HasPrefix(line, "#") {
							b.WriteString(dimStyle.Render(line))
						} else {
							b.WriteString(line)
						}
						b.WriteString("\n")
					}
				} else if w.ConfigError != nil {
					b.WriteString(statusErr.Render(fmt.Sprintf("Error: %v\n", w.ConfigError)))
				} else {
					b.WriteString(dimStyle.Render("No GitOps configuration to extract.\n"))
				}
				b.WriteString("\n")
				b.WriteString(dimStyle.Render("Press Esc to return to list"))
			}
		} else {
			// Show extraction results
			b.WriteString("Config Extraction Preview:\n\n")

			selected := m.getSelectedWorkloads()
			gitopsCount := 0
			for _, w := range selected {
				if w.GitOpsRef != nil {
					gitopsCount++
				}
			}

			if gitopsCount == 0 {
				b.WriteString(dimStyle.Render("No GitOps sources found. Units will be created empty.\n\n"))
			} else {
				b.WriteString(fmt.Sprintf("Found %d GitOps sources (%d extracted successfully):\n\n",
					gitopsCount, m.importExtractSuccess))
			}

			for i, w := range selected {
				cursor := dimStyle.Render("  ")
				if i == m.importCursor {
					cursor = activeStyle.Render("▸ ")
				}

				var status string
				var statusIcon string
				if w.GitOpsRef == nil {
					statusIcon = dimStyle.Render("○")
					status = dimStyle.Render("Native (no GitOps)")
				} else if w.ExtractedConfig != "" {
					statusIcon = statusOK.Render("✓")
					// Show brief summary of what was extracted
					lines := strings.Count(w.ExtractedConfig, "\n")
					status = statusOK.Render(fmt.Sprintf("%s %s/%s (%d lines)",
						w.GitOpsRef.Kind, w.GitOpsRef.Namespace, w.GitOpsRef.Name, lines))
				} else if w.ConfigError != nil {
					statusIcon = statusErr.Render("✗")
					status = statusErr.Render(fmt.Sprintf("Failed: %v", w.ConfigError))
				} else {
					statusIcon = statusWarn.Render("⚠")
					status = statusWarn.Render("No config extracted")
				}

				// Use padRight for styled elements to maintain alignment
				b.WriteString(fmt.Sprintf("%s%s %s %s\n", padRight(cursor, 2), padRight(statusIcon, 1), padRight(w.Name, 20), status))
			}

			b.WriteString("\n")
			b.WriteString(dimStyle.Render("↑↓ navigate  v view config  Enter proceed  Esc back"))
		}

	case importStepImporting:
		b.WriteString(fmt.Sprintf("Importing %d workloads into space '%s'...\n", m.importTotal, m.importSpace))

	case importStepArgoCleanup:
		b.WriteString(statusOK.Render("Import Complete!") + "\n\n")
		b.WriteString(fmt.Sprintf("Imported %d units from ArgoCD Application '%s'\n\n",
			m.importProgress, m.importSelectedArgo.Name))

		if m.importLoading {
			b.WriteString(dimStyle.Render("Processing ArgoCD cleanup...\n"))
		} else {
			b.WriteString("What would you like to do with the ArgoCD Application?\n\n")

			options := []struct {
				name string
				desc string
			}{
				{"Disable auto-sync", "Keep Application for reference, but stop syncing"},
				{"Delete Application", "Remove ArgoCD control (resources preserved)"},
				{"Keep as-is", "Leave Application unchanged (may conflict with ConfigHub)"},
			}
			for i, opt := range options {
				cursor := dimStyle.Render("  ")
				if i == m.importCursor {
					cursor = activeStyle.Render("▸ ")
				}
				style := dimStyle
				if i == argoCleanupKeepAsIs {
					style = statusWarn // Warn about potential conflicts
				}
				b.WriteString(fmt.Sprintf("%s%s\n", cursor, groupStyle.Render(opt.name)))
				b.WriteString(fmt.Sprintf("   %s\n", style.Render(opt.desc)))
			}
			b.WriteString("\n")
			b.WriteString(dimStyle.Render("↑↓ navigate  Enter select"))
		}

	case importStepTest:
		b.WriteString(statusOK.Render("Import Complete!") + "\n\n")

		// Show what was imported
		if m.importSource == importSourceArgoCD && m.importSelectedArgo != nil {
			b.WriteString(fmt.Sprintf("Imported %d units from ArgoCD Application '%s'\n\n",
				m.importProgress, m.importSelectedArgo.Name))
		} else {
			b.WriteString(fmt.Sprintf("Imported %d units into space '%s'\n\n",
				m.importProgress, m.importSpace))
		}

		// Show apply error warning if apply failed
		if m.importApplyError != nil {
			b.WriteString(statusWarn.Render("⚠ Apply failed") + "\n")
			b.WriteString(dimStyle.Render(m.importApplyError.Error()) + "\n\n")
			b.WriteString(dimStyle.Render("Pipeline test may not work without successful apply.") + "\n\n")
		}

		if m.importLoading {
			b.WriteString(groupStyle.Render("Testing ConfigHub Pipeline") + "\n\n")

			// Show what the test is doing
			b.WriteString("  " + activeStyle.Render("◐") + " " + dimStyle.Render("Waiting for livedata") + "\n")
			b.WriteString("    " + dimStyle.Render("Worker is applying unit to target...") + "\n")
			b.WriteString("  " + dimStyle.Render("○ Add restart annotation to pod template") + "\n")
			b.WriteString("  " + dimStyle.Render("○ Update unit via ConfigHub") + "\n")
			b.WriteString("  " + dimStyle.Render("○ Apply change to cluster") + "\n")
			b.WriteString("\n")
			b.WriteString(dimStyle.Render("Typical: 10-30 seconds. Max wait: 2 minutes.") + "\n")
			b.WriteString(dimStyle.Render("If slow, ensure worker is running and connected."))
		} else {
			b.WriteString("Test ConfigHub Pipeline?\n\n")
			b.WriteString(dimStyle.Render("Verify ConfigHub can update your resources via the cub CLI.\n\n"))

			options := []struct {
				name string
				desc string
			}{
				{"Test pipeline", "Add annotation + trigger rollout to verify ConfigHub can manage resources"},
				{"Skip", "Continue without testing"},
			}
			for i, opt := range options {
				cursor := "  "
				if i == m.importCursor {
					cursor = activeStyle.Render("▸ ")
				}
				// Use padRight for cursor to maintain alignment
				b.WriteString(fmt.Sprintf("%s%s\n", padRight(cursor, 2), groupStyle.Render(opt.name)))
				b.WriteString(fmt.Sprintf("   %s\n", dimStyle.Render(opt.desc)))
			}
			b.WriteString("\n")
			b.WriteString(dimStyle.Render("↑↓ navigate  Enter select"))
		}

	case importStepComplete:
		b.WriteString(statusOK.Render("Import Complete!") + "\n\n")

		// Summary - use consistent formatting with checkmarks aligned
		if m.importCreateNewSpace {
			b.WriteString(fmt.Sprintf("%s Created space: %s\n", statusOK.Render("✓"), activeStyle.Render(m.importSpace)))
			b.WriteString(fmt.Sprintf("%s Created worker: %s\n", statusOK.Render("✓"), activeStyle.Render(m.importNewWorkerName)))
			if m.importSelectedWorker != "" {
				// Worker was started automatically and is running
				b.WriteString(fmt.Sprintf("%s Worker started and running\n", statusOK.Render("✓")))
				if m.importNewTargetName != "" {
					b.WriteString(fmt.Sprintf("%s Target auto-created: %s\n", statusOK.Render("✓"), activeStyle.Render(m.importNewTargetName)))
				}
			}
		} else if m.importSelectedWorker != "" {
			b.WriteString(fmt.Sprintf("%s Using existing worker: %s\n", statusOK.Render("✓"), activeStyle.Render(m.importSelectedWorker)))
			if m.importNewTargetName != "" {
				b.WriteString(fmt.Sprintf("%s Target auto-created: %s\n", statusOK.Render("✓"), activeStyle.Render(m.importNewTargetName)))
			}
		}

		if m.importProgress == m.importTotal {
			b.WriteString(fmt.Sprintf("%s Imported %d workloads as units\n", statusOK.Render("✓"), m.importProgress))
		} else {
			b.WriteString(fmt.Sprintf("%s Imported %d/%d workloads as units\n", statusWarn.Render("⚠"), m.importProgress, m.importTotal))
		}

		// Show test result
		if m.importTestRan {
			if m.importTestResult != nil && m.importTestResult.Success {
				b.WriteString(fmt.Sprintf("%s Pipeline test passed: %s\n", statusOK.Render("✓"), m.importTestResult.Message))
			} else if m.importError != nil {
				b.WriteString(fmt.Sprintf("%s Pipeline test failed: %v\n", statusErr.Render("✗"), m.importError))
			} else {
				b.WriteString(fmt.Sprintf("%s Pipeline test failed\n", statusErr.Render("✗")))
			}
		} else {
			b.WriteString(fmt.Sprintf("%s Pipeline test: skipped\n", dimStyle.Render("○")))
		}

		b.WriteString("\n")

		// Next steps depend on whether everything is set up
		if m.importSelectedWorker != "" && m.importNewTargetName != "" {
			// Everything is ready - worker is running and target was auto-created
			b.WriteString(statusOK.Render("Your namespace is ready!") + "\n\n")
			b.WriteString(dimStyle.Render("The worker is running in this terminal session.") + "\n")
			b.WriteString(dimStyle.Render("To restart it later - or somewhere else, run:") + "\n\n")
			b.WriteString(fmt.Sprintf("  %s\n\n", activeStyle.Render(fmt.Sprintf("cub worker run %s --space %s", m.importNewWorkerName, m.importSpace))))
		} else {
			// Worker was created but didn't become ready or target wasn't found
			b.WriteString(dimStyle.Render("Next steps:") + "\n\n")
			b.WriteString(dimStyle.Render("Start the worker (in a separate terminal):") + "\n\n")
			b.WriteString(fmt.Sprintf("  %s\n\n", activeStyle.Render(fmt.Sprintf("cub worker run %s --space %s", m.importNewWorkerName, m.importSpace))))
			b.WriteString(dimStyle.Render("The target will be auto-created when the worker discovers your cluster.") + "\n\n")
		}

		// Show URL to open in browser
		b.WriteString(dimStyle.Render("View in browser:") + "\n")
		b.WriteString(fmt.Sprintf("  %s\n\n", activeStyle.Render(m.getSpaceURL())))

		b.WriteString(dimStyle.Render("o open in browser  Enter close"))
	}

	// Show error at bottom (except for namespace step where it's shown inline)
	if m.importError != nil && m.importStep != importStepNamespace {
		b.WriteString("\n\n")
		b.WriteString(statusErr.Render(fmt.Sprintf("Error: %v", m.importError)))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("Press Esc to cancel"))
	}

	return b.String()
}

func (m *Model) rebuildFlatList() {
	m.flatList = nil
	m.clearMatchCache() // Clear cache when rebuilding

	for _, node := range m.nodes {
		// When filter is active and we have a search query, skip nodes that don't match
		if m.filterActive && m.searchQuery != "" {
			if !m.nodeOrDescendantMatches(node) {
				continue
			}
		}
		m.flatList = append(m.flatList, node)
		if node.Expanded {
			if m.hubViewMode && node.Type == "org" {
				// Hub/AppSpace view: group spaces
				m.addHubAppSpaceView(node)
			} else {
				m.addChildrenToFlatList(node, 1)
			}
		}
	}

	// Inject pending creates into the flat list for optimistic UI
	m.injectPendingCreates()

	if m.cursor >= len(m.flatList) {
		m.cursor = len(m.flatList) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}

	// Update search matches for the new flat list
	if m.searchQuery != "" {
		m.updateSearchMatches()
	}
}

func (m *Model) addChildrenToFlatList(node *TreeNode, depth int) {
	for _, child := range node.Children {
		// When filter is active, skip children that don't match and have no matching descendants
		if m.filterActive && m.searchQuery != "" {
			if !m.nodeOrDescendantMatches(child) {
				continue
			}
		}
		// Skip nodes that are being deleted (optimistic UI)
		if m.isNodeDeleting(child) {
			continue
		}
		// When showAllUnits is false, filter units to current cluster only
		if !m.showAllUnits && child.Type == "unit" {
			if !m.unitMatchesCurrentCluster(child) {
				continue
			}
		}
		m.flatList = append(m.flatList, child)
		if child.Expanded {
			m.addChildrenToFlatList(child, depth+1)
		}
	}
}

// unitMatchesCurrentCluster checks if a unit's target matches the current cluster
func (m *Model) unitMatchesCurrentCluster(node *TreeNode) bool {
	if m.currentCluster == "" {
		return true // No cluster detected, show all
	}

	unitData, ok := node.Data.(CubUnitData)
	if !ok {
		return true // Not a unit, don't filter
	}

	// Check if the unit's target matches the current cluster
	targetSlug := unitData.Target.Slug
	if targetSlug == "" {
		return true // No target, show it (might be unconfigured)
	}

	// Use the matchesCluster helper for flexible matching
	return matchesCluster(targetSlug, m.currentCluster)
}

// addHubAppSpaceView adds spaces grouped into Hub (platform) and AppSpaces (apps)
func (m *Model) addHubAppSpaceView(orgNode *TreeNode) {
	// Categorize spaces into Hub (platform) vs AppSpaces
	var hubSpaces, appSpaces []*TreeNode
	for _, child := range orgNode.Children {
		if child.Type != "space" {
			continue
		}
		spaceName := child.Name
		// Hub spaces: platform-*, infra-*, hub-*, shared-*
		if strings.HasPrefix(spaceName, "platform-") ||
			strings.HasPrefix(spaceName, "infra-") ||
			strings.HasPrefix(spaceName, "hub-") ||
			strings.HasPrefix(spaceName, "shared-") {
			hubSpaces = append(hubSpaces, child)
		} else {
			appSpaces = append(appSpaces, child)
		}
	}

	// Create virtual Hub group node
	if len(hubSpaces) > 0 {
		hubGroup := &TreeNode{
			ID:       orgNode.ID + "/hub",
			Name:     "🏢 Hub (Platform)",
			Type:     "hub_group",
			Info:     fmt.Sprintf("(%d spaces)", len(hubSpaces)),
			Parent:   orgNode,
			Expanded: true,
			OrgID:    orgNode.OrgID,
		}
		m.flatList = append(m.flatList, hubGroup)
		for _, space := range hubSpaces {
			if m.filterActive && m.searchQuery != "" && !m.nodeOrDescendantMatches(space) {
				continue
			}
			if m.isNodeDeleting(space) {
				continue
			}
			m.flatList = append(m.flatList, space)
			if space.Expanded {
				m.addChildrenToFlatList(space, 2)
			}
		}
	}

	// Create virtual AppSpaces group node
	if len(appSpaces) > 0 {
		appGroup := &TreeNode{
			ID:       orgNode.ID + "/appspaces",
			Name:     "📦 AppSpaces (Teams)",
			Type:     "app_group",
			Info:     fmt.Sprintf("(%d spaces)", len(appSpaces)),
			Parent:   orgNode,
			Expanded: true,
			OrgID:    orgNode.OrgID,
		}
		m.flatList = append(m.flatList, appGroup)
		for _, space := range appSpaces {
			if m.filterActive && m.searchQuery != "" && !m.nodeOrDescendantMatches(space) {
				continue
			}
			if m.isNodeDeleting(space) {
				continue
			}
			m.flatList = append(m.flatList, space)
			if space.Expanded {
				m.addChildrenToFlatList(space, 2)
			}
		}
	}
}

// Optimistic UI helper methods

// addPendingAction adds a new pending action for optimistic UI updates
func (m *Model) addPendingAction(actionType, nodeType, name, parentID string) {
	m.pendingActions = append(m.pendingActions, PendingAction{
		ActionType: actionType,
		NodeType:   nodeType,
		Name:       name,
		ParentID:   parentID,
		StartTime:  time.Now(),
	})
}

// removePendingAction removes a pending action by type and name
func (m *Model) removePendingAction(nodeType, name string) {
	for i, pa := range m.pendingActions {
		if pa.NodeType == nodeType && pa.Name == name {
			m.pendingActions = append(m.pendingActions[:i], m.pendingActions[i+1:]...)
			return
		}
	}
}

// isNodeDeleting checks if a node is currently being deleted
func (m Model) isNodeDeleting(node *TreeNode) bool {
	for _, pa := range m.pendingActions {
		if pa.ActionType == "deleting" && pa.NodeType == node.Type && pa.Name == node.Name {
			return true
		}
	}
	return false
}

// injectPendingCreates adds synthetic nodes for pending create operations
func (m *Model) injectPendingCreates() {
	for _, pa := range m.pendingActions {
		if pa.ActionType != "creating" {
			continue
		}
		// Find the parent group node and inject the pending node
		for i, node := range m.flatList {
			// For units/targets, look for the appropriate group under the space
			if pa.NodeType == "unit" && node.Type == "group" && strings.HasSuffix(node.ID, "/units") {
				if parent := node.Parent; parent != nil && parent.Name == pa.ParentID {
					pendingNode := &TreeNode{
						ID:     "__pending_" + pa.Name,
						Name:   pa.Name,
						Type:   "unit",
						Status: "pending",
						Info:   "Creating...",
						Parent: node,
						OrgID:  node.OrgID,
					}
					// Insert after the group node if expanded
					if node.Expanded {
						insertPos := i + 1
						// Find end of this group's children
						for j := i + 1; j < len(m.flatList); j++ {
							if m.flatList[j].Parent == node {
								insertPos = j + 1
							} else {
								break
							}
						}
						m.flatList = append(m.flatList[:insertPos], append([]*TreeNode{pendingNode}, m.flatList[insertPos:]...)...)
					}
					break
				}
			}
			if pa.NodeType == "target" && node.Type == "group" && strings.HasSuffix(node.ID, "/targets") {
				if parent := node.Parent; parent != nil && parent.Name == pa.ParentID {
					pendingNode := &TreeNode{
						ID:     "__pending_" + pa.Name,
						Name:   pa.Name,
						Type:   "target",
						Status: "pending",
						Info:   "Creating...",
						Parent: node,
						OrgID:  node.OrgID,
					}
					if node.Expanded {
						insertPos := i + 1
						for j := i + 1; j < len(m.flatList); j++ {
							if m.flatList[j].Parent == node {
								insertPos = j + 1
							} else {
								break
							}
						}
						m.flatList = append(m.flatList[:insertPos], append([]*TreeNode{pendingNode}, m.flatList[insertPos:]...)...)
					}
					break
				}
			}
			// For spaces, look for the org node
			if pa.NodeType == "space" && node.Type == "org" {
				orgData, ok := node.Data.(CubOrganization)
				if ok && (orgData.Slug == pa.ParentID || orgData.ExternalID == pa.ParentID) {
					pendingNode := &TreeNode{
						ID:       "__pending_" + pa.Name,
						Name:     pa.Name,
						Type:     "space",
						Status:   "pending",
						Info:     "Creating...",
						Parent:   node,
						OrgID:    node.OrgID,
						Expanded: false,
					}
					if node.Expanded {
						insertPos := i + 1
						for j := i + 1; j < len(m.flatList); j++ {
							if m.flatList[j].Parent == node {
								insertPos = j + 1
							} else {
								break
							}
						}
						m.flatList = append(m.flatList[:insertPos], append([]*TreeNode{pendingNode}, m.flatList[insertPos:]...)...)
					}
					break
				}
			}
		}
	}
}

// removeNodeFromTree removes a node from the tree structure (for delete completion)
func (m *Model) removeNodeFromTree(nodeType, name, parentID string) {
	// Find and remove from parent's children
	for _, node := range m.nodes {
		if m.removeNodeRecursive(node, nodeType, name, parentID) {
			return
		}
	}
}

func (m *Model) removeNodeRecursive(node *TreeNode, nodeType, name, parentID string) bool {
	for i, child := range node.Children {
		if child.Type == nodeType && child.Name == name {
			// Check if parent matches (for units/targets under a space)
			if nodeType == "unit" || nodeType == "target" {
				if node.Type == "group" && node.Parent != nil && node.Parent.Name == parentID {
					node.Children = append(node.Children[:i], node.Children[i+1:]...)
					return true
				}
			} else if nodeType == "space" && node.Type == "org" {
				node.Children = append(node.Children[:i], node.Children[i+1:]...)
				return true
			}
		}
		if m.removeNodeRecursive(child, nodeType, name, parentID) {
			return true
		}
	}
	return false
}

// insertCreatedNode adds a newly created node to the tree (for create completion)
func (m *Model) insertCreatedNode(nodeType, name, parentID string) {
	newNode := &TreeNode{
		ID:       name,
		Name:     name,
		Type:     nodeType,
		Status:   "ok",
		Expanded: false,
	}

	// Find the appropriate parent and add the node
	for _, orgNode := range m.nodes {
		if nodeType == "space" {
			orgData, ok := orgNode.Data.(CubOrganization)
			if ok && (orgData.Slug == parentID || orgData.ExternalID == parentID) {
				newNode.Parent = orgNode
				newNode.OrgID = orgNode.OrgID
				orgNode.Children = append(orgNode.Children, newNode)
				return
			}
		}
		// For units/targets, need to find the space then the group
		if m.insertNodeRecursive(orgNode, newNode, nodeType, parentID) {
			return
		}
	}
}

func (m *Model) insertNodeRecursive(node *TreeNode, newNode *TreeNode, nodeType, parentID string) bool {
	// Looking for a space with matching name
	if node.Type == "space" && node.Name == parentID {
		// Find the appropriate group (Units or Targets)
		for _, child := range node.Children {
			if child.Type == "group" {
				if (nodeType == "unit" && strings.HasSuffix(child.ID, "/units")) ||
					(nodeType == "target" && strings.HasSuffix(child.ID, "/targets")) {
					newNode.Parent = child
					newNode.OrgID = node.OrgID
					child.Children = append(child.Children, newNode)
					return true
				}
			}
		}
	}
	for _, child := range node.Children {
		if m.insertNodeRecursive(child, newNode, nodeType, parentID) {
			return true
		}
	}
	return false
}

// updateSpaceData updates a space's children with loaded data (units, targets, workers)
// This preserves the tree structure and expanded state
func (m *Model) updateSpaceData(spaceSlug string, units []CubUnitData, targets []CubTargetData, workers []CubWorkerData) {
	// Find the space node in the tree
	for _, orgNode := range m.nodes {
		for _, spaceNode := range orgNode.Children {
			if spaceNode.Type == "space" && spaceNode.Name == spaceSlug {
				// Find the group nodes
				for _, groupNode := range spaceNode.Children {
					switch {
					case strings.HasSuffix(groupNode.ID, "/units"):
						// Clear existing children and add new ones
						groupNode.Children = nil
						for _, unit := range units {
							unitNode := &TreeNode{
								ID:     unit.Unit.Slug,
								Name:   unit.Unit.Slug,
								Type:   "unit",
								Parent: groupNode,
								Data:   unit,
								OrgID:  spaceNode.OrgID,
							}
							unitNode.Status = unit.DeriveStatus()
							unitNode.Info = buildUnitInfo(unit)
							groupNode.Children = append(groupNode.Children, unitNode)
						}

					case strings.HasSuffix(groupNode.ID, "/targets"):
						groupNode.Children = nil
						for _, target := range targets {
							targetNode := &TreeNode{
								ID:     target.Target.Slug,
								Name:   target.Target.Slug,
								Type:   "target",
								Status: "ok",
								Info:   target.Target.ProviderType,
								Parent: groupNode,
								Data:   target,
								OrgID:  spaceNode.OrgID,
							}
							groupNode.Children = append(groupNode.Children, targetNode)
						}

					case strings.HasSuffix(groupNode.ID, "/workers"):
						groupNode.Children = nil
						for _, worker := range workers {
							workerNode := &TreeNode{
								ID:     worker.BridgeWorker.Slug,
								Name:   worker.BridgeWorker.Slug,
								Type:   "worker",
								Status: worker.DeriveStatus(),
								Info:   worker.BridgeWorker.Condition,
								Parent: groupNode,
								Data:   worker,
								OrgID:  spaceNode.OrgID,
							}
							groupNode.Children = append(groupNode.Children, workerNode)
						}
					}
				}
				return
			}
		}
	}
}

// loadEntityDetailsCmd fetches full details for an entity and formats for display
func loadEntityDetailsCmd(node *TreeNode) tea.Cmd {
	return func() tea.Msg {
		if node == nil {
			return detailsLoadedMsg{
				node: node,
				err:  fmt.Errorf("no node selected"),
			}
		}

		// For hub_group and app_group, show pattern context
		if node.Type == "hub_group" || node.Type == "app_group" {
			content := formatGroupPatternContext(node)
			return detailsLoadedMsg{
				node:    node,
				content: content,
				err:     nil,
			}
		}

		// For units, fetch rich details using cub unit get
		if node.Type == "unit" {
			unitData, ok := node.Data.(CubUnitData)
			if ok {
				spaceSlug := unitData.Space.Slug
				unitSlug := unitData.Unit.Slug

				// Fetch full unit details
				output, err := runCubCommand("unit", "get", "--space", spaceSlug, "--json", unitSlug)
				if err == nil {
					// Parse and format the detailed output
					content := formatUnitDetails(output, unitData)
					return detailsLoadedMsg{
						node:    node,
						content: content,
						err:     nil,
					}
				}
				// Fall through to basic display if fetch fails
			}
		}

		// Default: show cached data as formatted JSON
		if node.Data == nil {
			return detailsLoadedMsg{
				node: node,
				err:  fmt.Errorf("no data available for this node"),
			}
		}

		jsonBytes, err := json.MarshalIndent(node.Data, "", "  ")
		if err != nil {
			return detailsLoadedMsg{
				node: node,
				err:  fmt.Errorf("failed to format JSON: %w", err),
			}
		}

		return detailsLoadedMsg{
			node:    node,
			content: string(jsonBytes),
			err:     nil,
		}
	}
}

// formatUnitDetails creates a rich formatted view of unit details
func formatUnitDetails(jsonData []byte, basicData CubUnitData) string {
	var b strings.Builder

	// Parse the full response to extract any additional fields
	var fullData map[string]interface{}
	if err := json.Unmarshal(jsonData, &fullData); err != nil {
		// Fallback to basic formatting
		b.WriteString("Unit: " + basicData.Unit.Slug + "\n\n")
		jsonBytes, _ := json.MarshalIndent(basicData, "", "  ")
		return b.String() + string(jsonBytes)
	}

	// Header section
	b.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	b.WriteString("UNIT DETAILS\n")
	b.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	// Basic info
	b.WriteString("Name:        " + basicData.Unit.Slug + "\n")
	b.WriteString("Toolchain:   " + basicData.Unit.ToolchainType + "\n")
	b.WriteString("Space:       " + basicData.Space.Slug + "\n")
	b.WriteString("\n")

	// Revisions
	b.WriteString("REVISIONS\n")
	b.WriteString("─────────────────────────────────────\n")
	b.WriteString(fmt.Sprintf("Head:        %d\n", basicData.Unit.HeadRevisionNum))
	b.WriteString(fmt.Sprintf("Live:        %d\n", basicData.Unit.LiveRevisionNum))
	if basicData.Unit.HeadRevisionNum != basicData.Unit.LiveRevisionNum {
		b.WriteString("⚠️  Drift detected (head ≠ live)\n")
	}
	b.WriteString("\n")

	// Status
	b.WriteString("STATUS\n")
	b.WriteString("─────────────────────────────────────\n")
	b.WriteString("Status:      " + basicData.UnitStatus.Status + "\n")
	b.WriteString("Sync:        " + basicData.UnitStatus.SyncStatus + "\n")
	if basicData.UnitStatus.Drift != "" {
		b.WriteString("Drift:       " + basicData.UnitStatus.Drift + "\n")
	}
	if basicData.UnitStatus.Action != "" {
		b.WriteString("Action:      " + basicData.UnitStatus.Action + "\n")
	}
	b.WriteString("\n")

	// Target info
	if basicData.Target.Slug != "" {
		b.WriteString("TARGET\n")
		b.WriteString("─────────────────────────────────────\n")
		b.WriteString("Target:      " + basicData.Target.Slug + "\n")
		b.WriteString("Provider:    " + basicData.Target.ProviderType + "\n")
		b.WriteString("\n")
	}

	// Worker info
	if basicData.BridgeWorker.Slug != "" {
		b.WriteString("WORKER\n")
		b.WriteString("─────────────────────────────────────\n")
		b.WriteString("Worker:      " + basicData.BridgeWorker.Slug + "\n")
		b.WriteString("Condition:   " + basicData.BridgeWorker.Condition + "\n")
		if basicData.BridgeWorker.IPAddress != "" {
			b.WriteString("IP Address:  " + basicData.BridgeWorker.IPAddress + "\n")
		}
		b.WriteString("\n")
	}

	// Check for labels in the full data
	if unit, ok := fullData["Unit"].(map[string]interface{}); ok {
		if labels, ok := unit["Labels"].(map[string]interface{}); ok && len(labels) > 0 {
			b.WriteString("LABELS\n")
			b.WriteString("─────────────────────────────────────\n")
			for key, val := range labels {
				b.WriteString(fmt.Sprintf("%-12s %v\n", key+":", val))
			}
			b.WriteString("\n")
		}
	}

	// Show raw JSON for any extra fields not captured above
	b.WriteString("RAW DATA\n")
	b.WriteString("─────────────────────────────────────\n")
	prettyJSON, _ := json.MarshalIndent(fullData, "", "  ")
	b.WriteString(string(prettyJSON))

	return b.String()
}

// formatGroupPatternContext creates context about Hub/AppSpace patterns
func formatGroupPatternContext(node *TreeNode) string {
	var b strings.Builder

	if node.Type == "hub_group" {
		b.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		b.WriteString("HUB (PLATFORM)\n")
		b.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

		// Count children
		spaceCount := len(node.Children)
		b.WriteString(fmt.Sprintf("%d spaces containing shared infrastructure\n\n", spaceCount))

		b.WriteString("WHAT IS HUB?\n")
		b.WriteString("─────────────────────────────────────\n")
		b.WriteString("Platform team's shared configuration:\n")
		b.WriteString("• Base templates (upstream for clones)\n")
		b.WriteString("• Shared infrastructure (cert-manager, etc)\n")
		b.WriteString("• Workers and Targets\n")
		b.WriteString("• Org-wide policies and constraints\n\n")

		b.WriteString("DETECTED PATTERN\n")
		b.WriteString("─────────────────────────────────────\n")

		// Detect pattern from space names
		pattern := detectPatternFromSpaces(node.Children)
		b.WriteString(pattern)
		b.WriteString("\n")

		b.WriteString("SPACES IN HUB\n")
		b.WriteString("─────────────────────────────────────\n")
		for _, child := range node.Children {
			if child.Type == "space" {
				b.WriteString("• " + child.Name)
				if child.Info != "" {
					b.WriteString(" (" + child.Info + ")")
				}
				b.WriteString("\n")
			}
		}

	} else if node.Type == "app_group" {
		b.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		b.WriteString("APPSPACES (TEAMS)\n")
		b.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

		// Count children
		spaceCount := len(node.Children)
		b.WriteString(fmt.Sprintf("%d team workspaces\n\n", spaceCount))

		b.WriteString("WHAT ARE APPSPACES?\n")
		b.WriteString("─────────────────────────────────────\n")
		b.WriteString("Application team workspaces:\n")
		b.WriteString("• One deployer per space (Flux OR Argo)\n")
		b.WriteString("• Units with labels (app, variant, region)\n")
		b.WriteString("• Team-specific saved queries\n")
		b.WriteString("• Reconciliation rules per variant\n\n")

		b.WriteString("DETECTED PATTERN\n")
		b.WriteString("─────────────────────────────────────\n")

		// Detect pattern from space names
		pattern := detectPatternFromSpaces(node.Children)
		b.WriteString(pattern)
		b.WriteString("\n")

		b.WriteString("APPSPACES\n")
		b.WriteString("─────────────────────────────────────\n")
		for _, child := range node.Children {
			if child.Type == "space" {
				b.WriteString("• " + child.Name)
				if child.Info != "" {
					b.WriteString(" (" + child.Info + ")")
				}
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n")
	b.WriteString("─────────────────────────────────────\n")
	b.WriteString("Press B to toggle Hub/AppSpace view\n")
	b.WriteString("See: docs/map/reference/hub-appspace-examples.md\n")

	return b.String()
}

// detectPatternFromSpaces analyzes space names to detect common patterns
func detectPatternFromSpaces(spaces []*TreeNode) string {
	if len(spaces) == 0 {
		return "No pattern detected (no spaces)\n"
	}

	var names []string
	for _, s := range spaces {
		if s.Type == "space" {
			names = append(names, s.Name)
		}
	}

	// Check for known patterns
	hasPlatform := false
	hasBase := false
	hasInfra := false
	hasDevStagingProd := false
	hasRegions := false
	hasClusters := false

	devCount, stagingCount, prodCount := 0, 0, 0
	regionCount := 0

	for _, name := range names {
		lower := strings.ToLower(name)
		if strings.Contains(lower, "platform") {
			hasPlatform = true
		}
		if strings.Contains(lower, "base") {
			hasBase = true
		}
		if strings.Contains(lower, "infra") {
			hasInfra = true
		}
		if strings.Contains(lower, "-dev") || strings.HasSuffix(lower, "-dev") {
			devCount++
		}
		if strings.Contains(lower, "staging") {
			stagingCount++
		}
		if strings.Contains(lower, "-prod") || strings.HasSuffix(lower, "-prod") {
			prodCount++
		}
		if strings.Contains(lower, "-asia") || strings.Contains(lower, "-eu") || strings.Contains(lower, "-us") {
			regionCount++
		}
		if strings.Contains(lower, "cluster-") || strings.Contains(lower, ".example.") {
			hasClusters = true
		}
	}

	if devCount > 0 && prodCount > 0 {
		hasDevStagingProd = true
	}
	if regionCount >= 2 {
		hasRegions = true
	}

	var b strings.Builder

	// Determine pattern
	if hasClusters && (hasBase || hasInfra) {
		b.WriteString("Pattern: Banko (Flux)\n")
		b.WriteString("• Cluster-per-directory structure\n")
		b.WriteString("• Versioned platform components\n")
		b.WriteString("• platform/ → Hub, clusters/* → AppSpaces\n")
	} else if hasBase && hasDevStagingProd {
		b.WriteString("Pattern: Arnie (ArgoCD)\n")
		b.WriteString("• Folders-per-environment\n")
		b.WriteString("• Promotion = file copy\n")
		b.WriteString("• base/ → Hub, envs/* → AppSpaces\n")
	} else if hasRegions && (hasBase || hasInfra) {
		b.WriteString("Pattern: TraderX (Multi-region)\n")
		b.WriteString("• Base/Infra Hub + regional AppSpaces\n")
		b.WriteString("• Labels: variant, region\n")
	} else if hasPlatform && hasDevStagingProd {
		b.WriteString("Pattern: KubeCon Demo\n")
		b.WriteString("• Platform team + App teams\n")
		b.WriteString("• platform-* → Hub, app*-dev/prod → AppSpaces\n")
	} else if hasBase && hasInfra && hasDevStagingProd {
		b.WriteString("Pattern: curious-cub (Standard)\n")
		b.WriteString("• Base/Infra Hub\n")
		b.WriteString("• dev/staging/prod AppSpaces\n")
	} else if hasDevStagingProd {
		b.WriteString("Pattern: Environment-based\n")
		b.WriteString("• Spaces per environment (dev/staging/prod)\n")
		b.WriteString("• Consider adding base/infra Hub spaces\n")
	} else {
		b.WriteString("Pattern: Custom\n")
		b.WriteString("• No standard pattern detected\n")
		b.WriteString("• See docs for reference architectures\n")
	}

	return b.String()
}

// buildOrgSummary creates summary content for the current organization
func (m *Model) buildOrgSummary() string {
	var b strings.Builder

	// Find current org node
	var currentOrgNode *TreeNode
	for _, node := range m.nodes {
		if node.Type == "org" {
			orgData, ok := node.Data.(CubOrganization)
			if ok {
				isCurrentOrg := m.isCurrentOrg(orgData)
				if isCurrentOrg {
					currentOrgNode = node
					break
				}
			}
		}
	}

	if currentOrgNode == nil {
		return dimStyle.Render("No organization selected\n\nPress Enter on any node to view details")
	}

	orgData, _ := currentOrgNode.Data.(CubOrganization)

	b.WriteString(detailsHeaderStyle.Render("Organization Summary"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("Name:   %s\n", orgData.DisplayName))
	b.WriteString(fmt.Sprintf("Slug:   %s\n", orgData.Slug))
	b.WriteString(fmt.Sprintf("ID:     %s\n", orgData.OrganizationID))
	b.WriteString("\n")

	// Count spaces, units, workers, targets
	spaceCount := 0
	unitCount := 0
	workerCount := 0
	targetCount := 0

	for _, spaceNode := range currentOrgNode.Children {
		if spaceNode.Type == "space" {
			spaceCount++
			spaceData, ok := spaceNode.Data.(CubSpaceData)
			if ok {
				unitCount += spaceData.TotalUnitCount
				workerCount += spaceData.TotalBridgeWorkerCount
				for _, count := range spaceData.TargetCountByType {
					targetCount += count
				}
			}
		}
	}

	b.WriteString(fmt.Sprintf("Spaces:  %d\n", spaceCount))
	b.WriteString(fmt.Sprintf("Units:   %d\n", unitCount))
	b.WriteString(fmt.Sprintf("Targets: %d\n", targetCount))
	b.WriteString(fmt.Sprintf("Workers: %d\n", workerCount))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Press Enter on any node to view its details"))

	return b.String()
}

// buildSpaceSummary generates a summary view for a space node (shown when auto-focusing on current space)
func (m *Model) buildSpaceSummary(node *TreeNode) string {
	var b strings.Builder

	if node == nil || node.Type != "space" {
		return dimStyle.Render("No space selected")
	}

	spaceData, ok := node.Data.(CubSpaceData)
	if !ok {
		return dimStyle.Render("Invalid space data")
	}

	b.WriteString(detailsHeaderStyle.Render("Space: " + node.Name))
	b.WriteString("\n\n")

	// Space stats
	b.WriteString(fmt.Sprintf("Slug:    %s\n", node.ID))
	b.WriteString(fmt.Sprintf("Units:   %d\n", spaceData.TotalUnitCount))
	b.WriteString(fmt.Sprintf("Workers: %d\n", spaceData.TotalBridgeWorkerCount))

	// Target counts by type
	totalTargets := 0
	for _, count := range spaceData.TargetCountByType {
		totalTargets += count
	}
	b.WriteString(fmt.Sprintf("Targets: %d\n", totalTargets))
	b.WriteString("\n")

	// Show what's available
	b.WriteString(dimStyle.Render("This is your current space. Expand to see:"))
	b.WriteString("\n")
	b.WriteString("  • Units - your configuration units\n")
	b.WriteString("  • Workers - deployment agents\n")
	b.WriteString("  • Targets - deployment destinations\n")
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Press Enter to expand, Tab to view details"))

	return b.String()
}

// getDisconnectedWorkers returns a list of disconnected worker names
func (m *Model) getDisconnectedWorkers() []string {
	var disconnected []string

	// Walk through all nodes looking for workers
	for _, node := range m.nodes {
		if node.Type == "org" {
			for _, spaceNode := range node.Children {
				if spaceNode.Type == "space" {
					for _, groupNode := range spaceNode.Children {
						if groupNode.Name == "Workers" {
							for _, workerNode := range groupNode.Children {
								if workerNode.Type == "worker" {
									// Check condition from Info field or Data
									condition := workerNode.Info
									if condition == "Disconnected" || condition == "NotReady" || condition == "unknown" {
										disconnected = append(disconnected, workerNode.Name)
									}
								}
							}
						}
					}
				}
			}
		}
	}

	return disconnected
}

// getCurrentOrgDisplayName returns the display name of the currently selected org
func (m *Model) getCurrentOrgDisplayName() string {
	for _, node := range m.nodes {
		if node.Type == "org" {
			if orgData, ok := node.Data.(CubOrganization); ok {
				if m.isCurrentOrg(orgData) {
					return orgData.DisplayName
				}
			}
		}
	}
	return ""
}

// getOrgList returns all orgs with their current status
func (m *Model) getOrgList() []CubOrganization {
	var orgs []CubOrganization
	for _, node := range m.nodes {
		if node.Type == "org" {
			if orgData, ok := node.Data.(CubOrganization); ok {
				orgs = append(orgs, orgData)
			}
		}
	}
	return orgs
}

// renderWorkerStatus returns a compact worker status line for the header
func (m *Model) renderWorkerStatus() string {
	// Get all workers from tree
	var workers []struct {
		name      string
		condition string
	}

	for _, node := range m.nodes {
		if node.Type == "org" {
			for _, spaceNode := range node.Children {
				if spaceNode.Type == "space" {
					for _, groupNode := range spaceNode.Children {
						if groupNode.Name == "Workers" {
							for _, workerNode := range groupNode.Children {
								if workerNode.Type == "worker" {
									workers = append(workers, struct {
										name      string
										condition string
									}{workerNode.Name, workerNode.Info})
								}
							}
						}
					}
				}
			}
		}
	}

	if len(workers) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(dimStyle.Render("workers: "))

	for i, w := range workers {
		if i > 0 {
			b.WriteString(" ")
		}
		if w.condition == "Ready" {
			b.WriteString(statusOK.Render("●"))
			b.WriteString(w.name)
		} else {
			b.WriteString(statusErr.Render("○"))
			b.WriteString(dimStyle.Render(w.name))
		}
	}

	return b.String()
}

// renderWorkerWarning returns a warning banner if any workers are disconnected
func (m *Model) renderWorkerWarning() string {
	disconnected := m.getDisconnectedWorkers()
	if len(disconnected) == 0 {
		return ""
	}

	var b strings.Builder
	warningStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		Bold(true).
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("196")).
		Padding(0, 1).
		Width(m.width - 4)

	if len(disconnected) == 1 {
		b.WriteString(fmt.Sprintf("⚠ WARNING: Worker '%s' is disconnected!\n", disconnected[0]))
	} else {
		b.WriteString(fmt.Sprintf("⚠ WARNING: %d workers disconnected!\n", len(disconnected)))
		for _, w := range disconnected {
			b.WriteString(fmt.Sprintf("  • %s\n", w))
		}
	}
	b.WriteString("\nRun: cub worker run <worker-name>  |  : cub worker run <name>")

	return warningStyle.Render(b.String()) + "\n\n"
}

// renderModeHeader returns a mode header showing connection and filter state
func (m Model) renderModeHeader() string {
	// Style definitions
	modeConnectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("82")).
		Bold(true)
	modeHeaderStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))
	filterActiveStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("214")).
		Bold(true)

	var b strings.Builder

	// Mode: Connected to ConfigHub
	mode := modeConnectedStyle.Render("Connected")
	b.WriteString(mode)

	// Cluster name
	if m.currentCluster != "" {
		b.WriteString(modeHeaderStyle.Render(fmt.Sprintf(" │ Cluster: %s", m.currentCluster)))
	}

	// Context name (if different from cluster)
	if m.contextName != "" && m.contextName != m.currentCluster {
		b.WriteString(modeHeaderStyle.Render(fmt.Sprintf(" │ Context: %s", m.contextName)))
	}

	// Filter state
	b.WriteString(modeHeaderStyle.Render(" │ "))
	if m.showAllUnits {
		b.WriteString(modeHeaderStyle.Render("Showing: All Units"))
		b.WriteString(dimStyle.Render(" │ Press 'a' for this cluster"))
	} else {
		b.WriteString(filterActiveStyle.Render("Showing: This cluster only"))
		b.WriteString(dimStyle.Render(" │ Press 'a' for all"))
	}

	b.WriteString("\n")
	return b.String()
}

// getDetailsHeader returns a header string for the details pane
func (m *Model) getDetailsHeader(node *TreeNode) string {
	if node == nil {
		return "Organization Summary"
	}

	switch node.Type {
	case "org":
		return fmt.Sprintf("Organization: %s", node.Name)
	case "space":
		return fmt.Sprintf("Space: %s", node.Name)
	case "unit":
		return fmt.Sprintf("Unit: %s", node.Name)
	case "target":
		return fmt.Sprintf("Target: %s", node.Name)
	case "worker":
		return fmt.Sprintf("Worker: %s", node.Name)
	default:
		return node.Name
	}
}

// formatJSONWithSyntaxHighlight adds color to JSON output
func (m *Model) formatJSONWithSyntaxHighlight(jsonStr string) string {
	var b strings.Builder
	lines := strings.Split(jsonStr, "\n")

	for _, line := range lines {
		// Check if line contains a key-value pair
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				key := parts[0]
				value := parts[1]

				// Color the key (text before colon that starts with quotes)
				trimmedKey := strings.TrimSpace(key)
				if strings.HasPrefix(trimmedKey, "\"") {
					// Preserve leading whitespace
					leadingSpace := key[:len(key)-len(strings.TrimLeft(key, " \t"))]
					b.WriteString(leadingSpace)
					b.WriteString(jsonKeyStyle.Render(trimmedKey))
					b.WriteString(":")
					b.WriteString(m.colorJSONValue(value))
					b.WriteString("\n")
					continue
				}
			}
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	return strings.TrimSuffix(b.String(), "\n")
}

// colorJSONValue applies syntax highlighting to a JSON value
func (m *Model) colorJSONValue(value string) string {
	trimmed := strings.TrimSpace(value)

	// String value (starts with quote)
	if strings.HasPrefix(trimmed, "\"") {
		return jsonStringStyle.Render(value)
	}

	// Boolean
	clean := strings.TrimSuffix(trimmed, ",")
	if clean == "true" || clean == "false" {
		return jsonBoolStyle.Render(value)
	}

	// Null
	if clean == "null" {
		return dimStyle.Render(value)
	}

	// Number
	if _, err := strconv.ParseFloat(clean, 64); err == nil {
		return jsonNumberStyle.Render(value)
	}

	// Arrays and objects - keep as-is
	return value
}

func (m Model) View() string {
	if !m.ready {
		return m.spinner.View() + " Initializing..."
	}

	if m.loading {
		msg := "Loading ConfigHub data..."
		if m.statusMsg != "" {
			msg = m.statusMsg
		}
		return m.spinner.View() + " " + msg
	}

	if m.err != nil {
		errMsg := fmt.Sprintf("Error: %v\n\n", m.err)
		// Add login hint for auth-related errors
		errStr := m.err.Error()
		if strings.Contains(errStr, "organization") || strings.Contains(errStr, "unauthorized") ||
			strings.Contains(errStr, "auth") || strings.Contains(errStr, "401") {
			errMsg += "Hint: Try running 'cub auth login' to authenticate.\n\n"
		}
		errMsg += "Press q to quit, r to retry"
		return errMsg
	}

	// Import wizard mode
	if m.importMode {
		return m.renderImportWizard()
	}

	// Create wizard mode
	if m.createMode {
		return m.renderCreateWizard()
	}

	// Delete wizard mode
	if m.deleteMode {
		return m.renderDeleteWizard()
	}

	// Org selector popup
	if m.orgSelectMode {
		return m.renderOrgSelector()
	}

	// Help overlay
	if m.helpMode {
		return m.renderHelpOverlay()
	}

	// Activity view
	if m.activityMode {
		return m.renderActivityView()
	}

	// Three Maps view
	if m.mapsMode {
		return m.renderMapsView()
	}

	// Panel view (WET↔LIVE)
	if m.panelMode {
		return m.renderPanelView()
	}

	// Suggest view (recommend units from cluster)
	if m.suggestMode {
		return m.renderSuggestView()
	}

	var b strings.Builder

	// Header with org name and worker status
	header := headerStyle.Render(" ⚡ CONFIGHUB HIERARCHY ")

	// Add current org indicator
	orgName := m.getCurrentOrgDisplayName()
	var orgIndicator string
	if orgName != "" {
		orgStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("141")).
			Bold(true)
		orgIndicator = dimStyle.Render("org: ") + orgStyle.Render(orgName)
	}

	workerStatus := m.renderWorkerStatus()

	// Build header line: title | org | workers
	b.WriteString(header)
	if orgIndicator != "" || workerStatus != "" {
		// Calculate remaining space
		usedWidth := lipgloss.Width(header)
		rightContent := ""
		if orgIndicator != "" && workerStatus != "" {
			rightContent = orgIndicator + dimStyle.Render(" │ ") + workerStatus
		} else if orgIndicator != "" {
			rightContent = orgIndicator
		} else {
			rightContent = workerStatus
		}

		padding := m.width - usedWidth - lipgloss.Width(rightContent) - 4
		if padding < 2 {
			padding = 2
		}
		b.WriteString(strings.Repeat(" ", padding))
		b.WriteString(rightContent)
	}
	b.WriteString("\n\n")

	// Worker disconnect warning (if any workers are disconnected)
	if warning := m.renderWorkerWarning(); warning != "" {
		b.WriteString(warning)
	}

	// Mode header showing cluster filter state
	modeHeader := m.renderModeHeader()
	b.WriteString(modeHeader)

	// Auth prompt
	if m.authPrompt {
		b.WriteString(promptStyle.Render(fmt.Sprintf("Switch to organization '%s'?", m.authOrgName)))
		b.WriteString("\n\n")
		b.WriteString("This will switch to a context configured for this organization.")
		b.WriteString("\n\n")
		b.WriteString("[y] Yes, switch  [n] No, cancel")
		return b.String()
	}

	// Breadcrumb
	if m.cursor < len(m.flatList) {
		node := m.flatList[m.cursor]
		crumbs := m.buildBreadcrumb(node)
		b.WriteString(breadcrumbStyle.Render(crumbs))
		b.WriteString("\n")
	}

	// Status message
	if m.statusMsg != "" {
		b.WriteString(dimStyle.Render(m.statusMsg))
		b.WriteString("\n")
	}

	// Calculate pane dimensions for 50/50 split
	leftWidth := (m.width / 2) - 2
	rightWidth := m.width - leftWidth - 4
	contentHeight := m.height - 8 // Reserve space for header/footer

	// Left pane: Tree view
	treeContent := m.renderTree()
	leftPaneStyled := leftPaneStyle
	if !m.detailsFocused {
		leftPaneStyled = leftPaneStyled.BorderForeground(lipgloss.Color("212"))
	}
	leftPane := leftPaneStyled.
		Width(leftWidth).
		Height(contentHeight).
		Render(treeContent)

	// Right pane: Details
	var rightContent string
	if m.detailsLoading {
		rightContent = dimStyle.Render("Loading...")
	} else if m.detailsContent != "" {
		// Add header for the entity
		header := m.getDetailsHeader(m.detailsNode)
		rightContent = detailsHeaderStyle.Render(header) + "\n\n" + m.detailsPane.View()
	} else {
		// Show org summary by default
		rightContent = m.buildOrgSummary()
	}

	// Use highlighted style if focused
	paneStyle := rightPaneStyle
	if m.detailsFocused {
		paneStyle = rightPaneActiveStyle
	}
	rightPane := paneStyle.
		Width(rightWidth).
		Height(contentHeight).
		Render(rightContent)

	// Join panes horizontally
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, leftPane, " ", rightPane))
	b.WriteString("\n")

	// Command palette
	if m.cmdMode {
		b.WriteString(promptStyle.Render(":"))
		b.WriteString(m.cmdInput)
		b.WriteString("█")
		if len(m.cmdHistory) > 0 {
			b.WriteString("  ")
			b.WriteString(searchInfoStyle.Render("↑↓ history"))
		}
		b.WriteString("\n")
	} else if m.cmdShowOutput && m.cmdOutput != "" {
		// Show command output in a box
		outputLines := strings.Split(m.cmdOutput, "\n")
		maxLines := 8
		if len(outputLines) > maxLines {
			outputLines = outputLines[:maxLines]
			outputLines = append(outputLines, dimStyle.Render("..."))
		}
		for _, line := range outputLines {
			b.WriteString(dimStyle.Render("│ "))
			b.WriteString(line)
			b.WriteString("\n")
		}
		b.WriteString(searchInfoStyle.Render("Esc to dismiss"))
		b.WriteString("\n")
	} else if m.searchMode {
		// Search bar
		b.WriteString(dimStyle.Render("Filter: "))
		b.WriteString(m.searchQuery)
		b.WriteString("█")
		if len(m.searchMatches) > 0 {
			b.WriteString("  ")
			b.WriteString(searchInfoStyle.Render(fmt.Sprintf("[%d matches]", len(m.searchMatches))))
		} else if m.searchQuery != "" {
			b.WriteString("  ")
			b.WriteString(searchInfoStyle.Render("[no matches]"))
		}
		b.WriteString("\n")
	} else if m.searchQuery != "" {
		// Show search info even when not in search mode (after pressing Enter)
		filterStatus := "filter:on"
		if !m.filterActive {
			filterStatus = "filter:off"
		}
		matchInfo := fmt.Sprintf("[%d matches]", len(m.searchMatches))
		if len(m.searchMatches) == 0 {
			matchInfo = "[no matches]"
		}
		b.WriteString(searchInfoStyle.Render(fmt.Sprintf("/%s  %s  %s  f toggle  n/N jump  Esc clear", m.searchQuery, matchInfo, filterStatus)))
		b.WriteString("\n")
	}

	// Help bar
	dot := helpDotStyle.Render(" · ")
	item := func(key, action string) string {
		return helpKeyStyle.Render(key) + " " + helpActionStyle.Render(action)
	}

	var helpBar string
	if m.detailsFocused {
		helpBar = item("j/k", "scroll") + dot + item("d/u", "page") + dot + item("g/G", "top/bottom") + dot +
			item("⇥", "tree") + dot + item("q", "quit")
	} else if m.searchQuery != "" {
		helpBar = item("↑↓", "move") + dot + item("←→", "expand") + dot + item("⏎", "details") + dot +
			item("⇥", "pane") + dot + item("f", "filter") + dot + item("n/N", "match") + dot + item("q", "quit")
	} else {
		helpBar = item("↑↓", "move") + dot + item("←→", "expand") + dot + item("⏎", "details") + dot +
			item("⇥", "pane") + dot + item("/", "filter") + dot + item(":", "cmd") + dot +
			item("L", "local") + dot + item("?", "help") + dot + item("q", "quit")
	}

	b.WriteString("\n")
	b.WriteString(helpBar)

	return b.String()
}

func (m Model) buildBreadcrumb(node *TreeNode) string {
	var parts []string
	current := node
	for current != nil {
		parts = append([]string{current.Name}, parts...)
		current = current.Parent
	}
	return strings.Join(parts, " → ")
}

func (m Model) renderTree() string {
	var b strings.Builder

	for i, node := range m.flatList {
		// Calculate depth
		depth := 0
		parent := node.Parent
		for parent != nil {
			depth++
			parent = parent.Parent
		}

		// Cursor and search match indicator
		isMatch := m.isSearchMatch(i)
		if i == m.cursor {
			if isMatch {
				b.WriteString(searchMatchStyle.Render("▸ "))
			} else {
				b.WriteString(activeStyle.Render("▸ "))
			}
		} else if isMatch {
			b.WriteString(searchMatchStyle.Render("● "))
		} else {
			b.WriteString("  ")
		}

		// Indentation
		b.WriteString(strings.Repeat("  ", depth))

		// Expand/collapse icon
		hasChildren := len(node.Children) > 0 || node.Type == "group" || node.Type == "space" || node.Type == "org" || node.Type == "unit"
		if hasChildren {
			if node.Expanded {
				b.WriteString(iconExpanded + " ")
			} else {
				b.WriteString(iconCollapsed + " ")
			}
		} else {
			b.WriteString("  ")
		}

		// Handle pending status (optimistic UI for creates)
		if node.Status == "pending" {
			b.WriteString(dimStyle.Render("⟳ " + node.Name + " (creating...)"))
			b.WriteString("\n")
			continue
		}

		// Type-specific icon and styling
		switch node.Type {
		case "org":
			// Status icon for orgs
			orgData, ok := node.Data.(CubOrganization)
			if ok {
				isCurrentOrg := m.isCurrentOrg(orgData)
				if isCurrentOrg {
					b.WriteString(activeStyle.Render(iconActive) + " ")
					b.WriteString(activeStyle.Render(node.Name))
				} else {
					b.WriteString(dimStyle.Render(iconInactive) + " ")
					b.WriteString(node.Name)
					b.WriteString(dimStyle.Render(" (switch org)"))
				}
			}

		case "space":
			// Status icon
			if icon := renderStatusIcon(node.Status); icon != "" {
				b.WriteString(icon)
			} else {
				b.WriteString(dimStyle.Render(iconInactive) + " ")
			}
			b.WriteString(node.Name)

		case "group":
			b.WriteString(groupStyle.Render(node.Name))

		case "hub_group", "app_group":
			// Virtual grouping nodes for Hub/AppSpace view
			b.WriteString(groupStyle.Render(node.Name))

		case "unit":
			if icon := renderStatusIcon(node.Status); icon != "" {
				b.WriteString(icon)
			} else {
				b.WriteString("  ")
			}
			b.WriteString(node.Name)

		case "target":
			b.WriteString(statusOK.Render(iconCheckOK) + " ")
			b.WriteString(node.Name)

		case "worker":
			if icon := renderStatusIcon(node.Status); icon != "" {
				b.WriteString(icon)
			} else {
				b.WriteString(statusWarn.Render(iconCheckWarn) + " ")
			}
			b.WriteString(node.Name)

		case "detail":
			// Detail rows show label: value format
			switch node.Status {
			case "ok":
				b.WriteString(statusOK.Render("•") + " ")
			case "warn":
				b.WriteString(statusWarn.Render("•") + " ")
			case "error":
				b.WriteString(statusErr.Render("•") + " ")
			default:
				b.WriteString(dimStyle.Render("•") + " ")
			}
			b.WriteString(dimStyle.Render(node.Name + ":"))

		default:
			b.WriteString(node.Name)
		}

		// Info text
		if node.Info != "" {
			b.WriteString("  ")
			b.WriteString(dimStyle.Render(node.Info))
		}

		b.WriteString("\n")
	}

	return b.String()
}

// Command
var hierarchyCmd = &cobra.Command{
	Use:        "hierarchy",
	Short:      "Interactive ConfigHub hierarchy explorer (deprecated: use 'map --hub')",
	Deprecated: "use 'cub-scout map --hub' or 'cub-scout map hub' instead",
	Long: `Launch an interactive TUI to explore your ConfigHub hierarchy.

DEPRECATED: This command is deprecated. Use 'cub-scout map --hub' or 'cub-scout map hub' instead.

Navigate through Organizations, Spaces, Units, Targets, and Workers in a tree view.

Navigation:
  ↑/k, ↓/j     Move up/down
  ←/h          Collapse node or go to parent
  →/l, Enter   Expand node (prompts to switch org if needed)
  /            Filter - type to filter, hides non-matching nodes while preserving hierarchy
  f            Toggle filter on/off (when search query is active)
  n/N          Jump to next/previous match
  i            Import workloads from Kubernetes (opens wizard)
  Esc          Clear filter
  r            Refresh data
  q            Quit

The current organization and space are highlighted.
Expanding a different organization will prompt you to switch context.
Units show their sync status: ✓ (ok), ⚠ (drifted), ✗ (error)
`,
	RunE: runHierarchy,
}

func init() {
	rootCmd.AddCommand(hierarchyCmd)
}

func runHierarchy(cmd *cobra.Command, args []string) error {
	if _, err := exec.LookPath("cub"); err != nil {
		return fmt.Errorf("cub CLI not found. Install from: https://docs.confighub.com/cli")
	}

	if _, err := runCubCommand("context", "get"); err != nil {
		return fmt.Errorf("not authenticated to ConfigHub. Run: cub auth login")
	}

	for {
		p := tea.NewProgram(initialModel(), tea.WithAltScreen())
		finalModel, err := p.Run()
		if err != nil {
			return fmt.Errorf("error running TUI: %w", err)
		}

		// Check if user requested import wizard
		if m, ok := finalModel.(Model); ok && m.launchImportWizard {
			// Run the import wizard
			if err := RunImportWizard(); err != nil {
				return fmt.Errorf("error running import wizard: %w", err)
			}
			// After wizard completes, restart hierarchy
			continue
		}

		// Check if user requested local cluster TUI
		if m, ok := finalModel.(Model); ok && m.launchLocalCluster {
			// Launch local cluster TUI (bash script)
			return runMapTUI(cmd, args)
		}

		// Normal exit
		break
	}

	return nil
}

// TreeNode methods are in hierarchy_types.go
var _ list.Item = (*TreeNode)(nil)
