// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"fmt"
	"time"
)

// Freshness is an immutable, stamped observation-freshness boundary on a
// receipt (#478). It is populated only when the caller passes a TTL (e.g.
// `--ttl`). It does NOT make the receipt mutable: observedAt records when the
// live read happened and expiresAt = observedAt + ttl, both fixed at creation.
// A consumer computes staleness at read time as `now > expiresAt`; the receipt
// itself never changes. This is what lets a workerless ConfigHub tell a fresh
// green from a stale one.
type Freshness struct {
	ObservedAt string `json:"observedAt"`
	ExpiresAt  string `json:"expiresAt"`
	TTL        string `json:"ttl"`
}

// ApplyFreshness stamps freshness onto an already-built receipt and re-stamps
// the fingerprint so it covers the freshness fields. ttl <= 0 is a no-op (the
// receipt keeps no freshness boundary and its existing fingerprint). observedAt
// is taken from the receipt's verifiedAt — the moment cub-scout read the
// cluster — so no separate clock read is needed and the two timestamps cannot
// disagree.
func ApplyFreshness(stmt *Statement, ttl time.Duration) error {
	if stmt == nil || ttl <= 0 {
		return nil
	}
	observedAt, err := time.Parse(time.RFC3339, stmt.Predicate.VerifiedAt)
	if err != nil {
		return fmt.Errorf("apply freshness: parse verifiedAt %q: %w", stmt.Predicate.VerifiedAt, err)
	}
	stmt.Predicate.Freshness = &Freshness{
		ObservedAt: stmt.Predicate.VerifiedAt,
		ExpiresAt:  observedAt.Add(ttl).UTC().Format(time.RFC3339),
		TTL:        ttl.String(),
	}
	if err := StampFingerprint(stmt); err != nil {
		return fmt.Errorf("apply freshness: re-stamp fingerprint: %w", err)
	}
	return nil
}
