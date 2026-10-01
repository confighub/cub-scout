// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

func TestArtifactReaderSkipsUnboundArgoOCISourceAndKeepsFluxArtifact(t *testing.T) {
	var mu sync.Mutex
	counts := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		counts[r.URL.Path]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/apis/source.toolkit.fluxcd.io/v1/namespaces/sources/gitrepositories/platform" {
			t.Errorf("unexpected Kubernetes request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"apiVersion":"source.toolkit.fluxcd.io/v1","kind":"GitRepository","metadata":{"name":"platform","namespace":"sources"},"status":{"artifact":{"url":"https://artifact.invalid/platform","revision":"main@sha1:abc","digest":"sha256:abc"}}}`))
	}))
	defer server.Close()
	session, err := newTraceSession(&rest.Config{Host: server.URL}, "selected")
	require.NoError(t, err)
	known := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	result := &agent.TraceResult{Chain: []agent.ChainLink{
		{Kind: "GitRepository", Name: "platform", Namespace: "sources"},
		{Kind: "OCIRepository", Name: "team/repo", URL: "oci://registry.example/team/repo", Revision: "sha256:def", LastTransitionTime: &known},
	}}
	artifacts, errs := collectTraceArtifactsWithTraceSessionAndErrors(context.Background(), session, result)
	require.Len(t, errs, 1)
	require.EqualError(t, errs[0], "artifact metadata unavailable for OCIRepository//team/repo: exact Kubernetes source name is missing or invalid")
	require.Equal(t, "https://artifact.invalid/platform", artifacts[traceArtifactKey("GitRepository", "sources", "platform")].URL)
	require.Equal(t, "main@sha1:abc", artifacts[traceArtifactKey("GitRepository", "sources", "platform")].Revision)
	require.Equal(t, "oci://registry.example/team/repo", result.Chain[1].URL)
	require.Equal(t, "sha256:def", result.Chain[1].Revision)
	require.Equal(t, &known, result.Chain[1].LastTransitionTime)
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 1, counts["/apis/source.toolkit.fluxcd.io/v1/namespaces/sources/gitrepositories/platform"])
	require.Len(t, counts, 1, "Argo OCI display name must not be treated as a Flux object identity")
}

func TestArtifactReaderUnsupportedSourceKindsDoNotWarnWithoutSession(t *testing.T) {
	result := &agent.TraceResult{Chain: []agent.ChainLink{
		{Kind: "Source", Name: "space/target"},
		{Kind: "Job", Name: "job", Namespace: "ns"},
	}}
	artifacts, errs := collectTraceArtifactsWithTraceSessionAndErrors(context.Background(), nil, result)
	require.Empty(t, artifacts)
	require.Empty(t, errs)
}
