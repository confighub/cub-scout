// Package agent — source-truth evidence contract.
//
// This file implements the read-only evidence contract the council placed at
// the center of #393. cub-scout is the *evidence provider*: it reads three
// surfaces (ConfigHub intent, GitOps controller observation, Kubernetes
// runtime) and emits a structured verdict that Pilot's acceptance kernel
// consumes. cub-scout is not the judge.
//
// Architectural triad (do not blur the split):
//
//   - cub-scout = read-only evidence provider for source truth
//   - Pilot     = acceptance judge
//   - ConfigHub = authority and workflow engine
//
// See the council verdict on #393 (#393#issuecomment-4407651183) for the
// canonical scope statement and the JSON contract shape that is mirrored
// below.
//
// Strict rules enforced here, not by intent:
//
//   1. Strategy-relative correctness. Identical observations get opposite
//      verdicts under different strategies. Argo reading directly from Git
//      is correct under `git-argo` and a MISMATCH under `confighub-oci-argo`.
//   2. Missing source/digest/runtime proof MUST produce WATCH, ASK, or
//      BLOCK — never a guessed PASS. The decision logic asserts this.
//
// v0.1 scope (this file): the JSON contract shape, the strategy enum, and
// the decision logic for presence + strategy-relative correctness. Cross-
// surface revision equality is intentionally deferred to v0.2 — see the
// `compareRevisions` placeholder and the comment in `Derive` for the
// rationale.

package agent

import (
	"encoding/json"
	"io"
	"strings"
)

// SourceTruthStrategy is the declared delivery path. It is required input
// to Derive — never inferred. See the `String()` form for the human-readable
// rendering that lands in JSON output.
type SourceTruthStrategy string

const (
	// StrategyConfigHubOCIArgo: ConfigHub authors a unit, renders to OCI,
	// Argo CD pulls the OCI artifact and applies to Kubernetes.
	StrategyConfigHubOCIArgo SourceTruthStrategy = "confighub-oci-argo"

	// StrategyConfigHubOCIFlux: same as above, with Flux instead of Argo.
	StrategyConfigHubOCIFlux SourceTruthStrategy = "confighub-oci-flux"

	// StrategyGitArgo: vanilla GitOps. Argo reads directly from Git, no
	// ConfigHub bridge.
	StrategyGitArgo SourceTruthStrategy = "git-argo"

	// StrategyGitFlux: vanilla GitOps with Flux.
	StrategyGitFlux SourceTruthStrategy = "git-flux"

	// StrategyHelmArgo: Argo CD pulls a Helm chart from a chart registry
	// and applies to Kubernetes. Source-of-truth anchor is the chart
	// version. ConfigHub is not in the chain.
	StrategyHelmArgo SourceTruthStrategy = "helm-argo"

	// StrategyHelmFlux: Flux pulls a Helm chart via HelmRepository or
	// HelmChart and applies to Kubernetes. Same anchor shape as
	// helm-argo.
	StrategyHelmFlux SourceTruthStrategy = "helm-flux"

	// StrategyKustomizeFlux: Flux Kustomization built from a Git source.
	// Source-of-truth anchor is the Git commit SHA — equality semantics
	// match git-flux but the controller-source shape is a GitRepository
	// + a Kustomization overlay.
	StrategyKustomizeFlux SourceTruthStrategy = "kustomize-flux"

	// StrategyOCIArgo: Argo CD pulls an OCI artifact from a non-ConfigHub
	// registry. Source-of-truth anchor is the OCI digest. Distinct from
	// confighub-oci-argo in that ConfigHub is not in the chain — equality
	// is controller↔runtime only.
	StrategyOCIArgo SourceTruthStrategy = "oci-argo"

	// StrategyOCIFlux: Flux OCIRepository pointed at a non-ConfigHub
	// registry. Same anchor shape as oci-argo.
	StrategyOCIFlux SourceTruthStrategy = "oci-flux"
)

// AllStrategies is the closed enum of strategies cub-scout understands.
// Adding a strategy requires updating expectsConfigHubOCISource,
// ExpectsArgoController, the Human rendering, and compareRevisions
// dispatch.
func AllStrategies() []SourceTruthStrategy {
	return []SourceTruthStrategy{
		StrategyConfigHubOCIArgo,
		StrategyConfigHubOCIFlux,
		StrategyGitArgo,
		StrategyGitFlux,
		StrategyHelmArgo,
		StrategyHelmFlux,
		StrategyKustomizeFlux,
		StrategyOCIArgo,
		StrategyOCIFlux,
	}
}

