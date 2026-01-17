// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/confighub/cub-scout/pkg/gitops"
	"github.com/spf13/cobra"
)

var (
	combinedGitURL    string
	combinedGitPath   string
	combinedNamespace string
	combinedJSON      bool
	combinedSuggest   bool
	combinedApply     bool
	combinedDryRun    bool
)

// CombinedResult shows Git repo structure + Cluster workloads together
type CombinedResult struct {
	GitRepo   *gitops.RepoStructure `json:"gitRepo,omitempty"`
	Cluster   *ImportResult         `json:"cluster,omitempty"`
	Alignment []AlignmentEntry      `json:"alignment,omitempty"`
	Proposal  *FullProposal         `json:"proposal,omitempty"` // Hub/App Space proposal
}

// AlignmentEntry shows how Git apps align with cluster workloads
type AlignmentEntry struct {
	App        string   `json:"app"`
	GitVariant string   `json:"gitVariant,omitempty"` // From parser
	LivePath   string   `json:"livePath,omitempty"`   // From cluster deployer
	Status     string   `json:"status"`               // "aligned", "git-only", "cluster-only"
	Workloads  []string `json:"workloads,omitempty"`
}

var combinedCmd = &cobra.Command{
	Use:   "combined",
	Short: "Show Git repo structure + Cluster workloads aligned",
	Long: `Parse a Git repo and scan a cluster, showing alignment between them.

This helps you understand:
- What apps are defined in Git
- What workloads are deployed in the cluster
- How they align (or don't)

Use --suggest to generate a full Hub/App Space model proposal.
Use --apply to create the App Space and Units in ConfigHub.

Examples:
  # Combine Git repo with current cluster
  cub-agent combined --git-url https://github.com/org/gitops-repo --namespace demo

  # Generate Hub/App Space proposal
  cub-agent combined --git-url https://github.com/org/gitops-repo --namespace demo --suggest

  # Preview what would be created (dry-run)
  cub-agent combined --namespace demo --suggest --apply --dry-run

  # Apply: create App Space and Units in ConfigHub
  cub-agent combined --namespace demo --suggest --apply

  # Use local Git repo with JSON output
  cub-agent combined --git-path ./my-repo --namespace demo --suggest --json
`,
	RunE: runCombined,
}

func init() {
	combinedCmd.Flags().StringVar(&combinedGitURL, "git-url", "", "Git repository URL to parse")
	combinedCmd.Flags().StringVar(&combinedGitPath, "git-path", "", "Local path to Git repository")
	combinedCmd.Flags().StringVarP(&combinedNamespace, "namespace", "n", "", "Namespace to scan in cluster")
	combinedCmd.Flags().BoolVar(&combinedJSON, "json", false, "Output as JSON")
	combinedCmd.Flags().BoolVar(&combinedSuggest, "suggest", false, "Generate Hub/App Space model proposal")
	combinedCmd.Flags().BoolVar(&combinedApply, "apply", false, "Create App Space and Units in ConfigHub")
	combinedCmd.Flags().BoolVar(&combinedDryRun, "dry-run", false, "Show what would be created without making changes")

	rootCmd.AddCommand(combinedCmd)
}

func runCombined(cmd *cobra.Command, args []string) error {
	result := &CombinedResult{}

	// Parse Git repo if provided
	if combinedGitURL != "" || combinedGitPath != "" {
		var repoPath string
		var cleanup func()

		if combinedGitURL != "" {
			tmpDir, err := os.MkdirTemp("", "gitops-combined-*")
			if err != nil {
				return fmt.Errorf("create temp dir: %w", err)
			}
			cleanup = func() { os.RemoveAll(tmpDir) }

			if !combinedJSON {
				fmt.Fprintf(os.Stderr, "Cloning %s...\n", combinedGitURL)
			}
			gitCmd := exec.Command("git", "clone", "--depth=1", combinedGitURL, tmpDir)
			if output, err := gitCmd.CombinedOutput(); err != nil {
				cleanup()
				return fmt.Errorf("clone failed: %w\n%s", err, output)
			}
			repoPath = tmpDir
		} else {
			repoPath = combinedGitPath
		}

		if cleanup != nil {
			defer cleanup()
		}

		repo, err := gitops.ParseRepo(repoPath)
		if err != nil {
			return fmt.Errorf("parse repo: %w", err)
		}
		result.GitRepo = repo
	}

	// Scan cluster if namespace provided
	var workloads []WorkloadInfo
	if combinedNamespace != "" {
		var err error
		workloads, err = discoverWorkloads(combinedNamespace)
		if err != nil {
			return fmt.Errorf("discover workloads: %w", err)
		}

		suggestion := SuggestHubAppSpaceStructure(workloads, "")
		suggestionJSON := convertToSuggestionJSON(&suggestion)

		result.Cluster = &ImportResult{
			Namespace:  combinedNamespace,
			Model:      "hub-appspace",
			Workloads:  convertToWorkloadJSON(workloads),
			Suggestion: suggestionJSON,
		}
	}

	// Build alignment if we have both
	if result.GitRepo != nil && result.Cluster != nil {
		result.Alignment = buildAlignment(result.GitRepo, result.Cluster)
	}

	// Build full Hub/App Space proposal if --suggest
	if combinedSuggest && result.GitRepo != nil {
		result.Proposal = SuggestFullProposal(result.GitRepo.Apps, workloads, "")
	}

	// Build proposal from cluster-only if no Git repo
	if combinedSuggest && result.GitRepo == nil && len(workloads) > 0 {
		result.Proposal = SuggestFullProposal(nil, workloads, "")
	}

	// Apply: create App Space and Units in ConfigHub
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
			status := "✓"
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

func convertToSuggestionJSON(s *HubAppSpaceSuggestion) *SuggestionJSON {
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
		AppSpace: s.AppSpace,
		Units:    units,
	}
}

// applyProposal creates the App Space and Units in ConfigHub
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

	// Step 1: Create App Space
	fmt.Printf("  Creating App Space: %s\n", proposal.AppSpace)
	if !dryRun {
		if err := createAppSpaceForImport(proposal.AppSpace); err != nil {
			return fmt.Errorf("create space: %w", err)
		}
		fmt.Printf("    ✓ Space created\n")
	}

	// Step 2: Create Units with workloads
	fmt.Println()
	fmt.Println("  Creating Units:")

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
				if err := createUnitWithManifest(proposal.AppSpace, unit.Slug, labels, manifest); err != nil {
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
	fmt.Printf("  Summary: %d units created, %d skipped\n", created, skipped)

	return nil
}

// createAppSpaceForImport creates an App Space for import using cub-agent app-space create
func createAppSpaceForImport(name string) error {
	result, err := CreateAppSpaceWithResult(name, true, nil)
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
