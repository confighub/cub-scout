// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/confighub/cub-scout/internal/releasetest"
	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type releaseFixture struct {
	options          releaseCheckOptions
	mu               sync.Mutex
	objects          map[string]*unstructured.Unstructured
	requests         int
	gets             map[string]int
	change           func(*unstructured.Unstructured, int) int
	host             string
	missingDiscovery string
}

func newReleaseFixture(t *testing.T, backend string, input ...[]byte) *releaseFixture {
	t.Helper()
	data, err := os.ReadFile("../../examples/oci-release-check/objects.yaml")
	require.NoError(t, err)
	if len(input) > 0 {
		data = input[0]
	}
	layout := t.TempDir()
	ref, err := releasetest.WriteLayout(layout, map[string][]byte{"objects.yaml": data})
	require.NoError(t, err)
	bundle, err := agent.LoadReleaseBundle(context.Background(), ref, layout, 100)
	require.NoError(t, err)
	f := &releaseFixture{objects: map[string]*unstructured.Unstructured{}, gets: map[string]int{}, options: releaseCheckOptions{Bundle: ref, Layout: layout, Controller: "Application/api", APIVersion: "argoproj.io/v1alpha1", ControllerNamespace: "delivery", Context: "cluster-a", MaxObjects: 100}}
	add := func(o *unstructured.Unstructured) {
		o.SetUID(types.UID("fixture-" + o.GetKind()))
		o.SetResourceVersion("1")
		o.SetGeneration(2)
		f.objects[o.GetKind()+"/"+o.GetName()] = o
	}
	for _, desired := range bundle.Objects {
		o := desired.DeepCopy()
		if o.GetKind() == "Deployment" {
			require.NoError(t, unstructured.SetNestedMap(o.Object, map[string]interface{}{"observedGeneration": int64(2), "replicas": int64(2), "updatedReplicas": int64(2), "readyReplicas": int64(2), "availableReplicas": int64(2), "conditions": []interface{}{map[string]interface{}{"type": "Available", "status": "True"}, map[string]interface{}{"type": "Progressing", "status": "True", "reason": "NewReplicaSetAvailable"}}}, "status"))
		}
		add(o)
	}
	c := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": f.options.APIVersion, "kind": "Application", "metadata": map[string]interface{}{"name": "api", "namespace": "delivery"}}}
	source := map[string]interface{}{"repoURL": "oci://example.invalid/config", "targetRevision": bundle.Evidence.Digest, "path": "."}
	dest := map[string]interface{}{"server": "https://kubernetes.default.svc", "namespace": "delivery"}
	require.NoError(t, unstructured.SetNestedMap(c.Object, map[string]interface{}{"source": source, "destination": dest}, "spec"))
	resources := []interface{}{}
	for _, o := range bundle.Objects {
		gv, _ := schema.ParseGroupVersion(o.GetAPIVersion())
		resources = append(resources, map[string]interface{}{"group": gv.Group, "kind": o.GetKind(), "namespace": o.GetNamespace(), "name": o.GetName()})
	}
	require.NoError(t, unstructured.SetNestedMap(c.Object, map[string]interface{}{"sync": map[string]interface{}{"status": "Synced", "revision": bundle.Evidence.Digest, "comparedTo": map[string]interface{}{"source": source, "destination": dest}}, "reconciledAt": time.Now().Add(-time.Minute).UTC().Format(time.RFC3339), "resources": resources}, "status"))
	if backend == "flux" {
		f.options.Controller = "Kustomization/api"
		f.options.APIVersion = "kustomize.toolkit.fluxcd.io/v1"
		c.SetAPIVersion(f.options.APIVersion)
		c.SetKind("Kustomization")
		require.NoError(t, unstructured.SetNestedMap(c.Object, map[string]interface{}{"path": "./", "sourceRef": map[string]interface{}{"kind": "OCIRepository", "name": "config"}}, "spec"))
		entries := []interface{}{}
		for _, o := range bundle.Objects {
			gv, _ := schema.ParseGroupVersion(o.GetAPIVersion())
			entries = append(entries, map[string]interface{}{"id": o.GetNamespace() + "_" + o.GetName() + "_" + gv.Group + "_" + o.GetKind(), "v": gv.Version})
		}
		require.NoError(t, unstructured.SetNestedMap(c.Object, map[string]interface{}{"observedGeneration": int64(2), "lastAppliedRevision": "v1@" + bundle.Evidence.Digest, "lastAttemptedRevision": "v1@" + bundle.Evidence.Digest, "conditions": []interface{}{map[string]interface{}{"type": "Ready", "status": "True", "observedGeneration": int64(2)}}, "inventory": map[string]interface{}{"entries": entries}}, "status"))
		s := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "source.toolkit.fluxcd.io/v1", "kind": "OCIRepository", "metadata": map[string]interface{}{"name": "config", "namespace": "delivery"}, "spec": map[string]interface{}{"url": "oci://example.invalid/config"}, "status": map[string]interface{}{"observedGeneration": int64(2)}}}
		require.NoError(t, unstructured.SetNestedSlice(s.Object, []interface{}{map[string]interface{}{"type": "Ready", "status": "True", "observedGeneration": int64(2)}}, "status", "conditions"))
		require.NoError(t, unstructured.SetNestedField(s.Object, "v1@"+bundle.Evidence.Digest, "status", "artifact", "revision"))
		add(s)
	}
	add(c)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.requests++
		if r.Method != "GET" || r.URL.RawQuery != "" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			http.Error(w, "forbidden", 403)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		for _, o := range f.objects {
			base := "/apis/" + o.GetAPIVersion()
			if o.GetAPIVersion() == "v1" {
				base = "/api/v1"
			}
			resource := strings.ToLower(o.GetKind()) + "s"
			if o.GetKind() == "OCIRepository" {
				resource = "ocirepositories"
			}
			if r.URL.Path == base {
				if f.missingDiscovery == o.GetAPIVersion() {
					w.WriteHeader(http.StatusNotFound)
					fmt.Fprint(w, `{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"NotFound","code":404}`)
					return
				}
				items := []interface{}{}
				for _, peer := range f.objects {
					if peer.GetAPIVersion() == o.GetAPIVersion() {
						plural := strings.ToLower(peer.GetKind()) + "s"
						if peer.GetKind() == "OCIRepository" {
							plural = "ocirepositories"
						}
						items = append(items, map[string]interface{}{"name": plural, "kind": peer.GetKind(), "namespaced": true, "verbs": []string{"get"}})
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"groupVersion": o.GetAPIVersion(), "resources": items})
				return
			}
			if r.URL.Path == base+"/namespaces/"+o.GetNamespace()+"/"+resource+"/"+o.GetName() {
				key := o.GetKind() + "/" + o.GetName()
				f.gets[key]++
				copy := o.DeepCopy()
				if f.change != nil {
					if code := f.change(copy, f.gets[key]); code != 0 {
						reason := "Forbidden"
						if code == 404 {
							reason = "NotFound"
						}
						w.WriteHeader(code)
						fmt.Fprintf(w, `{"apiVersion":"v1","kind":"Status","status":"Failure","reason":%q,"code":%d}`, reason, code)
						return
					}
				}
				_ = json.NewEncoder(w).Encode(copy.Object)
				return
			}
		}
		t.Errorf("unexpected path %s", r.URL)
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	f.host = server.URL
	boundedTestConfig(t, server.URL)
	return f
}