// ParseStrategy parses a CLI-friendly string into a strategy. Empty input
// returns ("", false) so callers can distinguish "unset" from "invalid".
func ParseStrategy(s string) (SourceTruthStrategy, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "", false
	}
	for _, st := range AllStrategies() {
		if string(st) == s {
			return st, true
		}
	}
	return "", false
}

// Human returns the human-readable rendering of the strategy that lands in
// JSON output's `declared_strategy` field, e.g.
//
//	"ConfigHub -> OCI -> Argo -> Kubernetes"
//
// It mirrors the council's example shape exactly.
func (s SourceTruthStrategy) Human() string {
	switch s {
	case StrategyConfigHubOCIArgo:
		return "ConfigHub -> OCI -> Argo -> Kubernetes"
	case StrategyConfigHubOCIFlux:
		return "ConfigHub -> OCI -> Flux -> Kubernetes"
	case StrategyGitArgo:
		return "Git -> Argo -> Kubernetes"
	case StrategyGitFlux:
		return "Git -> Flux -> Kubernetes"
	case StrategyHelmArgo:
		return "Helm -> Argo -> Kubernetes"
	case StrategyHelmFlux:
		return "Helm -> Flux -> Kubernetes"
	case StrategyKustomizeFlux:
		return "Kustomize -> Flux -> Kubernetes"
	case StrategyOCIArgo:
		return "OCI -> Argo -> Kubernetes"
	case StrategyOCIFlux:
		return "OCI -> Flux -> Kubernetes"
	default:
		return string(s)
	}
}

// expectsConfigHubOCISource reports whether the strategy requires the
// controller surface to be reading from a ConfigHub-rendered OCI artifact
// (vs. directly from Git). The non-ConfigHub OCI strategies (oci-flux,
// oci-argo) intentionally return false — they accept any OCI registry,
// not just ConfigHub.
func (s SourceTruthStrategy) expectsConfigHubOCISource() bool {
	return s == StrategyConfigHubOCIArgo || s == StrategyConfigHubOCIFlux
}

// ExpectsArgoController reports whether the strategy requires Argo CD as
// the GitOps controller (vs. Flux). Exported so the cub-scout CLI can
// pick the right tracer; never silently fall back across controllers
// because that would mask the controller-kind mismatch the contract is
// supposed to surface.
func (s SourceTruthStrategy) ExpectsArgoController() bool {
	switch s {
	case StrategyConfigHubOCIArgo, StrategyGitArgo, StrategyHelmArgo, StrategyOCIArgo:
		return true
	}
	return false
}

// SourceTruthStatus is cub-scout's evidence-quality verdict. Pilot layers
// acceptance on top of this; cub-scout never decides "approved".
type SourceTruthStatus string

const (
	// StatusPASS: all required surfaces present, all required equalities
	// hold, no proof gaps. Pilot may accept.
	StatusPASS SourceTruthStatus = "PASS"

	// StatusWATCH: surfaces are mostly present and consistent, but at least
	// one proof gap or soft signal warrants confirmation before acceptance.
	StatusWATCH SourceTruthStatus = "WATCH"

	// StatusBLOCK: a hard rule fails (strategy mismatch, controller-source
	// resolution failure, etc.). Pilot must not accept on this evidence.
	StatusBLOCK SourceTruthStatus = "BLOCK"

	// StatusASK: cub-scout cannot classify deterministically and asks the
	// human/Pilot for context. Used when strategy is unknown or surfaces
	// fundamentally cannot be compared.
	StatusASK SourceTruthStatus = "ASK"
)

// SourceTruthVerdict is the cross-surface agreement verdict, separate from
// status: a verdict can be MISMATCH while status is BLOCK (strategy
// violation) or WATCH (soft mismatch with proof gaps).
type SourceTruthVerdict string

const (
	// VerdictAGREED: the three surfaces line up under the declared strategy.
	VerdictAGREED SourceTruthVerdict = "AGREED"

	// VerdictMISMATCH: at least one surface diverges from the strategy-
	// implied authority.
	VerdictMISMATCH SourceTruthVerdict = "MISMATCH"

	// VerdictINCOMPLETE: one or more surfaces missing required proof. The
	// outlier cannot be assigned with confidence.
	VerdictINCOMPLETE SourceTruthVerdict = "INCOMPLETE"

	// VerdictBLOCKED: cub-scout could not fetch one of the surfaces in
	// read-only mode (auth failure, controller CLI missing, etc.).
	VerdictBLOCKED SourceTruthVerdict = "BLOCKED"

	// VerdictUNKNOWN: cub-scout has no opinion. Used in ASK.
	VerdictUNKNOWN SourceTruthVerdict = "UNKNOWN"
)

