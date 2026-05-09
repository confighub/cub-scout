package agent

import (
	"strings"
	"testing"
)

// The cases below cover the council's required v0.1 verdict shapes. The
// table is intentionally written so each test name maps 1:1 to a council-
// named state — when #395 fixtures land they will share these case names.

// TestDerive_Rules_StrategyRequired locks in the rule that an empty
// strategy short-circuits to ASK / UNKNOWN. cub-scout never infers the
// strategy.
func TestDerive_Rules_StrategyRequired(t *testing.T) {
	ev := Derive("", SourceTruthSurfaces{
		ConfigHub:  &ConfigHubSurface{Space: "s", Unit: "u", Revision: "1"},
		Controller: &ControllerSurface{Kind: "Flux", Source: "oci://example/u", RevisionOrDigest: "sha256:abc", Health: "Ready"},
		Runtime:    &RuntimeSurface{Resource: "Deployment/u in s", Field: "spec...", Value: "v", Health: "Current"},
	})

	if ev.Status != StatusASK {
		t.Errorf("status = %s, want ASK", ev.Status)
	}
	if ev.SourceTruth != VerdictUNKNOWN {
		t.Errorf("source_truth = %s, want UNKNOWN", ev.SourceTruth)
	}
	if !containsString(ev.ProofGaps, "declared_strategy") {
		t.Errorf("proof_gaps = %v, want to contain declared_strategy", ev.ProofGaps)
	}
}

// TestDerive_PASS_AGREED is the happy path: confighub-oci-flux strategy,
// all three surfaces populated, controller observes an OCI source.
func TestDerive_PASS_AGREED(t *testing.T) {
	ev := Derive(StrategyConfigHubOCIFlux, SourceTruthSurfaces{
		ConfigHub: &ConfigHubSurface{Space: "demo", Unit: "rag-server", Revision: "47", URL: "https://hub/units/x/y"},
		Controller: &ControllerSurface{
			Kind:             "Flux",
			Source:           "oci://oci.confighub.com/demo/rag-server",
			RevisionOrDigest: "sha256:abc123",
			Health:           "Ready",
		},
		Runtime: &RuntimeSurface{
			Resource: "Deployment/rag-server in demo",
			Field:    "spec.template.spec.containers[0].image",
			Value:    "oci.confighub.com/demo/rag-server@sha256:abc123",
			Health:   "Current",
		},
	})

	if ev.Status != StatusPASS {
		t.Errorf("status = %s, want PASS (gaps=%v)", ev.Status, ev.ProofGaps)
	}
	if ev.SourceTruth != VerdictAGREED {
		t.Errorf("source_truth = %s, want AGREED", ev.SourceTruth)
	}
	if len(ev.ProofGaps) != 0 {
		t.Errorf("proof_gaps = %v, want empty", ev.ProofGaps)
	}
	if ev.DeclaredStrategy != "ConfigHub -> OCI -> Flux -> Kubernetes" {
		t.Errorf("declared_strategy = %q, want council-shaped string", ev.DeclaredStrategy)
	}
}

// TestDerive_StrategyMismatch_ControllerOutlier is the council's primary
// trap: ConfigHub-OCI strategy declared, but the controller is observed
// reading directly from Git. cub-scout must classify this as BLOCK with
// controller as the outlier — Pilot must not call the Day-2 path proven
// when the bridge has been bypassed.
func TestDerive_StrategyMismatch_ControllerOutlier(t *testing.T) {
	ev := Derive(StrategyConfigHubOCIArgo, SourceTruthSurfaces{
		ConfigHub: &ConfigHubSurface{Space: "demo", Unit: "rag-server", Revision: "47"},
		Controller: &ControllerSurface{
			Kind:             "Argo",
			Source:           "https://github.com/example/rag-deploy",
			RevisionOrDigest: "abc123def456",
			Health:           "Synced/Healthy",
		},
		Runtime: &RuntimeSurface{
			Resource: "Deployment/rag-server in demo",
			Field:    "spec.template.spec.containers[0].image",
			Value:    "ghcr.io/example/rag:1.2.3",
			Health:   "Current",
		},
	})

	if ev.Status != StatusBLOCK {
		t.Errorf("status = %s, want BLOCK (council's stale-bridge trap)", ev.Status)
	}
	if ev.SourceTruth != VerdictMISMATCH {
		t.Errorf("source_truth = %s, want MISMATCH", ev.SourceTruth)
	}
	if ev.Outlier != OutlierController {
		t.Errorf("outlier = %s, want controller", ev.Outlier)
	}
	if !strings.Contains(ev.SafeNextAction, "OCI") {
		t.Errorf("safe_next_action = %q, want mention of OCI", ev.SafeNextAction)
	}
}

