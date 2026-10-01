// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/confighub/cub-scout/v2/internal/mapsvc"
	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func prepareTraceSelectionTest(t *testing.T) {
	t.Helper()
	oldNS, oldFormat, oldApp, oldPresentation := traceNamespace, traceFormat, traceApp, tracePresentation
	oldJSON, oldReverse, oldDiff, oldArtifacts, oldConnected := traceJSON, traceReverse, traceDiff, traceArtifacts, traceWithConfigHub
	t.Cleanup(func() {
		traceNamespace, traceFormat, traceApp, tracePresentation = oldNS, oldFormat, oldApp, oldPresentation
		traceJSON, traceReverse, traceDiff, traceArtifacts, traceWithConfigHub = oldJSON, oldReverse, oldDiff, oldArtifacts, oldConnected
	})
	traceNamespace, traceFormat, traceApp, tracePresentation = "delivery", "json", "api", ""
	traceJSON, traceReverse, traceDiff, traceArtifacts, traceWithConfigHub = false, false, false, false, false
	t.Setenv("CUB_SCOUT_TEST_TRACE_JSON", "")
	t.Setenv("CUB_SCOUT_TEST_TRACE_ARTIFACTS_JSON", "")
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")
	t.Setenv("PATH", t.TempDir())
}

func selectedTraceCommand(t *testing.T, selected string) *cobra.Command {
	t.Helper()
	command := &cobra.Command{}
	command.SetContext(context.Background())
	command.Flags().String("kube-context", "", "")
	require.NoError(t, command.Flags().Set("kube-context", selected))
	return command
}

func TestTraceCLIExplicitContextOverridesAmbient(t *testing.T) {
	prepareTraceSelectionTest(t)
	alpha, beta := newTraceSessionFixture(t, "alpha"), newTraceSessionFixture(t, "beta")
	path := filepath.Join(t.TempDir(), "config")
	before := writeTraceKubeconfig(t, path, "beta-context", alpha.server.URL, beta.server.URL)
	t.Setenv("KUBECONFIG", path)
	command := selectedTraceCommand(t, "alpha-context")
	var runErr error
	output := captureStdout(t, func() { runErr = runTrace(command, nil) })
	require.NoError(t, runErr)
	var result mapsvc.TraceOutput
	require.NoError(t, json.Unmarshal([]byte(output), &result))
	require.Equal(t, "alpha-context", result.Context)
	require.NotEmpty(t, result.Chain)
	beta.mu.Lock()
	require.Empty(t, beta.requests)
	beta.mu.Unlock()
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestTraceCLIInvalidSelectionAndCombinationsReadNothing(t *testing.T) {
	for _, tc := range []struct {
		name, selected, fixture, format string
		diff                            bool
		want                            string
	}{
		{name: "empty", selected: "", format: "json", want: "non-empty"},
		{name: "unknown", selected: "missing", format: "json", want: "missing"},
		{name: "fixture", selected: "alpha-context", fixture: "absent-fixture.json", format: "json", want: "fixture input"},
		{name: "delegated diff", selected: "alpha-context", format: "json", diff: true, want: "controller diff binding"},
		{name: "format", selected: "alpha-context", format: "invalid", want: "unsupported trace format"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prepareTraceSelectionTest(t)
			alpha := newTraceSessionFixture(t, "alpha")
			path := filepath.Join(t.TempDir(), "config")
			writeTraceKubeconfig(t, path, "alpha-context", alpha.server.URL, alpha.server.URL)
			t.Setenv("KUBECONFIG", path)
			t.Setenv("CUB_SCOUT_TEST_TRACE_JSON", tc.fixture)
			traceDiff, traceFormat = tc.diff, tc.format
			err := runTrace(selectedTraceCommand(t, tc.selected), nil)
			require.ErrorContains(t, err, tc.want)
			alpha.mu.Lock()
			require.Empty(t, alpha.requests)
			alpha.mu.Unlock()
		})
	}
}

func TestMCPTraceContextTypedAndForwarded(t *testing.T) {
	tool := newMCPGateway(nil).tools["trace"]
	args, err := tool.BuildArgs(map[string]interface{}{"resource": "deployment/api", "context": "alpha-context", "namespace": "team-a"})
	require.NoError(t, err)
	require.Equal(t, []string{"trace", "deployment/api", "--kube-context", "alpha-context", "-n", "team-a", "--format", "json"}, args)
	for _, invalid := range []interface{}{nil, 42, "", " "} {
		_, err := tool.BuildArgs(map[string]interface{}{"resource": "deployment/api", "context": invalid})
		require.Error(t, err)
	}
	properties := tool.Descriptor.InputSchema["properties"].(map[string]interface{})
	require.Equal(t, "string", properties["context"].(map[string]interface{})["type"])
}