func TestReleaseExampleGenerator(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "layout")
	args := []string{"run", "../../examples/oci-release-check/create-layout", "../../examples/oci-release-check/objects.yaml", dir}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	output, err := exec.CommandContext(ctx, "go", args...).CombinedOutput()
	require.NoError(t, err, "%s", output)
	bundle, err := agent.LoadReleaseBundle(ctx, strings.TrimSpace(string(output)), dir, 100)
	require.NoError(t, err)
	require.True(t, bundle.Evidence.Verified)
	require.Equal(t, 2, bundle.Evidence.Objects)
	output, err = exec.CommandContext(ctx, "go", args...).CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(output), "must not exist")
}

func TestReleaseCheckEvidence(t *testing.T) {
	for _, backend := range []string{"argo", "flux"} {
		t.Run(backend, func(t *testing.T) {
			f := newReleaseFixture(t, backend)
			report, err := observeReleaseCheck(context.Background(), f.options)
			require.NoError(t, err)
			require.Equal(t, agent.VerdictPASS, report.Verdict, "%+v", report.Stages)
			require.NotNil(t, report.Configuration)
			require.NotNil(t, report.Convergence)
			require.NoError(t, agent.VerifyStatementFingerprint(*report.Configuration))
			require.NoError(t, agent.VerifyStatementFingerprint(*report.Convergence))
			want := 8
			if backend == "flux" {
				want = 12
			}
			f.mu.Lock()
			requests, deploymentReads := f.requests, f.gets["Deployment/api"]
			f.mu.Unlock()
			require.Equal(t, want, requests)
			require.Equal(t, want, report.RequestCounts.Discovery+report.RequestCounts.Object)
			require.Equal(t, 1, deploymentReads, "configuration and convergence must share the same live read")
			require.Contains(t, renderReleaseCheck(report, "ascii"), "authored configuration matches")
			require.Contains(t, renderReleaseCheck(report, "md"), "## Configuration Release Check")
		})
	}
}

func TestReleaseCheckNegativeEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, backend, verdict string
		change                 func(*unstructured.Unstructured, int) int
	}{
		{"config only drift", "argo", "BLOCK", func(o *unstructured.Unstructured, _ int) int {
			if o.GetKind() == "ConfigMap" {
				_ = unstructured.SetNestedField(o.Object, "debug", "data", "LOG_LEVEL")
			}
			return 0
		}},
		{"absent object", "argo", "BLOCK", func(o *unstructured.Unstructured, _ int) int {
			if o.GetKind() == "ConfigMap" {
				return 404
			}
			return 0
		}},
		{"denied object", "argo", "INCONCLUSIVE", func(o *unstructured.Unstructured, _ int) int {
			if o.GetKind() == "ConfigMap" {
				return 403
			}
			return 0
		}},
		{"missing live identity", "argo", "INCONCLUSIVE", func(o *unstructured.Unstructured, _ int) int {
			if o.GetKind() == "ConfigMap" {
				o.SetUID("")
			}
			return 0
		}},
		{"deleting live object", "argo", "INCONCLUSIVE", func(o *unstructured.Unstructured, _ int) int {
			if o.GetKind() == "ConfigMap" {
				_ = unstructured.SetNestedField(o.Object, "2026-09-11T00:00:00Z", "metadata", "deletionTimestamp")
			}
			return 0
		}},
		{"malformed live deletion metadata", "argo", "INCONCLUSIVE", func(o *unstructured.Unstructured, _ int) int {
			if o.GetKind() == "ConfigMap" {
				_ = unstructured.SetNestedField(o.Object, "invalid", "metadata", "deletionTimestamp")
			}
			return 0
		}},
		{"missing generation", "argo", "INCONCLUSIVE", func(o *unstructured.Unstructured, _ int) int {
			if o.GetKind() == "Deployment" {
				unstructured.RemoveNestedField(o.Object, "status", "observedGeneration")
			}
			return 0
		}},
		{"stale workload", "argo", "WATCH", func(o *unstructured.Unstructured, _ int) int {
			if o.GetKind() == "Deployment" {
				_ = unstructured.SetNestedField(o.Object, int64(1), "status", "observedGeneration")
			}
			return 0
		}},
		{"progressing", "argo", "WATCH", func(o *unstructured.Unstructured, _ int) int {
			if o.GetKind() == "Deployment" {
				_ = unstructured.SetNestedField(o.Object, int64(1), "status", "updatedReplicas")
			}
			return 0
		}},
		{"recreated controller", "argo", "INCONCLUSIVE", func(o *unstructured.Unstructured, n int) int {
			if o.GetKind() == "Application" && n > 1 {
				o.SetUID("new-controller")
			}
			return 0
		}},
		{"refresh denied", "argo", "INCONCLUSIVE", func(o *unstructured.Unstructured, n int) int {
			if o.GetKind() == "Application" && n > 1 {
				return 403
			}
			return 0
		}},
		{"controller revision behind", "argo", "WATCH", func(o *unstructured.Unstructured, _ int) int {
			if o.GetKind() == "Application" {
				_ = unstructured.SetNestedField(o.Object, "sha256:"+strings.Repeat("b", 64), "status", "sync", "revision")
			}
			return 0
		}},
		{"source mismatch", "argo", "INCONCLUSIVE", func(o *unstructured.Unstructured, _ int) int {
			if o.GetKind() == "Application" {
				for _, p := range [][]string{{"spec", "source", "repoURL"}, {"status", "sync", "comparedTo", "source", "repoURL"}} {
					_ = unstructured.SetNestedField(o.Object, "oci://other.invalid/config", p...)
				}
			}
			return 0
		}},
		{"named destination", "argo", "INCONCLUSIVE", func(o *unstructured.Unstructured, _ int) int {
			if o.GetKind() == "Application" {
				_ = unstructured.SetNestedField(o.Object, "prod", "spec", "destination", "name")
			}
			return 0
		}},
		{"wrong inventory", "argo", "INCONCLUSIVE", func(o *unstructured.Unstructured, _ int) int {
			if o.GetKind() == "Application" {
				_ = unstructured.SetNestedSlice(o.Object, []interface{}{}, "status", "resources")
			}
			return 0
		}},
		{"source unavailable", "flux", "INCONCLUSIVE", func(o *unstructured.Unstructured, _ int) int {
			if o.GetKind() == "OCIRepository" {
				return 403
			}
			return 0
		}},
		{"source repository wrong", "flux", "INCONCLUSIVE", func(o *unstructured.Unstructured, _ int) int {
			if o.GetKind() == "OCIRepository" {
				_ = unstructured.SetNestedField(o.Object, "oci://other.invalid/config", "spec", "url")
			}
			return 0
		}},
		{"source ignore rules", "flux", "INCONCLUSIVE", func(o *unstructured.Unstructured, _ int) int {
			if o.GetKind() == "OCIRepository" {
				_ = unstructured.SetNestedField(o.Object, "objects.yaml", "spec", "ignore")
			}
			return 0
		}},
		{"stale source generation", "flux", "INCONCLUSIVE", func(o *unstructured.Unstructured, _ int) int {
			if o.GetKind() == "OCIRepository" {
				_ = unstructured.SetNestedField(o.Object, int64(1), "status", "observedGeneration")
			}
			return 0
		}},
		{"source artifact differs", "flux", "INCONCLUSIVE", func(o *unstructured.Unstructured, _ int) int {
			if o.GetKind() == "OCIRepository" {
				_ = unstructured.SetNestedField(o.Object, "sha256:"+strings.Repeat("b", 64), "status", "artifact", "revision")
			}
			return 0
		}},
		{"source changes", "flux", "INCONCLUSIVE", func(o *unstructured.Unstructured, n int) int {
			if o.GetKind() == "OCIRepository" && n > 1 {
				o.SetResourceVersion("2")
			}
			return 0
		}},
		{"remote kubeconfig", "flux", "INCONCLUSIVE", func(o *unstructured.Unstructured, _ int) int {
			if o.GetKind() == "Kustomization" {
				_ = unstructured.SetNestedMap(o.Object, map[string]interface{}{"secretRef": map[string]interface{}{"name": "other"}}, "spec", "kubeConfig")
			}
			return 0
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newReleaseFixture(t, tc.backend)
			f.change = tc.change
			r, err := observeReleaseCheck(context.Background(), f.options)
			require.NoError(t, err)
			require.Equal(t, agent.ReceiptVerdict(tc.verdict), r.Verdict, "%+v", r.Stages)
			require.NotEmpty(t, r.NextStep)
		})
	}
}

