// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/confighub/cub-scout/v3/pkg/agent"
	"github.com/spf13/cobra"
)

var (
	receiptSignInput  string
	receiptSignKey    string
	receiptSignOutput string

	receiptVerifySignedInput  string
	receiptVerifySignedPubKey string

	receiptKeygenOutPrivate string
	receiptKeygenOutPublic  string
)

var receiptSignCmd = &cobra.Command{
	Use:   "sign",
	Short: "Sign a JSON receipt or landed payload offline with an Ed25519 private key",
	Long: `sign reads a JSON payload (such as a receipt/v1 in-toto Statement or
cub-scout landed --format json output), canonicalizes it deterministically,
computes its SHA-256 digest, signs it with a local Ed25519 PKCS#8 private key,
and emits a signed-receipt/v1 JSON envelope.

sign is strictly offline and read-only against clusters (zero network or
Kubernetes calls).

Examples:
  cub-scout receipt sign --input api.receipt.json --key priv.pem -o api.signed.json
  cub-scout landed deploy/api -n prod --format json | cub-scout receipt sign --input - --key priv.pem
`,
	Args: cobra.NoArgs,
	RunE: runReceiptSign,
}

var receiptKeygenCmd = &cobra.Command{
	Use:   "keygen",
	Short: "Generate a local Ed25519 key pair for signing and verifying receipts",
	Long: `keygen generates a fresh Ed25519 key pair on local disk in PKCS#8 private
key PEM (0600) and PKIX public key PEM (0644) formats for use with
'cub-scout receipt sign' and 'cub-scout receipt verify --input ... --pubkey ...'.

keygen performs strictly local file operations and never touches Kubernetes or
any network endpoint.

Examples:
  cub-scout receipt keygen --out-private receipt-priv.pem --out-public receipt-pub.pem
`,
	Args: cobra.NoArgs,
	RunE: runReceiptKeygen,
}

func init() {
	receiptCmd.AddCommand(receiptSignCmd)
	receiptCmd.AddCommand(receiptKeygenCmd)

	receiptSignCmd.Flags().StringVar(&receiptSignInput, "input", "", "Path to JSON receipt/landed payload file, or '-' for stdin (required)")
	receiptSignCmd.Flags().StringVar(&receiptSignKey, "key", "", "Path to Ed25519 PKCS#8 private key PEM file (required)")
	receiptSignCmd.Flags().StringVarP(&receiptSignOutput, "output", "o", "", "Write signed envelope JSON to this file path (default: stdout)")

	receiptKeygenCmd.Flags().StringVar(&receiptKeygenOutPrivate, "out-private", "", "Output path for PKCS#8 Ed25519 private key PEM (required)")
	receiptKeygenCmd.Flags().StringVar(&receiptKeygenOutPublic, "out-public", "", "Output path for PKIX Ed25519 public key PEM (required)")

	// Offline signed-receipt verification flags on `receipt verify`.
	// When either flag is provided, runReceiptVerifyDispatch routes directly
	// to offline signature verification without touching kubeconfig or clusters.
	receiptVerifyCmd.Flags().StringVar(&receiptVerifySignedInput, "input", "", "Offline mode: path to a signed-receipt/v1 JSON envelope file, or '-' for stdin (requires --pubkey)")
	receiptVerifyCmd.Flags().StringVar(&receiptVerifySignedPubKey, "pubkey", "", "Offline mode: path to Ed25519 PKIX public key PEM file (requires --input)")
}

