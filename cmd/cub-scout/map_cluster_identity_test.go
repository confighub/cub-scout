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
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const mapIdentityBody = `{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"kube-system","uid":"selected-cluster-instance"}}`

type mapIdentityFixture struct {
	server         *httptest.Server
	identityReads  atomic.Int64
	allReads       atomic.Int64
	denyIdentity   atomic.Bool
	denyInventory  atomic.Bool
	emptyInventory atomic.Bool
}

func newMapIdentityFixture(t *testing.T) *mapIdentityFixture {
	t.Helper()
	f := &mapIdentityFixture{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.allReads.Add(1)
		if r.Method != "GET" {
			t.Errorf("unexpected mutation %s", r.Method)
			w.WriteHeader(405)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/namespaces/kube-system" {
			f.identityReads.Add(1)
			if !f.denyIdentity.Load() {
				fmt.Fprint(w, mapIdentityBody)
				return
			}
		} else if strings.HasSuffix(r.URL.Path, "/applicationsets") {
			fmt.Fprint(w, `{"apiVersion":"argoproj.io/v1alpha1","kind":"ApplicationSetList","items":[]}`)
			return
		} else if strings.HasSuffix(r.URL.Path, "/deployments") && !f.denyInventory.Load() {
			items := `[{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":"team-a","uid":"object-instance","labels":{"app.kubernetes.io/managed-by":"Helm"}}}]`
			if f.emptyInventory.Load() {
				items = "[]"
			}
			fmt.Fprintf(w, `{"apiVersion":"apps/v1","kind":"DeploymentList","items":%s}`, items)
			return
		}
		w.WriteHeader(403)
		fmt.Fprint(w, `{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"Forbidden","code":403}`)
	}))
	t.Cleanup(f.server.Close)
	return f
}

func setupMapIdentityFlags(t *testing.T) {
	t.Helper()
	format, jsonFlag, kind, ns, owner, query := mapListFormat, mapJSON, mapKind, mapNamespace, mapOwner, mapQuery
	summary, count, names, evidence, explain, verbose := mapSummary, mapCount, mapNamesOnly, mapOwnershipEvidence, mapExplain, mapVerbose
	t.Cleanup(func() {
		mapListFormat, mapJSON, mapKind, mapNamespace, mapOwner, mapQuery = format, jsonFlag, kind, ns, owner, query
		mapSummary, mapCount, mapNamesOnly, mapOwnershipEvidence, mapExplain, mapVerbose = summary, count, names, evidence, explain, verbose
	})
	mapListFormat, mapJSON, mapKind, mapNamespace, mapOwner, mapQuery = "json", false, "Deployment", "team-a", "", ""
	mapSummary, mapCount, mapNamesOnly, mapOwnershipEvidence, mapExplain, mapVerbose = false, false, false, false, false, false
	t.Setenv("CUB_SCOUT_OFFLINE", "true")
	t.Setenv("PATH", t.TempDir())
	t.Setenv(customResourceConfigEnvVar, filepath.Join(t.TempDir(), "absent.yaml"))
	t.Setenv("CUB_SCOUT_TEST_MAP_ENTRIES_JSON", "")
}

func TestMapClusterIdentityCLIFormatsAndCapturedSelection(t *testing.T) {
	setupMapIdentityFlags(t)
	other, selected := newMapIdentityFixture(t), newMapIdentityFixture(t)
	path, original := resolverKubeconfig(t, "other", map[string]string{"other": other.server.URL, "selected": selected.server.URL})
	cfg, label, err := resolveClusterConfig("selected", true, resolverRules(path), nil)
	require.NoError(t, err)
	// Retarget the same context name after capture; neither read may follow it.
	raw, err := clientcmd.Load(original)
	require.NoError(t, err)
	raw.Clusters["selected-cluster"].Server = other.server.URL
	changed, err := clientcmd.Write(*raw)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, changed, 0600))
	for _, format := range []string{"json", "ascii", "md"} {
		mapListFormat = format
		before := selected.identityReads.Load()
		output := captureStdout(t, func() {
			require.NoError(t, runMapListFromClusterWithConfigAndIdentity(context.Background(), cfg, nil, false, label, true))
		})
		require.Equal(t, before+1, selected.identityReads.Load())
		require.Zero(t, other.allReads.Load())
		if format == "json" {
			var result mapClusterIdentityOutput
			require.NoError(t, json.Unmarshal([]byte(output), &result))
			require.Equal(t, "map-list-cluster-identity.v1", result.Schema)
			require.Equal(t, "identity-reader", result.ClusterCostScope)
			require.Equal(t, "selected", result.Cluster.Context)
			require.Equal(t, selected.server.URL, result.Cluster.APIServer)
			require.Equal(t, "selected-cluster-instance", result.Cluster.ID)
			require.EqualValues(t, 1, result.Cluster.Cost.RequestsMade)
			require.EqualValues(t, len(mapIdentityBody), result.Cluster.Cost.ResponseBodyBytes)
			require.Len(t, result.Resources, 1)
			require.Equal(t, "complete", result.Collection.Status)
		} else {
			for _, fact := range []string{`"selected"`, `"selected-cluster-instance"`, "Identity read cost: requests=1", "Cost scope: identity reader only", "Inventory collection: complete", "api"} {
				require.Contains(t, output, fact)
			}
		}
	}
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, changed, after)
}