// TestDerive_VanillaGitOps_PASS proves strategy-relativity in the other
// direction: under git-argo, Argo reading directly from Git is correct
// and must produce PASS. The runtime image carries a Git short-SHA in
// its tag (the common release-pipeline shape "v1.2.3-<short-sha>"),
// which v0.2 cross-surface equality matches against the controller's
// observed SHA.
func TestDerive_VanillaGitOps_PASS(t *testing.T) {
	ev := Derive(StrategyGitArgo, SourceTruthSurfaces{
		ConfigHub: &ConfigHubSurface{Space: "demo", Unit: "rag-server", Revision: "47"},
		Controller: &ControllerSurface{
			Kind:             "Argo",
			Source:           "https://github.com/example/rag-deploy",
			RevisionOrDigest: "abc123def456",
			Health:           "Synced/Healthy",
		},
		Runtime: &RuntimeSurface{
			Resource: "Deployment/rag-server in demo",
			Field:    "spec.template.spec.containers[0].image",
			Value:    "ghcr.io/example/rag:v1.2.3-abc123de",
			Health:   "Current",
		},
	})

	if ev.Status != StatusPASS {
		t.Errorf("status = %s, want PASS under vanilla GitOps strategy (gaps=%v)", ev.Status, ev.ProofGaps)
	}
	if ev.SourceTruth != VerdictAGREED {
		t.Errorf("source_truth = %s, want AGREED", ev.SourceTruth)
	}
}

// TestDerive_INCOMPLETE_MissingDigest enforces the strict rule: missing
// proof never produces PASS. Even with all three surfaces present, an
// empty controller.revision_or_digest must surface as INCOMPLETE / WATCH.
func TestDerive_INCOMPLETE_MissingDigest(t *testing.T) {
	ev := Derive(StrategyConfigHubOCIFlux, SourceTruthSurfaces{
		ConfigHub: &ConfigHubSurface{Space: "demo", Unit: "rag-server", Revision: "47"},
		Controller: &ControllerSurface{
			Kind:             "Flux",
			Source:           "oci://oci.confighub.com/demo/rag-server",
			RevisionOrDigest: "", // <-- the missing proof
			Health:           "Ready",
		},
		Runtime: &RuntimeSurface{
			Resource: "Deployment/rag-server in demo",
			Field:    "spec.template.spec.containers[0].image",
			Value:    "oci.confighub.com/demo/rag-server@sha256:abc123",
			Health:   "Current",
		},
	})

	if ev.Status == StatusPASS {
		t.Fatalf("status = PASS with missing controller.revision_or_digest; the strict rule was violated")
	}
	if ev.Status != StatusWATCH {
		t.Errorf("status = %s, want WATCH", ev.Status)
	}
	if ev.SourceTruth != VerdictINCOMPLETE {
		t.Errorf("source_truth = %s, want INCOMPLETE", ev.SourceTruth)
	}
	if !containsString(ev.ProofGaps, "controller.revision_or_digest") {
		t.Errorf("proof_gaps = %v, want to contain controller.revision_or_digest", ev.ProofGaps)
	}
}

// TestDerive_BLOCKED_FetchFailure covers the case where cub-scout could
// not read one of the surfaces in read-only mode — auth missing, CLI
// missing, kubeconfig unreachable. The CLI passes nil for that surface;
// Derive emits BLOCK / BLOCKED with a concrete safe_next_action naming
// the missing surface.
func TestDerive_BLOCKED_FetchFailure(t *testing.T) {
	ev := Derive(StrategyConfigHubOCIFlux, SourceTruthSurfaces{
		ConfigHub:  nil, // <-- could not auth to ConfigHub
		Controller: &ControllerSurface{Kind: "Flux", Source: "oci://x", RevisionOrDigest: "sha256:y", Health: "Ready"},
		Runtime:    &RuntimeSurface{Resource: "r", Field: "f", Value: "v", Health: "Current"},
	})

	if ev.Status != StatusBLOCK {
		t.Errorf("status = %s, want BLOCK", ev.Status)
	}
	if ev.SourceTruth != VerdictBLOCKED {
		t.Errorf("source_truth = %s, want BLOCKED", ev.SourceTruth)
	}
	if !strings.Contains(ev.SafeNextAction, "ConfigHub") {
		t.Errorf("safe_next_action = %q, want explicit ConfigHub naming", ev.SafeNextAction)
	}
}

