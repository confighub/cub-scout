// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/confighub/cub-scout/pkg/gitops"
	"github.com/spf13/cobra"
)

var (
	combinedGitURL         string
	combinedGitPath        string
	combinedGitURLCompare  string
	combinedGitPathCompare string
	combinedNamespace      string
	combinedBundle         string
	combinedFormat         string
	combinedJSON           bool
	combinedSuggest        bool
	combinedApply          bool
	combinedDryRun         bool
)

// CombinedResult shows Git repo structure + Cluster workloads together
type CombinedResult struct {
	GitRepo    *gitops.RepoStructure `json:"gitRepo,omitempty"`
	GitCompare []GitCompareEntry     `json:"gitCompare,omitempty"`
	Cluster    *ImportResult         `json:"cluster,omitempty"`
	Alignment  []AlignmentEntry      `json:"alignment,omitempty"`
	Proposal   *FullProposal         `json:"proposal,omitempty"` // App model proposal
}

// AlignmentEntry shows how Git apps align with cluster workloads
type AlignmentEntry struct {
	App        string   `json:"app"`
	GitVariant string   `json:"gitVariant,omitempty"` // From parser
	LivePath   string   `json:"livePath,omitempty"`   // From cluster deployer
	Status     string   `json:"status"`               // "aligned", "git-only", "cluster-only"
	Workloads  []string `json:"workloads,omitempty"`
}

// GitCompareEntry represents a Git↔Git comparison row.
type GitCompareEntry struct {
	App       string `json:"app"`
	Variant   string `json:"variant,omitempty"`
	Status    string `json:"status"` // "aligned", "left-only", "right-only"
	LeftPath  string `json:"leftPath,omitempty"`
	RightPath string `json:"rightPath,omitempty"`
}

var combinedCmd = &cobra.Command{
	Use:   "compare",
	Short: "Compare Git repo structure + cluster workloads",
	Long: `Parse a Git repo and scan a cluster, showing alignment between them.

This helps you understand:
- What apps are defined in Git
- What workloads are deployed in the cluster
- How they align (or don't)

Use --suggest to generate a full App model proposal.
Use --apply to create the App and Deployments in ConfigHub.

Examples:
  # Compare one live resource (resource mode)
  cub-scout compare deploy/my-app -n prod --format ascii

  # Compare one live resource with JSON output
  cub-scout compare deploy/my-app -n prod --format json

  # Combine Git repo with current cluster
  cub-scout compare --git-url https://github.com/org/gitops-repo --namespace demo

  # Generate App model proposal
  cub-scout compare --git-url https://github.com/org/gitops-repo --namespace demo --suggest

  # Preview what would be created (dry-run)
  cub-scout compare --namespace demo --suggest --apply --dry-run

  # Apply: create App and Deployments in ConfigHub
  cub-scout compare --namespace demo --suggest --apply

  # Use local Git repo with JSON output
  cub-scout compare --git-path ./my-repo --namespace demo --suggest --json

  # Offline Git ↔ cluster compare from a debug bundle
  cub-scout compare --git-path ./my-repo --bundle ./debug-bundle --json

  # Git ↔ Git compare (left vs right repo)
  cub-scout compare --git-path ./repo-a --git-path-compare ./repo-b --json
`,
	RunE: runCombined,
}

func init() {
	// combined remains a backward-compatible alias for one release.
	combinedCmd.Aliases = []string{"combined"}

	combinedCmd.Flags().StringVar(&combinedGitURL, "git-url", "", "Git repository URL to parse")
	combinedCmd.Flags().StringVar(&combinedGitPath, "git-path", "", "Local path to Git repository")
	combinedCmd.Flags().StringVar(&combinedGitURLCompare, "git-url-compare", "", "Right-side Git repository URL for Git↔Git compare")
	combinedCmd.Flags().StringVar(&combinedGitPathCompare, "git-path-compare", "", "Right-side local Git repository path for Git↔Git compare")
	combinedCmd.Flags().StringVarP(&combinedNamespace, "namespace", "n", "", "Namespace to scan in cluster")
	combinedCmd.Flags().StringVar(&combinedBundle, "bundle", "", "Debug bundle directory to use as cluster snapshot (offline)")
	combinedCmd.Flags().StringVar(&combinedFormat, "format", "ascii", "Output format for resource compare mode: ascii, json, md")
	combinedCmd.Flags().BoolVar(&combinedJSON, "json", false, "Output as JSON")
	combinedCmd.Flags().BoolVar(&combinedSuggest, "suggest", false, "Generate App model proposal")
	combinedCmd.Flags().BoolVar(&combinedApply, "apply", false, "Create App and Deployments in ConfigHub")
	combinedCmd.Flags().BoolVar(&combinedDryRun, "dry-run", false, "Show what would be created without making changes")

	rootCmd.AddCommand(combinedCmd)
}

