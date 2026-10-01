// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"net/http"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestTraceTUIActionRetainsBindingAfterKubeconfigRetarget(t *testing.T) {
	alpha := newTraceSessionFixture(t, "alpha")
	beta := newTraceSessionFixture(t, "beta")
	path := filepath.Join(t.TempDir(), "config")
	writeTraceKubeconfig(t, path, "alpha-context", alpha.server.URL, beta.server.URL)
	t.Setenv("KUBECONFIG", path)
	binding := resolveLocalClusterBindingForSelection(clusterContextSelection{name: "alpha-context", explicit: true})
	require.NoError(t, binding.err)
	model := LocalClusterModel{
		clusterBinding: binding, explicitClusterContext: true, keymap: defaultLocalKeyMap(),
		entries: []MapEntry{{Kind: "Application", Name: "api", Namespace: "delivery", Owner: "ArgoCD"}},
	}
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
	require.Nil(t, command)
	model = updated.(LocalClusterModel)
	require.True(t, model.traceMode)
	require.Len(t, model.traceItems, 1)
	for attempt := 0; attempt < 2; attempt++ {
		if attempt == 1 {
			writeTraceKubeconfig(t, path, "beta-context", alpha.server.URL, beta.server.URL)
			updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
			model = updated.(LocalClusterModel)
		}
		before := alpha.count(http.MethodGet, "/apis/argoproj.io/v1alpha1/namespaces/delivery/applications/api")
		updated, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model = updated.(LocalClusterModel)
		require.NotNil(t, command)
		message := command().(traceResultMsg)
		require.NoError(t, message.err)
		require.Contains(t, message.output, "Selected Kubernetes context: alpha-context")
		require.Contains(t, message.output, "Events unavailable")
		require.Contains(t, message.output, "api")
		updated, _ = model.Update(message)
		model = updated.(LocalClusterModel)
		require.False(t, model.traceLoading)
		require.Contains(t, model.renderTrace(), "alpha-context")
		require.Greater(t, alpha.count(http.MethodGet, "/apis/argoproj.io/v1alpha1/namespaces/delivery/applications/api"), before)
	}
	beta.mu.Lock()
	defer beta.mu.Unlock()
	require.Empty(t, beta.requests)
}
