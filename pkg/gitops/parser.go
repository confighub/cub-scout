// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package gitops provides parsers for GitOps repository structures.
//
// Supports three architecture patterns:
//
//  1. Single-repo (flux2-kustomize-helm-example)
//     apps/, infrastructure/, clusters/ in one repo
//
//  2. D2 Split-repo (controlplaneio-fluxcd)
//     d2-fleet: clusters/, tenants/
//     d2-infra: components/ (controllers, configs)
//     d2-apps:  components/ (namespace-scoped)
//
//  3. Monorepo variants
//     Any combination of the above patterns
package gitops

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// RepoType identifies the repository architecture pattern
type RepoType string

const (
	RepoTypeSingleRepo     RepoType = "single-repo"      // Traditional apps/infra/clusters
	RepoTypeD2Fleet        RepoType = "d2-fleet"          // Fleet management (clusters, tenants)
	RepoTypeD2Infra        RepoType = "d2-infra"          // Infrastructure components
	RepoTypeD2Apps         RepoType = "d2-apps"           // Application components
	RepoTypeAppOfApps      RepoType = "argo-aoa-mono"     // Argo app-of-apps pattern (canonical skeleton ID)
	RepoTypeApplicationSet RepoType = "argo-appset-mono"  // Argo ApplicationSet generators (canonical skeleton ID)
	RepoTypeHelmUmbrella   RepoType = "helm-umbrella"     // Helm umbrella chart with dependencies
	RepoTypeFluxTenantMono RepoType = "flux-tenant-mono"  // Flux multi-tenancy pattern (canonical skeleton ID)
	RepoTypeFluxHelmMono   RepoType = "flux-helm-mono"    // Flux with HelmRelease pattern (canonical skeleton ID)
	RepoTypeUnknown        RepoType = "unknown"
)

// RepoStructure represents a parsed GitOps repository
type RepoStructure struct {
	Type            RepoType              `json:"type"`
	Apps            []AppDefinition       `json:"apps,omitempty"`
	Infrastructure  []InfraDefinition     `json:"infrastructure,omitempty"`
	Clusters        []ClusterDefinition   `json:"clusters,omitempty"`
	Tenants         []TenantDefinition    `json:"tenants,omitempty"`
	Components      []ComponentDefinition `json:"components,omitempty"`
	RootApp         *ArgoAppDefinition    `json:"rootApp,omitempty"`         // App-of-apps root
	ChildApps       []ArgoAppDefinition   `json:"childApps,omitempty"`       // Apps managed by root
	ApplicationSets []ApplicationSetDef   `json:"applicationSets,omitempty"` // ApplicationSet definitions
	HelmChart       *HelmChartDefinition  `json:"helmChart,omitempty"`       // Umbrella chart info
}

// ArgoAppDefinition represents an Argo CD Application
type ArgoAppDefinition struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Destination string `json:"destination,omitempty"` // Target cluster/namespace
	Source      string `json:"source,omitempty"`      // Git repo or Helm chart
}

// ApplicationSetDef represents an ApplicationSet generator
type ApplicationSetDef struct {
	Name           string            `json:"name"`
	Path           string            `json:"path"`
	Generator      string            `json:"generator"`                  // list, cluster, git, matrix, etc.
	TargetApps     []string          `json:"targetApps,omitempty"`       // App paths (unique identifiers)
	TargetAppNames map[string]string `json:"targetAppNames,omitempty"`   // Map of path -> app name (for display)
}

// HelmChartDefinition represents a Helm chart
type HelmChartDefinition struct {
	Name         string           `json:"name"`
	Version      string           `json:"version,omitempty"`
	Dependencies []HelmDependency `json:"dependencies,omitempty"`
}

// HelmDependency represents a Helm chart dependency
type HelmDependency struct {
	Name       string `json:"name"`
	Version    string `json:"version,omitempty"`
	Repository string `json:"repository,omitempty"`
}

// TenantDefinition represents a tenant in d2-fleet
type TenantDefinition struct {
	Name       string   `json:"name"`
	Path       string   `json:"path"`
	Namespaces []string `json:"namespaces,omitempty"`
}

// ComponentDefinition represents a component in d2-infra or d2-apps
type ComponentDefinition struct {
	Name     string   `json:"name"`
	Path     string   `json:"path"`
	Type     string   `json:"type"`               // "controller", "config", "app"
	Variants []string `json:"variants,omitempty"` // staging, production
}

// AppDefinition represents an application found in the repo
type AppDefinition struct {
	Name     string              `json:"name"`
	BasePath string              `json:"basePath,omitempty"` // e.g., "apps/base/podinfo"
	Variants []VariantDefinition `json:"variants"`
}

// VariantDefinition represents an environment variant (staging, prod, etc.)
type VariantDefinition struct {
	Name       string   `json:"name"`       // e.g., "staging", "production"
	Path       string   `json:"path"`       // e.g., "apps/staging"
	Apps       []string `json:"apps"`       // Apps included in this variant
	References string   `json:"references"` // What base it references
}

