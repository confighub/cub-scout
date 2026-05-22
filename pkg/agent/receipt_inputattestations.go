// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"fmt"
	"strings"
)

// receipt_inputattestations.go — helpers for the chained-receipts half
// of #446 v2 (#448). A receipt can reference earlier receipts by digest
// via the `predicate.inputAttestations[]` field, forming a chain or DAG
// of evidence artifacts.
//
// This file implements the CONSTRUCTION primitives — turning a loaded
// receipt into an AttestationRef the caller can attach to a new
// BuildReceiptInput. The full aggregate flow (auto-discovery across
// namespace / cluster + verdict synthesis + synthetic-aggregate://
// subject construction) is the second half of #448 and will land in a
// follow-up PR.
//
// URI scheme
// ----------
//
// The `AttestationRef.URI` for a receipt-to-receipt reference uses the
// scheme `cub-scout-receipt://<short-fingerprint>` where short-fingerprint
// is the first 12 hex chars after the `sha256:` prefix of the
// referenced receipt's fingerprint. The short form keeps the URI
// readable in JSON output; the full integrity check lives in the
// `digest` field on AttestationRef itself (which carries the complete
// SHA-256). This is symmetric with how the local store's canonical
// filename works (see `pkg/agent/receipt_store.go` DeriveFilename).
//
// Verification semantics
// ----------------------
//
// `BuildAttestationRef` always verifies the referenced receipt's
// fingerprint by recomputing it from the in-memory Statement. This
// catches corruption and tampering at chain-construction time —
// downstream consumers can re-verify each `inputAttestations[]` entry
// by loading the referenced receipt (the URI is just a label; the
// digest is what's actually cryptographically meaningful).
//
// A future Codex review may want to also verify the chain at *load*
// time (i.e., when reading an aggregate receipt, walk every input
// attestation and confirm the digests match). That's a separate
// helper; this file is only about construction.

// AttestationURIScheme is the URI prefix this package uses for
// receipt-to-receipt references. Format:
//
//	cub-scout-receipt://<short-fingerprint>
//
// The short fingerprint is the first 12 hex chars after the `sha256:`
// prefix. Symmetric with `DeriveFilename`.
const AttestationURIScheme = "cub-scout-receipt://"

// BuildAttestationRef turns a loaded Statement into an AttestationRef
// suitable for the `inputAttestations[]` field of another receipt.
//
// The returned AttestationRef carries:
//   - URI: cub-scout-receipt://<short-fingerprint>
//   - Digest: { "sha256": "<full hex without sha256: prefix>" }
//
// The function verifies the referenced statement's fingerprint
// (recomputes via VerifyStatementFingerprint) before returning. If the
// recomputed digest doesn't match the stamped one, the function
// returns an error — the caller should NOT silently chain a tampered
// receipt.
//
// Errors:
//   - stmt has empty Predicate.Fingerprint
//   - stmt.Predicate.Fingerprint is malformed (no sha256: prefix)
//   - VerifyStatementFingerprint fails (the in-memory Statement does
//     not match its stamped fingerprint)
func BuildAttestationRef(stmt Statement) (AttestationRef, error) {
	fp := strings.TrimSpace(stmt.Predicate.Fingerprint)
	if fp == "" {
		return AttestationRef{}, fmt.Errorf("build-attestation-ref: statement has empty fingerprint; cannot chain an unstamped receipt")
	}
	if !strings.HasPrefix(fp, "sha256:") {
		return AttestationRef{}, fmt.Errorf("build-attestation-ref: malformed fingerprint %q (expected sha256:<hex>)", fp)
	}
	hex := strings.TrimPrefix(fp, "sha256:")
	if len(hex) < 12 {
		return AttestationRef{}, fmt.Errorf("build-attestation-ref: fingerprint hex %q is shorter than 12 chars", hex)
	}

	// Verify the referenced receipt's integrity before chaining it.
	// Silently chaining a tampered receipt would break the chain's
	// trust property — the consumer of an aggregate / chained receipt
	// expects every reference to be a real attestation, not a forgery.
	if err := VerifyStatementFingerprint(stmt); err != nil {
		return AttestationRef{}, fmt.Errorf("build-attestation-ref: refusing to chain a receipt whose fingerprint doesn't verify: %w", err)
	}

	return AttestationRef{
		URI: AttestationURIScheme + hex[:12],
		Digest: map[string]string{
			"sha256": hex,
		},
	}, nil
}

// BuildAttestationRefsFromPaths is a convenience helper for the CLI
// path: take a list of receipt file paths, load each, build the
// AttestationRef, and return the slice. Errors at any step short-
// circuit — there is no "partial chain" semantics (chaining is an
// integrity claim; partial is not the right shape).
//
// The loader is injected as `loadFn` so tests can swap in a fake
// without touching the filesystem.
func BuildAttestationRefsFromPaths(paths []string, loadFn func(path string) (Statement, error)) ([]AttestationRef, error) {
	if loadFn == nil {
		loadFn = LoadStatement
	}
	out := make([]AttestationRef, 0, len(paths))
	for _, p := range paths {
		stmt, err := loadFn(p)
		if err != nil {
			return nil, fmt.Errorf("load input-attestation %s: %w", p, err)
		}
		ref, err := BuildAttestationRef(stmt)
		if err != nil {
			return nil, fmt.Errorf("build input-attestation from %s: %w", p, err)
		}
		out = append(out, ref)
	}
	return out, nil
}
