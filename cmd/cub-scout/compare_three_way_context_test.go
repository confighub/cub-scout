// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// Extend the existing two-endpoint TLS/auth source fixture with discovery and
// workload/Pod rollout responses. All data and credentials are synthetic.
func newThreeWayContextFixture(t *testing.T, marker string, denied map[string]int, labelsByNamespace ...map[string]map[string]string) *compareSessionFixture {
	f := newCompareSessionFixture(t, marker)
	original := f.server.Config.Handler
	f.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		intercepted := strings.HasPrefix(path, "/apis/apps/v1/") || strings.HasPrefix(path, "/api/v1/")
		if !intercepted {
			original.ServeHTTP(w, r)
			return
		}
		f.mu.Lock()
		f.reads[path]++
		f.mu.Unlock()
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "Bearer "+marker+"-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		if status := denied[path]; status != 0 {
			writeTraceStatus(w, status, http.StatusText(status))
			return
		}
		object := func(namespace string) map[string]interface{} {
			labels := f.labels
			if len(labelsByNamespace) > 0 {
				labels = labelsByNamespace[0][namespace]
			}
			return map[string]interface{}{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]interface{}{"name": "app", "namespace": namespace, "generation": int64(3), "labels": labels}, "spec": map[string]interface{}{"replicas": int64(2), "selector": map[string]interface{}{"matchLabels": map[string]interface{}{"app": "app"}}, "template": map[string]interface{}{"spec": map[string]interface{}{"containers": []interface{}{map[string]interface{}{"name": "app", "image": marker + ":v1"}}}}}, "status": map[string]interface{}{"observedGeneration": int64(3), "replicas": int64(2), "updatedReplicas": int64(2), "readyReplicas": int64(2), "availableReplicas": int64(2)}}
		}
		switch {
		case strings.HasSuffix(path, "/deployments/app"):
			ns := "team"
			if strings.Contains(path, "/kube-system/") {
				ns = "kube-system"
			}
			_ = json.NewEncoder(w).Encode(object(ns))
		case strings.HasSuffix(path, "/deployments"):
			items := []interface{}{object("team")}
			if path == "/apis/apps/v1/deployments" {
				items = append(items, object("kube-system"))
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"apiVersion": "apps/v1", "kind": "DeploymentList", "items": items})
		case strings.HasSuffix(path, "/statefulsets"), strings.HasSuffix(path, "/daemonsets"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"apiVersion": "apps/v1", "kind": "List", "items": []interface{}{}})
		case strings.HasSuffix(path, "/pods"):
			require.Equal(t, "app=app", r.URL.Query().Get("labelSelector"))
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"apiVersion": "v1", "kind": "PodList", "items": []interface{}{}})
		default:
			t.Errorf("unexpected comparison request %s", path)
			writeTraceStatus(w, 404, "Not Found")
		}
	})
	return f
}

func standaloneThreeWayTest(t *testing.T) {
	stubConnectedGate(t, nil) // Private fixture tests never launch a real auth subprocess.
	previous := compareConnectedFn
	compareConnectedFn = func() bool { return false }
	t.Cleanup(func() { compareConnectedFn = previous })
}

