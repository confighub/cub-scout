package main

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/confighub/cub-scout/v2/internal/mapsvc"
	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// The state words decided on 2026-11-19, each from the builder of a real
// explain path, every answer checked against the envelope.
func TestExplainAnswerStates(t *testing.T) {
	at := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	fluxChain := []agent.ChainLink{
		{Kind: "GitRepository", Name: "apps", Namespace: "flux-system", Ready: true, Status: "Ready"},
		{Kind: "Kustomization", Name: "apps", Namespace: "flux-system", Ready: true, Status: "Ready"},
		{Kind: "Deployment", Name: "api", Namespace: "prod", Ready: true, Status: "Managed by Flux"},
	}
	fluxObject := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]interface{}{"name": "api", "namespace": "prod", "labels": map[string]interface{}{
			"kustomize.toolkit.fluxcd.io/name": "apps", "kustomize.toolkit.fluxcd.io/namespace": "flux-system",
		}},
	}}
	evidence := agent.BoundedReadEvidence{Resource: agent.BoundedResourceRef{Kind: "Deployment", Name: "api", Namespace: "prod"}, ObservedAt: at}

	cases := []struct {
		name          string
		summary       ExplainSummary
		state, reason string
		check         func(t *testing.T, a *mapsvc.Answer)
	}{
		{"owner and chain read", buildExplainSummary(&agent.TraceResult{Object: agent.ResourceRef{Kind: "Deployment", Name: "api", Namespace: "prod"}, Tool: "flux", DetectedOwner: agent.OwnerFlux, Chain: fluxChain, TracedAt: at}),
			explainStateExplained, explainReasonChainRead, func(t *testing.T, a *mapsvc.Answer) {
				require.Equal(t, "Deployment/api is owned by Flux, through a delivery chain of 3 link(s).", a.Summary)
				require.Equal(t, "its delivery chain was read: GitRepository/apps → Kustomization/apps → Deployment/api", a.Basis[1].Claim)
				require.Equal(t, "cub-scout trace Deployment/api -n prod --format json", a.Next[0].Command)
				require.Equal(t, mapsvc.UnknownNotRequested, a.Unknowns[0].Kind, "readiness is trace's question")
			}},
		{"owner, trace stopped", buildExplainSummary(&agent.TraceResult{Object: agent.ResourceRef{Kind: "Deployment", Name: "api", Namespace: "prod"}, DetectedOwner: agent.OwnerArgo, Error: "Application guestbook not found", TracedAt: at}),
			explainStateOwnerOnly, explainReasonChainNotObserved, func(t *testing.T, a *mapsvc.Answer) {
				require.Equal(t, "Deployment/api carries ArgoCD's owner marker", a.Basis[0].Claim)
				require.Equal(t, "Application guestbook not found", a.Unknowns[0].Quoted)
			}},
		{"no owner marker", buildExplainSummary(&agent.TraceResult{Object: agent.ResourceRef{Kind: "Deployment", Name: "api", Namespace: "prod"}, DetectedOwner: agent.OwnerUnknown, TracedAt: at}),
			explainStateNoOwnerMarker, agent.TraceReasonNoOwnerMarker, nil},
		{"every tracer failed", buildExplainSummaryFromFailure("Deployment", "api", "prod", errors.New("deployments.apps \"api\" is forbidden")),
			explainStateNotObservable, explainReasonObjectNotRead, func(t *testing.T, a *mapsvc.Answer) {
				require.Contains(t, a.Unknowns[0].Quoted, "forbidden")
			}},
		{"bounded read, owner marker", buildBoundedExplainSummary(fluxObject, evidence, nil),
			explainStateOwnerOnly, explainReasonChainNotRequested, func(t *testing.T, a *mapsvc.Answer) {
				require.Equal(t, "2026-11-19T10:00:00Z", a.ObservedAt, "the bounded read's own time")
				require.Contains(t, a.Basis[0].Quoted, "marker: ")
				require.Equal(t, mapsvc.UnknownNotRequested, a.Unknowns[0].Kind)
			}},
		{"bounded read failed", buildBoundedExplainSummary(nil, evidence, errors.New("deployments.apps \"api\" is forbidden")),
			explainStateNotObservable, explainReasonObjectNotRead, func(t *testing.T, a *mapsvc.Answer) {
				require.Equal(t, mapsvc.UnknownDenied, a.Unknowns[0].Kind)
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			answer := buildExplainAnswer(tc.summary, at)
			requireConformingAnswer(t, answer, explainStates, explainReasons)
			require.Equal(t, tc.state, answer.State)
			require.Equal(t, tc.reason, answer.Reason)
			if tc.check != nil {
				tc.check(t, answer)
			}
		})
	}
}

// Live explain JSON carries the answer; recorded input, which observes no
// cluster and has no trusted time, carries none.
func TestExplainJSONCarriesTheAnswerExceptForRecordedInput(t *testing.T) {
	invocation, err := NewInvocationContext("human", TransportCLI)
	require.NoError(t, err)
	live := buildExplainSummary(&agent.TraceResult{Object: agent.ResourceRef{Kind: "Deployment", Name: "api", Namespace: "prod"}, DetectedOwner: agent.OwnerUnknown})
	out := captureStdout(t, func() { require.NoError(t, outputExplainSummary(live, "json", invocation, HintContext{})) })
	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(out), &doc))
	require.Contains(t, doc, "answer")

	recorded := live
	recorded.RecordedInput = &RecordedInputEvidence{Kind: "kubernetes-object-recording"}
	out = captureStdout(t, func() { require.NoError(t, outputExplainSummary(recorded, "json", invocation, HintContext{})) })
	doc = map[string]json.RawMessage{}
	require.NoError(t, json.Unmarshal([]byte(out), &doc))
	require.NotContains(t, doc, "answer")
}
