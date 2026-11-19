package main

// Conformance across surfaces (#596): one trace result, the same facts in
// CLI JSON, MCP, and the human renderings (CLI ASCII and Markdown, and the
// TUI, which uses the ASCII renderer). The JSON `answer` is the reference:
// a human surface may say less, never more.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/v2/internal/mapsvc"
	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;]*m")

// humanTraceSurfaces renders one result the ways a person sees it.
func humanTraceSurfaces(t *testing.T, result *agent.TraceResult) map[string]string {
	t.Helper()
	out := map[string]string{}
	for name, transport := range map[string]Transport{"cli-ascii": TransportCLI, "tui": TransportTUI} {
		invocation, err := NewInvocationContext("human", transport)
		require.NoError(t, err)
		var b strings.Builder
		require.NoError(t, renderTraceHuman(&b, result, nil, invocation, traceHumanOptions{Limit: 10}))
		out[name] = ansiEscape.ReplaceAllString(b.String(), "")
	}
	invocation, err := NewInvocationContext("human", TransportCLI)
	require.NoError(t, err)
	out["cli-md"] = captureStdout(t, func() { require.NoError(t, outputTraceMarkdown(result, nil, invocation)) })
	return out
}

// The recorded ModelDeployment from the Modelplane lane (run 44287261221)
// reports Ready=False (Creating). Trace finds its owner from metadata and
// reads no controller's readiness: the answer says readiness-not-read. The
// human surfaces said "All levels in sync", which neither the answer nor the
// object's own status supports.
func recordedModelplaneTrace(t *testing.T) *agent.TraceResult {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "modelplane-lane-modeldeployment.json"))
	require.NoError(t, err)
	obj := &unstructured.Unstructured{}
	require.NoError(t, json.Unmarshal(raw, &obj.Object))
	listKinds := map[schema.GroupVersionResource]string{
		{Group: "modelplane.ai", Version: "v1alpha1", Resource: "modelreplicas"}:  "ModelReplicaList",
		{Group: "modelplane.ai", Version: "v1alpha1", Resource: "modelendpoints"}: "ModelEndpointList",
	}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds, obj)
	result := buildModelplaneObservedTraceResult(context.Background(), client, "ModelDeployment", "smollm", "ml-team", &agent.Ownership{
		Type: agent.OwnerModelplane, SubType: "modeldeployment", Name: "smollm", Namespace: "ml-team", Source: "apiGroup:modelplane.ai",
	})
	result.TracedAt = time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	return result
}

func TestTraceHumanSurfacesSayInSyncOnlyWhenTheAnswerSaysTraced(t *testing.T) {
	at := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	cases := map[string]*agent.TraceResult{
		"recorded modelplane, not ready": recordedModelplaneTrace(t),
		"flux, controllers ready": {Tool: "flux", TracedAt: at, FullyManaged: true, Object: agent.ResourceRef{Kind: "Deployment", Name: "api", Namespace: "prod"}, Chain: []agent.ChainLink{
			{Kind: "GitRepository", Name: "apps", Namespace: "flux-system", Ready: true, Status: "Ready"},
			{Kind: "Kustomization", Name: "apps", Namespace: "flux-system", Ready: true, Status: "Ready"},
			{Kind: "Deployment", Name: "api", Namespace: "prod", Ready: true, Status: "Managed by Flux"},
		}},
		"sveltos, owner by annotation only": {Tool: "sveltos", TracedAt: at, FullyManaged: true, Object: agent.ResourceRef{Kind: "Deployment", Name: "web", Namespace: "prod"}, Chain: []agent.ChainLink{
			{Kind: "ClusterProfile", Name: "web", Ready: true, Status: "Matched"},
			{Kind: "Deployment", Name: "web", Namespace: "prod", Ready: true, Status: "Available"},
		}},
	}
	for name, result := range cases {
		t.Run(name, func(t *testing.T) {
			answer := buildTraceAnswer(result, result.Object.Kind, result.Object.Name, result.Object.Namespace)
			requireConformingAnswer(t, answer, traceStates, traceReasons)
			for surface, text := range humanTraceSurfaces(t, result) {
				inSync := strings.Contains(text, "All levels in sync")
				if answer.State == traceStateTraced {
					require.True(t, inSync, "%s: the answer says traced and the %s surface does not say so:\n%s", name, surface, text)
					continue
				}
				require.False(t, inSync, "%s: the answer is %s, and the %s surface says all levels are in sync:\n%s", name, answer.State, surface, text)
				require.Contains(t, text, answer.Summary, "%s: the %s surface must say what the answer says", name, surface)
			}
		})
	}
}