func TestThreeWayCapturedContextAllScopesAndSeparateViewAuthority(t *testing.T) {
	const viewID = "806aac53-236c-446d-8ad6-91d6daf6810e"
	for _, scope := range []threeWayScope{{ScopeType: threeWayScopeResource, ScopeValue: "deploy/app"}, {ScopeType: threeWayScopeNamespace, ScopeValue: "team"}, {ScopeType: threeWayScopeCluster, ScopeValue: "cluster"}, {ScopeType: threeWayScopeView, ScopeValue: viewID}} {
		t.Run(string(scope.ScopeType), func(t *testing.T) {
			standaloneThreeWayTest(t)
			alpha, beta := newThreeWayContextFixture(t, "alpha", nil), newThreeWayContextFixture(t, "beta", nil)
			alpha.labels["confighub.com/UnitSlug"] = "alpha-unit"
			alpha.labels["confighub.com/SpaceID"] = "separate-space"
			session := capturedCompareTestSession(t, alpha, beta)
			previous := viewCubRunner
			t.Cleanup(func() { viewCubRunner = previous })
			var hubCalls [][]string
			viewCubRunner = func(_ context.Context, args ...string) ([]byte, error) {
				hubCalls = append(hubCalls, args)
				switch strings.Join(args, " ") {
				case "view get " + viewID + " --space * -o json":
					return []byte(fakeViewJSON("Slug = 'alpha-unit'")), nil
				default:
					if len(args) > 1 && args[0] == "unit" && args[1] == "list" {
						return []byte(`[{"UnitID":"alpha-unit-id","SpaceID":"separate-space","Slug":"alpha-unit"}]`), nil
					}
					return nil, fmt.Errorf("unexpected ConfigHub request %v", args)
				}
			}
			report, err := collectThreeWayWithSession(context.Background(), session, scope, threeWayOptions{Namespace: "team"})
			require.NoError(t, err)
			require.Equal(t, "selected", report.Context)
			wantCount := 1
			if scope.ScopeType == threeWayScopeCluster || scope.ScopeType == threeWayScopeView {
				wantCount = 2
			}
			require.Len(t, report.Resources, wantCount, "system namespaces stay in cluster/View scope")
			require.Empty(t, beta.counts(), "retargeting the private caller config cannot redirect any nested read")
			for _, entry := range report.Resources {
				require.Equal(t, []string{"alpha:v1"}, entry.Result.Live.Images)
				require.Equal(t, "https://alpha.invalid/repo", entry.Result.Live.GitSource.RepoURL)
				require.NotNil(t, entry.CurrentChange)
				require.NotZero(t, alpha.counts()["/api/v1/namespaces/"+entry.Result.Namespace+"/pods"])
			}
			if scope.ScopeType == threeWayScopeView {
				require.Len(t, hubCalls, 2)
				require.Contains(t, strings.Join(hubCalls[1], " "), "--space *")
			} else {
				require.Empty(t, hubCalls)
			}
			require.Contains(t, renderThreeWayASCII(report), "Kubernetes context label: selected")
			require.Contains(t, renderThreeWayMarkdown(report), "Kubernetes context label: `selected`")
			data, err := json.Marshal(report)
			require.NoError(t, err)
			require.Contains(t, string(data), `"context":"selected"`)
			for _, hint := range report.NextSteps {
				if strings.Contains(hint.NextSurface, "cub-scout explain") || strings.Contains(hint.NextSurface, "cub-scout compare") {
					require.Contains(t, hint.NextSurface, "--kube-context selected")
				}
			}
		})
	}
}

func TestThreeWayCapturedFluxInvocation(t *testing.T) {
	standaloneThreeWayTest(t)
	alpha, beta := newThreeWayContextFixture(t, "alpha", nil), newThreeWayContextFixture(t, "beta", nil)
	alpha.labels = map[string]string{"helm.toolkit.fluxcd.io/name": "release", "helm.toolkit.fluxcd.io/namespace": "flux-system", "confighub.com/UnitSlug": "alpha-unit"}
	session := capturedCompareTestSession(t, alpha, beta)
	factory := func(got *traceSession) (agent.Tracer, func() error, error) {
		require.Same(t, session, got)
		return &compareBoundFluxFixture{session: got, t: t}, func() error { return nil }, nil
	}
	report, err := collectThreeWayWithSession(context.Background(), session, threeWayScope{ScopeType: threeWayScopeNamespace, ScopeValue: "team"}, threeWayOptions{Flux: factory})
	require.NoError(t, err)
	require.Len(t, report.Resources, 1)
	require.Equal(t, "https://alpha.invalid/repo", report.Resources[0].Result.Live.GitSource.RepoURL)
	require.NotZero(t, alpha.counts()["/apis/source.toolkit.fluxcd.io/v1/namespaces/flux-system/gitrepositories/source"])
	require.Empty(t, beta.counts())
}

