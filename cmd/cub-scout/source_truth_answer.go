// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/confighub/cub-scout/v3/internal/mapsvc"
	"github.com/confighub/cub-scout/v3/pkg/agent"
)

// The states a source-truth answer can have, and the keys for why. A closed
// list. They describe what was observed; PASS, WATCH, ASK and BLOCK beside
// them are verdict words, kept in JSON until 3.0 and in the ASCII and
// Markdown renderings after it (maintainer's decision, 2026-11-16).
const (
	sourceTruthStateAgreed        = "agreed"
	sourceTruthStateNotAgreed     = "not-agreed"
	sourceTruthStateIncomplete    = "incomplete"
	sourceTruthStateNotObservable = "not-observable"

	sourceTruthReasonObjectRevisionIsHead = "object-revision-is-head"
	sourceTruthReasonGitRevisionMatches   = "git-revision-matches"
	sourceTruthReasonOCIDigestMatches     = "oci-digest-matches"
	sourceTruthReasonSurfacesDisagree     = "surfaces-disagree"
	sourceTruthReasonSourceNotAsDeclared  = "source-not-as-declared"
	sourceTruthReasonNothingCompared      = "nothing-compared-for-strategy"
	sourceTruthReasonProofGaps            = "proof-gaps"
	sourceTruthReasonSurfaceNotRead       = "surface-not-read"
	sourceTruthReasonStrategyNotDeclared  = "strategy-not-declared"
)

var (
	sourceTruthStates  = []string{sourceTruthStateAgreed, sourceTruthStateNotAgreed, sourceTruthStateIncomplete, sourceTruthStateNotObservable}
	sourceTruthReasons = []string{sourceTruthReasonObjectRevisionIsHead, sourceTruthReasonGitRevisionMatches, sourceTruthReasonOCIDigestMatches, sourceTruthReasonSurfacesDisagree, sourceTruthReasonSourceNotAsDeclared, sourceTruthReasonNothingCompared, sourceTruthReasonProofGaps, sourceTruthReasonSurfaceNotRead, sourceTruthReasonStrategyNotDeclared}
)

// sourceTruthGapMeaning says, for each proof-gap key, whether the thing was
// not compared or could not be read, and why in a few words.
var sourceTruthGapMeaning = map[string]struct{ kind, reason string }{
	"confighub.object_revision":                   {mapsvc.UnknownNotCompared, "the object names no unit revision, so nothing ties what runs to the unit read"},
	"confighub.object_revision_differs_from_head": {mapsvc.UnknownNotCompared, "the object names another unit revision than the head read"},
	"controller.source_names_another_space":       {mapsvc.UnknownNotCompared, "the controller's source is another space than the unit's"},
	"controller.multi_source":                     {mapsvc.UnknownNotCompared, "the controller has several sources and only the first was read"},
	"runtime.helm_chart_anchor":                   {mapsvc.UnknownNotCompared, "the chart version at runtime is not read for this strategy"},
	"runtime.commit_sha_anchor":                   {mapsvc.UnknownNotCompared, "the running image carries no Git revision to compare"},
	"controller.commit_sha_unrecognised":          {mapsvc.UnknownNotCompared, "the controller's revision is not a Git revision that can be compared"},
	"runtime.oci_digest":                          {mapsvc.UnknownNotCompared, "the running image carries no digest to compare"},
	"controller.oci_digest":                       {mapsvc.UnknownNotCompared, "the controller reports no digest to compare"},
}

// sourceTruthEnvelope is the JSON document: the evidence as it has been,
// and the answer beside it.
type sourceTruthEnvelope struct {
	agent.SourceTruthEvidence
	Answer *mapsvc.LeanAnswer `json:"answer,omitempty"`
}