// TestDerive_StrategyMismatch_TakesPrecedenceOverGaps locks in ordering:
// when the controller-source check fires (BLOCK), the proof-gap WATCH
// path must NOT silently downgrade to WATCH. Strategy violation is a
// hard signal regardless of soft gaps.
func TestDerive_StrategyMismatch_TakesPrecedenceOverGaps(t *testing.T) {
	ev := Derive(StrategyConfigHubOCIArgo, SourceTruthSurfaces{
		ConfigHub: &ConfigHubSurface{Space: "demo", Unit: "rag-server", Revision: "47"},
		Controller: &ControllerSurface{
			Kind:             "Argo",
			Source:           "https://github.com/example/rag-deploy",
			RevisionOrDigest: "", // also a soft gap
			Health:           "Synced/Healthy",
		},
		Runtime: &RuntimeSurface{
			Resource: "Deployment/rag-server in demo",
			Field:    "spec.template.spec.containers[0].image",
			Value:    "ghcr.io/example/rag:1.2.3",
			Health:   "Current",
		},
	})

	if ev.Status != StatusBLOCK {
		t.Errorf("status = %s, want BLOCK (strategy violation > proof gap)", ev.Status)
	}
	if ev.Outlier != OutlierController {
		t.Errorf("outlier = %s, want controller", ev.Outlier)
	}
}

// TestDerive_NeverPASS_OnAnyMissingProof is a meta-test that sweeps every
// single-field omission and asserts no combination produces PASS. This
// is the strict rule the council placed at the centre of the contract.
func TestDerive_NeverPASS_OnAnyMissingProof(t *testing.T) {
	full := SourceTruthSurfaces{
		ConfigHub: &ConfigHubSurface{Space: "demo", Unit: "u", Revision: "1"},
		Controller: &ControllerSurface{
			Kind: "Flux", Source: "oci://x", RevisionOrDigest: "sha256:y", Health: "Ready",
		},
		Runtime: &RuntimeSurface{
			Resource: "r", Field: "f", Value: "v", Health: "Current",
		},
	}

	mutations := []struct {
		name  string
		blank func(*SourceTruthSurfaces)
	}{
		{"confighub.unit blank", func(s *SourceTruthSurfaces) { s.ConfigHub.Unit = "" }},
		{"confighub.revision blank", func(s *SourceTruthSurfaces) { s.ConfigHub.Revision = "" }},
		{"controller.source blank", func(s *SourceTruthSurfaces) { s.Controller.Source = "" }},
		{"controller.revision_or_digest blank", func(s *SourceTruthSurfaces) { s.Controller.RevisionOrDigest = "" }},
		{"runtime.value blank", func(s *SourceTruthSurfaces) { s.Runtime.Value = "" }},
	}

	for _, m := range mutations {
		t.Run(m.name, func(t *testing.T) {
			s := SourceTruthSurfaces{
				ConfigHub:  &ConfigHubSurface{Space: full.ConfigHub.Space, Unit: full.ConfigHub.Unit, Revision: full.ConfigHub.Revision},
				Controller: &ControllerSurface{Kind: full.Controller.Kind, Source: full.Controller.Source, RevisionOrDigest: full.Controller.RevisionOrDigest, Health: full.Controller.Health},
				Runtime:    &RuntimeSurface{Resource: full.Runtime.Resource, Field: full.Runtime.Field, Value: full.Runtime.Value, Health: full.Runtime.Health},
			}
			m.blank(&s)
			ev := Derive(StrategyConfigHubOCIFlux, s)
			if ev.Status == StatusPASS {
				t.Fatalf("Derive emitted PASS with %s — the strict missing-proof rule was violated. Evidence: %+v", m.name, ev)
			}
		})
	}
}

