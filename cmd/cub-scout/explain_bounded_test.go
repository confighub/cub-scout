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
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func boundedTestConfig(t *testing.T, host string) string {
	t.Helper()
	config := clientcmdapi.NewConfig()
	config.CurrentContext = "cluster-a"
	config.Clusters["server"] = &clientcmdapi.Cluster{Server: host}
	for _, name := range []string{"cluster-a", "cluster-b", "--refresh"} {
		config.Contexts[name] = &clientcmdapi.Context{Cluster: "server", AuthInfo: name}
		config.AuthInfos[name] = &clientcmdapi.AuthInfo{Token: "synthetic-" + name}
	}
	data, err := clientcmd.Write(*config)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "kubeconfig")
	require.NoError(t, os.WriteFile(path, data, 0600))
	t.Setenv("KUBECONFIG", path)
	return path
}

func TestBoundedExplainMCPBudgetAndContext(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/apis/apps/v1" {
			fmt.Fprint(w, `{"groupVersion":"apps/v1","resources":[{"name":"deployments","kind":"Deployment","namespaced":true,"verbs":["get"]}]}`)
			return
		}
		if r.URL.Path != "/apis/apps/v1/namespaces/team-a/deployments/api" {
			t.Errorf("unexpected read: %s", r.URL)
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":"team-a","generation":2},"spec":{"replicas":1},"status":{"observedGeneration":1}}`)
	}))
	defer server.Close()
	configPath := boundedTestConfig(t, server.URL)
	var fallbackCalls int
	runner := boundedMCPRunner(func(context.Context, []string) (string, error) { fallbackCalls++; return `{}`, nil })
	gateway := newMCPGatewayWithMode(runner, func(context.Context, []string) (string, error) {
		t.Fatal("bounded read must not call ConfigHub")
		return "", nil
	}, true)
	arguments := map[string]interface{}{"bounded": true, "resource": "Deployment/api", "api_version": "apps/v1", "context": "cluster-a", "namespace": "team-a"}
	call := func() ExplainSummary {
		params, err := json.Marshal(map[string]interface{}{"name": "explain", "arguments": arguments})
		require.NoError(t, err)
		result := gateway.callTool(context.Background(), params)
		require.Equal(t, false, result["isError"], "%v", result)
		var summary ExplainSummary
		require.NoError(t, json.Unmarshal([]byte(result["content"].([]map[string]string)[0]["text"]), &summary))
		require.NotNil(t, summary.ResourceRead)
		return summary
	}
	first := call()
	require.Equal(t, "miss", first.ResourceRead.Cache)
	require.NotEqual(t, agent.VerdictPASS, first.CurrentChange.Verdict)
	require.NotEmpty(t, first.NextSteps, "same structured-hint contract as CLI")
	second := call()
	require.Equal(t, "hit", second.ResourceRead.Cache)
	require.Equal(t, first.ResourceRead.ObservedAt, second.ResourceRead.ObservedAt)
	require.EqualValues(t, 2, requests.Load())
	arguments["refresh"] = true
	require.Equal(t, "refresh", call().ResourceRead.Cache)
	require.EqualValues(t, 4, requests.Load())
	delete(arguments, "refresh")
	arguments["context"] = "cluster-b"
	require.Equal(t, "miss", call().ResourceRead.Cache)
	require.EqualValues(t, 6, requests.Load())
	raw, err := clientcmd.LoadFromFile(configPath)
	require.NoError(t, err)
	raw.AuthInfos["cluster-b"].Token = "synthetic-new-credential"
	data, err := clientcmd.Write(*raw)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, data, 0600))
	require.Equal(t, "miss", call().ResourceRead.Cache)
	require.EqualValues(t, 8, requests.Load())
	require.Zero(t, fallbackCalls)
	require.Equal(t, "cluster-a", raw.CurrentContext, "never switch the user's current context")
	delete(arguments, "context")
	params, err := json.Marshal(map[string]interface{}{"name": "explain", "arguments": arguments})
	require.NoError(t, err)
	require.Equal(t, true, gateway.callTool(context.Background(), params)["isError"])
	require.EqualValues(t, 8, requests.Load(), "invalid scope performs no reads")
	arguments["context"] = "--refresh"
	require.Equal(t, "miss", call().ResourceRead.Cache, "flag-like context is a value, not a refresh flag")
	require.Equal(t, "hit", call().ResourceRead.Cache)
	require.EqualValues(t, 10, requests.Load())
}

