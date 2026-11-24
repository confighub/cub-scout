package main

// The answer block on `explain --format json`: who owns this, and how do I
// know? The state words were decided by the maintainer on 2026-11-19.
// Readiness is trace's and gitops status's question, not this one.

import (
	"fmt"
	"strings"
	"time"

	"github.com/confighub/cub-scout/v3/internal/mapsvc"
	"github.com/confighub/cub-scout/v3/pkg/agent"
)

const (
	explainStateExplained     = "explained"
	explainStateOwnerOnly     = "owner-only"
	explainStateNoOwnerMarker = "no-owner-marker"
	explainStateNotObservable = "not-observable"

	explainReasonChainRead         = "owner-and-chain-read"
	explainReasonChainNotRequested = "chain-not-requested"
	explainReasonChainNotObserved  = "chain-not-observed"
	explainReasonObjectNotRead     = "object-not-read"
)

// explainStates and explainReasons are what the conformance test checks
// answers against. Adding a value means documenting it in json-contracts.md.
var (
	explainStates  = []string{explainStateExplained, explainStateOwnerOnly, explainStateNoOwnerMarker, explainStateNotObservable}
	explainReasons = []string{explainReasonChainRead, explainReasonChainNotRequested, explainReasonChainNotObserved, agent.TraceReasonNoOwnerMarker, explainReasonObjectNotRead}
)

// explainAnswerFacts are what each explain path knows, set where it learns
// it. The answer is built from these alone.
type explainAnswerFacts struct {
	objectRead        bool
	readError         string
	ownerType         string
	ownerSource       string
	chain             []agent.ChainLink
	chainError        string
	chainNotRequested bool
	observedAt        time.Time
	// cost is set where the reads ran on a metered session.
	cost *agent.KubernetesReadCost
}

// explainCostCoverage is a key, spelled out in json-contracts.md: the
// Kubernetes requests of explain's session clients, on the explicit-context
// path. The default path's reads are not metered yet.
const explainCostCoverage = "explain-session-clients"

func readErrString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// explainFactsFromTrace takes the facts from a trace result. Ownership
// detection reads the object, and a trace records the owner only when it
// did, so a detected owner means the object was read.
func explainFactsFromTrace(result *agent.TraceResult) explainAnswerFacts {
	facts := explainAnswerFacts{observedAt: result.TracedAt}
	facts.ownerType = strings.TrimSpace(result.DetectedOwner)
	facts.objectRead = facts.ownerType != ""
	facts.chain = result.Chain
	facts.chainError = strings.TrimSpace(result.Error)
	if facts.ownerType == "" && len(result.Chain) > 0 {
		// A tracer found a chain without a detection record: the object was
		// read, and its owner is the tracer's tool.
		facts.objectRead = true
		facts.ownerType = strings.ToLower(strings.TrimSpace(result.Tool))
	}
	return facts
}

func explainOwnerRecognised(ownerType string) bool {
	switch strings.ToLower(strings.TrimSpace(ownerType)) {
	case "", agent.OwnerUnknown, agent.OwnerKubernetes, "native", "kubernetes":
		return false
	}
	return true
}

