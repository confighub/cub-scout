// Package agent — source-truth decision logic.
//
// Derive turns observed surfaces + a declared strategy into a structured
// SourceTruthEvidence value. The function is intentionally pure: no
// network, no clients, no logging. Callers populate Surfaces from
// whatever sources they want (live tracer, fixture, mock) and Derive
// produces the verdict.
//
// v0.1 scope: presence-based gap detection plus the strategy-relative
// correctness check. Cross-surface revision *equality* (does the OCI
// digest the controller pulled match the digest the runtime is running?)
// is intentionally deferred to v0.2 — heterogeneous version identifiers
// (ConfigHub revision number vs. Git SHA vs. OCI digest vs. image tag)
// require normalisation work that the producer fixture suite (#395) will
// drive. v0.1 emits MISMATCH only when the strategy contract itself is
// violated, plus WATCH when a soft proof gap is present. Pilot can layer
// stricter equality on top once the contract shape is stable.

package agent

import (
	"fmt"
	"strings"
)

// Derive builds the structured SourceTruthEvidence value from a declared
// strategy and the surfaces the caller was able to collect. It enforces:
//
//   - Strict missing-proof rule: a missing critical field never produces
//     PASS. The minimum verdict is WATCH.
//   - Strategy-relative correctness: under a confighub-oci-* strategy, the
//     controller must observe an OCI source. Git observation fails BLOCK
//     with controller as the outlier.
//   - Empty-strategy handling: empty/unknown strategy short-circuits to
//     ASK + UNKNOWN with a single proof gap "declared_strategy".
//
// The function is total: every input combination produces a
// SourceTruthEvidence; nil inputs are surfaces' caller responsibility.
func Derive(strategy SourceTruthStrategy, surfaces SourceTruthSurfaces) SourceTruthEvidence {
	ev := SourceTruthEvidence{
		DeclaredStrategy: strategy.Human(),
		Surfaces:         surfaces,
		Outlier:          OutlierUnknown,
		ProofGaps:        []string{},
	}

	// Empty / unknown strategy: cannot classify; refuse to guess.
	if strategy == "" {
		ev.DeclaredStrategy = ""
		ev.Status = StatusASK
		ev.SourceTruth = VerdictUNKNOWN
		ev.ProofGaps = append(ev.ProofGaps, "declared_strategy")
		ev.SafeNextAction = "Declare a delivery strategy with --strategy and re-run; cub-scout does not infer the strategy."
		return ev
	}

	// Collect proof gaps (presence-based). Each gap is a stable string key
	// Pilot can pattern-match.
	gaps := collectProofGaps(surfaces)
	ev.ProofGaps = gaps

	// Hard rule: a fundamentally unfetchable surface (caller passed nil)
	// is a BLOCK. The CLI converts read-time errors into nil surfaces +
	// a CollectionError that propagates as a BLOCK SafeNextAction.
	if surfaces.ConfigHub == nil || surfaces.Controller == nil || surfaces.Runtime == nil {
		ev.Status = StatusBLOCK
		ev.SourceTruth = VerdictBLOCKED
		ev.SafeNextAction = blockedSafeNextAction(surfaces)
		return ev
	}

	// Strategy-relative correctness: does the controller observe a source
	// shape consistent with the declared strategy?
	if mismatch, reason := checkStrategySource(strategy, surfaces.Controller); mismatch {
		ev.Status = StatusBLOCK
		ev.SourceTruth = VerdictMISMATCH
		ev.Outlier = OutlierController
		ev.SafeNextAction = "Read-only diagnostic: " + reason
		return ev
	}

	// Cross-surface equality: deferred to v0.2 (see file header). v0.1
	// only flags soft mismatches via the proof_gap mechanism.

	// Verdict synthesis based on proof gaps.
	if len(gaps) > 0 {
		ev.Status = StatusWATCH
		ev.SourceTruth = VerdictINCOMPLETE
		ev.Outlier = OutlierUnknown
		ev.SafeNextAction = fmt.Sprintf(
			"Read-only diagnostic: confirm the missing fields (%s) before acceptance.",
			strings.Join(gaps, ", "),
		)
		return ev
	}

	// All surfaces present, strategy contract holds, no soft gaps.
	ev.Status = StatusPASS
	ev.SourceTruth = VerdictAGREED
	ev.Outlier = OutlierUnknown
	ev.SafeNextAction = ""
	return ev
}

