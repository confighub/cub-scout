// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"github.com/spf13/cobra"
)

// Scout-style command aliases
// These provide more intuitive names that were originally planned but never implemented.
// They wrap existing functionality rather than duplicating code.

var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Discover resources in your cluster (alias for 'map workloads')",
	Long: `Discover all workloads in your cluster and who owns them.

This is a scout-style alias for 'cub-scout map workloads'.
Shows Deployments, StatefulSets, and DaemonSets grouped by owner
(Flux, ArgoCD, Helm, ConfigHub, or Native).

Examples:
  cub-scout discover              # List all workloads by owner
  cub-scout discover --json       # Output as JSON
  cub-scout discover -n prod      # Filter by namespace

See also:
  cub-scout map workloads   # Same functionality
  cub-scout tree ownership  # Similar, different format
`,
	RunE: runMapWorkloads, // Direct call to avoid cobra re-execution
}

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Check cluster health and issues (alias for 'map issues')",
	Long: `Check your cluster for stuck states, issues, and problems.

This is a scout-style alias for 'cub-scout map issues'.
Shows resources that are:
  - Not reconciling
  - Stuck in pending state
  - Have failed status
  - Missing expected resources

Examples:
  cub-scout health               # Show all issues
  cub-scout health --json        # Output as JSON

See also:
  cub-scout map issues    # Same functionality
  cub-scout scan          # Deeper configuration risk analysis
`,
	RunE: runMapProblems, // Direct call to avoid cobra re-execution
}

func init() {
	rootCmd.AddCommand(discoverCmd)
	rootCmd.AddCommand(healthCmd)
}