// buildExplainAnswer puts what explain found into the common envelope.
func buildExplainAnswer(summary ExplainSummary, observedAt time.Time) *mapsvc.Answer {
	f := summary.facts
	if !f.observedAt.IsZero() {
		observedAt = f.observedAt
	}
	answer := &mapsvc.Answer{
		Cluster:    mapsvc.AnswerCluster{Context: summary.KubernetesContext, IDSource: "not-read"},
		ObservedAt: observedAt.UTC().Format(time.RFC3339),
		Basis:      []mapsvc.AnswerBasis{},
		Unknowns:   []mapsvc.AnswerUnknown{},
		Next:       []mapsvc.AnswerNext{},
		Cost:       mapsvc.AnswerCost{Coverage: "not-measured"},
	}
	if f.cost != nil {
		answer.Cost = mapsvc.AnswerCostFrom(*f.cost)
	}
	kind, name, _ := strings.Cut(summary.Resource, "/")
	label, named := kind+"/"+name, traceSafeKind.MatchString(kind) && traceSafeName.MatchString(name)
	if !named {
		label = "the resource"
	}
	scope := ""
	if summary.Namespace != "" && traceSafeName.MatchString(summary.Namespace) {
		scope = " -n " + summary.Namespace
	}
	traceNext := func(why string) {
		if named {
			answer.Next = append(answer.Next, mapsvc.AnswerNext{Command: preferInvocationForm("cub-scout trace " + label + scope + " --format json"), Why: why})
		}
	}

	if !f.objectRead {
		answer.State, answer.Reason = explainStateNotObservable, explainReasonObjectNotRead
		answer.Summary = fmt.Sprintf("%s could not be read, so who owns it is not known.", label)
		unknown := mapsvc.AnswerUnknown{What: "the object and its owner marker", Kind: mapsvc.UnknownNotObservable, Reason: "the read failed; the error is quoted"}
		if f.readError != "" {
			unknown.Quoted = f.readError
			if strings.Contains(strings.ToLower(f.readError), "forbidden") {
				unknown.Kind = mapsvc.UnknownDenied
			}
		} else if f.chainError != "" {
			unknown.Quoted = f.chainError
		}
		answer.Unknowns = append(answer.Unknowns, unknown)
		return answer
	}

	if !explainOwnerRecognised(f.ownerType) {
		answer.State, answer.Reason = explainStateNoOwnerMarker, agent.TraceReasonNoOwnerMarker
		answer.Summary = fmt.Sprintf("No GitOps owner marker was recognised on %s; that does not show nothing manages it.", label)
		answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{Claim: label + " carries no owner marker cub-scout recognises", RestsOn: mapsvc.RestsOnObjectMetadata})
		if summary.MutationManager != "" {
			answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{Claim: "its fields were last written by the field manager quoted here", RestsOn: mapsvc.RestsOnObjectMetadata, Quoted: "manager: " + summary.MutationManager})
		}
		return answer
	}

	owner := mapsvc.DisplayOwner(f.ownerType)
	ownerBasis := mapsvc.AnswerBasis{Claim: fmt.Sprintf("%s carries %s's owner marker", label, owner), RestsOn: mapsvc.RestsOnObjectMetadata}
	if source := strings.TrimSpace(f.ownerSource); source != "" {
		ownerBasis.Quoted = "marker: " + source
	}
	if !traceSafeKind.MatchString(owner) {
		ownerBasis.Claim = label + " carries an owner marker cub-scout recognises"
		ownerBasis.Quoted = strings.TrimSpace(ownerBasis.Quoted + "; owner: " + owner)
	}
	answer.Basis = append(answer.Basis, ownerBasis)

	switch {
	case len(f.chain) > 0 && f.chainError == "":
		answer.State, answer.Reason = explainStateExplained, explainReasonChainRead
		var links []string
		for i, link := range f.chain {
			linkLabel, ok := traceLinkLabel(link, i)
			if !ok {
				linkLabel = fmt.Sprintf("link %d", i+1)
			}
			links = append(links, linkLabel)
		}
		answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{Claim: "its delivery chain was read: " + strings.Join(links, " → "), RestsOn: mapsvc.RestsOnObjectMetadata})
		answer.Summary = fmt.Sprintf("%s is owned by %s, through a delivery chain of %d link(s).", label, ownerWord(owner), len(f.chain))
		answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{What: "whether the chain's controllers report ready", Kind: mapsvc.UnknownNotRequested, Reason: "explain answers who owns it; trace reads readiness"})
		traceNext("whether its delivery controllers report ready")
	case f.chainNotRequested:
		answer.State, answer.Reason = explainStateOwnerOnly, explainReasonChainNotRequested
		answer.Summary = fmt.Sprintf("%s carries %s's owner marker; its delivery chain was not read.", label, ownerWord(owner))
		answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{What: "its delivery chain and source", Kind: mapsvc.UnknownNotRequested, Reason: "this read covers the object only"})
		traceNext("its delivery chain and what its controllers report")
	default:
		answer.State, answer.Reason = explainStateOwnerOnly, explainReasonChainNotObserved
		answer.Summary = fmt.Sprintf("%s carries %s's owner marker, and no complete delivery chain was observed for it.", label, ownerWord(owner))
		unknown := mapsvc.AnswerUnknown{What: "its delivery chain and source", Kind: mapsvc.UnknownNotObservable, Reason: "no complete chain was observed"}
		if f.chainError != "" {
			unknown.Reason, unknown.Quoted = "the trace stopped; the error is quoted", f.chainError
		}
		answer.Unknowns = append(answer.Unknowns, unknown)
	}
	return answer
}

func ownerWord(owner string) string {
	if traceSafeKind.MatchString(owner) {
		return owner
	}
	return "a recognised owner"
}
