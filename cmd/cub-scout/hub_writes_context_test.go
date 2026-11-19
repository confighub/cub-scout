package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func fakeHubKubectl(t *testing.T) (logPath string) {
	t.Helper()
	dir := t.TempDir()
	logPath = filepath.Join(dir, "kubectl.log")
	script := filepath.Join(dir, "kubectl")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho \"$*\" >> \""+logPath+"\"\n"), 0o755))
	old := hubKubectlBinary
	hubKubectlBinary = script
	t.Cleanup(func() { hubKubectlBinary = old })
	return logPath
}

func hubKubectlCalls(t *testing.T, logPath string) []string {
	t.Helper()
	raw, err := os.ReadFile(logPath)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}

// #955: map hub's writes go only to a cluster named with --kube-context.
// Without one, every write is refused and nothing is written.
func TestMapHubWritesAreRefusedWithoutANamedCluster(t *testing.T) {
	old := hubBinding
	hubBinding = nil
	t.Cleanup(func() { hubBinding = old })
	logPath := fakeHubKubectl(t)
	workloads := []WorkloadInfo{{Kind: "Deployment", Name: "api", Namespace: "shop"}}

	require.ErrorIs(t, disableArgoSyncCmd("argocd", "guestbook")().(argoSyncDisabledMsg).err, errHubWriteNeedsContext)
	require.ErrorIs(t, deleteArgoAppCmd("argocd", "guestbook")().(argoAppDeletedMsg).err, errHubWriteNeedsContext)
	require.ErrorIs(t, applyUnitCmd("space", "api", workloads)().(unitAppliedMsg).err, errHubWriteNeedsContext)
	require.ErrorIs(t, labelWorkload("Deployment", "shop", "api", "api"), errHubWriteNeedsContext)
	for name, msg := range map[string]importCompleteMsg{
		"import":                 importWorkloadsCmd("space", workloads)().(importCompleteMsg),
		"import argo":            importArgoWorkloadsCmd("space", "guestbook", workloads)().(importCompleteMsg),
		"import with suggestion": importWorkloadsWithSuggestionCmd("space", &ImportSuggestion{}, workloads)().(importCompleteMsg),
	} {
		require.ErrorIs(t, msg.applyError, errHubWriteNeedsContext, name)
		require.Zero(t, msg.success, name)
	}
	require.Empty(t, hubKubectlCalls(t, logPath), "kubectl wrote without a named cluster")
}

// With a named cluster, the writes go there: kubectl names its context, and
// the Argo CD patch reaches the selected API server only.
func TestMapHubWritesGoToTheNamedCluster(t *testing.T) {
	selected, ambient := newCountedKubeServer(t), newCountedKubeServer(t)
	path, _ := resolverKubeconfig(t, "ambient", map[string]string{"selected": selected.server.URL, "ambient": ambient.server.URL})
	t.Setenv("KUBECONFIG", path)
	binding := resolveLocalClusterBindingForSelection(clusterContextSelection{name: "selected", explicit: true})
	require.NoError(t, binding.err)
	old := hubBinding
	hubBinding = binding
	t.Cleanup(func() { hubBinding = old })
	logPath := fakeHubKubectl(t)

	_ = applyUnitCmd("space", "api", []WorkloadInfo{{Kind: "Deployment", Name: "api", Namespace: "shop"}})()
	require.NoError(t, labelWorkload("Deployment", "shop", "api", "api"))
	calls := hubKubectlCalls(t, logPath)
	require.Len(t, calls, 4, "three cleanup writes and one label: %v", calls)
	for _, call := range calls {
		require.True(t, strings.HasPrefix(call, "--context selected "), "kubectl wrote without the named cluster: %q", call)
	}

	_ = disableArgoSyncCmd("argocd", "guestbook")()
	require.NotZero(t, selected.requests.Load(), "the Argo CD patch did not reach the named cluster")
	require.Zero(t, ambient.requests.Load(), "the Argo CD patch reached the current context: %v", ambient.requestPaths())
}

// map hub --kube-context binds the hub; an unknown name is refused first.
func TestMapHubKubeContextFlag(t *testing.T) {
	path, _ := resolverKubeconfig(t, "ambient", map[string]string{"selected": "https://127.0.0.1:1", "ambient": "https://127.0.0.1:2"})
	t.Setenv("KUBECONFIG", path)
	old := hubBinding
	t.Cleanup(func() { hubBinding = old })
	flag := mapHubCmd.Flags().Lookup("kube-context")
	require.NotNil(t, flag)
	t.Cleanup(func() { _ = flag.Value.Set(""); flag.Changed = false })
	mapHubCmd.SetContext(context.Background())

	hubBinding = nil
	require.NoError(t, mapHubCmd.Flags().Set("kube-context", "selected"))
	require.NoError(t, bindHub(mapHubCmd))
	require.NotNil(t, hubBinding)
	require.Equal(t, "selected", hubBinding.context)

	hubBinding = nil
	require.NoError(t, mapHubCmd.Flags().Set("kube-context", "missing"))
	require.Error(t, bindHub(mapHubCmd))
	require.Nil(t, hubBinding)
}