func TestThreeWayPartialDiscoveryLiveSourceAndPods(t *testing.T) {
	for _, tc := range []struct {
		name          string
		denied        map[string]int
		sourceDenied  bool
		scope         threeWayScope
		wantResources int
		phase         string
	}{
		{name: "discovery", denied: map[string]int{"/apis/apps/v1/statefulsets": 403}, scope: threeWayScope{ScopeType: threeWayScopeCluster, ScopeValue: "cluster"}, wantResources: 2, phase: "discovery"},
		{name: "empty denied discovery", denied: map[string]int{"/apis/apps/v1/deployments": 403, "/apis/apps/v1/statefulsets": 403, "/apis/apps/v1/daemonsets": 403}, scope: threeWayScope{ScopeType: threeWayScopeCluster, ScopeValue: "cluster"}, wantResources: 0, phase: "discovery"},
		{name: "live missing", denied: map[string]int{"/apis/apps/v1/namespaces/team/deployments/app": 404}, scope: threeWayScope{ScopeType: threeWayScopeResource, ScopeValue: "deploy/app"}, wantResources: 1, phase: "live-source-link"},
		{name: "source denied", sourceDenied: true, scope: threeWayScope{ScopeType: threeWayScopeResource, ScopeValue: "deploy/app"}, wantResources: 1, phase: "live-source-link"},
		{name: "pods denied", denied: map[string]int{"/api/v1/namespaces/team/pods": 403}, scope: threeWayScope{ScopeType: threeWayScopeResource, ScopeValue: "deploy/app"}, wantResources: 1, phase: "current-change"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			standaloneThreeWayTest(t)
			alpha, beta := newThreeWayContextFixture(t, "alpha", tc.denied), newThreeWayContextFixture(t, "beta", nil)
			if tc.sourceDenied {
				alpha.sourceStatus = 403
			}
			session := capturedCompareTestSession(t, alpha, beta)
			report, err := collectThreeWayWithSession(context.Background(), session, tc.scope, threeWayOptions{Namespace: "team"})
			require.NoError(t, err)
			require.Len(t, report.Resources, tc.wantResources)
			require.Equal(t, StatePartial, report.Summary.Agreement.State)
			require.NotEmpty(t, report.Omissions)
			require.Equal(t, tc.phase, report.Omissions[0].Phase)
			if tc.name == "pods denied" {
				require.NotNil(t, report.Resources[0].CurrentChange)
				require.Contains(t, strings.Join(report.Resources[0].Result.Notes, " "), "forbidden")
			}
			if tc.sourceDenied {
				require.Equal(t, []string{"alpha:v1"}, report.Resources[0].Result.Live.Images)
				require.NotEmpty(t, report.Resources[0].Result.Notes)
			}
			require.Contains(t, renderThreeWayASCII(report), "Omission:")
			require.Contains(t, renderThreeWayMarkdown(report), "Omission:")
			require.Empty(t, beta.counts())
		})
	}
}

func TestThreeWayCancellationZeroReads(t *testing.T) {
	alpha := newThreeWayContextFixture(t, "alpha", nil)
	session, err := newTraceSession(alpha.config(), "alpha")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = collectThreeWayWithSession(ctx, session, threeWayScope{ScopeType: threeWayScopeCluster, ScopeValue: "cluster"}, threeWayOptions{})
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, alpha.counts())
}

