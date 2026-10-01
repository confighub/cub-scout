// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/confighub/cub-scout/v2/internal/summarystore"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type gitOpsContextServer struct {
	server   *httptest.Server
	marker   string
	requests []string
	tokens   []string
	caData   []byte
}

func newGitOpsContextServer(t *testing.T, marker string, deny bool) *gitOpsContextServer {
	t.Helper()
	s := &gitOpsContextServer{marker: marker}
	s.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests = append(s.requests, r.Method+" "+r.URL.Path)
		s.tokens = append(s.tokens, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		if deny && strings.Contains(r.URL.Path, "/modeldeployments") {
			http.Error(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Forbidden"}`, http.StatusForbidden)
			return
		}
		kind := "ResourceList"
		items := "[]"
		application := fmt.Sprintf(`{"apiVersion":"argoproj.io/v1alpha1","kind":"Application","metadata":{"name":"same","namespace":"team"},"spec":{"destination":{"namespace":"team"},"source":{"repoURL":"https://%s.example.invalid/repo.git"}},"status":{"sync":{"status":"Synced"},"health":{"status":"Healthy"}}}`, marker)
		if strings.HasSuffix(r.URL.Path, "/applications") {
			kind = "ApplicationList"
			items = "[" + application + "]"
		} else if strings.Contains(r.URL.Path, "/applications/") {
			_, _ = fmt.Fprint(w, application)
			return
		} else if strings.HasSuffix(r.URL.Path, "/pods") {
			kind = "PodList"
			items = `[{"apiVersion":"v1","kind":"Pod","metadata":{"name":"same-pod","namespace":"team","labels":{"argocd.argoproj.io/instance":"same"}},"status":{"phase":"Pending","containerStatuses":[{"name":"app","ready":false,"state":{"waiting":{"reason":"ImagePullBackOff"}}}]}}]`
		}
		if strings.Contains(r.URL.Path, "/deployments") {
			kind = "DeploymentList"
			items = fmt.Sprintf(`[{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"same","namespace":"team","labels":{"confighub.com/space":%q,"confighub.com/target":"target"}},"spec":{},"status":{}}]`, marker)
		}
		_, _ = fmt.Fprintf(w, `{"apiVersion":"v1","kind":%q,"metadata":{},"items":%s}`, kind, items)
	}))
	s.caData = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.server.Certificate().Raw})
	t.Cleanup(s.server.Close)
	return s
}

func gitOpsContextKubeconfig(t *testing.T, path, current string, alpha, beta *gitOpsContextServer) []byte {
	t.Helper()
	cfg := clientcmdapi.NewConfig()
	cfg.CurrentContext = current
	cfg.Clusters["alpha-cluster"] = &clientcmdapi.Cluster{Server: alpha.server.URL, CertificateAuthorityData: alpha.caData}
	cfg.Clusters["beta-cluster"] = &clientcmdapi.Cluster{Server: beta.server.URL, CertificateAuthorityData: beta.caData}
	cfg.AuthInfos["alpha-user"] = &clientcmdapi.AuthInfo{Token: "alpha-token"}
	cfg.AuthInfos["beta-user"] = &clientcmdapi.AuthInfo{Token: "beta-token"}
	cfg.Contexts["alpha"] = &clientcmdapi.Context{Cluster: "alpha-cluster", AuthInfo: "alpha-user"}
	cfg.Contexts["beta"] = &clientcmdapi.Context{Cluster: "beta-cluster", AuthInfo: "beta-user"}
	data, err := clientcmd.Write(*cfg)
	require.NoError(t, err)
	require.Contains(t, string(data), "token: alpha-token")
	require.Contains(t, string(data), "token: beta-token")
	require.NoError(t, os.WriteFile(path, data, 0600))
	return data
}

func prepareGitOpsContextState(t *testing.T) {
	t.Helper()
	oldSelector := gitopsKubeContext
	oldNamespace, oldFormat, oldJSON, oldTUI := gitopsNamespace, gitopsFormat, gitopsJSON, gitopsTUI
	oldWithConfigHub, oldSpace, oldSince, oldStale := gitopsWithConfigHub, gitopsConfigHubSpace, gitopsConfigHubSince, gitopsConfigHubStaleAfter
	flag := gitopsStatusCmd.Flags().Lookup("kube-context")
	oldChanged := flag.Changed
	oldConfigHubChanged := gitopsStatusCmd.Flags().Lookup("with-confighub").Changed
	oldSummaryConnected := summaryConnectedFn
	oldContext := rootCmd.Context()
	t.Cleanup(func() {
		gitopsKubeContext = oldSelector
		gitopsNamespace, gitopsFormat, gitopsJSON, gitopsTUI = oldNamespace, oldFormat, oldJSON, oldTUI
		gitopsWithConfigHub, gitopsConfigHubSpace, gitopsConfigHubSince, gitopsConfigHubStaleAfter = oldWithConfigHub, oldSpace, oldSince, oldStale
		flag.Changed = oldChanged
		gitopsStatusCmd.Flags().Lookup("with-confighub").Changed = oldConfigHubChanged
		summaryConnectedFn = oldSummaryConnected
		rootCmd.SetArgs(nil)
		rootCmd.SetContext(oldContext)
	})
	gitopsKubeContext = ""
	gitopsNamespace = ""
	gitopsFormat = "json"
	gitopsJSON = false
	gitopsTUI = false
	gitopsWithConfigHub = false
	gitopsConfigHubSpace = ""
	gitopsConfigHubSince = "24h"
	gitopsConfigHubStaleAfter = "15m"
	flag.Changed = false
	summaryConnectedFn = func() bool { return false }
	rootCmd.SetArgs(nil)
}

