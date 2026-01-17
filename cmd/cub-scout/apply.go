// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var applyCmd = &cobra.Command{
	Use:   "apply [proposal.json]",
	Short: "Apply a proposal from JSON (GUI)",
	Long: `Apply a Hub/App Space proposal to create resources in ConfigHub.

This is the GUI companion to "cub-agent import".

Workflow:
  TUI (interactive): cub-agent import
  GUI (from JSON):   cub-agent import --json | cub-agent apply -

The proposal can come from:
- A JSON file (from "cub-agent import --json")
- Stdin (for GUI integration)

Examples:
  # Single cluster: generate, edit, apply
  cub-agent import --json > proposal.json
  # (GUI displays, user edits)
  cub-agent apply proposal.json

  # Fleet: multiple clusters → unified proposal → apply
  cub-agent fleet cluster*.json --suggest --json | cub-agent apply -

  # Dry-run to preview
  cub-agent apply proposal.json --dry-run
`,
	Args: cobra.MaximumNArgs(1),
	RunE: runApply,
}

var (
	applyDryRun bool
	applyNoLog  bool
)

func init() {
	applyCmd.Flags().BoolVar(&applyDryRun, "dry-run", false, "Preview what would be created without making changes")
	applyCmd.Flags().BoolVar(&applyNoLog, "no-log", false, "Disable logging to file")
	rootCmd.AddCommand(applyCmd)
}

// ApplyInput represents the JSON input for apply
// Can be a FullProposal directly or wrapped in CombinedResult/FleetResult
type ApplyInput struct {
	// Direct proposal
	AppSpace       string               `json:"appSpace,omitempty"`
	Deployer       string               `json:"deployer,omitempty"`
	Reconciliation []ReconciliationRule `json:"reconciliation,omitempty"`
	Units          []UnitProposal       `json:"units,omitempty"`
	HubBases       []HubBaseProposal    `json:"hubBases,omitempty"`

	// Wrapped in combined/fleet result
	Proposal *FullProposal `json:"proposal,omitempty"`
}

func runApply(cmd *cobra.Command, args []string) error {
	// Initialize logger (unless disabled)
	var logger *ImportLogger
	if !applyNoLog {
		var err error
		logger, err = NewImportLogger("apply")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not create log file: %v\n", err)
		}
	}
	defer func() {
		if logger != nil {
			logPath := logger.Close()
			if logPath != "" {
				fmt.Printf("\nLog: %s\n", logPath)
			}
		}
	}()

	var data []byte
	var err error
	var source string

	// Read from file or stdin
	if len(args) == 0 || args[0] == "-" {
		source = "stdin"
		data, err = io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
	} else {
		source = args[0]
		data, err = os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
	}

	if logger != nil {
		logger.Log("Starting apply")
		logger.Log("Source: %s", source)
		if applyDryRun {
			logger.Log("Mode: dry-run")
		}
	}

	// Parse input - try to extract proposal
	var input ApplyInput
	if err := json.Unmarshal(data, &input); err != nil {
		return fmt.Errorf("parse JSON: %w", err)
	}

	// Get the proposal (either direct or wrapped)
	var proposal *FullProposal
	if input.Proposal != nil {
		proposal = input.Proposal
	} else if input.AppSpace != "" {
		// Direct proposal format
		proposal = &FullProposal{
			AppSpace:       input.AppSpace,
			Deployer:       input.Deployer,
			Reconciliation: input.Reconciliation,
			Units:          input.Units,
			HubBases:       input.HubBases,
		}
	} else {
		return fmt.Errorf("no proposal found in input (expected 'proposal' or 'appSpace' field)")
	}

	if logger != nil {
		logger.LogProposal(proposal)
	}

	// Apply the proposal
	return applyProposalFromJSONWithLogger(proposal, applyDryRun, logger)
}