// TestDerive_NeverPASS_OnAnyMissingGitAnchor is the v0.2 git-strategy
// variant of the NeverPASS sweep. Under git-* strategies the additional
// proof gap is "runtime.commit_sha_anchor" — when the runtime image
// carries no SHA-shaped tag segment, cross-surface equality is
// unverifiable and Derive must not emit PASS.
func TestDerive_NeverPASS_OnAnyMissingGitAnchor(t *testing.T) {
	full := SourceTruthSurfaces{
		ConfigHub: &ConfigHubSurface{Space: "demo", Unit: "u", Revision: "1"},
		Controller: &ControllerSurface{
			Kind:             "Argo",
			Source:           "https://github.com/example/repo",
			RevisionOrDigest: "abc123def456",
			Health:           "Synced/Healthy",
		},
		Runtime: &RuntimeSurface{
			Resource: "Deployment/u in demo",
			Field:    "spec.template.spec.containers[0].image",
			Value:    "ghcr.io/example/u:v1.2.3-abc123de",
			Health:   "Current",
		},
	}

	mutations := []struct {
		name string
		mod  func(*SourceTruthSurfaces)
	}{
		{"controller.revision_or_digest blank", func(s *SourceTruthSurfaces) { s.Controller.RevisionOrDigest = "" }},
		{"controller.revision_or_digest non-hex", func(s *SourceTruthSurfaces) { s.Controller.RevisionOrDigest = "main" }},
		{"runtime image has no SHA tag", func(s *SourceTruthSurfaces) { s.Runtime.Value = "ghcr.io/example/u:1.2.3" }},
		{"runtime image has only OCI digest", func(s *SourceTruthSurfaces) { s.Runtime.Value = "ghcr.io/example/u@sha256:dead" }},
	}

	for _, m := range mutations {
		t.Run(m.name, func(t *testing.T) {
			s := SourceTruthSurfaces{
				ConfigHub:  &ConfigHubSurface{Space: full.ConfigHub.Space, Unit: full.ConfigHub.Unit, Revision: full.ConfigHub.Revision},
				Controller: &ControllerSurface{Kind: full.Controller.Kind, Source: full.Controller.Source, RevisionOrDigest: full.Controller.RevisionOrDigest, Health: full.Controller.Health},
				Runtime:    &RuntimeSurface{Resource: full.Runtime.Resource, Field: full.Runtime.Field, Value: full.Runtime.Value, Health: full.Runtime.Health},
			}
			m.mod(&s)
			ev := Derive(StrategyGitArgo, s)
			if ev.Status == StatusPASS {
				t.Fatalf("Derive emitted PASS with %s — git-strategy equality should have caught this. Evidence: %+v", m.name, ev)
			}
		})
	}
}

// TestDerive_GitMismatch_ControllerOutlier locks in the v0.2 happy-path
// mismatch case: controller and runtime both expose Git SHAs and they
// disagree. Outlier is named so Pilot can route the mismatch.
func TestDerive_GitMismatch_ControllerOutlier(t *testing.T) {
	ev := Derive(StrategyGitArgo, SourceTruthSurfaces{
		ConfigHub: &ConfigHubSurface{Space: "demo", Unit: "u", Revision: "1"},
		Controller: &ControllerSurface{
			Kind:             "Argo",
			Source:           "https://github.com/example/repo",
			RevisionOrDigest: "abc123def456",
			Health:           "Synced/Healthy",
		},
		Runtime: &RuntimeSurface{
			Resource: "Deployment/u in demo",
			Field:    "spec.template.spec.containers[0].image",
			Value:    "ghcr.io/example/u:v1.2.4-def456ab",
			Health:   "Current",
		},
	})

	if ev.Status != StatusBLOCK {
		t.Errorf("status = %s, want BLOCK", ev.Status)
	}
	if ev.SourceTruth != VerdictMISMATCH {
		t.Errorf("source_truth = %s, want MISMATCH", ev.SourceTruth)
	}
	if ev.Outlier != OutlierController {
		t.Errorf("outlier = %s, want controller", ev.Outlier)
	}
}