func runGitOpsStatusArgs(t *testing.T, args ...string) (string, error) {
	t.Helper()
	rootCmd.SetArgs(args)
	var commandErr error
	output := captureStdout(t, func() { commandErr = rootCmd.ExecuteContext(context.Background()) })
	return output, commandErr
}

func TestGitOpsStatusExplicitContextUsesCapturedEndpointAfterRetarget(t *testing.T) {
	prepareGitOpsContextState(t)
	alpha := newGitOpsContextServer(t, "alpha-space", false)
	beta := newGitOpsContextServer(t, "beta-space", false)
	path := filepath.Join(t.TempDir(), "config")
	before := gitOpsContextKubeconfig(t, path, "beta", alpha, beta)
	t.Setenv("KUBECONFIG", path)
	t.Setenv("CUB_SCOUT_TEST_GITOPS_JSON", "")
	raw, rawErr := clientcmd.LoadFromFile(path)
	require.NoError(t, rawErr)
	require.Equal(t, "alpha-token", raw.AuthInfos["alpha-user"].Token)
	resolved, _, resolveErr := resolveClusterConfig("alpha", true, clientcmd.NewDefaultClientConfigLoadingRules(), nil)
	require.NoError(t, resolveErr)
	require.Equal(t, alpha.server.URL, resolved.Host)
	require.Equal(t, "alpha-token", resolved.BearerToken)
	oldSessionFactory := newGitOpsStatusSessionForSelection
	newGitOpsStatusSessionForSelection = func(selection clusterContextSelection) (*traceSession, error) {
		session, err := newTraceSessionForSelection(selection)
		if err != nil {
			return nil, err
		}
		require.Equal(t, "alpha-token", session.config.BearerToken)
		retargeted := gitOpsContextKubeconfig(t, path, "beta", beta, beta)
		_ = retargeted
		return session, nil
	}
	t.Cleanup(func() { newGitOpsStatusSessionForSelection = oldSessionFactory })

	output, err := runGitOpsStatusArgs(t, "gitops", "status", "--kube-context", "alpha", "--format", "json")
	require.NoError(t, err)
	var summary GitOpsSummary
	require.NoError(t, json.Unmarshal([]byte(output), &summary))
	require.Equal(t, "alpha", summary.Context)
	require.Equal(t, "alpha-space", summary.ConfigHubTarget.Space)
	require.Len(t, summary.Deployers, 1)
	require.Equal(t, []RuntimeIssue{{Reason: "ImagePullBackOff", Count: 1}}, summary.Deployers[0].RuntimeIssues)
	require.NotEmpty(t, alpha.requests)
	require.Empty(t, beta.requests)
	for i, request := range alpha.requests {
		require.True(t, strings.HasPrefix(request, "GET "), "read-only API request[%d]=%s", i, request)
		require.Equal(t, "Bearer alpha-token", alpha.tokens[i], "the captured credential must remain paired with context alpha")
	}
	require.Contains(t, alpha.requests, "GET /api/v1/namespaces/team/pods")
	after, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.NotEqual(t, before, after, "the test retargeted the source file after capture")
	require.Contains(t, string(after), beta.server.URL)
}

func TestGitOpsStatusExplicitInvalidContextsFailBeforeReads(t *testing.T) {
	prepareGitOpsContextState(t)
	alpha := newGitOpsContextServer(t, "alpha-space", false)
	path := filepath.Join(t.TempDir(), "config")
	gitOpsContextKubeconfig(t, path, "alpha", alpha, alpha)
	t.Setenv("KUBECONFIG", path)
	t.Setenv("CUB_SCOUT_TEST_GITOPS_JSON", "")
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "empty", args: []string{"--kube-context", " "}, want: "non-empty"},
		{name: "missing", args: []string{"--kube-context", "missing"}, want: "not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := runGitOpsStatusArgs(t, append([]string{"gitops", "status"}, tc.args...)...)
			require.ErrorContains(t, err, tc.want)
			require.Empty(t, alpha.requests)
		})
	}
}

