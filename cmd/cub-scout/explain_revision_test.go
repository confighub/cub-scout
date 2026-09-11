// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func revisionTestServer(t *testing.T, fixture string) (string, *atomic.Int32, *atomic.Bool) {
	t.Helper()
	data, err := os.ReadFile("../../examples/controller-revision/" + fixture + ".json")
	require.NoError(t, err)
	obj := &unstructured.Unstructured{}
	require.NoError(t, obj.UnmarshalJSON(data))
	if fixture == "application" {
		require.NoError(t, unstructured.SetNestedField(obj.Object, time.Now().Add(-time.Minute).UTC().Format(time.RFC3339), "status", "reconciledAt"))
	}
	resource := fixture + "s"
	var requests atomic.Int32
	var denied atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet || r.URL.RawQuery != "" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			http.Error(w, "forbidden", 403)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		base := "/apis/" + obj.GetAPIVersion()
		if r.URL.Path == base {
			fmt.Fprintf(w, `{"groupVersion":%q,"resources":[{"name":%q,"kind":%q,"namespaced":true,"verbs":["get"]}]}`, obj.GetAPIVersion(), resource, obj.GetKind())
			return
		}
		if denied.Load() {
			http.Error(w, `{"kind":"Status","status":"Failure","reason":"Forbidden","code":403}`, 403)
			return
		}
		for _, ns := range []string{"delivery", "other"} {
			if r.URL.Path == base+"/namespaces/"+ns+"/"+resource+"/api" {
				copy := obj.DeepCopy()
				copy.SetNamespace(ns)
				require.NoError(t, json.NewEncoder(w).Encode(copy.Object))
				return
			}
		}
		t.Errorf("unexpected scope: %s", r.URL)
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	return server.URL, &requests, &denied
}

func TestBoundedRevisionMCP(t *testing.T) {
	for _, fixture := range []string{"application", "kustomization"} {
		t.Run(fixture, func(t *testing.T) {
			host, requests, denied := revisionTestServer(t, fixture)
			boundedTestConfig(t, host)
			runner := boundedMCPRunner(func(context.Context, []string) (string, error) { t.Fatal("unexpected broad reader"); return "", nil })
			gateway := newMCPGatewayWithMode(runner, func(context.Context, []string) (string, error) { t.Fatal("unexpected connected read"); return "", nil }, true)
			apiVersion, kind := "argoproj.io/v1alpha1", "Application"
			if fixture == "kustomization" {
				apiVersion, kind = "kustomize.toolkit.fluxcd.io/v1", "Kustomization"
			}
			args := map[string]interface{}{"bounded": true, "resource": kind + "/api", "api_version": apiVersion, "context": "cluster-a", "namespace": "delivery", "expected_revision": strings.Repeat("a", 40)}
			call := func() ExplainSummary {
				params, err := json.Marshal(map[string]interface{}{"name": "explain", "arguments": args})
				require.NoError(t, err)
				result := gateway.callTool(context.Background(), params)
				require.Equal(t, false, result["isError"], "%v", result)
				var summary ExplainSummary
				require.NoError(t, json.Unmarshal([]byte(result["content"].([]map[string]string)[0]["text"]), &summary))
				require.NotNil(t, summary.ControllerRevision)
				return summary
			}
			first := call()
			require.Equal(t, "match", first.ControllerRevision.Comparison)
			require.EqualValues(t, 2, requests.Load())
			require.Nil(t, first.DeliveryEvidence)
			require.Nil(t, first.CurrentChange, "controller revision is not workload convergence")
			require.Contains(t, first.NextSteps[0].NextCommand, "--expected-revision "+strings.Repeat("a", 40))
			args["expected_revision"] = strings.Repeat("b", 40)
			second := call()
			require.Equal(t, "mismatch", second.ControllerRevision.Comparison, "changing the expectation recomputes from the same object, not cached verdicts")
			require.Equal(t, "hit", second.ResourceRead.Cache)
			require.Equal(t, first.ResourceRead.ObservedAt, second.ResourceRead.ObservedAt)
			require.EqualValues(t, 2, requests.Load())
			for _, output := range []string{renderExplainText(second, PresentationHuman, true, HintContext{}), renderExplainMarkdown(second, PresentationHuman, true, HintContext{})} {
				require.Contains(t, output, "Controller revision")
				require.Contains(t, output, "mismatch")
				require.Contains(t, output, "not workload convergence")
			}
			args["namespace"] = "other"
			require.Equal(t, "other", call().ResourceRead.Resource.Namespace)
			require.EqualValues(t, 4, requests.Load())
			args["context"] = "cluster-b"
			require.Equal(t, "cluster-b", call().ResourceRead.Context)
			require.EqualValues(t, 6, requests.Load())
			denied.Store(true)
			args["refresh"] = true
			failed := call()
			require.False(t, failed.ResourceRead.Available)
			require.Equal(t, "unknown", failed.ControllerRevision.Comparison)
			require.Empty(t, failed.ControllerRevision.ReportedRevision)
			require.EqualValues(t, 8, requests.Load())
			delete(args, "refresh")
			require.Equal(t, "unknown", call().ControllerRevision.Comparison, "failed refresh cannot serve stale evidence")
			require.EqualValues(t, 10, requests.Load())
			for _, invalid := range []interface{}{"", "main", true, nil, "aaaaaaa", "$(touch never)"} {
				args["expected_revision"] = invalid
				params, err := json.Marshal(map[string]interface{}{"name": "explain", "arguments": args})
				require.NoError(t, err)
				require.Equal(t, true, gateway.callTool(context.Background(), params)["isError"])
			}
			require.EqualValues(t, 10, requests.Load(), "invalid expectations never read")
			args["expected_revision"], args["bounded"] = strings.Repeat("a", 40), false
			params, err := json.Marshal(map[string]interface{}{"name": "explain", "arguments": args})
			require.NoError(t, err)
			require.Equal(t, true, gateway.callTool(context.Background(), params)["isError"], "an expected revision must not trigger unbounded fallback")
			require.EqualValues(t, 10, requests.Load())
		})
	}
}

