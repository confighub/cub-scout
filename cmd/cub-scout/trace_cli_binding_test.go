// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/confighub/cub-scout/v2/internal/mapsvc"
	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTraceCLIApplicationUsesKubernetesBindingWithoutArgoLogin(t *testing.T) {
	alpha := newTraceSessionFixture(t, "alpha")
	path := filepath.Join(t.TempDir(), "config")
	writeTraceKubeconfig(t, path, "alpha-context", alpha.server.URL, alpha.server.URL)
	t.Setenv("KUBECONFIG", path)
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")
	t.Setenv("CUB_SCOUT_TEST_TRACE_JSON", "")
	t.Setenv("PATH", t.TempDir()) // No Argo/Flux/kubectl binary is available.
	oldNamespace, oldFormat, oldApp, oldPresentation := traceNamespace, traceFormat, traceApp, tracePresentation
	oldJSON, oldReverse, oldDiff, oldArtifacts, oldConnected := traceJSON, traceReverse, traceDiff, traceArtifacts, traceWithConfigHub
	t.Cleanup(func() {
		traceNamespace, traceFormat, traceApp, tracePresentation = oldNamespace, oldFormat, oldApp, oldPresentation
		traceJSON, traceReverse, traceDiff, traceArtifacts, traceWithConfigHub = oldJSON, oldReverse, oldDiff, oldArtifacts, oldConnected
	})
	traceNamespace, traceFormat, traceApp, tracePresentation = "delivery", "json", "api", ""
	traceJSON, traceReverse, traceDiff, traceArtifacts, traceWithConfigHub = false, false, false, false, false
	command := &cobra.Command{}
	command.SetContext(context.Background())
	var runErr error
	output := captureStdout(t, func() { runErr = runTrace(command, nil) })
	require.NoError(t, runErr)
	var result mapsvc.TraceOutput
	require.NoError(t, json.Unmarshal([]byte(output), &result))
	require.Equal(t, "alpha-context", result.Context)
	require.Equal(t, "delivery", result.Target.Namespace)
	require.Equal(t, "api", result.Target.Name)
	require.NotEmpty(t, result.Chain)
	require.NotEmpty(t, result.Warnings)
	require.Contains(t, result.Warnings[0], "Events unavailable")
}

func TestTraceJSONPreservesPartialWarningAndContext(t *testing.T) {
	result := &agent.TraceResult{
		Context: "selected-context", Error: "Events unavailable: forbidden",
		Chain: []agent.ChainLink{{Kind: "Application", Name: "api", Namespace: "delivery"}},
	}
	output := convertTraceToV014(result, "Application", "api", "delivery", nil)
	assert.Equal(t, "selected-context", output.Context)
	assert.Equal(t, []string{"Events unavailable: forbidden"}, output.Warnings)
	require.Len(t, output.Chain, 1)
}
