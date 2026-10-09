// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

var (
	sharedTestBinaryOnce sync.Once
	sharedTestBinaryPath string
	sharedTestBinaryErr  error
)

// sharedTestBinary builds cub-scout once per test run. Commands that end in
// os.Exit can only be exercised as a process; building per test is what makes
// this package slow (#800), so tests that need a binary should share this one.
func sharedTestBinary(t *testing.T) string {
	t.Helper()
	sharedTestBinaryOnce.Do(func() {
		dir, err := os.MkdirTemp("", "cub-scout-test-binary-")
		if err != nil {
			sharedTestBinaryErr = err
			return
		}
		sharedTestBinaryPath = filepath.Join(dir, "cub-scout")
		out, err := exec.Command("go", "build", "-o", sharedTestBinaryPath, ".").CombinedOutput()
		if err != nil {
			sharedTestBinaryErr = &exec.ExitError{Stderr: out}
		}
	})
	require.NoError(t, sharedTestBinaryErr, "build cub-scout for process tests")
	return sharedTestBinaryPath
}

// #809: debug, drift, graph explain, patterns and context-pack read the
// ambient context and had no way to select another. Run as real processes,
// because several of them finish through os.Exit.
func TestReadCommandsReadOnlyTheSelectedContext(t *testing.T) {
	binary := sharedTestBinary(t)
	desired := filepath.Join(t.TempDir(), "desired.yaml")
	require.NoError(t, os.WriteFile(desired, []byte("apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: api\n  namespace: team-a\nspec:\n  replicas: 1\n"), 0o600))
	for name, args := range map[string][]string{
		"debug":            {"debug", "deployment/api", "--namespace", "team-a", "--non-interactive", "--format", "json"},
		"drift":            {"drift", "--file", desired},
		"compare drift":    {"compare", "drift", "--file", desired},
		"graph explain":    {"graph", "explain", "Deployment/api", "--namespace", "team-a"},
		"patterns detect":  {"patterns", "detect"},
		"patterns explain": {"patterns", "explain", "delivery.bridge.confighub_oci"},
		"context-pack":     {"context-pack"},
		"gitops settings":  {"gitops", "settings", "--format", "json"},
	} {
		t.Run(name, func(t *testing.T) { assertProcessReadsOnlySelectedContext(t, binary, args) })
	}
}

// assertProcessReadsOnlySelectedContext runs one command as a process against a
// selected and an ambient fake API server whose kubeconfig current-context is
// the ambient one.
func assertProcessReadsOnlySelectedContext(t *testing.T, binary string, args []string) {
	t.Helper()
	selected, ambient := newCountedKubeServer(t), newCountedKubeServer(t)
	path, before := resolverKubeconfig(t, "ambient", map[string]string{"selected": selected.server.URL, "ambient": ambient.server.URL})
	run := func(selection string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, append(append([]string{}, args...), "--kube-context", selection)...)
		// A private HOME and no cub on PATH: standalone, and nothing of the
		// developer's own configuration can stand in for the kubeconfig.
		cmd.Env = []string{"KUBECONFIG=" + path, "HOME=" + t.TempDir(), "PATH=/usr/bin:/bin", "CUB_SCOUT_OFFLINE=true"}
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	// The fake API serves empty lists, so a command may legitimately exit
	// non-zero (target not found). What matters is which server it asked.
	out, _ := run("selected")
	require.NotZero(t, selected.requests.Load(), "the selected context was not read:\n%s", out)
	require.Zero(t, ambient.requests.Load(), "the ambient context was read: %v\n%s", ambient.paths, out)

	for _, refused := range []string{"missing", ""} {
		seen := selected.requests.Load()
		out, err := run(refused)
		require.Error(t, err, "--kube-context=%q must refuse:\n%s", refused, out)
		require.Equal(t, seen, selected.requests.Load(), "--kube-context=%q read a cluster before refusing", refused)
		require.Zero(t, ambient.requests.Load(), "--kube-context=%q fell back to the ambient context", refused)
	}

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after, "the kubeconfig was modified")
}

// The cluster label printed beside the evidence must name the context the
// reads went to, not the ambient current-context.
func TestBoundCommandsLabelTheSelectedContext(t *testing.T) {
	selected, ambient := newCountedKubeServer(t), newCountedKubeServer(t)
	path, _ := resolverKubeconfig(t, "ambient", map[string]string{"selected": selected.server.URL, "ambient": ambient.server.URL})
	t.Setenv("KUBECONFIG", path)
	t.Setenv("CUB_SCOUT_TEST_CLUSTER", "")
	t.Setenv("CUB_SCOUT_TEST_GRAPH_JSON", "")
	t.Setenv("CUB_SCOUT_TEST_TIME", "")

	bound, err := boundCommandContext(exportContextCommand(t, "patterns", "selected"))
	require.NoError(t, err)
	require.Equal(t, "selected", boundContextLabel(bound))
	g, err := buildPatternsGraph(bound, "", false)
	require.NoError(t, err)
	require.Equal(t, "selected", g.Cluster)
	require.Equal(t, extractClusterFromHost(selected.server.URL), getCurrentClusterName(bound))
	require.NotZero(t, selected.requests.Load())
	require.Zero(t, ambient.requests.Load())

	unbound, err := boundCommandContext(&cobra.Command{Use: "patterns"})
	require.NoError(t, err)
	require.Equal(t, "ambient", boundContextLabel(unbound), "without the flag the label is the ambient context, as before")
}