func runReceiptSign(cmd *cobra.Command, _ []string) error {
	inputPath := strings.TrimSpace(receiptSignInput)
	keyPath := strings.TrimSpace(receiptSignKey)
	if inputPath == "" {
		return fmt.Errorf("--input is required (path to JSON payload or '-' for stdin)")
	}
	if keyPath == "" {
		return fmt.Errorf("--key is required (path to Ed25519 PKCS#8 private key PEM)")
	}

	payloadBytes, err := readInputBytes(cmd, inputPath)
	if err != nil {
		return fmt.Errorf("read payload %s: %w", inputPath, err)
	}
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("read private key %s: %w", keyPath, err)
	}

	env, err := agent.SignReceipt(payloadBytes, keyBytes)
	if err != nil {
		return fmt.Errorf("sign receipt: %w", err)
	}

	outBytes, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal signed envelope: %w", err)
	}
	outBytes = append(outBytes, '\n')

	outPath := strings.TrimSpace(receiptSignOutput)
	if outPath != "" {
		if dir := filepath.Dir(outPath); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("create output directory %s: %w", dir, err)
			}
		}
		if err := os.WriteFile(outPath, outBytes, 0o644); err != nil {
			return fmt.Errorf("write signed envelope %s: %w", outPath, err)
		}
		return nil
	}

	_, err = cmd.OutOrStdout().Write(outBytes)
	return err
}

func runReceiptVerifySigned(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("offline signed receipt verification (--input/--pubkey) does not accept positional arguments")
	}
	inputPath := strings.TrimSpace(receiptVerifySignedInput)
	pubKeyPath := strings.TrimSpace(receiptVerifySignedPubKey)
	if inputPath == "" {
		return fmt.Errorf("--input is required when --pubkey is set")
	}
	if pubKeyPath == "" {
		return fmt.Errorf("--pubkey is required when --input is set")
	}

	format := strings.ToLower(strings.TrimSpace(receiptFormat))
	if format != "ascii" && format != "json" {
		return fmt.Errorf("invalid --format %q (valid: ascii, json)", receiptFormat)
	}

	envBytes, err := readInputBytes(cmd, inputPath)
	if err != nil {
		return newExitCodeError(fmt.Errorf("read signed envelope %s: %w", inputPath, err), 2)
	}
	pubBytes, err := os.ReadFile(pubKeyPath)
	if err != nil {
		return newExitCodeError(fmt.Errorf("read public key %s: %w", pubKeyPath, err), 2)
	}

	result, verifyErr := agent.VerifySignedReceipt(envBytes, pubBytes)

	out := cmd.OutOrStdout()
	switch format {
	case "json":
		buf, mErr := json.MarshalIndent(result, "", "  ")
		if mErr != nil {
			return fmt.Errorf("marshal verification result: %w", mErr)
		}
		if _, wErr := fmt.Fprintln(out, string(buf)); wErr != nil {
			return wErr
		}
	case "ascii":
		if result.Valid {
			_, _ = fmt.Fprintf(out, "Signature OK: %s\n  algorithm: %s\n  keyId:     %s\n  digest:    %s\n  signedAt:  %s\n",
				inputPath, result.Algorithm, result.KeyID, result.PayloadDigest, result.SignedAt)
		} else {
			_, _ = fmt.Fprintf(out, "Signature INVALID: %s\n  error: %s\n", inputPath, result.Error)
		}
	}

	if verifyErr != nil {
		return newExitCodeError(fmt.Errorf("signed receipt verification failed: %w", verifyErr), 1)
	}
	return nil
}

func runReceiptKeygen(cmd *cobra.Command, _ []string) error {
	privPath := strings.TrimSpace(receiptKeygenOutPrivate)
	pubPath := strings.TrimSpace(receiptKeygenOutPublic)
	if privPath == "" {
		return fmt.Errorf("--out-private is required")
	}
	if pubPath == "" {
		return fmt.Errorf("--out-public is required")
	}

	privPEM, pubPEM, keyID, err := agent.GenerateEd25519KeyPair()
	if err != nil {
		return err
	}

	for _, target := range []struct {
		path string
		data []byte
		perm os.FileMode
	}{
		{path: privPath, data: privPEM, perm: 0o600},
		{path: pubPath, data: pubPEM, perm: 0o644},
	} {
		if dir := filepath.Dir(target.path); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("create directory %s: %w", dir, err)
			}
		}
		if err := os.WriteFile(target.path, target.data, target.perm); err != nil {
			return fmt.Errorf("write %s: %w", target.path, err)
		}
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Generated Ed25519 key pair\n  keyId:   %s\n  private: %s\n  public:  %s\n", keyID, privPath, pubPath)
	return err
}

func readInputBytes(cmd *cobra.Command, path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(cmd.InOrStdin())
	}
	return os.ReadFile(path)
}
