// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

func TestTraceDiffScopeDiscoveryHonorsCancellation(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	session, err := newTraceSession(&rest.Config{Host: server.URL}, "chosen")
	require.NoError(t, err)
	path := traceDiffManifest(t, "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: api}\nspec: {replicas: 1}\n")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := observeTraceDiffWithAPIVersion(ctx, session, "Deployment", "api", "team-a", "apps/v1", path)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("scope request never started")
	}
	cancel()
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("scope discovery ignored operation cancellation")
	}
}

func TestTraceDiffFailedReadDoesNotInventObservationTime(t *testing.T) {
	for _, status := range []string{traceDiffStatusMissing, traceDiffStatusInconclusive} {
		t.Run(status, func(t *testing.T) {
			result := &traceDiffObservation{Status: status, Read: &agent.BoundedReadEvidence{Available: false}}
			var output strings.Builder
			require.NoError(t, renderTraceDiffObservation(&output, result, "ascii"))
			require.NotContains(t, output.String(), "0001-01-01")
			require.NotContains(t, output.String(), "Live read: UID=")
			require.Contains(t, output.String(), "Live read unavailable")
		})
	}
}
