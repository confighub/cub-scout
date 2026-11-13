// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/confighub/cub-scout/v2/internal/mapsvc"
	"github.com/confighub/cub-scout/v2/pkg/agent"
)

// The states a trace answer can have. A closed list: an agent decides on
// these, and on the reason keys below, not on the summary's words.
//
// A trace reads the readiness of delivery controllers' own objects and of
// nothing else. A chain link's Ready flag is not evidence on its own: several
// builders set it to true for a link they never read (the traced workload, a
// Helm chart in template mode, a Sveltos owner named only by an annotation, a
// Crossplane object). So an answer says "reported ready" only of a link whose
// kind is a controller object and whose status is that controller's word.
const (
	traceStateTraced           = "traced"
	traceStateChainNotReady    = "chain-not-ready"
	traceStateReadinessNotRead = "readiness-not-read"
	traceStateNoOwnerMarker    = "no-owner-marker"
	traceStateNotObservable    = "not-observable"

	traceReasonControllersReady = "controllers-report-ready"
	traceReasonLinkNotReady     = "link-not-ready"
	traceReasonNoControllerRead = "no-controller-readiness-read"
	traceReasonNoChainObserved  = "no-chain-observed"

	// traceCostCoverage is a key, spelled out in json-contracts.md: the
	// Kubernetes requests of the trace session's own clients. Not counted:
	// the OCI source confirmation of --with-confighub, subprocesses (the
	// flux CLI), ConfigHub reads.
	traceCostCoverage = "trace-session-clients"
)

// traceStates and traceReasons are what the conformance test checks answers
// against. Adding a value means documenting it in json-contracts.md.
var (
	traceStates  = []string{traceStateTraced, traceStateChainNotReady, traceStateReadinessNotRead, traceStateNoOwnerMarker, traceStateNotObservable}
	traceReasons = []string{traceReasonControllersReady, traceReasonLinkNotReady, traceReasonNoControllerRead, agent.TraceReasonNoOwnerMarker, traceReasonNoChainObserved}
)

// traceControllerKinds are the kinds whose Ready in a chain is read from a
// controller's own conditions (Flux) or sync and health (Argo CD).
var traceControllerKinds = map[string]bool{
	"Kustomization": true, "HelmRelease": true, "GitRepository": true,
	"OCIRepository": true, "HelmRepository": true, "Bucket": true,
	"Application": true,
}

// traceUnreadStatuses are status words builders use for a link they did not
// read. A controller object carrying one of them has no reported readiness:
// "Unknown" is what the Flux reader writes when its read of the object failed.
var traceUnreadStatuses = map[string]bool{"": true, "unknown": true, "observed": true}

