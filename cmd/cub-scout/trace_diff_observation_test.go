// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

const traceDiffDiscovery = `{"groupVersion":"apps/v1","resources":[{"name":"deployments","kind":"Deployment","namespaced":true,"verbs":["get"]}]}`

func traceDiffManifest(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "desired.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func traceDiffSession(t *testing.T, host, contextName string) *traceSession {
	t.Helper()
	session, err := newTraceSession(&rest.Config{Host: host}, contextName)
	require.NoError(t, err)
	return session
}

func traceDiffHandler(t *testing.T, wantPath, live string, status int, requests *atomic.Int32) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		require.Equal(t, http.MethodGet, r.Method, "the observation primitive must be GET-only")
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/apps/v1":
			_, _ = w.Write([]byte(traceDiffDiscovery))
		case wantPath:
			w.WriteHeader(status)
			_, _ = w.Write([]byte(live))
		default:
			t.Errorf("unexpected bounded API path %q", r.URL.Path)
			http.NotFound(w, r)
		}
	})
}

func TestObserveTraceDiffBindsBeforeCallerRetargetAndReportsAuthoredFields(t *testing.T) {
	var alphaRequests, betaRequests atomic.Int32
	alpha := httptest.NewServer(traceDiffHandler(t, "/apis/apps/v1/namespaces/team-a/deployments/api", `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":"team-a","uid":"uid-alpha","resourceVersion":"7"},"spec":{"replicas":2},"status":{"availableReplicas":1}}`, http.StatusOK, &alphaRequests))
	defer alpha.Close()
	beta := httptest.NewServer(traceDiffHandler(t, "/apis/apps/v1/namespaces/team-a/deployments/api", `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":"team-a","uid":"uid-beta"},"spec":{"replicas":9}}`, http.StatusOK, &betaRequests))
	defer beta.Close()
	callerConfig := &rest.Config{Host: alpha.URL}
	session, err := newTraceSession(callerConfig, "alpha")
	require.NoError(t, err)
	callerConfig.Host = beta.URL
	path := traceDiffManifest(t, `apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: team-a
spec:
  replicas: 1
  strategy:
    type: RollingUpdate
`)

	got, err := observeTraceDiff(context.Background(), session, "Deployment", "api", "team-a", path)
	require.NoError(t, err)
	require.Equal(t, traceDiffStatusChanged, got.Status)
	require.Equal(t, "local-rendered", got.Source.Kind)
	require.NotEmpty(t, got.Source.Digest)
	require.Equal(t, "alpha", got.Context)
	require.Equal(t, "uid-alpha", got.Read.UID)
	require.Equal(t, 1, got.Summary.Total)
	require.Equal(t, 1, got.Summary.Changed)
	require.Equal(t, []string{"spec.replicas", "spec.strategy"}, traceDiffFields(got.Differences))
	require.Equal(t, int32(2), alphaRequests.Load(), "one discovery GET and one exact object GET")
	require.Zero(t, betaRequests.Load(), "caller retarget after session construction must not redirect reads")
	encoded, err := json.Marshal(got)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "availableReplicas", "status is not authored desired state")
}

func TestObserveTraceDiffSelectsExactGVKAndNamespaceBeforeReads(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(traceDiffHandler(t, "/apis/apps/v1/namespaces/team-a/deployments/api", `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":"team-a"},"spec":{"replicas":1}}`, http.StatusOK, &requests))
	defer server.Close()
	session := traceDiffSession(t, server.URL, "alpha")
	path := traceDiffManifest(t, `apiVersion: apps/v1
kind: Deployment
metadata: {name: api, namespace: team-a}
spec: {replicas: 1}
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: api, namespace: team-b}
spec: {replicas: 1}
`)
	got, err := observeTraceDiff(context.Background(), session, "deployment", "api", "team-a", path)
	require.NoError(t, err)
	require.Equal(t, "team-a", got.Resource.Namespace)
	require.Equal(t, int32(2), requests.Load())

	requests.Store(0)
	_, err = observeTraceDiff(context.Background(), session, "Deployment", "api", "", path)
	require.ErrorContains(t, err, "ambiguous")
	require.Zero(t, requests.Load(), "ambiguous namespace must be rejected before API reads")
}

func TestObserveTraceDiffRejectsBadDesiredBeforeReads(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(traceDiffHandler(t, "/apis/apps/v1/namespaces/team-a/deployments/api", `{}`, http.StatusOK, &requests))
	defer server.Close()
	session := traceDiffSession(t, server.URL, "alpha")
	cases := []struct{ name, body, want string }{
		{"absent", "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: other, namespace: team-a}\n", "not found"},
		{"ambiguous GVK", "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: api, namespace: team-a}\n---\napiVersion: apps/v1beta1\nkind: Deployment\nmetadata: {name: api, namespace: team-a}\n", "ambiguous"},
		{"malformed", "apiVersion: [\nkind: Deployment\n", "parse"},
		{"invalid API identity", "apiVersion: invalid group/version\nkind: Deployment\nmetadata: {name: api, namespace: team-a}\n", "invalid Kubernetes identity"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requests.Store(0)
			_, err := observeTraceDiff(context.Background(), session, "Deployment", "api", "team-a", traceDiffManifest(t, tc.body))
			require.ErrorContains(t, err, tc.want)
			require.Zero(t, requests.Load(), "invalid operand must fail before API reads")
		})
	}
}