func TestMCPThreeWayActualDispatchAndSelectors(t *testing.T) {
	standaloneThreeWayTest(t)
	alpha, beta := newThreeWayContextFixture(t, "alpha", nil), newThreeWayContextFixture(t, "beta", nil)
	writeThreeWayContextConfig(t, alpha, beta)
	gateway := newMCPGatewayWithMode(func(ctx context.Context, args []string) (string, error) {
		command := &cobra.Command{}
		command.Flags().String("kube-context", "", "")
		scope, err := parseThreeWayScope(args[5])
		if args[4] == "--view" {
			scope = threeWayScope{ScopeType: threeWayScopeView, ScopeValue: args[5]}
		} else {
			require.NoError(t, err)
		}
		selection := clusterContextSelection{}
		for i, arg := range args {
			if arg == "--kube-context" {
				require.NoError(t, command.Flags().Set("kube-context", args[i+1]))
				selection, _ = clusterContextSelectionFromFlag(command)
			}
		}
		report, err := collectThreeWayForSelection(ctx, selection, scope, threeWayOptions{Namespace: "team"})
		if err != nil {
			return "", err
		}
		data, err := json.Marshal(report)
		return string(data), err
	}, nil, true)
	tool := gateway.tools["compare_three_way"]
	props := tool.Descriptor.InputSchema["properties"].(map[string]interface{})
	require.Equal(t, "string", props["context"].(map[string]interface{})["type"])
	call := func(value interface{}) map[string]interface{} {
		raw, err := json.Marshal(map[string]interface{}{"name": "compare_three_way", "arguments": map[string]interface{}{"scope": "deploy/app", "namespace": "team", "context": value}})
		require.NoError(t, err)
		return gateway.callTool(context.Background(), raw)
	}
	for _, selector := range []interface{}{"", " ", 42, nil, "unknown"} {
		require.Equal(t, true, call(selector)["isError"])
	}
	require.Empty(t, alpha.counts())
	require.Empty(t, beta.counts())
	require.Equal(t, false, call("selected")["isError"])
	require.NotEmpty(t, alpha.counts())
	require.Empty(t, beta.counts(), "explicit selection must not reach ambient current-context")
	alpha.labels["confighub.com/UnitSlug"] = "alpha-unit"
	alpha.labels["confighub.com/SpaceID"] = "hub-space"
	installFakeRunner(t, func(_ context.Context, args ...string) ([]byte, error) {
		if args[0] == "view" {
			return []byte(fakeViewJSON("Slug = 'alpha-unit'")), nil
		}
		return []byte(`[{"Slug":"alpha-unit","SpaceID":"hub-space"}]`), nil
	})
	raw, err := json.Marshal(map[string]interface{}{"name": "compare_three_way", "arguments": map[string]interface{}{"view": "806aac53-236c-446d-8ad6-91d6daf6810e", "context": "selected"}})
	require.NoError(t, err)
	require.Equal(t, false, gateway.callTool(context.Background(), raw)["isError"])
	_, err = tool.BuildArgs(map[string]interface{}{"view": "806aac53-236c-446d-8ad6-91d6daf6810e", "scope": "cluster"})
	require.Error(t, err)
	require.Empty(t, beta.counts())

}

func TestThreeWayCLIContextValidationAndOutput(t *testing.T) {
	standaloneThreeWayTest(t)
	resetThreeWayFlags(t)
	alpha, beta := newThreeWayContextFixture(t, "alpha", nil), newThreeWayContextFixture(t, "beta", nil)
	writeThreeWayContextConfig(t, alpha, beta)
	compareThreeWayScopeRaw, combinedNamespace = "deploy/app", "team"
	for _, selector := range []string{"", "unknown"} {
		command := &cobra.Command{}
		command.Flags().String("kube-context", "", "")
		require.NoError(t, command.Flags().Set("kube-context", selector))
		require.Error(t, runCompareThreeWay(command, nil))
	}
	require.Empty(t, alpha.counts())
	require.Empty(t, beta.counts())
	for _, format := range []string{"ascii", "json", "md"} {
		compareThreeWayFormat = format
		command := &cobra.Command{}
		command.Flags().String("kube-context", "", "")
		require.NoError(t, command.Flags().Set("kube-context", "selected"))
		var err error
		output := captureStdout(t, func() { err = runCompareThreeWay(command, nil) })
		require.NoError(t, err)
		require.Contains(t, output, "selected")
		require.Contains(t, output, "alpha:v1")
	}
}