func runCombined(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return runCombinedResourceCompare(cmd, args)
	}

	result := &CombinedResult{}

	// Parse left-side Git repo (primary).
	leftRepo, leftCleanup, err := loadCombinedRepoSource(combinedGitURL, combinedGitPath)
	if err != nil {
		return err
	}
	if leftCleanup != nil {
		defer leftCleanup()
	}
	result.GitRepo = leftRepo

	// Parse right-side Git repo (compare).
	rightRepo, rightCleanup, err := loadCombinedRepoSource(combinedGitURLCompare, combinedGitPathCompare)
	if err != nil {
		return err
	}
	if rightCleanup != nil {
		defer rightCleanup()
	}
	if rightRepo != nil && leftRepo == nil {
		return fmt.Errorf("--git-url-compare/--git-path-compare requires --git-url or --git-path")
	}
	if leftRepo != nil && rightRepo != nil {
		result.GitCompare = buildGitComparison(leftRepo, rightRepo)
	}

	// Collect cluster-side workloads from live namespace OR bundle snapshot.
	workloads, clusterNamespace, err := resolveCombinedWorkloadsSource(combinedNamespace, combinedBundle)
	if err != nil {
		return err
	}
	if len(workloads) > 0 || clusterNamespace != "" {
		suggestion := SuggestAppStructure(workloads, "")
		suggestionJSON := convertToSuggestionJSON(&suggestion)

		result.Cluster = &ImportResult{
			Namespace:  clusterNamespace,
			Model:      "app-deployment-target",
			Workloads:  convertToWorkloadJSON(workloads),
			Suggestion: suggestionJSON,
		}
	}

	// Build alignment if we have both
	if leftRepo != nil && result.Cluster != nil {
		result.Alignment = buildAlignment(leftRepo, result.Cluster)
	}

	// Build full App model proposal if --suggest
	if combinedSuggest && leftRepo != nil {
		result.Proposal = SuggestFullProposal(leftRepo.Apps, workloads, "")
	}

	// Build proposal from cluster-only if no Git repo
	if combinedSuggest && result.GitRepo == nil && len(workloads) > 0 {
		result.Proposal = SuggestFullProposal(nil, workloads, "")
	}

	// Apply: create App and Deployments in ConfigHub
	if combinedApply && result.Proposal != nil {
		if err := applyProposal(result.Proposal, workloads, combinedDryRun); err != nil {
			return err
		}
		if !combinedDryRun {
			fmt.Println("\n✓ Import complete")
		}
		return nil
	}

	if combinedJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	// Pretty print
	if combinedSuggest && result.Proposal != nil {
		result.Proposal.Print()
	} else {
		printCombinedResult(result)
	}
	return nil
}

func loadCombinedRepoSource(gitURL, gitPath string) (*gitops.RepoStructure, func(), error) {
	gitURL = strings.TrimSpace(gitURL)
	gitPath = strings.TrimSpace(gitPath)

	if gitURL == "" && gitPath == "" {
		return nil, nil, nil
	}
	if gitURL != "" && gitPath != "" {
		return nil, nil, fmt.Errorf("--git-url and --git-path cannot be used together")
	}

	var (
		repoPath string
		cleanup  func()
	)
	if gitURL != "" {
		tmpDir, err := os.MkdirTemp("", "gitops-combined-*")
		if err != nil {
			return nil, nil, fmt.Errorf("create temp dir: %w", err)
		}
		cleanup = func() { os.RemoveAll(tmpDir) }

		if !combinedJSON {
			fmt.Fprintf(os.Stderr, "Cloning %s...\n", gitURL)
		}
		gitCmd := exec.Command("git", "clone", "--depth=1", gitURL, tmpDir)
		if output, err := gitCmd.CombinedOutput(); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("clone failed: %w\n%s", err, output)
		}
		repoPath = tmpDir
	} else {
		repoPath = gitPath
	}

	repo, err := gitops.ParseRepo(repoPath)
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return nil, nil, fmt.Errorf("parse repo: %w", err)
	}
	return repo, cleanup, nil
}

