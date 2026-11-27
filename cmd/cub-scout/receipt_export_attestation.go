// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/confighub/cub-scout/v3/pkg/agent"
	"github.com/spf13/cobra"
)

const landedAttestationPredicateType = "https://confighub.com/attestations/landed/v1"

var (
	receiptExportAttestationInput   string
	receiptExportAttestationPubKey  string
	receiptExportAttestationCluster string
	receiptExportAttestationTarget  string
	receiptExportAttestationRelease string
	receiptExportAttestationOutput  string
)

var receiptExportAttestationCmd = &cobra.Command{
	Use:   "export-attestation",
	Short: "Export a signed receipt as a deterministic in-toto Statement v1 attestation offline",
	Long: `export-attestation reads a signed-receipt/v1 JSON envelope from a local file
or stdin ('-') and emits a deterministic in-toto Statement v1 JSON document with
predicateType "https://confighub.com/attestations/landed/v1" for ConfigHub G4
attestation ingestion.

When --pubkey is provided, export-attestation verifies the Ed25519 signature and
canonical payloadDigest via VerifySignedReceipt first and fails closed (non-zero
exit, no output written) on any invalid signature, wrong key, or tampered payload.
When --pubkey is omitted, it still validates the signed-receipt/v1 envelope
structure and canonical payloadDigest, and emits the Statement with verified=false.

export-attestation is strictly offline and read-only: zero network calls, zero
Kubernetes calls, and zero live ConfigHub writes.

Examples:
  cub-scout receipt export-attestation --input prod.signed.json --pubkey pub.pem --release rel-42 -o prod.att.json
  cub-scout receipt export-attestation --input - --cluster prod-east --target Deployment/prod/api < prod.signed.json
`,
	Args: cobra.NoArgs,
	RunE: runReceiptExportAttestation,
}

func init() {
	receiptCmd.AddCommand(receiptExportAttestationCmd)

	receiptExportAttestationCmd.Flags().StringVar(&receiptExportAttestationInput, "input", "", "Path to signed-receipt/v1 JSON file, or '-' for stdin (required)")
	receiptExportAttestationCmd.Flags().StringVar(&receiptExportAttestationPubKey, "pubkey", "", "Optional path to Ed25519 PKIX public key PEM file; verifies signature before export")
	receiptExportAttestationCmd.Flags().StringVar(&receiptExportAttestationCluster, "cluster", "", "Override cluster name in the exported attestation")
	receiptExportAttestationCmd.Flags().StringVar(&receiptExportAttestationTarget, "target", "", "Override target slug/reference in the exported attestation")
	receiptExportAttestationCmd.Flags().StringVar(&receiptExportAttestationRelease, "release", "", "Release identifier to record in the exported attestation predicate")
	receiptExportAttestationCmd.Flags().StringVarP(&receiptExportAttestationOutput, "output", "o", "", "Write in-toto Statement v1 JSON to this file path (default: stdout)")
}

type landedAttestationStatement struct {
	Type          string                     `json:"_type"`
	Subject       []agent.Subject            `json:"subject"`
	PredicateType string                     `json:"predicateType"`
	Predicate     landedAttestationPredicate `json:"predicate"`
}

type landedAttestationPredicate struct {
	Cluster            string `json:"cluster,omitempty"`
	Target             string `json:"target,omitempty"`
	Release            string `json:"release,omitempty"`
	Revision           string `json:"revision,omitempty"`
	State              string `json:"state"`
	ObservedAt         string `json:"observedAt,omitempty"`
	SignedAt           string `json:"signedAt,omitempty"`
	KeyID              string `json:"keyId"`
	PayloadDigest      string `json:"payloadDigest"`
	SignatureAlgorithm string `json:"signatureAlgorithm"`
	Verified           bool   `json:"verified"`
}

func runReceiptExportAttestation(cmd *cobra.Command, _ []string) error {
	inputPath := strings.TrimSpace(receiptExportAttestationInput)
	if inputPath == "" {
		return fmt.Errorf("--input is required (path to signed-receipt/v1 JSON file or '-' for stdin)")
	}

	envBytes, err := readInputBytes(cmd, inputPath)
	if err != nil {
		return newExitCodeError(fmt.Errorf("read signed receipt %s: %w", inputPath, err), 2)
	}

	var pubBytes []byte
	pubKeyPath := strings.TrimSpace(receiptExportAttestationPubKey)
	if pubKeyPath != "" {
		pubBytes, err = os.ReadFile(pubKeyPath)
		if err != nil {
			return newExitCodeError(fmt.Errorf("read public key %s: %w", pubKeyPath, err), 2)
		}
	}

	stmt, err := buildLandedAttestationStatement(
		envBytes,
		pubBytes,
		inputPath,
		strings.TrimSpace(receiptExportAttestationCluster),
		strings.TrimSpace(receiptExportAttestationTarget),
		strings.TrimSpace(receiptExportAttestationRelease),
	)
	if err != nil {
		return newExitCodeError(err, 1)
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(stmt); err != nil {
		return fmt.Errorf("marshal attestation statement: %w", err)
	}
	outBytes := buf.Bytes()

	outPath := strings.TrimSpace(receiptExportAttestationOutput)
	if outPath != "" {
		if dir := filepath.Dir(outPath); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("create output directory %s: %w", dir, err)
			}
		}
		if err := os.WriteFile(outPath, outBytes, 0o644); err != nil {
			return fmt.Errorf("write attestation %s: %w", outPath, err)
		}
		return nil
	}

	_, err = cmd.OutOrStdout().Write(outBytes)
	return err
}

