// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/confighub/cub-scout/pkg/gitops"
)

// ImportSuggestion represents a suggested import structure (org-space model)
type ImportSuggestion struct {
	Space string
	Apps  []AppSuggestion
}

// HubAppSpaceSuggestion represents a suggested import structure (hub-appspace model)
// In this model, one Space acts as the "App Space" containing all variants via labels
type HubAppSpaceSuggestion struct {
	AppSpace string            // The team's App Space (one per team, not per env)
	Units    []HubAppSpaceUnit // Units with app/variant labels
}

// HubAppSpaceUnit represents a unit in the Hub/App Space model
type HubAppSpaceUnit struct {
	Slug      string         // e.g., "payment-api-prod"
	App       string         // app label value
	Variant   string         // variant label value
	Workloads []WorkloadInfo // workloads that map to this unit
}

// AppSuggestion represents a suggested app grouping
type AppSuggestion struct {
	Name     string
	Variants []VariantSuggestion
}

// VariantSuggestion represents a suggested variant within an app
type VariantSuggestion struct {
	Name      string
	Workloads []WorkloadInfo
	UnitSlug  string // Generated unit slug: app-variant or just app if default
}

// Common environment/variant suffixes and prefixes
var variantPatterns = []string{
	"prod", "production",
	"staging", "stage", "stg",
	"dev", "development",
	"test", "testing", "qa",
	"uat", "sit",
	"demo", "sandbox",
	"preview", "canary",
}

// SuggestStructure analyzes workloads and suggests an import structure
func SuggestStructure(workloads []WorkloadInfo, defaultSpace string) ImportSuggestion {
	// Group workloads by inferred app name
	appGroups := make(map[string]map[string][]WorkloadInfo) // app -> variant -> workloads

	for _, w := range workloads {
		app, variant := inferAppAndVariant(w)
		if app == "" {
			app = w.Name // fallback to workload name
		}
		if variant == "" {
			variant = "default"
		}

		if appGroups[app] == nil {
			appGroups[app] = make(map[string][]WorkloadInfo)
		}
		appGroups[app][variant] = append(appGroups[app][variant], w)
	}

	// Build suggestion structure
	suggestion := ImportSuggestion{
		Space: inferSpace(workloads, defaultSpace),
	}

	// Sort apps for consistent output
	appNames := make([]string, 0, len(appGroups))
	for app := range appGroups {
		appNames = append(appNames, app)
	}
	sort.Strings(appNames)

	for _, appName := range appNames {
		variants := appGroups[appName]

		app := AppSuggestion{Name: appName}

		// Sort variants for consistent output
		variantNames := make([]string, 0, len(variants))
		for v := range variants {
			variantNames = append(variantNames, v)
		}
		sort.Strings(variantNames)

		for _, variantName := range variantNames {
			wls := variants[variantName]

			// Generate unit slug
			unitSlug := appName
			if variantName != "default" && len(variants) > 1 {
				unitSlug = fmt.Sprintf("%s-%s", appName, variantName)
			}

			app.Variants = append(app.Variants, VariantSuggestion{
				Name:      variantName,
				Workloads: wls,
				UnitSlug:  sanitizeSlug(unitSlug),
			})
		}

		suggestion.Apps = append(suggestion.Apps, app)
	}

	return suggestion
}

// inferAppAndVariant extracts app name and variant from workload metadata
func inferAppAndVariant(w WorkloadInfo) (app, variant string) {
	// Priority 0: GitOps deployer path (most reliable signal)
	// Flux Kustomization: spec.path: "./staging" -> variant=staging
	// Argo Application: spec.source.path: "apps/prod" -> variant=prod
	if w.KustomizationPath != "" {
		variant = extractVariantFromPath(w.KustomizationPath)
	} else if w.ApplicationPath != "" {
		variant = extractVariantFromPath(w.ApplicationPath)
	}

	// Priority 1: Kubernetes recommended labels
	if name, ok := w.Labels["app.kubernetes.io/name"]; ok {
		app = name
	}
	if variant == "" {
		if instance, ok := w.Labels["app.kubernetes.io/instance"]; ok && instance != app {
			// instance is often variant (e.g., "myapp-prod")
			variant = extractVariantFromInstance(instance, app)
		}
	}

	// Priority 2: Common labels
	if app == "" {
		if name, ok := w.Labels["app"]; ok {
			app = name
		}
	}
	if variant == "" {
		if env, ok := w.Labels["environment"]; ok {
			variant = env
		} else if env, ok := w.Labels["env"]; ok {
			variant = env
		}
	}

	// Priority 3: Namespace pattern detection
	if app == "" || variant == "" {
		nsApp, nsVariant := parseNamespacePattern(w.Namespace)
		if app == "" {
			app = nsApp
		}
		if variant == "" {
			variant = nsVariant
		}
	}

	// Priority 4: Workload name parsing
	if app == "" {
		app = w.Name
	}

	return app, variant
}

