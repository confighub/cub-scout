package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func setImportContextFlag(t *testing.T, value string) {
	t.Helper()
	flag := importCmd.Flags().Lookup("kube-context")
	require.NotNil(t, flag)
	oldJSON, oldNoLog, oldNamespace, oldWizard, oldDryRun := importJSON, importNoLog, importNamespace, importWizard, importDryRun
	t.Cleanup(func() {
		importJSON, importNoLog, importNamespace, importWizard, importDryRun = oldJSON, oldNoLog, oldNamespace, oldWizard, oldDryRun
		_ = flag.Value.Set("")
		flag.Changed = false
		importClusterConfig, importClusterContext = nil, ""
	})
	require.NoError(t, importCmd.Flags().Set("kube-context", value))
	importCmd.SetContext(context.Background())
}

// #599: with --kube-context the import's discovery reads only that context,
// and an empty or missing name, or --wizard, is refused before any read.
func TestImportDiscoveryReadsOnlyTheSelectedContext(t *testing.T) {
	selected, ambient := newCountedKubeServer(t), newCountedKubeServer(t)
	path, before := resolverKubeconfig(t, "ambient", map[string]string{"selected": selected.server.URL, "ambient": ambient.server.URL})
	t.Setenv("KUBECONFIG", path)

	setImportContextFlag(t, "selected")
	importJSON, importNoLog, importNamespace, importWizard = true, true, "", false
	var err error
	captureStdout(t, func() { err = runImport(importCmd, nil) })
	require.NoError(t, err)
	require.NotZero(t, selected.requests.Load(), "the selected context was not read")
	require.Zero(t, ambient.requests.Load(), "the ambient context was read: %v", ambient.requestPaths())

	for _, tc := range []struct {
		context string
		wizard  bool
	}{{"missing", false}, {"", false}, {"selected", true}} {
		seen := selected.requests.Load()
		require.NoError(t, importCmd.Flags().Set("kube-context", tc.context))
		importWizard = tc.wizard
		captureStdout(t, func() { err = runImport(importCmd, nil) })
		require.Error(t, err, "%+v must refuse", tc)
		require.Equal(t, seen, selected.requests.Load(), "%+v read a cluster before refusing", tc)
		require.Zero(t, ambient.requests.Load(), "%+v fell back to the ambient context", tc)
	}

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after, "the kubeconfig was modified")
}

// The manifest an import uploads is read with kubectl from the selected
// context.
func TestImportFetchManifestNamesTheSelectedContext(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "kubectl.log")
	script := filepath.Join(dir, "kubectl")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho \"$*\" >> \""+logPath+"\"\necho 'apiVersion: apps/v1'\necho 'kind: Deployment'\necho 'metadata:'\necho '  name: api'\n"), 0o755))
	old := importKubectlBinary
	importKubectlBinary = script
	t.Cleanup(func() { importKubectlBinary = old; importClusterContext = "" })

	for _, tc := range []struct{ context, want string }{
		{"selected", "--context selected get deployment api -n shop -o yaml"},
		{"", "get deployment api -n shop -o yaml"},
	} {
		require.NoError(t, os.RemoveAll(logPath))
		importClusterContext = tc.context
		_, err := fetchManifest("Deployment", "shop", "api")
		require.NoError(t, err)
		raw, err := os.ReadFile(logPath)
		require.NoError(t, err)
		require.Equal(t, tc.want, strings.TrimSpace(string(raw)))
	}
}
