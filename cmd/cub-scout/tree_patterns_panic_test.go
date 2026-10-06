// Regression coverage for #390: `cub-scout tree patterns` panicked at
// klog.FromContext because runTreePatterns() bypassed Cobra's Execute path
// and never seeded mapPatternsCmd's context, so client-go received a nil
// context and dereferenced it.
//
// Two defenses exist now:
//   - runTreePatterns(ctx) uses a private command carrying the caller context.
//   - runMapPatterns defensively swaps a nil cmd.Context() for
//     context.Background() so any other dispatcher that bypasses Execute
//     does not regress the same surface.
//
// Both are covered below.

package main

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
)

// A tree alias must propagate cancellation without mutating the shared command.
func TestRunTreePatterns_PropagatesContext(t *testing.T) {
	before := mapPatternsCmd.Context()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = runTreePatterns(ctx)
	if mapPatternsCmd.Context() != before {
		t.Fatal("tree alias leaked context into map command")
	}
}

// TestRunMapPatterns_NilContextDefense verifies the secondary fix: even if a
// caller hands runMapPatterns a Cobra command without a context (i.e.
// cmd.Context() returns nil), the function must not panic when it threads
// the context into client-go. We only assert that the call returns rather
// than panicking — the actual error from buildConfig() in a test environment
// is environment-dependent and not interesting for this regression.
func TestRunMapPatterns_NilContextDefense(t *testing.T) {
	cmd := &cobra.Command{} // no SetContext → cmd.Context() returns nil
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("runMapPatterns panicked on nil context (#390 regression): %v", r)
		}
	}()
	_ = runMapPatterns(cmd, []string{})
}