func TestGitOpsStatusDeniedControllerAPIIsVisibleOnSelectedEndpoint(t *testing.T) {
	prepareGitOpsContextState(t)
	alpha := newGitOpsContextServer(t, "alpha-space", true)
	beta := newGitOpsContextServer(t, "beta-space", false)
	path := filepath.Join(t.TempDir(), "config")
	gitOpsContextKubeconfig(t, path, "beta", alpha, beta)
	t.Setenv("KUBECONFIG", path)
	t.Setenv("CUB_SCOUT_TEST_GITOPS_JSON", "")
	output, err := runGitOpsStatusArgs(t, "gitops", "status", "--kube-context", "alpha", "--format", "json")
	require.NoError(t, err)
	var summary GitOpsSummary
	require.NoError(t, json.Unmarshal([]byte(output), &summary))
	var modelplane *ControllerCoverageStatus
	for i := range summary.ControllerCoverage {
		if summary.ControllerCoverage[i].Family == "Modelplane" {
			modelplane = &summary.ControllerCoverage[i]
		}
	}
	require.NotNil(t, modelplane)
	require.Equal(t, controllerCoverageUnreadable, modelplane.Status)
	require.Contains(t, modelplane.Omissions[0].Reason, "forbidden")
	require.Contains(t, strings.Join(alpha.requests, "\n"), "GET /apis/modelplane.ai/v1alpha1/modeldeployments")
	require.Empty(t, beta.requests)
	require.Equal(t, "alpha", summary.Context)
}

func TestGitOpsStatusRejectsExplicitContextWithRecordedFixtureBeforeReads(t *testing.T) {
	prepareGitOpsContextState(t)
	alpha := newGitOpsContextServer(t, "alpha-space", false)
	path := filepath.Join(t.TempDir(), "config")
	gitOpsContextKubeconfig(t, path, "alpha", alpha, alpha)
	t.Setenv("KUBECONFIG", path)
	fixture := filepath.Join(t.TempDir(), "status.json")
	require.NoError(t, os.WriteFile(fixture, []byte(`{"backend":"none","transport":"unknown"}`), 0600))
	t.Setenv("CUB_SCOUT_TEST_GITOPS_JSON", fixture)
	_, err := runGitOpsStatusArgs(t, "gitops", "status", "--kube-context", "alpha")
	require.ErrorContains(t, err, "cannot be combined")
	require.Empty(t, alpha.requests)
}

func TestGitOpsStatusOmittedSelectionPreservesCurrentContext(t *testing.T) {
	prepareGitOpsContextState(t)
	alpha := newGitOpsContextServer(t, "alpha-space", false)
	beta := newGitOpsContextServer(t, "beta-space", false)
	path := filepath.Join(t.TempDir(), "config")
	gitOpsContextKubeconfig(t, path, "beta", alpha, beta)
	t.Setenv("KUBECONFIG", path)
	t.Setenv("CUB_SCOUT_TEST_GITOPS_JSON", "")
	output, err := runGitOpsStatusArgs(t, "gitops", "status", "--format", "json")
	require.NoError(t, err)
	var summary GitOpsSummary
	require.NoError(t, json.Unmarshal([]byte(output), &summary))
	require.Equal(t, "beta", summary.Context)
	require.Equal(t, "beta-space", summary.ConfigHubTarget.Space)
	require.Empty(t, alpha.requests)
	require.NotEmpty(t, beta.requests)
}

func TestMCPGitOpsStatusContextRunsScoutAndRejectsWrongTypes(t *testing.T) {
	prepareGitOpsContextState(t)
	alpha := newGitOpsContextServer(t, "alpha-space", false)
	beta := newGitOpsContextServer(t, "beta-space", false)
	path := filepath.Join(t.TempDir(), "config")
	gitOpsContextKubeconfig(t, path, "beta", alpha, beta)
	t.Setenv("KUBECONFIG", path)
	t.Setenv("CUB_SCOUT_TEST_GITOPS_JSON", "")
	var args []string
	gateway := newMCPGateway(mcpTraceParserRunner(t, &args))
	contextSchema := gateway.tools["gitops_status"].Descriptor.InputSchema["properties"].(map[string]interface{})["context"].(map[string]interface{})
	require.Equal(t, "string", contextSchema["type"])
	require.Nil(t, gateway.tools["gitops_status"].Runner, "the mixed Kubernetes/ConfigHub tool must stay on Scout's runner")
	params, _ := json.Marshal(map[string]interface{}{"name": "gitops_status", "arguments": map[string]interface{}{"context": "alpha"}})
	response := gateway.handleRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/call", Params: params})
	require.Nil(t, response.Error)
	wire := decodeMCPResult(t, response.Result)
	require.False(t, wire.IsError)
	var summary GitOpsSummary
	require.NoError(t, json.Unmarshal([]byte(wire.Content[0].Text), &summary))
	require.Equal(t, "alpha", summary.Context)
	require.Equal(t, "alpha-space", summary.ConfigHubTarget.Space)
	require.Equal(t, []string{"gitops", "status", "--format", "json", "--kube-context", "alpha"}, args)
	require.NotEmpty(t, alpha.requests)
	require.Empty(t, beta.requests)
	for i, request := range alpha.requests {
		require.True(t, strings.HasPrefix(request, "GET "), "MCP API request[%d]=%s", i, request)
		require.Equal(t, "Bearer alpha-token", alpha.tokens[i])
	}

	readsBeforeInvalid := len(alpha.requests)
	for _, invalid := range []interface{}{17, "", "missing"} {
		params, _ := json.Marshal(map[string]interface{}{"name": "gitops_status", "arguments": map[string]interface{}{"context": invalid}})
		response = gateway.handleRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage(`2`), Method: "tools/call", Params: params})
		wire = decodeMCPResult(t, response.Result)
		require.True(t, wire.IsError, "invalid context %#v", invalid)
	}
	require.Len(t, alpha.requests, readsBeforeInvalid)
	require.Empty(t, beta.requests)
}

