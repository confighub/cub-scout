// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// Pattern types for GitOps organization
type RepoPattern struct {
	URL          string   `json:"url"`
	Owner        string   `json:"owner"`        // GitHub org/user
	Name         string   `json:"name"`         // Repo name
	Paths        []string `json:"paths"`        // Paths used
	Apps         []string `json:"apps"`         // Apps deployed from this repo
	Tool         string   `json:"tool"`         // Flux, ArgoCD
	PatternType  string   `json:"patternType"`  // monorepo, polyrepo, platform, external
	NamedPattern string   `json:"namedPattern"` // Arnie, Banko, Fluxy, D2, etc.
}

type EnvChain struct {
	AppName      string            `json:"appName"`
	Environments map[string]string `json:"environments"` // namespace -> image
}

type TeamPattern struct {
	Name       string   `json:"name"`
	Namespaces []string `json:"namespaces"`
	Workloads  int      `json:"workloads"`
}

type PatternsResult struct {
	Repos     []RepoPattern       `json:"repos"`
	EnvChains []EnvChain          `json:"envChains"`
	Teams     []TeamPattern       `json:"teams"`
	EnvGroups map[string][]string `json:"envGroups"` // prod/staging/dev -> namespaces
	Suggested SuggestedOrg        `json:"suggested"`
}

type SuggestedOrg struct {
	Organization string         `json:"organization"`
	Hubs         []SuggestedHub `json:"hubs"`
}

type SuggestedHub struct {
	Name      string           `json:"name"`
	AppSpaces []SuggestedSpace `json:"appSpaces"`
}

type SuggestedSpace struct {
	Name      string   `json:"name"`
	Workloads []string `json:"workloads"`
	Env       string   `json:"env"`
}

var mapPatternsCmd = &cobra.Command{
	Use:     "patterns",
	Aliases: []string{"repos", "structure"},
	Short:   "Analyze GitOps repository and organization patterns",
	Long: `Discover how your GitOps repositories are organized and suggest ConfigHub structure.

Shows:
- Repository patterns (monorepo, polyrepo, platform, external)
- Path conventions (D2-style, flux2-kustomize-helm, etc.)
- Environment chains (same app across dev/staging/prod)
- Team groupings (from namespace patterns)
- Suggested ConfigHub organization (Hubs, AppSpaces)

This helps you understand your GitOps structure and plan imports.

Examples:
  cub-scout map patterns             # Full analysis
  cub-scout map patterns --json      # JSON output for tooling
  cub-scout map patterns --verbose   # Include path details`,
	RunE: runMapPatterns,
}

func init() {
	mapCmd.AddCommand(mapPatternsCmd)
}

func runMapPatterns(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	cfg, err := buildConfig()
	if err != nil {
		return fmt.Errorf("build kubernetes config: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("create dynamic client: %w", err)
	}

	result := &PatternsResult{
		EnvGroups: make(map[string][]string),
	}

	// 1. Collect Flux GitRepositories
	fluxRepos := collectFluxRepos(ctx, dynClient)

	// 2. Collect Flux Kustomizations with their paths
	fluxKustomizations := collectFluxKustomizations(ctx, dynClient)

	// 3. Collect ArgoCD Applications
	argoApps := collectArgoApps(ctx, dynClient)

	// 4. Build repo patterns
	result.Repos = buildRepoPatterns(fluxRepos, fluxKustomizations, argoApps)

	// 5. Collect all deployments for env chain analysis
	deployments := collectDeployments(ctx, dynClient)

	// 6. Build environment chains
	result.EnvChains = buildEnvChains(deployments)

	// 7. Detect environment groups from namespaces
	result.EnvGroups = detectEnvGroups(ctx, dynClient)

	// 8. Detect team patterns
	result.Teams = detectTeamPatterns(ctx, dynClient, deployments)

	// 9. Suggest organization structure
	result.Suggested = suggestOrganization(result)

	if mapJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	printPatterns(result)
	return nil
}

func collectFluxRepos(ctx context.Context, client dynamic.Interface) map[string]string {
	repos := make(map[string]string) // name -> URL

	gvr := schema.GroupVersionResource{
		Group:    "source.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "gitrepositories",
	}

	list, err := client.Resource(gvr).Namespace("").List(ctx, v1.ListOptions{})
	if err != nil {
		return repos
	}

	for _, item := range list.Items {
		name := item.GetName()
		url, _, _ := unstructured.NestedString(item.Object, "spec", "url")
		if url != "" {
			repos[name] = url
		}
	}

	return repos
}

func collectFluxKustomizations(ctx context.Context, client dynamic.Interface) []map[string]string {
	var kustomizations []map[string]string

	gvr := schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}

	list, err := client.Resource(gvr).Namespace("").List(ctx, v1.ListOptions{})
	if err != nil {
		return kustomizations
	}

	for _, item := range list.Items {
		name := item.GetName()
		path, _, _ := unstructured.NestedString(item.Object, "spec", "path")
		sourceRef, _, _ := unstructured.NestedString(item.Object, "spec", "sourceRef", "name")

		kustomizations = append(kustomizations, map[string]string{
			"name":   name,
			"path":   path,
			"source": sourceRef,
		})
	}

	return kustomizations
}