// parseNamespacePattern extracts app and variant from namespace naming conventions
func parseNamespacePattern(namespace string) (app, variant string) {
	// Skip system namespaces
	if isSystemNamespaceForSuggest(namespace) {
		return namespace, ""
	}

	// Try suffix patterns: my-app-prod, my-app-staging
	for _, v := range variantPatterns {
		suffix := "-" + v
		if strings.HasSuffix(namespace, suffix) {
			app = strings.TrimSuffix(namespace, suffix)
			variant = v
			return normalizeVariant(app), normalizeVariant(variant)
		}
	}

	// Try prefix patterns: prod-my-app, staging-my-app
	for _, v := range variantPatterns {
		prefix := v + "-"
		if strings.HasPrefix(namespace, prefix) {
			app = strings.TrimPrefix(namespace, prefix)
			variant = v
			return normalizeVariant(app), normalizeVariant(variant)
		}
	}

	// No pattern found - namespace is the app, no variant
	return namespace, ""
}

// extractVariantFromInstance extracts variant from instance label
// e.g., "myapp-prod" with app="myapp" -> variant="prod"
func extractVariantFromInstance(instance, app string) string {
	if app != "" && strings.HasPrefix(instance, app+"-") {
		return strings.TrimPrefix(instance, app+"-")
	}
	// Check if instance ends with a known variant
	for _, v := range variantPatterns {
		if strings.HasSuffix(instance, "-"+v) {
			return v
		}
	}
	return ""
}

// extractVariantFromPath extracts variant from Flux Kustomization spec.path
// Examples:
//
//	"./staging" -> "staging"
//	"./production" -> "prod"
//	"./clusters/prod/apps" -> "prod"
//	"./apps/staging/podinfo" -> "staging"
func extractVariantFromPath(path string) string {
	if path == "" {
		return ""
	}

	// Clean up the path - remove leading "./" and split
	path = strings.TrimPrefix(path, "./")
	parts := strings.Split(path, "/")

	// Look for variant patterns in any path segment
	for _, part := range parts {
		part = strings.ToLower(part)
		for _, v := range variantPatterns {
			if part == v {
				return normalizeVariant(v)
			}
		}
	}

	// If path is a single segment and matches a variant, use it
	if len(parts) == 1 {
		normalized := normalizeVariant(parts[0])
		for _, v := range variantPatterns {
			if normalizeVariant(v) == normalized {
				return normalized
			}
		}
	}

	return ""
}

// inferSpace suggests a space name from workloads
func inferSpace(workloads []WorkloadInfo, defaultSpace string) string {
	if defaultSpace != "" {
		return defaultSpace
	}

	// Try to find a common namespace prefix
	namespaces := make(map[string]int)
	for _, w := range workloads {
		namespaces[w.Namespace]++
	}

	// If all workloads are in one namespace, suggest that
	if len(namespaces) == 1 {
		for ns := range namespaces {
			if !isSystemNamespaceForSuggest(ns) {
				// Strip variant suffix if present
				app, _ := parseNamespacePattern(ns)
				return app
			}
		}
	}

	// Try to find common prefix
	var nsList []string
	for ns := range namespaces {
		if !isSystemNamespaceForSuggest(ns) {
			nsList = append(nsList, ns)
		}
	}
	if prefix := longestCommonPrefix(nsList); prefix != "" && len(prefix) > 2 {
		return strings.TrimSuffix(prefix, "-")
	}

	return "imported"
}