// buildSourceTruthAnswer puts a source-truth result into the lean form of
// the envelope. It observes nothing: every field is drawn from the evidence.
// Values read from ConfigHub or the cluster stay in `surfaces`. Whose word
// each surface is does not vary and is in the contract, not in every answer:
// confighub is ConfigHub's record, controller is the controller's report,
// runtime is the workload object.
func buildSourceTruthAnswer(ev agent.SourceTruthEvidence, strategy agent.SourceTruthStrategy, kind, name, namespace string, observedAt time.Time) *mapsvc.LeanAnswer {
	answer := &mapsvc.LeanAnswer{ObservedAt: observedAt.UTC().Format(time.RFC3339), Unknowns: []mapsvc.AnswerUnknown{}, Next: []mapsvc.AnswerNext{}}
	targetNamed := traceSafeKind.MatchString(kind) && traceSafeName.MatchString(name) && traceSafeName.MatchString(namespace)

	confighubOCI := strategy == agent.StrategyConfigHubOCIArgo || strategy == agent.StrategyConfigHubOCIFlux
	switch {
	case strings.TrimSpace(ev.DeclaredStrategy) == "":
		answer.State, answer.Reason = sourceTruthStateNotObservable, sourceTruthReasonStrategyNotDeclared
	case ev.SourceTruth == agent.VerdictAGREED:
		// The reason names what was compared, strategy by strategy. A
		// strategy not named here has no comparison this code knows of: the
		// evidence may say AGREED, and the answer does not repeat it.
		answer.State = sourceTruthStateAgreed
		switch strategy {
		case agent.StrategyConfigHubOCIArgo, agent.StrategyConfigHubOCIFlux:
			answer.Reason = sourceTruthReasonObjectRevisionIsHead
		case agent.StrategyOCIArgo, agent.StrategyOCIFlux:
			answer.Reason = sourceTruthReasonOCIDigestMatches
		case agent.StrategyGitArgo, agent.StrategyGitFlux, agent.StrategyKustomizeFlux:
			answer.Reason = sourceTruthReasonGitRevisionMatches
		default:
			answer.State, answer.Reason = sourceTruthStateIncomplete, sourceTruthReasonNothingCompared
		}
	case ev.SourceTruth == agent.VerdictMISMATCH && agent.StrategySourceViolated(strategy, ev.Surfaces.Controller):
		// Not two revisions that differ: the controller reads from a kind
		// of source the declared strategy rules out.
		answer.State, answer.Reason = sourceTruthStateNotAgreed, sourceTruthReasonSourceNotAsDeclared
	case ev.SourceTruth == agent.VerdictMISMATCH:
		answer.State, answer.Reason = sourceTruthStateNotAgreed, sourceTruthReasonSurfacesDisagree
	case ev.SourceTruth == agent.VerdictINCOMPLETE:
		answer.State, answer.Reason = sourceTruthStateIncomplete, sourceTruthReasonProofGaps
	default:
		answer.State, answer.Reason = sourceTruthStateNotObservable, sourceTruthReasonSurfaceNotRead
	}

	for _, gap := range ev.ProofGaps {
		if gap == "declared_strategy" {
			// Not something that could not be read: nothing was asked for.
			answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{What: gap, Kind: mapsvc.UnknownNotRequested, Reason: "no --strategy was given, and cub-scout does not infer one"})
			continue
		}
		meaning, known := sourceTruthGapMeaning[gap]
		if !known {
			meaning.kind, meaning.reason = mapsvc.UnknownNotObservable, "this field was not available to read"
		}
		answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{What: gap, Kind: meaning.kind, Reason: meaning.reason})
	}
	// "agreed" is said with what it did not compare, every time, so it is
	// not read as more than it is. Under a ConfigHub OCI strategy revisions
	// are compared and the artifact is not. Under the Git and plain OCI
	// strategies the controller is compared with the workload, and
	// ConfigHub's record is not consulted at all.
	switch {
	case answer.State == sourceTruthStateAgreed && confighubOCI:
		answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{
			What:   "the controller's digest against ConfigHub's",
			Kind:   mapsvc.UnknownNotCompared,
			Reason: "not implemented for this strategy",
		})
	case answer.State == sourceTruthStateAgreed:
		answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{
			What:   "ConfigHub's record against the controller and the workload",
			Kind:   mapsvc.UnknownNotCompared,
			Reason: "this strategy compares the controller with the workload only",
		})
	case answer.Reason == sourceTruthReasonNothingCompared:
		answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{
			What:   "the three surfaces against each other",
			Kind:   mapsvc.UnknownNotCompared,
			Reason: "no comparison is defined for this strategy",
		})
	}
	// A surface that is absent is named, whether or not an error came with
	// it: an answer that could not read something says what.
	if strings.TrimSpace(ev.DeclaredStrategy) != "" {
		for _, surface := range []struct {
			name   string
			absent bool
		}{{"surfaces.confighub", ev.Surfaces.ConfigHub == nil}, {"surfaces.controller", ev.Surfaces.Controller == nil}, {"surfaces.runtime", ev.Surfaces.Runtime == nil}} {
			if surface.absent {
				answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{What: surface.name, Kind: mapsvc.UnknownNotObservable, Reason: "this surface was not read"})
			}
		}
	}
	// Why a surface could not be read is error text. It is in
	// collection_errors already and is not copied here: the answer says
	// that reads failed and where the messages are.
	if n := len(ev.CollectionErrors); n > 0 {
		answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{
			What:   fmt.Sprintf("why %d read(s) failed", n),
			Kind:   mapsvc.UnknownNotObservable,
			Reason: "the messages are in collection_errors, as text from the cluster or ConfigHub",
		})
	}

	if answer.Reason == sourceTruthReasonStrategyNotDeclared {
		// The fix is to say which delivery path to judge against. The
		// strategies are listed by the command's own help.
		answer.Next = append(answer.Next, mapsvc.AnswerNext{
			Command: preferInvocationForm("cub-scout compare source-truth --help"),
			Why:     "the strategies to choose from; pass one with --strategy",
		})
	}
	if targetNamed && answer.State != sourceTruthStateAgreed && answer.Reason != sourceTruthReasonStrategyNotDeclared {
		answer.Next = append(answer.Next, mapsvc.AnswerNext{
			Command: preferInvocationForm(fmt.Sprintf("cub-scout trace %s/%s -n %s --format json", kind, name, namespace)),
			Why:     "the delivery chain and what each controller in it reports",
		})
	}
	return answer
}
