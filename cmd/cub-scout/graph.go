package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/confighub/cub-scout/internal/graph"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
)

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Resource graph operations (v0.6)",
	Long: `Resource graph operations for exploring cluster relationships.

This is a v0.6 contract surface. It does not modify any v0.5 contracts.

Available subcommands:
  export    Export the resource graph as JSON`,
}

var graphExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export resource graph as JSON",
	Long: `Export the resource graph as deterministic JSON.

The output includes:
  - schema_version: Graph schema version (graph.v1)
  - generated_at: Timestamp of generation
  - cluster: Cluster name
  - nodes: Resources in the graph
  - edges: Relationships with evidence

Output is deterministic: same input produces identical output.`,
	RunE: runGraphExport,
}

var (
	graphExportJSON      bool
	graphExportNamespace string
	graphExportEmpty     bool
)

func init() {
	rootCmd.AddCommand(graphCmd)
	graphCmd.AddCommand(graphExportCmd)

	graphExportCmd.Flags().BoolVar(&graphExportJSON, "json", true, "Output as JSON (default, only format supported)")
	graphExportCmd.Flags().StringVarP(&graphExportNamespace, "namespace", "n", "", "Namespace to collect (empty = all namespaces)")
	graphExportCmd.Flags().BoolVar(&graphExportEmpty, "empty", false, "Output empty graph (skip cluster collection)")
}

func runGraphExport(cmd *cobra.Command, args []string) error {
	// Get cluster name (allow override for testing)
	cluster := os.Getenv("CUB_SCOUT_TEST_CLUSTER")
	if cluster == "" {
		cluster = getCurrentContext()
	}

	// Create graph
	g := graph.NewGraph(cluster)

	// Collect from cluster unless --empty is set or we're in test mode
	if !graphExportEmpty && os.Getenv("CUB_SCOUT_TEST_TIME") == "" {
		cfg, err := buildConfig()
		if err != nil {
			// If no cluster access, output empty graph
			// This allows the command to work without a cluster
		} else {
			client, err := kubernetes.NewForConfig(cfg)
			if err != nil {
				return fmt.Errorf("failed to create kubernetes client: %w", err)
			}

			collector := graph.NewCollector(client, cluster)
			ctx := context.Background()

			if err := collector.CollectOwnershipChain(ctx, g, graphExportNamespace); err != nil {
				return fmt.Errorf("failed to collect ownership chain: %w", err)
			}
		}
	}

	// Override GeneratedAt for deterministic testing if TEST_TIME is set
	if testTime := os.Getenv("CUB_SCOUT_TEST_TIME"); testTime != "" {
		if t, err := time.Parse(time.RFC3339, testTime); err == nil {
			g.GeneratedAt = t
		}
	}

	// Export
	data, err := g.Export()
	if err != nil {
		return fmt.Errorf("failed to export graph: %w", err)
	}

	fmt.Println(string(data))
	return nil
}