var (
	traceSafeName = regexp.MustCompile(`^[a-z0-9]([-a-z0-9.]{0,251}[a-z0-9])?$`)
	traceSafeKind = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{0,62}$`)
)

type traceLinkReadiness int

const (
	traceLinkNotRead traceLinkReadiness = iota
	traceLinkReportedReady
	traceLinkReportedNotReady
)

func traceReadinessOf(link agent.ChainLink) traceLinkReadiness {
	if !traceControllerKinds[link.Kind] || traceUnreadStatuses[strings.ToLower(strings.TrimSpace(link.Status))] {
		return traceLinkNotRead
	}
	if link.Ready {
		return traceLinkReportedReady
	}
	return traceLinkReportedNotReady
}

// traceLinkLabel names a link in cub-scout's own words. A kind and name are
// used only when they have the shape of a Kubernetes kind and object name.
// Some links are named from free text (a Sveltos annotation, a fragment of a
// repository URL); those are referred to by position, and their text is left
// to Quoted.
func traceLinkLabel(link agent.ChainLink, position int) (label string, named bool) {
	if traceSafeKind.MatchString(link.Kind) && traceSafeName.MatchString(link.Name) {
		return link.Kind + "/" + link.Name, true
	}
	return fmt.Sprintf("link %d of the chain", position+1), false
}

// buildTraceAnswer puts what a trace found into the common envelope. It adds
// no observation: every field is drawn from the result the rest of the
// output is drawn from, and it claims less than that output where the result
// does not support more.
func buildTraceAnswer(result *agent.TraceResult, kind, name, namespace string) *mapsvc.Answer {
	if result == nil {
		return nil
	}
	targetNamed := traceSafeKind.MatchString(kind) && traceSafeName.MatchString(name) && (namespace == "" || traceSafeName.MatchString(namespace))
	target := "the traced resource"
	if targetNamed {
		target = kind + "/" + name
		if namespace != "" {
			target += " in " + namespace
		}
	}
	answer := &mapsvc.Answer{
		Cluster:  mapsvc.AnswerCluster{Context: result.Context, IDSource: "not-read"},
		Basis:    []mapsvc.AnswerBasis{},
		Unknowns: []mapsvc.AnswerUnknown{},
		Next:     []mapsvc.AnswerNext{},
	}
	observed := result.TracedAt
	if observed.IsZero() {
		observed = time.Now()
	}
	answer.ObservedAt = observed.UTC().Format(time.RFC3339)
	if result.ReadCost != nil {
		answer.Cost = *result.ReadCost
	} else {
		answer.Cost.Coverage = "not-measured"
	}

	var reportedReady int
	var notRead []string
	var broken *agent.ChainLink
	brokenLabel := ""
	for i, link := range result.Chain {
		label, named := traceLinkLabel(link, i)
		readiness := traceReadinessOf(link)
		// Basis holds the links that are somebody's word for something: a
		// controller's report, and the traced resource's own tie to the
		// chain. The other links are in `chain` already; repeating each one
		// to say it was not read doubled the answer on a real Argo CD chain
		// of nine links (CI run 43517230657). They are counted in unknowns.
		isTarget := strings.EqualFold(link.Kind, kind) && link.Name == name && link.Namespace == namespace
		if readiness != traceLinkNotRead || isTarget {
			answer.Basis = append(answer.Basis, traceLinkBasis(link, label, named, readiness))
		}
		switch readiness {
		case traceLinkReportedReady:
			reportedReady++
		case traceLinkReportedNotReady:
			if broken == nil {
				broken, brokenLabel = &result.Chain[i], label
			}
		default:
			notRead = append(notRead, label)
		}
	}

	noOwner := len(result.Chain) == 0 && agent.IsNoOwnerTrace(result.Reason, result.Error)
	scope := ""
	if namespace != "" {
		scope = " -n " + namespace
	}
	explainTarget := func(why string) {
		if targetNamed {
			answer.Next = append(answer.Next, mapsvc.AnswerNext{
				Command: preferInvocationForm(fmt.Sprintf("cub-scout explain %s/%s%s --format json", kind, name, scope)),
				Why:     why,
			})
		}
	}
	switch {
	case noOwner:
		answer.State, answer.Reason = traceStateNoOwnerMarker, agent.TraceReasonNoOwnerMarker
		answer.Summary = fmt.Sprintf("No GitOps owner marker was recognised on %s; that does not show nothing manages it.", target)
		explainTarget("who set its fields, and its recent events")
	case len(result.Chain) == 0:
		answer.State, answer.Reason = traceStateNotObservable, traceReasonNoChainObserved
		answer.Summary = fmt.Sprintf("No delivery chain was observed for %s.", target)
	case broken != nil:
		answer.State, answer.Reason = traceStateChainNotReady, traceReasonLinkNotReady
		answer.Summary = fmt.Sprintf("A delivery controller in the chain of %s reports not ready: %s.", target, brokenLabel)
		statusScope := ""
		if traceSafeName.MatchString(broken.Namespace) {
			statusScope = " -n " + broken.Namespace
		}
		answer.Next = append(answer.Next, mapsvc.AnswerNext{
			Command: preferInvocationForm("cub-scout gitops status" + statusScope + " --format json"),
			Why:     "what the delivery controllers report, with their reasons",
		})
	case reportedReady == 0:
		// A chain was found, and nothing in it is a controller whose
		// readiness was read: a Crossplane object, an owner known only from
		// an annotation, a controller object whose read failed.
		answer.State, answer.Reason = traceStateReadinessNotRead, traceReasonNoControllerRead
		answer.Summary = fmt.Sprintf("A chain of %d links was found for %s, and trace read the readiness of none of them.", len(result.Chain), target)
		explainTarget("its health and recent events")
	default:
		answer.State, answer.Reason = traceStateTraced, traceReasonControllersReady
		answer.Summary = fmt.Sprintf("%s has a delivery chain of %d links; the %d delivery controllers in it report ready.", target, len(result.Chain), reportedReady)
		if len(notRead) > 0 {
			answer.Summary += fmt.Sprintf(" Trace did not read the readiness of the other %d.", len(notRead))
		}
	}

	// What trace did not read is said, not left to be inferred from a state
	// that sounds complete.
	if len(result.Chain) > 0 && len(notRead) > 0 {
		listed := notRead
		more := ""
		if len(listed) > 4 {
			more = fmt.Sprintf(" and %d more links", len(listed)-4)
			listed = listed[:4]
		}
		answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{
			What:   "the readiness of " + strings.Join(listed, ", ") + more,
			Kind:   mapsvc.UnknownNotRequested,
			Reason: "trace reads delivery controllers' conditions, not workload health",
		})
		if answer.State == traceStateTraced {
			explainTarget("its health and recent events, which trace does not read")
		}
	}

	// Everything else the trace could not read. The text was assembled from
	// API errors and, on some paths, from a controller's or the flux CLI's
	// own output, so it is quoted, not restated. For the no-owner answer the
	// first sentence is the finding itself; anything after it is not.
	text := strings.TrimSpace(result.Error)
	if noOwner {
		for _, finding := range []string{agent.NoOwnerTraceMessage, agent.NoFluxOwnerTraceMessage} {
			text = strings.TrimSpace(strings.TrimPrefix(text, finding))
		}
		text = strings.TrimSpace(strings.TrimLeft(text, ".; "))
	}
	if text != "" && !(noOwner && result.Reason == "" && agent.IsNoOwnerTrace("", text) && text == strings.TrimSpace(result.Error)) {
		answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{
			What:   "part of the trace evidence",
			Kind:   mapsvc.UnknownNotObservable,
			Reason: "one or more reads failed or were incomplete; the messages are quoted",
			Quoted: text,
		})
	}
	return answer
}

// traceLinkBasis says what a chain link contributes and whose word that is.
//
// For a delivery controller's object whose status was read, the claim is its
// readiness, and it rests on the controller's report. For every other link
// the claim is only that it is in the chain, which trace established from
// ownership metadata; its readiness is not claimed either way.
func traceLinkBasis(link agent.ChainLink, label string, named bool, readiness traceLinkReadiness) mapsvc.AnswerBasis {
	basis := mapsvc.AnswerBasis{}
	switch readiness {
	case traceLinkReportedReady:
		basis.RestsOn, basis.Claim = mapsvc.RestsOnControllerReport, label+" is reported ready by its controller"
	case traceLinkReportedNotReady:
		basis.RestsOn, basis.Claim = mapsvc.RestsOnControllerReport, label+" is reported not ready by its controller"
	default:
		basis.RestsOn, basis.Claim = mapsvc.RestsOnObjectMetadata, label+" is in the chain by ownership metadata; trace did not read its readiness"
	}
	// A link's status and revision are in `chain` already and are not
	// repeated here. Two things are quoted, word for word, because the claim
	// needs them and they are the cluster's text: why a controller says it is
	// not ready, and a link's own name when it does not have the shape of an
	// object name.
	var quoted []string
	if !named {
		quoted = append(quoted, "link: "+strings.TrimSpace(link.Kind+"/"+link.Name))
	}
	if readiness == traceLinkReportedNotReady {
		if status := strings.TrimSpace(link.Status); status != "" {
			quoted = append(quoted, "status: "+status)
		}
		if message := strings.TrimSpace(link.Message); message != "" {
			quoted = append(quoted, "message: "+message)
		}
	}
	basis.Quoted = strings.Join(quoted, "; ")
	// A reference only where an object by that kind and name exists to be
	// fetched. An Argo CD Application's source, a chart, an OCI address are
	// described by a controller; they are not objects.
	if named && kindToGVR(link.Kind).Resource != "" && link.Kind != "ConfigHub OCI" {
		basis.Ref = &mapsvc.AnswerRef{Kind: link.Kind, Namespace: link.Namespace, Name: link.Name}
	}
	return basis
}
