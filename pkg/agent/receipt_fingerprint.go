// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// ComputeStatementFingerprint returns the SHA-256 fingerprint of stmt
// over canonical JSON of the full Statement with the
// `predicate.fingerprint` key REMOVED from the JSON shape.
//
// Trust-core requirements per the locked design (#446 round 3) and the
// receipt contract:
//
//   - Hash covers the full Statement — `_type`, `subject`, `predicateType`,
//     and every predicate field except `fingerprint`. Hashing the predicate
//     body alone would leave the in-toto envelope unprotected (a tamperer
//     could swap subjects without changing the digest).
//   - The `fingerprint` key is REMOVED from the JSON shape before hashing,
//     not zeroed in place. An external verifier following the contract
//     prose would otherwise compute a different digest than the emitter
//     that hashed `"fingerprint":""`. See Codex #446 round-4 review.
//
// Output format: "sha256:<64 hex chars>".
func ComputeStatementFingerprint(stmt Statement) (string, error) {
	canon, err := canonicalStatementWithoutFingerprint(stmt)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canon)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// canonicalStatementWithoutFingerprint marshals stmt, parses it into a
// generic map, deletes the `predicate.fingerprint` key, and returns
// canonical JSON of the result. Centralizing the delete-not-zero shape
// here keeps every fingerprint call site honest.
func canonicalStatementWithoutFingerprint(stmt Statement) ([]byte, error) {
	raw, err := json.Marshal(stmt)
	if err != nil {
		return nil, fmt.Errorf("fingerprint: marshal statement: %w", err)
	}
	var generic map[string]interface{}
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, fmt.Errorf("fingerprint: re-parse statement: %w", err)
	}
	pred, ok := generic["predicate"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("fingerprint: predicate is not a JSON object")
	}
	delete(pred, "fingerprint")

	canon, err := CanonicalJSON(generic)
	if err != nil {
		return nil, fmt.Errorf("fingerprint: canonical-json: %w", err)
	}
	return canon, nil
}

// VerifyStatementFingerprint recomputes the fingerprint of stmt (with
// the fingerprint key removed from the hashed JSON shape) and compares
// against stmt.Predicate.Fingerprint. Returns nil on match; an error
// describing both values on mismatch.
//
// Any change to `_type`, `subject`, `predicateType`, or any predicate
// field other than `fingerprint` must change the computed value.
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
	stmt.Predicate.Fingerprint = "" // ensure no stale value influences the compute
	fp, err := ComputeStatementFingerprint(*stmt)
	if err != nil {
		return err
	}
	stmt.Predicate.Fingerprint = fp
	return nil
}