func TestMapClusterIdentityDeniedAndEmptyInventoryStaySeparate(t *testing.T) {
	setupMapIdentityFlags(t)
	f := newMapIdentityFixture(t)
	f.denyIdentity.Store(true)
	read := func() mapClusterIdentityOutput {
		output := captureStdout(t, func() {
			require.NoError(t, runMapListFromClusterWithConfigAndIdentity(context.Background(), &rest.Config{Host: f.server.URL}, nil, false, "selected", true))
		})
		var result mapClusterIdentityOutput
		require.NoError(t, json.Unmarshal([]byte(output), &result))
		return result
	}
	result := read()
	require.Equal(t, "unverified", result.Cluster.Identity)
	require.Equal(t, "forbidden", result.Cluster.Omission)
	require.Empty(t, result.Cluster.ID)
	require.Nil(t, result.Cluster.ObservedAt)
	require.Len(t, result.Resources, 1, "identity denial must retain observed inventory")
	f.emptyInventory.Store(true)
	result = read()
	require.Empty(t, result.Resources)
	require.NotNil(t, result.Resources)
	require.Equal(t, "complete", result.Collection.Status)
	f.denyInventory.Store(true)
	result = read()
	require.Empty(t, result.Resources)
	require.Equal(t, "partial", result.Collection.Status)
	require.NotEmpty(t, result.Collection.Omissions)
	require.Equal(t, "forbidden", result.Cluster.Omission)
}

func TestMapClusterIdentityDefaultBudgetAndMCPAdmission(t *testing.T) {
	setupMapIdentityFlags(t)
	f := newMapIdentityFixture(t)
	output := captureStdout(t, func() {
		require.NoError(t, runMapListFromClusterWithConfig(context.Background(), &rest.Config{Host: f.server.URL}, nil))
	})
	require.Zero(t, f.identityReads.Load())
	require.True(t, strings.HasPrefix(strings.TrimSpace(output), "["), "legacy JSON remains an array")
	var invoked int
	var args []string
	gateway := newMCPGateway(func(_ context.Context, got []string) (string, error) {
		invoked++
		args = got
		return `{"schema":"map-list-cluster-identity.v1","cluster":{"identity":"unverified","omission":"forbidden"},"resources":[],"collection":{"status":"partial","omissions":[{"reason":"forbidden"}]},"clusterCostScope":"identity-reader"}`, nil
	})
	call := func(values map[string]interface{}) map[string]interface{} {
		params, err := json.Marshal(map[string]interface{}{"name": "map", "arguments": values})
		require.NoError(t, err)
		return gateway.callTool(context.Background(), params)
	}
	result := call(map[string]interface{}{"context": "selected", "cluster_identity": true})
	require.Equal(t, false, result["isError"])
	require.Equal(t, []string{"map", "list", "--json", "--cluster-identity", "--kube-context", "selected"}, args)
	data := result["structuredContent"].(map[string]interface{})["data"].(map[string]interface{})
	require.Equal(t, "identity-reader", data["clusterCostScope"])
	require.Equal(t, "forbidden", data["cluster"].(map[string]interface{})["omission"])
	for _, values := range []map[string]interface{}{{"cluster_identity": "true"}, {"cluster_identity": true, "summary": true}, {"cluster_identity": true, "count": true}, {"cluster_identity": true, "names_only": true}, {"cluster_identity": true, "ownership_evidence": true}} {
		before := invoked
		require.Equal(t, true, call(values)["isError"])
		require.Equal(t, before, invoked)
	}
	require.Nil(t, buildMCPStructuredContent("map", output), "legacy MCP map contract remains unchanged")
}