func collectArgoApps(ctx context.Context, client dynamic.Interface) []map[string]string {
	var apps []map[string]string

	gvr := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}

	list, err := client.Resource(gvr).Namespace("").List(ctx, v1.ListOptions{})
	if err != nil {
		return apps
	}

	for _, item := range list.Items {
		name := item.GetName()
		repoURL, _, _ := unstructured.NestedString(item.Object, "spec", "source", "repoURL")
		path, _, _ := unstructured.NestedString(item.Object, "spec", "source", "path")
		destNS, _, _ := unstructured.NestedString(item.Object, "spec", "destination", "namespace")

		apps = append(apps, map[string]string{
			"name":      name,
			"repoURL":   repoURL,
			"path":      path,
			"namespace": destNS,
		})
	}

	return apps
}

func collectDeployments(ctx context.Context, client dynamic.Interface) []map[string]string {
	var deployments []map[string]string

	gvr := schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "deployments",
	}

	list, err := client.Resource(gvr).Namespace("").List(ctx, v1.ListOptions{})
	if err != nil {
		return deployments
	}

	for _, item := range list.Items {
		name := item.GetName()
		namespace := item.GetNamespace()

		// Get container image
		containers, _, _ := unstructured.NestedSlice(item.Object, "spec", "template", "spec", "containers")
		image := ""
		if len(containers) > 0 {
			if c, ok := containers[0].(map[string]interface{}); ok {
				image, _, _ = unstructured.NestedString(c, "image")
			}
		}

		deployments = append(deployments, map[string]string{
			"name":      name,
			"namespace": namespace,
			"image":     image,
		})
	}

	return deployments
}

func buildRepoPatterns(fluxRepos map[string]string, kustomizations []map[string]string, argoApps []map[string]string) []RepoPattern {
	repoMap := make(map[string]*RepoPattern)

	// Process Flux repos
	for name, url := range fluxRepos {
		owner, repoName := parseGitURL(url)
		pattern := &RepoPattern{
			URL:   url,
			Owner: owner,
			Name:  repoName,
			Tool:  "Flux",
		}
		repoMap[url] = pattern

		// Find kustomizations using this repo
		for _, k := range kustomizations {
			if k["source"] == name {
				pattern.Paths = appendUnique(pattern.Paths, k["path"])
				pattern.Apps = appendUnique(pattern.Apps, k["name"])
			}
		}
	}

	// Process ArgoCD apps
	for _, app := range argoApps {
		url := app["repoURL"]
		if url == "" {
			continue
		}

		pattern, exists := repoMap[url]
		if !exists {
			owner, repoName := parseGitURL(url)
			pattern = &RepoPattern{
				URL:   url,
				Owner: owner,
				Name:  repoName,
				Tool:  "ArgoCD",
			}
			repoMap[url] = pattern
		} else if pattern.Tool != "ArgoCD" {
			pattern.Tool = "Flux+ArgoCD"
		}

		pattern.Paths = appendUnique(pattern.Paths, app["path"])
		pattern.Apps = appendUnique(pattern.Apps, app["name"])
	}

	// Classify patterns and detect named architectures
	for _, p := range repoMap {
		p.PatternType = classifyRepoPattern(p)
		p.NamedPattern = detectNamedPattern(p.Paths)
	}

	// Convert to slice
	var result []RepoPattern
	for _, p := range repoMap {
		result = append(result, *p)
	}

	// Sort by owner, then name
	sort.Slice(result, func(i, j int) bool {
		if result[i].Owner != result[j].Owner {
			return result[i].Owner < result[j].Owner
		}
		return result[i].Name < result[j].Name
	})

	return result
}