// isSystemNamespaceForSuggest returns true for Kubernetes system namespaces (used in suggestions)
// This is more conservative than the import version, including gitops namespaces
func isSystemNamespaceForSuggest(ns string) bool {
	systemNS := []string{
		"default", "kube-system", "kube-public", "kube-node-lease",
		"flux-system", "argocd", "cert-manager", "ingress-nginx",
	}
	for _, s := range systemNS {
		if ns == s {
			return true
		}
	}
	// Prefix matches for dynamic system namespaces (e.g., local-path-storage)
	systemPrefixes := []string{"local-path-"}
	for _, prefix := range systemPrefixes {
		if strings.HasPrefix(ns, prefix) {
			return true
		}
	}
	return false
}

// normalizeVariant standardizes variant names
func normalizeVariant(v string) string {
	v = strings.ToLower(v)
	switch v {
	case "production":
		return "prod"
	case "development":
		return "dev"
	case "staging", "stage":
		return "staging"
	case "testing":
		return "test"
	}
	return v
}

// sanitizeSlug creates a valid ConfigHub slug from a string
func sanitizeSlug(s string) string {
	// Convert to lowercase
	s = strings.ToLower(s)
	// Replace invalid characters with hyphens
	reg := regexp.MustCompile(`[^a-z0-9-]`)
	s = reg.ReplaceAllString(s, "-")
	// Remove consecutive hyphens
	reg = regexp.MustCompile(`-+`)
	s = reg.ReplaceAllString(s, "-")
	// Trim hyphens from ends
	s = strings.Trim(s, "-")
	return s
}

// longestCommonPrefix finds the longest common prefix of strings
func longestCommonPrefix(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	if len(strs) == 1 {
		return strs[0]
	}

	prefix := strs[0]
	for _, s := range strs[1:] {
		for !strings.HasPrefix(s, prefix) {
			prefix = prefix[:len(prefix)-1]
			if prefix == "" {
				return ""
			}
		}
	}
	return prefix
}

// PrintSuggestion displays the suggestion in a human-readable format
func (s *ImportSuggestion) Print() {
	fmt.Printf("Suggested structure:\n")
	fmt.Printf("  Space: %s\n", s.Space)

	for _, app := range s.Apps {
		if len(app.Variants) == 1 && app.Variants[0].Name == "default" {
			// Single variant, show simpler output
			v := app.Variants[0]
			fmt.Printf("    └── Unit: %s (%d workload(s))\n", v.UnitSlug, len(v.Workloads))
		} else {
			fmt.Printf("    ├── App: %s\n", app.Name)
			for i, v := range app.Variants {
				prefix := "│   ├──"
				if i == len(app.Variants)-1 {
					prefix = "│   └──"
				}
				fmt.Printf("    %s Unit: %s (variant=%s, %d workload(s))\n",
					prefix, v.UnitSlug, v.Name, len(v.Workloads))
			}
		}
	}
}

// TotalUnits returns the total number of units that would be created
func (s *ImportSuggestion) TotalUnits() int {
	count := 0
	for _, app := range s.Apps {
		count += len(app.Variants)
	}
	return count
}

// TotalWorkloads returns the total number of workloads
func (s *ImportSuggestion) TotalWorkloads() int {
	count := 0
	for _, app := range s.Apps {
		for _, v := range app.Variants {
			count += len(v.Workloads)
		}
	}
	return count
}

// SuggestHubAppSpaceStructure analyzes workloads and suggests a Hub/App Space structure
// Key difference: One App Space contains ALL variants via labels (not one space per env)
func SuggestHubAppSpaceStructure(workloads []WorkloadInfo, defaultSpace string) HubAppSpaceSuggestion {
	// Group workloads by inferred app and variant
	type unitKey struct {
		app, variant string
	}
	unitGroups := make(map[unitKey][]WorkloadInfo)

	for _, w := range workloads {
		app, variant := inferAppAndVariant(w)
		if app == "" {
			app = w.Name // fallback to workload name
		}
		if variant == "" {
			variant = "default"
		}

		key := unitKey{app: app, variant: variant}
		unitGroups[key] = append(unitGroups[key], w)
	}

	// Build suggestion
	suggestion := HubAppSpaceSuggestion{
		AppSpace: inferAppSpace(workloads, defaultSpace),
	}

	// Sort keys for consistent output
	var keys []unitKey
	for k := range unitGroups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].app != keys[j].app {
			return keys[i].app < keys[j].app
		}
		return keys[i].variant < keys[j].variant
	})

	for _, key := range keys {
		wls := unitGroups[key]

		// Generate unit slug: app-variant (or just app if default)
		slug := key.app
		if key.variant != "default" {
			slug = fmt.Sprintf("%s-%s", key.app, key.variant)
		}

		suggestion.Units = append(suggestion.Units, HubAppSpaceUnit{
			Slug:      sanitizeSlug(slug),
			App:       key.app,
			Variant:   key.variant,
			Workloads: wls,
		})
	}

	return suggestion
}

