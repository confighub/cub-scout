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
	"sync/atomic"
	"testing"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// argoApplicationServer serves one Application whose source is repoURL.
func argoApplicationServer(t *testing.T, repoURL string) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	reads := &atomic.Int64{}
	app := `{"apiVersion":"argoproj.io/v1alpha1","kind":"Application","metadata":{"name":"guestbook","namespace":"argocd","uid":"app-uid","resourceVersion":"1"},` +
		`"spec":{"project":"default","source":{"repoURL":"` + repoURL + `","path":"guestbook","targetRevision":"HEAD"},"destination":{"namespace":"guestbook"}},` +
		`"status":{"sync":{"status":"Synced","revision":"0d521c6e049889134f3122eb32d7ed342f43ca0d"},"health":{"status":"Healthy"}}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/applications/guestbook"):
			reads.Add(1)
			fmt.Fprint(w, app)
		case strings.HasSuffix(r.URL.Path, "/applications"):
			reads.Add(1)
			fmt.Fprint(w, `{"apiVersion":"argoproj.io/v1alpha1","kind":"ApplicationList","metadata":{},"items":[`+app+`]}`)
		default:
			fmt.Fprint(w, `{"apiVersion":"v1","kind":"List","metadata":{},"items":[]}`)
		}
	}))
	t.Cleanup(server.Close)
	return server, reads
}

// #821: with no argocd CLI, the default paths of explain, receipt verify and
// compare could not name an Argo source, while trace and every --kube-context
// path read it through the Kubernetes API. The same command gave a different
// answer depending on an optional binary.
func TestArgoSourceIsReadThroughTheAPIWithoutTheCLI(t *testing.T) {
	const repo = "https://github.com/argoproj/argocd-example-apps"
	server, reads := argoApplicationServer(t, repo)
	path, _ := resolverKubeconfig(t, "ambient", map[string]string{"ambient": server.URL})
	t.Setenv("KUBECONFIG", path)
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	live := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]interface{}{"name": "guestbook-ui", "namespace": "guestbook"},
	}}
	owner := agent.Ownership{Type: agent.OwnerArgo, Name: "guestbook", Namespace: "argocd"}

	t.Run("no argocd CLI: the Application is read through the Kubernetes API", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir()) // nothing on PATH
		require.False(t, agent.NewArgoTracer().Available(), "the test needs argocd to be absent")
		tracer := ambientArgoTracer(context.Background())
		require.True(t, tracer.Available(), "no usable Argo tracer without the CLI")

		anchor := receiptGitSourceAnchor(context.Background(), live, owner)
		require.NotNil(t, anchor, "no Git source for an Argo-delivered workload without the argocd CLI")
		require.Equal(t, repo, anchor.RepoURL)
		require.NotZero(t, reads.Load(), "the Application was not read from the cluster")
	})

	t.Run("argocd CLI present: it is still the tracer used", func(t *testing.T) {
		tools := t.TempDir()
		record := filepath.Join(tools, "calls.log")
		script := "#!/bin/sh\necho \"argocd $*\" >> \"" + record + "\"\n[ \"$1\" = version ] && exit 0\nexit 1\n"
		require.NoError(t, os.WriteFile(filepath.Join(tools, "argocd"), []byte(script), 0o755))
		t.Setenv("PATH", tools+string(os.PathListSeparator)+"/usr/bin:/bin")
		before := reads.Load()
		receiptGitSourceAnchor(context.Background(), live, owner)
		calls, err := os.ReadFile(record)
		require.NoError(t, err)
		require.Contains(t, string(calls), "argocd app get guestbook", "the CLI tracer was not used although the CLI is present:\n%s", calls)
		require.Equal(t, before, reads.Load(), "the API fallback ran although the CLI is present")
	})
}
