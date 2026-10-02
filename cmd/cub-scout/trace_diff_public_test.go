// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/stretchr/testify/require"
)

func TestTraceDiffCLIRequiresRenderedOperandBeforeAnyDelegateOrRead(t *testing.T) {
	prepareTraceSelectionTest(t)
	traceApp, traceNamespace = "", "team-a"
	traceDiff = true
	traceDesiredFile = ""
	traceAPIVersion = ""
	binDir := t.TempDir()
	marker := filepath.Join(binDir, "spawned")
	script := "#!/bin/sh\nprintf invoked >> \"$TRACE_DIFF_MARKER\"\n"
	for _, name := range []string{"flux", "argocd", "helm"} {
		require.NoError(t, os.WriteFile(filepath.Join(binDir, name), []byte(script), 0o755))
	}
	t.Setenv("TRACE_DIFF_MARKER", marker)
	t.Setenv("PATH", binDir)
	cmd := selectedTraceCommand(t, "missing-context")
	err := runTrace(cmd, []string{"deployment/api"})
	require.ErrorContains(t, err, "--desired-file")
	_, statErr := os.Stat(marker)
	require.True(t, os.IsNotExist(statErr), "missing operand must fail before any external diff command")
}

func TestTraceDiffCLIUsesCapturedContextAndRendersOneModelInAllFormats(t *testing.T) {
	for _, format := range []string{"json", "ascii", "md"} {
		t.Run(format, func(t *testing.T) {
			prepareTraceSelectionTest(t)
			traceApp, traceNamespace = "", "team-a"
			var alphaRequests, betaRequests atomicCounter
			alpha := httptest.NewServer(traceDiffHandler(t, "/apis/apps/v1/namespaces/team-a/deployments/api", `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":"team-a","uid":"uid-alpha","resourceVersion":"8"},"spec":{"replicas":2}}`, http.StatusOK, &alphaRequests.value))
			defer alpha.Close()
			beta := httptest.NewServer(traceDiffHandler(t, "/apis/apps/v1/namespaces/team-a/deployments/api", `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":"team-a","uid":"uid-beta"},"spec":{"replicas":4}}`, http.StatusOK, &betaRequests.value))
			defer beta.Close()
			configPath := filepath.Join(t.TempDir(), "config")
			before := writeTraceKubeconfig(t, configPath, "beta-context", alpha.URL, beta.URL)
			t.Setenv("KUBECONFIG", configPath)
			path := traceDiffManifest(t, "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: api, namespace: team-a}\nspec: {replicas: 1}\n")
			traceDiff, traceDesiredFile, traceAPIVersion, traceFormat = true, path, "apps/v1", format
			var runErr error
			output := captureStdout(t, func() { runErr = runTrace(selectedTraceCommand(t, "alpha-context"), []string{"deployment/api"}) })
			require.NoError(t, runErr)
			require.Contains(t, output, "uid-alpha")
			require.Contains(t, output, "local-rendered")
			require.Contains(t, output, "authored")
			if format == "json" {
				require.Contains(t, output, `"context": "alpha-context"`)
				require.Contains(t, output, `"resourceVersion": "8"`)
				require.Contains(t, output, `"field": "spec.replicas"`)
			}
			require.Equal(t, int32(2), alphaRequests.value.Load())
			require.Zero(t, betaRequests.value.Load())
			after, err := os.ReadFile(configPath)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestTraceDiffCLIOmittedNamespaceUsesManifestNamespace(t *testing.T) {
	prepareTraceSelectionTest(t)
	traceNamespace, traceApp = "", ""
	var requests atomic.Int32
	server := httptest.NewServer(traceDiffHandler(t, "/apis/apps/v1/namespaces/team-a/deployments/api", `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":"team-a","uid":"alpha-uid"},"spec":{"replicas":1}}`, http.StatusOK, &requests))
	defer server.Close()
	configPath := filepath.Join(t.TempDir(), "config")
	writeTraceKubeconfig(t, configPath, "alpha-context", server.URL, server.URL)
	t.Setenv("KUBECONFIG", configPath)
	traceDiff, traceDesiredFile, traceAPIVersion, traceFormat = true,
		traceDiffManifest(t, "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: api, namespace: team-a}\nspec: {replicas: 1}\n"),
		"apps/v1", "json"

	var runErr error
	output := captureStdout(t, func() { runErr = runTrace(selectedTraceCommand(t, "alpha-context"), []string{"deployment/api"}) })
	require.NoError(t, runErr)
	require.Contains(t, output, `"namespace": "team-a"`)
	require.Contains(t, output, `"uid": "alpha-uid"`)
	require.Equal(t, int32(2), requests.Load(), "manifest namespace avoids separate scope discovery")
}

func TestTraceDiffAPIVersionDisambiguatesBeforeReads(t *testing.T) {
	prepareTraceSelectionTest(t)
	var requests atomicCounter
	server := httptest.NewServer(traceDiffHandler(t, "/apis/apps/v1/namespaces/team-a/deployments/api", `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":"team-a"},"spec":{"replicas":1}}`, http.StatusOK, &requests.value))
	defer server.Close()
	path := traceDiffManifest(t, "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: api, namespace: team-a}\nspec: {replicas: 1}\n---\napiVersion: extensions/v1beta1\nkind: Deployment\nmetadata: {name: api, namespace: team-a}\nspec: {replicas: 1}\n")
	session := traceDiffSession(t, server.URL, "alpha")
	_, err := observeTraceDiffWithAPIVersion(context.Background(), session, "Deployment", "api", "team-a", "", path)
	require.ErrorContains(t, err, "ambiguous")
	require.Zero(t, requests.value.Load())
	got, err := observeTraceDiffWithAPIVersion(context.Background(), session, "Deployment", "api", "team-a", "apps/v1", path)
	require.NoError(t, err)
	require.Equal(t, "apps/v1", got.Resource.APIVersion)
	require.Equal(t, int32(2), requests.value.Load())
}

func TestTraceDiffNamespaceUsesExactManifestNamespaceOrFailsClosed(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(traceDiffHandler(t, "/apis/apps/v1/namespaces/team-a/deployments/api", `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":"team-a"},"spec":{"replicas":1}}`, http.StatusOK, &requests))
	defer server.Close()
	session := traceDiffSession(t, server.URL, "alpha")

	withNamespace := traceDiffManifest(t, "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: api, namespace: team-a}\nspec: {replicas: 1}\n")
	got, err := observeTraceDiffWithAPIVersion(context.Background(), session, "Deployment", "api", "", "apps/v1", withNamespace)
	require.NoError(t, err)
	require.Equal(t, "team-a", got.Resource.Namespace, "omitted -n uses the manifest's exact namespace")
	requests.Store(0)

	withoutNamespace := traceDiffManifest(t, "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: api}\nspec: {replicas: 1}\n")
	_, err = observeTraceDiffWithAPIVersion(context.Background(), session, "Deployment", "api", "", "apps/v1", withoutNamespace)
	require.ErrorContains(t, err, "has no namespace")
	require.ErrorContains(t, err, "scope discovery GETs=1")
	require.Equal(t, int32(1), requests.Load(), "only the namespaced-scope discovery may be read; no live object GET")

	requests.Store(0)
	withNamespaceFlag := traceDiffManifest(t, "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: api}\nspec: {replicas: 1}\n")
	got, err = observeTraceDiffWithAPIVersion(context.Background(), session, "Deployment", "api", "team-a", "apps/v1", withNamespaceFlag)
	require.NoError(t, err, "-n can supply a missing namespace for a namespaced resource")
	require.Equal(t, "team-a", got.Resource.Namespace)
	require.Equal(t, 1, got.ScopeDiscoveryReads, "scope selection GET is distinct from bounded-reader request counts")
	require.Equal(t, int32(3), requests.Load(), "scope discovery, bounded-reader discovery, and exact live GET use the supplied namespace")

	requests.Store(0)
	twoNamespaces := traceDiffManifest(t, "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: api, namespace: team-a}\nspec: {replicas: 1}\n---\napiVersion: apps/v1\nkind: Deployment\nmetadata: {name: api, namespace: team-b}\nspec: {replicas: 1}\n")
	_, err = observeTraceDiffWithAPIVersion(context.Background(), session, "Deployment", "api", "", "apps/v1", twoNamespaces)
	require.ErrorContains(t, err, "ambiguous")
	require.Zero(t, requests.Load(), "ambiguous namespaces fail before API reads")

	_, err = observeTraceDiffWithAPIVersion(context.Background(), session, "Deployment", "api", "team-b", "apps/v1", withNamespace)
	require.ErrorContains(t, err, "does not match")
	require.Zero(t, requests.Load(), "namespace mismatch fails before API reads")
}

func TestTraceDiffRejectsInvalidOperandIdentityBeforeAnyDiscovery(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(traceDiffHandler(t, "", "", http.StatusOK, &requests))
	defer server.Close()
	session := traceDiffSession(t, server.URL, "alpha")
	for _, tc := range []struct {
		name, manifest, kind, objectName, namespace string
		want                                        string
	}{
		{name: "api version", manifest: "apiVersion: apps//v1\nkind: Deployment\nmetadata: {name: api, namespace: team-a}\n", kind: "Deployment", objectName: "api", want: "invalid Kubernetes identity"},
		{name: "name", manifest: "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: bad_name, namespace: team-a}\n", kind: "Deployment", objectName: "bad_name", want: "invalid Kubernetes identity"},
		{name: "kind", manifest: "apiVersion: apps/v1\nkind: Bad Kind\nmetadata: {name: api}\n", kind: "Bad Kind", objectName: "api", want: "invalid Kubernetes identity"},
		{name: "manifest namespace", manifest: "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: api, namespace: Team_A}\n", kind: "Deployment", objectName: "api", want: "invalid Kubernetes identity"},
		{name: "requested namespace", manifest: "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: api}\n", kind: "Deployment", objectName: "api", namespace: "BAD_NS", want: "requested namespace is invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests.Store(0)
			_, err := observeTraceDiff(context.Background(), session, tc.kind, tc.objectName, tc.namespace, traceDiffManifest(t, tc.manifest))
			require.ErrorContains(t, err, tc.want)
			require.Zero(t, requests.Load(), "malformed identity must fail before discovery or object reads")
		})
	}
}

func TestTraceDiffMarkdownCarriesIdentityReadEvidenceAndEscapesCells(t *testing.T) {
	observedAt := time.Date(2026, 10, 1, 12, 13, 14, 0, time.UTC)
	result := &traceDiffObservation{
		Status: traceDiffStatusChanged, Comparison: "authored-fields-only", Coverage: "one-selected-object",
		Source:  traceDiffSource{Kind: "local-rendered", Reference: "manifest.yaml", Digest: "digest"},
		Context: "alpha", Resource: agent.BoundedResourceRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "team-a", Name: "api"},
		ScopeDiscoveryReads: 1,
		Read:                &agent.BoundedReadEvidence{UID: "uid-alpha", ResourceVersion: "9", ObservedAt: observedAt, Reads: agent.BoundedReadCounts{Discovery: 1, Object: 1}},
		Differences:         []agent.ObjectSetFieldDiff{{Field: "spec.note|name", Desired: "wanted|one\nline", Live: "has`tick"}},
	}
	var output strings.Builder
	require.NoError(t, renderTraceDiffObservationMarkdown(&output, result))
	markdown := output.String()
	for _, want := range []string{
		"apps/v1 Deployment team-a/api", "2026-10-01T12:13:14Z",
		"scope discovery=1; bounded reader discovery=1 object=1", "spec.note\\|name",
		"wanted\\|one<br>line", "has&#96;tick",
	} {
		require.Contains(t, markdown, want)
	}
}

func TestTraceDiffTUIResultCancelsCompletedContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	model := LocalClusterModel{traceMode: true, traceDiffMode: true, traceDiffLoading: true, traceDiffRequestID: 3, traceDiffCancel: cancel}
	updated, _ := model.Update(traceDiffResultMsg{requestID: 3, output: "complete"})
	model = updated.(LocalClusterModel)
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	require.False(t, model.traceDiffLoading)
	require.Nil(t, model.traceDiffCancel)
}

func TestTraceDiffRejectsNamespaceForClusterScopedDesiredObject(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		require.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1" {
			t.Errorf("unexpected API request %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"groupVersion":"v1","resources":[{"name":"namespaces","kind":"Namespace","namespaced":false,"verbs":["get"]}]}`))
	}))
	defer server.Close()
	session := traceDiffSession(t, server.URL, "alpha")
	desired := traceDiffManifest(t, "apiVersion: v1\nkind: Namespace\nmetadata: {name: team-a}\n")
	_, err := observeTraceDiffWithAPIVersion(context.Background(), session, "Namespace", "team-a", "team-a", "v1", desired)
	require.ErrorContains(t, err, "cluster-scoped")
	require.ErrorContains(t, err, "scope discovery GETs=1")
	require.Equal(t, int32(1), requests.Load(), "scope discovery only; no resource GET")
}

