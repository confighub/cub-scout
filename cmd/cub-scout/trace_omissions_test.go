// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func TestSecretEvidenceReaderReturnsReadErrorsAndSafePartialEvidence(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
		want string
	}{{"forbidden", 403, "forbidden"}, {"missing", 404, "not found"}} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			counts := map[string]int{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				counts[r.URL.Path]++
				mu.Unlock()
				writeTraceStatus(w, tc.code, tc.want)
			}))
			defer server.Close()
			session, err := newTraceSession(&rest.Config{Host: server.URL}, "test")
			require.NoError(t, err)
			result, err := collectSecretEvidenceWithTraceSessionAndError(context.Background(), session, "Deployment", "app", "ns")
			require.ErrorContains(t, err, tc.want)
			require.Nil(t, result)
			require.Equal(t, 1, counts["/apis/apps/v1/namespaces/ns/deployments/app"], "one object read, no probe read")
			require.NotContains(t, err.Error(), "secret-payload-marker")
		})
	}

	t.Run("successful metadata never includes secret data", func(t *testing.T) {
		var mu sync.Mutex
		counts := map[string]int{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			counts[r.URL.Path]++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Path {
			case "/apis/apps/v1/namespaces/ns/deployments/app":
				_, _ = w.Write([]byte(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"app","namespace":"ns"},"spec":{"template":{"spec":{"containers":[{"name":"main","envFrom":[{"secretRef":{"name":"creds"}}]}]}}}}`))
			case "/api/v1/namespaces/ns/secrets/creds":
				_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"Secret","metadata":{"name":"creds","namespace":"ns"},"type":"Opaque","data":{"password":"secret-payload-marker"}}`))
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()
		session, err := newTraceSession(&rest.Config{Host: server.URL}, "test")
		require.NoError(t, err)
		result, err := collectSecretEvidenceWithTraceSessionAndError(context.Background(), session, "Deployment", "app", "ns")
		require.NoError(t, err)
		require.Equal(t, 1, result.Summary.Present)
		require.Equal(t, "creds", result.Secrets[0].Name)
		encoded, err := json.Marshal(result)
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "secret-payload-marker")
		require.Equal(t, 1, counts["/apis/apps/v1/namespaces/ns/deployments/app"])
		require.Equal(t, 1, counts["/api/v1/namespaces/ns/secrets/creds"])
	})
	t.Run("unsupported kind is not an omission", func(t *testing.T) {
		session, err := newTraceSession(&rest.Config{Host: "http://127.0.0.1:1"}, "test")
		require.NoError(t, err)
		result, err := collectSecretEvidenceWithTraceSessionAndError(context.Background(), session, "Job", "job", "ns")
		require.NoError(t, err)
		require.Nil(t, result)
	})
}

