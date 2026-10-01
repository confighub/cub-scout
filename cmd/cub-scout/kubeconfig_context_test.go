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
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const resolverTestToken = "test-only-kubeconfig-token-do-not-log"

type countedKubeServer struct {
	server   *httptest.Server
	requests atomic.Int32
	mu       sync.Mutex
	paths    []string
}

func newCountedKubeServer(t *testing.T) *countedKubeServer {
	t.Helper()
	result := &countedKubeServer{}
	result.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result.requests.Add(1)
		result.mu.Lock()
		result.paths = append(result.paths, r.URL.Path)
		result.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"apiVersion":"v1","kind":"NamespaceList","metadata":{},"items":[]}`)
	}))
	t.Cleanup(result.server.Close)
	return result
}

func (s *countedKubeServer) requestPaths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.paths...)
}

func resolverKubeconfig(t *testing.T, current string, contexts map[string]string) (string, []byte) {
	t.Helper()
	cfg := clientcmdapi.NewConfig()
	cfg.CurrentContext = current
	for name, server := range contexts {
		clusterName, userName := name+"-cluster", name+"-user"
		cfg.Clusters[clusterName] = &clientcmdapi.Cluster{Server: server}
		cfg.AuthInfos[userName] = &clientcmdapi.AuthInfo{Token: resolverTestToken}
		cfg.Contexts[name] = &clientcmdapi.Context{Cluster: clusterName, AuthInfo: userName}
	}
	data, err := clientcmd.Write(*cfg)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "kubeconfig")
	require.NoError(t, os.WriteFile(path, data, 0600))
	return path, data
}

func resolverRules(path string) *clientcmd.ClientConfigLoadingRules {
	return &clientcmd.ClientConfigLoadingRules{ExplicitPath: path}
}

func requestKubernetesNamespaces(t *testing.T, config *rest.Config) {
	t.Helper()
	client, err := kubernetes.NewForConfig(config)
	require.NoError(t, err)
	_, err = client.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
}

func TestResolveClusterConfigExplicitContextUsesOnlyNamedServer(t *testing.T) {
	alpha := newCountedKubeServer(t)
	beta := newCountedKubeServer(t)
	path, before := resolverKubeconfig(t, "alpha", map[string]string{"alpha": alpha.server.URL, "beta": beta.server.URL})

	inClusterCalls := 0
	cfg, selected, err := resolveClusterConfig("beta", true, resolverRules(path), func() (*rest.Config, error) {
		inClusterCalls++
		return &rest.Config{Host: alpha.server.URL}, nil
	})
	require.NoError(t, err)
	require.Equal(t, "beta", selected)
	require.Equal(t, 0, inClusterCalls, "strict selection must not consult in-cluster credentials")
	require.Equal(t, beta.server.URL, cfg.Host)
	requestKubernetesNamespaces(t, cfg)
	require.EqualValues(t, 0, alpha.requests.Load())
	require.EqualValues(t, 1, beta.requests.Load())
	require.Equal(t, []string{"/api/v1/namespaces"}, beta.requestPaths())
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after, "resolving a named context must not rewrite kubeconfig")
}

func TestDoctorNestedReadsStayOnCapturedContextAfterKubeconfigRetarget(t *testing.T) {
	t.Setenv("CUB_SCOUT_OFFLINE", "true")
	t.Setenv("CLUSTER_NAME", "")
	t.Setenv("PATH", t.TempDir())
	oldGate := requireGitOpsConfigHubFn
	requireGitOpsConfigHubFn = func() error { return fmt.Errorf("recorded ConfigHub unavailable") }
	t.Cleanup(func() { requireGitOpsConfigHubFn = oldGate })
	markedServer := func(marker, uid string) *countedKubeServer {
		server := &countedKubeServer{}
		server.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			server.requests.Add(1)
			server.mu.Lock()
			server.paths = append(server.paths, r.Method+" "+r.URL.Path)
			server.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/apis/apps/v1/deployments" {
				_, _ = fmt.Fprintf(w, `{"apiVersion":"apps/v1","kind":"DeploymentList","metadata":{},"items":[{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"same","namespace":"team-a","uid":%q,"labels":{"proof":%q}},"spec":{},"status":{}}]}`, uid, marker)
				return
			}
			_, _ = fmt.Fprint(w, `{"apiVersion":"v1","kind":"NamespaceList","metadata":{},"items":[]}`)
		}))
		t.Cleanup(server.server.Close)
		return server
	}
	alpha := markedServer("alpha", "alpha-uid")
	beta := markedServer("beta", "beta-uid")
	path, _ := resolverKubeconfig(t, "gamma", map[string]string{"alpha": alpha.server.URL, "beta": beta.server.URL, "gamma": alpha.server.URL})
	t.Setenv("KUBECONFIG", path)
	// Use the same resolver rules as the CLI, while keeping the test independent
	// of the machine's default kubeconfig.
	config, selected, err := resolveClusterConfig("beta", true, resolverRules(path), nil)
	require.NoError(t, err)
	binding := &localClusterBinding{config: config, context: selected, explicit: true}

	retargeted, loadErr := clientcmd.LoadFromFile(path)
	require.NoError(t, loadErr)
	retargeted.Clusters["beta-cluster"].Server = alpha.server.URL
	retargeted.CurrentContext = "alpha"
	data, writeErr := clientcmd.Write(*retargeted)
	require.NoError(t, writeErr)
	require.NoError(t, os.WriteFile(path, data, 0600))

	t.Setenv("CUB_SCOUT_SCAN_PROVIDER", "legacy")
	t.Setenv("CUB_SPACE", "")
	before := beta.requests.Load()
	entries, cluster, err := collectDoctorEntriesWithBinding(context.Background(), "", binding)
	require.NoError(t, err)
	require.Equal(t, "default", cluster, "context names must not be represented as stable cluster IDs")
	require.Contains(t, beta.requestPaths(), "GET /apis/apps/v1/deployments")
	foundMarker := false
	for _, entry := range entries {
		if entry.Name == "same" && entry.Kind == "Deployment" {
			foundMarker = true
			require.Equal(t, "beta", entry.Labels["proof"])
			require.Equal(t, "default", entry.ClusterName)
		}
	}
	require.True(t, foundMarker, "selected server's colliding deployment identity should be observed")
	require.Greater(t, beta.requests.Load(), before, "inventory must use selected server")
	require.Zero(t, alpha.requests.Load(), "inventory must not use ambient endpoint")

	before = beta.requests.Load()
	findings, err := collectDoctorFindingsWithBinding(context.Background(), "", binding)
	require.NoError(t, err)
	require.Empty(t, findings, "empty runtime fixtures contain no risk finding")
	require.Greater(t, beta.requests.Load(), before, "findings provider must use selected endpoint")
	require.Zero(t, alpha.requests.Load(), "findings provider must not use ambient endpoint")
	before = beta.requests.Load()
	rollouts, err := collectDoctorRolloutsWithBinding(context.Background(), "", 3, binding)
	require.NoError(t, err)
	require.NotNil(t, rollouts)
	require.Equal(t, 1, rollouts.Total, "selected deployment must appear in rollout evidence")
	require.Greater(t, beta.requests.Load(), before, "rollout reader must use selected endpoint")
	require.Zero(t, alpha.requests.Load(), "rollout reader must not use ambient endpoint")
	before = beta.requests.Load()
	delivery, err := collectDoctorDeliveryEvidenceWithBinding(context.Background(), "", ObserveScopeSummaryRequest{WithConfigHub: true}, binding)
	require.NoError(t, err)
	require.NotNil(t, delivery)
	require.Empty(t, delivery.EventConsumers, "fixture deployment has no consumer identity")
	require.Contains(t, delivery.Omissions, GitOpsDeliveryEvidenceOmission{Layer: "confighub", Reason: "recorded ConfigHub unavailable", Impact: "release, unit-event, and live-status evidence are omitted"})
	require.Greater(t, beta.requests.Load(), before, "delivery evidence reader must use selected endpoint")
	require.Zero(t, alpha.requests.Load(), "delivery reader must not use ambient endpoint")
	require.Zero(t, alpha.requests.Load(), "later kubeconfig current-context changes must not redirect reads")
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, data, after, "nested readers never rewrite kubeconfig")
}

func TestResolveClusterConfigMissingExplicitContextFailsWithoutCredentialFallback(t *testing.T) {
	alpha := newCountedKubeServer(t)
	path, before := resolverKubeconfig(t, "", map[string]string{"default": alpha.server.URL})
	inClusterCalls := 0
	cfg, selected, err := resolveClusterConfig("missing", true, resolverRules(path), func() (*rest.Config, error) {
		inClusterCalls++
		return &rest.Config{Host: alpha.server.URL}, nil
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing")
	require.Nil(t, cfg)
	require.Empty(t, selected)
	require.Zero(t, inClusterCalls, "missing explicit selection must fail instead of falling back")
	encoded, marshalErr := json.Marshal(map[string]string{"error": err.Error()})
	require.NoError(t, marshalErr)
	require.NotContains(t, string(encoded), resolverTestToken, "configuration errors must not expose credentials")
	require.Zero(t, alpha.requests.Load())
	after, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, before, after)
}

func TestResolveClusterConfigDoesNotApplyKubeconfigMigrationRules(t *testing.T) {
	alpha := newCountedKubeServer(t)
	path, _ := resolverKubeconfig(t, "alpha", map[string]string{"alpha": alpha.server.URL})
	destination := filepath.Join(t.TempDir(), "migrated-kubeconfig")
	rules := resolverRules(path)
	rules.MigrationRules = map[string]string{destination: path}

	cfg, selected, err := resolveClusterConfig("alpha", true, rules, nil)
	require.NoError(t, err)
	require.Equal(t, "alpha", selected)
	require.Equal(t, alpha.server.URL, cfg.Host)
	_, statErr := os.Stat(destination)
	require.ErrorIs(t, statErr, os.ErrNotExist, "context resolution must not write a migrated kubeconfig")
	require.Equal(t, map[string]string{destination: path}, rules.MigrationRules, "resolver must not mutate caller-owned rules")
}

func TestResolveClusterConfigOmittedSelectionKeepsLegacyPriority(t *testing.T) {
	alpha := newCountedKubeServer(t)
	inCluster := newCountedKubeServer(t)
	path, before := resolverKubeconfig(t, "", map[string]string{"default": alpha.server.URL})

	// The TUI may display "default" when kubeconfig has no current context.
	// A legacy launch still prefers in-cluster credentials over that display name.
	binding := resolveLocalClusterBinding("default", resolverRules(path), func() (*rest.Config, error) {
		return &rest.Config{Host: inCluster.server.URL}, nil
	})
	require.NoError(t, binding.err)
	require.Empty(t, binding.context, "in-cluster inventory has no kubeconfig context binding")
	require.Equal(t, inCluster.server.URL, binding.config.Host)
	model := LocalClusterModel{contextName: "default", clusterBinding: binding}
	loaded, ok := model.loadLocalClusterData().(localDataLoadedMsg)
	require.True(t, ok)
	require.NoError(t, loaded.err)
	require.Empty(t, loaded.boundedContext)
	require.Positive(t, inCluster.requests.Load())
	require.Zero(t, alpha.requests.Load())

	// If in-cluster credentials are unavailable, the TUI's legacy default
	// context hint selects the default context, as before.
	fallback, selected, err := resolveClusterConfig("default", false, resolverRules(path), func() (*rest.Config, error) {
		return nil, fmt.Errorf("not running in cluster")
	})
	require.NoError(t, err)
	require.Equal(t, "default", selected)
	require.Equal(t, alpha.server.URL, fallback.Host)
	requestKubernetesNamespaces(t, fallback)
	require.EqualValues(t, 1, alpha.requests.Load())
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestLocalClusterTUIRefreshUsesPinnedKubeconfigContext(t *testing.T) {
	alpha := newCountedKubeServer(t)
	beta := newCountedKubeServer(t)
	path, _ := resolverKubeconfig(t, "alpha", map[string]string{"alpha": alpha.server.URL, "beta": beta.server.URL})
	rules := resolverRules(path)
	binding := resolveLocalClusterBinding("", rules, func() (*rest.Config, error) {
		return nil, fmt.Errorf("not running in cluster")
	})
	require.NoError(t, binding.err)
	require.Equal(t, "alpha", binding.context)

	// Simulate a kubeconfig current-context change during a TUI session. A
	// refresh must reuse the immutable binding captured on launch.
	updated, _ := resolverKubeconfig(t, "beta", map[string]string{"alpha": alpha.server.URL, "beta": beta.server.URL})
	updatedBytes, err := os.ReadFile(updated)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, updatedBytes, 0600))

	model := LocalClusterModel{contextName: "alpha", clusterBinding: binding}
	loaded, ok := model.loadLocalClusterData().(localDataLoadedMsg)
	require.True(t, ok)
	require.NoError(t, loaded.err)
	require.Equal(t, "alpha", loaded.boundedContext)
	require.Positive(t, alpha.requests.Load())
	require.Zero(t, beta.requests.Load(), "TUI refresh must not switch to changed current-context")
}

func TestMapListExplicitContextSelectsOnlyNamedServerWithoutConfigMutation(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("CUB_SCOUT_TEST_MAP_ENTRIES_JSON", "")
	var alphaRequests, betaRequests atomic.Int32
	server := func(requests *atomic.Int32) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"apiVersion":"v1","kind":"List","metadata":{},"items":[]}`)
		}))
	}
	alpha := server(&alphaRequests)
	beta := server(&betaRequests)
	defer alpha.Close()
	defer beta.Close()
	path, before := resolverKubeconfig(t, "alpha", map[string]string{"alpha": alpha.URL, "beta": beta.URL})
	t.Setenv("KUBECONFIG", path)

	flag := mapListCmd.Flags().Lookup("kube-context")
	require.NotNil(t, flag)
	oldValue, oldChanged := flag.Value.String(), flag.Changed
	t.Cleanup(func() {
		_ = flag.Value.Set(oldValue)
		flag.Changed = oldChanged
	})
	for _, tc := range []struct {
		name     string
		value    string
		changed  bool
		selected string
		wantErr  string
	}{
		{name: "selected beta", value: "beta", changed: true, selected: "beta"},
		{name: "omitted uses current alpha", value: "", changed: false, selected: "alpha"},
		{name: "missing explicit name fails", value: "missing", changed: true, wantErr: "missing"},
		{name: "empty explicit name fails", value: "  ", changed: true, wantErr: "non-empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			beforeAlpha, beforeBeta := alphaRequests.Load(), betaRequests.Load()
			require.NoError(t, flag.Value.Set(tc.value))
			flag.Changed = tc.changed
			err := runMapList(mapListCmd, nil)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			if tc.wantErr != "" {
				require.Equal(t, beforeAlpha, alphaRequests.Load())
				require.Equal(t, beforeBeta, betaRequests.Load(), "invalid explicit contexts must not fall back")
			} else if tc.selected == "alpha" {
				require.Greater(t, alphaRequests.Load(), beforeAlpha)
				require.Equal(t, beforeBeta, betaRequests.Load())
			} else {
				require.Equal(t, beforeAlpha, alphaRequests.Load())
				require.Greater(t, betaRequests.Load(), beforeBeta)
			}
		})
	}
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after, "map list context selection must not rewrite kubeconfig")
	for _, unsupported := range []*cobra.Command{mapStatusCmd, mapFleetCmd, mapActivityCmd} {
		require.Nil(t, unsupported.Flags().Lookup("kube-context"), "%s must not inherit the map list-only flag", unsupported.Name())
	}
}

