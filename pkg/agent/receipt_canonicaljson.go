// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// CanonicalJSON returns the RFC 8785-style canonical JSON serialization of v.
//
// The algorithm:
//  1. Marshal v with the stdlib json package (handles struct json tags +
//     value formatting).
//  2. Re-parse into a generic interface{} tree. This converts the struct
//     into nested map[string]interface{} for objects.
//  3. Re-marshal. Go's encoding/json sorts map keys lexicographically and
//     produces compact output with no extra whitespace, matching the
//     core RFC 8785 requirements for our well-controlled data shape.
//
// Scope and known limitations:
//  - Receipt data has no float64 values that round-trip lossy through
//    JSON (timestamps are strings, counts are integers). The stdlib
//    json package's number formatting matches RFC 8785 for integers and
//    for the limited float cases we encounter.
//  - Receipt data has no NaN / Infinity / lone surrogates. Stdlib json
//    will reject these on encode anyway.
//  - Unicode normalization (NFC) is NOT performed. RFC 8785 expects
//    strings to be NFC-normalized; our receipt content is operator-
//    generated strings (resource names, paths, manager strings) which
//    are conventionally ASCII or NFC already.
//
// For v2 signing where third-party verifiers will recompute this hash,
// the conformance test suite in receipt_canonicaljson_test.go locks
// behavior against RFC 8785 reference vectors. Adding a vector that
// fails means we need a fuller RFC 8785 implementation (likely
// github.com/gowebpki/jcs); until that test fails, this lightweight
// implementation is sufficient.
func CanonicalJSON(v interface{}) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("canonical-json: initial marshal: %w", err)
	}
	var generic interface{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber() // preserve numeric precision across round-trips
	if err := decoder.Decode(&generic); err != nil {
		return nil, fmt.Errorf("canonical-json: re-parse: %w", err)
	}
	// Re-marshal. encoding/json sorts map keys; compact output by default.
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false) // RFC 8785 does not require HTML escaping
	if err := encoder.Encode(generic); err != nil {
		return nil, fmt.Errorf("canonical-json: final marshal: %w", err)
	}
	// json.Encoder.Encode appends a trailing newline; strip it for
	// canonical-bytes purposes.
	out := buf.Bytes()
	if n := len(out); n > 0 && out[n-1] == '\n' {
		out = out[:n-1]
	}
	return out, nil
}
