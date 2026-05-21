// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/spf13/cobra"
)

var (
	receiptListDir    string
	receiptListFormat string
)

var receiptListCmd = &cobra.Command{
	Use:   "list",
	Short: "List receipts stored locally",
	Long: `list walks the local receipt store and prints one line per
receipt sorted by VerifiedAt descending (newest first). The store is
resolved in this priority order:

  1. --dir <path> (explicit override)
  2. $CUB_SCOUT_RECEIPTS_DIR
  3. $XDG_DATA_HOME/cub-scout/receipts
  4. $HOME/.local/share/cub-scout/receipts

Receipts are written to the store when you pass --save to receipt
verify, when --out points inside the store directory, or by any other
process that drops *.receipt.json files there.

ASCII output (default):
  VERDICT  PREDICATE              SCOPE                          VERIFIED-AT          PATH
  PASS     applied-matches-spec   Deployment/api in prod         2026-05-22T10:30:00Z .../...
  BLOCK    no-manual-edits-since  Deployment/api in prod         2026-05-22T08:00:00Z .../...

JSON output is the structured ReceiptListEntry array — same content,
machine-readable.

Examples:
  cub-scout receipt list
  cub-scout receipt list --dir ./ci-receipts/
  cub-scout receipt list --format json | jq '.[] | select(.verdict == "BLOCK")'
`,
	Args: cobra.NoArgs,
	RunE: runReceiptList,
}

func init() {
	receiptCmd.AddCommand(receiptListCmd)
	receiptListCmd.Flags().StringVar(&receiptListDir, "dir", "", "Receipt store directory (default: $CUB_SCOUT_RECEIPTS_DIR or $XDG_DATA_HOME/cub-scout/receipts)")
	receiptListCmd.Flags().StringVar(&receiptListFormat, "format", "ascii", "Output format: ascii | json")
}

func runReceiptList(cmd *cobra.Command, args []string) error {
	format := strings.ToLower(strings.TrimSpace(receiptListFormat))
	if format != "ascii" && format != "json" {
		return fmt.Errorf("invalid --format %q (valid: ascii, json)", receiptListFormat)
	}

	dir := strings.TrimSpace(receiptListDir)
	if dir == "" {
		resolved, err := agent.DefaultStoreDir()
		if err != nil {
			return fmt.Errorf("resolve receipt store: %w", err)
		}
		dir = resolved
	}

	entries, listErr := agent.ListStatements(dir)
	// listErr may be non-nil with a partial result (one or more files
	// failed to parse). Surface as a warning rather than fatal.

	switch format {
	case "json":
		buf, mErr := json.MarshalIndent(entries, "", "  ")
		if mErr != nil {
			return fmt.Errorf("marshal list: %w", mErr)
		}
		fmt.Println(string(buf))
	case "ascii":
		renderReceiptListASCII(entries, dir)
	}

	if listErr != nil {
		// Print a warning to stderr but exit 0 — partial list is the
		// designed semantics.
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v\n", listErr)
	}
	return nil
}

// renderReceiptListASCII prints a fixed-width table of receipts. Style
// matches the existing cub-scout map/explain output (no color, simple
// header underline, columns padded for readability).
func renderReceiptListASCII(entries []agent.ReceiptListEntry, dir string) {
	fmt.Printf("Store: %s\n\n", dir)
	if len(entries) == 0 {
		fmt.Println("(no receipts found)")
		return
	}

	fmt.Printf("%-13s  %-22s  %-40s  %-22s  %s\n",
		"VERDICT", "PREDICATE", "SCOPE", "VERIFIED-AT", "PATH")
	fmt.Println(strings.Repeat("-", 120))
	for _, e := range entries {
		scope := fmt.Sprintf("%s/%s in %s", e.Scope.Kind, e.Scope.Name, e.Scope.Namespace)
		fmt.Printf("%-13s  %-22s  %-40s  %-22s  %s\n",
			e.Verdict,
			e.PredicateName,
			truncateReceiptScope(scope, 40),
			e.VerifiedAt,
			filepath.Base(e.Path))
	}
}

// truncateReceiptScope shortens s to at most n runes, appending an ellipsis byte
// when truncateReceiptScoped. Cheap; non-ASCII-safe but adequate for K8s identifiers
// (which are ASCII per RFC 1123).
func truncateReceiptScope(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n < 1 {
		return ""
	}
	return s[:n-1] + "…"
}