func resolveCombinedWorkloadsSource(namespace, bundlePath string) ([]WorkloadInfo, string, error) {
	if namespace != "" && bundlePath != "" {
		return nil, "", fmt.Errorf("--namespace and --bundle cannot be used together")
	}

	if bundlePath != "" {
		_, workloads, namespaces, err := buildImportFromBundlePreview(bundlePath)
		if err != nil {
			return nil, "", fmt.Errorf("load bundle workloads: %w", err)
		}
		clusterNamespace := ""
		if len(namespaces) > 0 {
			clusterNamespace = namespaces[0]
		}
		return workloads, clusterNamespace, nil
	}

	if namespace != "" {
		workloads, err := discoverWorkloads(namespace)
		if err != nil {
			return nil, "", fmt.Errorf("discover workloads: %w", err)
		}
		return workloads, namespace, nil
	}

	return []WorkloadInfo{}, "", nil
}

func buildGitComparison(left, right *gitops.RepoStructure) []GitCompareEntry {
	if left == nil || right == nil {
		return nil
	}

	leftIndex := indexRepoAppsForCompare(left)
	rightIndex := indexRepoAppsForCompare(right)

	keySet := make(map[gitCompareKey]struct{}, len(leftIndex)+len(rightIndex))
	for k := range leftIndex {
		keySet[k] = struct{}{}
	}
	for k := range rightIndex {
		keySet[k] = struct{}{}
	}

	keys := make([]gitCompareKey, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].App != keys[j].App {
			return keys[i].App < keys[j].App
		}
		return keys[i].Variant < keys[j].Variant
	})

	entries := make([]GitCompareEntry, 0, len(keys))
	for _, key := range keys {
		leftPath, leftOK := leftIndex[key]
		rightPath, rightOK := rightIndex[key]
		entry := GitCompareEntry{
			App:       key.App,
			Variant:   key.Variant,
			LeftPath:  leftPath,
			RightPath: rightPath,
		}
		switch {
		case leftOK && rightOK:
			entry.Status = "aligned"
		case leftOK:
			entry.Status = "left-only"
		default:
			entry.Status = "right-only"
		}
		entries = append(entries, entry)
	}
	return entries
}

type gitCompareKey struct {
	App     string
	Variant string
}

func indexRepoAppsForCompare(repo *gitops.RepoStructure) map[gitCompareKey]string {
	index := make(map[gitCompareKey]string)
	for _, app := range repo.Apps {
		appName := strings.TrimSpace(app.Name)
		if appName == "" {
			continue
		}
		if len(app.Variants) == 0 {
			key := gitCompareKey{
				App:     appName,
				Variant: "default",
			}
			setComparePath(index, key, app.BasePath)
			continue
		}

		for _, variant := range app.Variants {
			variantName := strings.TrimSpace(variant.Name)
			if variantName == "" {
				variantName = "default"
			}
			path := strings.TrimSpace(variant.Path)
			if path == "" {
				path = app.BasePath
			}
			key := gitCompareKey{
				App:     appName,
				Variant: variantName,
			}
			setComparePath(index, key, path)
		}
	}
	return index
}

func setComparePath(index map[gitCompareKey]string, key gitCompareKey, path string) {
	current, exists := index[key]
	path = strings.TrimSpace(path)
	if !exists {
		index[key] = path
		return
	}
	// Keep deterministic tie-breaking for duplicate app/variant rows.
	if current == "" || (path != "" && path < current) {
		index[key] = path
	}
}

func buildAlignment(repo *gitops.RepoStructure, cluster *ImportResult) []AlignmentEntry {
	entries := []AlignmentEntry{}

	// Index cluster workloads by app name
	clusterApps := make(map[string][]WorkloadJSON)
	clusterPaths := make(map[string]string)
	for _, w := range cluster.Workloads {
		app := w.Labels["app.kubernetes.io/name"]
		if app == "" {
			app = w.Labels["app"]
		}
		if app == "" {
			app = w.Name
		}
		clusterApps[app] = append(clusterApps[app], w)
		if w.KustomizationPath != "" {
			clusterPaths[app] = w.KustomizationPath
		} else if w.ApplicationPath != "" {
			clusterPaths[app] = w.ApplicationPath
		}
	}

	// Process Git apps
	gitApps := make(map[string]bool)
	for _, app := range repo.Apps {
		gitApps[app.Name] = true
		for _, v := range app.Variants {
			entry := AlignmentEntry{
				App:        app.Name,
				GitVariant: v.Name,
			}

			if workloads, ok := clusterApps[app.Name]; ok {
				entry.Status = "aligned"
				entry.LivePath = clusterPaths[app.Name]
				for _, w := range workloads {
					entry.Workloads = append(entry.Workloads, fmt.Sprintf("%s/%s", w.Namespace, w.Name))
				}
			} else {
				entry.Status = "git-only"
			}
			entries = append(entries, entry)
		}
	}

	// Find cluster-only apps
	for app, workloads := range clusterApps {
		if !gitApps[app] {
			entry := AlignmentEntry{
				App:      app,
				Status:   "cluster-only",
				LivePath: clusterPaths[app],
			}
			for _, w := range workloads {
				entry.Workloads = append(entry.Workloads, fmt.Sprintf("%s/%s", w.Namespace, w.Name))
			}
			entries = append(entries, entry)
		}
	}

	return entries
}

