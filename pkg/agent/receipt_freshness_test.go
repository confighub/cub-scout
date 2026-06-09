// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"
	"time"
)

func freshnessTestReceipt(t *testing.T) Statement {
	t.Helper()
	desired := objectSetDeployment(1, "example/api:v1")
	live := objectSetDeployment(1, "example/api:v1")
	ev, err := BuildObjectSetEvidence(
		ObjectSetSource{Type: "file", Ref: "rendered.yaml"},
		ObjectSetScope{Kind: "namespace", Namespace: "prod"},
		[]ObjectSetObservedObject{{Desired: desired, Live: live}},
	)
	if err != nil {
		t.Fatalf("BuildObjectSetEvidence: %v", err)
	}
	stmt, err := BuildObjectSetReceipt(BuildObjectSetReceiptInput{
		Evidence:   ev,
		Verifier:   Verifier{Tool: "cub-scout", Version: "test"},
		VerifiedAt: time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildObjectSetReceipt: %v", err)
	}
	return stmt
}

func TestApplyFreshness_StampsAndReFingerprints(t *testing.T) {
	stmt := freshnessTestReceipt(t)
	if stmt.Predicate.Freshness != nil {
		t.Fatal("freshness should be nil before apply")
	}
	if err := ApplyFreshness(&stmt, time.Hour); err != nil {
		t.Fatalf("ApplyFreshness: %v", err)
	}
	f := stmt.Predicate.Freshness
	if f == nil {
		t.Fatal("freshness nil after apply")
	}
	if f.ObservedAt != "2026-05-28T12:00:00Z" {
		t.Fatalf("observedAt = %s", f.ObservedAt)
	}
	if f.ExpiresAt != "2026-05-28T13:00:00Z" {
		t.Fatalf("expiresAt = %s, want observedAt + 1h", f.ExpiresAt)
	}
	if f.TTL != "1h0m0s" {
		t.Fatalf("ttl = %s", f.TTL)
	}
	// The fingerprint must now cover the freshness fields.
	if err := VerifyStatementFingerprint(stmt); err != nil {
		t.Fatalf("fingerprint after freshness: %v", err)
	}
}

func TestApplyFreshness_ZeroTTLIsNoOp(t *testing.T) {
	stmt := freshnessTestReceipt(t)
	before := stmt.Predicate.Fingerprint
	if err := ApplyFreshness(&stmt, 0); err != nil {
		t.Fatalf("ApplyFreshness(0): %v", err)
	}
	if stmt.Predicate.Freshness != nil {
		t.Fatal("zero ttl must not stamp freshness")
	}
	if stmt.Predicate.Fingerprint != before {
		t.Fatal("zero ttl must not change the fingerprint")
	}
}