func TestBoundedRevisionTUI(t *testing.T) {
	host, requests, _ := revisionTestServer(t, "application")
	boundedTestConfig(t, host)
	m := LocalClusterModel{ready: true, width: 100, height: 30, boundedContext: "cluster-a", entries: []MapEntry{{APIVersion: "argoproj.io/v1alpha1", Kind: "Application", Namespace: "delivery", Name: "api"}}}
	m.openBoundedExplain()
	p := m.boundedPanel
	m.acceptBoundedExplain(p.read(false)().(boundedExplainMsg))
	key := func(msg tea.KeyMsg) tea.Cmd {
		updated, cmd := m.boundedExplainKey(msg)
		m = updated.(LocalClusterModel)
		return cmd
	}
	key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	require.True(t, p.editingRevision)
	p.revisionInput.SetValue("main")
	require.Nil(t, key(tea.KeyMsg{Type: tea.KeyEnter}))
	require.NotEmpty(t, p.revisionError)
	p.revisionInput.SetValue("sha256:" + strings.Repeat("a", 64) + "x")
	require.Nil(t, key(tea.KeyMsg{Type: tea.KeyEnter}), "must not truncate an invalid pasted identifier into a valid digest")
	require.True(t, p.editingRevision)
	require.EqualValues(t, 2, requests.Load())
	for _, size := range [][2]int{{100, 30}, {40, 12}, {20, 5}} {
		p.resize(size[0], size[1])
		lines := strings.Split(ansi.Strip(p.view()), "\n")
		require.LessOrEqual(t, len(lines), size[1])
		for _, line := range lines {
			require.LessOrEqual(t, ansi.StringWidth(line), size[0])
		}
	}
	p.resize(100, 30)
	p.revisionInput.SetValue(strings.Repeat("a", 40))
	msg := key(tea.KeyMsg{Type: tea.KeyEnter})().(boundedExplainMsg)
	m.acceptBoundedExplain(msg)
	require.Contains(t, p.content, "--expected-revision "+strings.Repeat("a", 40))
	require.Contains(t, p.content, "match: expected=")
	require.EqualValues(t, 2, requests.Load(), "TUI comparison reuses the dated observation")
	key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	p.revisionInput.SetValue(strings.Repeat("b", 40))
	latest := key(tea.KeyMsg{Type: tea.KeyEnter})().(boundedExplainMsg)
	m.acceptBoundedExplain(latest)
	m.acceptBoundedExplain(msg)
	require.Contains(t, p.content, "mismatch: expected=")
	key(tea.KeyMsg{Type: tea.KeyEsc})
	require.Empty(t, p.expected, "expectation is not carried to another selected controller")
}

