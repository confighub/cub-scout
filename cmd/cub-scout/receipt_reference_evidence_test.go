// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// --reference-evidence chains an external (non-cub-scout) artifact into a
// cub-scout receipt's inputAttestations[] by content digest, under the
// external-evidence:// scheme, and the receipt fingerprint covers it.
func TestReceiptVerify_ReferenceEvidence_ChainsExternalByDigest(t *testing.T) {
	resetReceiptFlags(t)
	resetReceiptBatch3Flags(t)
	resetReceiptFailOnFlag(t)
	prevRef := receiptReferenceEvidence
	receiptReferenceEvidence = nil
	t.Cleanup(func() { receiptReferenceEvidence = prevRef })

	dir := t.TempDir()
	ext := filepath.Join(dir, "upstream-package.receipt.json")
	if err := os.WriteFile(ext, []byte(`{"kind":"InstallerPackageReceipt","renderedObjectSetSHA256":"deadbeef"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := writeObjectSetManifest(t, `
apiVersion: v1
kind: ConfigMap
metadata:
  name: cfg
  namespace: prod
`)
	withFakeObjectSetLoader(t, func(_ context.Context, desired []*unstructured.Unstructured, _ string) ([]agent.ObjectSetObservedObject, error) {
		live := desired[0].DeepCopy()
		live.SetNamespace("prod")
		desired[0].SetNamespace("prod")
		return []agent.ObjectSetObservedObject{{Desired: desired[0], Live: live}}, nil
	})

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"receipt", "verify", "--file", manifest, "--scope", "namespace/prod", "--predicate", "object-set-matches", "--reference-evidence", ext, "--format", "json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var stmt agent.Statement
	if err := json.Unmarshal([]byte(out), &stmt); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	if len(stmt.Predicate.InputAttestations) != 1 {
		t.Fatalf("expected 1 inputAttestation, got %d", len(stmt.Predicate.InputAttestations))
	}
	ref := stmt.Predicate.InputAttestations[0]
	if !strings.HasPrefix(ref.URI, agent.ExternalEvidenceURIScheme) {
		t.Fatalf("URI %q is not external-evidence://", ref.URI)
	}
	if ref.Digest["sha256"] == "" {
		t.Fatal("external ref missing sha256 digest")
	}
	if err := agent.VerifyStatementFingerprint(stmt); err != nil {
		t.Fatalf("fingerprint must cover the external reference: %v", err)
	}
}
