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

// The states a trace answer can have. A closed list: an agent decides on
// these, and on the reason keys below, not on the summary's words.
const (
	traceStateTraced        = "traced"
	traceStateChainNotReady = "chain-not-ready"
	traceStateNoOwnerMarker = "no-owner-marker"
	traceStateNotObservable = "not-observable"

	traceReasonChainReady      = "chain-ready"
	traceReasonLinkNotReady    = "link-not-ready"
	traceReasonNoChainObserved = "no-chain-observed"
)

// traceStates and traceReasons are what the conformance test checks answers
// against. Adding a value means documenting it in json-contracts.md.
var (
	traceStates  = []string{traceStateTraced, traceStateChainNotReady, traceStateNoOwnerMarker, traceStateNotObservable}
	traceReasons = []string{traceReasonChainReady, traceReasonLinkNotReady, agent.TraceReasonNoOwnerMarker, traceReasonNoChainObserved}
)

// buildTraceAnswer puts what a trace found into the common envelope. It adds
// no observation: every field is drawn from the result the rest of the
// output is drawn from.
func buildTraceAnswer(result *agent.TraceResult, kind, name, namespace string) *mapsvc.Answer {
	if result == nil {
		return nil
	}
	target := kind + "/" + name
	if namespace != "" {
		target += " in " + namespace
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

	for _, link := range result.Chain {
		answer.Basis = append(answer.Basis, traceLinkBasis(link, kind, name, namespace))
	}

	var broken *agent.ChainLink
	for i := range result.Chain {
		if !result.Chain[i].Ready {
			broken = &result.Chain[i]
			break
		}
	}
	scope := ""
	if namespace != "" {
		scope = " -n " + namespace
	}
	switch {
	case len(result.Chain) == 0 && agent.IsNoOwnerTrace(result.Reason, result.Error):
		answer.State, answer.Reason = traceStateNoOwnerMarker, agent.TraceReasonNoOwnerMarker
		answer.Summary = fmt.Sprintf("No GitOps owner marker was recognised on %s; that does not show nothing manages it.", target)
		answer.Next = append(answer.Next, mapsvc.AnswerNext{
			Command: preferInvocationForm(fmt.Sprintf("cub-scout explain %s/%s%s --format json", kind, name, scope)),
			Why:     "who set its fields, and its recent events",
		})
	case len(result.Chain) == 0:
		answer.State, answer.Reason = traceStateNotObservable, traceReasonNoChainObserved
		answer.Summary = fmt.Sprintf("No delivery chain was observed for %s.", target)
	case broken != nil:
		answer.State, answer.Reason = traceStateChainNotReady, traceReasonLinkNotReady
		answer.Summary = fmt.Sprintf("The delivery chain of %s is not ready at %s/%s.", target, broken.Kind, broken.Name)
		statusScope := ""
		if broken.Namespace != "" {
			statusScope = " -n " + broken.Namespace
		}
		answer.Next = append(answer.Next, mapsvc.AnswerNext{
			Command: preferInvocationForm("cub-scout gitops status" + statusScope + " --format json"),
			Why:     fmt.Sprintf("what the controllers report about %s/%s", broken.Kind, broken.Name),
		})
	default:
		answer.State, answer.Reason = traceStateTraced, traceReasonChainReady
		answer.Summary = fmt.Sprintf("%s is delivered through a chain of %d links, each reported ready.", target, len(result.Chain))
	}

	// Everything the trace could not read. For the no-owner answer the
	// message is the finding itself, not something unknown.
	if text := strings.TrimSpace(result.Error); text != "" && answer.State != traceStateNoOwnerMarker {
		answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{
			What:   "part of the trace evidence",
			Kind:   mapsvc.UnknownNotObservable,
			Reason: text,
		})
	}
	return answer
}

// traceLinkBasis says whose word a chain link rests on. The traced resource
// itself is read from its own object. Every link above it is what a
// controller reports about a source or a deployer: its Ready and its
// revision are that controller's statement, which cub-scout relays and has
// not checked against anything else.
func traceLinkBasis(link agent.ChainLink, kind, name, namespace string) mapsvc.AnswerBasis {
	label := link.Kind + "/" + link.Name
	basis := mapsvc.AnswerBasis{RestsOn: mapsvc.RestsOnControllerReport}
	if strings.EqualFold(link.Kind, kind) && link.Name == name && link.Namespace == namespace {
		basis.RestsOn = mapsvc.RestsOnObjectMetadata
	}
	// The claim is in cub-scout's words and says only ready or not. The
	// status, revision and message are the controller's own text: they go in
	// Quoted, word for word, and nowhere else in the answer.
	if link.Ready {
		basis.Claim = label + " is reported ready"
	} else {
		basis.Claim = label + " is reported not ready"
	}
	var quoted []string
	if status := strings.TrimSpace(link.Status); status != "" {
		quoted = append(quoted, "status: "+status)
	}
	if revision := strings.TrimSpace(link.Revision); revision != "" {
		quoted = append(quoted, "revision: "+revision)
	}
	if message := strings.TrimSpace(link.Message); message != "" {
		quoted = append(quoted, "message: "+message)
	}
	basis.Quoted = strings.Join(quoted, "; ")
	// A reference only where an object by that kind and name exists to be
	// fetched. An Argo CD Application's source, a chart, an OCI address are
	// described by a controller; they are not objects.
	if kindToGVR(link.Kind).Resource != "" && traceLinkNameIsAnObjectName(link.Name) && link.Kind != "ConfigHub OCI" {
		basis.Ref = &mapsvc.AnswerRef{Kind: link.Kind, Namespace: link.Namespace, Name: link.Name}
	}
	return basis
}

func traceLinkNameIsAnObjectName(name string) bool {
	return name != "" && !strings.ContainsAny(name, "/: ")
}
