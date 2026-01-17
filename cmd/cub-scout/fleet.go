// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/spf13/cobra"
)

// FleetResult aggregates imports from multiple clusters
type FleetResult struct {
	Clusters []ClusterImport `json:"clusters"`
	Summary  FleetSummary    `json:"summary"`
	Proposal *FullProposal   `json:"proposal,omitempty"` // Unified proposal for fleet
}

// ClusterImport represents one cluster's import data
type ClusterImport struct {
	Name   string        `json:"name"`   // Cluster name/context
	Import *ImportResult `json:"import"` // The import data
}

// FleetSummary shows aggregate stats
type FleetSummary struct {
	TotalClusters  int              `json:"totalClusters"`
	TotalWorkloads int              `json:"totalWorkloads"`
	ByOwner        map[string]int   `json:"byOwner"`     // Owner -> count
	ByApp          map[string][]string `json:"byApp"`       // App -> clusters where deployed
}

var fleetCmd = &cobra.Command{
	Use:   "import-cluster-aggregator",
	Short: "Aggregate imports from multiple clusters (GUI)",
	Long: `Aggregate import data from multiple clusters into a fleet view.

This is the GUI/multi-cluster companion to "cub-scout import".

Workflow:
  TUI (1 cluster):  cub-scout import
  GUI (N clusters): cub-scout import --json → import-cluster-aggregator → apply

Input can be:
- Multiple JSON files from "cub-scout import --json"
- Stdin (piped from multiple imports)

Examples:
  # Full workflow: scan clusters, generate unified proposal, apply
  for ctx in cluster-a cluster-b; do
    kubectl config use-context $ctx
    cub-scout import --json > ${ctx}.json
  done
  cub-scout import-cluster-aggregator cluster-*.json --suggest --json | cub-scout apply -

  # Generate unified proposal
  cub-scout import-cluster-aggregator cluster1.json cluster2.json --suggest

  # Just aggregate (no proposal)
  cub-scout import-cluster-aggregator cluster1.json cluster2.json cluster3.json
`,
	RunE: runFleet,
}

var (
	fleetJSON    bool
	fleetSuggest bool
)

func init() {
	fleetCmd.Flags().BoolVar(&fleetJSON, "json", false, "Output as JSON")
	fleetCmd.Flags().BoolVar(&fleetSuggest, "suggest", false, "Generate unified Hub/App Space proposal")
	rootCmd.AddCommand(fleetCmd)
}

// parseFleetInput handles both raw ImportResult and CombinedResult formats
func parseFleetInput(data []byte, name string) (*ImportResult, error) {
	// Try CombinedResult format first (has "cluster" field)
	var combined struct {
		Cluster *ImportResult `json:"cluster"`
	}
	if err := json.Unmarshal(data, &combined); err == nil && combined.Cluster != nil {
		return combined.Cluster, nil
	}

	// Try raw ImportResult format
	var imp ImportResult
	if err := json.Unmarshal(data, &imp); err != nil {
		return nil, err
	}

	// Check if it looks valid (has namespace or workloads)
	if imp.Namespace != "" || len(imp.Workloads) > 0 {
		return &imp, nil
	}

	return nil, fmt.Errorf("unrecognized format: expected ImportResult or CombinedResult")
}

func runFleet(cmd *cobra.Command, args []string) error {
	result := &FleetResult{
		Summary: FleetSummary{
			ByOwner: make(map[string]int),
			ByApp:   make(map[string][]string),
		},
	}

	// Read from files or stdin
	if len(args) > 0 {
		for _, file := range args {
			data, err := os.ReadFile(file)
			if err != nil {
				return fmt.Errorf("read %s: %w", file, err)
			}

			imp, err := parseFleetInput(data, file)
			if err != nil {
				return fmt.Errorf("parse %s: %w", file, err)
			}

			result.Clusters = append(result.Clusters, ClusterImport{
				Name:   file,
				Import: imp,
			})
		}
	} else {
		// Read from stdin (expect newline-delimited JSON)
		decoder := json.NewDecoder(os.Stdin)
		i := 0
		for {
			var raw json.RawMessage
			if err := decoder.Decode(&raw); err != nil {
				if err == io.EOF {
					break
				}
				return fmt.Errorf("parse stdin: %w", err)
			}
			i++

			imp, err := parseFleetInput(raw, fmt.Sprintf("cluster-%d", i))
			if err != nil {
				return fmt.Errorf("parse stdin entry %d: %w", i, err)
			}

			result.Clusters = append(result.Clusters, ClusterImport{
				Name:   fmt.Sprintf("cluster-%d", i),
				Import: imp,
			})
		}
	}

	// Build summary
	result.Summary.TotalClusters = len(result.Clusters)
	for _, c := range result.Clusters {
		if c.Import == nil {
			continue
		}
		result.Summary.TotalWorkloads += len(c.Import.Workloads)

		for _, w := range c.Import.Workloads {
			result.Summary.ByOwner[w.Owner]++

			// Track app by cluster
			app := w.Labels["app.kubernetes.io/name"]
			if app == "" {
				app = w.Labels["app"]
			}
			if app == "" {
				app = w.Name
			}

			// Check if cluster already in list
			found := false
			for _, existing := range result.Summary.ByApp[app] {
				if existing == c.Name {
					found = true
					break
				}
			}
			if !found {
				result.Summary.ByApp[app] = append(result.Summary.ByApp[app], c.Name)
			}
		}
	}

	// Generate unified proposal if --suggest
	if fleetSuggest {
		result.Proposal = buildFleetProposal(result)
	}

	if fleetJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	// Pretty print
	if fleetSuggest && result.Proposal != nil {
		result.Proposal.Print()
	} else {
		printFleetResult(result)
	}
	return nil
}

