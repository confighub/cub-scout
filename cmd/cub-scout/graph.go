package main

import (
	"context"
	"fmt"
	"os"
	"strings"
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
  export    Export the resource graph as JSON, DOT, SVG, or HTML`,
}

var graphExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export resource graph",
	Long: `Export the resource graph in deterministic formats.

The output includes:
  - schema_version: Graph schema version (graph.v1)
  - generated_at: Timestamp of generation
  - cluster: Cluster name
  - nodes: Resources in the graph
  - edges: Relationships with evidence

Supported formats:
  - json (default): machine-readable graph payload
  - dot: Graphviz DOT
  - svg: embeddable static visual
  - html: self-contained interactive visual`,
	RunE: runGraphExport,
}

var (
	graphExportJSON      bool
	graphExportFormat    string
	graphExportOutput    string
	graphExportMaxNodes  int
	graphExportNamespace string
	graphExportEmpty     bool
)

func init() {
	rootCmd.AddCommand(graphCmd)
	graphCmd.AddCommand(graphExportCmd)

	graphExportCmd.Flags().BoolVar(&graphExportJSON, "json", false, "DEPRECATED: use --format json")
	_ = graphExportCmd.Flags().MarkDeprecated("json", "use --format json")
	graphExportCmd.Flags().StringVar(&graphExportFormat, "format", "json", "Output format: json|dot|svg|html")
	graphExportCmd.Flags().StringVarP(&graphExportOutput, "output", "o", "", "Write output to file (default: stdout)")
	graphExportCmd.Flags().IntVar(&graphExportMaxNodes, "max-nodes", 300, "Maximum nodes in visual formats (0 = unlimited)")
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

	if graphExportMaxNodes < 0 {
		return fmt.Errorf("--max-nodes must be >= 0")
	}

	format := strings.ToLower(strings.TrimSpace(graphExportFormat))
	if graphExportJSON {
		format = "json"
	}

	data, err := renderGraphOutput(g, format, graphExportMaxNodes)
	if err != nil {
		return fmt.Errorf("failed to export graph: %w", err)
	}

	if outputPath := strings.TrimSpace(graphExportOutput); outputPath != "" {
		if err := os.WriteFile(outputPath, data, 0644); err != nil {
			return fmt.Errorf("write output file: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Wrote graph export (%s) to %s\n", format, outputPath)
		return nil
	}

	fmt.Println(string(data))
	return nil
}