// applyProposalFromJSONWithLogger applies a proposal read from JSON with logging
func applyProposalFromJSONWithLogger(proposal *FullProposal, dryRun bool, logger *ImportLogger) error {
	fmt.Println("┌─────────────────────────────────────────────────────────────┐")
	fmt.Println("│ APPLY PROPOSAL TO CONFIGHUB                                 │")
	fmt.Println("└─────────────────────────────────────────────────────────────┘")

	if dryRun {
		fmt.Println("  (dry-run mode - no changes will be made)")
		fmt.Println()
	}

	if logger != nil {
		logger.Section("APPLYING")
	}

	// Step 1: Create App Space
	fmt.Printf("  Creating App Space: %s\n", proposal.AppSpace)
	if logger != nil {
		logger.Log("Creating App Space: %s", proposal.AppSpace)
	}
	if !dryRun {
		if err := createAppSpaceForImport(proposal.AppSpace); err != nil {
			if logger != nil {
				logger.Log("FAILED: create space: %v", err)
				logger.LogResult(0, 1, err)
			}
			return fmt.Errorf("create space: %w", err)
		}
		fmt.Printf("    ✓ Space created\n")
		if logger != nil {
			logger.Log("  OK: Space created")
		}
	}

	// Step 2: Create Units
	fmt.Println()
	fmt.Println("  Creating Units:")

	created := 0
	skipped := 0

	for _, unit := range proposal.Units {
		// Build labels
		labels := []string{}
		for k, v := range unit.Labels {
			labels = append(labels, fmt.Sprintf("%s=%s", k, v))
		}
		labelStr := strings.Join(labels, ",")

		// Check if unit has workloads (for fleet, workloads include cluster prefix)
		hasClusterWorkloads := len(unit.Workloads) > 0 && strings.Contains(unit.Workloads[0], ":")

		if len(unit.Workloads) == 0 {
			fmt.Printf("    • %s (skipped - no workloads)\n", unit.Slug)
			if logger != nil {
				logger.Log("Skipped unit %s: no workloads", unit.Slug)
			}
			skipped++
			continue
		}

		fmt.Printf("    • %s [%s]\n", unit.Slug, labelStr)
		if logger != nil {
			logger.Log("Creating unit: %s [%s]", unit.Slug, labelStr)
		}

		if !dryRun {
			var manifest []byte
			var err error

			if hasClusterWorkloads {
				// Fleet mode: workload ref is "cluster:namespace/name"
				// We need to fetch from the right cluster context
				// For now, try to fetch from current context
				ref := unit.Workloads[0]
				parts := strings.SplitN(ref, ":", 2)
				if len(parts) == 2 {
					nsParts := strings.SplitN(parts[1], "/", 2)
					if len(nsParts) == 2 {
						// Try to determine kind from the unit or default to Deployment
						manifest, err = fetchWorkloadManifest("Deployment", nsParts[0], nsParts[1])
					}
				}
			} else {
				// Single cluster mode: workload ref is "namespace/name"
				parts := strings.SplitN(unit.Workloads[0], "/", 2)
				if len(parts) == 2 {
					manifest, err = fetchWorkloadManifest("Deployment", parts[0], parts[1])
				}
			}

			if err != nil {
				fmt.Printf("      ⚠ failed to fetch manifest: %v\n", err)
				if logger != nil {
					logger.Log("  WARN: fetch manifest failed: %v", err)
				}
				// Create unit with empty manifest as placeholder
				manifest = []byte(fmt.Sprintf("# Placeholder for %s\n# Workloads: %v\n", unit.Slug, unit.Workloads))
			}

			if err := createUnitWithManifest(proposal.AppSpace, unit.Slug, labels, manifest); err != nil {
				fmt.Printf("      ⚠ failed to create: %v\n", err)
				if logger != nil {
					logger.Log("  FAILED: create unit: %v", err)
				}
				skipped++
				continue
			}
			fmt.Printf("      ✓ created\n")
			if logger != nil {
				logger.Log("  OK: created")
			}
			created++
		} else {
			created++
		}
	}

	fmt.Println()
	fmt.Printf("  Summary: %d units created, %d skipped\n", created, skipped)

	if logger != nil {
		logger.LogResult(created, skipped, nil)
	}

	if !dryRun {
		fmt.Println("\n✓ Apply complete")
	}

	return nil
}

// createUnitWithManifest is defined in combined.go
// fetchWorkloadManifest is defined in combined.go
// createAppSpaceForImport is defined in combined.go
