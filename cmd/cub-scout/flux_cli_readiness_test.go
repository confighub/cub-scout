// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/stretchr/testify/require"
)

// #826: with the flux CLI installed, trace drew a Kustomization whose build
// was failing with a green tick, because `flux trace` prints the same status
// line for a failing object as for a healthy one. Both places a CLI Flux
// tracer is built, with and without an explicit context, must take readiness
// from the cluster. The fixtures were recorded from a real failing cluster.
func TestFluxCLIPathsReportAFailingKustomizationAsNotReady(t *testing.T) {
	recorded, err := filepath.Abs(filepath.Join("..", "..", "pkg", "agent", "testdata", "flux-readiness-826"))
	require.NoError(t, err)
	serve := func(file string) []byte {
		data, readErr := os.ReadFile(filepath.Join(recorded, file))
		require.NoError(t, readErr)
		return data
	}
	objects := map[string][]byte{
		"/kustomizations/podinfo":  serve("kustomization-failing.json"),
		"/gitrepositories/podinfo": serve("gitrepository-ready.json"),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		for suffix, body := range objects {
			if strings.HasSuffix(r.URL.Path, suffix) {
				_, _ = w.Write(body)
				return
			}
		}
		_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"List","metadata":{},"items":[]}`))
	}))
	t.Cleanup(server.Close)
	path, _ := resolverKubeconfig(t, "ambient", map[string]string{"ambient": server.URL, "selected": server.URL})
	t.Setenv("KUBECONFIG", path)
	t.Setenv("KUBERNETES_SERVICE_HOST", "")

	// A flux stand-in that prints what real flux printed for this state.
	tools := t.TempDir()
	script := "#!/bin/sh\n[ \"$1\" = version ] && exit 0\ncat \"" + filepath.Join(recorded, "flux-trace-deployment-failing-kustomization.txt") + "\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(tools, "flux"), []byte(script), 0o755))
	t.Setenv("PATH", tools+string(os.PathListSeparator)+"/usr/bin:/bin")

	check := func(t *testing.T, tracer agent.Tracer) {
		t.Helper()
		result, err := tracer.Trace(context.Background(), "Deployment", "podinfo", "team-a")
		require.NoError(t, err)
		require.False(t, result.FullyManaged, "a chain with a failing Kustomization is reported fully managed")
		for _, link := range result.Chain {
			if link.Kind == "Kustomization" {
				require.False(t, link.Ready, "the failing Kustomization is reported ready")
				require.Contains(t, link.StatusReason, "kustomization path not found")
				return
			}
		}
		t.Fatalf("no Kustomization link in %+v", result.Chain)
	}

	t.Run("no explicit context", func(t *testing.T) {
		check(t, ambientFluxTracer(context.Background()))
	})
	t.Run("explicit context", func(t *testing.T) {
		session, err := newTraceSessionForSelection(clusterContextSelection{name: "selected", explicit: true})
		require.NoError(t, err)
		tracer, cleanup, err := capturedTraceFluxFactory(session)
		require.NoError(t, err)
		defer func() { require.NoError(t, cleanup()) }()
		check(t, tracer)
	})
}
