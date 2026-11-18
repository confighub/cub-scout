package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// #599: with --kube-context every read of the quickstart tour goes to that
// context: the kubectl probe, the inventory, the scan and the explain. An
// empty or missing name is refused before any read.
func TestQuickstartReadsOnlyTheSelectedContext(t *testing.T) {
	t.Setenv("CUB_SCOUT_TEST_QUICKSTART_JSON", "")
	// The selected cluster holds one Deployment, so the tour picks it and
	// explains it: the explain's reads are then checked too.
	selected, ambient := newDeploymentKubeServer(t), newCountedKubeServer(t)
	path, before := resolverKubeconfig(t, "ambient", map[string]string{"selected": selected.server.URL, "ambient": ambient.server.URL})
	t.Setenv("KUBECONFIG", path)

	dir := t.TempDir()
	logPath := filepath.Join(dir, "kubectl.log")
	script := filepath.Join(dir, "kubectl")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho \"$*\" >> \""+logPath+"\"\n"), 0o755))
	oldKubectl := quickstartKubectlBinary
	quickstartKubectlBinary = script
	t.Cleanup(func() { quickstartKubectlBinary = oldKubectl })
	probes := func() string {
		raw, _ := os.ReadFile(logPath)
		return strings.TrimSpace(string(raw))
	}

	cmd := quickstartCmd
	flag := cmd.Flags().Lookup("kube-context")
	require.NotNil(t, flag)
	t.Cleanup(func() {
		_ = flag.Value.Set("")
		flag.Changed = false
	})
	cmd.SetContext(context.Background())

	require.NoError(t, cmd.Flags().Set("kube-context", "selected"))
	var err error
	captureStdout(t, func() { err = runQuickstart(cmd, nil) })
	require.NoError(t, err)
	require.Equal(t, "--context selected cluster-info", probes(), "the kubectl probe must name the selected context")
	require.NotZero(t, selected.requests.Load(), "the selected context was not read")
	require.Zero(t, ambient.requests.Load(), "the ambient context was read: %v", ambient.requestPaths())
	require.Contains(t, selected.requestPaths(), "/apis/apps/v1/namespaces/shop/deployments/api", "the explain did not read the Deployment from the selected context")

	for _, refused := range []string{"missing", ""} {
		if err := os.Remove(logPath); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		seen := selected.requests.Load()
		require.NoError(t, cmd.Flags().Set("kube-context", refused))
		captureStdout(t, func() { err = runQuickstart(cmd, nil) })
		require.Error(t, err, "--kube-context=%q must refuse", refused)
		require.Empty(t, probes(), "--kube-context=%q ran kubectl before refusing", refused)
		require.Equal(t, seen, selected.requests.Load(), "--kube-context=%q read a cluster before refusing", refused)
		require.Zero(t, ambient.requests.Load(), "--kube-context=%q fell back to the ambient context", refused)
	}

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after, "the kubeconfig was modified")
}

// newDeploymentKubeServer counts requests like newCountedKubeServer and holds
// one Deployment, shop/api; every other list is empty.
func newDeploymentKubeServer(t *testing.T) *countedKubeServer {
	t.Helper()
	const deployment = `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":"shop","uid":"u1"},"spec":{"replicas":1},"status":{"replicas":1,"readyReplicas":1,"availableReplicas":1}}`
	result := &countedKubeServer{}
	result.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result.requests.Add(1)
		result.mu.Lock()
		result.paths = append(result.paths, r.URL.Path)
		result.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/apps/v1/deployments":
			_, _ = fmt.Fprint(w, `{"apiVersion":"apps/v1","kind":"DeploymentList","metadata":{},"items":[`+deployment+`]}`)
		case "/apis/apps/v1/namespaces/shop/deployments/api":
			_, _ = fmt.Fprint(w, deployment)
		default:
			_, _ = fmt.Fprint(w, `{"apiVersion":"v1","kind":"List","metadata":{},"items":[]}`)
		}
	}))
	t.Cleanup(result.server.Close)
	return result
}