// inferAppSpace suggests an App Space name (team workspace)
// In Hub/App Space model, this is the team's workspace containing all their variants
func inferAppSpace(workloads []WorkloadInfo, defaultSpace string) string {
	if defaultSpace != "" {
		return defaultSpace
	}

	// Try to find a common app name across workloads
	apps := make(map[string]int)
	for _, w := range workloads {
		app, _ := inferAppAndVariant(w)
		if app != "" && app != w.Name { // Only count if actually inferred
			apps[app]++
		}
	}

	// If there's a dominant app, use that
	var maxApp string
	var maxCount int
	for app, count := range apps {
		if count > maxCount {
			maxApp = app
			maxCount = count
		}
	}
	if maxApp != "" {
		return maxApp + "-team" // e.g., "payment-api-team"
	}

	// Fall back to namespace-based inference (strip variant suffix)
	if len(workloads) > 0 {
		app, _ := parseNamespacePattern(workloads[0].Namespace)
		if app != "" && !isSystemNamespaceForSuggest(app) {
			return app + "-team"
		}
	}

	return "imported-team"
}

// Print displays the Hub/App Space suggestion
func (s *HubAppSpaceSuggestion) Print() {
	fmt.Printf("Suggested structure (Hub/App Space model):\n")
	fmt.Printf("  App Space: %s\n", s.AppSpace)
	fmt.Println()

	// Group by app for display
	appUnits := make(map[string][]HubAppSpaceUnit)
	for _, u := range s.Units {
		appUnits[u.App] = append(appUnits[u.App], u)
	}

	// Sort apps
	var apps []string
	for app := range appUnits {
		apps = append(apps, app)
	}
	sort.Strings(apps)

	for _, app := range apps {
		units := appUnits[app]
		if len(units) == 1 && units[0].Variant == "default" {
			// Single variant
			u := units[0]
			fmt.Printf("    └── Unit: %s (app=%s, %d workload(s))\n",
				u.Slug, u.App, len(u.Workloads))
		} else {
			// Multiple variants
			fmt.Printf("    ├── app=%s\n", app)
			for i, u := range units {
				prefix := "│   ├──"
				if i == len(units)-1 {
					prefix = "│   └──"
				}
				fmt.Printf("    %s Unit: %s (variant=%s, %d workload(s))\n",
					prefix, u.Slug, u.Variant, len(u.Workloads))
			}
		}
	}
}

// FullProposal represents the complete Hub/App Space mapping proposal
// combining Git repo structure with cluster workloads
type FullProposal struct {
	// Hub-level items
	HubBases []HubBaseProposal `json:"hubBases,omitempty"` // Templates from Git base/

	// App Space level
	AppSpace       string               `json:"appSpace"`
	Deployer       string               `json:"deployer,omitempty"`       // Detected deployer (Flux, Argo, etc.)
	Reconciliation []ReconciliationRule `json:"reconciliation,omitempty"` // Suggested rules by variant
	Units          []UnitProposal       `json:"units"`

	// Alignment issues
	GitOnly     []GitOnlyApp     `json:"gitOnly,omitempty"`     // In Git, not deployed
	ClusterOnly []ClusterOnlyApp `json:"clusterOnly,omitempty"` // In cluster, not in Git
}

// ReconciliationRule defines behavior by label match
type ReconciliationRule struct {
	Match    map[string]string `json:"match"`    // e.g., {"variant": "prod"}
	Drift    string            `json:"drift"`    // "revert" or "accept"
	Approval string            `json:"approval"` // "required" or "none"
}

// HubBaseProposal represents a template that should go in Hub catalog
type HubBaseProposal struct {
	Name    string `json:"name"`
	GitPath string `json:"gitPath"` // e.g., "apps/base/payment-api"
	Source  string `json:"source"`  // Where it came from
}

