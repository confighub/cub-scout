// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func resetChainFlags(t *testing.T) {
	t.Helper()
	prevD, prevP, prevF, prevE := digestFile, digestProfile, chainFile, chainEvidenceDir
	prevIA, prevRE := receiptInputAttestations, receiptReferenceEvidence
	digestFile, digestProfile, chainFile, chainEvidenceDir = "", "", "", ""
	receiptInputAttestations, receiptReferenceEvidence = nil, nil
	t.Cleanup(func() {
		digestFile, digestProfile, chainFile, chainEvidenceDir = prevD, prevP, prevF, prevE
		receiptInputAttestations, receiptReferenceEvidence = prevIA, prevRE
	})
}

func buildChainFixture(t *testing.T, dir string) (leafPath string) {
	t.Helper()
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

	upstream := filepath.Join(dir, "upstream.receipt.json")
	receiptInputAttestations, receiptReferenceEvidence = nil, nil
	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"receipt", "verify", "--file", manifest, "--scope", "namespace/prod", "--predicate", "object-set-matches", "--format", "json", "--out", upstream})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("build upstream receipt: %v", err)
		}
	})

	external := filepath.Join(dir, "render.evidence.json")
	if err := os.WriteFile(external, []byte(`{"kind":"InstallerPackageReceipt","renderedObjectSetSHA256":"abc"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	leafPath = filepath.Join(dir, "leaf.receipt.json")
	receiptInputAttestations, receiptReferenceEvidence = nil, nil
	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"receipt", "verify", "--file", manifest, "--scope", "namespace/prod", "--predicate", "object-set-matches", "--format", "json",
			"--input-attestation", upstream, "--reference-evidence", external, "--out", leafPath})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("build leaf receipt: %v", err)
		}
	})
	return leafPath
}

func TestReceiptChain_VerifiesAndDetectsTamper(t *testing.T) {
	resetReceiptFlags(t)
	resetReceiptBatch3Flags(t)
	resetReceiptFailOnFlag(t)
	resetChainFlags(t)
	dir := t.TempDir()
	leaf := buildChainFixture(t, dir)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"receipt", "chain", "--file", leaf, "--evidence-dir", dir})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("chain verify should pass: %v", err)
		}
	})
	if !strings.Contains(out, "chain verified") {
		t.Fatalf("expected chain verified, got:\n%s", out)
	}
	if !strings.Contains(out, "digest-asserted, not fingerprint-verified") {
		t.Fatalf("external link must print its weaker trust label, got:\n%s", out)
	}

	// Tamper with the upstream receipt: the chain must fail with exit 2.
	upstream := filepath.Join(dir, "upstream.receipt.json")
	data, _ := os.ReadFile(upstream)
	if err := os.WriteFile(upstream, []byte(strings.Replace(string(data), "PASS", "FAIL", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	resetChainFlags(t)
	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"receipt", "chain", "--file", leaf, "--evidence-dir", dir})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("tampered upstream must fail the chain")
		}
		var ec interface{ ExitCode() int }
		if !errors.As(err, &ec) || ec.ExitCode() != 2 {
			t.Fatalf("expected exit 2, got %v", err)
		}
	})
}

func TestReceiptDigest_MatchesReceiptSubject(t *testing.T) {
	resetReceiptFlags(t)
	resetReceiptBatch3Flags(t)
	resetReceiptFailOnFlag(t)
	resetChainFlags(t)
	dir := t.TempDir()
	leaf := buildChainFixture(t, dir)

	// receipt digest over the same manifests must equal the leaf's
	// rendered-object-set subject digest (the cross-tool convention).
	stmt, err := agent.LoadStatement(leaf)
	if err != nil {
		t.Fatal(err)
	}
	var subjectDigest string
	for _, s := range stmt.Subject {
		if strings.HasPrefix(s.Name, agent.SubjectSchemeRenderedObjectSet) {
			subjectDigest = s.Digest["sha256"]
		}
	}
	if subjectDigest == "" {
		t.Fatal("leaf has no rendered-object-set subject")
	}

	manifest := writeObjectSetManifest(t, `
apiVersion: v1
kind: ConfigMap
metadata:
  name: cfg
  namespace: prod
`)
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"receipt", "digest", "--file", manifest})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("receipt digest: %v", err)
		}
	})
	if !strings.Contains(out, subjectDigest) {
		t.Fatalf("digest command output does not contain the receipt subject digest %s:\n%s", subjectDigest, out)
	}
}
