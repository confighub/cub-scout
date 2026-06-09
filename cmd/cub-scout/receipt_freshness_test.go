// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestReceiptVerify_TTLStampsFreshness(t *testing.T) {
	resetReceiptFlags(t)
	resetReceiptBatch3Flags(t)
	resetReceiptFailOnFlag(t)
	path := writeObjectSetManifest(t, `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
spec:
  replicas: 1
`)
	withFakeObjectSetLoader(t, func(_ context.Context, desired []*unstructured.Unstructured, ns string) ([]agent.ObjectSetObservedObject, error) {
		live := desired[0].DeepCopy()
		live.SetNamespace(ns)
		desired[0].SetNamespace(ns)
		return []agent.ObjectSetObservedObject{{Desired: desired[0], Live: live}}, nil
	})

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"receipt", "verify", "--file", path, "--scope", "namespace/prod", "--predicate", "object-set-matches", "--ttl", "2h", "--format", "json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var stmt agent.Statement
	if err := json.Unmarshal([]byte(out), &stmt); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	if stmt.Predicate.Freshness == nil {
		t.Fatal("freshness missing with --ttl")
	}
	if stmt.Predicate.Freshness.TTL != "2h0m0s" {
		t.Fatalf("ttl = %s, want 2h0m0s", stmt.Predicate.Freshness.TTL)
	}
	if stmt.Predicate.Freshness.ObservedAt == "" || stmt.Predicate.Freshness.ExpiresAt == "" {
		t.Fatalf("observedAt/expiresAt empty: %+v", stmt.Predicate.Freshness)
	}
	if err := agent.VerifyStatementFingerprint(stmt); err != nil {
		t.Fatalf("fingerprint with freshness: %v", err)
	}
}

func TestReceiptVerify_NoTTLNoFreshness(t *testing.T) {
	resetReceiptFlags(t)
	resetReceiptBatch3Flags(t)
	resetReceiptFailOnFlag(t)
	path := writeObjectSetManifest(t, `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
spec:
  replicas: 1
`)
	withFakeObjectSetLoader(t, func(_ context.Context, desired []*unstructured.Unstructured, ns string) ([]agent.ObjectSetObservedObject, error) {
		live := desired[0].DeepCopy()
		live.SetNamespace(ns)
		desired[0].SetNamespace(ns)
		return []agent.ObjectSetObservedObject{{Desired: desired[0], Live: live}}, nil
	})

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"receipt", "verify", "--file", path, "--scope", "namespace/prod", "--predicate", "object-set-matches", "--format", "json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})
	var stmt agent.Statement
	if err := json.Unmarshal([]byte(out), &stmt); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	if stmt.Predicate.Freshness != nil {
		t.Fatalf("no --ttl must not stamp freshness, got %+v", stmt.Predicate.Freshness)
	}
}

func TestReceiptVerify_RejectsBadTTL(t *testing.T) {
	resetReceiptFlags(t)
	resetReceiptBatch3Flags(t)
	resetReceiptFailOnFlag(t)
	rootCmd.SetArgs([]string{"receipt", "verify", "deploy/api", "-n", "prod", "--ttl", "nope"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("invalid --ttl must error")
	}
}