func TestRecordedModelplaneTraceAnswer(t *testing.T) {
	result := recordedModelplaneTrace(t)
	require.True(t, result.FullyManaged, "the builder still marks an owned object fully managed; the renderers must not take that as readiness")
	require.Len(t, result.Chain, 1)
	require.False(t, result.Chain[0].Ready, "the recorded object reports Ready=False")
	require.Equal(t, traceReadyReporterCrossplane, result.Chain[0].ReadyReporter, "its Ready condition is Crossplane's")

	// #948: Crossplane's Ready condition is a controller's report. The lane
	// recorded readiness-not-read before this; the answer now says what
	// Crossplane says, and quotes its reason.
	answer := buildTraceAnswer(result, "ModelDeployment", "smollm", "ml-team")
	requireConformingAnswer(t, answer, traceStates, traceReasons)
	require.Equal(t, traceStateChainNotReady, answer.State)
	require.Equal(t, traceReasonLinkNotReady, answer.Reason)
	require.Equal(t, "Crossplane reports ModelDeployment/smollm, in the chain of ModelDeployment/smollm in ml-team, not ready.", answer.Summary)
	require.Len(t, answer.Basis, 1)
	require.Equal(t, mapsvc.RestsOnControllerReport, answer.Basis[0].RestsOn)
	require.Equal(t, "ModelDeployment/smollm is reported not ready by Crossplane", answer.Basis[0].Claim)
	require.Contains(t, answer.Basis[0].Quoted, "reason: Creating")
	require.Len(t, answer.Next, 1)
	require.Contains(t, answer.Next[0].Command, "explain ModelDeployment/smollm -n ml-team", "gitops status does not read composites")
}

// A composite without the crossplane.io/composite label, or without a Ready
// condition, is not Crossplane's word: the answer still says it was not read.
func TestTraceAnswerReadsReadyOnlyFromACrossplaneComposite(t *testing.T) {
	for name, obj := range map[string]map[string]interface{}{
		"no composite label": {"metadata": map[string]interface{}{"name": "x", "namespace": "ns"}, "status": map[string]interface{}{"conditions": []interface{}{map[string]interface{}{"type": "Ready", "status": "False"}}}},
		"no Ready condition": {"metadata": map[string]interface{}{"name": "x", "namespace": "ns", "labels": map[string]interface{}{"crossplane.io/composite": "x"}}, "status": map[string]interface{}{"conditions": []interface{}{map[string]interface{}{"type": "Synced", "status": "False"}}}},
	} {
		obj["apiVersion"], obj["kind"] = "example.org/v1", "XThing"
		link := chainLinkFromObject(&unstructured.Unstructured{Object: obj})
		require.Empty(t, link.ReadyReporter, name)
		require.Equal(t, traceLinkNotRead, traceReadinessOf(link, "crossplane"), name)
	}
}