func buildLandedAttestationStatement(envBytes, pubKeyPEM []byte, inputPath, clusterOverride, targetOverride, releaseOverride string) (*landedAttestationStatement, error) {
	verified := false
	if len(pubKeyPEM) > 0 {
		res, err := agent.VerifySignedReceipt(envBytes, pubKeyPEM)
		if err != nil || res == nil || !res.Valid {
			if err != nil {
				return nil, fmt.Errorf("signed receipt verification failed: %w", err)
			}
			return nil, fmt.Errorf("signed receipt verification failed")
		}
		verified = true
	}

	env, sha256Hex, err := validateSignedReceiptEnvelope(envBytes)
	if err != nil {
		return nil, err
	}

	meta, payloadRelease := extractAttestationPayloadMetadata(env.Payload, inputPath, env.SignedAt)

	cluster := meta.Cluster
	if clusterOverride != "" {
		cluster = clusterOverride
	}
	target := meta.Target
	if targetOverride != "" {
		target = targetOverride
	}
	release := payloadRelease
	if releaseOverride != "" {
		release = releaseOverride
	}

	subjectName := formatAttestationSubjectName(cluster, target)

	return &landedAttestationStatement{
		Type: agent.StatementType,
		Subject: []agent.Subject{
			{
				Name: subjectName,
				Digest: map[string]string{
					"sha256": sha256Hex,
				},
			},
		},
		PredicateType: landedAttestationPredicateType,
		Predicate: landedAttestationPredicate{
			Cluster:            cluster,
			Target:             target,
			Release:            release,
			Revision:           meta.Revision,
			State:              meta.State,
			ObservedAt:         meta.ObservedAt,
			SignedAt:           strings.TrimSpace(env.SignedAt),
			KeyID:              strings.TrimSpace(env.KeyID),
			PayloadDigest:      strings.TrimSpace(env.PayloadDigest),
			SignatureAlgorithm: env.Algorithm,
			Verified:           verified,
		},
	}, nil
}

func validateSignedReceiptEnvelope(envBytes []byte) (*agent.SignedReceiptEnvelope, string, error) {
	if len(bytes.TrimSpace(envBytes)) == 0 {
		return nil, "", fmt.Errorf("signed receipt envelope is empty")
	}
	var env agent.SignedReceiptEnvelope
	if err := json.Unmarshal(envBytes, &env); err != nil {
		return nil, "", fmt.Errorf("parse signed receipt envelope: %w", err)
	}
	if env.SchemaVersion != agent.SignedReceiptSchemaVersion {
		return nil, "", fmt.Errorf("unsupported schemaVersion %q (expected %q)", env.SchemaVersion, agent.SignedReceiptSchemaVersion)
	}
	if env.Algorithm != agent.SignedReceiptAlgorithm {
		return nil, "", fmt.Errorf("unsupported signature algorithm %q (expected %q)", env.Algorithm, agent.SignedReceiptAlgorithm)
	}
	if strings.TrimSpace(env.Signature) == "" {
		return nil, "", fmt.Errorf("missing signature in signed receipt envelope")
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(env.Signature))
	if err != nil {
		return nil, "", fmt.Errorf("decode base64 signature: %w", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return nil, "", fmt.Errorf("invalid ed25519 signature length %d (expected %d)", len(sig), ed25519.SignatureSize)
	}

	actualDigest, _, err := agent.ComputePayloadDigest(env.Payload)
	if err != nil {
		return nil, "", fmt.Errorf("canonicalize envelope payload: %w", err)
	}
	if strings.TrimSpace(env.PayloadDigest) == "" {
		return nil, "", fmt.Errorf("missing payloadDigest in signed receipt envelope")
	}
	if subtle.ConstantTimeCompare([]byte(actualDigest), []byte(env.PayloadDigest)) != 1 {
		return nil, "", fmt.Errorf("payload digest mismatch: computed %s, envelope has %s", actualDigest, env.PayloadDigest)
	}

	sha256Hex, ok := strings.CutPrefix(actualDigest, "sha256:")
	if !ok || sha256Hex == "" {
		return nil, "", fmt.Errorf("invalid canonical payloadDigest %q", actualDigest)
	}
	return &env, sha256Hex, nil
}

func extractAttestationPayloadMetadata(payload json.RawMessage, inputPath, signedAt string) (extractedReceiptMetadata, string) {
	meta := extractedReceiptMetadata{
		ObservedAt: strings.TrimSpace(signedAt),
		State:      agent.LandingStateNotObservable,
	}
	if inputPath != "" && inputPath != "-" {
		meta.Cluster = clusterNameFromFilename(inputPath)
	}

	var doc map[string]interface{}
	if err := json.Unmarshal(payload, &doc); err != nil {
		meta.Reason = "payload-not-object"
		return meta, ""
	}

	if c := extractClusterIdentity(doc); c != "" {
		meta.Cluster = c
	}
	if ts := extractObservedAt(doc); ts != "" {
		meta.ObservedAt = ts
	}
	meta.Target = extractTargetRef(doc)
	meta.Revision = extractObservedRevision(doc)
	meta.State, meta.Reason = extractLandingState(doc)

	release := fleetStringField(doc, "release")
	if release == "" {
		if predObj, ok := doc["predicate"].(map[string]interface{}); ok {
			release = fleetStringField(predObj, "release")
		}
	}
	return meta, release
}

func formatAttestationSubjectName(cluster, target string) string {
	switch {
	case cluster != "" && target != "":
		return cluster + "/" + target
	case cluster != "":
		return cluster
	case target != "":
		return target
	default:
		return "unknown"
	}
}