// InfraDefinition represents infrastructure components
type InfraDefinition struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// ClusterDefinition represents a cluster entry point
type ClusterDefinition struct {
	Name string   `json:"name"` // e.g., "staging", "production"
	Path string   `json:"path"` // e.g., "clusters/staging"
	Apps []string `json:"apps"` // References to app kustomizations
}

// Kustomization represents a kustomization.yaml file
type Kustomization struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Resources  []string `yaml:"resources"`
	Patches    []Patch  `yaml:"patches,omitempty"`
}

// Patch represents a kustomize patch
type Patch struct {
	Path   string `yaml:"path"`
	Target Target `yaml:"target,omitempty"`
}

// Target represents a patch target
type Target struct {
	Kind string `yaml:"kind"`
	Name string `yaml:"name"`
}

// ParseRepo parses a GitOps repository and returns its structure
func ParseRepo(repoPath string) (*RepoStructure, error) {
	result := &RepoStructure{}

	// Detect repo type
	result.Type = detectRepoType(repoPath)

	switch result.Type {
	case RepoTypeD2Fleet:
		return parseD2Fleet(repoPath)
	case RepoTypeD2Infra, RepoTypeD2Apps:
		return parseD2Components(repoPath, result.Type)
	case RepoTypeAppOfApps:
		return parseAppOfApps(repoPath)
	case RepoTypeApplicationSet:
		return parseApplicationSets(repoPath)
	case RepoTypeHelmUmbrella:
		return parseHelmUmbrella(repoPath)
	case RepoTypeSingleRepo:
		return parseSingleRepo(repoPath)
	default:
		// Try to parse whatever we find
		return parseSingleRepo(repoPath)
	}
}

// detectRepoType identifies the architecture pattern
func detectRepoType(repoPath string) RepoType {
	hasApps := dirExists(filepath.Join(repoPath, "apps"))
	hasClusters := dirExists(filepath.Join(repoPath, "clusters"))
	hasTenants := dirExists(filepath.Join(repoPath, "tenants"))
	hasComponents := dirExists(filepath.Join(repoPath, "components"))
	hasInfra := dirExists(filepath.Join(repoPath, "infrastructure"))
	hasRootApp := dirExists(filepath.Join(repoPath, "root-app"))
	hasGenerators := dirExists(filepath.Join(repoPath, "generators"))
	hasAppSets := dirExists(filepath.Join(repoPath, "my-application-sets")) ||
		dirExists(filepath.Join(repoPath, "application-sets")) ||
		dirExists(filepath.Join(repoPath, "applicationsets"))

	// Check for Helm umbrella chart (Chart.yaml with dependencies)
	if isHelmUmbrellaChart(repoPath) {
		return RepoTypeHelmUmbrella
	}

	// App-of-apps: has root-app/ directory OR apps/ with Argo Application manifests
	if hasRootApp || isArgoAppOfApps(repoPath) {
		return RepoTypeAppOfApps
	}

	// ApplicationSet: has generators/ or applicationsets/ OR contains ApplicationSet YAMLs
	if hasGenerators || hasAppSets || hasApplicationSetYAMLs(repoPath) {
		return RepoTypeApplicationSet
	}

	// D2 Fleet: has clusters/ and tenants/, no apps/
	if hasClusters && hasTenants && !hasApps {
		return RepoTypeD2Fleet
	}

	// D2 Infra/Apps: has components/, check for controllers vs namespaced
	if hasComponents && !hasApps {
		// Check if any component has controllers/ or configs/ subdirs (infra pattern)
		componentsDir := filepath.Join(repoPath, "components")
		if entries, err := os.ReadDir(componentsDir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					componentPath := filepath.Join(componentsDir, entry.Name())
					if dirExists(filepath.Join(componentPath, "controllers")) ||
						dirExists(filepath.Join(componentPath, "configs")) {
						return RepoTypeD2Infra
					}
				}
			}
		}
		return RepoTypeD2Apps
	}

	// Traditional single-repo: has apps/ or infrastructure/
	if hasApps || hasInfra {
		return RepoTypeSingleRepo
	}

	return RepoTypeUnknown
}

// isHelmUmbrellaChart checks if the repo is a Helm umbrella chart
func isHelmUmbrellaChart(repoPath string) bool {
	chartFile := filepath.Join(repoPath, "Chart.yaml")
	if !fileExists(chartFile) {
		return false
	}

	// Read Chart.yaml and check for dependencies
	data, err := os.ReadFile(chartFile)
	if err != nil {
		return false
	}

	var chart struct {
		Dependencies []struct {
			Name string `yaml:"name"`
		} `yaml:"dependencies"`
	}
	if err := yaml.Unmarshal(data, &chart); err != nil {
		return false
	}

	return len(chart.Dependencies) > 0
}

