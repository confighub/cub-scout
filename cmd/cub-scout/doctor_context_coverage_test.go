package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"k8s.io/client-go/tools/clientcmd"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/v2/internal/scan"
	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

func TestDoctorCoverageSurvivesJSONAndHumanRendering(t *testing.T) {
	t.Setenv("CUB_SCOUT_OFFLINE", "true")
	t.Setenv("PATH", t.TempDir())
	for _, mode := range []string{"denied", "partial", "unreachable"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if mode == "partial" && strings.HasSuffix(r.URL.Path, "/configmaps") {
					fmt.Fprint(w, `{"apiVersion":"v1","kind":"ConfigMapList","items":[{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"known-marker","namespace":"team-a","uid":"known-uid"}}]}`)
					return
				}
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"Forbidden","code":403,"message":"fixture-reader forbidden"}`)
			}))
			defer server.Close()
			if mode == "unreachable" {
				server.Close()
			}
			binding := &localClusterBinding{context: "selected-beta", explicit: true, config: &rest.Config{Host: server.URL, QPS: 1000, Burst: 1000, Timeout: time.Second}}
			old := selectScanProviderFn
			defer func() { selectScanProviderFn = old }()
			selectScanProviderFn = func(scan.ProviderConfig) scan.Provider {
				return &captureScanProvider{result: &scan.CombinedResult{
					Static: &agent.StaticScanResult{Findings: []agent.StaticFinding{{CCVEID: "CCVE-2026-0042", Severity: "warning", Category: "CONFIG", ResourceName: "Deployment/known", Namespace: "team-a", Message: "retained scanner finding"}}},
					State:  &agent.StateScanResult{Warnings: []string{"scanner coverage unavailable fixture"}},
				}}
			}
			result, err := ObserveScopeSummary(context.Background(), ObserveScopeSummaryRequest{Namespace: "team-a", TopIssues: 3, ClusterBinding: binding})
			require.NoError(t, err, "coverage failures should retain partial evidence")
			assert.Equal(t, "selected-beta", result.Summary.KubernetesContext)
			expectedResources := 0
			if mode == "partial" {
				expectedResources = 1
			}
			require.Equal(t, expectedResources, result.Summary.Resources.Total)
			require.Equal(t, 1, result.Summary.Risks.Total, "known scanner findings must survive other read failures")
			require.NotEmpty(t, result.Summary.Warnings)
			encoded, err := json.Marshal(result.Summary)
			require.NoError(t, err)
			var document map[string]any
			require.NoError(t, json.Unmarshal(encoded, &document))
			assert.Equal(t, "selected-beta", document["kubernetesContext"])
			require.NotEmpty(t, document["warnings"])
			for _, expected := range []string{"inventory", "rollout", "scanner coverage unavailable fixture"} {
				require.Contains(t, strings.Join(result.Summary.Warnings, " "), expected)
				rendered := renderDoctorASCII(result.Summary, DefaultPresentationMode, false, DefaultHintContext())
				require.Contains(t, rendered, expected, "human rendering must retain the same incomplete-coverage evidence as JSON")
				require.Contains(t, rendered, "selected-beta")
			}
		})
	}
}

// Closing and reopening scan is the existing TUI refresh gesture; no new key is
// introduced. Both actual S actions must reuse the session's captured config.
func TestExplicitContextTUIScanReopenUsesCapturedEndpoint(t *testing.T) {
	t.Setenv("CUB_SCOUT_OFFLINE", "true")
	t.Setenv("CUB_SCOUT_SCAN_PROVIDER", "legacy")
	t.Setenv("PATH", t.TempDir())
	alpha, beta := newCountedKubeServer(t), newCountedKubeServer(t)
	path, _ := resolverKubeconfig(t, "alpha", map[string]string{"alpha": alpha.server.URL, "beta": beta.server.URL})
	t.Setenv("KUBECONFIG", path)
	config, selected, err := resolveClusterConfig("beta", true, resolverRules(path), nil)
	require.NoError(t, err)
	model := LocalClusterModel{explicitClusterContext: true, clusterBinding: &localClusterBinding{config: config, context: selected, explicit: true}, keymap: defaultLocalKeyMap()}
	runVisibleScan := func() {
		before := beta.requests.Load()
		next, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("S")})
		require.NotNil(t, command)
		model = next.(LocalClusterModel)
		require.True(t, model.scanMode)
		msg := command().(scanResultMsg)
		require.NoError(t, msg.err)
		next, _ = model.Update(msg)
		model = next.(LocalClusterModel)
		require.False(t, model.scanLoading)
		require.Greater(t, beta.requests.Load(), before)
		require.Zero(t, alpha.requests.Load())
	}
	runVisibleScan()
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = next.(LocalClusterModel)
	require.False(t, model.scanMode)
	updated, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	updated.CurrentContext = "alpha"
	updated.Clusters["beta-cluster"].Server = alpha.server.URL
	bytes, err := clientcmd.Write(*updated)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, bytes, 0600))
	runVisibleScan()
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, bytes, after, "scan must not rewrite shared context/config to recover its binding")
}

func TestExplicitDoctorHintsNeverOfferUnboundFollowups(t *testing.T) {
	hints := bindDoctorHintContext([]Hint{
		{Command: "cub-scout map orphans -n team-a"},
		{Command: "cub-scout import --dry-run -n team-a"},
		{Command: "cub-scout quickstart -n team-a --yes"},
		{Command: "kubectl get pods"},
		{Command: "cub-scout scan --kube-context wrong"},
		{Command: "cub scout doctor --format json"},
	}, "selected")
	for _, hint := range hints[:5] {
		require.Empty(t, hint.Command)
		require.Contains(t, hint.Rationale, "no context-safe")
	}
	require.Equal(t, "cub scout doctor --format json --kube-context 'selected'", hints[5].Command)
}
