// Package agent — source-truth decision logic.
//
// Derive turns observed surfaces + a declared strategy into a structured
// SourceTruthEvidence value. The function is intentionally pure: no
// network, no clients, no logging. Callers populate Surfaces from
// whatever sources they want (live tracer, fixture, mock) and Derive
// produces the verdict.
//
// v0.1 scope: presence-based gap detection plus the strategy-relative
// correctness check.
//
// v0.2 scope (#409): cross-surface revision equality for git-* and
// kustomize-flux strategies (Git SHA anchor) and oci-* non-ConfigHub
// strategies (OCI digest anchor). When the controller and runtime both
// expose the strategy's canonical anchor, Derive asserts equality and
// emits MISMATCH (with controller as the outlier) when they disagree.
// Missing anchors surface as soft proof gaps — incomplete, not PASS.
//
// Phase 2 (#409 enum expansion): added helm-flux, helm-argo,
// kustomize-flux, oci-flux, oci-argo. Helm strategies emit
// "runtime.helm_chart_anchor" until the runtime extractor is wired
// (Helm chart version is already readable from `helm.sh/chart` labels
// via pkg/agent/ownership.go but not yet plumbed into RuntimeSurface).
//
// Still deferred: confighub-oci-* equality. ConfigHub does not yet
// expose a rendered digest per unit revision (verified 2026-05-09), so
// cub-scout cannot honestly assert that the controller pulled *the*
// ConfigHub-rendered artifact (only that *some* digest matches end-to-
// end). Equality for confighub-oci-* will land once the ConfigHub-side
// field exists.

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

	// Cross-surface equality (v0.2, git-* strategies only). OCI-strategy
	// equality is deferred until ConfigHub exposes a rendered digest per
	// unit revision — see file header.
	agreement := compareRevisions(strategy, surfaces)

	if !agreement.Agreed && !agreement.Incomplete {
		// Concrete mismatch detected: controller and runtime both have
		// SHA-shaped anchors and they disagree.
		ev.Status = StatusBLOCK
		ev.SourceTruth = VerdictMISMATCH
		ev.Outlier = agreement.Outlier
		ev.SafeNextAction = fmt.Sprintf(
			"Read-only diagnostic: cross-surface revision equality failed; the %s surface diverges. Confirm which commit each surface observed before acceptance.",
			agreement.Outlier,
		)
		return ev
	}

	if agreement.Incomplete && len(agreement.ProofGaps) > 0 {
		// Soft equality gap (e.g. runtime image has no SHA-bearing tag):
		// add to the gap list so the WATCH/INCOMPLETE branch below picks
		// it up. Order: existing presence gaps first, then equality gaps.
		gaps = append(gaps, agreement.ProofGaps...)
		ev.ProofGaps = gaps
	}

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

// revisionAgreement is the result of cross-surface revision equality.
// Three states: Agreed (anchors present and equal), Mismatch (anchors
// present and disagree — Outlier names the divergent surface), and
// Incomplete (an anchor isn't extractable — ProofGaps explains why).
type revisionAgreement struct {
	Agreed     bool
	Outlier    SourceTruthOutlier
	Incomplete bool
	ProofGaps  []string
}

// compareRevisions performs strategy-aware cross-surface revision
// equality. Strategy dispatch:
//
//   - git-* and kustomize-flux: Git-SHA controller↔runtime equality
//     (kustomize-flux uses the same anchor — the Git commit SHA flows
//     through Flux's Kustomization unchanged)
//   - oci-* (non-ConfigHub): OCI digest controller↔runtime equality
//   - confighub-oci-*: deferred (ConfigHub-side rendered digest field
//     does not exist yet — see file header)
//   - helm-*: emit "runtime.helm_chart_anchor" proof gap. Helm runtime
//     anchor is the chart-version label (`helm.sh/chart`) and is
//     extractable from ownership.go, but RuntimeSurface does not yet
//     carry it. Wired as future work; until then helm-* is honestly
//     INCOMPLETE.
func compareRevisions(strategy SourceTruthStrategy, surfaces SourceTruthSurfaces) revisionAgreement {
	// Multi-source is strategy-agnostic. If the controller declares more
	// than one source and cub-scout only parsed the first, equality
	// across the un-parsed sources is fundamentally unverifiable —
	// regardless of strategy, regardless of whether the parsed source
	// matches the runtime. Surface as an explicit proof gap.
	if surfaces.Controller != nil && surfaces.Controller.MultiSource {
		return revisionAgreement{
			Incomplete: true,
			ProofGaps:  []string{"controller.multi_source"},
		}
	}

	switch strategy {
	case StrategyGitArgo, StrategyGitFlux, StrategyKustomizeFlux:
		return compareGitRevisions(surfaces)
	case StrategyOCIArgo, StrategyOCIFlux:
		return compareOCIRevisions(surfaces)
	case StrategyHelmArgo, StrategyHelmFlux:
		return revisionAgreement{
			Incomplete: true,
			ProofGaps:  []string{"runtime.helm_chart_anchor"},
		}
	case StrategyConfigHubOCIArgo, StrategyConfigHubOCIFlux:
		// Deferred: OCI equality requires the controller-observed digest
		// to be matched against the ConfigHub-rendered digest. ConfigHub
		// does not expose a rendered digest per unit revision today.
		// File header explains the cross-repo dependency. v0.2 returns
		// Agreed=true here so existing OCI fixtures retain their v0.1
		// behaviour; the follow-up PR will tighten this.
		return revisionAgreement{Agreed: true}
	}
	// Unknown / future strategies: don't block.
	return revisionAgreement{Agreed: true}
}