// isArgoAppOfApps checks if this is an Argo app-of-apps pattern
// Detects: root app-of-apps.yaml pointing to apps/ OR apps/ containing Argo Applications
func isArgoAppOfApps(repoPath string) bool {
	// Check for root-level app-of-apps manifest
	entries, err := os.ReadDir(repoPath)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Common naming patterns: app-of-apps.yaml, apps.yaml, root-app.yaml
		if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
			if strings.Contains(name, "app-of-apps") || name == "apps.yaml" || name == "root-app.yaml" {
				// Verify it's an Argo Application
				if isArgoApplication(filepath.Join(repoPath, name)) {
					return true
				}
			}
		}
	}

	// Check if apps/ contains Argo Application manifests (not kustomizations)
	appsDir := filepath.Join(repoPath, "apps")
	if !dirExists(appsDir) {
		return false
	}

	// Look for Argo Application YAMLs in apps/
	appEntries, err := os.ReadDir(appsDir)
	if err != nil {
		return false
	}

	argoAppCount := 0
	for _, entry := range appEntries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
			if isArgoApplication(filepath.Join(appsDir, name)) {
				argoAppCount++
			}
		}
	}

	// If apps/ has multiple Argo Application manifests, it's app-of-apps
	return argoAppCount >= 2
}

// isArgoApplication checks if a file is an Argo CD Application manifest
func isArgoApplication(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	var manifest struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
	}
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return false
	}

	return strings.HasPrefix(manifest.APIVersion, "argoproj.io/") && manifest.Kind == "Application"
}

// hasApplicationSetYAMLs checks if the repo contains ApplicationSet manifests
func hasApplicationSetYAMLs(repoPath string) bool {
	count := 0
	filepath.WalkDir(repoPath, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		// Skip hidden directories and common non-manifest paths
		if strings.Contains(path, "/.") || strings.Contains(path, "/vendor/") ||
			strings.Contains(path, "/node_modules/") {
			return nil
		}
		name := d.Name()
		if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
			if isApplicationSet(path) {
				count++
				if count >= 1 {
					return filepath.SkipAll // Found at least one, stop walking
				}
			}
		}
		return nil
	})
	return count > 0
}

// isApplicationSet checks if a file is an Argo CD ApplicationSet manifest
func isApplicationSet(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	var manifest struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
	}
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return false
	}

	return strings.HasPrefix(manifest.APIVersion, "argoproj.io/") && manifest.Kind == "ApplicationSet"
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// dirExists checks if a directory exists
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// parseSingleRepo parses traditional single-repo structure
func parseSingleRepo(repoPath string) (*RepoStructure, error) {
	result := &RepoStructure{Type: RepoTypeSingleRepo}

	if dirExists(filepath.Join(repoPath, "apps")) {
		apps, err := parseAppsDirectory(repoPath)
		if err != nil {
			return nil, err
		}
		result.Apps = apps
	}

	if dirExists(filepath.Join(repoPath, "infrastructure")) {
		infra, err := parseInfraDirectory(repoPath)
		if err != nil {
			return nil, err
		}
		result.Infrastructure = infra
	}

	if dirExists(filepath.Join(repoPath, "clusters")) {
		clusters, err := parseClustersDirectory(repoPath)
		if err != nil {
			return nil, err
		}
		result.Clusters = clusters
	}

	return result, nil
}

// parseD2Fleet parses d2-fleet repo structure
func parseD2Fleet(repoPath string) (*RepoStructure, error) {
	result := &RepoStructure{Type: RepoTypeD2Fleet}

	// Parse clusters/
	if dirExists(filepath.Join(repoPath, "clusters")) {
		clusters, err := parseClustersDirectory(repoPath)
		if err != nil {
			return nil, err
		}
		result.Clusters = clusters
	}

	// Parse tenants/
	tenantsDir := filepath.Join(repoPath, "tenants")
	if dirExists(tenantsDir) {
		entries, err := os.ReadDir(tenantsDir)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				result.Tenants = append(result.Tenants, TenantDefinition{
					Name: entry.Name(),
					Path: filepath.Join("tenants", entry.Name()),
				})
			}
		}
	}

	return result, nil
}

