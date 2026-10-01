// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/stretchr/testify/require"
)

func TestMCPSourceTruthContextIsTypedAndForwarded(t *testing.T) {
	var captured []string
	runner := func(_ context.Context, args []string) (string, error) {
		captured = append([]string(nil), args...)
		return strings.Join(args, " "), nil
	}
	gateway := newMCPGatewayWithMode(nil, runner, true)
	params, err := json.Marshal(map[string]interface{}{
		"name":      "compare_source_truth",
		"arguments": map[string]interface{}{"target": "Deployment/api", "namespace": "team-a", "strategy": "git-argo", "context": "alpha-context"},
	})
	require.NoError(t, err)
	response := gateway.handleRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/call", Params: params})
	require.Nil(t, response.Error)
	require.Equal(t, []string{"compare", "source-truth", "Deployment/api", "-n", "team-a", "--strategy", "git-argo", "--kube-context", "alpha-context", "--format", "json"}, captured)
	tool := gateway.tools["compare_source_truth"]
	args, err := tool.BuildArgs(map[string]interface{}{
		"target": "Deployment/api", "namespace": "team-a", "strategy": "git-argo", "context": "alpha-context",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"compare", "source-truth", "Deployment/api", "-n", "team-a", "--strategy", "git-argo", "--kube-context", "alpha-context", "--format", "json"}, args)
	properties := tool.Descriptor.InputSchema["properties"].(map[string]interface{})
	require.Equal(t, "string", properties["context"].(map[string]interface{})["type"])
	for _, invalid := range []interface{}{nil, 12, "", " "} {
		_, err := tool.BuildArgs(map[string]interface{}{"target": "Deployment/api", "namespace": "team-a", "strategy": "git-argo", "context": invalid})
		require.Error(t, err)
	}
}

func TestSourceTruthCLIExplicitContextBindsRuntimeAndArgoAfterRetarget(t *testing.T) {
	alpha := newSourceTruthHTTPFixture(t, "alpha")
	alpha.listObservedRevision = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	beta := newSourceTruthHTTPFixture(t, "beta")
	path := filepath.Join(t.TempDir(), "config")
	before := writeTraceKubeconfig(t, path, "beta-context", alpha.URL, beta.URL)
	t.Setenv("KUBECONFIG", path)
	oldUnitGet := sourceTruthUnitGet
	t.Cleanup(func() { sourceTruthUnitGet = oldUnitGet })
	unitReads := 0
	sourceTruthUnitGet = func(context.Context, string, string) ([]byte, error) {
		unitReads++
		return []byte(`{"HeadRevisionNum":3,"SpaceID":"space-id","UnitID":"unit-id"}`), nil
	}
	stubConnectedGate(t, nil)
	writeTraceKubeconfig(t, path, "beta-context", alpha.URL, beta.URL)
	command := selectedTraceCommand(t, "alpha-context")
	oldNamespace, oldStrategy, oldFormat := sourceTruthNamespace, sourceTruthStrategy, sourceTruthFormat
	t.Cleanup(func() {
		sourceTruthNamespace, sourceTruthStrategy, sourceTruthFormat = oldNamespace, oldStrategy, oldFormat
	})
	sourceTruthNamespace, sourceTruthStrategy, sourceTruthFormat = "team-a", "git-argo", "json"
	var runErr error
	output := captureStdout(t, func() { runErr = runSourceTruth(command, []string{"Deployment/api"}) })
	require.NoError(t, runErr)
	var evidence agent.SourceTruthEvidence
	require.NoError(t, json.Unmarshal([]byte(output), &evidence))
	require.Equal(t, "alpha-context", evidence.Context)
	require.Equal(t, agent.StatusPASS, evidence.Status)
	require.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", evidence.Surfaces.Controller.RevisionOrDigest, "Argo's observed sync revision, not spec.source.targetRevision, is the comparison anchor")
	require.Equal(t, 1, unitReads, "ConfigHub lookup is retained and separate from Kubernetes context selection")
	require.Greater(t, alpha.count(http.MethodGet, "/apis/apps/v1/namespaces/team-a/deployments/api"), 0)
	require.Greater(t, alpha.count(http.MethodGet, "/apis/argoproj.io/v1alpha1/applications"), 0)
	require.Equal(t, 1, alpha.count(http.MethodGet, "/apis/argoproj.io/v1alpha1/namespaces/argo-system/applications/api"), "same-name Application collision must resolve by exact workload namespace membership")
	require.Zero(t, alpha.count(http.MethodGet, "/apis/argoproj.io/v1alpha1/namespaces/other-system/applications/api"))
	require.Equal(t, before, mustReadFile(t, path), "the caller kubeconfig must remain unchanged")
	require.Empty(t, beta.allRequests(), "selected context must not fall through to ambient current-context")
}

