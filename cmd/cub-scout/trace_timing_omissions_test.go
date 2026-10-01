// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

func TestCapturedTraceTimingOmissionsReachJSONWarnings(t *testing.T) {
	var mu sync.Mutex
	counts := map[string]int{}
	readyPath := "/apis/source.toolkit.fluxcd.io/v1/namespaces/ns/gitrepositories/ready"
	deniedPath := "/apis/source.toolkit.fluxcd.io/v1beta2/namespaces/ns/buckets/denied"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		counts[r.URL.Path]++
		mu.Unlock()
		if r.URL.Path == deniedPath {
			writeTraceStatus(w, 403, "private-response-marker")
			return
		}
		if r.URL.Path == readyPath {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"apiVersion":"source.toolkit.fluxcd.io/v1","kind":"GitRepository","metadata":{"name":"ready","namespace":"ns"},"status":{"artifact":{"lastUpdateTime":"2025-01-02T03:04:05Z"}}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	session, err := newTraceSession(&rest.Config{Host: server.URL}, "captured")
	require.NoError(t, err)
	result := &agent.TraceResult{Object: agent.ResourceRef{Kind: "Kustomization", Name: "root", Namespace: "ns"}, Chain: []agent.ChainLink{{Kind: "GitRepository", Name: "ready", Namespace: "ns"}, {Kind: "Bucket", Name: "denied", Namespace: "ns"}, {Kind: "Job", Name: "unsupported", Namespace: "ns"}}}
	enrichTraceWithTimingSession(context.Background(), session, result)
	require.NotNil(t, result.Chain[0].LastTransitionTime)
	require.Nil(t, result.Chain[1].LastTransitionTime)
	require.Nil(t, result.Chain[2].LastTransitionTime)
	require.Contains(t, result.Error, "timing unavailable for Bucket/ns/denied: forbidden")
	require.NotContains(t, result.Error, "private-response-marker")
	output := convertTraceToV014(result, "Kustomization", "root", "ns", nil)
	require.Len(t, output.Warnings, 1)
	require.Contains(t, output.Warnings[0], "Bucket/ns/denied")
	require.NotContains(t, output.Warnings[0], "private-response-marker")
	require.Equal(t, 1, counts[readyPath])
	require.Equal(t, 1, counts[deniedPath])
	require.Zero(t, counts["/apis/batch/v1/namespaces/ns/jobs/unsupported"])
}
