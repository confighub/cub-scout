// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT
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

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func preserveTreeContextFlags(t *testing.T) {
	t.Helper()
	format, ns, all, js := treeFormat, treeNamespace, treeAll, treeJSON
	t.Cleanup(func() { treeFormat, treeNamespace, treeAll, treeJSON = format, ns, all, js })
	treeFormat, treeNamespace, treeAll, treeJSON = "json", "team-a", true, false
	t.Setenv("CUB_SCOUT_TEST_TREE_JSON", "")
}

func TestTreeExplicitContextSelectedAndPinned(t *testing.T) {
	for _, view := range []string{"runtime", "ownership", "git", "suggest", "patterns", "workloads"} {
		t.Run(view, func(t *testing.T) {
			preserveTreeContextFlags(t)
			ambient := newCountedKubeServer(t)
			requests := []string{}
			selected := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "/deployments") {
					fmt.Fprint(w, `{"apiVersion":"apps/v1","kind":"DeploymentList","items":[{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"selected-workload","namespace":"team-a","uid":"selected-uid"},"spec":{"replicas":1},"status":{"readyReplicas":1}}]}`)
				} else {
					fmt.Fprint(w, `{"apiVersion":"v1","kind":"List","items":[]}`)
				}
			}))
			defer selected.Close()
			path, before := resolverKubeconfig(t, "ambient", map[string]string{"selected": selected.URL, "ambient": ambient.server.URL})
			t.Setenv("KUBECONFIG", path)
			var err error
			out := captureStdout(t, func() { err = runTree(exportContextCommand(t, "tree", "selected"), []string{view}) })
			require.NoError(t, err)
			require.NotEmpty(t, requests)
			require.Zero(t, ambient.requests.Load())
			if view == "ownership" {
				require.Contains(t, out, `"cluster": "selected"`)
			}
			if view == "runtime" || view == "ownership" || view == "workloads" || view == "suggest" {
				require.Contains(t, requests, "/apis/apps/v1/namespaces/team-a/deployments")
			}
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestTreeExplicitInvalidOrCancelledRefusesAllViews(t *testing.T) {
	for _, view := range []string{"runtime", "ownership", "composition", "workloads", "git", "patterns", "suggest", "config"} {
		for _, selection := range []string{"", "missing"} {
			t.Run(view+"/"+selection, func(t *testing.T) {
				preserveTreeContextFlags(t)
				ambient := newCountedKubeServer(t)
				path, before := resolverKubeconfig(t, "ambient", map[string]string{"ambient": ambient.server.URL})
				t.Setenv("KUBECONFIG", path)
				var err error
				out := captureStdout(t, func() { err = runTree(exportContextCommand(t, "tree", selection), []string{view}) })
				require.Error(t, err)
				require.Empty(t, out)
				require.Zero(t, ambient.requests.Load())
				after, e := os.ReadFile(path)
				require.NoError(t, e)
				require.Equal(t, before, after)
				cmd := exportContextCommand(t, "tree", "ambient")
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				cmd.SetContext(ctx)
				require.ErrorIs(t, runTree(cmd, []string{view}), context.Canceled)
				require.Zero(t, ambient.requests.Load())
			})
		}
	}
}

func TestTreeCompositionChildUsesPrivateCaptureAndCleans(t *testing.T) {
	preserveTreeContextFlags(t)
	ambient := newCountedKubeServer(t)
	selected := newCountedKubeServer(t)
	path, before := resolverKubeconfig(t, "ambient", map[string]string{"ambient": ambient.server.URL, "selected": selected.server.URL})
	t.Setenv("KUBECONFIG", path)
	dir := t.TempDir()
	record := filepath.Join(dir, "child-path")
	// Fixture child records only its private path; its config is never emitted.
	script := `#!/bin/sh
[ "$1" = --kubeconfig ] || exit 10
[ "$3" = --context ] || exit 11
[ "$4" = cub-scout-trace ] || exit 12
[ -f "$2" ] || exit 13
grep -F "server: $TREE_EXPECTED_SERVER" "$2" >/dev/null || exit 14
printf '%s' "$2" > "$TREE_CHILD_RECORD"
case "$5" in
api-resources) printf 'deployments.apps\n';;
get) printf '{"apiVersion":"apps/v1","kind":"DeploymentList","items":[]}';;
esac
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kubectl"), []byte(script), 0700))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TREE_CHILD_RECORD", record)
	t.Setenv("TREE_EXPECTED_SERVER", selected.server.URL)
	var err error
	captureStdout(t, func() { err = runTree(exportContextCommand(t, "tree", "selected"), []string{"composition"}) })
	require.NoError(t, err)
	require.Positive(t, selected.requests.Load())
	require.Zero(t, ambient.requests.Load())
	child, e := os.ReadFile(record)
	require.NoError(t, e)
	require.NotEqual(t, path, string(child))
	_, e = os.Stat(string(child))
	require.True(t, os.IsNotExist(e), "private config must be removed")
	after, e := os.ReadFile(path)
	require.NoError(t, e)
	require.Equal(t, before, after)
}

func TestTreeCaptureSurvivesAmbientConfigChange(t *testing.T) {
	preserveTreeContextFlags(t)
	ambient := newCountedKubeServer(t)
	var path string
	var changed bool
	var changeErr error
	selected := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !changed {
			changed = true
			// This is the test-owned file, changed after the first selected request.
			changeErr = os.WriteFile(path, []byte("apiVersion: v1\nkind: Config\ncurrent-context: missing\n"), 0600)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"apiVersion":"v1","kind":"List","items":[]}`)
	}))
	defer selected.Close()
	path, _ = resolverKubeconfig(t, "ambient", map[string]string{"selected": selected.URL, "ambient": ambient.server.URL})
	t.Setenv("KUBECONFIG", path)
	var err error
	out := captureStdout(t, func() { err = runTree(exportContextCommand(t, "tree", "selected"), []string{"ownership"}) })
	require.NoError(t, err)
	require.NoError(t, changeErr)
	require.True(t, changed)
	require.Contains(t, out, `"cluster": "selected"`)
	require.Zero(t, ambient.requests.Load())
}

// `tree workloads --kube-context` pinned the context while the commands it
// aliases, `map workloads` and `map patterns`, had no such flag and always
// read the ambient context.
func TestMapWorkloadsAndPatternsAcceptExplicitContext(t *testing.T) {
	for name, tc := range map[string]struct {
		command *cobra.Command
		run     func(*cobra.Command, []string) error
	}{
		"workloads": {mapWorkloadsCmd, runMapWorkloads},
		"patterns":  {mapPatternsCmd, runMapPatterns},
	} {
		t.Run(name, func(t *testing.T) {
			require.NotNil(t, tc.command.Flags().Lookup("kube-context"), "map %s has no --kube-context flag", name)
			ambient := newCountedKubeServer(t)
			requests := 0
			selected := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"apiVersion":"v1","kind":"List","items":[]}`)
			}))
			defer selected.Close()
			path, before := resolverKubeconfig(t, "ambient", map[string]string{"selected": selected.URL, "ambient": ambient.server.URL})
			t.Setenv("KUBECONFIG", path)
			var err error
			captureStdout(t, func() { err = tc.run(exportContextCommand(t, name, "selected"), nil) })
			require.NoError(t, err)
			require.NotZero(t, requests)
			require.Zero(t, ambient.requests.Load())
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, after)

			err = tc.run(exportContextCommand(t, name, "missing"), nil)
			require.Error(t, err, "a missing explicit context must refuse, not fall back")
			require.Zero(t, ambient.requests.Load())
		})
	}
}
