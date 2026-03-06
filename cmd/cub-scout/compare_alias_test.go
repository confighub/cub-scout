package main

import (
	"strings"
	"testing"
)

func TestCompareAliasRegisteredOnCombined(t *testing.T) {
	if combinedCmd == nil {
		t.Fatal("combinedCmd is nil")
	}

	found := false
	for _, alias := range combinedCmd.Aliases {
		if alias == "compare" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected combined command to expose \"compare\" alias")
	}
}

func TestCompareAliasHelp(t *testing.T) {
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"compare", "--help"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("compare --help returned error: %v", err)
		}
	})

	if !strings.Contains(out, "combined") {
		t.Fatalf("expected compare help output to route through combined command, got:\n%s", out)
	}
}