func TestSourceTruthArgoMissingObservedRevisionIsUnavailable(t *testing.T) {
	alpha := newSourceTruthHTTPFixture(t, "alpha")
	alpha.observedRevision = ""
	path := filepath.Join(t.TempDir(), "config")
	writeTraceKubeconfig(t, path, "alpha-context", alpha.URL, alpha.URL)
	t.Setenv("KUBECONFIG", path)
	oldUnitGet := sourceTruthUnitGet
	t.Cleanup(func() { sourceTruthUnitGet = oldUnitGet })
	sourceTruthUnitGet = func(context.Context, string, string) ([]byte, error) {
		return []byte(`{"HeadRevisionNum":3,"SpaceID":"space-id","UnitID":"unit-id"}`), nil
	}
	session, err := newTraceSessionForSelection(clusterContextSelection{name: "alpha-context", explicit: true})
	require.NoError(t, err)
	observed := collectSourceTruthObservation(context.Background(), session, "Deployment", "api", "team-a", agent.StrategyGitArgo)
	require.NotNil(t, observed.Evidence.Surfaces.Controller)
	require.Empty(t, observed.Evidence.Surfaces.Controller.RevisionOrDigest)
	require.NotEqual(t, agent.StatusPASS, observed.Evidence.Status)
}

func TestSourceTruthArgoMultiSourceRevisionIsUnavailable(t *testing.T) {
	alpha := newSourceTruthHTTPFixture(t, "alpha")
	alpha.multiSource = true
	path := filepath.Join(t.TempDir(), "config")
	writeTraceKubeconfig(t, path, "alpha-context", alpha.URL, alpha.URL)
	t.Setenv("KUBECONFIG", path)
	oldUnitGet := sourceTruthUnitGet
	t.Cleanup(func() { sourceTruthUnitGet = oldUnitGet })
	sourceTruthUnitGet = func(context.Context, string, string) ([]byte, error) {
		return []byte(`{"HeadRevisionNum":3,"SpaceID":"space-id","UnitID":"unit-id"}`), nil
	}
	session, err := newTraceSessionForSelection(clusterContextSelection{name: "alpha-context", explicit: true})
	require.NoError(t, err)
	observed := collectSourceTruthObservation(context.Background(), session, "Deployment", "api", "team-a", agent.StrategyGitArgo)
	require.NotNil(t, observed.Evidence.Surfaces.Controller)
	require.True(t, observed.Evidence.Surfaces.Controller.MultiSource)
	require.Empty(t, observed.Evidence.Surfaces.Controller.RevisionOrDigest)
	require.NotEqual(t, agent.StatusPASS, observed.Evidence.Status)
}

func TestSourceTruthDeniedControllerCannotProducePass(t *testing.T) {
	alpha := newSourceTruthHTTPFixture(t, "alpha")
	alpha.denyArgo = true
	path := filepath.Join(t.TempDir(), "config")
	writeTraceKubeconfig(t, path, "alpha-context", alpha.URL, alpha.URL)
	t.Setenv("KUBECONFIG", path)
	oldUnitGet := sourceTruthUnitGet
	t.Cleanup(func() { sourceTruthUnitGet = oldUnitGet })
	sourceTruthUnitGet = func(context.Context, string, string) ([]byte, error) {
		return []byte(`{"HeadRevisionNum":3,"SpaceID":"space-id","UnitID":"unit-id"}`), nil
	}
	session, err := newTraceSessionForSelection(clusterContextSelection{name: "alpha-context", explicit: true})
	require.NoError(t, err)
	observed := collectSourceTruthObservation(context.Background(), session, "Deployment", "api", "team-a", agent.StrategyGitArgo)
	require.Equal(t, agent.StatusBLOCK, observed.Evidence.Status)
	require.Equal(t, agent.VerdictBLOCKED, observed.Evidence.SourceTruth)
	require.Nil(t, observed.Evidence.Surfaces.Controller)
	require.NotEmpty(t, observed.ControllerError)
}

