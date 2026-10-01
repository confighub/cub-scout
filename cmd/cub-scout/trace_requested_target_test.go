// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

func TestObserveArgoWorkloadPreservesRequestedTargetAcrossRenderings(t *testing.T) {
	app := map[string]interface{}{
		"apiVersion": "argoproj.io/v1alpha1", "kind": "Application",
		"metadata": map[string]interface{}{"name": "delivery-app", "namespace": "gitops"},
		"spec":     map[string]interface{}{"source": map[string]interface{}{"repoURL": "https://example.invalid/app.git", "targetRevision": "main"}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/apps/v1/namespaces/workloads/deployments/api":
			writeTraceFixtureJSON(w, map[string]interface{}{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]interface{}{"name": "api", "namespace": "workloads", "labels": map[string]interface{}{"argocd.argoproj.io/instance": "delivery-app"}}})
		case "/apis/argoproj.io/v1alpha1/applications":
			writeTraceFixtureJSON(w, map[string]interface{}{"apiVersion": "argoproj.io/v1alpha1", "kind": "ApplicationList", "items": []interface{}{app}})
		case "/apis/argoproj.io/v1alpha1/namespaces/gitops/applications/delivery-app":
			writeTraceFixtureJSON(w, app)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	session, err := newTraceSession(&rest.Config{Host: server.URL}, "selected")
	require.NoError(t, err)
	observed, err := observeTrace(context.Background(), session, "Deployment", "api", "workloads", traceObservationOptions{})
	require.NoError(t, err)
	target := agent.ResourceRef{Kind: "Deployment", Name: "api", Namespace: "workloads"}
	require.Equal(t, target, observed.Result.Object, "controller lineage must not replace the requested workload")
	require.True(t, len(observed.Result.Chain) >= 2)
	require.Equal(t, "Application", observed.Result.Chain[1].Kind)
	require.Equal(t, "gitops", observed.Result.Chain[1].Namespace)
	invocation, err := NewInvocationContext("human", TransportTUI)
	require.NoError(t, err)
	var human bytes.Buffer
	require.NoError(t, renderTraceHuman(&human, observed.Result, observed.Artifacts, invocation, traceHumanOptions{}))
	require.Contains(t, traceRenderANSI.ReplaceAllString(human.String(), ""), "Deployment/api in workloads")
	var markdownErr error
	markdown := captureStdout(t, func() { markdownErr = outputTraceMarkdown(observed.Result, observed.Artifacts, invocation) })
	require.NoError(t, markdownErr)
	require.Contains(t, markdown, "Deployment/api in workloads")
	require.Contains(t, markdown, "delivery-app")
	jsonResult := convertTraceToV014(observed.Result, target.Kind, target.Name, target.Namespace, observed.Artifacts)
	require.Equal(t, target.Name, jsonResult.Target.Name)
	require.Equal(t, target.Namespace, jsonResult.Target.Namespace)
	require.False(t, strings.Contains(human.String(), "Trace: Application/delivery-app"))
}