func TestMapTUISelectionBindsInventoryAndBoundedExplainAndSurvivesHandoff(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	var alphaRequests, betaRequests atomic.Int32
	server := func(requests *atomic.Int32) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			w.Header().Set("Content-Type", "application/json")
			if strings.HasSuffix(r.URL.Path, "/deployments/api") {
				_, _ = fmt.Fprint(w, `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":"team-a","managedFields":[]},"spec":{},"status":{}}`)
				return
			}
			_, _ = fmt.Fprint(w, `{"apiVersion":"v1","kind":"List","metadata":{},"items":[]}`)
		}))
	}
	alpha, beta := server(&alphaRequests), server(&betaRequests)
	defer alpha.Close()
	defer beta.Close()
	path, before := resolverKubeconfig(t, "alpha", map[string]string{"alpha": alpha.URL, "beta": beta.URL})
	t.Setenv("KUBECONFIG", path)
	mapFlag := mapCmd.Flags().Lookup("kube-context")
	require.NotNil(t, mapFlag)
	oldMapValue, oldMapChanged := mapFlag.Value.String(), mapFlag.Changed
	t.Cleanup(func() { _ = mapFlag.Value.Set(oldMapValue); mapFlag.Changed = oldMapChanged })
	require.NoError(t, mapFlag.Value.Set("beta"))
	mapFlag.Changed = true
	selection, err := clusterContextSelectionFromFlag(mapCmd)
	require.NoError(t, err)
	require.Equal(t, clusterContextSelection{name: "beta", explicit: true}, selection)
	binding := resolveLocalClusterBindingForSelection(selection)
	require.NoError(t, binding.err)
	require.True(t, binding.explicit)
	model := initialLocalModelWithBinding(ViewOptions{}, binding)
	loaded := model.loadLocalClusterData().(localDataLoadedMsg)
	require.NoError(t, loaded.err)
	require.Equal(t, "beta", loaded.boundedContext)
	require.Zero(t, alphaRequests.Load())
	require.Positive(t, betaRequests.Load())
	afterInventory, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, afterInventory, "TUI selection and inventory reads must not mutate kubeconfig")

	model.entries = []MapEntry{{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "team-a", Name: "api"}}
	model.boundedContext = loaded.boundedContext
	model.openBoundedExplain()
	beforeBeta := betaRequests.Load()
	// Retarget beta's name to alpha; the selected model and bounded session
	// must continue using the config captured at startup.
	raw, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	raw.Clusters["beta-cluster"].Server = alpha.URL
	updated, err := clientcmd.Write(*raw)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, updated, 0600))
	cmd := model.boundedPanel.read(false)
	require.NotNil(t, cmd)
	_ = cmd().(boundedExplainMsg)
	require.Greater(t, betaRequests.Load(), beforeBeta)
	require.Zero(t, alphaRequests.Load(), "bounded explain must not follow a same-name kubeconfig retarget")

	// Exercise the production local→Hub→local control-loop seam. The Hub may
	// run while the same-name kubeconfig entry changes, but re-entry gets the
	// exact captured binding object back.
	calls := 0
	err = runLocalClusterWithSwitchUsing(binding,
		func(got *localClusterBinding) (bool, string, bool, *localClusterBinding, error) {
			calls++
			require.Same(t, binding, got)
			if calls == 1 {
				return true, "hub-app", false, got, nil
			}
			return false, "", false, got, nil
		},
		func(string) (bool, error) { return true, nil },
		func() error { t.Fatal("explicit context must never enter import wizard"); return nil },
	)
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	model = initialLocalModelWithBinding(ViewOptions{}, binding)
	require.Equal(t, beta.URL, model.clusterBinding.config.Host)
	loaded = model.loadLocalClusterData().(localDataLoadedMsg)
	require.NoError(t, loaded.err)
	require.Positive(t, betaRequests.Load())
	require.Zero(t, alphaRequests.Load())
}