// parseD2Components parses d2-infra or d2-apps repo structure
func parseD2Components(repoPath string, repoType RepoType) (*RepoStructure, error) {
	result := &RepoStructure{Type: repoType}

	componentsDir := filepath.Join(repoPath, "components")
	if !dirExists(componentsDir) {
		return result, nil
	}

	entries, err := os.ReadDir(componentsDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		componentPath := filepath.Join(componentsDir, entry.Name())
		component := ComponentDefinition{
			Name: entry.Name(),
			Path: filepath.Join("components", entry.Name()),
		}

		// Check if this is d2-infra style (has controllers/ or configs/ subdirs)
		hasControllers := dirExists(filepath.Join(componentPath, "controllers"))
		hasConfigs := dirExists(filepath.Join(componentPath, "configs"))

		if hasControllers || hasConfigs {
			// d2-infra style: component has controllers/ and configs/ subdirs
			component.Type = "infra"

			// Find variants from controllers or configs subdirs
			variantSet := make(map[string]bool)
			for _, subdir := range []string{"controllers", "configs"} {
				subdirPath := filepath.Join(componentPath, subdir)
				if subEntries, err := os.ReadDir(subdirPath); err == nil {
					for _, sub := range subEntries {
						if sub.IsDir() {
							variantSet[normalizeVariant(sub.Name())] = true
						}
					}
				}
			}
			for v := range variantSet {
				component.Variants = append(component.Variants, v)
			}
		} else {
			// d2-apps style: component has base/staging/production directly
			component.Type = "app"

			subEntries, _ := os.ReadDir(componentPath)
			for _, sub := range subEntries {
				if sub.IsDir() {
					name := sub.Name()
					if name == "base" || name == "staging" || name == "production" ||
						name == "prod" || name == "dev" {
						component.Variants = append(component.Variants, normalizeVariant(name))
					}
				}
			}
		}

		result.Components = append(result.Components, component)
	}

	return result, nil
}

// parseAppsDirectory parses the apps/ directory structure
func parseAppsDirectory(repoPath string) ([]AppDefinition, error) {
	appsDir := filepath.Join(repoPath, "apps")

	// First, find base templates
	bases := make(map[string]string) // app name -> base path
	baseDir := filepath.Join(appsDir, "base")
	if entries, err := os.ReadDir(baseDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				bases[entry.Name()] = filepath.Join("apps", "base", entry.Name())
			}
		}
	}

	// Find variants (overlays)
	variants := []VariantDefinition{}
	entries, err := os.ReadDir(appsDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "base" {
			continue
		}

		variantPath := filepath.Join(appsDir, entry.Name())
		variant := VariantDefinition{
			Name: normalizeVariant(entry.Name()),
			Path: filepath.Join("apps", entry.Name()),
			Apps: []string{},
		}

		// Parse kustomization.yaml to find what it includes
		kustFile := filepath.Join(variantPath, "kustomization.yaml")
		if kust, err := parseKustomization(kustFile); err == nil {
			for _, res := range kust.Resources {
				// Check if it references a base
				if strings.Contains(res, "../base/") {
					parts := strings.Split(res, "/")
					if len(parts) >= 3 {
						appName := parts[len(parts)-1]
						variant.Apps = append(variant.Apps, appName)
						variant.References = "apps/base"
					}
				}
			}
		}

		// Also check for app-specific patches
		patchFiles, _ := filepath.Glob(filepath.Join(variantPath, "*-patch.yaml"))
		for _, pf := range patchFiles {
			base := filepath.Base(pf)
			appName := strings.TrimSuffix(base, "-patch.yaml")
			if !contains(variant.Apps, appName) {
				variant.Apps = append(variant.Apps, appName)
			}
		}

		variants = append(variants, variant)
	}

	// Build app definitions
	apps := []AppDefinition{}
	for appName, basePath := range bases {
		app := AppDefinition{
			Name:     appName,
			BasePath: basePath,
			Variants: []VariantDefinition{},
		}

		// Find which variants include this app
		for _, v := range variants {
			if contains(v.Apps, appName) {
				app.Variants = append(app.Variants, VariantDefinition{
					Name: v.Name,
					Path: v.Path,
				})
			}
		}

		apps = append(apps, app)
	}

	// Handle apps without base (directly in variant dirs)
	for _, v := range variants {
		for _, appName := range v.Apps {
			if _, exists := bases[appName]; !exists {
				// App exists only in variant, no base
				found := false
				for i := range apps {
					if apps[i].Name == appName {
						found = true
						break
					}
				}
				if !found {
					apps = append(apps, AppDefinition{
						Name: appName,
						Variants: []VariantDefinition{{
							Name: v.Name,
							Path: v.Path,
						}},
					})
				}
			}
		}
	}

	return apps, nil
}

// parseInfraDirectory parses infrastructure/ directory
func parseInfraDirectory(repoPath string) ([]InfraDefinition, error) {
	infraDir := filepath.Join(repoPath, "infrastructure")
	result := []InfraDefinition{}

	entries, err := os.ReadDir(infraDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			result = append(result, InfraDefinition{
				Name: entry.Name(),
				Path: filepath.Join("infrastructure", entry.Name()),
			})
		}
	}

	return result, nil
}

