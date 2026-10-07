// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// #812: compare <resource> and compare object-set read the ambient context.
func TestCompareReadsOnlyTheSelectedContext(t *testing.T) {
	binary := sharedTestBinary(t)
	desired := filepath.Join(t.TempDir(), "desired.yaml")
	require.NoError(t, os.WriteFile(desired, []byte("apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: api\n  namespace: team-a\nspec:\n  replicas: 1\n"), 0o600))
	for name, args := range map[string][]string{
		"resource":   {"compare", "deployment/api", "--namespace", "team-a", "--format", "json"},
		"object set": {"compare", "object-set", "--dry-from", desired, "--scope", "namespace/team-a", "--format", "json"},
	} {
		t.Run(name, func(t *testing.T) { assertProcessReadsOnlySelectedContext(t, binary, args) })
	}
}

// The Git/namespace comparison and --apply read through kubectl and other
// ambient paths. They must refuse the selection, not bind part of it.
func TestCompareWithoutAResourceRefusesAContextSelection(t *testing.T) {
	binary := sharedTestBinary(t)
	selected, ambient := newCountedKubeServer(t), newCountedKubeServer(t)
	path, _ := resolverKubeconfig(t, "ambient", map[string]string{"selected": selected.server.URL, "ambient": ambient.server.URL})
	for _, args := range [][]string{
		{"compare", "--namespace", "team-a", "--kube-context", "selected"},
		{"compare", "--namespace", "team-a", "--suggest", "--apply", "--dry-run", "--kube-context", "selected"},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Env = []string{"KUBECONFIG=" + path, "HOME=" + t.TempDir(), "PATH=/usr/bin:/bin", "CUB_SCOUT_OFFLINE=true"}
		out, err := cmd.CombinedOutput()
		cancel()
		require.Error(t, err, "%v must refuse:\n%s", args, out)
		require.Contains(t, string(out), "--kube-context applies to compare <resource>")
		require.Zero(t, selected.requests.Load()+ambient.requests.Load(), "%v read a cluster before refusing", args)
	}
}

// compare <resource> takes its Git source from a tracer. With a selection the
// tracer must be pointed at the selected cluster, never the ambient one.
func TestCompareResourceTracerIsBoundToTheSelectedContext(t *testing.T) {
	binary := sharedTestBinary(t)
	ambient := newCountedKubeServer(t)
	selected := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/namespaces/team-a/deployments/api") {
			fmt.Fprint(w, `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":"team-a","uid":"u1","labels":{"kustomize.toolkit.fluxcd.io/name":"shop-apps","kustomize.toolkit.fluxcd.io/namespace":"flux-system"}},"spec":{"replicas":1}}`)
			return
		}
		fmt.Fprint(w, `{"apiVersion":"v1","kind":"List","metadata":{},"items":[]}`)
	}))
	t.Cleanup(selected.Close)
	path, _ := resolverKubeconfig(t, "ambient", map[string]string{"selected": selected.URL, "ambient": ambient.server.URL})

	tools := t.TempDir()
	record := filepath.Join(tools, "calls.log")
	for _, tool := range []string{"flux", "argocd", "kubectl"} {
		script := "#!/bin/sh\nserver=$(grep -Eo 'https?://[^\" ,]+' \"$KUBECONFIG\" 2>/dev/null | head -1)\n" +
			"echo \"" + tool + " $* SERVER=$server\" >> \"" + record + "\"\n[ \"$1\" = version ] && exit 0\nexit 1\n"
		require.NoError(t, os.WriteFile(filepath.Join(tools, tool), []byte(script), 0o755))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "compare", "deployment/api", "--namespace", "team-a", "--format", "json", "--kube-context", "selected")
	cmd.Env = []string{"KUBECONFIG=" + path, "HOME=" + t.TempDir(), "PATH=" + tools + ":/usr/bin:/bin", "CUB_SCOUT_OFFLINE=true"}
	out, _ := cmd.CombinedOutput()

	data, err := os.ReadFile(record)
	require.NoError(t, err, "no tracer ran for a Flux-labelled Deployment:\n%s", out)
	traced := 0
	for _, call := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.Contains(call, " version") {
			continue
		}
		traced++
		require.True(t, strings.HasPrefix(call, "flux trace "), "only a flux trace may run: %s", call)
		require.Contains(t, call, "--kubeconfig ", "flux ran unbound: %s", call)
		require.NotContains(t, call, path, "flux was given the shared kubeconfig: %s", call)
		require.Contains(t, call, "SERVER="+selected.URL, "flux was not pointed at the selected cluster: %s", call)
	}
	require.NotZero(t, traced, "the Flux tracer did not run:\n%s", out)
	require.Zero(t, ambient.requests.Load(), "the ambient cluster was read: %v", ambient.paths)
}
