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

var receiptValidateFormat string

var receiptValidateCmd = &cobra.Command{
	Use:   "validate <receipt-path>",
	Short: "Verify the fingerprint integrity of an on-disk receipt artifact",
	Long: `validate reads a receipt artifact from disk and recomputes its
fingerprint, comparing against the stamped value. Any tampering with
_type, subject, predicateType, or any predicate field other than
fingerprint produces a non-zero exit and an explanatory message.

validate is read-only and never touches the cluster. It catches:
  - receipt body modification (e.g., verdict flip, scope rename, subject digest swap)
  - envelope tampering (e.g., predicateType change)
  - corrupted on-disk JSON

Exit code:
  0   fingerprint matches
  1   fingerprint mismatch (tampering or corruption)
  2   I/O or parse error

Examples:
  cub-scout receipt validate ./api.receipt.json
  cub-scout receipt validate ~/.local/share/cub-scout/receipts/2026-05-22T10-30-00Z__applied-matches-spec__Deployment-api__abc123def456.receipt.json
`,
	Args: cobra.ExactArgs(1),
	RunE: runReceiptValidate,
}

func init() {
	receiptCmd.AddCommand(receiptValidateCmd)
	receiptValidateCmd.Flags().StringVar(&receiptValidateFormat, "format", "ascii", "Output format: ascii | json (json emits a structured validation result)")
}

// receiptValidateResult is the JSON shape emitted by --format json. The
// schema is intentionally tiny because consumers should drive CI pass/
// fail on the exit code, not the JSON.
type receiptValidateResult struct {
	Path        string `json:"path"`
	Fingerprint string `json:"fingerprint"`
	Valid       bool   `json:"valid"`
	Error       string `json:"error,omitempty"`
}

func runReceiptValidate(cmd *cobra.Command, args []string) error {
	format := strings.ToLower(strings.TrimSpace(receiptValidateFormat))
	if format != "ascii" && format != "json" {
		// Invalid flag value is a usage error, not an I/O / mismatch
		// case. Falls through to the default cobra error → exit 1.
		return fmt.Errorf("invalid --format %q (valid: ascii, json)", receiptValidateFormat)
	}

	path := args[0]
	stmt, err := agent.LoadStatement(path)
	if err != nil {
		// I/O or parse error → exit code 2 (per docs). main.go's
		// exitCodeError dispatch picks this up via errors.As.
		return newExitCodeError(
			fmt.Errorf("load receipt %s: %w", path, err),
			2,
		)
	}

	vErr := agent.VerifyStatementFingerprint(stmt)
	result := receiptValidateResult{
		Path:        path,
		Fingerprint: stmt.Predicate.Fingerprint,
		Valid:       vErr == nil,
	}
	if vErr != nil {
		result.Error = vErr.Error()
	}

	switch format {
	case "json":
		buf, mErr := json.MarshalIndent(result, "", "  ")
		if mErr != nil {
			return fmt.Errorf("marshal validate result: %w", mErr)
		}
		fmt.Println(string(buf))
	case "ascii":
		if result.Valid {
			fmt.Printf("Fingerprint OK: %s\n  %s\n", path, result.Fingerprint)
		} else {
			fmt.Printf("Fingerprint MISMATCH: %s\n  recorded:  %s\n  error:     %s\n", path, result.Fingerprint, result.Error)
		}
	}

	if vErr != nil {
		// Fingerprint mismatch → exit code 1 (per docs).
		return newExitCodeError(
			fmt.Errorf("receipt fingerprint mismatch"),
			1,
		)
	}
	return nil
}