// compareOCIRevisions performs controller↔runtime OCI digest equality
// for the non-ConfigHub OCI strategies (oci-flux, oci-argo). Anchors:
//
//   - Controller: Chain[0].Revision in `sha256:hex` form (Flux
//     OCIRepository) or in `<tag>@sha256:hex` form (some Argo
//     emissions). normalizeOCIDigest accepts both.
//   - Runtime: the `@sha256:hex` suffix on the container image
//     reference. extractOCIDigestFromImage returns the canonical
//     `sha256:hex` form.
//
// If the controller emits a tag-only revision (no `sha256:` prefix or
// embedded digest), equality is unverifiable — proof gap
// "controller.oci_digest". Same for tag-only runtime images
// ("runtime.oci_digest"). Direct hex compare on the digest portion.
func compareOCIRevisions(s SourceTruthSurfaces) revisionAgreement {
	if s.Controller == nil || s.Runtime == nil {
		return revisionAgreement{Incomplete: true}
	}

	ctrlDigest := normalizeOCIDigest(s.Controller.RevisionOrDigest)
	if ctrlDigest == "" {
		if strings.TrimSpace(s.Controller.RevisionOrDigest) == "" {
			return revisionAgreement{Incomplete: true}
		}
		return revisionAgreement{
			Incomplete: true,
			ProofGaps:  []string{"controller.oci_digest"},
		}
	}

	runtimeDigest := extractOCIDigestFromImage(s.Runtime.Value)
	if runtimeDigest == "" {
		return revisionAgreement{
			Incomplete: true,
			ProofGaps:  []string{"runtime.oci_digest"},
		}
	}

	if ctrlDigest == runtimeDigest {
		return revisionAgreement{Agreed: true}
	}

	return revisionAgreement{
		Agreed:  false,
		Outlier: OutlierController,
	}
}

// normalizeOCIDigest extracts a canonical "sha256:hex" digest from an
// arbitrary controller revision string. Accepts:
//
//   - "sha256:abc123..."        — direct
//   - "main@sha256:abc123..."   — tag@digest form
//   - "v1.0.0@sha256:abc123..." — same
//
// Returns "" if no sha256-shaped digest is found. Hex must be at least
// 7 chars for the parser to consider it a valid digest.
func normalizeOCIDigest(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	// Tag@digest form: take the part after "@".
	if at := strings.Index(s, "@"); at >= 0 && at+1 < len(s) {
		s = s[at+1:]
	}
	if !strings.HasPrefix(s, "sha256:") {
		return ""
	}
	hex := s[len("sha256:"):]
	if len(hex) < 7 {
		return ""
	}
	for _, c := range hex {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return ""
		}
	}
	return s
}

// extractOCIDigestFromImage pulls the @sha256:hex suffix from a
// container image reference. Returns "" if the image carries no
// digest suffix (tag-only images are a runtime proof gap under
// OCI strategies).
func extractOCIDigestFromImage(image string) string {
	image = strings.TrimSpace(image)
	at := strings.LastIndex(image, "@")
	if at < 0 {
		return ""
	}
	return normalizeOCIDigest(image[at+1:])
}