// collectProofGaps returns the stable proof-gap keys the council's
// fixture rule depends on. Order is deterministic so JSON diffs are stable.
func collectProofGaps(s SourceTruthSurfaces) []string {
	var gaps []string

	if s.ConfigHub != nil {
		if strings.TrimSpace(s.ConfigHub.Revision) == "" {
			gaps = append(gaps, "confighub.revision")
		}
		if strings.TrimSpace(s.ConfigHub.Unit) == "" {
			gaps = append(gaps, "confighub.unit")
		}
	}

	if s.Controller != nil {
		if strings.TrimSpace(s.Controller.Source) == "" {
			gaps = append(gaps, "controller.source")
		}
		if strings.TrimSpace(s.Controller.RevisionOrDigest) == "" {
			gaps = append(gaps, "controller.revision_or_digest")
		}
	}

	if s.Runtime != nil {
		if strings.TrimSpace(s.Runtime.Value) == "" {
			gaps = append(gaps, "runtime.value")
		}
	}

	return gaps
}

// checkStrategySource enforces the canonical council rule: under a
// confighub-oci-* strategy, the controller must read from a ConfigHub-
// rendered OCI artifact. Reading directly from Git is a strategy
// violation. Git-strategies pose no controller-source constraint at this
// layer — Git is the expected source.
//
// Returns (true, reason) when a violation is detected.
func checkStrategySource(strategy SourceTruthStrategy, ctrl *ControllerSurface) (bool, string) {
	if !strategy.expectsConfigHubOCISource() {
		return false, ""
	}
	if ctrl == nil {
		return false, ""
	}

	source := strings.TrimSpace(ctrl.Source)
	kind := strings.TrimSpace(ctrl.Kind)

	if source == "" {
		// No source observed. That is a soft gap, handled by
		// collectProofGaps — not a hard MISMATCH.
		return false, ""
	}

	// Positive markers for OCI source: scheme prefix or canonical Kind
	// label set by the existing tracers (see pkg/agent/flux_trace.go and
	// argo_trace.go where OCI sources are tagged "ConfigHub OCI").
	if strings.HasPrefix(source, "oci://") {
		return false, ""
	}
	if strings.Contains(strings.ToLower(kind), "oci") {
		return false, ""
	}

	// Negative marker: a Git-shaped URL under an OCI strategy is the
	// canonical trap.
	if isGitURL(source) {
		return true, fmt.Sprintf(
			"strategy %q expects controller source to be ConfigHub OCI, but observed Git source %q",
			strategy.Human(), source,
		)
	}

	// Source is present but unrecognised. Conservative: treat as
	// not-OCI, which is a strategy mismatch under confighub-oci-*.
	return true, fmt.Sprintf(
		"strategy %q expects controller source to be ConfigHub OCI, but observed source %q is not recognised as OCI",
		strategy.Human(), source,
	)
}

// isGitURL recognises common Git source shapes the existing controller
// tracers emit. Conservative: only known-Git markers count.
func isGitURL(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case strings.HasPrefix(s, "git@"):
		return true
	case strings.HasPrefix(s, "https://github.com/"),
		strings.HasPrefix(s, "https://gitlab.com/"),
		strings.HasPrefix(s, "https://bitbucket.org/"):
		return true
	case strings.HasSuffix(s, ".git"):
		return true
	}
	return false
}

// blockedSafeNextAction renders a stable, read-only diagnostic when one
// of the surfaces could not be fetched. Names the missing surface so the
// operator knows which auth/CLI/connectivity to fix.
func blockedSafeNextAction(s SourceTruthSurfaces) string {
	missing := []string{}
	if s.ConfigHub == nil {
		missing = append(missing, "ConfigHub (verify `cub` auth and connectivity)")
	}
	if s.Controller == nil {
		missing = append(missing, "controller (verify Argo CD / Flux CLI is installed and authenticated)")
	}
	if s.Runtime == nil {
		missing = append(missing, "runtime (verify kubeconfig points at a reachable cluster)")
	}
	if len(missing) == 0 {
		return ""
	}
	return "Read-only diagnostic: cannot fetch " + strings.Join(missing, "; ") + ". Resolve the auth/CLI gap and re-run."
}