func TestSourceTruthDeniedConfigHubCannotProducePass(t *testing.T) {
	alpha := newSourceTruthHTTPFixture(t, "alpha")
	path := filepath.Join(t.TempDir(), "config")
	writeTraceKubeconfig(t, path, "alpha-context", alpha.URL, alpha.URL)
	t.Setenv("KUBECONFIG", path)
	oldUnitGet := sourceTruthUnitGet
	t.Cleanup(func() { sourceTruthUnitGet = oldUnitGet })
	sourceTruthUnitGet = func(context.Context, string, string) ([]byte, error) {
		return nil, os.ErrPermission
	}
	session, err := newTraceSessionForSelection(clusterContextSelection{name: "alpha-context", explicit: true})
	require.NoError(t, err)
	observed := collectSourceTruthObservation(context.Background(), session, "Deployment", "api", "team-a", agent.StrategyGitArgo)
	require.Equal(t, agent.StatusBLOCK, observed.Evidence.Status)
	require.Equal(t, agent.VerdictBLOCKED, observed.Evidence.SourceTruth)
	require.Nil(t, observed.Evidence.Surfaces.ConfigHub)
	require.NotEmpty(t, observed.ConfigHubError)
	require.NotEmpty(t, observed.Evidence.CollectionErrors)
}

func TestSourceTruthInvalidExplicitContextReadsNoEndpoint(t *testing.T) {
	alpha := newSourceTruthHTTPFixture(t, "alpha")
	path := filepath.Join(t.TempDir(), "config")
	writeTraceKubeconfig(t, path, "alpha-context", alpha.URL, alpha.URL)
	t.Setenv("KUBECONFIG", path)
	oldNamespace, oldStrategy, oldFormat := sourceTruthNamespace, sourceTruthStrategy, sourceTruthFormat
	t.Cleanup(func() {
		sourceTruthNamespace, sourceTruthStrategy, sourceTruthFormat = oldNamespace, oldStrategy, oldFormat
	})
	sourceTruthNamespace, sourceTruthStrategy, sourceTruthFormat = "team-a", "git-argo", "json"
	err := runSourceTruth(selectedTraceCommand(t, "missing-context"), []string{"Deployment/api"})
	require.ErrorContains(t, err, "missing-context")
	require.Empty(t, alpha.allRequests())
}

func TestSourceTruthUnknownStrategyRespectsFormatAndValidatesExplicitContext(t *testing.T) {
	alpha := newSourceTruthHTTPFixture(t, "alpha")
	path := filepath.Join(t.TempDir(), "config")
	writeTraceKubeconfig(t, path, "alpha-context", alpha.URL, alpha.URL)
	t.Setenv("KUBECONFIG", path)
	oldNamespace, oldStrategy, oldFormat := sourceTruthNamespace, sourceTruthStrategy, sourceTruthFormat
	t.Cleanup(func() {
		sourceTruthNamespace, sourceTruthStrategy, sourceTruthFormat = oldNamespace, oldStrategy, oldFormat
	})
	sourceTruthNamespace, sourceTruthStrategy = "team-a", "not-a-strategy"
	for _, format := range []string{"json", "ascii", "md"} {
		sourceTruthFormat = format
		output := captureStdout(t, func() {
			command := selectedTraceCommand(t, "")
			command.Flags().Lookup("kube-context").Changed = false
			require.NoError(t, runSourceTruth(command, []string{"Deployment/api"}))
		})
		switch format {
		case "json":
			var evidence agent.SourceTruthEvidence
			require.NoError(t, json.Unmarshal([]byte(output), &evidence))
			require.Equal(t, agent.StatusASK, evidence.Status)
		case "ascii":
			require.Contains(t, output, "Source truth: UNKNOWN (ASK)")
		case "md":
			require.Contains(t, output, "## Source truth: UNKNOWN (ASK)")
		}
	}
	err := runSourceTruth(selectedTraceCommand(t, "missing-context"), []string{"Deployment/api"})
	require.ErrorContains(t, err, "missing-context")
	require.Empty(t, alpha.allRequests(), "invalid explicit selection must fail before any cluster read")
}