func printCombinedResult(r *CombinedResult) {
	if r.GitRepo != nil {
		fmt.Println("┌─────────────────────────────────────────────────────────────┐")
		fmt.Println("│ GIT REPO                                                    │")
		fmt.Println("└─────────────────────────────────────────────────────────────┘")
		fmt.Printf("  Type: %s\n", r.GitRepo.Type)
		if len(r.GitRepo.Apps) > 0 {
			fmt.Println("  Apps:")
			for _, app := range r.GitRepo.Apps {
				fmt.Printf("    • %s\n", app.Name)
				for _, v := range app.Variants {
					fmt.Printf("      └─ %s (%s)\n", v.Name, v.Path)
				}
			}
		}
		fmt.Println()
	}

	if r.Cluster != nil {
		fmt.Println("┌─────────────────────────────────────────────────────────────┐")
		fmt.Println("│ CLUSTER                                                     │")
		fmt.Println("└─────────────────────────────────────────────────────────────┘")
		fmt.Printf("  Namespace: %s\n", r.Cluster.Namespace)
		fmt.Printf("  Workloads: %d\n", len(r.Cluster.Workloads))
		for _, w := range r.Cluster.Workloads {
			path := ""
			if w.KustomizationPath != "" {
				path = fmt.Sprintf(" (path: %s)", w.KustomizationPath)
			} else if w.ApplicationPath != "" {
				path = fmt.Sprintf(" (path: %s)", w.ApplicationPath)
			}
			fmt.Printf("    • %s/%s [%s]%s\n", w.Namespace, w.Name, w.Owner, path)
		}
		fmt.Println()
	}

	if len(r.Alignment) > 0 {
		fmt.Println("┌─────────────────────────────────────────────────────────────┐")
		fmt.Println("│ ALIGNMENT                                                   │")
		fmt.Println("└─────────────────────────────────────────────────────────────┘")
		for _, a := range r.Alignment {
			status := SymOK
			if a.Status == "git-only" {
				status = "⚠ git-only"
			} else if a.Status == "cluster-only" {
				status = "⚠ cluster-only"
			}
			fmt.Printf("  %s %s", status, a.App)
			if a.GitVariant != "" {
				fmt.Printf(" (variant=%s)", a.GitVariant)
			}
			if a.LivePath != "" {
				fmt.Printf(" [path: %s]", a.LivePath)
			}
			fmt.Println()
		}
		fmt.Println()
	}

	if len(r.GitCompare) > 0 {
		fmt.Println("┌─────────────────────────────────────────────────────────────┐")
		fmt.Println("│ GIT COMPARE                                                 │")
		fmt.Println("└─────────────────────────────────────────────────────────────┘")
		for _, c := range r.GitCompare {
			status := SymOK
			if c.Status == "left-only" || c.Status == "right-only" {
				status = "⚠ " + c.Status
			}
			fmt.Printf("  %s %s", status, c.App)
			if c.Variant != "" {
				fmt.Printf(" (variant=%s)", c.Variant)
			}
			if c.LeftPath != "" {
				fmt.Printf(" [left: %s]", c.LeftPath)
			}
			if c.RightPath != "" {
				fmt.Printf(" [right: %s]", c.RightPath)
			}
			fmt.Println()
		}
		fmt.Println()
	}
}