func TestMapCommandContextFlagIsLocalAndExplicitEmptyFails(t *testing.T) {
	mapFlag := mapCmd.Flags().Lookup("kube-context")
	listFlag := mapListCmd.Flags().Lookup("kube-context")
	require.NotNil(t, mapFlag)
	require.NotNil(t, listFlag)
	for _, unsupported := range []*cobra.Command{mapStatusCmd, mapFleetCmd, mapActivityCmd} {
		require.Nil(t, unsupported.InheritedFlags().Lookup("kube-context"), "%s must not inherit a context flag", unsupported.Name())
	}
	oldValue, oldChanged := mapFlag.Value.String(), mapFlag.Changed
	t.Cleanup(func() { _ = mapFlag.Value.Set(oldValue); mapFlag.Changed = oldChanged })
	path, _ := resolverKubeconfig(t, "default", map[string]string{"default": "http://127.0.0.1:1"})
	t.Setenv("KUBECONFIG", path)
	for _, tc := range []struct{ value, want string }{{"", "non-empty"}, {"missing", "missing"}} {
		require.NoError(t, mapFlag.Value.Set(tc.value))
		mapFlag.Changed = true
		err := runMapTUI(mapCmd, nil)
		require.ErrorContains(t, err, tc.want)
	}
}

func TestExplicitContextTUIActionsFailClosed(t *testing.T) {
	server := newCountedKubeServer(t)
	binDir := t.TempDir()
	marker := filepath.Join(binDir, "spawned")
	script := "#!/bin/sh\nprintf invoked >> \"$SCOUT_ACTION_MARKER\"\n"
	for _, name := range []string{"cub-scout", "flux", "fake-command", "shell"} {
		path := filepath.Join(binDir, name)
		require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	}
	t.Setenv("PATH", binDir)
	t.Setenv("SCOUT_ACTION_MARKER", marker)
	t.Setenv("SHELL", filepath.Join(binDir, "shell"))
	t.Chdir(binDir)
	oldGraphExport := runGraphExportCommand
	runGraphExportCommand = func(string, string) error {
		_, err := os.Stat(marker)
		return err
	}
	t.Cleanup(func() { runGraphExportCommand = oldGraphExport })

	model := LocalClusterModel{explicitClusterContext: true, keymap: defaultLocalKeyMap()}
	trace := model.runTrace(TraceItem{Kind: "Deployment", Name: "api", Namespace: "team-a", Owner: "Flux"})().(traceResultMsg)
	require.ErrorContains(t, trace.err, "unavailable with --kube-context")
	scan := model.runScan()().(scanResultMsg)
	require.ErrorContains(t, scan.err, "selected Kubernetes context is unavailable")
	graph := model.runGraphExport("svg")().(graphExportMsg)
	require.ErrorContains(t, graph.err, "unavailable with --kube-context")
	shell := model.runShellOut()().(shellExitMsg)
	require.ErrorContains(t, shell.err, "unavailable with --kube-context")
	require.Zero(t, server.requests.Load(), "blocked TUI actions must not make an API read")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("I")})
	require.Nil(t, cmd)
	require.False(t, updated.(LocalClusterModel).switchToImport)
	require.Contains(t, updated.(LocalClusterModel).statusMsg, "unavailable with --kube-context")
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	require.Nil(t, cmd)
	require.False(t, updated.(LocalClusterModel).cmdMode)

	model.cmdMode = true // Also defend against a stale/pre-opened command prompt.
	model.cmdInput = "fake-command"
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.Nil(t, cmd)
	require.False(t, updated.(LocalClusterModel).cmdRunning)
	require.False(t, updated.(LocalClusterModel).cmdMode)
	_, err := os.Stat(marker)
	require.True(t, os.IsNotExist(err), "blocked actions must not spawn subprocesses")
}

