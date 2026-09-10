// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
)

func TestReceiptVerify_ModelplaneCrossplaneEvidenceJSON(t *testing.T) {
	resetReceiptFlags(t)
	withFakeReceiptLoader(t, makeModelplaneCrossplaneWorkload())

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"receipt", "verify", "deploy/qwen-engine", "-n", "models", "--format", "json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("receipt verify returned error: %v", err)
		}
	})

	var stmt agent.Statement
	if err := json.Unmarshal([]byte(out), &stmt); err != nil {
		t.Fatalf("unmarshal receipt: %v\nraw:\n%s", err, out)
	}
	evidence := stmt.Predicate.Evidence.PlatformSubstrate
	if evidence == nil {
		t.Fatalf("platformSubstrate evidence missing: %+v", stmt.Predicate.Evidence)
	}
	if evidence.Platform != agent.OwnerModelplane || evidence.Substrate != agent.OwnerCrossplane {
		t.Fatalf("platformSubstrate = %+v, want Modelplane-on-Crossplane", evidence)
	}
	if evidence.Composite != "x-qwen" || evidence.CompositionResource != "engine" {
		t.Fatalf("platformSubstrate = %+v, want composite and composition resource", evidence)
	}
	if evidence.Claim == nil || evidence.Claim.Name != "qwen-claim" || evidence.Claim.Namespace != "models" {
		t.Fatalf("claim = %+v, want models/qwen-claim", evidence.Claim)
	}
	if err := agent.VerifyStatementFingerprint(stmt); err != nil {
		t.Fatalf("receipt fingerprint must cover platformSubstrate evidence: %v", err)
	}
}

func TestReceiptVerify_ModelplaneCrossplaneEvidenceASCII(t *testing.T) {
	resetReceiptFlags(t)
	withFakeReceiptLoader(t, makeModelplaneCrossplaneWorkload())

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"receipt", "verify", "deploy/qwen-engine", "-n", "models"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("receipt verify returned error: %v", err)
		}
	})

	for _, want := range []string{
		"Evidence (platform substrate)",
		"platform:    modelplane",
		"substrate:   crossplane",
		"summary:     Modelplane-on-Crossplane evidence",
		"composite=x-qwen",
		"claim=models/qwen-claim",
		"compositionResource=engine",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ASCII output missing %q:\n%s", want, out)
		}
	}
}