func TestBoundedExplainMCPMalformedInputsNeverRun(t *testing.T) {
	for _, tc := range []struct {
		key   string
		value interface{}
	}{
		{"bounded", "true"}, {"bounded", nil}, {"refresh", "false"},
		{"api_version", []string{"apps/v1"}}, {"context", 7}, {"namespace", true},
	} {
		t.Run(tc.key+fmt.Sprint(tc.value), func(t *testing.T) {
			gateway := newMCPGatewayWithMode(func(context.Context, []string) (string, error) {
				t.Fatal("malformed input must not fall back to a broad read")
				return "", nil
			}, nil, false)
			arguments := map[string]interface{}{"resource": "Deployment/api", tc.key: tc.value}
			params, err := json.Marshal(map[string]interface{}{"name": "explain", "arguments": arguments})
			require.NoError(t, err)
			require.Equal(t, true, gateway.callTool(context.Background(), params)["isError"])
		})
	}
}

func TestBoundedExplainHintsStayScoped(t *testing.T) {
	ref := agent.BoundedResourceRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "team-a", Name: "api"}
	evidence := agent.BoundedReadEvidence{Context: "cluster'$(touch unwanted)`id`", Resource: ref}
	summary := buildBoundedExplainSummary(nil, evidence, fmt.Errorf("forbidden"))
	for _, mode := range []HintMode{HintModeDefault, HintModeOperator, HintModeBeginner} {
		hints := explainHintsWithContext(summary, HintContext{Mode: mode})
		require.Len(t, hints, 1)
		require.Equal(t, ActionReadOnly, hints[0].ActionType)
		require.Equal(t, "cub-scout explain Deployment/api --bounded --api-version apps/v1 --kube-context 'cluster'\"'\"'$(touch unwanted)`id`' --namespace team-a --refresh", hints[0].Command)
		require.NotContains(t, hints[0].Rationale, "unmanaged")
		require.NotContains(t, hints[0].Command, "orphans")
	}
}

func TestBoundedExplainUnknownAndPayloadBoundary(t *testing.T) {
	ref := agent.BoundedResourceRef{APIVersion: "example.io/v1", Kind: "Deployment", Namespace: "team-a", Name: "api"}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": ref.APIVersion, "kind": ref.Kind,
		"metadata": map[string]interface{}{"name": ref.Name, "namespace": ref.Namespace},
		"data":     map[string]interface{}{"password": "sensitive-fixture-do-not-emit"},
	}}
	evidence := agent.BoundedReadEvidence{Context: "cluster-a", Resource: ref, ObservedAt: time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)}
	summary := buildBoundedExplainSummary(obj, evidence, nil)
	require.Equal(t, "Unknown", summary.Health, "custom kind collisions are not native workload evidence")
	require.Nil(t, summary.CurrentChange)
	require.Nil(t, summary.Events)
	require.Nil(t, summary.ThreeWay)
	require.Nil(t, summary.DeliveryEvidence)
	encoded, err := json.Marshal(summary)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "sensitive-fixture")
	require.Contains(t, summary.Drift, "Not assessed")
	failure := buildBoundedExplainSummary(nil, evidence, fmt.Errorf("forbidden"))
	require.Equal(t, "Unknown", failure.Owner)
	require.Equal(t, "Unavailable", failure.Health)
	require.Contains(t, strings.Join(failure.Notes, " "), "forbidden")
	text := renderExplainText(summary, PresentationHuman, true, HintContext{Mode: HintModeDefault})
	md := renderExplainMarkdown(summary, PresentationHuman, true, HintContext{Mode: HintModeDefault})
	for _, output := range []string{text, md} {
		require.Contains(t, output, "cluster-a")
		require.Contains(t, output, "example.io/v1")
		require.Contains(t, output, "no source/controller")
	}
}

