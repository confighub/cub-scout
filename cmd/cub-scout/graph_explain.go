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

var graphExplainCmd = &cobra.Command{
	Use:   "explain <kind>/<name>",
	Short: "Explain a resource's graph relationships",
	Long: `Explain a resource's relationships in the graph.

Shows the resource's details and all incoming/outgoing edges with evidence.

Output is deterministic: same graph produces identical output.

Example:
  cub-scout graph explain Deployment/nginx -n default
  cub-scout graph explain Pod/nginx-abc123-xyz -n default`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			fmt.Fprintln(os.Stderr, "Error: requires exactly 1 argument: <kind>/<name>")
			os.Exit(2)
		}
		return nil
	},
	RunE:              runGraphExplain,
	SilenceUsage:      true, // Don't print usage on error
	SilenceErrors:     true, // We handle errors ourselves
	DisableFlagsInUseLine: true,
}

var (
	graphExplainNamespace string
)

func init() {
	graphCmd.AddCommand(graphExplainCmd)

	graphExplainCmd.Flags().StringVarP(&graphExplainNamespace, "namespace", "n", "", "Namespace of the resource (required)")
	// Don't use MarkFlagRequired - we handle it ourselves for deterministic error output
}

func runGraphExplain(cmd *cobra.Command, args []string) error {
	// Parse kind/name argument
	kindName := args[0]
	parts := strings.SplitN(kindName, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		fmt.Fprintln(os.Stderr, "Error: invalid argument format, expected <kind>/<name>")
		os.Exit(2)
	}
	kind := parts[0]
	name := parts[1]

	// Check namespace flag
	if graphExplainNamespace == "" {
		fmt.Fprintln(os.Stderr, "Error: --namespace is required")
		os.Exit(2)
	}

	// Get cluster name
	cluster := os.Getenv("CUB_SCOUT_TEST_CLUSTER")
	if cluster == "" {
		cluster = getCurrentContext()
	}

	// Build graph
	g := graph.NewGraph(cluster)

	// Collect from cluster unless we're in test mode
	if os.Getenv("CUB_SCOUT_TEST_TIME") == "" {
		cfg, err := buildConfig()
		if err != nil {
			// If no cluster access, proceed with empty graph
			// Target won't be found, which is correct
		} else {
			client, err := kubernetes.NewForConfig(cfg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: failed to create kubernetes client: %v\n", err)
				os.Exit(1)
			}

			collector := graph.NewCollector(client, cluster)
			ctx := context.Background()

			// Collect ownership chain (ignore errors for best-effort)
			_ = collector.CollectOwnershipChain(ctx, g, "")
		}
	}

	// Override GeneratedAt for deterministic testing
	if testTime := os.Getenv("CUB_SCOUT_TEST_TIME"); testTime != "" {
		if t, err := time.Parse(time.RFC3339, testTime); err == nil {
			g.GeneratedAt = t
		}
	}

	// Explain the target
	output, exitCode := graph.Explain(g, cluster, graphExplainNamespace, kind, name)
	fmt.Print(output)

	if exitCode != 0 {
		os.Exit(exitCode)
	}

	return nil
}