func TestExplicitContextTUISScanKeyUsesPinnedProvider(t *testing.T) {
	alpha := newCountedKubeServer(t)
	beta := newCountedKubeServer(t)
	path, _ := resolverKubeconfig(t, "alpha", map[string]string{"alpha": alpha.server.URL, "beta": beta.server.URL})
	config, selected, err := resolveClusterConfig("beta", true, resolverRules(path), nil)
	require.NoError(t, err)
	model := LocalClusterModel{explicitClusterContext: true, clusterBinding: &localClusterBinding{config: config, context: selected, explicit: true}, keymap: defaultLocalKeyMap()}
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("S")})
	require.NotNil(t, cmd, "the visible TUI scan action must run with the explicit binding")
	require.True(t, updated.(LocalClusterModel).scanMode)
	result := cmd().(scanResultMsg)
	require.NotContains(t, fmt.Sprint(result.err), explicitContextUnsupportedAction)
	require.Positive(t, beta.requests.Load(), "TUI scan should use the selected API server")
	require.Zero(t, alpha.requests.Load(), "TUI scan must not consult the ambient current context")
}

func TestDoctorHintCommandsPreserveOnlySupportedContextSurfaces(t *testing.T) {
	hints := bindDoctorHintContext([]Hint{
		{Command: "cub-scout doctor --format json"},
		{Command: "cub-scout scan --json"},
		{Command: "cub-scout map list --format json"},
		{Command: "cub-scout trace Deployment/api --format json"},
	}, "team's-cluster")
	require.Contains(t, hints[0].Command, `--kube-context 'team'"'"'s-cluster'`)
	require.Contains(t, hints[1].Command, "--kube-context")
	require.Contains(t, hints[2].Command, "--kube-context")
	require.NotContains(t, hints[3].Command, "--kube-context", "unsupported trace surface must not receive the selector")
	require.Empty(t, hints[3].Command, "a trace command would silently use the ambient context")
	require.Contains(t, hints[3].Rationale, "no context-safe trace command")
}

