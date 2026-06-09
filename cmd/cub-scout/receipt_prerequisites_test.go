// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
)

func withFakePrerequisitesLoader(t *testing.T, fn func(context.Context, prerequisiteSpec, string) ([]agent.PrerequisiteFactResult, error)) {
	t.Helper()
	prev := loadPrerequisitesLiveFn
	loadPrerequisitesLiveFn = fn
	t.Cleanup(func() { loadPrerequisitesLiveFn = prev })
}

func writePrereqsFile(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "prereqs.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write prereqs: %v", err)
	}
	return path
}

// F3 root cause as a pre-flight check: the required Secret is absent.
func TestReceiptVerifyPrerequisites_BLOCKOnMissingSecretFailOnExit2(t *testing.T) {
	resetReceiptFlags(t)
	resetReceiptBatch3Flags(t)
	resetReceiptFailOnFlag(t)
	path := writePrereqsFile(t, `
requiredNamespaces:
  - helm-expt-demo
requiredSecrets:
  - name: app-db-secret
    namespace: helm-expt-demo
    keys: [password]
`)
	withFakePrerequisitesLoader(t, func(_ context.Context, spec prerequisiteSpec, _ string) ([]agent.PrerequisiteFactResult, error) {
		if len(spec.RequiredSecrets) != 1 || spec.RequiredSecrets[0].Name != "app-db-secret" {
			t.Fatalf("spec not parsed: %+v", spec)
		}
		return []agent.PrerequisiteFactResult{
			{Kind: agent.PrerequisiteKindNamespace, Name: "helm-expt-demo", Status: agent.PrerequisitePresent},
			{Kind: agent.PrerequisiteKindSecret, Name: "app-db-secret", Namespace: "helm-expt-demo", Status: agent.PrerequisiteMissing, Error: "secrets \"app-db-secret\" not found"},
		}, nil
	})

	rootCmd.SetArgs([]string{"receipt", "verify", "--prerequisites", path, "--scope", "namespace/helm-expt-demo", "--format", "json", "--fail-on", "any-non-pass"})
	out := captureStdout(t, func() {
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("missing prerequisite with --fail-on any-non-pass must error")
		}
		var ec interface{ ExitCode() int }
		if !errors.As(err, &ec) || ec.ExitCode() != 2 {
			t.Fatalf("expected exit code 2 wrapper; got %v", err)
		}
	})

	var stmt agent.Statement
	if err := json.Unmarshal([]byte(out), &stmt); err != nil {
		t.Fatalf("unmarshal statement: %v\n%s", err, out)
	}
	if stmt.Predicate.PredicateName != string(agent.PredicatePrerequisitesMet) {
		t.Fatalf("predicate = %s", stmt.Predicate.PredicateName)
	}
	if stmt.Predicate.Verdict != agent.VerdictBLOCK {
		t.Fatalf("verdict = %s, want BLOCK", stmt.Predicate.Verdict)
	}
	if stmt.Predicate.Evidence.Prerequisites == nil || stmt.Predicate.Evidence.Prerequisites.Summary.Missing != 1 {
		t.Fatalf("expected 1 missing prerequisite, got %+v", stmt.Predicate.Evidence.Prerequisites)
	}
}

func TestReceiptVerifyPrerequisites_PASSWhenAllPresent(t *testing.T) {
	resetReceiptFlags(t)
	resetReceiptBatch3Flags(t)
	resetReceiptFailOnFlag(t)
	path := writePrereqsFile(t, `
requiredNamespaces:
  - helm-expt-demo
`)
	withFakePrerequisitesLoader(t, func(_ context.Context, _ prerequisiteSpec, _ string) ([]agent.PrerequisiteFactResult, error) {
		return []agent.PrerequisiteFactResult{
			{Kind: agent.PrerequisiteKindNamespace, Name: "helm-expt-demo", Status: agent.PrerequisitePresent},
		}, nil
	})

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"receipt", "verify", "--prerequisites", path, "--scope", "namespace/helm-expt-demo", "--format", "json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})
	var stmt agent.Statement
	if err := json.Unmarshal([]byte(out), &stmt); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	if stmt.Predicate.Verdict != agent.VerdictPASS {
		t.Fatalf("verdict = %s, want PASS", stmt.Predicate.Verdict)
	}
}

func TestReceiptVerifyPrerequisites_RequiresFile(t *testing.T) {
	resetReceiptFlags(t)
	resetReceiptBatch3Flags(t)
	resetReceiptFailOnFlag(t)
	rootCmd.SetArgs([]string{"receipt", "verify", "--predicate", "prerequisites-met"})
	err := rootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "requires --prerequisites") {
		t.Fatalf("expected requires --prerequisites error, got %v", err)
	}
}