// MCP carries the CLI's JSON: the answer an agent reads through a tool is
// the one the CLI printed, field for field, in the text and, where the tool
// adds it, in structuredContent.
func TestAnswerIsTheSameThroughMCP(t *testing.T) {
	result := recordedModelplaneTrace(t)
	traceJSON, err := json.Marshal(convertTraceToV014(result, "ModelDeployment", "smollm", "ml-team", nil))
	require.NoError(t, err)

	evidence := agent.Derive(agent.StrategyConfigHubOCIFlux, agent.SourceTruthSurfaces{
		Controller: &agent.ControllerSurface{Kind: "Flux", Source: "oci://registry/space/demo", RevisionOrDigest: "latest@sha256:0828", Health: "Ready"},
		Runtime:    &agent.RuntimeSurface{Resource: "Deployment/api in demo", Field: "spec.template.spec.containers[0].image", Value: "example/api:1", Health: "Current"},
	})
	var sourceTruth strings.Builder
	require.NoError(t, outputSourceTruthAnswered(&sourceTruth, evidence, "json",
		buildSourceTruthAnswer(evidence, agent.StrategyConfigHubOCIFlux, "Deployment", "api", "demo", observedAtForTests)))

	gitopsSummary := GitOpsSummary{Context: "prod-east", Deployers: []DeployerStatus{{Kind: "Kustomization", Name: "apps", Namespace: "flux-system", Ready: false, Reason: "BuildFailed", Stage: "build"}}}
	gitopsSummary.Answer = buildGitOpsStatusAnswer(gitopsSummary, observedAtForTests, nil)
	gitopsJSON, err := json.Marshal(gitopsSummary)
	require.NoError(t, err)

	explainInvocation, err := NewInvocationContext("human", TransportCLI)
	require.NoError(t, err)
	explainJSON := captureStdout(t, func() {
		require.NoError(t, outputExplainSummary(buildExplainSummary(result), "json", explainInvocation, HintContext{}))
	})

	doctorSummary := DoctorSummary{Namespace: "prod", Resources: DoctorResourceSummary{Total: 4}, Risks: DoctorRiskSummary{Warning: 1}}
	doctorSummary.Answer = buildDoctorAnswer(doctorSummary, observedAtForTests)
	doctorJSON, err := json.Marshal(doctorSummary)
	require.NoError(t, err)

	mapSummaryDoc := buildMapListSummary([]MapEntry{{Kind: "Deployment", Name: "api", Namespace: "prod", Owner: "Flux"}})
	mapSummaryDoc.Answer = buildMapListAnswer([]MapEntry{{Kind: "Deployment", Name: "api", Namespace: "prod", Owner: "Flux"}}, nil, "", nil, observedAtForTests)
	mapJSONDoc, err := json.Marshal(mapSummaryDoc)
	require.NoError(t, err)

	for tool, cli := range map[string]string{"trace": string(traceJSON), "compare_source_truth": sourceTruth.String(), "gitops_status": string(gitopsJSON), "explain": explainJSON, "doctor": string(doctorJSON), "map": string(mapJSONDoc)} {
		t.Run(tool, func(t *testing.T) {
			var cliDoc struct {
				Answer json.RawMessage `json:"answer"`
			}
			require.NoError(t, json.Unmarshal([]byte(cli), &cliDoc))
			require.NotEmpty(t, cliDoc.Answer, "the CLI JSON carries an answer")

			runner := func(context.Context, []string) (string, error) { return strings.TrimSpace(cli), nil }
			// compare_source_truth is offered only with ConfigHub connected.
			gateway := newMCPGatewayWithMode(runner, runner, tool == "compare_source_truth")
			tool, ok := gateway.tools[tool]
			require.True(t, ok)
			reply := gateway.runNamedTool(context.Background(), tool.Descriptor.Name, tool, map[string]interface{}{"kind": "Deployment", "name": "api", "namespace": "demo", "resource": "deployment/api", "target": "deployment/api", "strategy": "confighub-oci-flux"})
			require.Equal(t, false, reply["isError"], "reply: %v", reply)
			content := reply["content"].([]map[string]string)
			var mcpDoc struct {
				Answer json.RawMessage `json:"answer"`
			}
			require.NoError(t, json.Unmarshal([]byte(content[0]["text"]), &mcpDoc))
			require.JSONEq(t, string(cliDoc.Answer), string(mcpDoc.Answer), "MCP text must carry the CLI's answer unchanged")

			if structured, ok := reply["structuredContent"]; ok {
				raw, err := json.Marshal(structured)
				require.NoError(t, err)
				var wrapped struct {
					Data struct {
						Answer json.RawMessage `json:"answer"`
					} `json:"data"`
				}
				require.NoError(t, json.Unmarshal(raw, &wrapped))
				require.JSONEq(t, string(cliDoc.Answer), string(wrapped.Data.Answer), "structuredContent must carry the CLI's answer unchanged")
			}
		})
	}
}

var _ = mapsvc.Answer{}
