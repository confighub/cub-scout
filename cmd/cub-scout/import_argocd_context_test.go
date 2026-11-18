package main

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// #599: import argocd reads, and with --disable-sync, --delete-app or
// --test-update writes, through one config. With --kube-context that config
// is the selected context's: --list reads only it, and an empty or missing
// name is refused before any read.
func TestImportArgoCDReadsOnlyTheSelectedContext(t *testing.T) {
	selected, ambient := newCountedKubeServer(t), newCountedKubeServer(t)
	path, before := resolverKubeconfig(t, "ambient", map[string]string{"selected": selected.server.URL, "ambient": ambient.server.URL})
	t.Setenv("KUBECONFIG", path)

	cmd := importArgoSubCmd
	flag := cmd.Flags().Lookup("kube-context")
	require.NotNil(t, flag)
	oldList := argoImportList
	argoImportList = true
	t.Cleanup(func() {
		argoImportList = oldList
		_ = flag.Value.Set("")
		flag.Changed = false
	})
	cmd.SetContext(context.Background())

	require.NoError(t, cmd.Flags().Set("kube-context", "selected"))
	var err error
	captureStdout(t, func() { err = runImportArgoCD(cmd, nil) })
	require.NoError(t, err)
	require.NotZero(t, selected.requests.Load(), "the selected context was not read")
	require.Zero(t, ambient.requests.Load(), "the ambient context was read: %v", ambient.requestPaths())

	for _, refused := range []string{"missing", ""} {
		seen := selected.requests.Load()
		require.NoError(t, cmd.Flags().Set("kube-context", refused))
		captureStdout(t, func() { err = runImportArgoCD(cmd, nil) })
		require.Error(t, err, "--kube-context=%q must refuse", refused)
		require.Equal(t, seen, selected.requests.Load(), "--kube-context=%q read a cluster before refusing", refused)
		require.Zero(t, ambient.requests.Load(), "--kube-context=%q fell back to the ambient context", refused)
	}

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after, "the kubeconfig was modified")
}
