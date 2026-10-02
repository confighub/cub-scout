// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// Recorded API contracts exercise the same dynamic client as live observation,
// without a cluster, ConfigHub, discovery permissions, or ambient credentials.
func TestFetchRolloutDecisionFromCapturedClient(t *testing.T) {
	const deployment = `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"app","namespace":"team","generation":1},"spec":{"replicas":1,"selector":{"matchLabels":{"app":"app"}}},"status":{"observedGeneration":1,"replicas":1,"updatedReplicas":1,"availableReplicas":0}}`
	const pods = `{"apiVersion":"v1","kind":"PodList","items":[{"apiVersion":"v1","kind":"Pod","metadata":{"name":"app-pod","namespace":"team","labels":{"app":"app"}},"status":{"phase":"Pending","containerStatuses":[{"name":"app","ready":false,"state":{"waiting":{"reason":"ImagePullBackOff"}}}]}}]}`
	for _, tc := range []struct {
		name                              string
		workloadStatus, podStatus         int
		cancelBefore, cancelAfterWorkload bool
		wantDecision                      bool
		wantReads                         int32
	}{
		{name: "runtime failure", workloadStatus: 200, podStatus: 200, wantDecision: true, wantReads: 2},
		{name: "workload missing", workloadStatus: 404, podStatus: 200, wantReads: 1},
		{name: "workload forbidden", workloadStatus: 403, podStatus: 200, wantReads: 1},
		{name: "pods forbidden preserves workload-only behavior", workloadStatus: 200, podStatus: 403, wantDecision: true, wantReads: 2},
		{name: "cancel before workload", workloadStatus: 200, podStatus: 200, cancelBefore: true},
		{name: "cancel between reads", workloadStatus: 200, podStatus: 200, cancelAfterWorkload: true, wantDecision: true, wantReads: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var reads, ambientReads atomic.Int32
			ambient := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ambientReads.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer ambient.Close()
			selected := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reads.Add(1)
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "Bearer selected-token", r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/apis/apps/v1/namespaces/team/deployments/app":
					if tc.workloadStatus != 200 {
						w.WriteHeader(tc.workloadStatus)
						_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"Status","status":"Failure"}`))
						return
					}
					_, _ = w.Write([]byte(deployment))
				case "/api/v1/namespaces/team/pods":
					assert.Equal(t, "app=app", r.URL.Query().Get("labelSelector"))
					w.WriteHeader(tc.podStatus)
					if tc.podStatus != 200 {
						_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"Status","status":"Failure"}`))
						return
					}
					_, _ = w.Write([]byte(pods))
				default:
					t.Errorf("unexpected API path: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer selected.Close()
			cfg := &rest.Config{Host: selected.URL, BearerToken: "selected-token", TLSClientConfig: rest.TLSClientConfig{CAData: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: selected.Certificate().Raw})}}
			if tc.cancelAfterWorkload {
				cfg.WrapTransport = func(next http.RoundTripper) http.RoundTripper {
					return rolloutTransportFunc(func(r *http.Request) (*http.Response, error) {
						if r.URL.Path == "/api/v1/namespaces/team/pods" {
							cancel()
							return nil, ctx.Err()
						}
						return next.RoundTrip(r)
					})
				}
			}
			client, err := dynamic.NewForConfig(cfg)
			require.NoError(t, err)
			// Supply a different ambient endpoint after capturing the client.
			path := filepath.Join(t.TempDir(), "config")
			ambientConfig := []byte("apiVersion: v1\nkind: Config\ncurrent-context: ambient\ncontexts:\n- name: ambient\n  context:\n    cluster: ambient\nclusters:\n- name: ambient\n  cluster:\n    server: " + ambient.URL + "\n    insecure-skip-tls-verify: true\n")
			require.NoError(t, os.WriteFile(path, ambientConfig, 0600))
			t.Setenv("KUBECONFIG", path)
			if tc.cancelBefore {
				cancel()
			}
			decision, ok := fetchRolloutDecisionFrom(ctx, client, "team", "Deployment", "app")
			require.Equal(t, tc.wantDecision, ok)
			if ok {
				require.NotNil(t, decision)
				if tc.podStatus == 200 && !tc.cancelAfterWorkload {
					require.Equal(t, agent.RolloutReasonRuntimeFailed, decision.Reason)
					require.Len(t, decision.Evidence.PodReasons, 1)
					require.Equal(t, "ImagePullBackOff", decision.Evidence.PodReasons[0].Reason)
				} else {
					require.Empty(t, decision.Evidence.PodReasons)
					require.NotEqual(t, agent.RolloutReasonConverged, decision.Reason)
				}
			} else {
				require.Nil(t, decision)
			}
			require.Equal(t, tc.wantReads, reads.Load())
			require.Zero(t, ambientReads.Load())
			unchanged, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, ambientConfig, unchanged)
		})
	}
}

type rolloutTransportFunc func(*http.Request) (*http.Response, error)

func (f rolloutTransportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFetchRolloutDecisionFromUnavailableClient(t *testing.T) {
	decision, ok := fetchRolloutDecisionFrom(nil, nil, "team", "Deployment", "app")
	require.False(t, ok)
	require.Nil(t, decision)
}
