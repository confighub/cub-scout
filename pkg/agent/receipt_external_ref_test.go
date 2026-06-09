// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestBuildExternalAttestationRef_DigestAndScheme(t *testing.T) {
	content := []byte(`{"kind":"InstallerPackageReceipt","renderedObjectSetSHA256":"abc"}`)
	ref, err := BuildExternalAttestationRef(content)
	if err != nil {
		t.Fatalf("BuildExternalAttestationRef: %v", err)
	}
	sum := sha256.Sum256(content)
	want := hex.EncodeToString(sum[:])
	if got := ref.Ref().Digest["sha256"]; got != want {
		t.Fatalf("digest = %s, want %s", got, want)
	}
	if !strings.HasPrefix(ref.Ref().URI, ExternalEvidenceURIScheme) {
		t.Fatalf("URI %q lacks the external-evidence:// scheme", ref.Ref().URI)
	}
	// Must be distinguishable from a fingerprint-verified cub-scout receipt ref.
	if strings.HasPrefix(ref.Ref().URI, AttestationURIScheme) {
		t.Fatalf("external ref must NOT use the cub-scout-receipt:// scheme")
	}
	if ref.IsZero() {
		t.Fatal("external ref must be non-zero (usable by BuildReceipt)")
	}
}

func TestBuildExternalAttestationRef_EmptyErrors(t *testing.T) {
	if _, err := BuildExternalAttestationRef(nil); err == nil {
		t.Fatal("empty content must error")
	}
}

func TestBuildExternalAttestationRefsFromPaths_FakeReader(t *testing.T) {
	refs, err := BuildExternalAttestationRefsFromPaths([]string{"a", "b"}, func(p string) ([]byte, error) {
		return []byte("content-of-" + p), nil
	})
	if err != nil {
		t.Fatalf("BuildExternalAttestationRefsFromPaths: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("got %d refs, want 2", len(refs))
	}
	// Distinct content -> distinct digests.
	if refs[0].Ref().Digest["sha256"] == refs[1].Ref().Digest["sha256"] {
		t.Fatal("distinct content must yield distinct digests")
	}
}