func TestBoundedExplainTUISelectionCancellationAndLateResults(t *testing.T) {
	m := LocalClusterModel{ready: true, width: 100, height: 30, contextName: "cluster-a", boundedContext: "cluster-a", entries: []MapEntry{
		{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "team-b", Name: "api"},
		{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "team-a", Name: "api"},
		{Kind: "Deployment", Namespace: "unknown", Name: "api"},
		{APIVersion: "v1", Kind: "Secret", Namespace: "team-a", Name: "token"},
	}}
	m.openBoundedExplain()
	p := m.boundedPanel
	require.Len(t, p.items, 2)
	require.Equal(t, "team-a", p.items[0].Namespace)
	var captured context.Context
	p.observe = func(ctx context.Context, ref agent.BoundedResourceRef, kubeContext string, refresh bool) (ExplainSummary, error) {
		captured = ctx
		require.Equal(t, "cluster-a", kubeContext)
		return ExplainSummary{Resource: ref.Kind + "/" + ref.Name, Namespace: ref.Namespace, Notes: []string{"selected-object-proof"}}, nil
	}
	cmd := p.read(false)
	msg := cmd().(boundedExplainMsg)
	m.acceptBoundedExplain(msg)
	require.ErrorIs(t, captured.Err(), context.Canceled)
	require.Contains(t, p.viewport.View(), "--kube-context 'cluster-a'")
	require.Contains(t, p.viewport.View(), "--namespace team-a")
	p.stop()
	p.cursor = 1
	newMsg := p.read(true)().(boundedExplainMsg)
	m.acceptBoundedExplain(newMsg)
	m.acceptBoundedExplain(msg)
	require.Contains(t, p.viewport.View(), "--namespace team-b", "late earlier result must be ignored")
	updated, _ := m.boundedExplainKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(LocalClusterModel)
	require.False(t, p.viewing)
	m.acceptBoundedExplain(newMsg)
	require.False(t, p.viewing, "late message must not reopen a closed read")
	updated, _ = m.boundedExplainKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(LocalClusterModel)
	require.Nil(t, m.boundedPanel)
}

func TestBoundedExplainTUINarrowView(t *testing.T) {
	for _, size := range [][2]int{{100, 30}, {40, 12}, {20, 5}} {
		m := LocalClusterModel{ready: true, width: size[0], height: size[1], contextName: "cluster-with-a-long-context-name", boundedContext: "cluster-with-a-long-context-name", entries: []MapEntry{
			{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "team-a", Name: "resource-with-a-long-name"},
		}}
		m.openBoundedExplain()
		m.boundedPanel.setContent("selected object evidence with a long line that must reflow when the terminal becomes narrower")
		m.boundedPanel.viewing = true
		m.boundedPanel.resize(size[0], size[1])
		view := ansi.Strip(m.View())
		lines := strings.Split(view, "\n")
		require.LessOrEqual(t, len(lines), size[1])
		for _, line := range lines {
			require.LessOrEqual(t, ansi.StringWidth(line), size[0])
		}
	}
}