// parseClustersDirectory parses clusters/ directory
func parseClustersDirectory(repoPath string) ([]ClusterDefinition, error) {
	clustersDir := filepath.Join(repoPath, "clusters")
	result := []ClusterDefinition{}

	entries, err := os.ReadDir(clustersDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		cluster := ClusterDefinition{
			Name: entry.Name(),
			Path: filepath.Join("clusters", entry.Name()),
			Apps: []string{},
		}

		// Find what apps this cluster references
		clusterPath := filepath.Join(clustersDir, entry.Name())
		files, _ := os.ReadDir(clusterPath)
		for _, f := range files {
			if strings.HasSuffix(f.Name(), ".yaml") {
				name := strings.TrimSuffix(f.Name(), ".yaml")
				cluster.Apps = append(cluster.Apps, name)
			}
		}

		result = append(result, cluster)
	}

	return result, nil
}

// parseKustomization reads and parses a kustomization.yaml file
func parseKustomization(path string) (*Kustomization, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var kust Kustomization
	if err := yaml.Unmarshal(data, &kust); err != nil {
		return nil, err
	}

	return &kust, nil
}

// normalizeVariant normalizes variant names
func normalizeVariant(name string) string {
	switch strings.ToLower(name) {
	case "production", "prod":
		return "prod"
	case "staging", "stage", "stg":
		return "staging"
	case "development", "dev":
		return "dev"
	default:
		return name
	}
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// parseAppOfApps parses Argo app-of-apps pattern
func parseAppOfApps(repoPath string) (*RepoStructure, error) {
	result := &RepoStructure{Type: RepoTypeAppOfApps}

	// Pattern 1: root-app/ directory
	rootAppDir := filepath.Join(repoPath, "root-app")
	if dirExists(rootAppDir) {
		files, _ := filepath.Glob(filepath.Join(rootAppDir, "*.yaml"))
		for _, f := range files {
			if app := parseArgoApplication(f); app != nil {
				result.RootApp = app
				break
			}
		}
	}

	// Pattern 2: root-level app-of-apps.yaml (CNCF pattern)
	entries, _ := os.ReadDir(repoPath)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if (strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")) &&
			(strings.Contains(name, "app-of-apps") || name == "apps.yaml" || name == "root-app.yaml") {
			if app := parseArgoApplication(filepath.Join(repoPath, name)); app != nil {
				result.RootApp = app
				break
			}
		}
	}

	// Pattern 3: apps/ directory with Argo Application manifests
	appsDir := filepath.Join(repoPath, "apps")
	if dirExists(appsDir) {
		appFiles, _ := os.ReadDir(appsDir)
		for _, entry := range appFiles {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
				if app := parseArgoApplication(filepath.Join(appsDir, name)); app != nil {
					result.ChildApps = append(result.ChildApps, *app)
				}
			}
		}
	}

	// Pattern 4: charts/ directory with Helm charts
	chartsDir := filepath.Join(repoPath, "charts")
	if dirExists(chartsDir) {
		chartEntries, _ := os.ReadDir(chartsDir)
		for _, entry := range chartEntries {
			if !entry.IsDir() {
				continue
			}
			chartPath := filepath.Join(chartsDir, entry.Name())
			if fileExists(filepath.Join(chartPath, "Chart.yaml")) {
				// Only add if not already in ChildApps from apps/ manifests
				found := false
				for _, child := range result.ChildApps {
					if child.Name == entry.Name() {
						found = true
						break
					}
				}
				if !found {
					result.ChildApps = append(result.ChildApps, ArgoAppDefinition{
						Name:   entry.Name(),
						Path:   filepath.Join("charts", entry.Name()),
						Source: "helm",
					})
				}
			}
		}
	}

	// Legacy: find child apps in other directories (root-app pattern)
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "root-app" || entry.Name() == "apps" ||
			entry.Name() == "charts" || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		dirPath := filepath.Join(repoPath, entry.Name())
		yamlFiles, _ := filepath.Glob(filepath.Join(dirPath, "*.yaml"))
		for _, f := range yamlFiles {
			if app := parseArgoApplication(f); app != nil {
				result.ChildApps = append(result.ChildApps, *app)
			}
		}

		if fileExists(filepath.Join(dirPath, "Chart.yaml")) {
			result.ChildApps = append(result.ChildApps, ArgoAppDefinition{
				Name:   entry.Name(),
				Path:   entry.Name(),
				Source: "helm",
			})
		}
	}

	return result, nil
}

// parseArgoApplication parses an Argo CD Application YAML
func parseArgoApplication(path string) *ArgoAppDefinition {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var app struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
		Metadata   struct {
			Name string `yaml:"name"`
		} `yaml:"metadata"`
		Spec struct {
			Destination struct {
				Server    string `yaml:"server"`
				Namespace string `yaml:"namespace"`
			} `yaml:"destination"`
			Source struct {
				RepoURL string `yaml:"repoURL"`
				Path    string `yaml:"path"`
			} `yaml:"source"`
		} `yaml:"spec"`
	}

	if err := yaml.Unmarshal(data, &app); err != nil {
		return nil
	}

	if app.Kind != "Application" {
		return nil
	}

	return &ArgoAppDefinition{
		Name:        app.Metadata.Name,
		Path:        app.Spec.Source.Path,
		Destination: app.Spec.Destination.Namespace,
		Source:      app.Spec.Source.RepoURL,
	}
}

