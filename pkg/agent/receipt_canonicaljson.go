// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"encoding/json"
	"fmt"

	"github.com/gowebpki/jcs"
)

// CanonicalJSON returns the RFC 8785 (JSON Canonicalization Scheme)
// serialization of v.
//
// Algorithm:
//  1. Marshal v with the stdlib `encoding/json` to produce well-formed
//     JSON respecting struct tags and zero-value rules.
//  2. Run the bytes through `gowebpki/jcs.Transform`, which is the
//     reference Go implementation of RFC 8785. This handles every
//     requirement the stdlib encoder skips:
//     - lexicographic UTF-16 code-unit ordering of object keys
//     - canonical number formatting per ECMAScript ToString
//     - canonical string escaping (no `\u00XX` for printables, etc.)
//     - NFC normalization is the caller's responsibility per RFC 8785 §3.3
//
// Why a real JCS library rather than `encoding/json` round-trip alone:
// the fingerprint is a public trust artifact. Third-party verifiers
// implementing receipts independently must compute the same digest from
// the same Statement bytes; that requires bit-exact RFC 8785 conformance,
// not Go-specific JSON formatting heuristics. The first batch of this
// package used a round-trip approach and was correctly flagged by Codex
// as "necessary but not sufficient" for the locked design.
//
// Receipt content has no `NaN` / `Infinity` / non-NFC strings in v1 —
// resource names, controller managers, git paths, and SHA values are
// already ASCII or NFC. RFC 8785 explicitly rejects non-finite numbers,
// which is consistent with our policy.
func CanonicalJSON(v interface{}) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("canonical-json: initial marshal: %w", err)
	}
	canon, err := jcs.Transform(raw)
	if err != nil {
		return nil, fmt.Errorf("canonical-json: rfc-8785 transform: %w", err)
	}
	return canon, nil
}