func TestTraceDiffTUIRejectsCanceledStaleResult(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(traceDiffHandler(t, "/apis/apps/v1/namespaces/team-a/deployments/api", `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":"team-a"},"spec":{"replicas":1}}`, http.StatusOK, &requests))
	defer server.Close()
	pathConfig := filepath.Join(t.TempDir(), "config")
	writeTraceKubeconfig(t, pathConfig, "alpha-context", server.URL, server.URL)
	t.Setenv("KUBECONFIG", pathConfig)
	binding := resolveLocalClusterBindingForSelection(clusterContextSelection{name: "alpha-context", explicit: true})
	require.NoError(t, binding.err)
	desired := traceDiffManifest(t, "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: api, namespace: team-a}\nspec: {replicas: 1}\n")
	ctx, cancel := context.WithCancel(context.Background())
	model := LocalClusterModel{clusterBinding: binding, traceMode: true, traceDiffMode: true, traceDiffLoading: true, traceDiffRequestID: 4, traceDiffCancel: cancel, traceCursor: 0}
	command := model.runTraceDiff(ctx, 4, TraceItem{Kind: "Deployment", Name: "api", Namespace: "team-a"}, desired, "apps/v1")
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(LocalClusterModel)
	require.False(t, model.traceDiffMode)
	require.Equal(t, 5, int(model.traceDiffRequestID))
	message := command().(traceDiffResultMsg)
	updated, _ = model.Update(message)
	model = updated.(LocalClusterModel)
	require.Empty(t, model.traceDiffOutput, "stale completion after cancellation must not overwrite the selected view")
	require.Zero(t, requests.Load(), "cancelled pending work performs no API read")
}