func TestTraceCLIReverseExplicitContextOverridesAmbient(t *testing.T) {
	prepareTraceSelectionTest(t)
	traceApp, traceNamespace, traceReverse = "", "team-a", true
	alpha, beta := newTraceSessionFixture(t, "alpha"), newTraceSessionFixture(t, "beta")
	path := filepath.Join(t.TempDir(), "config")
	before := writeTraceKubeconfig(t, path, "beta-context", alpha.server.URL, beta.server.URL)
	t.Setenv("KUBECONFIG", path)
	var runErr error
	output := captureStdout(t, func() { runErr = runTrace(selectedTraceCommand(t, "alpha-context"), []string{"deployment/api"}) })
	require.NoError(t, runErr)
	var result agent.ReverseTraceResult
	require.NoError(t, json.Unmarshal([]byte(output), &result))
	require.Equal(t, "alpha-context", result.Context)
	require.NotEmpty(t, result.K8sChain)
	beta.mu.Lock()
	require.Empty(t, beta.requests)
	beta.mu.Unlock()
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestTraceCLIRejectsExplicitContextWithArtifactFixture(t *testing.T) {
	prepareTraceSelectionTest(t)
	t.Setenv("CUB_SCOUT_TEST_TRACE_ARTIFACTS_JSON", "absent-artifact.json")
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "missing-config"))
	require.ErrorContains(t, runTrace(selectedTraceCommand(t, "selected"), nil), "fixture input")
}

func TestTraceCLILegacyReversePrecedesDiff(t *testing.T) {
	prepareTraceSelectionTest(t)
	traceApp, traceNamespace, traceReverse, traceDiff = "", "team-a", true, true
	alpha := newTraceSessionFixture(t, "alpha")
	path := filepath.Join(t.TempDir(), "config")
	writeTraceKubeconfig(t, path, "alpha-context", alpha.server.URL, alpha.server.URL)
	t.Setenv("KUBECONFIG", path)
	command := &cobra.Command{}
	command.SetContext(context.Background())
	var runErr error
	output := captureStdout(t, func() { runErr = runTrace(command, []string{"deployment/api"}) })
	require.NoError(t, runErr)
	var result agent.ReverseTraceResult
	require.NoError(t, json.Unmarshal([]byte(output), &result))
	require.Equal(t, "alpha-context", result.Context)
	require.NotEmpty(t, result.K8sChain)
}

func TestTraceScopedFollowupHintsPreserveOrWithholdBinding(t *testing.T) {
	invCtx, err := NewInvocationContext("", TransportCLI)
	require.NoError(t, err)
	var out bytes.Buffer
	result := &agent.TraceResult{Context: "selected", Object: agent.ResourceRef{Kind: "Deployment", Name: "api", Namespace: "team-a"}, Chain: []agent.ChainLink{{Kind: "Deployment", Name: "api", Namespace: "team-a", Ready: true}}}
	require.NoError(t, renderTraceHuman(&out, result, nil, invCtx, traceHumanOptions{Explain: true}))
	require.Contains(t, out.String(), "follow-ups are withheld")
	require.NotContains(t, out.String(), "cub-scout map orphans")
	require.NotContains(t, out.String(), "--diff")
	out.Reset()
	reverse := &agent.ReverseTraceResult{Context: "team's cluster", Owner: "flux", TopResource: &agent.ResourceRef{Kind: "Deployment", Name: "api", Namespace: "team-a"}}
	require.NoError(t, renderReverseTraceHuman(&out, reverse, false))
	require.Contains(t, out.String(), "--kube-context 'team'\"'\"'s cluster'")
	out.Reset()
	reverse.Context = "in-cluster"
	require.NoError(t, renderReverseTraceHuman(&out, reverse, false))
	require.Contains(t, out.String(), "Follow-up command withheld")
	require.NotContains(t, out.String(), "./cub-scout trace")
}

func TestReverseTraceSecretOmissionPreservedAcrossRenderers(t *testing.T) {
	result := &agent.ReverseTraceResult{Object: agent.ResourceRef{Kind: "Secret", Name: "credentials", Namespace: "team-a"}, Owner: "native", OrphanMeta: &agent.OrphanMetadata{LastAppliedConfigOmission: "Secret last-applied configuration omitted because it may contain payloads"}}
	for _, format := range []string{"ascii", "md", "json"} {
		t.Run(format, func(t *testing.T) {
			var out bytes.Buffer
			var err error
			switch format {
			case "ascii":
				err = renderReverseTraceHuman(&out, result, false)
			case "md":
				err = renderReverseTraceMarkdown(&out, result)
			case "json":
				err = outputReverseTraceJSONTo(&out, result)
			}
			require.NoError(t, err)
			require.Contains(t, out.String(), result.OrphanMeta.LastAppliedConfigOmission)
			require.NotContains(t, out.String(), "No last-applied-configuration annotation was found")
		})
	}
}
