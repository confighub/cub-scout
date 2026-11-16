// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strings"

	"github.com/confighub/cub-scout/v2/internal/mapsvc"
	"github.com/confighub/cub-scout/v2/pkg/agent"
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
	sourceTruthReasonProofGaps            = "proof-gaps"
	sourceTruthReasonSurfaceNotRead       = "surface-not-read"
	sourceTruthReasonStrategyNotDeclared  = "strategy-not-declared"
)

var (
	sourceTruthStates  = []string{sourceTruthStateAgreed, sourceTruthStateNotAgreed, sourceTruthStateIncomplete, sourceTruthStateNotObservable}
	sourceTruthReasons = []string{sourceTruthReasonObjectRevisionIsHead, sourceTruthReasonGitRevisionMatches, sourceTruthReasonOCIDigestMatches, sourceTruthReasonSurfacesDisagree, sourceTruthReasonProofGaps, sourceTruthReasonSurfaceNotRead, sourceTruthReasonStrategyNotDeclared}
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
func buildSourceTruthAnswer(ev agent.SourceTruthEvidence, strategy agent.SourceTruthStrategy, kind, name, namespace string) *mapsvc.LeanAnswer {
	answer := &mapsvc.LeanAnswer{Unknowns: []mapsvc.AnswerUnknown{}, Next: []mapsvc.AnswerNext{}}
	targetNamed := traceSafeKind.MatchString(kind) && traceSafeName.MatchString(name) && traceSafeName.MatchString(namespace)

	confighubOCI := strategy == agent.StrategyConfigHubOCIArgo || strategy == agent.StrategyConfigHubOCIFlux
	switch {
	case strings.TrimSpace(ev.DeclaredStrategy) == "":
		answer.State, answer.Reason = sourceTruthStateNotObservable, sourceTruthReasonStrategyNotDeclared
	case ev.SourceTruth == agent.VerdictAGREED:
		// The reason names what was compared.
		answer.State = sourceTruthStateAgreed
		switch strategy {
		case agent.StrategyConfigHubOCIArgo, agent.StrategyConfigHubOCIFlux:
			answer.Reason = sourceTruthReasonObjectRevisionIsHead
		case agent.StrategyOCIArgo, agent.StrategyOCIFlux:
			answer.Reason = sourceTruthReasonOCIDigestMatches
		default:
			answer.Reason = sourceTruthReasonGitRevisionMatches
		}
	case ev.SourceTruth == agent.VerdictMISMATCH:
		answer.State, answer.Reason = sourceTruthStateNotAgreed, sourceTruthReasonSurfacesDisagree
	case ev.SourceTruth == agent.VerdictINCOMPLETE:
		answer.State, answer.Reason = sourceTruthStateIncomplete, sourceTruthReasonProofGaps
	default:
		answer.State, answer.Reason = sourceTruthStateNotObservable, sourceTruthReasonSurfaceNotRead
	}

	for _, gap := range ev.ProofGaps {
		meaning, known := sourceTruthGapMeaning[gap]
		if !known {
			meaning.kind, meaning.reason = mapsvc.UnknownNotObservable, "this field was not available to read"
		}
		answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{What: gap, Kind: meaning.kind, Reason: meaning.reason})
	}
	// Agreement under a ConfigHub OCI strategy compares revisions, not the
	// artifact. Said every time, so "agreed" is not read as more than it is.
	if answer.State == sourceTruthStateAgreed && confighubOCI {
		answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{
			What:   "the controller's digest against ConfigHub's",
			Kind:   mapsvc.UnknownNotCompared,
			Reason: "not implemented for this strategy",
		})
	}
	// Why a surface could not be read is error text. It is in
	// collection_errors already and is not copied here: the answer says
	// that reads failed and where the messages are.
	if n := len(ev.CollectionErrors); n > 0 {
		answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{
			What:   fmt.Sprintf("%d surface read(s)", n),
			Kind:   mapsvc.UnknownNotObservable,
			Reason: "the read failed; the messages are in collection_errors, as text from the cluster or ConfigHub",
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
