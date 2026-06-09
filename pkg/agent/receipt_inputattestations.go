// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
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
//
// API-boundary enforcement (#463 / Codex round-6 P1 #463-B)
// ---------------------------------------------------------
//
// `VerifiedAttestationRef` is a typed wrapper around `AttestationRef`
// with only unexported fields. External callers cannot construct one
// directly — they MUST go through `BuildAttestationRef` or
// `BuildAttestationRefsFromPaths`, both of which verify the source
// Statement's fingerprint before producing a wrapper. BuildReceipt
// accepts `[]VerifiedAttestationRef` and rejects zero-valued wrappers
// at the boundary, so a programmatic caller cannot forge a raw ref and
// bypass the verify step. The wire shape (Predicate.InputAttestations
// is still []AttestationRef) is unchanged.

// AttestationURIScheme is the URI prefix this package uses for
// receipt-to-receipt references. Format:
//
//	cub-scout-receipt://<short-fingerprint>
//
// The short fingerprint is the first 12 hex chars after the `sha256:`
// prefix. Symmetric with `DeriveFilename`.
const AttestationURIScheme = "cub-scout-receipt://"

// VerifiedAttestationRef wraps an AttestationRef whose source Statement
// was verified at construction time. Only `BuildAttestationRef` and
// `BuildAttestationRefsFromPaths` produce values of this type; both
// recompute and verify the source Statement's fingerprint before
// returning. The zero value is invalid and `BuildReceipt` rejects it.
//
// This type enforces the chained-receipt trust property at the API
// boundary: a programmatic caller of `BuildReceipt` CANNOT construct a
// raw `AttestationRef`, hand-pick a digest, and have BuildReceipt
// accept it. The only path into the chain is through the verify-and-
// wrap helpers below. See #463 / Codex round-6 P1 #463-B for the
// rationale.
//
// The wire shape is unchanged: BuildReceipt unwraps each
// VerifiedAttestationRef back to a plain AttestationRef when it
// populates Predicate.InputAttestations, so the serialized receipt
// looks exactly the same as the v1 spec.
type VerifiedAttestationRef struct {
	// ref is the plain AttestationRef that will be serialized. It is
	// unexported so external packages cannot populate it — they must
	// call BuildAttestationRef (which sets it after a verify step).
	ref AttestationRef
}

// Ref returns the underlying AttestationRef. Provided for inspection
// and debugging; the typical caller passes the wrapper directly to
// BuildReceipt and never needs this.
func (v VerifiedAttestationRef) Ref() AttestationRef {
	return v.ref
}

// IsZero reports whether the wrapper is the zero value (the only way
// external code could obtain a VerifiedAttestationRef without going
// through BuildAttestationRef). BuildReceipt calls this to reject
// forged wrappers at the API boundary.
func (v VerifiedAttestationRef) IsZero() bool {
	return v.ref.URI == "" || v.ref.Digest["sha256"] == ""
}

// BuildAttestationRef turns a loaded Statement into a
// VerifiedAttestationRef suitable for the `InputAttestations` field of
// BuildReceiptInput.
//
// The wrapped AttestationRef carries:
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
func BuildAttestationRef(stmt Statement) (VerifiedAttestationRef, error) {
	fp := strings.TrimSpace(stmt.Predicate.Fingerprint)
	if fp == "" {
		return VerifiedAttestationRef{}, fmt.Errorf("build-attestation-ref: statement has empty fingerprint; cannot chain an unstamped receipt")
	}
	if !strings.HasPrefix(fp, "sha256:") {
		return VerifiedAttestationRef{}, fmt.Errorf("build-attestation-ref: malformed fingerprint %q (expected sha256:<hex>)", fp)
	}
	hex := strings.TrimPrefix(fp, "sha256:")
	if len(hex) < 12 {
		return VerifiedAttestationRef{}, fmt.Errorf("build-attestation-ref: fingerprint hex %q is shorter than 12 chars", hex)
	}

	// Verify the referenced receipt's integrity before chaining it.
	// Silently chaining a tampered receipt would break the chain's
	// trust property — the consumer of an aggregate / chained receipt
	// expects every reference to be a real attestation, not a forgery.
	if err := VerifyStatementFingerprint(stmt); err != nil {
		return VerifiedAttestationRef{}, fmt.Errorf("build-attestation-ref: refusing to chain a receipt whose fingerprint doesn't verify: %w", err)
	}

	return VerifiedAttestationRef{
		ref: AttestationRef{
			URI: AttestationURIScheme + hex[:12],
			Digest: map[string]string{
				"sha256": hex,
			},
		},
	}, nil
}

