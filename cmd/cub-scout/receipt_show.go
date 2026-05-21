// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/spf13/cobra"
)

var receiptShowFormat string

var receiptShowCmd = &cobra.Command{
	Use:   "show <receipt-path>",
	Short: "Render an on-disk receipt artifact",
	Long: `show reads a receipt artifact from disk and renders it as ASCII
(default) or JSON. This is the read-side complement to receipt verify,
useful for inspecting receipts produced earlier in the pipeline, in CI
output, or attached to an audit trail.

show does NOT verify the fingerprint — that's receipt validate's job.
It's read-only by construction.

Examples:
  cub-scout receipt show ./api.receipt.json
  cub-scout receipt show ~/.local/share/cub-scout/receipts/2026-05-22T10-30-00Z__applied-matches-spec__Deployment-api__abc123def456.receipt.json --format json
`,
	Args: cobra.ExactArgs(1),
	RunE: runReceiptShow,
}

func init() {
	receiptCmd.AddCommand(receiptShowCmd)
	receiptShowCmd.Flags().StringVar(&receiptShowFormat, "format", "ascii", "Output format: ascii | json")
}

func runReceiptShow(cmd *cobra.Command, args []string) error {
	format := strings.ToLower(strings.TrimSpace(receiptShowFormat))
	if format != "ascii" && format != "json" {
		return fmt.Errorf("invalid --format %q (valid: ascii, json)", receiptShowFormat)
	}

	stmt, err := agent.LoadStatement(args[0])
	if err != nil {
		return err
	}

	switch format {
	case "json":
		buf, mErr := json.MarshalIndent(stmt, "", "  ")
		if mErr != nil {
			return fmt.Errorf("marshal receipt: %w", mErr)
		}
		fmt.Println(string(buf))
	case "ascii":
		fmt.Print(renderReceiptASCII(stmt))
	}
	return nil
}