// TestDerive_MultiSourceArgo_IncompleteRegardlessOfStrategy locks in the
// Phase 3 rule: when the controller surface flags MultiSource=true,
// equality is unverifiable beyond the parsed source. Derive must emit
// the proof gap and downgrade to WATCH/INCOMPLETE under any strategy —
// even when the parsed source's anchor would otherwise have produced
// a clean PASS.
func TestDerive_MultiSourceArgo_IncompleteRegardlessOfStrategy(t *testing.T) {
	strategies := []SourceTruthStrategy{
		StrategyGitArgo,
		StrategyGitFlux,
		StrategyConfigHubOCIArgo,
		StrategyConfigHubOCIFlux,
	}

	for _, strategy := range strategies {
		t.Run(string(strategy), func(t *testing.T) {
			ev := Derive(strategy, SourceTruthSurfaces{
				ConfigHub: &ConfigHubSurface{Space: "demo", Unit: "rag-server", Revision: "47"},
				Controller: &ControllerSurface{
					Kind:             "Argo",
					Source:           "oci://oci.confighub.com/demo/rag-server",
					RevisionOrDigest: "abc123def456",
					Health:           "Synced/Healthy",
					MultiSource:      true,
				},
				Runtime: &RuntimeSurface{
					Resource: "Deployment/rag-server in demo",
					Field:    "spec.template.spec.containers[0].image",
					Value:    "ghcr.io/example/rag:v1.2.3-abc123de",
					Health:   "Current",
				},
			})

			if ev.Status == StatusPASS {
				t.Fatalf("status = PASS with multi_source=true under strategy %s — equality across un-parsed sources cannot be PASS", strategy)
			}
			if !containsString(ev.ProofGaps, "controller.multi_source") {
				t.Errorf("proof_gaps = %v, want to contain controller.multi_source", ev.ProofGaps)
			}
		})
	}
}

