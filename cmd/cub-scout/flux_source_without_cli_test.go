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
	"sync"
	"testing"
	"time"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// fluxChainServer serves Deployment team-a/api, applied by Kustomization
// flux-system/shop-apps from a GitRepository at repoURL, and records the paths
// it was asked for.
type fluxChainServer struct {
	server *httptest.Server
	mu     sync.Mutex
	paths  []string
}

func newFluxChainServer(t *testing.T, repoURL string) *fluxChainServer {
	t.Helper()
	f := &fluxChainServer{}
	ready := `"status":{"conditions":[{"type":"Ready","status":"True","reason":"Succeeded","message":"ok"}]`
	objects := map[string]string{
		"/apis/apps/v1/namespaces/team-a/deployments/api":                                      `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":"team-a","uid":"d1","resourceVersion":"1","labels":{"kustomize.toolkit.fluxcd.io/name":"shop-apps","kustomize.toolkit.fluxcd.io/namespace":"flux-system"}},"spec":{"replicas":1}}`,
		"/apis/kustomize.toolkit.fluxcd.io/v1/namespaces/flux-system/kustomizations/shop-apps": `{"apiVersion":"kustomize.toolkit.fluxcd.io/v1","kind":"Kustomization","metadata":{"name":"shop-apps","namespace":"flux-system"},"spec":{"path":"./apps","sourceRef":{"kind":"GitRepository","name":"shop"}},` + ready + `,"lastAppliedRevision":"main@sha1:abc123"}}`,
		"/apis/source.toolkit.fluxcd.io/v1/namespaces/flux-system/gitrepositories/shop":        `{"apiVersion":"source.toolkit.fluxcd.io/v1","kind":"GitRepository","metadata":{"name":"shop","namespace":"flux-system"},"spec":{"url":"` + repoURL + `"},` + ready + `,"artifact":{"revision":"main@sha1:abc123"}}}`,
	}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.paths = append(f.paths, r.URL.Path)
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if body, ok := objects[r.URL.Path]; ok {
			fmt.Fprint(w, body)
			return
		}
		fmt.Fprint(w, `{"apiVersion":"v1","kind":"List","metadata":{},"items":[]}`)
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fluxChainServer) asked(suffix string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, path := range f.paths {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}

// #824: without the flux CLI no surface could name the source of a
// Flux-delivered workload. The Kubernetes API holds the same chain flux trace
// reads, so the CLI must not be required to read it.
func TestFluxSourceIsReadThroughTheAPIWithoutTheCLI(t *testing.T) {
	const repo = "https://example.invalid/shop.git"
	live := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]interface{}{"name": "api", "namespace": "team-a", "labels": map[string]interface{}{
			"kustomize.toolkit.fluxcd.io/name": "shop-apps", "kustomize.toolkit.fluxcd.io/namespace": "flux-system"}},
	}}
	owner := agent.DetectOwnership(live)
	require.Equal(t, agent.OwnerFlux, owner.Type)
	t.Setenv("KUBERNETES_SERVICE_HOST", "")

	t.Run("no flux CLI, no selection: the ambient cluster's API is read", func(t *testing.T) {
		cluster := newFluxChainServer(t, repo)
		path, _ := resolverKubeconfig(t, "ambient", map[string]string{"ambient": cluster.server.URL})
		t.Setenv("KUBECONFIG", path)
		t.Setenv("PATH", t.TempDir())
		require.False(t, agent.NewFluxTracer().Available(), "the test needs flux to be absent")

		anchor := receiptGitSourceAnchor(context.Background(), live, owner)
		require.NotNil(t, anchor, "no Git source for a Flux-delivered workload without the flux CLI")
		require.Equal(t, repo, anchor.RepoURL)
		require.Equal(t, "main@sha1:abc123", anchor.Revision)
	})

	t.Run("no flux CLI, explicit selection: only the selected cluster's API is read", func(t *testing.T) {
		selected, ambient := newFluxChainServer(t, repo), newFluxChainServer(t, "https://example.invalid/ambient.git")
		path, _ := resolverKubeconfig(t, "ambient", map[string]string{"selected": selected.server.URL, "ambient": ambient.server.URL})
		t.Setenv("KUBECONFIG", path)
		t.Setenv("PATH", t.TempDir())
		started := time.Now()
		bound, err := boundCommandContext(exportContextCommand(t, "receipt", "selected"))
		require.NoError(t, err)

		anchor := receiptGitSourceAnchor(bound, live, owner)
		require.NotNil(t, anchor)
		require.Equal(t, repo, anchor.RepoURL, "the source came from the ambient cluster")
		require.Empty(t, ambient.paths, "the ambient cluster was read")
		// No child kubeconfig is needed for an API read, so none is left behind.
		leftovers, _ := filepath.Glob(filepath.Join(os.TempDir(), "cub-scout-trace-*"))
		for _, leftover := range leftovers {
			info, statErr := os.Stat(leftover)
			require.True(t, statErr != nil || info.ModTime().Before(started), "a trace credential directory was left behind: %s", leftover)
		}
	})

	t.Run("flux CLI present: it is still the tracer used", func(t *testing.T) {
		cluster := newFluxChainServer(t, repo)
		path, _ := resolverKubeconfig(t, "ambient", map[string]string{"ambient": cluster.server.URL})
		t.Setenv("KUBECONFIG", path)
		record := fakeTracerCLIs(t)
		receiptGitSourceAnchor(context.Background(), live, owner)
		calls := recordedCalls(t, record)
		require.NotEmpty(t, calls, "the flux CLI was not used although it is present")
		require.True(t, strings.HasPrefix(calls[0], "flux trace deployment api"), calls[0])
		require.False(t, cluster.asked("/kustomizations/shop-apps"), "the API fallback ran although the CLI is present")
	})
}