// parseApplicationSets parses ApplicationSet definitions
func parseApplicationSets(repoPath string) (*RepoStructure, error) {
	result := &RepoStructure{Type: RepoTypeApplicationSet}

	// First try specific directories
	searchDirs := []string{
		"generators",
		"my-application-sets",
		"application-sets",
		"applicationsets",
	}

	foundInDirs := false
	for _, dir := range searchDirs {
		dirPath := filepath.Join(repoPath, dir)
		if !dirExists(dirPath) {
			continue
		}
		foundInDirs = true

		filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".yaml") {
				return nil
			}

			if appSet := parseApplicationSet(path, repoPath); appSet != nil {
				result.ApplicationSets = append(result.ApplicationSets, *appSet)
			}
			return nil
		})
	}

	// If no specific dirs found, walk entire repo
	if !foundInDirs {
		filepath.WalkDir(repoPath, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			// Skip hidden directories and common non-manifest paths
			if strings.Contains(path, "/.") || strings.Contains(path, "/vendor/") ||
				strings.Contains(path, "/node_modules/") {
				return nil
			}
			name := d.Name()
			if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
				if appSet := parseApplicationSet(path, repoPath); appSet != nil {
					result.ApplicationSets = append(result.ApplicationSets, *appSet)
				}
			}
			return nil
		})
	}

	// Populate Apps from discovered ApplicationSet targets
	// TargetApps now contains paths as unique identifiers
	seenPaths := make(map[string]bool)
	for _, appSet := range result.ApplicationSets {
		for _, appPath := range appSet.TargetApps {
			// Skip duplicates (same path from different ApplicationSets)
			if seenPaths[appPath] {
				continue
			}
			seenPaths[appPath] = true

			// Get the display name from TargetAppNames
			appName := appSet.TargetAppNames[appPath]
			if appName == "" {
				// Fallback to basename if name not found
				appName = filepath.Base(appPath)
			}

			result.Apps = append(result.Apps, AppDefinition{
				Name:     appName,
				BasePath: appPath,
			})
		}
	}

	return result, nil
}

// findAppBasePath attempts to find the base path for an app discovered via ApplicationSet
func findAppBasePath(repoPath, appName string) string {
	// Common patterns for app locations
	searchPatterns := []string{
		filepath.Join("apps", appName),
		filepath.Join("apps", "base", appName),
		filepath.Join("applications", appName),
		filepath.Join("workloads", appName),
		appName,
	}

	for _, pattern := range searchPatterns {
		fullPath := filepath.Join(repoPath, pattern)
		if dirExists(fullPath) {
			return pattern
		}
	}

	// Return empty if not found - the app was discovered but path is unknown
	return ""
}

// parseApplicationSet parses an ApplicationSet YAML and extracts git generator patterns
func parseApplicationSet(path, repoPath string) *ApplicationSetDef {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var appSet struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
		Metadata   struct {
			Name string `yaml:"name"`
		} `yaml:"metadata"`
		Spec struct {
			Generators []map[string]interface{} `yaml:"generators"`
		} `yaml:"spec"`
	}

	if err := yaml.Unmarshal(data, &appSet); err != nil {
		return nil
	}

	if appSet.Kind != "ApplicationSet" {
		return nil
	}

	// Determine generator type and extract git generator patterns
	generator := "unknown"
	var allPatterns gitGeneratorPatterns

	for _, gen := range appSet.Spec.Generators {
		for k, v := range gen {
			if generator == "unknown" {
				generator = k
			}

			// Extract git generator directory patterns
			if k == "git" {
				patterns := extractGitGeneratorPatternsWithExcludes(v)
				allPatterns.Include = append(allPatterns.Include, patterns.Include...)
				allPatterns.Exclude = append(allPatterns.Exclude, patterns.Exclude...)
			}

			// Handle matrix generators that may contain git generators
			if k == "matrix" {
				patterns := extractMatrixGitPatternsWithExcludes(v)
				allPatterns.Include = append(allPatterns.Include, patterns.Include...)
				allPatterns.Exclude = append(allPatterns.Exclude, patterns.Exclude...)
			}
		}
	}

	relPath, _ := filepath.Rel(repoPath, path)
	result := &ApplicationSetDef{
		Name:           appSet.Metadata.Name,
		Path:           relPath,
		Generator:      generator,
		TargetAppNames: make(map[string]string),
	}

	// Scan directories matching git generator patterns
	if len(allPatterns.Include) > 0 {
		discovered := scanGitGeneratorPatternsWithExcludesFullPaths(repoPath, allPatterns)
		for _, app := range discovered {
			// Store path as the unique identifier in TargetApps
			result.TargetApps = append(result.TargetApps, app.Path)
			// Map path -> name for display purposes
			result.TargetAppNames[app.Path] = app.Name
		}
	}

	return result
}

