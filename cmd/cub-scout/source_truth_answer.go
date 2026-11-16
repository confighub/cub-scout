// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strings"
	"time"

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

	// sourceTruthCostCoverage is a key, spelled out in json-contracts.md:
	// the Kubernetes requests of this command's own clients. The ConfigHub
	// unit read and any subprocess are not counted.
	sourceTruthCostCoverage = "kubernetes-session-clients"
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
	Answer *mapsvc.Answer `json:"answer,omitempty"`
}

// buildSourceTruthAnswer puts a source-truth result into the common
// envelope. It observes nothing: every field is drawn from the evidence.
// Values read from ConfigHub or the cluster stay in `surfaces`; the answer
// says which party each surface rests on and what was, and was not, compared.
func buildSourceTruthAnswer(ev agent.SourceTruthEvidence, strategy agent.SourceTruthStrategy, kind, name, namespace string, observedAt time.Time, cost *agent.KubernetesReadCost) *mapsvc.Answer {
	answer := &mapsvc.Answer{
		ObservedAt: observedAt.UTC().Format(time.RFC3339),
		Cluster:    mapsvc.AnswerCluster{Context: ev.Context, IDSource: "not-read"},
		Basis:      []mapsvc.AnswerBasis{},
		Unknowns:   []mapsvc.AnswerUnknown{},
		Next:       []mapsvc.AnswerNext{},
	}
	if cost != nil {
		answer.Cost = mapsvc.AnswerCostFrom(*cost)
	} else {
		answer.Cost.Coverage = "not-measured"
	}
	targetNamed := traceSafeKind.MatchString(kind) && traceSafeName.MatchString(name) && traceSafeName.MatchString(namespace)
	target := "the workload"
	if targetNamed {
		target = kind + "/" + name + " in " + namespace
	}

	// Whose word each surface is, in as few bytes as say it: the claim names
	// the surface in `surfaces`, where the values are. The workload is not
	// referenced again; it is the object the command was asked about.
	if ev.Surfaces.ConfigHub != nil {
		answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{Claim: "surfaces.confighub", RestsOn: mapsvc.RestsOnConfigHubRecord})
	}
	if ev.Surfaces.Controller != nil {
		answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{Claim: "surfaces.controller", RestsOn: mapsvc.RestsOnControllerReport})
	}
	if ev.Surfaces.Runtime != nil {
		answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{Claim: "surfaces.runtime", RestsOn: mapsvc.RestsOnObjectMetadata})
	}

	confighubOCI := strategy == agent.StrategyConfigHubOCIArgo || strategy == agent.StrategyConfigHubOCIFlux
	switch {
	case strings.TrimSpace(ev.DeclaredStrategy) == "":
		answer.State, answer.Reason = sourceTruthStateNotObservable, sourceTruthReasonStrategyNotDeclared
		answer.Summary = "No delivery strategy was declared, so nothing was read or compared; cub-scout does not infer one."
	case ev.SourceTruth == agent.VerdictAGREED:
		answer.State = sourceTruthStateAgreed
		compared := ""
		switch strategy {
		case agent.StrategyConfigHubOCIArgo, agent.StrategyConfigHubOCIFlux:
			answer.Reason = sourceTruthReasonObjectRevisionIsHead
			compared = "the object names the unit's head revision, and the controller's source has the declared shape"
		case agent.StrategyOCIArgo, agent.StrategyOCIFlux:
			answer.Reason = sourceTruthReasonOCIDigestMatches
			compared = "the digest the controller reports is the digest of the running image"
		default:
			answer.Reason = sourceTruthReasonGitRevisionMatches
			compared = "the Git revision the controller reports is the revision in the running image"
		}
		// What was compared is said once, in the summary. The agreement
		// itself is cub-scout's comparison of the three surfaces.
		answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{Claim: "the agreement", RestsOn: mapsvc.RestsOnCubScoutComparison})
		answer.Summary = fmt.Sprintf("For %s, %s.", target, compared)
	case ev.SourceTruth == agent.VerdictMISMATCH:
		answer.State, answer.Reason = sourceTruthStateNotAgreed, sourceTruthReasonSurfacesDisagree
		outlier := "one surface"
		switch ev.Outlier {
		case agent.OutlierConfigHub:
			outlier = "the ConfigHub surface"
		case agent.OutlierController:
			outlier = "the controller surface"
		case agent.OutlierRuntime:
			outlier = "the runtime surface"
		}
		answer.Summary = fmt.Sprintf("For %s, the surfaces do not agree under the declared strategy; %s differs.", target, outlier)
	case ev.SourceTruth == agent.VerdictINCOMPLETE:
		answer.State, answer.Reason = sourceTruthStateIncomplete, sourceTruthReasonProofGaps
		answer.Summary = fmt.Sprintf("For %s, the three surfaces were read and %d thing(s) needed to compare them are missing; nothing is shown to agree or to differ.", target, len(ev.ProofGaps))
	default:
		answer.State, answer.Reason = sourceTruthStateNotObservable, sourceTruthReasonSurfaceNotRead
		answer.Summary = fmt.Sprintf("For %s, at least one surface could not be read, so nothing was compared.", target)
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
	// Why a surface could not be read is error text: quoted, not restated.
	for _, text := range ev.CollectionErrors {
		if text = strings.TrimSpace(text); text != "" {
			answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{
				What:   "a surface",
				Kind:   mapsvc.UnknownNotObservable,
				Reason: "the read failed; the message is quoted",
				Quoted: text,
			})
		}
	}

	if targetNamed && answer.State != sourceTruthStateAgreed && answer.Reason != sourceTruthReasonStrategyNotDeclared {
		answer.Next = append(answer.Next, mapsvc.AnswerNext{
			Command: preferInvocationForm(fmt.Sprintf("cub-scout trace %s/%s -n %s --format json", kind, name, namespace)),
			Why:     "the delivery chain and what each controller in it reports",
		})
	}
	return answer
}
