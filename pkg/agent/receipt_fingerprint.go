// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// ComputeStatementFingerprint returns the SHA-256 fingerprint of stmt over
// canonical JSON of the full Statement with the predicate's Fingerprint
// field zeroed.
//
// Why the full Statement: hashing only the predicate body would leave the
// in-toto envelope fields (_type, subject, predicateType) unprotected — a
// tamperer could swap subjects without changing the fingerprint. Hashing
// the full Statement closes that. See the round-1 external review of #446
// for the original finding.
//
// Why zeroing the Fingerprint field rather than excluding it: the
// fingerprint occupies one field of the predicate; zeroing it is
// deterministic and produces the same canonical bytes regardless of which
// fingerprint value the caller had stamped before recomputation. The
// receipt-validate command re-computes against the same zeroed shape.
//
// Output format: "sha256:<64 hex chars>".
func ComputeStatementFingerprint(stmt Statement) (string, error) {
	// Clone via JSON round-trip so we don't mutate the caller's Statement.
	cloneRaw, err := json.Marshal(stmt)
	if err != nil {
		return "", fmt.Errorf("fingerprint: clone marshal: %w", err)
	}
	var clone Statement
	if err := json.Unmarshal(cloneRaw, &clone); err != nil {
		return "", fmt.Errorf("fingerprint: clone unmarshal: %w", err)
	}
	clone.Predicate.Fingerprint = ""

	canon, err := CanonicalJSON(clone)
	if err != nil {
		return "", fmt.Errorf("fingerprint: canonical-json: %w", err)
	}

	sum := sha256.Sum256(canon)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// VerifyStatementFingerprint recomputes the fingerprint of stmt with the
// caller-supplied fingerprint zeroed, compares against stmt.Predicate.
// Fingerprint, and returns nil on match. On mismatch, returns an error
// describing both values.
//
// This is the function `cub-scout receipt validate` calls to detect
// tampering. The contract: any change to _type, subject, predicateType,
// or any predicate field other than Fingerprint must change the computed
// value.
func VerifyStatementFingerprint(stmt Statement) error {
	got, err := ComputeStatementFingerprint(stmt)
	if err != nil {
		return fmt.Errorf("verify-fingerprint: %w", err)
	}
	if got != stmt.Predicate.Fingerprint {
		return fmt.Errorf("verify-fingerprint: mismatch (recomputed=%s; receipt=%s)", got, stmt.Predicate.Fingerprint)
	}
	return nil
}

// StampFingerprint computes and writes the fingerprint into the
// predicate. The Predicate.Fingerprint field is set in place; the
// caller's Statement is mutated.
func StampFingerprint(stmt *Statement) error {
	if stmt == nil {
		return fmt.Errorf("stamp-fingerprint: nil statement")
	}
	stmt.Predicate.Fingerprint = "" // ensure zeroed before computing
	fp, err := ComputeStatementFingerprint(*stmt)
	if err != nil {
		return err
	}
	stmt.Predicate.Fingerprint = fp
	return nil
}