func TestSourceTruthFormatRenderersProjectSameEvidence(t *testing.T) {
	evidence := agent.Derive(agent.StrategyGitArgo, agent.SourceTruthSurfaces{})
	evidence.Context = "alpha-context"
	outputs := map[string]string{}
	for _, format := range []string{"json", "ascii", "md"} {
		var output strings.Builder
		require.NoError(t, outputSourceTruth(&output, evidence, format))
		outputs[format] = output.String()
	}
	var decoded agent.SourceTruthEvidence
	require.NoError(t, json.Unmarshal([]byte(outputs["json"]), &decoded))
	require.Equal(t, evidence.Status, decoded.Status)
	require.Equal(t, evidence.Context, decoded.Context)
	require.Contains(t, outputs["ascii"], "Selected Kubernetes context: alpha-context")
	require.Contains(t, outputs["ascii"], "Runtime: unavailable")
	require.Contains(t, outputs["md"], "Selected Kubernetes context: `alpha-context`")
	require.Contains(t, outputs["md"], "Runtime: unavailable")
}

func TestSourceTruthFluxChildUsesCapturedContextAfterRetarget(t *testing.T) {
	alpha := newSourceTruthHTTPFixture(t, "alpha")
	beta := newSourceTruthHTTPFixture(t, "beta")
	path := filepath.Join(t.TempDir(), "config")
	writeTraceKubeconfig(t, path, "alpha-context", alpha.URL, beta.URL)
	t.Setenv("KUBECONFIG", path)
	fluxDir := t.TempDir()
	log := filepath.Join(fluxDir, "seen")
	script := `#!/bin/sh
cfg=""
for arg in "$@"; do
  if [ "$previous" = "--kubeconfig" ]; then cfg="$arg"; fi
  previous="$arg"
done
server=""
while IFS= read -r line; do case "$line" in *"server: "*) server=${line#*server: };; esac; done < "$cfg"
printf '%s|%s|%s\n' "$KUBECONFIG" "$cfg" "$server" > "$SCOUT_FLUX_LOG"
printf 'Kustomization: api\nNamespace: team-a\nStatus: Ready\n---\nGitRepository: source\nNamespace: flux-system\nURL: https://git.example/repo\nRevision: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\nStatus: Ready\n'
`
	require.NoError(t, os.WriteFile(filepath.Join(fluxDir, "flux"), []byte(script), 0700))
	t.Setenv("PATH", fluxDir)
	t.Setenv("SCOUT_FLUX_LOG", log)
	oldUnitGet := sourceTruthUnitGet
	t.Cleanup(func() { sourceTruthUnitGet = oldUnitGet })
	sourceTruthUnitGet = func(context.Context, string, string) ([]byte, error) {
		return []byte(`{"HeadRevisionNum":3,"SpaceID":"space-id","UnitID":"unit-id"}`), nil
	}
	session, err := newTraceSessionForSelection(clusterContextSelection{name: "alpha-context", explicit: true})
	require.NoError(t, err)
	writeTraceKubeconfig(t, path, "beta-context", alpha.URL, beta.URL)
	observed := collectSourceTruthObservation(context.Background(), session, "Deployment", "api", "team-a", agent.StrategyGitFlux)
	require.NotNil(t, observed.Evidence.Surfaces.Controller)
	require.Equal(t, agent.StatusPASS, observed.Evidence.Status, "%+v", observed.Evidence)
	lines, err := os.ReadFile(log)
	require.NoError(t, err)
	parts := strings.Split(strings.TrimSpace(string(lines)), "|")
	require.Len(t, parts, 3)
	require.Equal(t, parts[1], parts[0], "Flux KUBECONFIG must be the exact captured child file")
	require.Equal(t, alpha.URL, parts[2], "the child's private kubeconfig must select endpoint A after ambient retarget")
	require.Empty(t, beta.allRequests())
}