func TestMapClusterIdentityTUIRefreshRetainsOnlyLatestEvidence(t *testing.T) {
	setupMapIdentityFlags(t)
	f := newMapIdentityFixture(t)
	binding := &localClusterBinding{config: &rest.Config{Host: f.server.URL}, context: "selected", explicit: true, observeClusterIdentity: true}
	msg := loadLocalClusterDataWithBinding(binding).(localDataLoadedMsg)
	require.NoError(t, msg.err)
	require.NotNil(t, msg.clusterIdentity)
	require.EqualValues(t, 1, f.identityReads.Load())
	model := LocalClusterModel{clusterBinding: binding, keymap: defaultLocalKeyMap(), width: 120, height: 40, ready: true, panelPane: viewport.New(60, 30)}
	next, _ := model.Update(msg)
	model = next.(LocalClusterModel)
	before := f.allReads.Load()
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("V")})
	require.Nil(t, command)
	model = next.(LocalClusterModel)
	require.Equal(t, viewOwnershipEvidence, model.panelView)
	require.Contains(t, model.getPanelOwnershipEvidence(), `ID: "selected-cluster-instance"`)
	require.Equal(t, before, f.allReads.Load(), "opening evidence view must not read")
	f.denyIdentity.Store(true)
	msg = loadLocalClusterDataWithBinding(binding).(localDataLoadedMsg)
	next, _ = model.Update(msg)
	model = next.(LocalClusterModel)
	require.Equal(t, "unverified", model.clusterIdentity.Identity)
	require.Empty(t, model.clusterIdentity.ID)
	require.Nil(t, model.clusterIdentity.ObservedAt)
	require.Contains(t, model.getPanelOwnershipEvidence(), `Identity omission: "forbidden"`)
	require.Contains(t, model.panelPane.View(), "forbidden", "visible evidence panel must refresh with loaded model")
	require.NotContains(t, model.panelPane.View(), "selected-cluster-instance", "visible panel must not retain a stale verified ID")
	next, _ = model.Update(localDataLoadedMsg{err: fmt.Errorf("configuration unavailable")})
	model = next.(LocalClusterModel)
	require.Equal(t, "unverified", model.clusterIdentity.Identity, "failed refresh must not relabel stale verified identity as current")
	require.Equal(t, "inventory_refresh_unavailable", model.clusterIdentity.Omission)
	require.Empty(t, model.clusterIdentity.ID)
	require.Contains(t, model.panelPane.View(), "inventory_refresh_unavailable")
	// Recovering inventory replaces the error state instead of renewing old data.
	f.denyIdentity.Store(false)
	next, _ = model.Update(loadLocalClusterDataWithBinding(binding))
	model = next.(LocalClusterModel)
	require.Nil(t, model.err)
	require.Equal(t, "verified", model.clusterIdentity.Identity)
}

func TestMapClusterIdentityRefusalsHappenBeforeBinding(t *testing.T) {
	setupMapIdentityFlags(t)
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "absent-kubeconfig"))
	cmd := &cobra.Command{}
	cmd.Flags().Bool("cluster-identity", true, "")
	cmd.Flags().String("kube-context", "", "")
	addRecordedMapFlags(cmd)
	for _, set := range []func(){func() { mapSummary = true }, func() { mapCount = true }, func() { mapNamesOnly = true }, func() { mapOwnershipEvidence = true }, func() { mapListFormat = "unsupported" }} {
		mapSummary, mapCount, mapNamesOnly, mapOwnershipEvidence = false, false, false, false
		mapListFormat = "json"
		set()
		err := runMapList(cmd, nil)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "kubeconfig", "must refuse mode before resolving credentials")
	}
	mapListFormat = "json"
	require.NoError(t, cmd.Flags().Set("recording", "missing-source"))
	require.ErrorContains(t, runMapList(cmd, nil), "recorded or test-hook")
	cmd.Flags().Lookup("recording").Changed = false
	t.Setenv("CUB_SCOUT_TEST_MAP_ENTRIES_JSON", "missing-test-fixture")
	require.ErrorContains(t, runMapList(cmd, nil), "recorded or test-hook")
}