func TestResolveClusterConfigParallelSelectionsStayIndependent(t *testing.T) {
	alpha := newCountedKubeServer(t)
	beta := newCountedKubeServer(t)
	path, before := resolverKubeconfig(t, "alpha", map[string]string{"alpha": alpha.server.URL, "beta": beta.server.URL})
	rules := resolverRules(path)
	type result struct {
		name   string
		config *rest.Config
		err    error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, name := range []string{"alpha", "beta"} {
		name := name
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg, selected, err := resolveClusterConfig(name, true, rules, nil)
			if err == nil && selected != name {
				err = fmt.Errorf("selected context %q, want %q", selected, name)
			}
			results <- result{name: name, config: cfg, err: err}
		}()
	}
	wg.Wait()
	close(results)
	resolved := map[string]*rest.Config{}
	for result := range results {
		require.NoError(t, result.err)
		resolved[result.name] = result.config
	}
	require.Equal(t, alpha.server.URL, resolved["alpha"].Host)
	require.Equal(t, beta.server.URL, resolved["beta"].Host)
	requestKubernetesNamespaces(t, resolved["alpha"])
	requestKubernetesNamespaces(t, resolved["beta"])
	require.EqualValues(t, 1, alpha.requests.Load())
	require.EqualValues(t, 1, beta.requests.Load())
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