// UnitProposal represents a unit that should be created
type UnitProposal struct {
	Slug      string            `json:"slug"`
	App       string            `json:"app"`
	Variant   string            `json:"variant"`
	Region    string            `json:"region,omitempty"`    // From cluster path or labels
	Tier      string            `json:"tier,omitempty"`      // frontend, backend, database
	Upstream  string            `json:"upstream,omitempty"`  // Hub Base this clones from
	GitPath   string            `json:"gitPath,omitempty"`   // Path in Git repo
	Workloads []string          `json:"workloads,omitempty"` // Connected cluster workloads
	Status    string            `json:"status"`              // "aligned", "git-only", "cluster-only"
	Labels    map[string]string `json:"labels,omitempty"`    // All proposed labels
}

// GitOnlyApp represents an app defined in Git but not deployed
type GitOnlyApp struct {
	App      string   `json:"app"`
	Variants []string `json:"variants"`
	GitPaths []string `json:"gitPaths"`
}

// ClusterOnlyApp represents workloads not tracked in Git
type ClusterOnlyApp struct {
	App       string   `json:"app"`
	Workloads []string `json:"workloads"`
	Owner     string   `json:"owner"` // Native = orphan
}

// SuggestFullProposal combines Git facts + Cluster facts into a full Hub/App Space proposal
func SuggestFullProposal(gitApps []gitops.AppDefinition, workloads []WorkloadInfo, defaultSpace string) *FullProposal {
	proposal := &FullProposal{
		AppSpace: inferAppSpace(workloads, defaultSpace),
	}

	// Detect dominant deployer from workloads
	deployers := make(map[string]int)
	for _, w := range workloads {
		if w.Owner != "Native" && w.Owner != "" {
			deployers[w.Owner]++
		}
	}
	maxCount := 0
	for d, count := range deployers {
		if count > maxCount {
			proposal.Deployer = d
			maxCount = count
		}
	}

	// Index cluster workloads by app name
	clusterApps := make(map[string][]WorkloadInfo)
	for _, w := range workloads {
		app, _ := inferAppAndVariant(w)
		if app == "" {
			app = w.Name
		}
		clusterApps[app] = append(clusterApps[app], w)
	}

	// Track which Git apps are matched and which Hub bases exist
	matchedGitApps := make(map[string]bool)
	hubBaseNames := make(map[string]bool)

	// Process Git apps - first pass to identify Hub bases
	for _, gitApp := range gitApps {
		if gitApp.BasePath != "" && strings.Contains(gitApp.BasePath, "base") {
			proposal.HubBases = append(proposal.HubBases, HubBaseProposal{
				Name:    gitApp.Name,
				GitPath: gitApp.BasePath,
				Source:  "git",
			})
			hubBaseNames[gitApp.Name] = true
		}
	}

	// Track variants found for reconciliation rules
	variantsFound := make(map[string]bool)

	// Process Git apps - second pass to create units
	for _, gitApp := range gitApps {
		// Process variants -> Units
		for _, variant := range gitApp.Variants {
			slug := gitApp.Name
			variantName := normalizeVariant(variant.Name)
			if variantName != "default" && variantName != "" {
				slug = fmt.Sprintf("%s-%s", gitApp.Name, variantName)
			}
			variantsFound[variantName] = true

			unit := UnitProposal{
				Slug:    sanitizeSlug(slug),
				App:     gitApp.Name,
				Variant: variantName,
				GitPath: variant.Path,
				Labels:  make(map[string]string),
			}

			// Set upstream if Hub base exists
			if hubBaseNames[gitApp.Name] {
				unit.Upstream = gitApp.Name
			}

			// Build labels map
			unit.Labels["app"] = gitApp.Name
			unit.Labels["variant"] = variantName

			// Check if deployed in cluster and extract additional labels
			if wls, ok := clusterApps[gitApp.Name]; ok {
				matchedGitApps[gitApp.Name] = true
				unit.Status = "aligned"
				for _, w := range wls {
					unit.Workloads = append(unit.Workloads, fmt.Sprintf("%s/%s", w.Namespace, w.Name))

					// Extract region from labels or path
					if region := extractRegion(w, variant.Path); region != "" {
						unit.Region = region
						unit.Labels["region"] = region
					}

					// Extract tier from labels
					if tier := extractTier(w); tier != "" {
						unit.Tier = tier
						unit.Labels["tier"] = tier
					}

					// Extract team from labels or app space
					if team := extractTeam(w, proposal.AppSpace); team != "" {
						unit.Labels["team"] = team
					}
				}
			} else {
				unit.Status = "git-only"
			}

			proposal.Units = append(proposal.Units, unit)
		}

		// If no variants, create default unit
		if len(gitApp.Variants) == 0 {
			unit := UnitProposal{
				Slug:    sanitizeSlug(gitApp.Name),
				App:     gitApp.Name,
				Variant: "default",
				GitPath: gitApp.BasePath,
				Labels:  map[string]string{"app": gitApp.Name, "variant": "default"},
			}
			if hubBaseNames[gitApp.Name] {
				unit.Upstream = gitApp.Name
			}
			if wls, ok := clusterApps[gitApp.Name]; ok {
				matchedGitApps[gitApp.Name] = true
				unit.Status = "aligned"
				for _, w := range wls {
					unit.Workloads = append(unit.Workloads, fmt.Sprintf("%s/%s", w.Namespace, w.Name))
				}
			} else {
				unit.Status = "git-only"
			}
			proposal.Units = append(proposal.Units, unit)
		}
	}

	// Find cluster-only apps (orphans)
	for appName, wls := range clusterApps {
		if !matchedGitApps[appName] {
			orphan := ClusterOnlyApp{
				App:   appName,
				Owner: wls[0].Owner,
			}
			for _, w := range wls {
				orphan.Workloads = append(orphan.Workloads, fmt.Sprintf("%s/%s", w.Namespace, w.Name))
			}
			proposal.ClusterOnly = append(proposal.ClusterOnly, orphan)

			// Also add as unit with cluster-only status
			slug := sanitizeSlug(appName)
			unit := UnitProposal{
				Slug:      slug,
				App:       appName,
				Variant:   "default",
				Workloads: orphan.Workloads,
				Status:    "cluster-only",
				Labels:    map[string]string{"app": appName, "variant": "default"},
			}

			// Extract labels from cluster workloads
			for _, w := range wls {
				if region := extractRegion(w, ""); region != "" && unit.Region == "" {
					unit.Region = region
					unit.Labels["region"] = region
				}
				if tier := extractTier(w); tier != "" && unit.Tier == "" {
					unit.Tier = tier
					unit.Labels["tier"] = tier
				}
				if team := extractTeam(w, proposal.AppSpace); team != "" {
					if _, exists := unit.Labels["team"]; !exists {
						unit.Labels["team"] = team
					}
				}
			}

			proposal.Units = append(proposal.Units, unit)
		}
	}

	// Generate reconciliation rules based on variants found
	proposal.Reconciliation = suggestReconciliationRules(variantsFound)

	// Sort units
	sort.Slice(proposal.Units, func(i, j int) bool {
		if proposal.Units[i].App != proposal.Units[j].App {
			return proposal.Units[i].App < proposal.Units[j].App
		}
		return proposal.Units[i].Variant < proposal.Units[j].Variant
	})

	return proposal
}