func TestThreeWayTUIActionsCancelStaleAndPreserveSelection(t *testing.T) {
	standaloneThreeWayTest(t)
	alpha, beta := newThreeWayContextFixture(t, "alpha", nil), newThreeWayContextFixture(t, "beta", nil)
	session := capturedCompareTestSession(t, alpha, beta)
	model := LocalClusterModel{width: 100, height: 50, clusterBinding: &localClusterBinding{config: session.config, context: session.context}, traceMode: true, traceItems: []TraceItem{{Kind: "Deployment", Name: "app", Namespace: "team"}}, traceCursor: 0}
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	model = updated.(LocalClusterModel)
	require.NotNil(t, command)
	require.NotNil(t, model.threeWayPane)
	message := command().(threeWayTUIResultMsg)
	updated, _ = model.Update(message)
	model = updated.(LocalClusterModel)
	require.Contains(t, model.renderTrace(), "alpha:v1")
	require.Equal(t, 0, model.traceCursor)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(LocalClusterModel)
	require.Nil(t, model.threeWayPane)
	require.Equal(t, "app", model.traceItems[model.traceCursor].Name)
	// Reopening the same selection must not reuse the canceled pane's ID.
	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	model = updated.(LocalClusterModel)
	require.NotEqual(t, message.requestID, model.threeWayPane.requestID)
	updated, _ = model.Update(message)
	model = updated.(LocalClusterModel)
	require.True(t, model.threeWayPane.loading)
	require.Empty(t, model.threeWayPane.output)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(LocalClusterModel)

	// Exercise every CLI scope in the same standalone pane, including View syntax.
	pane := newThreeWayTUIModel(context.Background(), session, threeWayScope{ScopeType: threeWayScopeResource, ScopeValue: "deploy/app"}, threeWayOptions{Namespace: "team"})
	for _, raw := range []string{"namespace/team", "cluster", "view/806aac53-236c-446d-8ad6-91d6daf6810e", "deploy/app"} {
		next, _ := pane.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
		pane = next.(threeWayTUIModel)
		pane.input = raw
		next, cmd := pane.Update(tea.KeyMsg{Type: tea.KeyEnter})
		pane = next.(threeWayTUIModel)
		require.NotNil(t, cmd)
		require.Equal(t, raw, pane.scope.String())
		oldID := pane.requestID
		next, _ = pane.Update(tea.KeyMsg{Type: tea.KeyEsc})
		pane = next.(threeWayTUIModel)
		next, _ = pane.Update(threeWayTUIResultMsg{requestID: oldID, scope: pane.scope, contextLabel: session.contextLabel(), report: threeWayReport{Scope: "stale"}})
		pane = next.(threeWayTUIModel)
		require.Empty(t, pane.output)
		require.True(t, pane.closed)
		pane.closed = false
	}
	// Parent cancellation/deadline also propagate to collection.
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	pane = newThreeWayTUIModel(ctx, session, threeWayScope{ScopeType: threeWayScopeCluster, ScopeValue: "cluster"}, threeWayOptions{})
	result := pane.start()().(threeWayTUIResultMsg)
	require.ErrorIs(t, result.err, context.DeadlineExceeded)
	require.Empty(t, beta.counts())
}

func writeThreeWayContextConfig(t *testing.T, alpha, beta *compareSessionFixture) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "private-config")
	cfg := api.NewConfig()
	cfg.CurrentContext = "ambient"
	for label, f := range map[string]*compareSessionFixture{"selected": alpha, "ambient": beta} {
		cfg.Contexts[label] = &api.Context{Cluster: label, AuthInfo: label}
		cfg.Clusters[label] = &api.Cluster{Server: f.server.URL, CertificateAuthorityData: f.config().CAData}
		cfg.AuthInfos[label] = &api.AuthInfo{Token: f.marker + "-token"}
	}
	require.NoError(t, clientcmd.WriteToFile(*cfg, path))
	t.Setenv("KUBECONFIG", path)
	return path
}

func TestThreeWayViewCrossSpaceSlugIdentityAndMissingMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, units string
		missing     bool
	}{
		{name: "exact unit ID", units: `[{"UnitID":"unit-a","SpaceID":"space-a","Slug":"same"}]`},
		{name: "exact space slug", units: `[{"SpaceID":"space-a","Slug":"same"}]`},
		{name: "slug alone is insufficient", units: `[{"Slug":"same"}]`, missing: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubConnectedGate(t, nil)
			alpha := newThreeWayContextFixture(t, "alpha", nil, map[string]map[string]string{
				"team":        {"confighub.com/UnitSlug": "same", "confighub.com/SpaceID": "space-a", "confighub.com/UnitID": "unit-a"},
				"kube-system": {"confighub.com/UnitSlug": "same", "confighub.com/SpaceID": "space-b", "confighub.com/UnitID": "unit-b"},
			})
			beta := newThreeWayContextFixture(t, "beta", nil)
			session := capturedCompareTestSession(t, alpha, beta)
			installFakeRunner(t, func(_ context.Context, args ...string) ([]byte, error) {
				if args[0] == "view" {
					return []byte(fakeViewJSON("Slug = 'same'")), nil
				}
				return []byte(tc.units), nil
			})
			targets, omissions, err := collectThreeWayTargetsWithSession(context.Background(), session, threeWayScope{ScopeType: threeWayScopeView, ScopeValue: "806aac53-236c-446d-8ad6-91d6daf6810e"}, "")
			require.NoError(t, err)
			if tc.missing {
				require.Empty(t, targets)
				require.NotEmpty(t, omissions)
				require.Equal(t, "unit_identity_missing", omissions[0].Reason)
			} else {
				require.Equal(t, []threeWayTarget{{ResourceArg: "Deployment/app", Namespace: "team"}}, targets)
				require.Empty(t, omissions)
			}
			require.Empty(t, beta.counts())
		})
	}
}

