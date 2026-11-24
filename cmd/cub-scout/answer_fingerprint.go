// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/confighub/cub-scout/v3/internal/mapsvc"
)

const answerFingerprintHexLen = 16

// unchangedEnvelope is the compact JSON document emitted when --if-unchanged
// matches the freshly computed answer fingerprint.
type unchangedEnvelope struct {
	Unchanged   bool   `json:"unchanged"`
	Fingerprint string `json:"fingerprint"`
	ObservedAt  string `json:"observedAt"`
}

// computeAnswerFingerprint returns a deterministic 16-hex-character SHA-256
// prefix of the canonical JSON representation of answer, excluding the
// per-call volatile fields ObservedAt and Cost.
func computeAnswerFingerprint(answer *mapsvc.Answer) string {
	if answer == nil {
		return ""
	}
	canonical := *answer
	canonical.ObservedAt = ""
	canonical.Cost = mapsvc.AnswerCost{}
	raw, err := json.Marshal(canonical)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])[:answerFingerprintHexLen]
}

// matchesIfUnchanged reports whether a caller-supplied --if-unchanged hash
// matches the freshly computed answer fingerprint (case-insensitive hex).
func matchesIfUnchanged(ifUnchanged, fingerprint string) bool {
	want := strings.ToLower(strings.TrimSpace(ifUnchanged))
	return want != "" && fingerprint != "" && want == fingerprint
}

// writeUnchangedEnvelope writes the compact unchanged JSON envelope to w.
func writeUnchangedEnvelope(w io.Writer, fingerprint, observedAt string) error {
	if strings.TrimSpace(observedAt) == "" {
		observedAt = time.Now().UTC().Format(time.RFC3339)
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(unchangedEnvelope{
		Unchanged:   true,
		Fingerprint: fingerprint,
		ObservedAt:  observedAt,
	})
}