// compareGitRevisions performs controller↔runtime Git-SHA equality for
// vanilla GitOps strategies. ConfigHub is opaque under git-* (Git is the
// source of truth), so this function does not consult the ConfigHub
// surface at all.
//
// Anchors:
//   - Controller: Chain[0].Revision parsed by the existing tracers.
//     Common shapes: bare hex SHA ("abc123def456"), "sha1:" prefix
//     (Flux), or branch@sha1:hex form. normalizeGitSHA handles all of
//     these.
//   - Runtime: a SHA-shaped substring inside the container image tag
//     (e.g. "ghcr.io/x/y:abc123de" or "ghcr.io/x/y:1.2.3-abc123de").
//     extractGitSHAFromImage tries the whole tag first, then segments.
//
// If either anchor is missing, the result is Incomplete with a stable
// proof-gap key. If both are present and prefix-compatible, Agreed.
// If both are present and disagree, controller is named the outlier
// (the most common explanation for a mismatch is that the controller
// observed a different commit than the one that got built into the
// runtime image; absent a third anchor cub-scout cannot tell whether
// the controller is stale or the image is stale, so it surfaces the
// mismatch and lets Pilot escalate).
func compareGitRevisions(s SourceTruthSurfaces) revisionAgreement {
	if s.Controller == nil || s.Runtime == nil {
		return revisionAgreement{Incomplete: true}
	}

	ctrlSHA := normalizeGitSHA(s.Controller.RevisionOrDigest)
	if ctrlSHA == "" {
		// Already covered by collectProofGaps as
		// "controller.revision_or_digest" if blank, or as "non-hex"
		// here when the controller emitted something Derive can't
		// recognise as a SHA. Distinguish the two via different keys
		// so Pilot can separate "controller emitted nothing" from
		// "controller emitted something we can't read".
		if strings.TrimSpace(s.Controller.RevisionOrDigest) == "" {
			return revisionAgreement{Incomplete: true}
		}
		return revisionAgreement{
			Incomplete: true,
			ProofGaps:  []string{"controller.commit_sha_unrecognised"},
		}
	}

	runtimeSHA := extractGitSHAFromImage(s.Runtime.Value)
	if runtimeSHA == "" {
		return revisionAgreement{
			Incomplete: true,
			ProofGaps:  []string{"runtime.commit_sha_anchor"},
		}
	}

	if shaMatches(ctrlSHA, runtimeSHA) {
		return revisionAgreement{Agreed: true}
	}

	return revisionAgreement{
		Agreed:  false,
		Outlier: OutlierController,
	}
}

// normalizeGitSHA returns a lowercase hex prefix of s if s looks like a
// Git SHA (raw hex, "sha1:hex", or "branch@sha1:hex"). Returns "" when
// no SHA can be extracted. Minimum hex length is 7 (Git's standard
// short-SHA cutoff).
func normalizeGitSHA(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	// Flux frequently emits "branch@sha1:hex"; take the part after "@".
	if at := strings.Index(s, "@"); at >= 0 && at+1 < len(s) {
		s = s[at+1:]
	}
	// Strip "sha1:" prefix if present.
	s = strings.TrimPrefix(s, "sha1:")
	// Take the leading hex run.
	end := 0
	for end < len(s) {
		c := s[end]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') {
			end++
			continue
		}
		break
	}
	if end < 7 {
		return ""
	}
	return s[:end]
}

// extractGitSHAFromImage looks for a Git SHA embedded in a container
// image reference. Strips any OCI digest suffix (@sha256:...), splits
// the tag on common separators (-_./), and returns the first segment
// that looks like a Git SHA. Returns "" if no anchor is found.
func extractGitSHAFromImage(image string) string {
	image = strings.TrimSpace(image)
	if image == "" {
		return ""
	}
	// Strip @sha256:... suffix — that's an OCI digest, not a git SHA.
	if at := strings.LastIndex(image, "@"); at >= 0 {
		image = image[:at]
	}
	colon := strings.LastIndex(image, ":")
	if colon < 0 || colon == len(image)-1 {
		return ""
	}
	tag := image[colon+1:]

	// Direct: tag is itself a hex SHA.
	if sha := normalizeGitSHA(tag); sha != "" {
		return sha
	}

	// Split by common separators and try each segment.
	segments := strings.FieldsFunc(tag, func(r rune) bool {
		return r == '-' || r == '_' || r == '.' || r == '+'
	})
	for _, seg := range segments {
		if sha := normalizeGitSHA(seg); sha != "" {
			return sha
		}
	}
	return ""
}

// shaMatches checks if two SHAs are prefix-compatible. Controllers
// emit full or 12-char SHAs; image tags often carry 7-9 char prefixes.
// We accept prefix equality in either direction, which is correct
// because Git itself uses unique-prefix matching for short SHAs.
func shaMatches(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if len(a) > len(b) {
		a, b = b, a
	}
	return strings.HasPrefix(b, a)
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