// buildFleetProposal creates a unified proposal from fleet data
func buildFleetProposal(fleet *FleetResult) *FullProposal {
	proposal := &FullProposal{
		AppSpace: "fleet-team",
		Units:    []UnitProposal{},
	}

	// Track deployers across clusters
	deployers := make(map[string]int)

	// Track variants for reconciliation rules
	variantsFound := make(map[string]bool)

	// Merge all cluster proposals into unified view
	// Group by app across clusters
	appUnits := make(map[string]map[string]*UnitProposal) // app -> variant -> unit

	for _, cluster := range fleet.Clusters {
		if cluster.Import == nil {
			continue
		}

		for _, w := range cluster.Import.Workloads {
			if w.Owner != "Native" && w.Owner != "" {
				deployers[w.Owner]++
			}

			app := w.Labels["app.kubernetes.io/name"]
			if app == "" {
				app = w.Labels["app"]
			}
			if app == "" {
				app = w.Name
			}

			variant := "default"
			if v, ok := w.Labels["environment"]; ok {
				variant = normalizeVariant(v)
			} else if v, ok := w.Labels["env"]; ok {
				variant = normalizeVariant(v)
			}
			variantsFound[variant] = true

			// Get or create unit for this app+variant
			if appUnits[app] == nil {
				appUnits[app] = make(map[string]*UnitProposal)
			}

			unit, exists := appUnits[app][variant]
			if !exists {
				slug := app
				if variant != "default" {
					slug = fmt.Sprintf("%s-%s", app, variant)
				}
				unit = &UnitProposal{
					Slug:    sanitizeSlug(slug),
					App:     app,
					Variant: variant,
					Status:  "aligned",
					Labels:  map[string]string{"app": app, "variant": variant},
				}
				appUnits[app][variant] = unit
			}

			// Add workload with cluster context
			workloadRef := fmt.Sprintf("%s:%s/%s", cluster.Name, w.Namespace, w.Name)
			unit.Workloads = append(unit.Workloads, workloadRef)

			// Extract tier/team from first workload
			if unit.Tier == "" {
				if tier, ok := w.Labels["app.kubernetes.io/component"]; ok {
					unit.Tier = tier
					unit.Labels["tier"] = tier
				}
			}
			if _, ok := unit.Labels["team"]; !ok {
				if team, ok := w.Labels["app.kubernetes.io/part-of"]; ok {
					unit.Labels["team"] = team
				}
			}
		}
	}

	// Flatten to units list
	for _, variants := range appUnits {
		for _, unit := range variants {
			proposal.Units = append(proposal.Units, *unit)
		}
	}

	// Set dominant deployer
	maxCount := 0
	for d, count := range deployers {
		if count > maxCount {
			proposal.Deployer = d
			maxCount = count
		}
	}

	// Generate reconciliation rules
	proposal.Reconciliation = suggestReconciliationRules(variantsFound)

	return proposal
}

func printFleetResult(r *FleetResult) {
	fmt.Println("┌─────────────────────────────────────────────────────────────┐")
	fmt.Println("│ FLEET SUMMARY                                               │")
	fmt.Println("└─────────────────────────────────────────────────────────────┘")
	fmt.Printf("  Clusters: %d\n", r.Summary.TotalClusters)
	fmt.Printf("  Workloads: %d\n", r.Summary.TotalWorkloads)
	fmt.Println()

	// Ownership breakdown
	fmt.Println("  Ownership:")
	owners := make([]string, 0, len(r.Summary.ByOwner))
	for o := range r.Summary.ByOwner {
		owners = append(owners, o)
	}
	sort.Strings(owners)
	for _, o := range owners {
		fmt.Printf("    %s: %d\n", o, r.Summary.ByOwner[o])
	}
	fmt.Println()

	// Apps across clusters
	fmt.Println("  Apps across clusters:")
	apps := make([]string, 0, len(r.Summary.ByApp))
	for a := range r.Summary.ByApp {
		apps = append(apps, a)
	}
	sort.Strings(apps)
	for _, a := range apps {
		clusters := r.Summary.ByApp[a]
		if len(clusters) > 1 {
			fmt.Printf("    • %s (%d clusters)\n", a, len(clusters))
		} else {
			fmt.Printf("    • %s\n", a)
		}
	}
	fmt.Println()

	// Per-cluster details
	for _, c := range r.Clusters {
		fmt.Printf("┌─ %s ", c.Name)
		fmt.Println("─────────────────────────────────────────────────────┐")
		if c.Import != nil {
			fmt.Printf("  Namespace: %s\n", c.Import.Namespace)
			fmt.Printf("  Workloads: %d\n", len(c.Import.Workloads))
			for _, w := range c.Import.Workloads {
				fmt.Printf("    • %s/%s [%s]\n", w.Namespace, w.Name, w.Owner)
			}
		}
		fmt.Println()
	}
}