func parseGitURL(url string) (owner, name string) {
	// Handle https://github.com/owner/repo or git@github.com:owner/repo
	url = strings.TrimSuffix(url, ".git")

	if strings.Contains(url, "github.com") {
		parts := strings.Split(url, "/")
		if len(parts) >= 2 {
			return parts[len(parts)-2], parts[len(parts)-1]
		}
	}

	return "external", url
}

func classifyRepoPattern(p *RepoPattern) string {
	if len(p.Apps) == 0 {
		return "unused"
	}

	name := strings.ToLower(p.Name)

	// Platform patterns
	if strings.Contains(name, "platform") || strings.Contains(name, "infrastructure") || strings.Contains(name, "infra") {
		return "platform"
	}

	// External/upstream
	if p.Owner != "" && !strings.Contains(strings.ToLower(p.Owner), "acme") &&
		!strings.Contains(strings.ToLower(p.Owner), "internal") {
		// Check if it looks like an external repo
		if strings.Contains(p.Owner, "stefanprodan") ||
			strings.Contains(p.Owner, "argoproj") ||
			strings.Contains(p.Owner, "fluxcd") {
			return "external"
		}
	}

	// Monorepo vs polyrepo
	if len(p.Apps) > 3 || len(p.Paths) > 3 {
		return "monorepo"
	}

	return "polyrepo"
}

// detectNamedPattern identifies well-known GitOps reference architectures
// Named patterns from docs/map/reference/gitops-repo-structures.md:
// - Arnie: Environment-per-Folder (Kustomize/Helm overlays)
// - Banko: Cluster-per-Directory (Flux multi-cluster)
// - Fluxy: Multi-Repo Fleet (OCI artifacts)
// - D2: Control Plane reference architecture (components/base|env)
func detectNamedPattern(paths []string) string {
	hasEnvs := false
	hasBase := false
	hasClusters := false
	hasComponents := false
	hasOverlays := false

	for _, path := range paths {
		pathLower := strings.ToLower(path)
		if strings.Contains(pathLower, "/envs/") || strings.Contains(pathLower, "/environments/") {
			hasEnvs = true
		}
		if strings.Contains(pathLower, "/base") {
			hasBase = true
		}
		if strings.Contains(pathLower, "/clusters/") {
			hasClusters = true
		}
		if strings.Contains(pathLower, "/components/") {
			hasComponents = true
		}
		if strings.Contains(pathLower, "/overlays/") {
			hasOverlays = true
		}
	}

	// D2 pattern: components/<name>/controllers/base|production
	if hasComponents && hasBase {
		return "D2 (Control Plane)"
	}

	// Banko pattern: clusters/<cluster>/<app>
	if hasClusters {
		return "Banko (Cluster-per-Dir)"
	}

	// Arnie pattern: base/ + overlays/ or envs/
	if hasBase && (hasOverlays || hasEnvs) {
		return "Arnie (Env-per-Folder)"
	}

	// Simple kustomize with env folders
	if hasEnvs {
		return "Arnie (Env-per-Folder)"
	}

	return ""
}

func buildEnvChains(deployments []map[string]string) []EnvChain {
	// Group by app name
	appEnvs := make(map[string]map[string]string) // app -> namespace -> image

	for _, d := range deployments {
		name := d["name"]
		ns := d["namespace"]
		image := d["image"]

		if appEnvs[name] == nil {
			appEnvs[name] = make(map[string]string)
		}
		appEnvs[name][ns] = image
	}

	// Filter to apps in multiple namespaces
	var chains []EnvChain
	for app, envs := range appEnvs {
		if len(envs) > 1 {
			chains = append(chains, EnvChain{
				AppName:      app,
				Environments: envs,
			})
		}
	}

	// Sort by app name
	sort.Slice(chains, func(i, j int) bool {
		return chains[i].AppName < chains[j].AppName
	})

	return chains
}

func detectEnvGroups(ctx context.Context, client dynamic.Interface) map[string][]string {
	groups := map[string][]string{
		"production": {},
		"staging":    {},
		"dev":        {},
		"team":       {},
		"system":     {},
	}

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "namespaces",
	}

	list, err := client.Resource(gvr).List(ctx, v1.ListOptions{})
	if err != nil {
		return groups
	}

	for _, item := range list.Items {
		ns := item.GetName()
		nsLower := strings.ToLower(ns)

		switch {
		case strings.Contains(nsLower, "prod") || nsLower == "production":
			groups["production"] = append(groups["production"], ns)
		case strings.Contains(nsLower, "staging") || strings.Contains(nsLower, "stage"):
			groups["staging"] = append(groups["staging"], ns)
		case strings.Contains(nsLower, "dev") || strings.Contains(nsLower, "qa") || strings.Contains(nsLower, "test"):
			groups["dev"] = append(groups["dev"], ns)
		case strings.HasPrefix(nsLower, "team-"):
			groups["team"] = append(groups["team"], ns)
		case strings.HasSuffix(nsLower, "-system"):
			groups["system"] = append(groups["system"], ns)
		}
	}

	return groups
}