func TestGitOpsStatusConfigHubEnrichmentKeepsAuthSeparateFromKubeContext(t *testing.T) {
	prepareGitOpsContextState(t)
	alpha := newGitOpsContextServer(t, "alpha-space", false)
	beta := newGitOpsContextServer(t, "beta-space", false)
	path := filepath.Join(t.TempDir(), "config")
	gitOpsContextKubeconfig(t, path, "beta", alpha, beta)
	t.Setenv("KUBECONFIG", path)
	t.Setenv("CUB_SCOUT_TEST_GITOPS_JSON", "")
	gitopsWithConfigHub = true
	gitopsConfigHubSpace = "connected-space"
	oldRequire, oldRun := requireGitOpsConfigHubFn, runGitOpsCubCommand
	var commands [][]string
	requireGitOpsConfigHubFn = func() error { return nil }
	runGitOpsCubCommand = func(_ context.Context, args []string) (string, error) {
		commands = append(commands, append([]string(nil), args...))
		return "[]", nil
	}
	t.Cleanup(func() {
		requireGitOpsConfigHubFn, runGitOpsCubCommand = oldRequire, oldRun
	})
	output, err := runGitOpsStatusArgs(t, "gitops", "status", "--kube-context", "alpha", "--with-confighub", "--confighub-space", "connected-space", "--format", "json")
	require.NoError(t, err)
	var summary GitOpsSummary
	require.NoError(t, json.Unmarshal([]byte(output), &summary))
	require.Equal(t, "alpha", summary.Context)
	require.Equal(t, "alpha-space", summary.ConfigHubTarget.Space)
	require.NotNil(t, summary.DeliveryEvidence)
	require.Len(t, commands, 3, "connected ConfigHub queries use their separate mocked service path")
	require.NotEmpty(t, alpha.requests, "Kubernetes-backed evidence stays on the captured API")
	require.Empty(t, beta.requests)
}

func TestGitOpsStatusContextIsSharedWithTUIAndSummaryRecord(t *testing.T) {
	summary := GitOpsSummary{Context: "prod-context", Backend: "none", Transport: "unknown"}
	model := newGitOpsStatusTUIModel(summary)
	require.Contains(t, model.content, "prod-context")
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	require.Same(t, model, updated)
	_, quit := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	require.NotNil(t, quit)
	require.IsType(t, tea.QuitMsg{}, quit())
	record := buildGitOpsSummaryRecord(summary, "cluster-label", "team", gitOpsTestTime())
	require.Equal(t, "prod-context", record.ContextLabel)
	encoded, err := json.Marshal(record)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"contextLabel":"prod-context"`)

	unsafe := renderGitOpsStatusMarkdown(GitOpsSummary{Context: "prod`context\ncontinued"})
	require.Contains(t, unsafe, "prod`context continued")
	require.NotContains(t, unsafe, "\ncontinued")

	oldConnected, oldPersist := summaryConnectedFn, persistSummaryRecordFn
	t.Cleanup(func() { summaryConnectedFn, persistSummaryRecordFn = oldConnected, oldPersist })
	var saved summarystore.Record
	summaryConnectedFn = func() bool { return true }
	persistSummaryRecordFn = func(record summarystore.Record) error { saved = record; return nil }
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "does-not-exist"))
	persistConnectedGitOpsSummary(GitOpsSummary{}, "team")
	require.Equal(t, "unknown", saved.Cluster, "persistence must not reread ambient kubeconfig after collection")
}

func gitOpsTestTime() time.Time { return time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC) }