func TestThreeWayOmittedContextCapturedBeforeViewReadRetargets(t *testing.T) {
	standaloneThreeWayTest(t)
	alpha, beta := newThreeWayContextFixture(t, "alpha", nil), newThreeWayContextFixture(t, "beta", nil)
	beta.labels["confighub.com/UnitSlug"] = "same"
	beta.labels["confighub.com/SpaceID"] = "space-b"
	path := writeThreeWayContextConfig(t, alpha, beta)
	installFakeRunner(t, func(_ context.Context, args ...string) ([]byte, error) {
		if args[0] == "view" {
			cfg, err := clientcmd.LoadFromFile(path)
			require.NoError(t, err)
			cfg.CurrentContext = "selected"
			cfg.Clusters["ambient"].Server = alpha.server.URL
			cfg.Clusters["ambient"].CertificateAuthorityData = alpha.config().CAData
			cfg.AuthInfos["ambient"].Token = "alpha-token"
			require.NoError(t, clientcmd.WriteToFile(*cfg, path))
			return []byte(fakeViewJSON("Slug = 'same'")), nil
		}
		return []byte(`[{"Slug":"same","SpaceID":"space-b"}]`), nil
	})
	report, err := collectThreeWayForSelection(context.Background(), clusterContextSelection{}, threeWayScope{ScopeType: threeWayScopeView, ScopeValue: "806aac53-236c-446d-8ad6-91d6daf6810e"}, threeWayOptions{})
	require.NoError(t, err)
	require.Equal(t, "ambient", report.Context)
	require.Len(t, report.Resources, 2)
	for _, entry := range report.Resources {
		require.Equal(t, []string{"beta:v1"}, entry.Result.Live.Images)
	}
	require.Empty(t, alpha.counts())
	cfg, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(cfg), "current-context: selected")
}

func TestThreeWayInFlightPodCancellationRetainsNoStaleResult(t *testing.T) {
	standaloneThreeWayTest(t)
	alpha, beta := newThreeWayContextFixture(t, "alpha", nil), newThreeWayContextFixture(t, "beta", nil)
	original := alpha.server.Config.Handler
	reached := make(chan struct{})
	alpha.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/pods") {
			close(reached)
			<-r.Context().Done()
			return
		}
		original.ServeHTTP(w, r)
	})
	session := capturedCompareTestSession(t, alpha, beta)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pane := newThreeWayTUIModel(ctx, session, threeWayScope{ScopeType: threeWayScopeResource, ScopeValue: "deploy/app"}, threeWayOptions{Namespace: "team"})
	command := pane.start()
	done := make(chan threeWayTUIResultMsg, 1)
	go func() { done <- command().(threeWayTUIResultMsg) }()
	select {
	case <-reached:
	case <-time.After(3 * time.Second):
		t.Fatal("Pod request did not start")
	}
	updated, _ := pane.Update(tea.KeyMsg{Type: tea.KeyEsc})
	pane = updated.(threeWayTUIModel)
	select {
	case result := <-done:
		require.ErrorIs(t, result.err, context.Canceled)
		updated, _ = pane.Update(result)
		pane = updated.(threeWayTUIModel)
		require.Empty(t, pane.output)
	case <-time.After(3 * time.Second):
		t.Fatal("canceled comparison did not return")
	}
	require.Empty(t, beta.counts())
}