// extractRegion tries to find region from workload labels or path
func extractRegion(w WorkloadInfo, gitPath string) string {
	// Check labels
	if region, ok := w.Labels["topology.kubernetes.io/region"]; ok {
		return region
	}
	if region, ok := w.Labels["region"]; ok {
		return region
	}

	// Check Git path for region patterns
	regionPatterns := []string{
		"us-east", "us-west", "eu-west", "eu-central",
		"asia-east", "asia-south", "ap-southeast",
	}
	pathLower := strings.ToLower(gitPath)
	for _, r := range regionPatterns {
		if strings.Contains(pathLower, r) {
			return r
		}
	}
	return ""
}

// extractTier gets tier from workload labels
func extractTier(w WorkloadInfo) string {
	if tier, ok := w.Labels["app.kubernetes.io/component"]; ok {
		return tier
	}
	if tier, ok := w.Labels["tier"]; ok {
		return tier
	}
	return ""
}

// extractTeam infers team from labels or namespace
func extractTeam(w WorkloadInfo, appSpace string) string {
	// Check labels
	if team, ok := w.Labels["team"]; ok {
		return team
	}
	if team, ok := w.Labels["app.kubernetes.io/part-of"]; ok {
		return team
	}
	// Derive from App Space name (strip -team suffix)
	if appSpace != "" {
		return strings.TrimSuffix(appSpace, "-team")
	}
	return ""
}