func TestReleaseCheckCLIAndMCP(t *testing.T) {
	if testing.Short() {
		t.Skip("builds actual CLI and MCP processes")
	}
	f := newReleaseFixture(t, "argo")
	t.Setenv("CUB_SCOUT_OFFLINE", "true")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "cub-scout")
	out, err := exec.CommandContext(ctx, "go", "build", "-o", binary, ".").CombinedOutput()
	require.NoError(t, err, "%s", out)
	a := map[string]interface{}{"bundle": f.options.Bundle, "oci_layout": f.options.Layout, "controller": f.options.Controller, "controller_namespace": "delivery", "api_version": f.options.APIVersion, "context": "cluster-a"}
	base, err := releaseCheckMCPTool().BuildArgs(a)
	require.NoError(t, err)
	for _, plugin := range []string{"", "1"} {
		for _, format := range []string{"json", "ascii", "md"} {
			cmd := exec.CommandContext(ctx, binary, append(base, "--format", format)...)
			cmd.Env = append(os.Environ(), "CUB_PLUGIN="+plugin)
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", out)
			if format == "json" {
				var r agent.ReleaseCheckReport
				require.NoError(t, json.Unmarshal(out, &r))
				require.Equal(t, agent.VerdictPASS, r.Verdict)
			} else {
				require.Contains(t, string(out), "authored configuration matches")
			}
		}
	}
	if _, err := exec.LookPath("cub"); err == nil {
		dir := t.TempDir()
		p := filepath.Join(dir, "plugins", "scout")
		require.NoError(t, os.MkdirAll(p, 0700))
		require.NoError(t, os.Link(binary, filepath.Join(p, "main")))
		cmd := exec.CommandContext(ctx, "cub", append([]string{"scout"}, base...)...)
		cmd.Env = append(os.Environ(), "CUB_CONFIG="+dir, "CUB_CONTEXT=", "CUB_SPACE=", "CUB_TOKEN=")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", out)
		var r agent.ReleaseCheckReport
		require.NoError(t, json.Unmarshal(out, &r))
		require.Equal(t, agent.VerdictPASS, r.Verdict)
	}
	process := exec.CommandContext(ctx, binary, "mcp", "serve")
	stdin, err := process.StdinPipe()
	require.NoError(t, err)
	stdout, err := process.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, process.Start())
	defer func() { _ = stdin.Close(); _ = process.Process.Kill(); _ = process.Wait() }()
	reader := bufio.NewReader(stdout)
	for i := 0; i < 2; i++ {
		payload, err := json.Marshal(map[string]interface{}{"jsonrpc": "2.0", "id": i + 1, "method": "tools/call", "params": map[string]interface{}{"name": "release_check", "arguments": a}})
		require.NoError(t, err)
		_, err = fmt.Fprintf(stdin, "Content-Length: %d\r\n\r\n%s", len(payload), payload)
		require.NoError(t, err)
		data, err := readMCPFrame(reader)
		require.NoError(t, err)
		var response struct {
			Result struct {
				IsError bool `json:"isError"`
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"result"`
		}
		require.NoError(t, json.Unmarshal(data, &response))
		require.False(t, response.Result.IsError, "%s", data)
		require.Len(t, response.Result.Content, 1)
		var r agent.ReleaseCheckReport
		require.NoError(t, json.Unmarshal([]byte(response.Result.Content[0].Text), &r))
		require.Equal(t, agent.VerdictPASS, r.Verdict)
		require.Equal(t, 8, r.RequestCounts.Discovery+r.RequestCounts.Object)
	}
	cmd := newReleaseCheckCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs(append(base[2:], "--bundle", "oci://example.invalid/config:latest"))
	require.Error(t, cmd.Execute())
	require.Empty(t, output.String())
	for key, value := range map[string]interface{}{"max_objects": 1.5, "context": false, "bundle": "oci://example.invalid/config:latest", "unexpected": "x"} {
		copy := map[string]interface{}{}
		for k, v := range a {
			copy[k] = v
		}
		copy[key] = value
		_, err := releaseCheckMCPTool().BuildArgs(copy)
		require.Error(t, err)
	}
}

func TestReleaseCheckTUI(t *testing.T) {
	m := newReleaseCheckModel(context.Background(), releaseCheckOptions{})
	var contexts []context.Context
	m.observe = func(ctx context.Context, _ releaseCheckOptions) (agent.ReleaseCheckReport, error) {
		contexts = append(contexts, ctx)
		return agent.ReleaseCheckReport{Headline: "fixture"}, nil
	}
	first := m.Init()().(releaseCheckMessage)
	_, next := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	require.ErrorIs(t, contexts[0].Err(), context.Canceled)
	second := next().(releaseCheckMessage)
	m.Update(second)
	m.Update(first)
	require.Contains(t, m.content, "fixture")
	for _, size := range [][2]int{{100, 30}, {40, 12}, {20, 5}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, line := range strings.Split(m.View(), "\n") {
			require.LessOrEqual(t, ansi.StringWidth(line), size[0])
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	for _, line := range strings.Split(m.View(), "\n") {
		require.LessOrEqual(t, ansi.StringWidth(line), 20, "loading after refresh must also fit")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	require.ErrorIs(t, contexts[1].Err(), context.Canceled)
	before := m.content
	m.Update(second)
	require.Equal(t, before, m.content)
}

func TestReleaseCheckTerminalControls(t *testing.T) {
	r := agent.NewReleaseCheckReport(agent.BoundedResourceRef{}, "fixture", "fixture", 1, time.Now())
	r.Stages[0].Reason = "hostile\x1b[2J\u009b31m source"
	r.Finish(time.Now())
	for _, format := range []string{"ascii", "md"} {
		out := renderReleaseCheck(r, format)
		require.NotContains(t, out, "\x1b")
		require.NotContains(t, out, "\u009b")
	}
}

func TestReleaseCheckCoverageAndGate(t *testing.T) {
	t.Run("discovery not found is not object absence", func(t *testing.T) {
		f := newReleaseFixture(t, "argo")
		f.missingDiscovery = "v1"
		r, err := observeReleaseCheck(context.Background(), f.options)
		require.NoError(t, err)
		require.Equal(t, agent.VerdictINCONCLUSIVE, r.Verdict, "%+v", r.Stages)
		require.Zero(t, r.Configuration.Predicate.Evidence.ObjectSet.Summary.Missing)
		require.Equal(t, 1, r.Configuration.Predicate.Evidence.ObjectSet.Summary.Inconclusive)
		require.Equal(t, 3, r.RequestCounts.Object)
	})
	for _, tc := range []struct {
		kind    string
		verdict agent.ReceiptVerdict
	}{{"ConfigMap", agent.VerdictPASS}, {"Secret", agent.VerdictINCONCLUSIVE}} {
		t.Run(tc.kind, func(t *testing.T) {
			data := []byte(fmt.Sprintf("apiVersion: v1\nkind: %s\nmetadata:\n  name: settings\n  namespace: delivery\n", tc.kind))
			f := newReleaseFixture(t, "argo", data)
			r, err := observeReleaseCheck(context.Background(), f.options)
			require.NoError(t, err)
			require.Equal(t, tc.verdict, r.Verdict, "%+v", r.Stages)
			require.Nil(t, r.Convergence)
			require.Equal(t, "NOT_ASSESSED", r.Stages[3].Verdict)
			require.NotContains(t, r.Headline, "running")
			if tc.kind == "Secret" {
				f.mu.Lock()
				reads := f.gets["Secret/settings"]
				f.mu.Unlock()
				require.Zero(t, reads)
			}
		})
	}
	f := newReleaseFixture(t, "argo")
	f.options.MaxObjects = 1
	r, err := observeReleaseCheck(context.Background(), f.options)
	require.NoError(t, err)
	require.Equal(t, agent.VerdictINCONCLUSIVE, r.Verdict)
	require.Contains(t, r.Stages[0].Reason, "object limit")
	require.Zero(t, r.RequestCounts)
	f.options.MaxObjects = 100
	f.change = func(o *unstructured.Unstructured, _ int) int {
		if o.GetKind() == "ConfigMap" {
			return 404
		}
		if o.GetKind() == "Application" {
			return 403
		}
		return 0
	}
	a := map[string]interface{}{"bundle": f.options.Bundle, "oci_layout": f.options.Layout, "controller": f.options.Controller, "controller_namespace": "delivery", "api_version": f.options.APIVersion, "context": "cluster-a"}
	args, err := releaseCheckMCPTool().BuildArgs(a)
	require.NoError(t, err)
	outPath := filepath.Join(t.TempDir(), "report.json")
	cmd := newReleaseCheckCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(append(args[2:], "--out", outPath, "--fail-on", "any-non-pass"))
	require.Error(t, cmd.Execute())
	require.NoError(t, json.Unmarshal(out.Bytes(), &r))
	require.Equal(t, agent.VerdictBLOCK, r.Verdict, "known failure must not disappear behind unavailable controller")
	written, err := os.ReadFile(outPath)
	require.NoError(t, err)
	require.JSONEq(t, out.String(), string(written))
}

func TestReleaseCheckContextIsolation(t *testing.T) {
	management := newReleaseFixture(t, "argo")
	target := newReleaseFixture(t, "argo")
	cfg := clientcmdapi.NewConfig()
	cfg.CurrentContext = "management"
	for name, host := range map[string]string{"management": management.host, "target": target.host} {
		cfg.Clusters[name] = &clientcmdapi.Cluster{Server: host}
		cfg.Contexts[name] = &clientcmdapi.Context{Cluster: name}
	}
	data, err := clientcmd.Write(*cfg)
	require.NoError(t, err)
	p := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.WriteFile(p, data, 0600))
	t.Setenv("KUBECONFIG", p)
	management.change = func(o *unstructured.Unstructured, _ int) int {
		if o.GetKind() == "Application" {
			for _, prefix := range [][]string{{"spec", "destination", "server"}, {"status", "sync", "comparedTo", "destination", "server"}} {
				_ = unstructured.SetNestedField(o.Object, target.host, prefix...)
			}
		}
		return 0
	}
	target.change = func(o *unstructured.Unstructured, _ int) int {
		if o.GetKind() == "ConfigMap" {
			_ = unstructured.SetNestedField(o.Object, "debug", "data", "LOG_LEVEL")
		}
		return 0
	}
	o := management.options
	o.Context = "target"
	o.ControllerContext = "management"
	r, err := observeReleaseCheck(context.Background(), o)
	require.NoError(t, err)
	require.Equal(t, "PASS", r.Stages[1].Verdict)
	require.Equal(t, agent.VerdictBLOCK, r.Verdict)
	management.mu.Lock()
	managementCount := management.requests
	managementObjects := len(management.gets)
	management.mu.Unlock()
	target.mu.Lock()
	targetCount := target.requests
	targetControllerReads := target.gets["Application/api"]
	target.mu.Unlock()
	require.Equal(t, 4, managementCount)
	require.Equal(t, 1, managementObjects)
	require.Equal(t, 4, targetCount)
	require.Zero(t, targetControllerReads)
}

func TestReleaseCheckLive(t *testing.T) {
	kubeContext := os.Getenv("CUB_SCOUT_RELEASE_LIVE_CONTEXT")
	if kubeContext == "" {
		t.Skip("opt-in non-production read-only smoke")
	}
	ns, name := os.Getenv("CUB_SCOUT_RELEASE_LIVE_NAMESPACE"), os.Getenv("CUB_SCOUT_RELEASE_LIVE_NAME")
	require.NotEmpty(t, ns)
	require.NotEmpty(t, name)
	ref := agent.BoundedResourceRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: ns, Name: name}
	session := &boundedExplainSession{}
	reader, err := session.forContext(kubeContext)
	require.NoError(t, err)
	live, _, err := reader.Read(context.Background(), ref, true)
	require.NoError(t, err)
	// The observed replica count is an explicit smoke-test expectation, not a
	// published release or proof that the original intended config was recovered.
	desired := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]interface{}{"namespace": ns, "name": name}}}
	if replicas, found, _ := unstructured.NestedInt64(live.Object, "spec", "replicas"); found {
		require.NoError(t, unstructured.SetNestedField(desired.Object, replicas, "spec", "replicas"))
	}
	data, err := desired.MarshalJSON()
	require.NoError(t, err)
	dir := t.TempDir()
	bundleRef, err := releasetest.WriteLayout(dir, map[string][]byte{"smoke.json": data})
	require.NoError(t, err)
	o := releaseCheckOptions{Bundle: bundleRef, Layout: dir, Controller: "Deployment/" + name, APIVersion: "apps/v1", ControllerNamespace: ns, Context: kubeContext, MaxObjects: 100}
	r, err := observeReleaseCheck(context.Background(), o)
	require.NoError(t, err)
	require.Equal(t, agent.VerdictINCONCLUSIVE, r.Verdict, "native workload cannot provide a GitOps controller binding")
	require.Equal(t, "PASS", r.Stages[0].Verdict)
	require.Equal(t, "PASS", r.Stages[2].Verdict)
	require.Equal(t, agent.BoundedReadCounts{Discovery: 3, Object: 3}, r.RequestCounts)
	t.Logf("read-only live proof: context=%s workload=%s/%s bundle=PASS configuration=PASS convergence=%s controller=INCONCLUSIVE requests=%+v", kubeContext, ns, name, r.Stages[3].Verdict, r.RequestCounts)
}
