// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
)

func TestImportCommandHasAuditReasonFlag(t *testing.T) {
	flag := importCmd.Flags().Lookup("audit-reason")
	if flag == nil {
		t.Fatal("expected --audit-reason flag to be registered on import command")
	}
}

func TestNormalizeImportAuditReason(t *testing.T) {
	got, err := normalizeImportAuditReason("  approved by SRE lead for Q1 migration  ")
	if err != nil {
		t.Fatalf("normalizeImportAuditReason() error = %v", err)
	}
	if got != "approved by SRE lead for Q1 migration" {
		t.Fatalf("normalized reason = %q", got)
	}
}

func TestNormalizeImportAuditReason_RejectsTooLong(t *testing.T) {
	_, err := normalizeImportAuditReason(strings.Repeat("a", 513))
	if err == nil {
		t.Fatal("expected length validation error")
	}
	if !strings.Contains(err.Error(), "512") {
		t.Fatalf("unexpected error: %v", err)
	}
}