func TestTraceDiffTUIExplicitPathUsesBoundSessionAndRetainsSelection(t *testing.T) {
	var alphaRequests, betaRequests atomic.Int32
	alpha := httptest.NewServer(traceDiffHandler(t, "/apis/apps/v1/namespaces/team-a/deployments/api", `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":"team-a","uid":"uid-alpha","resourceVersion":"4"},"spec":{"replicas":2}}`, http.StatusOK, &alphaRequests))
	defer alpha.Close()
	beta := httptest.NewServer(traceDiffHandler(t, "/apis/apps/v1/namespaces/team-a/deployments/api", `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":"team-a","uid":"uid-beta"},"spec":{"replicas":9}}`, http.StatusOK, &betaRequests))
	defer beta.Close()
	pathConfig := filepath.Join(t.TempDir(), "config")
	writeTraceKubeconfig(t, pathConfig, "alpha-context", alpha.URL, beta.URL)
	t.Setenv("KUBECONFIG", pathConfig)
	binding := resolveLocalClusterBindingForSelection(clusterContextSelection{name: "alpha-context", explicit: true})
	require.NoError(t, binding.err)
	model := LocalClusterModel{
		clusterBinding: binding, explicitClusterContext: true, keymap: defaultLocalKeyMap(),
		traceMode: true, traceItems: []TraceItem{{Kind: "Deployment", Name: "api", Namespace: "team-a", Owner: "Flux"}},
	}
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	require.Nil(t, cmd, "picker input must not start a read")
	model = updated.(LocalClusterModel)
	require.True(t, model.traceDiffMode)
	for _, r := range traceDiffManifest(t, "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: api, namespace: team-a}\nspec: {replicas: 1}\n") {
		updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		require.Nil(t, cmd, "picker input must not start a read")
		model = updated.(LocalClusterModel)
	}
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.Nil(t, cmd, "picker input must not start a read")
	model = updated.(LocalClusterModel)
	require.Equal(t, "api-version", model.traceDiffPrompt)
	for _, r := range "apps/v1" {
		updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		require.Nil(t, cmd, "picker input must not start a read")
		model = updated.(LocalClusterModel)
	}
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(LocalClusterModel)

	require.NotNil(t, cmd)
	message := cmd().(traceDiffResultMsg)
	require.NoError(t, message.err)
	require.Contains(t, message.output, "uid-alpha")
	updated, _ = model.Update(message)
	model = updated.(LocalClusterModel)
	require.Contains(t, model.renderTrace(), "uid-alpha")
	require.Equal(t, 0, model.traceCursor)

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	model = updated.(LocalClusterModel)
	require.False(t, model.traceDiffMode)
	require.True(t, model.traceMode)
	require.Equal(t, 0, model.traceCursor)
	require.Greater(t, alphaRequests.Load(), int32(0))
	require.Zero(t, betaRequests.Load())
}

type atomicCounter struct{ value atomic.Int32 }
