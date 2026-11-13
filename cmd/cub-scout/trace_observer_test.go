// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func TestObserveTraceDirectApplicationKeepsCapturedScopeAndPartialEvidence(t *testing.T) {
	alpha := newTraceSessionFixture(t, "alpha")
	beta := newTraceSessionFixture(t, "beta")
	path := filepath.Join(t.TempDir(), "config")
	writeTraceKubeconfig(t, path, "alpha-context", alpha.server.URL, beta.server.URL)
	t.Setenv("KUBECONFIG", path)
	config, label, err := resolveClusterConfig("alpha-context", true, &clientcmd.ClientConfigLoadingRules{ExplicitPath: path}, nil)
	require.NoError(t, err)
	session, err := newTraceSession(config, label)
	require.NoError(t, err)
	config.Host = beta.server.URL
	writeTraceKubeconfig(t, path, "beta-context", alpha.server.URL, beta.server.URL)
	observation, err := observeTrace(context.Background(), session, "Application", "api", "delivery", traceObservationOptions{
		DirectApplication: true,
		Flux: func(*traceSession) (agent.Tracer, func() error, error) {
			t.Fatal("direct Application observation must not probe Flux")
			return nil, nil, nil
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, observation.Result.Chain)
	require.Equal(t, "api", observation.Result.Object.Name)
	require.Equal(t, "delivery", observation.Result.Object.Namespace)
	require.Equal(t, agent.OwnerArgo, observation.Result.DetectedOwner)
	// The fixture does not expose Events in delivery. Preserve the Application
	// chain while explaining the missing read instead of reporting full coverage.
	require.Contains(t, observation.Result.Error, "Events unavailable")
	require.Greater(t, alpha.count(http.MethodGet, "/apis/argoproj.io/v1alpha1/namespaces/delivery/applications/api"), 0)
	beta.mu.Lock()
	defer beta.mu.Unlock()
	require.Empty(t, beta.requests, "no requests of any kind may reach ambient beta")
}

func TestObserveTraceFluxSetupFailureCleansUpAndPreservesError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":"team-a","labels":{"kustomize.toolkit.fluxcd.io/name":"app","kustomize.toolkit.fluxcd.io/namespace":"flux-system"}}}`))
	}))
	defer server.Close()
	session, err := newTraceSession(&rest.Config{Host: server.URL}, "selected")
	require.NoError(t, err)
	cleaned := false
	observation, err := observeTrace(context.Background(), session, "Deployment", "api", "team-a", traceObservationOptions{
		Flux: func(got *traceSession) (agent.Tracer, func() error, error) {
			require.Same(t, session, got)
			return nil, func() error { cleaned = true; return nil }, fmt.Errorf("capture failed")
		},
	})
	require.ErrorContains(t, err, "capture failed")
	require.Nil(t, observation)
	require.True(t, cleaned)
}

func TestObserveTraceMissingSessionCannotUseAmbient(t *testing.T) {
	ambient := newTraceSessionFixture(t, "ambient")
	path := filepath.Join(t.TempDir(), "config")
	writeTraceKubeconfig(t, path, "alpha-context", ambient.server.URL, ambient.server.URL)
	t.Setenv("KUBECONFIG", path)
	observation, err := observeTrace(context.Background(), nil, "Application", "api", "delivery", traceObservationOptions{DirectApplication: true})
	require.ErrorContains(t, err, "session is unavailable")
	require.Nil(t, observation)
	ambient.mu.Lock()
	defer ambient.mu.Unlock()
	require.Empty(t, ambient.requests)
}

func TestObserveTraceCleanupFailureIsVisibleWithoutPrivatePath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":"team-a","labels":{"kustomize.toolkit.fluxcd.io/name":"app"}}}`))
	}))
	defer server.Close()
	session, err := newTraceSession(&rest.Config{Host: server.URL}, "selected")
	require.NoError(t, err)
	_, err = observeTrace(context.Background(), session, "Deployment", "api", "team-a", traceObservationOptions{
		Flux: func(*traceSession) (agent.Tracer, func() error, error) {
			return nil, func() error { return fmt.Errorf("private-file-path") }, fmt.Errorf("setup failed")
		},
	})
	require.ErrorContains(t, err, "setup failed")
	require.ErrorContains(t, err, "unable to remove temporary Trace credentials")
	require.NotContains(t, err.Error(), "private-file-path")
}

type fixedTracer struct {
	result *agent.TraceResult
	err    error
}

func (f fixedTracer) Trace(context.Context, string, string, string) (*agent.TraceResult, error) {
	return f.result, f.err
}
func (fixedTracer) Available() bool  { return true }
func (fixedTracer) ToolName() string { return "flux" }

// A resource with no owner marker is an answer, not a failure. On real
// clusters the command printed "trace failed: ..." and no JSON at all
// (CI run 43484756241), because the Flux reader reports "no Flux marker" as an
// error. Whichever way the tracer says it, the observation carries the typed
// reason and words that name no tracer; a real read failure stays an error.
func TestObserveTraceOfAResourceWithNoOwnerMarkerIsAnAnswer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"coredns","namespace":"kube-system"}}`))
	}))
	defer server.Close()
	observe := func(tracer fixedTracer) (*traceObservation, error) {
		session, err := newTraceSession(&rest.Config{Host: server.URL}, "selected")
		require.NoError(t, err)
		return observeTrace(context.Background(), session, "Deployment", "coredns", "kube-system", traceObservationOptions{
			Flux: func(*traceSession) (agent.Tracer, func() error, error) {
				return tracer, func() error { return nil }, nil
			},
		})
	}
	for name, tracer := range map[string]fixedTracer{
		"the tracer returns an error":         {err: fmt.Errorf("%s", agent.NoFluxOwnerTraceMessage)},
		"the tracer returns a result":         {result: &agent.TraceResult{Tool: "flux", Error: agent.NoFluxOwnerTraceMessage, Reason: agent.TraceReasonNoOwnerMarker}},
		"the flux CLI's own words, no reason": {result: &agent.TraceResult{Tool: "flux", Error: "resource not managed by Flux"}},
	} {
		t.Run(name, func(t *testing.T) {
			observation, err := observe(tracer)
			require.NoError(t, err)
			require.Empty(t, observation.Result.Chain)
			require.Equal(t, agent.TraceReasonNoOwnerMarker, observation.Result.Reason)
			require.Contains(t, observation.Result.Error, agent.NoOwnerTraceMessage)
			require.NotContains(t, observation.Result.Error, "not managed")
			require.NotContains(t, observation.Result.Error, "Flux")
		})
	}
	t.Run("a read failure stays an error", func(t *testing.T) {
		observation, err := observe(fixedTracer{err: fmt.Errorf("kustomizations.kustomize.toolkit.fluxcd.io is forbidden")})
		require.ErrorContains(t, err, "forbidden")
		require.Nil(t, observation)
	})
}
