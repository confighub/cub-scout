package main

import (
	"strings"
	"testing"
)

func TestCombinedAliasRegisteredOnCompare(t *testing.T) {
	if combinedCmd == nil {
		t.Fatal("combinedCmd is nil")
	}

	found := false
	for _, alias := range combinedCmd.Aliases {
		if alias == "combined" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected compare command to expose \"combined\" alias")
	}
}

func TestCompareAliasHelp(t *testing.T) {
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"compare", "--help"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("compare --help returned error: %v", err)
		}
	})

	if !strings.Contains(out, "compare") {
		t.Fatalf("expected compare help output to mention compare command, got:\n%s", out)
	}
}

func TestCombinedLegacyAliasHelp(t *testing.T) {
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"combined", "--help"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("combined --help returned error: %v", err)
		}
	})

	if !strings.Contains(out, "compare") {
		t.Fatalf("expected combined help output to route through compare command, got:\n%s", out)
	}
}