// TestNormalizeGitSHA_AcceptsCommonShapes locks in the parser's
// tolerance for the shapes the existing tracers emit.
func TestNormalizeGitSHA_AcceptsCommonShapes(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"abc123def456", "abc123def456"},
		{"ABC123DEF456", "abc123def456"},
		{"sha1:abc123def456", "abc123def456"},
		{"main@sha1:abc123def456", "abc123def456"},
		{"abc123d", "abc123d"}, // 7 chars, the Git short-SHA minimum
		{"main", ""},           // alphabetic, not hex
		{"abc12", ""},          // 5 chars, below short-SHA minimum
		{"", ""},
	}
	for _, c := range cases {
		got := normalizeGitSHA(c.input)
		if got != c.want {
			t.Errorf("normalizeGitSHA(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// TestNormalizeOCIDigest covers the controller-side digest shapes the
// existing tracers emit (#409 Phase 2).
func TestNormalizeOCIDigest(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"sha256:abc123def456", "sha256:abc123def456"},
		{"SHA256:ABC123DEF456", "sha256:abc123def456"},
		{"latest@sha256:abc123def456", "sha256:abc123def456"}, // tag@digest form
		{"v1.0.0@sha256:abc123def456", "sha256:abc123def456"},
		{"abc123def456", ""}, // raw hex with no sha256: prefix is a Git SHA, not OCI
		{"sha256:abc", ""},   // hex too short
		{"sha256:xyz123abc", ""}, // non-hex chars
		{"", ""},
	}
	for _, c := range cases {
		got := normalizeOCIDigest(c.input)
		if got != c.want {
			t.Errorf("normalizeOCIDigest(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// TestExtractOCIDigestFromImage covers the runtime image-reference
// shapes that carry an OCI digest suffix.
func TestExtractOCIDigestFromImage(t *testing.T) {
	cases := []struct {
		image string
		want  string
	}{
		{"ghcr.io/x/y@sha256:abc123def456", "sha256:abc123def456"},
		{"ghcr.io/x/y:v1.0@sha256:abc123def456", "sha256:abc123def456"},
		{"ghcr.io/x/y:v1.0", ""},          // tag-only, no digest
		{"ghcr.io/x/y", ""},                // no tag, no digest
		{"", ""},
	}
	for _, c := range cases {
		got := extractOCIDigestFromImage(c.image)
		if got != c.want {
			t.Errorf("extractOCIDigestFromImage(%q) = %q, want %q", c.image, got, c.want)
		}
	}
}

// TestDerive_OCIStrategy_DigestEquality locks in the Phase 2 OCI
// equality behaviour for non-ConfigHub OCI strategies. PASS when
// digests agree, BLOCK/MISMATCH/controller-outlier when they don't,
// WATCH/INCOMPLETE with proof gaps when an anchor is missing.
func TestDerive_OCIStrategy_DigestEquality(t *testing.T) {
	mkSurfaces := func(ctrlRev, runtimeImage string) SourceTruthSurfaces {
		return SourceTruthSurfaces{
			ConfigHub: &ConfigHubSurface{Space: "demo", Unit: "u", Revision: "1"},
			Controller: &ControllerSurface{
				Kind:             "Flux",
				Source:           "oci://ghcr.io/example/u",
				RevisionOrDigest: ctrlRev,
				Health:           "Ready",
			},
			Runtime: &RuntimeSurface{
				Resource: "Deployment/u in demo",
				Field:    "spec.template.spec.containers[0].image",
				Value:    runtimeImage,
				Health:   "Current",
			},
		}
	}

	t.Run("digests match -> PASS", func(t *testing.T) {
		ev := Derive(StrategyOCIFlux, mkSurfaces(
			"sha256:abc123def456",
			"ghcr.io/example/u@sha256:abc123def456",
		))
		if ev.Status != StatusPASS {
			t.Errorf("status = %s, want PASS (gaps=%v)", ev.Status, ev.ProofGaps)
		}
	})

	t.Run("digests disagree -> BLOCK/MISMATCH/controller", func(t *testing.T) {
		ev := Derive(StrategyOCIArgo, mkSurfaces(
			"sha256:abc123def456",
			"ghcr.io/example/u@sha256:def456abc123",
		))
		if ev.Status != StatusBLOCK {
			t.Errorf("status = %s, want BLOCK", ev.Status)
		}
		if ev.SourceTruth != VerdictMISMATCH {
			t.Errorf("source_truth = %s, want MISMATCH", ev.SourceTruth)
		}
		if ev.Outlier != OutlierController {
			t.Errorf("outlier = %s, want controller", ev.Outlier)
		}
	})

	t.Run("runtime tag-only -> WATCH/INCOMPLETE/runtime.oci_digest gap", func(t *testing.T) {
		ev := Derive(StrategyOCIFlux, mkSurfaces(
			"sha256:abc123def456",
			"ghcr.io/example/u:v1.2.3", // no @sha256: suffix
		))
		if ev.Status != StatusWATCH {
			t.Errorf("status = %s, want WATCH", ev.Status)
		}
		if !containsString(ev.ProofGaps, "runtime.oci_digest") {
			t.Errorf("proof_gaps = %v, want runtime.oci_digest", ev.ProofGaps)
		}
	})

	t.Run("controller tag-only -> WATCH/INCOMPLETE/controller.oci_digest gap", func(t *testing.T) {
		ev := Derive(StrategyOCIFlux, mkSurfaces(
			"v1.2.3", // no sha256: prefix
			"ghcr.io/example/u@sha256:abc123def456",
		))
		if ev.Status != StatusWATCH {
			t.Errorf("status = %s, want WATCH", ev.Status)
		}
		if !containsString(ev.ProofGaps, "controller.oci_digest") {
			t.Errorf("proof_gaps = %v, want controller.oci_digest", ev.ProofGaps)
		}
	})
}

// TestDerive_HelmStrategy_AlwaysIncomplete locks in Phase 2's intentional
// limitation: helm-* strategies emit "runtime.helm_chart_anchor" as a
// proof gap because the runtime extractor for helm.sh/chart labels is
// not yet wired into RuntimeSurface. Helm strategies will go from WATCH
// to PASS-capable in a follow-up.
func TestDerive_HelmStrategy_AlwaysIncomplete(t *testing.T) {
	for _, strategy := range []SourceTruthStrategy{StrategyHelmFlux, StrategyHelmArgo} {
		t.Run(string(strategy), func(t *testing.T) {
			ev := Derive(strategy, SourceTruthSurfaces{
				ConfigHub: &ConfigHubSurface{Space: "demo", Unit: "redis", Revision: "1"},
				Controller: &ControllerSurface{
					Kind:             "Flux",
					Source:           "https://charts.bitnami.com/bitnami",
					RevisionOrDigest: "17.0.0",
					Health:           "Ready",
				},
				Runtime: &RuntimeSurface{
					Resource: "Deployment/redis in demo",
					Field:    "spec.template.spec.containers[0].image",
					Value:    "docker.io/bitnami/redis:7.0.0",
					Health:   "Current",
				},
			})
			if ev.Status != StatusWATCH {
				t.Errorf("status = %s, want WATCH (helm equality not wired yet)", ev.Status)
			}
			if !containsString(ev.ProofGaps, "runtime.helm_chart_anchor") {
				t.Errorf("proof_gaps = %v, want runtime.helm_chart_anchor", ev.ProofGaps)
			}
		})
	}
}

// TestStrategy_KustomizeFluxReusesGitEquality verifies that
// kustomize-flux dispatches to the git equality path (the anchor
// is a Git commit SHA — the Kustomize overlay sits on top but
// doesn't change the source-of-truth identifier).
func TestStrategy_KustomizeFluxReusesGitEquality(t *testing.T) {
	ev := Derive(StrategyKustomizeFlux, SourceTruthSurfaces{
		ConfigHub: &ConfigHubSurface{Space: "demo", Unit: "u", Revision: "1"},
		Controller: &ControllerSurface{
			Kind:             "Flux",
			Source:           "https://github.com/example/repo",
			RevisionOrDigest: "main@sha1:abc123def456",
			Health:           "Ready",
		},
		Runtime: &RuntimeSurface{
			Resource: "Deployment/u in demo",
			Field:    "spec.template.spec.containers[0].image",
			Value:    "ghcr.io/example/u:v1.2.3-abc123de",
			Health:   "Current",
		},
	})
	if ev.Status != StatusPASS {
		t.Errorf("status = %s, want PASS (kustomize-flux should reuse git equality)", ev.Status)
	}
}

// TestAllStrategies_HumanRenderingsExist verifies every enum value has
// a Human() rendering distinct from the raw enum string.
func TestAllStrategies_HumanRenderingsExist(t *testing.T) {
	for _, s := range AllStrategies() {
		human := s.Human()
		if human == "" {
			t.Errorf("strategy %q has empty Human() rendering", s)
		}
		if human == string(s) {
			t.Errorf("strategy %q Human() = %q, want a council-shaped arrow string", s, human)
		}
	}
}

// TestExpectsArgoController_AllStrategies sweeps all strategies to
// catch the case where a new strategy is added without updating the
// controller-kind dispatch.
func TestExpectsArgoController_AllStrategies(t *testing.T) {
	want := map[SourceTruthStrategy]bool{
		StrategyConfigHubOCIArgo: true,
		StrategyConfigHubOCIFlux: false,
		StrategyGitArgo:          true,
		StrategyGitFlux:          false,
		StrategyHelmArgo:         true,
		StrategyHelmFlux:         false,
		StrategyKustomizeFlux:    false,
		StrategyOCIArgo:          true,
		StrategyOCIFlux:          false,
	}
	for _, s := range AllStrategies() {
		got := s.ExpectsArgoController()
		w, ok := want[s]
		if !ok {
			t.Errorf("strategy %q in AllStrategies() but missing from test table", s)
			continue
		}
		if got != w {
			t.Errorf("ExpectsArgoController(%q) = %v, want %v", s, got, w)
		}
	}
}

// TestExtractGitSHAFromImage covers the segment-splitting cases real
// release pipelines produce.
func TestExtractGitSHAFromImage(t *testing.T) {
	cases := []struct {
		image string
		want  string
	}{
		{"ghcr.io/x/y:abc123de", "abc123de"},
		{"ghcr.io/x/y:v1.2.3-abc123de", "abc123de"},
		{"ghcr.io/x/y:main-abc123de", "abc123de"},
		{"ghcr.io/x/y:1.2.3_abc123de", "abc123de"},
		{"ghcr.io/x/y:1.2.3", ""},
		{"ghcr.io/x/y@sha256:dead", ""}, // OCI digest only, no git SHA
		{"ghcr.io/x/y", ""},              // no tag at all
		{"", ""},
	}
	for _, c := range cases {
		got := extractGitSHAFromImage(c.image)
		if got != c.want {
			t.Errorf("extractGitSHAFromImage(%q) = %q, want %q", c.image, got, c.want)
		}
	}
}

// containsString is a slice-membership helper. Named to avoid colliding
// with the package-local substring `contains` in kyverno_scan_test.go.
func containsString(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