func TestBoundedRevisionCLIPlugin(t *testing.T) {
	if testing.Short() {
		t.Skip("builds actual CLI for process-boundary proof")
	}
	host, requests, _ := revisionTestServer(t, "application")
	boundedTestConfig(t, host)
	t.Setenv("CUB_SCOUT_OFFLINE", "true")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "cub-scout")
	out, err := exec.CommandContext(ctx, "go", "build", "-o", binary, ".").CombinedOutput()
	require.NoError(t, err, "%s", out)
	base := []string{"explain", "Application/api", "-n", "delivery", "--bounded", "--api-version", "argoproj.io/v1alpha1", "--kube-context", "cluster-a", "--expected-revision", strings.Repeat("a", 40)}
	for _, plugin := range []string{"", "1"} {
		for _, format := range []string{"json", "ascii", "md"} {
			cmd := exec.CommandContext(ctx, binary, append(base, "--format", format)...)
			cmd.Env = append(os.Environ(), "CUB_PLUGIN="+plugin)
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", out)
			if format == "json" {
				var summary ExplainSummary
				require.NoError(t, json.Unmarshal(out, &summary))
				require.Equal(t, "match", summary.ControllerRevision.Comparison)
				require.Equal(t, agent.BoundedReadCounts{Discovery: 1, Object: 1}, summary.ResourceRead.Reads)
				prefix := "cub-scout explain"
				if plugin != "" {
					prefix = "cub scout explain"
				}
				require.True(t, strings.HasPrefix(summary.NextSteps[0].NextCommand, prefix))
			} else {
				require.Contains(t, string(out), "Controller revision")
				require.Contains(t, string(out), "match: expected=")
			}
		}
	}
	require.EqualValues(t, 12, requests.Load())
	for _, args := range [][]string{
		{"explain", "Application/api", "--expected-revision", strings.Repeat("a", 40)},
		append(append([]string{}, base...), "--expected-revision", "main"),
		append(append([]string{}, base...), "--expected-revision", ""),
	} {
		cmd := exec.CommandContext(ctx, binary, args...)
		out, err := cmd.Output()
		require.Error(t, err)
		require.Empty(t, out)
	}
	require.EqualValues(t, 12, requests.Load())
	if _, err := exec.LookPath("cub"); err == nil {
		configDir := t.TempDir()
		pluginDir := filepath.Join(configDir, "plugins", "scout")
		require.NoError(t, os.MkdirAll(pluginDir, 0700))
		require.NoError(t, os.Link(binary, filepath.Join(pluginDir, "main")))
		args := append([]string{"scout"}, base...)
		cmd := exec.CommandContext(ctx, "cub", append(args, "--format", "json")...)
		cmd.Env = append(os.Environ(), "CUB_CONFIG="+configDir, "CUB_CONTEXT=", "CUB_TOKEN=", "CUB_SPACE=")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", out)
		var summary ExplainSummary
		require.NoError(t, json.Unmarshal(out, &summary))
		require.Equal(t, "match", summary.ControllerRevision.Comparison)
		t.Log("actual cub host passed using an isolated plugin/config directory")
	} else {
		t.Log("cub host unavailable; standalone and plugin-mode process checks passed")
	}
	server := exec.CommandContext(ctx, binary, "mcp", "serve")
	stdin, err := server.StdinPipe()
	require.NoError(t, err)
	stdout, err := server.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, server.Start())
	defer func() { _ = stdin.Close(); _ = server.Process.Kill(); _ = server.Wait() }()
	reader := bufio.NewReader(stdout)
	id := 0
	request := func(method string, params interface{}) map[string]interface{} {
		t.Helper()
		id++
		payload, err := json.Marshal(map[string]interface{}{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
		require.NoError(t, err)
		_, err = fmt.Fprintf(stdin, "Content-Length: %d\r\n\r\n%s", len(payload), payload)
		require.NoError(t, err)
		data, err := readMCPFrame(reader)
		require.NoError(t, err)
		var response map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &response))
		require.Nil(t, response["error"])
		return response["result"].(map[string]interface{})
	}
	request("initialize", map[string]interface{}{"protocolVersion": "2025-03-26", "capabilities": map[string]interface{}{}, "clientInfo": map[string]string{"name": "revision-fixture", "version": "1"}})
	arguments := map[string]interface{}{"resource": "Application/api", "namespace": "delivery", "bounded": true, "api_version": "argoproj.io/v1alpha1", "context": "cluster-a", "expected_revision": strings.Repeat("a", 40)}
	before := requests.Load()
	for _, expectedCache := range []string{"miss", "hit", "refresh"} {
		arguments["refresh"] = expectedCache == "refresh"
		result := request("tools/call", map[string]interface{}{"name": "explain", "arguments": arguments})
		require.Equal(t, false, result["isError"], "%v", result)
		var summary ExplainSummary
		require.NoError(t, json.Unmarshal([]byte(result["content"].([]interface{})[0].(map[string]interface{})["text"].(string)), &summary))
		require.Equal(t, "match", summary.ControllerRevision.Comparison)
		require.Equal(t, expectedCache, summary.ResourceRead.Cache)
	}
	require.EqualValues(t, 4, requests.Load()-before, "stdio cold/hit/refresh must use 2/0/2 requests")
}