func TestArtifactReaderKeepsPartialMetadataAndReportsEachFailedSource(t *testing.T) {
	var mu sync.Mutex
	counts := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		counts[r.URL.Path]++
		mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/gitrepositories/denied") {
			writeTraceStatus(w, 403, "forbidden")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/gitrepositories/gone") {
			writeTraceStatus(w, 404, "not found")
			return
		}
		if strings.HasSuffix(r.URL.Path, "/gitrepositories/ready") {
			_, _ = w.Write([]byte(`{"apiVersion":"source.toolkit.fluxcd.io/v1","kind":"GitRepository","metadata":{"name":"ready","namespace":"sources"},"status":{"artifact":{"url":"https://artifact.invalid/ready","revision":"ready@sha1:abc","digest":"sha256:abc"}}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	session, err := newTraceSession(&rest.Config{Host: server.URL}, "test")
	require.NoError(t, err)
	result := &agent.TraceResult{Chain: []agent.ChainLink{{Kind: "GitRepository", Name: "ready", Namespace: "sources"}, {Kind: "GitRepository", Name: "denied", Namespace: "sources"}, {Kind: "GitRepository", Name: "gone", Namespace: "sources"}}}
	artifacts, errs := collectTraceArtifactsWithTraceSessionAndErrors(context.Background(), session, result)
	require.Equal(t, "https://artifact.invalid/ready", artifacts[traceArtifactKey("GitRepository", "sources", "ready")].URL)
	require.Equal(t, "ready@sha1:abc", artifacts[traceArtifactKey("GitRepository", "sources", "ready")].Revision)
	require.Len(t, errs, 2)
	require.ErrorContains(t, errs[0], "GitRepository/sources/denied")
	require.ErrorContains(t, errs[0], "forbidden")
	require.ErrorContains(t, errs[1], "GitRepository/sources/gone")
	require.ErrorContains(t, errs[1], "not found")
	require.Equal(t, 1, counts["/apis/source.toolkit.fluxcd.io/v1/namespaces/sources/gitrepositories/ready"])
	require.Equal(t, 1, counts["/apis/source.toolkit.fluxcd.io/v1/namespaces/sources/gitrepositories/denied"])
	require.Equal(t, 1, counts["/apis/source.toolkit.fluxcd.io/v1/namespaces/sources/gitrepositories/gone"])
	// The legacy wrapper retains its original map-only API.
	legacy := collectTraceArtifactsWithTraceSession(context.Background(), session, result)
	require.Equal(t, artifacts[traceArtifactKey("GitRepository", "sources", "ready")], legacy[traceArtifactKey("GitRepository", "sources", "ready")])
}

func TestObserveTraceProjectsArtifactReadOmissionAsJSONWarning(t *testing.T) {
	var mu sync.Mutex
	counts := map[string]int{}
	kustomizationPath := "/apis/kustomize.toolkit.fluxcd.io/v1/namespaces/team-a/kustomizations/root"
	readyPath := "/apis/source.toolkit.fluxcd.io/v1/namespaces/sources/gitrepositories/ready"
	deniedPath := "/apis/source.toolkit.fluxcd.io/v1/namespaces/sources/gitrepositories/denied"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		counts[r.URL.Path]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case kustomizationPath:
			_, _ = w.Write([]byte(`{"apiVersion":"kustomize.toolkit.fluxcd.io/v1","kind":"Kustomization","metadata":{"name":"root","namespace":"team-a","labels":{"kustomize.toolkit.fluxcd.io/name":"root"}},"spec":{"sourceRef":{"kind":"GitRepository","name":"ready"}}}`))
		case readyPath:
			_, _ = w.Write([]byte(`{"apiVersion":"source.toolkit.fluxcd.io/v1","kind":"GitRepository","metadata":{"name":"ready","namespace":"sources"},"status":{"artifact":{"url":"https://artifact.invalid/ready","revision":"r1"}}}`))
		case deniedPath:
			writeTraceStatus(w, 403, "forbidden")
		case "/api/v1/namespaces/team-a/events":
			_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"EventList","items":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	session, err := newTraceSession(&rest.Config{Host: server.URL}, "selected")
	require.NoError(t, err)
	fake := omissionTestTracer{result: &agent.TraceResult{Object: agent.ResourceRef{Kind: "Kustomization", Name: "root", Namespace: "team-a"}, Chain: []agent.ChainLink{{Kind: "GitRepository", Name: "ready", Namespace: "sources"}, {Kind: "GitRepository", Name: "denied", Namespace: "sources"}, {Kind: "Kustomization", Name: "root", Namespace: "team-a"}}}}
	observation, err := observeTrace(context.Background(), session, "Kustomization", "root", "team-a", traceObservationOptions{Artifacts: true, Flux: func(*traceSession) (agent.Tracer, func() error, error) { return fake, nil, nil }})
	require.NoError(t, err)
	require.NotNil(t, observation)
	require.Equal(t, "https://artifact.invalid/ready", observation.Artifacts[traceArtifactKey("GitRepository", "sources", "ready")].URL)
	require.Equal(t, "unknown", observation.Artifacts[traceArtifactKey("GitRepository", "sources", "denied")].URL)
	require.Contains(t, observation.Result.Error, "artifact metadata unavailable for GitRepository/sources/denied")
	require.Contains(t, observation.Result.Error, "forbidden")
	output := convertTraceToV014(observation.Result, "Kustomization", "root", "team-a", observation.Artifacts)
	require.Len(t, output.Warnings, 1)
	require.Contains(t, output.Warnings[0], "GitRepository/sources/denied")
	require.Contains(t, output.Warnings[0], "forbidden")
	// Each source is read once for existing timing enrichment and once for
	// artifact enrichment; no extra request is made just to diagnose failure.
	require.Equal(t, 2, counts[readyPath])
	require.Equal(t, 2, counts[deniedPath])
}

type omissionTestTracer struct{ result *agent.TraceResult }

func (f omissionTestTracer) ToolName() string { return "flux" }
func (f omissionTestTracer) Available() bool  { return true }
func (f omissionTestTracer) Trace(context.Context, string, string, string) (*agent.TraceResult, error) {
	return f.result, nil
}

func writeTraceStatus(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(metav1.Status{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"}, Status: metav1.StatusFailure, Message: message, Reason: map[int]metav1.StatusReason{403: metav1.StatusReasonForbidden, 404: metav1.StatusReasonNotFound}[code], Code: int32(code)})
}