// SourceTruthOutlier names the surface that diverges from the strategy-
// implied authority. "import_render" covers the case where the rendered
// OCI artifact disagrees with the unit's declared data — caught by #395
// fixtures, currently emitted as Unknown in v0.1.
type SourceTruthOutlier string

const (
	OutlierConfigHub    SourceTruthOutlier = "confighub"
	OutlierController   SourceTruthOutlier = "controller"
	OutlierRuntime      SourceTruthOutlier = "runtime"
	OutlierImportRender SourceTruthOutlier = "import_render"
	OutlierUnknown      SourceTruthOutlier = "unknown"
)

// ConfigHubSurface is what cub-scout reads from ConfigHub via the cub CLI.
type ConfigHubSurface struct {
	Space    string `json:"space,omitempty"`
	Unit     string `json:"unit,omitempty"`
	Revision string `json:"revision,omitempty"`
	URL      string `json:"url,omitempty"`
}

// ControllerSurface is what cub-scout reads from the GitOps controller
// (Flux or Argo) via the existing tracers.
type ControllerSurface struct {
	Kind             string `json:"kind,omitempty"`              // "Argo" | "Flux"
	Source           string `json:"source,omitempty"`            // observed source URL/identifier
	RevisionOrDigest string `json:"revision_or_digest,omitempty"` // SHA, digest, tag the controller observed
	Health           string `json:"health,omitempty"`            // human label: "Ready", "Reconciling", etc.

	// MultiSource is true when the controller observes multiple sources
	// (Argo CD's spec.sources[] with len > 1). cub-scout parses the first
	// source for the rest of the surface; equality across sources beyond
	// index 0 is unverifiable, so Derive emits an explicit
	// "controller.multi_source" proof gap. Strategy-agnostic — the gap
	// applies under any declared strategy.
	MultiSource bool `json:"multi_source,omitempty"`
}

// RuntimeSurface is what cub-scout reads from the live cluster.
type RuntimeSurface struct {
	Resource string `json:"resource,omitempty"` // e.g. "Deployment/foo in bar"
	Field    string `json:"field,omitempty"`    // e.g. "spec.template.spec.containers[0].image"
	Value    string `json:"value,omitempty"`    // observed value of the field
	Health   string `json:"health,omitempty"`   // kstatus-derived: Current/InProgress/Failed/Unknown
}

// SourceTruthSurfaces holds whatever evidence cub-scout was able to collect.
// Any surface may be nil — that's a proof gap, not a panic case.
type SourceTruthSurfaces struct {
	ConfigHub  *ConfigHubSurface  `json:"confighub,omitempty"`
	Controller *ControllerSurface `json:"controller,omitempty"`
	Runtime    *RuntimeSurface    `json:"runtime,omitempty"`
}

// SourceTruthEvidence is the full JSON contract — the structured value
// Pilot consumes. Field order in the JSON output mirrors the council's
// example to keep diffs reviewable.
type SourceTruthEvidence struct {
	DeclaredStrategy string              `json:"declared_strategy"`
	Status           SourceTruthStatus   `json:"status"`
	SourceTruth      SourceTruthVerdict  `json:"source_truth"`
	Outlier          SourceTruthOutlier  `json:"outlier"`
	Surfaces         SourceTruthSurfaces `json:"surfaces"`
	ProofGaps        []string            `json:"proof_gaps,omitempty"`
	SafeNextAction   string              `json:"safe_next_action,omitempty"`
}

// CollectionError is the typed error the CLI command returns when it
// cannot fetch one of the surfaces in read-only mode (auth failure,
// controller CLI missing, kubeconfig unreachable). The decision logic
// converts these into BLOCK status / BLOCKED verdict.
type CollectionError struct {
	Surface string // "confighub" | "controller" | "runtime"
	Reason  string // human-readable
}

func (e *CollectionError) Error() string {
	return e.Surface + ": " + e.Reason
}

// EncodeEvidence writes ev as the canonical wire-format JSON to w. The
// CLI command and the producer fixture suite both use this so the bytes
// they assert against and the bytes Pilot consumes are guaranteed to
// match.
//
// HTML escaping is disabled so the human-readable `>` character in
// `declared_strategy` (e.g. "ConfigHub -> OCI -> Flux -> Kubernetes")
// stays a literal `>` instead of `>`. Pilot decodes either form
// identically — this is purely a presentation choice that improves
// reviewability of fixture files and CLI output.
func EncodeEvidence(w io.Writer, ev SourceTruthEvidence) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(ev)
}