func TestMapClusterIdentityPreReadFailuresPreserveUnavailableEnvelope(t *testing.T) {
	setupMapIdentityFlags(t)
	f := newMapIdentityFixture(t)
	for _, tc := range []struct {
		cfg    *rest.Config
		err    error
		reason string
	}{
		{nil, fmt.Errorf("private-auth-path-sentinel"), "cluster_binding_unavailable"},
		{nil, nil, "cluster_binding_unavailable"},
		{&rest.Config{Host: f.server.URL, TLSClientConfig: rest.TLSClientConfig{CAData: []byte("private-auth-path-sentinel")}}, nil, "inventory_client_unavailable"},
	} {
		for _, format := range []string{"json", "ascii", "md"} {
			mapListFormat = format
			output := captureStdout(t, func() {
				require.NoError(t, runMapListFromClusterWithConfigAndIdentity(context.Background(), tc.cfg, tc.err, false, "requested", true))
			})
			require.NotContains(t, output, "private-auth-path-sentinel")
			require.Contains(t, output, tc.reason)
			if format == "json" {
				var result mapClusterIdentityOutput
				require.NoError(t, json.Unmarshal([]byte(output), &result))
				require.Equal(t, "unavailable", result.Collection.Status)
				require.Equal(t, tc.reason, result.Collection.UnavailableReason)
				require.Equal(t, "unverified", result.Cluster.Identity)
				require.Nil(t, result.Cluster.ObservedAt)
				require.Empty(t, result.Cluster.ID)
				require.NotNil(t, result.Resources)
				require.Empty(t, result.Resources)
				require.Zero(t, result.Cluster.Cost.RequestsMade)
			} else {
				require.Contains(t, output, "Inventory collection: unavailable")
			}
		}
	}
	require.Zero(t, f.allReads.Load(), "pre-read failures must not attempt API requests")
	mapListFormat = "json"
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "missing"))
	cmd := &cobra.Command{}
	cmd.Flags().Bool("cluster-identity", true, "")
	cmd.Flags().String("kube-context", "", "")
	require.NoError(t, cmd.Flags().Set("kube-context", "unknown-selected"))
	output := captureStdout(t, func() { require.NoError(t, runMapList(cmd, nil)) })
	var result mapClusterIdentityOutput
	require.NoError(t, json.Unmarshal([]byte(output), &result))
	require.Equal(t, "unknown-selected", result.Cluster.Context)
	require.Equal(t, "unavailable", result.Collection.Status)
}

func TestMapClusterIdentityHumanTextEscapesControlsAndFenceInjection(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	e := &agent.ClusterIdentityEvidence{Identity: "verified", Context: "context\x1b[31m\u009b", ID: "```\nforged", APIServer: "https://api.example", ObservedAt: &now}
	text := mapClusterIdentityText(e)
	require.NotContains(t, text, "\x1b")
	require.NotContains(t, text, "\u009b")
	require.NotContains(t, text, "\nforged")
	require.NotContains(t, text, "\n```", "quoted values cannot create a Markdown fence line")
}

func TestMapClusterIdentityImplicitTUIUsesCapturedActionGuards(t *testing.T) {
	setupMapIdentityFlags(t)
	fixture := newMapIdentityFixture(t)
	binding := &localClusterBinding{config: &rest.Config{Host: fixture.server.URL}, context: "captured-current", observeClusterIdentity: true}
	model := initialLocalModelWithBinding(ViewOptions{}, binding)
	model.panelMode = false // Saved navigation state must not affect this control.
	require.True(t, model.explicitClusterContext)
	require.Same(t, binding, model.clusterBinding)
	for _, action := range []string{"I", ":"} {
		updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(action)})
		require.Nil(t, cmd)
		result := updated.(LocalClusterModel)
		require.False(t, result.switchToImport)
		require.False(t, result.cmdMode)
		require.Contains(t, result.statusMsg, "--cluster-identity")
	}
	shell := model.runShellOut()().(shellExitMsg)
	require.ErrorContains(t, shell.err, "--cluster-identity")
	graph := model.runGraphExport("svg")().(graphExportMsg)
	require.ErrorContains(t, graph.err, "--cluster-identity")
	require.Zero(t, fixture.allReads.Load())
	binding.observeClusterIdentity = false
	legacy := initialLocalModelWithBinding(ViewOptions{}, binding)
	require.False(t, legacy.explicitClusterContext)
}