func TestObserveTraceDiffMissingAndInconclusiveAreDistinct(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		want   string
	}{
		{"missing", http.StatusNotFound, traceDiffStatusMissing},
		{"forbidden", http.StatusForbidden, traceDiffStatusInconclusive},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			statusReason := "Forbidden"
			if tc.status == http.StatusNotFound {
				statusReason = "NotFound"
			}
			statusBody := fmt.Sprintf(`{"kind":"Status","apiVersion":"v1","status":"Failure","reason":%q,"code":%d,"message":"private-server-detail"}`, statusReason, tc.status)
			server := httptest.NewServer(traceDiffHandler(t, "/apis/apps/v1/namespaces/team-a/deployments/api", statusBody, tc.status, &requests))
			defer server.Close()
			got, err := observeTraceDiff(context.Background(), traceDiffSession(t, server.URL, "alpha"), "Deployment", "api", "team-a", traceDiffManifest(t, "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: api, namespace: team-a}\nspec: {replicas: 1}\n"))
			require.NoError(t, err)
			require.Equal(t, tc.want, got.Status)
			if tc.status == http.StatusForbidden {
				require.Contains(t, got.Omissions[0], "live object could not be read; comparison is inconclusive")
			} else {
				require.Equal(t, 1, got.Summary.Removed)
			}
			encoded, marshalErr := json.Marshal(got)
			require.NoError(t, marshalErr)
			require.NotContains(t, string(encoded), "private-server-detail")
			require.Equal(t, int32(2), requests.Load())
		})
	}
}

func TestObserveTraceDiffSecretWithholdsPayloadAndDigest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(traceDiffHandler(t, "", "", http.StatusOK, &requests))
	defer server.Close()
	path := traceDiffManifest(t, "apiVersion: v1\nkind: Secret\nmetadata: {name: db, namespace: team-a}\ndata:\n  password: c2Vuc2l0aXZl\n")
	got, err := observeTraceDiff(context.Background(), traceDiffSession(t, server.URL, "alpha"), "Secret", "db", "team-a", path)
	require.NoError(t, err)
	require.Equal(t, traceDiffStatusInconclusive, got.Status)
	require.Contains(t, got.Omissions[0], "Secret payload comparison omitted")
	require.Empty(t, got.Source.Digest, "whole-source digest could encode Secret payload information")
	encoded, marshalErr := json.Marshal(got)
	require.NoError(t, marshalErr)
	require.NotContains(t, string(encoded), "c2Vuc2l0aXZl")
	require.Empty(t, got.Differences)
	require.Zero(t, requests.Load(), "bounded reader intentionally excludes Secrets")
}

func TestObserveTraceDiffRejectsMalformedSecretIdentityBeforeReads(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(traceDiffHandler(t, "", "", http.StatusOK, &requests))
	defer server.Close()
	session := traceDiffSession(t, server.URL, "alpha")
	cases := []struct{ name, body string }{
		{"apiVersion", "apiVersion: invalid group/version\nkind: Secret\nmetadata: {name: db, namespace: team-a}\ndata: {password: c2VjcmV0}\n"},
		{"name", "apiVersion: v1\nkind: Secret\nmetadata: {name: Invalid_Name, namespace: team-a}\ndata: {password: c2VjcmV0}\n"},
		{"namespace", "apiVersion: v1\nkind: Secret\nmetadata: {name: db, namespace: Invalid_NS}\ndata: {password: c2VjcmV0}\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requests.Store(0)
			_, err := observeTraceDiff(context.Background(), session, "Secret", "db", "", traceDiffManifest(t, tc.body))
			require.ErrorContains(t, err, "invalid Kubernetes identity")
			require.Zero(t, requests.Load(), "malformed Secret identity must fail before API reads")
		})
	}
}

func TestObserveTraceDiffIgnoresStatusAndUnrequestedDefaults(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(traceDiffHandler(t, "/apis/apps/v1/namespaces/team-a/deployments/api", `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":"team-a","uid":"uid-1","resourceVersion":"9","generation":3},"spec":{"replicas":1,"revisionHistoryLimit":10},"status":{"availableReplicas":1}}`, http.StatusOK, &requests))
	defer server.Close()
	path := traceDiffManifest(t, "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: api, namespace: team-a}\nspec: {replicas: 1}\n")
	got, err := observeTraceDiff(context.Background(), traceDiffSession(t, server.URL, "alpha"), "Deployment", "api", "team-a", path)
	require.NoError(t, err)
	require.Equal(t, traceDiffStatusMatched, got.Status)
	require.Empty(t, got.Differences)
	require.Zero(t, got.Summary.Changed)
}

func TestTraceDiffParserErrorsDoNotEchoManifest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(traceDiffHandler(t, "", "", http.StatusOK, &requests))
	defer server.Close()
	path := traceDiffManifest(t, "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: api, namespace: team-a}\nspec: [private-manifest-value\n")
	_, err := observeTraceDiff(context.Background(), traceDiffSession(t, server.URL, "alpha"), "Deployment", "api", "team-a", path)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "private-manifest-value")
	require.Zero(t, requests.Load())
}

func traceDiffFields(diffs []agent.ObjectSetFieldDiff) []string {
	fields := make([]string, 0, len(diffs))
	for _, diff := range diffs {
		fields = append(fields, diff.Field)
	}
	return fields
}