func TestBoundedExplainTUICancelsPendingRead(t *testing.T) {
	m := LocalClusterModel{ready: true, width: 100, height: 30, contextName: "cluster-a", boundedContext: "cluster-a", entries: []MapEntry{
		{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "team-a", Name: "api"},
	}}
	m.openBoundedExplain()
	started := make(chan struct{})
	finished := make(chan boundedExplainMsg, 1)
	m.boundedPanel.observe = func(ctx context.Context, _ agent.BoundedResourceRef, _ string, _ bool) (ExplainSummary, error) {
		close(started)
		<-ctx.Done()
		return ExplainSummary{}, ctx.Err()
	}
	cmd := m.boundedPanel.read(false)
	go func() { finished <- cmd().(boundedExplainMsg) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("read did not start")
	}
	updated, _ := m.boundedExplainKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(LocalClusterModel)
	select {
	case msg := <-finished:
		require.ErrorIs(t, msg.err, context.Canceled)
		m.acceptBoundedExplain(msg)
		require.False(t, m.boundedPanel.viewing)
	case <-time.After(5 * time.Second):
		t.Fatal("closing the view did not cancel the read")
	}
}

func TestBoundedExplainTUIRespectsFilters(t *testing.T) {
	for _, tc := range []struct {
		name      string
		options   ViewOptions
		query     *SavedQuery
		namespace int
		want      int
	}{
		{name: "namespace flag", options: ViewOptions{Namespace: "team-a"}, want: 2},
		{name: "kind flag", options: ViewOptions{Kind: "deployment"}, want: 2},
		{name: "owner flag", options: ViewOptions{Owner: "flux"}, want: 1},
		{name: "namespace navigation", namespace: 2, want: 1},
		{name: "saved query", query: &SavedQuery{Query: "owner=Flux"}, want: 1},
		{name: "conflicting scope", options: ViewOptions{Namespace: "team-a"}, namespace: 2, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := LocalClusterModel{ready: true, boundedContext: "cluster-a", viewOpts: tc.options, activeQuery: tc.query, namespaceIdx: tc.namespace, namespaces: []string{"team-a", "team-b"}, entries: []MapEntry{
				{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "team-a", Name: "api", Owner: "Flux"},
				{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "team-b", Name: "api", Owner: "ArgoCD"},
				{APIVersion: "apps/v1", Kind: "StatefulSet", Namespace: "team-a", Name: "db", Owner: "Helm"},
			}}
			m.openBoundedExplain()
			require.Len(t, m.boundedPanel.items, tc.want)
		})
	}
}