func convertToWorkloadJSON(workloads []WorkloadInfo) []WorkloadJSON {
	result := make([]WorkloadJSON, 0, len(workloads))
	for _, w := range workloads {
		result = append(result, WorkloadJSON{
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
	return result
}

func convertToSuggestionJSON(s *AppModelSuggestion) *SuggestionJSON {
	if s == nil {
		return nil
	}
	units := make([]UnitJSON, 0, len(s.Units))
	for _, u := range s.Units {
		workloads := make([]string, 0, len(u.Workloads))
		for _, w := range u.Workloads {
			workloads = append(workloads, fmt.Sprintf("%s/%s", w.Namespace, w.Name))
		}
		units = append(units, UnitJSON{
			Slug:      u.Slug,
			App:       u.App,
			Variant:   u.Variant,
			Workloads: workloads,
		})
	}
	return &SuggestionJSON{
		App:   s.App,
		Units: units,
	}
}

// applyProposal creates the App and Deployments in ConfigHub
func applyProposal(proposal *FullProposal, workloads []WorkloadInfo, dryRun bool) error {
	// Index workloads by namespace/name for manifest lookup
	workloadIndex := make(map[string]WorkloadInfo)
	for _, w := range workloads {
		key := fmt.Sprintf("%s/%s", w.Namespace, w.Name)
		workloadIndex[key] = w
	}

	fmt.Println("┌─────────────────────────────────────────────────────────────┐")
	fmt.Println("│ IMPORT TO CONFIGHUB                                         │")
	fmt.Println("└─────────────────────────────────────────────────────────────┘")

	if dryRun {
		fmt.Println("  (dry-run mode - no changes will be made)")
		fmt.Println()
	}

	// Step 1: Create App
	fmt.Printf("  Creating App: %s\n", proposal.App)
	if !dryRun {
		if err := createAppForImport(proposal.App); err != nil {
			return fmt.Errorf("create space: %w", err)
		}
		fmt.Printf("    ✓ Space created\n")
	}

	// Step 2: Create Deployments with workloads
	fmt.Println()
	fmt.Println("  Creating Deployments:")

	created := 0
	skipped := 0

	for _, unit := range proposal.Units {
		// Skip git-only units (no workloads to import)
		if len(unit.Workloads) == 0 {
			fmt.Printf("    • %s (skipped - no workloads)\n", unit.Slug)
			skipped++
			continue
		}

		// Build labels string
		labels := []string{}
		for k, v := range unit.Labels {
			labels = append(labels, fmt.Sprintf("%s=%s", k, v))
		}
		labelStr := strings.Join(labels, ",")

		fmt.Printf("    • %s [%s]\n", unit.Slug, labelStr)

		if !dryRun {
			// Get the first workload's manifest
			if len(unit.Workloads) > 0 {
				w, ok := workloadIndex[unit.Workloads[0]]
				if !ok {
					fmt.Printf("      ⚠ workload not found: %s\n", unit.Workloads[0])
					skipped++
					continue
				}

				// Fetch manifest from cluster
				manifest, err := fetchWorkloadManifest(w.Kind, w.Namespace, w.Name)
				if err != nil {
					fmt.Printf("      ⚠ failed to fetch manifest: %v\n", err)
					skipped++
					continue
				}

				// Create unit in ConfigHub
				if err := createUnitWithManifest(proposal.App, unit.Slug, labels, manifest); err != nil {
					fmt.Printf("      ⚠ failed to create: %v\n", err)
					skipped++
					continue
				}
				fmt.Printf("      ✓ created\n")
				created++
			}
		} else {
			created++
		}
	}

	fmt.Println()
	fmt.Printf("  Summary: %d deployments created, %d skipped\n", created, skipped)

	return nil
}

// createAppForImport creates an App for import using cub-scout app-space create
func createAppForImport(name string) error {
	result, err := CreateAppWithResult(name, true, nil)
	if err != nil {
		return err
	}
	if !result.Created {
		// Space already exists, that's OK
		return nil
	}
	return nil
}

// fetchWorkloadManifest gets the YAML manifest for a workload from the cluster
func fetchWorkloadManifest(kind, namespace, name string) ([]byte, error) {
	cmd := exec.Command("kubectl", "get", strings.ToLower(kind), name, "-n", namespace, "-o", "yaml")
	return cmd.Output()
}

// createUnitWithManifest creates a unit in ConfigHub using cub CLI with manifest
func createUnitWithManifest(space, slug string, labels []string, manifest []byte) error {
	args := []string{"unit", "create", "--space", space}

	// Add labels
	for _, l := range labels {
		args = append(args, "--label", l)
	}

	// Unit name and stdin for manifest
	args = append(args, slug, "-")

	cmd := exec.Command("cub", args...)
	cmd.Stdin = bytes.NewReader(manifest)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", string(output), err)
	}
	return nil
}