// gitGeneratorPatterns holds include and exclude patterns from a git generator
type gitGeneratorPatterns struct {
	Include []string
	Exclude []string
}

// discoveredApp represents an app discovered via git generator scanning
type discoveredApp struct {
	Name string // basename of the directory
	Path string // relative path from repo root
}

// extractGitGeneratorPatterns extracts directory patterns from a git generator
func extractGitGeneratorPatterns(gitGen interface{}) []string {
	patterns := extractGitGeneratorPatternsWithExcludes(gitGen)
	return patterns.Include
}

// extractGitGeneratorPatternsWithExcludes extracts both include and exclude patterns
func extractGitGeneratorPatternsWithExcludes(gitGen interface{}) gitGeneratorPatterns {
	var patterns gitGeneratorPatterns

	gitMap, ok := gitGen.(map[string]interface{})
	if !ok {
		return patterns
	}

	// Extract directories[].path patterns
	dirs, ok := gitMap["directories"]
	if !ok {
		return patterns
	}

	dirSlice, ok := dirs.([]interface{})
	if !ok {
		return patterns
	}

	for _, d := range dirSlice {
		dirMap, ok := d.(map[string]interface{})
		if !ok {
			continue
		}

		if pathVal, ok := dirMap["path"]; ok {
			if pathStr, ok := pathVal.(string); ok {
				if strings.HasPrefix(pathStr, "!") {
					// Exclude pattern - remove the ! prefix
					patterns.Exclude = append(patterns.Exclude, strings.TrimPrefix(pathStr, "!"))
				} else {
					patterns.Include = append(patterns.Include, pathStr)
				}
			}
		}
	}

	return patterns
}

// extractMatrixGitPatterns extracts git patterns from matrix generators
func extractMatrixGitPatterns(matrixGen interface{}) []string {
	patterns := extractMatrixGitPatternsWithExcludes(matrixGen)
	return patterns.Include
}

// extractMatrixGitPatternsWithExcludes extracts git patterns from matrix generators including excludes
func extractMatrixGitPatternsWithExcludes(matrixGen interface{}) gitGeneratorPatterns {
	var patterns gitGeneratorPatterns

	matrixMap, ok := matrixGen.(map[string]interface{})
	if !ok {
		return patterns
	}

	// Matrix generators have a "generators" array
	generators, ok := matrixMap["generators"]
	if !ok {
		return patterns
	}

	genSlice, ok := generators.([]interface{})
	if !ok {
		return patterns
	}

	for _, g := range genSlice {
		genMap, ok := g.(map[string]interface{})
		if !ok {
			continue
		}

		// Look for git generator within matrix
		if gitGen, ok := genMap["git"]; ok {
			gitPatterns := extractGitGeneratorPatternsWithExcludes(gitGen)
			patterns.Include = append(patterns.Include, gitPatterns.Include...)
			patterns.Exclude = append(patterns.Exclude, gitPatterns.Exclude...)
		}
	}

	return patterns
}

// scanGitGeneratorPatterns scans the repo for directories matching git generator patterns
func scanGitGeneratorPatterns(repoPath string, patterns []string) []string {
	discovered := scanGitGeneratorPatternsWithExcludesFullPaths(repoPath, gitGeneratorPatterns{Include: patterns})
	names := make([]string, len(discovered))
	for i, d := range discovered {
		names[i] = d.Name
	}
	return names
}

// scanGitGeneratorPatternsWithExcludes scans the repo for directories matching git generator patterns
// while filtering out directories that match exclude patterns. Returns only app names for backward compatibility.
func scanGitGeneratorPatternsWithExcludes(repoPath string, patterns gitGeneratorPatterns) []string {
	discovered := scanGitGeneratorPatternsWithExcludesFullPaths(repoPath, patterns)
	names := make([]string, len(discovered))
	for i, d := range discovered {
		names[i] = d.Name
	}
	return names
}