// BuildAttestationRefsFromPaths is a convenience helper for the CLI
// path: take a list of receipt file paths, load each, build the
// VerifiedAttestationRef, and return the slice. Errors at any step
// short-circuit — there is no "partial chain" semantics (chaining is
// an integrity claim; partial is not the right shape).
//
// The loader is injected as `loadFn` so tests can swap in a fake
// without touching the filesystem.
func BuildAttestationRefsFromPaths(paths []string, loadFn func(path string) (Statement, error)) ([]VerifiedAttestationRef, error) {
	if loadFn == nil {
		loadFn = LoadStatement
	}
	out := make([]VerifiedAttestationRef, 0, len(paths))
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

// ExternalEvidenceURIScheme is the URI prefix for references to EXTERNAL
// (non-cub-scout) evidence artifacts chained by content digest (#482). Format:
//
//	external-evidence://sha256/<short-digest>
//
// The distinct scheme is the whole point: a consumer reading
// inputAttestations[] can tell a fingerprint-verified cub-scout receipt
// (cub-scout-receipt://) apart from a digest-asserted external artifact
// (external-evidence://). External refs carry a WEAKER trust basis — cub-scout
// vouches only that the referenced bytes hashed to this SHA-256, not that the
// artifact is a valid cub-scout receipt. This is what lets an upstream
// producer's artifact (e.g. helm-expt's installer-package receipt) enter a
// cub-scout receipt's chain without cub-scout having to understand its format.
const ExternalEvidenceURIScheme = "external-evidence://"

// BuildExternalAttestationRef references an external evidence artifact by the
// SHA-256 of its raw bytes. Unlike BuildAttestationRef there is no cub-scout
// fingerprint to verify — the trust basis is the content digest, recorded
// under the external-evidence:// scheme (see ExternalEvidenceURIScheme).
func BuildExternalAttestationRef(content []byte) (VerifiedAttestationRef, error) {
	if len(content) == 0 {
		return VerifiedAttestationRef{}, fmt.Errorf("build-external-attestation-ref: empty content; cannot reference an empty artifact")
	}
	sum := sha256.Sum256(content)
	hexDigest := hex.EncodeToString(sum[:])
	return VerifiedAttestationRef{
		ref: AttestationRef{
			URI: ExternalEvidenceURIScheme + "sha256/" + hexDigest[:12],
			Digest: map[string]string{
				"sha256": hexDigest,
			},
		},
	}, nil
}

// BuildExternalAttestationRefsFromPaths reads each path and builds an external
// (digest-asserted) attestation ref. The reader is injectable for tests.
// Errors short-circuit — there is no partial-chain semantics.
func BuildExternalAttestationRefsFromPaths(paths []string, readFn func(path string) ([]byte, error)) ([]VerifiedAttestationRef, error) {
	if readFn == nil {
		readFn = os.ReadFile
	}
	out := make([]VerifiedAttestationRef, 0, len(paths))
	for _, p := range paths {
		content, err := readFn(p)
		if err != nil {
			return nil, fmt.Errorf("read reference-evidence %s: %w", p, err)
		}
		ref, err := BuildExternalAttestationRef(content)
		if err != nil {
			return nil, fmt.Errorf("build reference-evidence from %s: %w", p, err)
		}
		out = append(out, ref)
	}
	return out, nil
}