func TestBoundedExplainTUIPinsInventoryContext(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	var requestsA, requestsB atomic.Int32
	server := func(requests *atomic.Int32) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"apiVersion":"v1","kind":"List","items":[]}`)
		}))
	}
	a, b := server(&requestsA), server(&requestsB)
	defer a.Close()
	defer b.Close()
	path := boundedTestConfig(t, a.URL)
	m := LocalClusterModel{contextName: "cluster-a"}
	refresh := m.loadLocalClusterData
	raw, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	raw.CurrentContext = "cluster-b"
	raw.Clusters["other"] = &clientcmdapi.Cluster{Server: b.URL}
	raw.Contexts["cluster-b"].Cluster = "other"
	data, err := clientcmd.Write(*raw)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0600))
	loaded := refresh().(localDataLoadedMsg)
	require.NoError(t, loaded.err)
	require.Equal(t, "cluster-a", loaded.boundedContext)
	require.Positive(t, requestsA.Load())
	require.Zero(t, requestsB.Load(), "refresh must stay in the displayed context")
	updated, _ := m.Update(loaded)
	m = updated.(LocalClusterModel)
	require.Equal(t, "cluster-a", m.boundedContext)
	m.openBoundedExplain()
	require.Equal(t, "cluster-a", m.boundedPanel.context)
	m.boundedContext = ""
	m.openBoundedExplain()
	require.Empty(t, m.boundedPanel.items, "unbound or in-cluster inventory must not infer a kubeconfig context")
	require.Nil(t, m.boundedPanel.read(false))
}

func TestBoundedExplainOwnerFamilies(t *testing.T) {
	for _, tc := range []struct {
		owner       string
		labels      map[string]interface{}
		annotations map[string]interface{}
	}{
		{"Flux", map[string]interface{}{"kustomize.toolkit.fluxcd.io/name": "app", "kustomize.toolkit.fluxcd.io/namespace": "flux-system"}, nil},
		{"ArgoCD", map[string]interface{}{"argocd.argoproj.io/instance": "app"}, nil},
		{"Helm", map[string]interface{}{"app.kubernetes.io/managed-by": "Helm", "app.kubernetes.io/instance": "app"}, nil},
		{"Crossplane", map[string]interface{}{"crossplane.io/claim-name": "app", "crossplane.io/claim-namespace": "team-a"}, nil},
		{"kro", map[string]interface{}{"kro.run/instance-id": "app"}, nil},
		{"ConfigHub", map[string]interface{}{"confighub.com/UnitSlug": "app"}, nil},
		{"Modelplane", map[string]interface{}{"modelplane.ai/deployment": "app"}, nil},
		{"Sveltos", nil, map[string]interface{}{"projectsveltos.io/owner-kind": "ClusterProfile", "projectsveltos.io/owner-name": "app"}},
	} {
		t.Run(tc.owner, func(t *testing.T) {
			obj := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]interface{}{"name": "api", "namespace": "team-a", "labels": tc.labels}}}
			if tc.annotations != nil {
				obj.Object["metadata"].(map[string]interface{})["annotations"] = tc.annotations
			}
			summary := buildBoundedExplainSummary(obj, agent.BoundedReadEvidence{}, nil)
			require.Equal(t, tc.owner, summary.Owner)
			require.Nil(t, summary.DeliveryEvidence)
			require.Len(t, summary.Omissions, 4)
			require.Nil(t, summary.ConfigHubOrigin)
			require.Equal(t, "confighub-origin", summary.Omissions[3].Missing)
			annotations := obj.GetAnnotations()
			if annotations == nil {
				annotations = map[string]string{}
			}
			annotations[agent.ConfigHubOriginAnnotation] = `{"spaceId":"space-a","unitSlug":"app"}`
			obj.SetAnnotations(annotations)
			withOrigin := buildBoundedExplainSummary(obj, agent.BoundedReadEvidence{}, nil)
			require.Equal(t, summary.Owner, withOrigin.Owner)
			require.Equal(t, summary.Health, withOrigin.Health)
			require.NotNil(t, withOrigin.ConfigHubOrigin)
			require.Len(t, withOrigin.Omissions, 3)
		})
	}
}

// Opt-in proof against an existing object. It never creates or mutates fixtures.
func TestBoundedExplainLive(t *testing.T) {
	kubeContext := os.Getenv("CUB_SCOUT_BOUNDED_LIVE_CONTEXT")
	if kubeContext == "" {
		t.Skip("set CUB_SCOUT_BOUNDED_LIVE_CONTEXT and exact resource scope for read-only live smoke")
	}
	resource, apiVersion := os.Getenv("CUB_SCOUT_BOUNDED_LIVE_RESOURCE"), os.Getenv("CUB_SCOUT_BOUNDED_LIVE_APIVERSION")
	namespace := os.Getenv("CUB_SCOUT_BOUNDED_LIVE_NAMESPACE")
	ref, err := boundedExplainRef([]string{resource}, apiVersion, namespace, kubeContext)
	require.NoError(t, err)
	before, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "cub-scout")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, ".")
	output, err := build.CombinedOutput()
	require.NoError(t, err, "%s", output)
	args := []string{"explain", resource, "--bounded", "--api-version", apiVersion, "--kube-context", kubeContext, "--format", "json"}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}
	assertSummary := func(summary ExplainSummary, cache string) {
		t.Helper()
		require.NotNil(t, summary.ResourceRead)
		require.True(t, summary.ResourceRead.Available, "%v", summary.Notes)
		require.Equal(t, ref, summary.ResourceRead.Resource)
		require.Equal(t, kubeContext, summary.ResourceRead.Context)
		require.Equal(t, cache, summary.ResourceRead.Cache)
		reads := agent.BoundedReadCounts{Discovery: 1, Object: 1}
		if cache == "hit" {
			reads = agent.BoundedReadCounts{}
		}
		require.Equal(t, reads, summary.ResourceRead.Reads)
		missing := []string{}
		for _, omission := range summary.Omissions {
			missing = append(missing, omission.Missing)
		}
		expectedMissing := []string{"source-controller-evidence", "related-pod-event-evidence", "desired-live-comparison"}
		if summary.ConfigHubOrigin == nil {
			expectedMissing = append(expectedMissing, "confighub-origin")
		}
		require.ElementsMatch(t, expectedMissing, missing)
		require.Nil(t, summary.DeliveryEvidence)
		require.Nil(t, summary.Events)
		require.Nil(t, summary.ThreeWay)
	}
	for _, plugin := range []string{"", "1"} {
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Env = append(os.Environ(), "CUB_PLUGIN="+plugin)
		output, err := cmd.Output()
		require.NoError(t, err)
		var summary ExplainSummary
		require.NoError(t, json.Unmarshal(output, &summary))
		assertSummary(summary, "miss")
		prefix := "cub-scout explain"
		if plugin != "" {
			prefix = "cub scout explain"
		}
		require.True(t, strings.HasPrefix(summary.NextSteps[0].NextCommand, prefix))
	}
	if _, err := exec.LookPath("cub"); err == nil {
		configDir := t.TempDir()
		pluginDir := filepath.Join(configDir, "plugins", "scout")
		require.NoError(t, os.MkdirAll(pluginDir, 0700))
		require.NoError(t, os.Link(binary, filepath.Join(pluginDir, "main")))
		cmd := exec.CommandContext(ctx, "cub", append([]string{"scout"}, args...)...)
		cmd.Env = append(os.Environ(), "CUB_CONFIG="+configDir, "CUB_CONTEXT=", "CUB_TOKEN=", "CUB_SPACE=")
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", output)
		var summary ExplainSummary
		require.NoError(t, json.Unmarshal(output, &summary))
		assertSummary(summary, "miss")
		t.Log("actual cub host passed with isolated plugin/config directory")
	} else {
		t.Log("cub not installed; actual host check skipped (plugin-mode binary check passed)")
	}
	server := exec.CommandContext(ctx, binary, "mcp", "serve")
	stdin, err := server.StdinPipe()
	require.NoError(t, err)
	stdout, err := server.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, server.Start())
	defer func() {
		_ = stdin.Close()
		_ = server.Process.Kill()
		_ = server.Wait()
	}()
	reader := bufio.NewReader(stdout)
	var id int
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
		require.Nil(t, response["error"], "%v", response)
		return response["result"].(map[string]interface{})
	}
	request("initialize", map[string]interface{}{"protocolVersion": "2025-03-26", "capabilities": map[string]interface{}{}, "clientInfo": map[string]string{"name": "bounded-live-smoke", "version": "1"}})
	arguments := map[string]interface{}{"resource": resource, "namespace": namespace, "bounded": true, "api_version": apiVersion, "context": kubeContext}
	var observed time.Time
	for _, cache := range []string{"miss", "hit", "refresh"} {
		arguments["refresh"] = cache == "refresh"
		result := request("tools/call", map[string]interface{}{"name": "explain", "arguments": arguments})
		require.Equal(t, false, result["isError"], "%v", result)
		var summary ExplainSummary
		content := result["content"].([]interface{})[0].(map[string]interface{})["text"].(string)
		require.NoError(t, json.Unmarshal([]byte(content), &summary))
		assertSummary(summary, cache)
		if cache == "hit" {
			require.Equal(t, observed, summary.ResourceRead.ObservedAt)
		} else {
			observed = summary.ResourceRead.ObservedAt
		}
		t.Logf("MCP %s: discovery=%d object=%d observed=%s", cache, summary.ResourceRead.Reads.Discovery, summary.ResourceRead.Reads.Object, summary.ResourceRead.ObservedAt.Format(time.RFC3339Nano))
	}
	after, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
	require.NoError(t, err)
	require.Equal(t, before.CurrentContext, after.CurrentContext)
}
