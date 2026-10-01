// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTraceDiffDeniedOwnershipIsUnknownNotUnmanaged(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"Forbidden","message":"fixture ownership read denied","code":403}`))
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "config")
	writeTraceKubeconfig(t, path, "alpha-context", server.URL, server.URL)
	t.Setenv("KUBECONFIG", path)
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")
	t.Setenv("PATH", t.TempDir())
	var runErr error
	output := captureStdout(t, func() {
		runErr = runTraceDiff(context.Background(), "Deployment", "api", "team-a")
	})
	assert.ErrorContains(t, runErr, "ownership")
	assert.NotContains(t, output, "not managed by GitOps")
	assert.NotContains(t, output, "Consider importing")
	require.EqualValues(t, 1, requests.Load())
}