func detectTeamPatterns(ctx context.Context, client dynamic.Interface, deployments []map[string]string) []TeamPattern {
	teamNS := make(map[string][]string)
	teamWorkloads := make(map[string]int)

	// Find team-* namespaces
	for _, d := range deployments {
		ns := d["namespace"]
		if strings.HasPrefix(strings.ToLower(ns), "team-") {
			team := strings.TrimPrefix(strings.ToLower(ns), "team-")
			if !contains(teamNS[team], ns) {
				teamNS[team] = append(teamNS[team], ns)
			}
			teamWorkloads[team]++
		}
	}

	var teams []TeamPattern
	for name, namespaces := range teamNS {
		teams = append(teams, TeamPattern{
			Name:       name,
			Namespaces: namespaces,
			Workloads:  teamWorkloads[name],
		})
	}

	sort.Slice(teams, func(i, j int) bool {
		return teams[i].Workloads > teams[j].Workloads
	})

	return teams
}

func suggestOrganization(result *PatternsResult) SuggestedOrg {
	org := SuggestedOrg{
		Organization: "your-org",
	}

	// Detect organization from repo owners
	ownerCounts := make(map[string]int)
	for _, r := range result.Repos {
		if r.Owner != "" && r.Owner != "external" {
			ownerCounts[r.Owner]++
		}
	}

	maxCount := 0
	for owner, count := range ownerCounts {
		if count > maxCount {
			org.Organization = owner
			maxCount = count
		}
	}

	// Build hubs from patterns
	hubMap := make(map[string]*SuggestedHub)

	// Platform hub from platform repos
	for _, r := range result.Repos {
		if r.PatternType == "platform" {
			if hubMap["platform"] == nil {
				hubMap["platform"] = &SuggestedHub{Name: "platform"}
			}
			for _, app := range r.Apps {
				hubMap["platform"].AppSpaces = append(hubMap["platform"].AppSpaces, SuggestedSpace{
					Name:      app,
					Workloads: []string{app},
					Env:       "shared",
				})
			}
		}
	}

	// Team hubs from team namespaces
	for _, team := range result.Teams {
		hubName := team.Name
		if hubMap[hubName] == nil {
			hubMap[hubName] = &SuggestedHub{Name: hubName}
		}
		for _, ns := range team.Namespaces {
			hubMap[hubName].AppSpaces = append(hubMap[hubName].AppSpaces, SuggestedSpace{
				Name:      ns,
				Workloads: []string{fmt.Sprintf("%d workloads", team.Workloads/len(team.Namespaces))},
				Env:       inferEnv(ns),
			})
		}
	}

	// App hubs from env chains
	for _, chain := range result.EnvChains {
		hubName := chain.AppName
		if hubMap[hubName] == nil && len(chain.Environments) > 2 {
			hubMap[hubName] = &SuggestedHub{Name: hubName}
			for ns := range chain.Environments {
				hubMap[hubName].AppSpaces = append(hubMap[hubName].AppSpaces, SuggestedSpace{
					Name:      fmt.Sprintf("%s-%s", chain.AppName, inferEnv(ns)),
					Workloads: []string{fmt.Sprintf("%s/%s", ns, chain.AppName)},
					Env:       inferEnv(ns),
				})
			}
		}
	}

	for _, hub := range hubMap {
		org.Hubs = append(org.Hubs, *hub)
	}

	sort.Slice(org.Hubs, func(i, j int) bool {
		return org.Hubs[i].Name < org.Hubs[j].Name
	})

	return org
}

func inferEnv(namespace string) string {
	ns := strings.ToLower(namespace)
	switch {
	case strings.Contains(ns, "prod"):
		return "prod"
	case strings.Contains(ns, "staging") || strings.Contains(ns, "stage"):
		return "staging"
	case strings.Contains(ns, "dev") || strings.Contains(ns, "qa"):
		return "dev"
	default:
		return "default"
	}
}

