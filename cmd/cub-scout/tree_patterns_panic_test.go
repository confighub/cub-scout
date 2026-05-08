// Regression coverage for #390: `cub-scout tree patterns` panicked at
// klog.FromContext because runTreePatterns() bypassed Cobra's Execute path
// and never seeded mapPatternsCmd's context, so client-go received a nil
// context and dereferenced it.
//
// Two defenses exist now:
//   - runTreePatterns(ctx) seeds mapPatternsCmd.Context() before delegating.
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

// TestRunTreePatterns_PropagatesContext verifies the primary fix: the wrapper
// must call mapPatternsCmd.SetContext(ctx) before invoking runMapPatterns, so
// that cmd.Context() inside runMapPatterns returns the caller's context, not
// nil.
func TestRunTreePatterns_PropagatesContext(t *testing.T) {
	type ctxKey struct{}
	want := "tree-patterns-#390-marker"
	ctx := context.WithValue(context.Background(), ctxKey{}, want)

	// We do not care whether runTreePatterns succeeds; in a unit-test
	// environment buildConfig() typically fails because there is no live
	// kubeconfig pointing at a cluster. What we care about is that
	// mapPatternsCmd's context is the one we passed in afterwards — proving
	// runTreePatterns did the seeding before delegating.
	_ = runTreePatterns(ctx)

	got := mapPatternsCmd.Context()
	if got == nil {
		t.Fatal("mapPatternsCmd.Context() is nil after runTreePatterns; #390 regression")
	}
	if v := got.Value(ctxKey{}); v != want {
		t.Fatalf("ctx not propagated: want %q, got %v", want, v)
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
