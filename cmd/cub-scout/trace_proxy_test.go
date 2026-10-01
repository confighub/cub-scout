// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
)

func TestTraceChildRetainsParsedProxyFromSelectedBinding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	writeTraceKubeconfig(t, path, "beta-context", "https://alpha.invalid", "https://beta.invalid")
	raw, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	raw.Clusters["alpha"].ProxyURL = "http://proxy-alpha.invalid:8080"
	raw.Clusters["beta"].ProxyURL = "http://proxy-beta.invalid:8080"
	data, err := clientcmd.Write(*raw)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0600))
	t.Setenv("KUBECONFIG", path)
	binding := resolveLocalClusterBindingForSelection(clusterContextSelection{name: "alpha-context", explicit: true})
	require.NoError(t, binding.err)
	session, err := newTraceSessionFromBinding(binding)
	require.NoError(t, err)
	// Both selection and static proxy come from that same loaded snapshot.
	raw.Clusters["alpha"].ProxyURL = "http://changed.invalid:8080"
	data, err = clientcmd.Write(*raw)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0600))
	child, err := session.createChildKubeconfig()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, child.Cleanup()) })
	captured, err := clientcmd.LoadFromFile(child.Path)
	require.NoError(t, err)
	selected := captured.Contexts[child.Context]
	require.NotNil(t, selected)
	require.Equal(t, "https://alpha.invalid", captured.Clusters[selected.Cluster].Server)
	require.Equal(t, "http://proxy-alpha.invalid:8080", captured.Clusters[selected.Cluster].ProxyURL)
	unchanged, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, data, unchanged)
}