func TestThreeWayConnectedReadsKeepSeparateSpaceAndBindingAuthority(t *testing.T) {
	previousConnected, previousDryWet, previousLink := compareConnectedFn, loadCompareDryWetSnapshotFn, compareLinkRunner
	t.Cleanup(func() {
		compareConnectedFn, loadCompareDryWetSnapshotFn, compareLinkRunner = previousConnected, previousDryWet, previousLink
	})
	compareConnectedFn = func() bool { return true }
	alpha, beta := newThreeWayContextFixture(t, "alpha", nil), newThreeWayContextFixture(t, "beta", nil)
	alpha.labels["confighub.com/UnitSlug"] = "same"
	alpha.labels["confighub.com/SpaceID"] = "hub-space"
	alpha.labels["confighub.com/UnitID"] = "hub-unit"
	session := capturedCompareTestSession(t, alpha, beta)
	hubReads := 0
	loadCompareDryWetSnapshotFn = func(_ context.Context, slug, space string, target compareResourceRef) (compareDryWetResult, error) {
		require.Equal(t, "same", slug)
		require.Equal(t, "hub-space", space)
		require.Equal(t, "team", target.Namespace)
		hubReads++
		side := summarizeCompareManifestObject("dry", threeWayDeploymentDryMap("app", "team", 2))
		return compareDryWetResult{Dry: side, Wet: side}, nil
	}
	compareLinkRunner = func(_ context.Context, args ...string) ([]byte, error) {
		require.Contains(t, strings.Join(args, " "), "FromUnitID = 'hub-unit'")
		require.Contains(t, strings.Join(args, " "), "--space hub-space")
		hubReads++
		return []byte(`[]`), nil
	}
	report, err := collectThreeWayWithSession(context.Background(), session, threeWayScope{ScopeType: threeWayScopeResource, ScopeValue: "deploy/app"}, threeWayOptions{Namespace: "team"})
	require.NoError(t, err)
	require.Equal(t, 2, hubReads)
	require.Equal(t, StateAgreed, report.Summary.Agreement.State)
	// ConfigHub binding refusal must not be treated as complete attribution.
	compareLinkRunner = func(context.Context, ...string) ([]byte, error) {
		return nil, errors.New("recorded ConfigHub denied read")
	}
	report, err = collectThreeWayWithSession(context.Background(), session, threeWayScope{ScopeType: threeWayScopeResource, ScopeValue: "deploy/app"}, threeWayOptions{Namespace: "team"})
	require.NoError(t, err)
	require.Equal(t, StatePartial, report.Summary.Agreement.State)
	require.Empty(t, beta.counts())
}

func TestThreeWayMissingMetadataRetainsUnknownCoverage(t *testing.T) {
	standaloneThreeWayTest(t)
	alpha, beta := newThreeWayContextFixture(t, "alpha", nil), newThreeWayContextFixture(t, "beta", nil)
	alpha.labels = nil
	original := alpha.server.Config.Handler
	alpha.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/deployments/app") {
			original.ServeHTTP(w, r)
			return
		}
		recorder := httptest.NewRecorder()
		original.ServeHTTP(recorder, r)
		var object map[string]interface{}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &object))
		delete(object["spec"].(map[string]interface{}), "selector")
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(object))
	})
	session := capturedCompareTestSession(t, alpha, beta)
	report, err := collectThreeWayWithSession(context.Background(), session, threeWayScope{ScopeType: threeWayScopeResource, ScopeValue: "deploy/app"}, threeWayOptions{Namespace: "team"})
	require.NoError(t, err)
	require.Equal(t, StatePartial, report.Summary.Agreement.State)
	require.Equal(t, []string{"alpha:v1"}, report.Resources[0].Result.Live.Images)
	var sourceUnknown, podsUnknown bool
	for _, omission := range report.Omissions {
		sourceUnknown = sourceUnknown || omission.Phase == "source-coverage"
		podsUnknown = podsUnknown || omission.Phase == "current-change"
	}
	require.True(t, sourceUnknown)
	require.True(t, podsUnknown)
	require.Zero(t, alpha.counts()["/api/v1/namespaces/team/pods"])
	require.Empty(t, beta.counts())
}
