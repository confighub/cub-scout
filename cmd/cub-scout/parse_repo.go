// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/monadic/confighub-agent/pkg/gitops"
	"github.com/spf13/cobra"
)

var (
	parseRepoURL  string
	parseRepoPath string
	parseRepoJSON bool
)

var parseRepoCmd = &cobra.Command{
	Use:   "parse-repo",
	Short: "Parse a GitOps repository structure",
	Long: `Parse a GitOps repository and show its structure.

Supports multiple architecture patterns:
  - Single-repo (flux2-kustomize-helm-example style)
  - D2 Fleet (clusters + tenants)
  - D2 Infra (cluster add-ons)
  - D2 Apps (namespace-scoped apps)

Examples:
  # Parse a remote repo
  cub-agent parse-repo --url https://github.com/fluxcd/flux2-kustomize-helm-example

  # Parse a local directory
  cub-agent parse-repo --path ./my-gitops-repo

  # JSON output
  cub-agent parse-repo --url https://github.com/org/repo --json
`,
	RunE: runParseRepo,
}

func init() {
	parseRepoCmd.Flags().StringVar(&parseRepoURL, "url", "", "Git repository URL to clone and parse")
	parseRepoCmd.Flags().StringVar(&parseRepoPath, "path", "", "Local path to parse")
	parseRepoCmd.Flags().BoolVar(&parseRepoJSON, "json", false, "Output as JSON")

	rootCmd.AddCommand(parseRepoCmd)
}

func runParseRepo(cmd *cobra.Command, args []string) error {
	var repoPath string
	var cleanup func()

	if parseRepoURL != "" {
		// Clone to temp directory
		tmpDir, err := os.MkdirTemp("", "gitops-parse-*")
		if err != nil {
			return fmt.Errorf("create temp dir: %w", err)
		}
		cleanup = func() { os.RemoveAll(tmpDir) }

		fmt.Fprintf(os.Stderr, "Cloning %s...\n", parseRepoURL)
		gitCmd := exec.Command("git", "clone", "--depth=1", parseRepoURL, tmpDir)
		if output, err := gitCmd.CombinedOutput(); err != nil {
			cleanup()
			return fmt.Errorf("clone failed: %w\n%s", err, output)
		}
		repoPath = tmpDir
	} else if parseRepoPath != "" {
		repoPath = parseRepoPath
	} else {
		return fmt.Errorf("either --url or --path is required")
	}

	if cleanup != nil {
		defer cleanup()
	}

	// Parse the repo
	result, err := gitops.ParseRepo(repoPath)
	if err != nil {
		return fmt.Errorf("parse repo: %w", err)
	}

	if parseRepoJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	// Pretty print
	printRepoStructure(result)
	return nil
}

func printRepoStructure(r *gitops.RepoStructure) {
	fmt.Printf("Repository Type: %s\n", r.Type)
	fmt.Println(strings.Repeat("─", 60))

	if len(r.Apps) > 0 {
		fmt.Println("\n📦 APPS")
		for _, app := range r.Apps {
			fmt.Printf("  %s\n", app.Name)
			if app.BasePath != "" {
				fmt.Printf("    Base: %s\n", app.BasePath)
			}
			if len(app.Variants) > 0 {
				fmt.Printf("    Variants:\n")
				for _, v := range app.Variants {
					fmt.Printf("      - %s (%s)\n", v.Name, v.Path)
				}
			}
		}
	}

	if len(r.Components) > 0 {
		fmt.Println("\n🔧 COMPONENTS")
		for _, comp := range r.Components {
			fmt.Printf("  %s [%s]\n", comp.Name, comp.Type)
			fmt.Printf("    Path: %s\n", comp.Path)
			if len(comp.Variants) > 0 {
				fmt.Printf("    Variants: %s\n", strings.Join(comp.Variants, ", "))
			}
		}
	}

	if len(r.Infrastructure) > 0 {
		fmt.Println("\n🏗️  INFRASTRUCTURE")
		for _, infra := range r.Infrastructure {
			fmt.Printf("  %s (%s)\n", infra.Name, infra.Path)
		}
	}

	if len(r.Clusters) > 0 {
		fmt.Println("\n☸️  CLUSTERS")
		for _, cluster := range r.Clusters {
			fmt.Printf("  %s (%s)\n", cluster.Name, cluster.Path)
			if len(cluster.Apps) > 0 {
				fmt.Printf("    Includes: %s\n", strings.Join(cluster.Apps, ", "))
			}
		}
	}

	if len(r.Tenants) > 0 {
		fmt.Println("\n👥 TENANTS")
		for _, tenant := range r.Tenants {
			fmt.Printf("  %s (%s)\n", tenant.Name, tenant.Path)
		}
	}

	// App-of-apps pattern
	if r.RootApp != nil {
		fmt.Println("\n🎯 ROOT APP")
		fmt.Printf("  %s\n", r.RootApp.Name)
		fmt.Printf("    Path: %s\n", r.RootApp.Path)
		if r.RootApp.Destination != "" {
			fmt.Printf("    Destination: %s\n", r.RootApp.Destination)
		}
	}

	if len(r.ChildApps) > 0 {
		fmt.Println("\n📦 CHILD APPS")
		for _, app := range r.ChildApps {
			fmt.Printf("  %s\n", app.Name)
			fmt.Printf("    Path: %s\n", app.Path)
			if app.Destination != "" {
				fmt.Printf("    Namespace: %s\n", app.Destination)
			}
			if app.Source == "helm" {
				fmt.Printf("    Type: Helm chart\n")
			}
		}
	}

	if len(r.ApplicationSets) > 0 {
		fmt.Println("\n⚡ APPLICATION SETS")
		for _, appset := range r.ApplicationSets {
			fmt.Printf("  %s [%s generator]\n", appset.Name, appset.Generator)
			fmt.Printf("    Path: %s\n", appset.Path)
		}
	}

	if r.HelmChart != nil {
		fmt.Println("\n⎈ HELM UMBRELLA")
		fmt.Printf("  %s", r.HelmChart.Name)
		if r.HelmChart.Version != "" {
			fmt.Printf(" v%s", r.HelmChart.Version)
		}
		fmt.Println()
		if len(r.HelmChart.Dependencies) > 0 {
			fmt.Printf("    Dependencies:\n")
			for _, dep := range r.HelmChart.Dependencies {
				fmt.Printf("      - %s", dep.Name)
				if dep.Version != "" {
					fmt.Printf(" (%s)", dep.Version)
				}
				fmt.Println()
			}
		}
	}

	fmt.Println()
}