// scanGitGeneratorPatternsWithExcludesFullPaths scans the repo for directories matching git generator patterns
// while filtering out directories that match exclude patterns. Returns full path information.
func scanGitGeneratorPatternsWithExcludesFullPaths(repoPath string, patterns gitGeneratorPatterns) []discoveredApp {
	var apps []discoveredApp
	seen := make(map[string]bool)

	for _, pattern := range patterns.Include {
		// Convert git generator pattern to filesystem glob
		// Git generator uses: apps/*, apps/**, clusters/*/apps/*
		// filepath.Glob doesn't support **, so we handle it specially

		if strings.Contains(pattern, "**") {
			// For ** patterns, we need to walk directories
			apps = append(apps, scanRecursivePatternWithExcludesFullPaths(repoPath, pattern, patterns.Exclude, seen)...)
		} else {
			// Standard glob
			fullPattern := filepath.Join(repoPath, pattern)
			matches, err := filepath.Glob(fullPattern)
			if err != nil {
				continue
			}

			for _, match := range matches {
				info, err := os.Stat(match)
				if err != nil || !info.IsDir() {
					continue
				}

				// Get relative path for exclude matching
				relPath, err := filepath.Rel(repoPath, match)
				if err != nil {
					continue
				}

				// Check if this path matches any exclude pattern
				if matchesExcludePattern(relPath, patterns.Exclude) {
					continue
				}

				// Use relPath as the unique key to avoid collisions between
				// apps with the same name in different directories
				if !seen[relPath] {
					seen[relPath] = true
					apps = append(apps, discoveredApp{
						Name: filepath.Base(match),
						Path: relPath,
					})
				}
			}
		}
	}

	return apps
}

// matchesExcludePattern checks if a path matches any exclude pattern
func matchesExcludePattern(relPath string, excludes []string) bool {
	for _, exclude := range excludes {
		// Exact match
		if relPath == exclude {
			return true
		}

		// Glob match (handle patterns like apps/excluded/*)
		if matched, _ := filepath.Match(exclude, relPath); matched {
			return true
		}

		// Check if the path starts with the exclude pattern (for directory exclusions)
		if strings.HasPrefix(relPath, exclude+"/") || strings.HasPrefix(relPath+"/", exclude+"/") {
			return true
		}
	}
	return false
}

// scanRecursivePatternWithExcludesFullPaths handles ** patterns while respecting excludes, returning full path info
func scanRecursivePatternWithExcludesFullPaths(repoPath, pattern string, excludes []string, seen map[string]bool) []discoveredApp {
	var apps []discoveredApp

	// Split pattern at **
	parts := strings.SplitN(pattern, "**", 2)
	if len(parts) != 2 {
		return apps
	}

	baseDir := filepath.Join(repoPath, strings.TrimSuffix(parts[0], "/"))
	suffix := strings.TrimPrefix(parts[1], "/")

	if !dirExists(baseDir) {
		return apps
	}

	filepath.WalkDir(baseDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}

		// Skip hidden directories
		if strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}

		// Get relative path for exclude matching
		relPath, err := filepath.Rel(repoPath, path)
		if err != nil {
			return nil
		}

		// Check if this path matches any exclude pattern
		if matchesExcludePattern(relPath, excludes) {
			return filepath.SkipDir
		}

		// If there's a suffix pattern, check if this directory matches
		if suffix != "" {
			// For patterns like **/kustomize, check if the dir name matches
			if !matchSuffix(path, repoPath, suffix) {
				return nil
			}
		}

		// Use relPath as the unique key to avoid collisions between
		// apps with the same name in different directories
		if !seen[relPath] && relPath != strings.TrimSuffix(parts[0], "/") {
			seen[relPath] = true
			apps = append(apps, discoveredApp{
				Name: d.Name(),
				Path: relPath,
			})
		}

		return nil
	})

	return apps
}

// matchSuffix checks if a path matches the suffix pattern
func matchSuffix(path, repoPath, suffix string) bool {
	relPath, err := filepath.Rel(repoPath, path)
	if err != nil {
		return false
	}

	// Simple suffix matching - check if the path ends with the pattern
	if strings.HasSuffix(relPath, suffix) {
		return true
	}

	// Check if it's a wildcard match
	if strings.Contains(suffix, "*") {
		matched, _ := filepath.Match(suffix, filepath.Base(path))
		return matched
	}

	return false
}

// parseHelmUmbrella parses a Helm umbrella chart
func parseHelmUmbrella(repoPath string) (*RepoStructure, error) {
	result := &RepoStructure{Type: RepoTypeHelmUmbrella}

	chartFile := filepath.Join(repoPath, "Chart.yaml")
	data, err := os.ReadFile(chartFile)
	if err != nil {
		return result, nil
	}

	var chart struct {
		Name         string `yaml:"name"`
		Version      string `yaml:"version"`
		Dependencies []struct {
			Name       string `yaml:"name"`
			Version    string `yaml:"version"`
			Repository string `yaml:"repository"`
		} `yaml:"dependencies"`
	}

	if err := yaml.Unmarshal(data, &chart); err != nil {
		return result, nil
	}

	result.HelmChart = &HelmChartDefinition{
		Name:    chart.Name,
		Version: chart.Version,
	}

	for _, dep := range chart.Dependencies {
		result.HelmChart.Dependencies = append(result.HelmChart.Dependencies, HelmDependency{
			Name:       dep.Name,
			Version:    dep.Version,
			Repository: dep.Repository,
		})
	}

	return result, nil
}