func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func printPatterns(r *PatternsResult) {
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                         GITOPS PATTERNS ANALYSIS")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println()

	// Quick summary
	totalRepos := len(r.Repos)
	totalApps := 0
	namedPatterns := make(map[string]int)
	for _, repo := range r.Repos {
		totalApps += len(repo.Apps)
		if repo.NamedPattern != "" {
			namedPatterns[repo.NamedPattern]++
		}
	}
	fmt.Printf("Summary: %d repos, %d apps, %d env chains, %d teams\n",
		totalRepos, totalApps, len(r.EnvChains), len(r.Teams))
	if len(namedPatterns) > 0 {
		fmt.Print("Detected patterns: ")
		first := true
		for pattern, count := range namedPatterns {
			if !first {
				fmt.Print(", ")
			}
			fmt.Printf("%s (%d)", pattern, count)
			first = false
		}
		fmt.Println()
	}
	fmt.Println()

	// Repo patterns
	fmt.Println("REPOSITORY PATTERNS")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	// Group by owner
	ownerRepos := make(map[string][]RepoPattern)
	for _, r := range r.Repos {
		ownerRepos[r.Owner] = append(ownerRepos[r.Owner], r)
	}

	for owner, repos := range ownerRepos {
		fmt.Printf("\n%s (%d repos)\n", owner, len(repos))
		for _, repo := range repos {
			typeIcon := "📦"
			switch repo.PatternType {
			case "platform":
				typeIcon = "🏗️"
			case "monorepo":
				typeIcon = "📚"
			case "external":
				typeIcon = "🌐"
			}
			namedStr := ""
			if repo.NamedPattern != "" {
				namedStr = fmt.Sprintf(" (%s)", repo.NamedPattern)
			}
			fmt.Printf("├── %s %s [%s]%s → %d apps\n", typeIcon, repo.Name, repo.Tool, namedStr, len(repo.Apps))
			if mapVerbose && len(repo.Paths) > 0 {
				for _, path := range repo.Paths {
					fmt.Printf("│   └── path: %s\n", path)
				}
			}
		}
	}
	fmt.Println()

	// Environment chains
	if len(r.EnvChains) > 0 {
		fmt.Println("ENVIRONMENT CHAINS (same app across namespaces)")
		fmt.Println("───────────────────────────────────────────────────────────────────────────────")

		for _, chain := range r.EnvChains {
			if len(chain.Environments) < 2 {
				continue
			}
			fmt.Printf("\n%s (%d environments)\n", chain.AppName, len(chain.Environments))

			// Sort by env type (dev -> staging -> prod)
			var envList []string
			for ns := range chain.Environments {
				envList = append(envList, ns)
			}
			sort.Slice(envList, func(i, j int) bool {
				return envPriority(envList[i]) < envPriority(envList[j])
			})

			for i, ns := range envList {
				prefix := "├──"
				if i == len(envList)-1 {
					prefix = "└──"
				}
				image := chain.Environments[ns]
				if len(image) > 40 {
					image = image[len(image)-40:]
				}
				fmt.Printf("%s %s: %s\n", prefix, ns, image)
			}
		}
		fmt.Println()
	}

	// Team patterns
	if len(r.Teams) > 0 {
		fmt.Println("TEAM PATTERNS")
		fmt.Println("───────────────────────────────────────────────────────────────────────────────")
		for _, team := range r.Teams {
			fmt.Printf("  • team-%s: %d workloads in %v\n", team.Name, team.Workloads, team.Namespaces)
		}
		fmt.Println()
	}

	// Environment groups
	fmt.Println("ENVIRONMENT GROUPS")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	for env, namespaces := range r.EnvGroups {
		if len(namespaces) > 0 {
			fmt.Printf("  %-12s %v\n", env+":", namespaces)
		}
	}
	fmt.Println()

	// Suggested organization
	fmt.Println("SUGGESTED CONFIGHUB ORGANIZATION")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Printf("\nOrganization: %s\n", r.Suggested.Organization)

	for _, hub := range r.Suggested.Hubs {
		fmt.Printf("\n├── Hub: %s\n", hub.Name)
		for _, space := range hub.AppSpaces {
			fmt.Printf("│   └── AppSpace: %s [%s]\n", space.Name, space.Env)
		}
	}
	fmt.Println()
}

func envPriority(namespace string) int {
	ns := strings.ToLower(namespace)
	switch {
	case strings.Contains(ns, "dev") || strings.Contains(ns, "qa"):
		return 1
	case strings.Contains(ns, "staging") || strings.Contains(ns, "stage"):
		return 2
	case strings.Contains(ns, "prod"):
		return 3
	default:
		return 4
	}
}