// suggestReconciliationRules generates rules based on variants
func suggestReconciliationRules(variants map[string]bool) []ReconciliationRule {
	rules := []ReconciliationRule{}

	if variants["prod"] {
		rules = append(rules, ReconciliationRule{
			Match:    map[string]string{"variant": "prod"},
			Drift:    "revert",
			Approval: "required",
		})
	}

	if variants["staging"] {
		rules = append(rules, ReconciliationRule{
			Match:    map[string]string{"variant": "staging"},
			Drift:    "revert",
			Approval: "none",
		})
	}

	if variants["dev"] {
		rules = append(rules, ReconciliationRule{
			Match:    map[string]string{"variant": "dev"},
			Drift:    "accept",
			Approval: "none",
		})
	}

	return rules
}

// Print displays the full proposal
func (p *FullProposal) Print() {
	fmt.Println("┌─────────────────────────────────────────────────────────────┐")
	fmt.Println("│ PROPOSED HUB/APP SPACE MODEL                                │")
	fmt.Println("└─────────────────────────────────────────────────────────────┘")

	// Hub Bases
	if len(p.HubBases) > 0 {
		fmt.Println("\n  HUB (Platform Catalog)")
		for _, base := range p.HubBases {
			fmt.Printf("    └── Base: %s (%s)\n", base.Name, base.GitPath)
		}
	}

	// App Space
	fmt.Printf("\n  APP SPACE: %s\n", p.AppSpace)

	// Deployer
	if p.Deployer != "" {
		fmt.Printf("    Deployer: %s\n", p.Deployer)
	}

	// Reconciliation Rules
	if len(p.Reconciliation) > 0 {
		fmt.Println("    Reconciliation Rules:")
		for _, r := range p.Reconciliation {
			matchParts := []string{}
			for k, v := range r.Match {
				matchParts = append(matchParts, fmt.Sprintf("%s=%s", k, v))
			}
			fmt.Printf("      • %s → drift:%s, approval:%s\n",
				strings.Join(matchParts, ","), r.Drift, r.Approval)
		}
	}

	fmt.Println()

	// Group units by app
	appUnits := make(map[string][]UnitProposal)
	for _, u := range p.Units {
		appUnits[u.App] = append(appUnits[u.App], u)
	}

	var apps []string
	for app := range appUnits {
		apps = append(apps, app)
	}
	sort.Strings(apps)

	for _, app := range apps {
		units := appUnits[app]
		fmt.Printf("    ├── app=%s\n", app)
		for i, u := range units {
			prefix := "│   ├──"
			if i == len(units)-1 {
				prefix = "│   └──"
			}
			status := ""
			switch u.Status {
			case "git-only":
				status = " ⚠ NOT DEPLOYED"
			case "cluster-only":
				status = " ⚠ ORPHAN (not in Git)"
			case "aligned":
				status = " ✓"
			}
			fmt.Printf("    %s Unit: %s (variant=%s)%s\n", prefix, u.Slug, u.Variant, status)
			if u.Upstream != "" {
				fmt.Printf("    │       ↑ clones: %s\n", u.Upstream)
			}
			if u.GitPath != "" {
				fmt.Printf("    │       Git: %s\n", u.GitPath)
			}
			if len(u.Workloads) > 0 {
				fmt.Printf("    │       Cluster: %v\n", u.Workloads)
			}
			// Show additional labels (region, tier, team)
			extraLabels := []string{}
			if u.Region != "" {
				extraLabels = append(extraLabels, fmt.Sprintf("region=%s", u.Region))
			}
			if u.Tier != "" {
				extraLabels = append(extraLabels, fmt.Sprintf("tier=%s", u.Tier))
			}
			if team, ok := u.Labels["team"]; ok && team != "" {
				extraLabels = append(extraLabels, fmt.Sprintf("team=%s", team))
			}
			if len(extraLabels) > 0 {
				fmt.Printf("    │       Labels: %s\n", strings.Join(extraLabels, ", "))
			}
		}
	}

	// Summary
	fmt.Println()
	aligned := 0
	gitOnly := 0
	clusterOnly := 0
	for _, u := range p.Units {
		switch u.Status {
		case "aligned":
			aligned++
		case "git-only":
			gitOnly++
		case "cluster-only":
			clusterOnly++
		}
	}
	fmt.Printf("  Summary: %d aligned, %d git-only, %d cluster-only\n", aligned, gitOnly, clusterOnly)
}
