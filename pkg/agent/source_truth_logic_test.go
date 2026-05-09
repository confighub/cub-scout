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