func TestSourceTruthTUIEventUsesSharedCollectorAndRenderer(t *testing.T) {
	alpha := newSourceTruthHTTPFixture(t, "alpha")
	beta := newSourceTruthHTTPFixture(t, "beta")
	path := filepath.Join(t.TempDir(), "config")
	writeTraceKubeconfig(t, path, "alpha-context", alpha.URL, beta.URL)
	t.Setenv("KUBECONFIG", path)
	oldUnitGet := sourceTruthUnitGet
	t.Cleanup(func() { sourceTruthUnitGet = oldUnitGet })
	sourceTruthUnitGet = func(context.Context, string, string) ([]byte, error) {
		return []byte(`{"HeadRevisionNum":3,"SpaceID":"space-id","UnitID":"unit-id"}`), nil
	}
	stubConnectedGate(t, nil)
	binding := resolveLocalClusterBindingForSelection(clusterContextSelection{name: "alpha-context", explicit: true})
	require.NoError(t, binding.err)
	model := LocalClusterModel{ready: true, cursor: 0, clusterBinding: binding, entries: []MapEntry{{Kind: "Deployment", Name: "api", Namespace: "team-a"}}}
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Y")})
	model = updated.(LocalClusterModel)
	require.True(t, model.sourceTruthMode)
	for i, strategy := range agent.AllStrategies() {
		if strategy == agent.StrategyGitArgo {
			for model.sourceTruthCursor < i {
				updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
				model = updated.(LocalClusterModel)
			}
			break
		}
	}
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(LocalClusterModel)
	require.NotNil(t, cmd)
	message := cmd().(sourceTruthResultMsg)
	require.NoError(t, message.err)
	updated, _ = model.Update(message)
	model = updated.(LocalClusterModel)
	require.Contains(t, model.renderSourceTruth(), "Selected Kubernetes context: alpha-context")
	require.Contains(t, model.renderSourceTruth(), "ConfigHub")
	require.Contains(t, model.renderSourceTruth(), "Controller")
	require.Empty(t, beta.allRequests())
}

func TestSourceTruthTUIDiscardsLateResultAfterCloseAndReopen(t *testing.T) {
	alpha := newSourceTruthHTTPFixture(t, "alpha")
	path := filepath.Join(t.TempDir(), "config")
	writeTraceKubeconfig(t, path, "alpha-context", alpha.URL, alpha.URL)
	t.Setenv("KUBECONFIG", path)
	stubConnectedGate(t, nil)
	binding := resolveLocalClusterBindingForSelection(clusterContextSelection{name: "alpha-context", explicit: true})
	require.NoError(t, binding.err)
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstCanceled := make(chan struct{})
	oldCollect := sourceTruthCollectFn
	t.Cleanup(func() { sourceTruthCollectFn = oldCollect })
	calls := 0
	sourceTruthCollectFn = func(ctx context.Context, _ *traceSession, _, name, _ string, _ agent.SourceTruthStrategy) sourceTruthObservation {
		calls++
		if calls == 1 {
			close(firstStarted)
			<-releaseFirst
			if ctx.Err() != nil {
				close(firstCanceled)
			}
			return sourceTruthObservation{Evidence: agent.SourceTruthEvidence{Status: agent.StatusWATCH, ProofGaps: []string{"stale-" + name}}}
		}
		return sourceTruthObservation{Evidence: agent.SourceTruthEvidence{Status: agent.StatusPASS, ProofGaps: []string{"fresh-" + name}}}
	}
	model := LocalClusterModel{ready: true, cursor: 0, clusterBinding: binding, entries: []MapEntry{
		{Kind: "Deployment", Name: "first", Namespace: "team-a"},
		{Kind: "Deployment", Name: "second", Namespace: "team-b"},
	}}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Y")})
	model = updated.(LocalClusterModel)
	updated, firstCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(LocalClusterModel)
	firstDone := make(chan tea.Msg, 1)
	go func() { firstDone <- firstCmd() }()
	<-firstStarted
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(LocalClusterModel)
	require.False(t, model.sourceTruthMode)
	require.Nil(t, model.sourceTruthContext, "the view should release its canceled context after close")
	model.cursor = 1
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Y")})
	model = updated.(LocalClusterModel)
	require.Equal(t, "second", model.sourceTruthItem.Name)
	updated, secondCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(LocalClusterModel)
	secondMsg := secondCmd().(sourceTruthResultMsg)
	updated, _ = model.Update(secondMsg)
	model = updated.(LocalClusterModel)
	require.Contains(t, model.sourceTruthEvidence.ProofGaps, "fresh-second")
	close(releaseFirst)
	staleMsg := <-firstDone
	updated, _ = model.Update(staleMsg)
	model = updated.(LocalClusterModel)
	require.Contains(t, model.sourceTruthEvidence.ProofGaps, "fresh-second", "late result for the prior workload must not replace current evidence")
	select {
	case <-firstCanceled:
	default:
		t.Fatal("the canceled collector context was not observed by the source-truth read path")
	}
}

