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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func withFakeObjectSetLoader(t *testing.T, fn func(context.Context, []*unstructured.Unstructured, string) ([]agent.ObjectSetObservedObject, error)) {
	t.Helper()
	prev := loadObjectSetLiveObjectsFn
	loadObjectSetLiveObjectsFn = fn
	t.Cleanup(func() { loadObjectSetLiveObjectsFn = prev })
}

func writeObjectSetManifest(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "rendered.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return path
}

func TestReceiptVerifyObjectSet_JSONPass(t *testing.T) {
	resetReceiptFlags(t)
	resetReceiptBatch3Flags(t)
	resetReceiptFailOnFlag(t)
	path := writeObjectSetManifest(t, `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: api
        image: example/api:v1
`)
	withFakeObjectSetLoader(t, func(_ context.Context, desired []*unstructured.Unstructured, defaultNS string) ([]agent.ObjectSetObservedObject, error) {
		if defaultNS != "prod" {
			t.Fatalf("default namespace = %q, want prod", defaultNS)
		}
		live := desired[0].DeepCopy()
		live.SetNamespace(defaultNS)
		desired[0].SetNamespace(defaultNS)
		_ = unstructured.SetNestedField(live.Object, "123", "metadata", "resourceVersion")
		_ = unstructured.SetNestedField(live.Object, "server-added", "metadata", "annotations", "example.com/defaulted")
		return []agent.ObjectSetObservedObject{{Desired: desired[0], Live: live}}, nil
	})

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"receipt", "verify", "--file", path, "--scope", "namespace/prod", "--format", "json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("receipt verify --file returned error: %v", err)
		}
	})

	var stmt agent.Statement
	if err := json.Unmarshal([]byte(out), &stmt); err != nil {
		t.Fatalf("unmarshal statement: %v\n%s", err, out)
	}
	if stmt.Predicate.PredicateName != string(agent.PredicateObjectSetMatches) {
		t.Fatalf("predicate = %s", stmt.Predicate.PredicateName)
	}
	if stmt.Predicate.Verdict != agent.VerdictPASS {
		t.Fatalf("verdict = %s, want PASS", stmt.Predicate.Verdict)
	}
	if stmt.Predicate.Evidence.ObjectSet == nil {
		t.Fatal("objectSet evidence missing")
	}
	if stmt.Predicate.Evidence.ObjectSet.Summary.Matched != 1 {
		t.Fatalf("matched = %d, want 1", stmt.Predicate.Evidence.ObjectSet.Summary.Matched)
	}
}

func TestReceiptVerifyObjectSet_FailOnBlock(t *testing.T) {
	resetReceiptFlags(t)
	resetReceiptBatch3Flags(t)
	resetReceiptFailOnFlag(t)
	path := writeObjectSetManifest(t, `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: prod
spec:
  replicas: 3
`)
	withFakeObjectSetLoader(t, func(_ context.Context, desired []*unstructured.Unstructured, _ string) ([]agent.ObjectSetObservedObject, error) {
		live := desired[0].DeepCopy()
		_ = unstructured.SetNestedField(live.Object, int64(5), "spec", "replicas")
		return []agent.ObjectSetObservedObject{{Desired: desired[0], Live: live}}, nil
	})

	rootCmd.SetArgs([]string{"receipt", "verify", "--file", path, "--format", "json", "--fail-on", "BLOCK"})
	captureStdout(t, func() {
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("BLOCK object-set receipt with --fail-on BLOCK must error")
		}
		var ec interface{ ExitCode() int }
		if !errors.As(err, &ec) || ec.ExitCode() != 2 {
			t.Fatalf("expected exit code 2 wrapper; got %v", err)
		}
		if !strings.Contains(err.Error(), "fail-on") {
			t.Fatalf("expected fail-on error, got %v", err)
		}
	})
}

func TestReceiptVerifyObjectSet_RejectsPositionalSubject(t *testing.T) {
	resetReceiptFlags(t)
	path := writeObjectSetManifest(t, `
apiVersion: v1
kind: ConfigMap
metadata:
  name: cfg
`)
	rootCmd.SetArgs([]string{"receipt", "verify", "deploy/api", "--file", path})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("--file with positional subject must error")
	}
	if !strings.Contains(err.Error(), "does not accept a positional subject") {
		t.Fatalf("unexpected error: %v", err)
	}
}