type sourceTruthHTTPFixture struct {
	URL                  string
	server               *httptest.Server
	mu                   sync.Mutex
	requests             map[string]int
	denyArgo             bool
	observedRevision     string
	listObservedRevision string
	multiSource          bool
}

func newSourceTruthHTTPFixture(t *testing.T, marker string) *sourceTruthHTTPFixture {
	t.Helper()
	f := &sourceTruthHTTPFixture{requests: map[string]int{}, observedRevision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		key := r.Method + " " + r.URL.Path
		f.requests[key]++
		f.mu.Unlock()
		if f.denyArgo && strings.Contains(r.URL.Path, "/applications") {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/apps/v1/namespaces/team-a/deployments/api":
			writeTraceFixtureJSON(w, map[string]interface{}{
				"apiVersion": "apps/v1", "kind": "Deployment",
				"metadata": map[string]interface{}{"name": "api", "namespace": "team-a", "labels": map[string]interface{}{"confighub.com/UnitSlug": "api-unit", "argocd.argoproj.io/instance": "api"}, "annotations": map[string]interface{}{"confighub.com/SpaceName": "prod"}},
				"spec":     map[string]interface{}{"template": map[string]interface{}{"spec": map[string]interface{}{"containers": []interface{}{map[string]interface{}{"name": "api", "image": "registry.example/api:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}}}},
				"status":   map[string]interface{}{"conditions": []interface{}{map[string]interface{}{"type": "Available", "status": "True", "lastTransitionTime": "2025-01-02T03:04:05Z"}}},
			})
		case "/apis/argoproj.io/v1alpha1/applications":
			if r.URL.Query().Get("fieldSelector") != "metadata.name=api" {
				t.Errorf("Argo lookup must be bounded to the observed application name; query=%s", r.URL.RawQuery)
			}
			writeTraceFixtureJSON(w, map[string]interface{}{"apiVersion": "argoproj.io/v1alpha1", "kind": "ApplicationList", "metadata": map[string]interface{}{}, "items": []interface{}{
				sourceTruthTestApplication("argo-system", "https://git.example/repo", "declared-target", firstNonEmpty(f.listObservedRevision, f.observedRevision), "team-a", f.multiSource),
				sourceTruthTestApplication("other-system", "https://attacker.invalid/wrong", "wrong", "wrong", "team-b", false),
			}})
		case "/apis/argoproj.io/v1alpha1/namespaces/argo-system/applications/api":
			writeTraceFixtureJSON(w, sourceTruthTestApplication("argo-system", "https://git.example/repo", "declared-target", f.observedRevision, "team-a", f.multiSource))
		default:
			http.NotFound(w, r)
		}
	}))
	f.URL = f.server.URL
	t.Cleanup(f.server.Close)
	return f
}

func sourceTruthTestApplication(appNamespace, repo, targetRevision, observedRevision, workloadNamespace string, multiSource bool) map[string]interface{} {
	source := map[string]interface{}{"repoURL": repo, "targetRevision": targetRevision}
	spec := map[string]interface{}{"destination": map[string]interface{}{"namespace": workloadNamespace}}
	if multiSource {
		spec["sources"] = []interface{}{source, map[string]interface{}{"repoURL": "https://charts.example.invalid", "chart": "api", "targetRevision": "1.0"}}
	} else {
		spec["source"] = source
	}
	return map[string]interface{}{
		"apiVersion": "argoproj.io/v1alpha1", "kind": "Application",
		"metadata": map[string]interface{}{"name": "api", "namespace": appNamespace},
		"spec":     spec,
		"status": map[string]interface{}{
			"sync":      map[string]interface{}{"status": "Synced", "revision": observedRevision},
			"health":    map[string]interface{}{"status": "Healthy"},
			"resources": []interface{}{map[string]interface{}{"kind": "Deployment", "name": "api", "namespace": workloadNamespace}},
		},
	}
}

func (f *sourceTruthHTTPFixture) allRequests() map[string]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	copy := make(map[string]int, len(f.requests))
	for key, value := range f.requests {
		copy[key] = value
	}
	return copy
}

func (f *sourceTruthHTTPFixture) count(method, path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[method+" "+path]
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return b
}
