// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// Replay requests entirely in-process. No listeners, subprocesses or providers.
type explainContextReplay struct {
	t        *testing.T
	requests []string
	denied   string
	first    func()
}

func (f *explainContextReplay) RoundTrip(r *http.Request) (*http.Response, error) {
	f.requests = append(f.requests, r.URL.Host+r.URL.Path)
	if f.first != nil {
		fn := f.first
		f.first = nil
		fn()
	}
	marker := strings.Split(r.URL.Host, ".")[0]
	require.Equal(f.t, "Bearer "+marker+"-token", r.Header.Get("Authorization"))
	status := 200
	var payload interface{}
	path := r.URL.Path
	ns := "team-a"
	if strings.Contains(path, "/namespaces/team-b/") {
		ns = "team-b"
	}
	switch {
	case f.denied != "" && strings.HasSuffix(path, f.denied):
		status = 403
		payload = map[string]interface{}{"apiVersion": "v1", "kind": "Status", "status": "Failure", "reason": "Forbidden", "message": "recorded forbidden", "code": 403}
	case strings.HasSuffix(path, "/deployments/api"):
		payload = map[string]interface{}{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]interface{}{
			"name": "api", "namespace": ns, "uid": marker + "-" + ns, "generation": int64(2),
			"labels":        map[string]interface{}{"argocd.argoproj.io/instance": "api", "confighub.com/UnitSlug": "unit-" + marker, "confighub.com/SpaceName": "space-" + marker},
			"managedFields": []interface{}{map[string]interface{}{"manager": "kubectl-edit", "operation": "Update", "apiVersion": "apps/v1", "fieldsType": "FieldsV1", "fieldsV1": map[string]interface{}{"f:spec": map[string]interface{}{"f:replicas": map[string]interface{}{}}}}},
		}, "spec": map[string]interface{}{"replicas": int64(1), "selector": map[string]interface{}{"matchLabels": map[string]interface{}{"app": "api"}}, "template": map[string]interface{}{"spec": map[string]interface{}{"containers": []interface{}{map[string]interface{}{"name": "api", "image": marker + "/api:1"}}}}}, "status": map[string]interface{}{"observedGeneration": int64(2), "readyReplicas": int64(1), "updatedReplicas": int64(1), "availableReplicas": int64(1)}}
	case strings.HasSuffix(path, "/applications"), strings.HasSuffix(path, "/applications/api"):
		app := map[string]interface{}{"apiVersion": "argoproj.io/v1alpha1", "kind": "Application", "metadata": map[string]interface{}{"name": "api", "namespace": "delivery", "generation": int64(1)}, "spec": map[string]interface{}{"source": map[string]interface{}{"repoURL": "https://" + marker + ".invalid/" + ns, "path": "apps/api", "targetRevision": "main"}, "destination": map[string]interface{}{"namespace": ns}}, "status": map[string]interface{}{"sync": map[string]interface{}{"status": "Synced", "revision": strings.Repeat("a", 40)}, "health": map[string]interface{}{"status": "Healthy"}}}
		payload = app
		if strings.HasSuffix(path, "/applications") {
			payload = map[string]interface{}{"apiVersion": "argoproj.io/v1alpha1", "kind": "ApplicationList", "items": []interface{}{app}}
		}
	case strings.HasSuffix(path, "/events"):
		payload = map[string]interface{}{"apiVersion": "v1", "kind": "EventList", "items": []interface{}{map[string]interface{}{"apiVersion": "v1", "kind": "Event", "metadata": map[string]interface{}{"name": "event-" + marker, "namespace": ns}, "type": "Warning", "reason": "Replay", "message": marker + " " + ns, "count": 1, "involvedObject": map[string]interface{}{"kind": "Deployment", "name": "api", "namespace": ns}}}}
	case strings.HasSuffix(path, "/pods"):
		payload = map[string]interface{}{"apiVersion": "v1", "kind": "PodList", "items": []interface{}{}}
	case strings.HasSuffix(path, "/deployments"):
		payload = map[string]interface{}{"apiVersion": "apps/v1", "kind": "DeploymentList", "items": []interface{}{}}
	default:
		f.t.Errorf("unexpected recorded read %s", r.URL)
		return nil, fmt.Errorf("unexpected recorded read")
	}
	data, err := json.Marshal(payload)
	require.NoError(f.t, err)
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(data))), Request: r}, nil
}

func setupExplainContextReplay(t *testing.T) (*explainContextReplay, string) {
	t.Helper()
	stubConnectedGate(t, fmt.Errorf("recorded disconnected"))
	previousFactory := newExplainSessionForSelection
	f := &explainContextReplay{t: t}
	newExplainSessionForSelection = func(selection clusterContextSelection) (*traceSession, error) {
		s, err := newTraceSessionForSelection(selection)
		if err == nil {
			s.config.WrapTransport = func(http.RoundTripper) http.RoundTripper { return f }
		}
		return s, err
	}
	t.Cleanup(func() { newExplainSessionForSelection = previousFactory })
	path := filepath.Join(t.TempDir(), "config")
	writeTraceKubeconfig(t, path, "beta-context", "https://alpha.invalid", "https://beta.invalid")
	t.Setenv("KUBECONFIG", path)
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")
	return f, path
}

func TestEnrichedExplainCapturedEndpointNamespaceAndCredentials(t *testing.T) {
	f, path := setupExplainContextReplay(t)
	for _, selected := range []string{"alpha", "beta"} {
		for _, ns := range []string{"team-a", "team-b"} {
			session, err := newExplainSessionForSelection(clusterContextSelection{name: selected + "-context", explicit: true})
			require.NoError(t, err)
			f.requests = nil
			f.first = func() { writeTraceKubeconfig(t, path, "beta-context", "https://beta.invalid", "https://beta.invalid") }
			summary, err := observeExplainWithSession(context.Background(), session, "deploy", "api", ns, explainObservationOptions{FieldPath: ".spec.replicas"})
			require.NoError(t, err)
			require.Equal(t, selected+"-context", summary.KubernetesContext)
			require.Equal(t, ns, summary.Namespace)
			require.Equal(t, "Deployment/api", summary.Resource)
			require.Contains(t, summary.Owner, "Argo")
			require.Contains(t, summary.Source, "https://"+selected+".invalid")
			require.NotNil(t, summary.Events)
			require.Equal(t, selected+" "+ns, summary.Events.Events[0].Message)
			require.NotNil(t, summary.CurrentChange)
			require.Equal(t, agent.CauseManualEdit, summary.FieldAttribution.Cause)
			for _, request := range f.requests {
				require.True(t, strings.HasPrefix(request, selected+".invalid"), request)
				require.NotContains(t, request, "/namespaces/default/")
			}
			writeTraceKubeconfig(t, path, "beta-context", "https://alpha.invalid", "https://beta.invalid")
		}
	}
}

func TestEnrichedExplainDenialRemainsUnknownWithoutFallback(t *testing.T) {
	for _, denied := range []string{"/deployments/api", "/applications", "/events", "/pods"} {
		t.Run(denied, func(t *testing.T) {
			f, _ := setupExplainContextReplay(t)
			f.denied = denied
			s, err := newExplainSessionForSelection(clusterContextSelection{name: "alpha-context", explicit: true})
			require.NoError(t, err)
			summary, err := observeExplainWithSession(context.Background(), s, "Deployment", "api", "team-a", explainObservationOptions{FieldPath: ".spec.replicas"})
			require.NoError(t, err)
			require.NotEmpty(t, summary.Omissions)
			if denied == "/deployments/api" {
				require.Equal(t, "Unknown", summary.Owner)
				require.Equal(t, agent.CauseUnknown, summary.FieldAttribution.Cause)
				require.Equal(t, "Unavailable", summary.Health)
			}
			if denied == "/applications" {
				require.Contains(t, summary.Owner, "Argo")
				require.NotEqual(t, "Healthy", summary.Health)
			}
			if denied == "/pods" {
				require.NotNil(t, summary.CurrentChange)
			}
			for _, req := range f.requests {
				require.True(t, strings.HasPrefix(req, "alpha.invalid"), req)
			}
		})
	}
}

func TestEnrichedExplainInvalidSelectionAndCancellationReadNothing(t *testing.T) {
	f, path := setupExplainContextReplay(t)
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	for _, selected := range []string{"", "missing", " "} {
		_, err := newExplainSessionForSelection(clusterContextSelection{name: selected, explicit: true})
		require.Error(t, err)
	}
	_, err = observeExplainWithSession(context.Background(), nil, "Deployment", "api", "team-a", explainObservationOptions{})
	require.Error(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = observeExplainWithSession(ctx, nil, "Deployment", "api", "team-a", explainObservationOptions{})
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, f.requests)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestEnrichedExplainMCPAndTUIShareSelectedModel(t *testing.T) {
	f, _ := setupExplainContextReplay(t)
	tool := newMCPGateway(nil).tools["explain"]
	args, err := tool.BuildArgs(map[string]interface{}{"resource": "Deployment/api", "namespace": "team-a", "context": "alpha-context"})
	require.NoError(t, err)
	require.Contains(t, args, "--kube-context")
	for _, value := range []interface{}{"", " ", 42} {
		_, err := tool.BuildArgs(map[string]interface{}{"resource": "Deployment/api", "context": value})
		require.Error(t, err)
	}
	for _, field := range []string{"api_version", "expected_revision", "refresh"} {
		values := map[string]interface{}{"api_version": "apps/v1", "expected_revision": strings.Repeat("a", 40), "refresh": true}
		_, err := tool.BuildArgs(map[string]interface{}{"resource": "Deployment/api", "context": "alpha-context", field: values[field]})
		require.Error(t, err)
	}
	s, err := newExplainSessionForSelection(clusterContextSelection{name: "alpha-context", explicit: true})
	require.NoError(t, err)
	summary, err := observeExplainWithSession(context.Background(), s, "Deployment", "api", "team-a", explainObservationOptions{})
	require.NoError(t, err)
	summary.KubernetesContext += "\x1b]52;c;injected\x07"
	viewer := newEnrichedExplainViewer(summary)
	require.NotContains(t, viewer.content, "\x1b")
	require.Contains(t, viewer.content, "alpha-context")
	model, _ := viewer.Update(tea.WindowSizeMsg{Width: 40, Height: 15})
	require.IsType(t, enrichedExplainViewer{}, model)
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	require.NotNil(t, cmd)
	require.NotEmpty(t, f.requests)
	for _, hint := range withExplainJSONHints(summary, HintContext{}).NextSteps {
		if hint.NextCommand != "" {
			require.Contains(t, hint.NextCommand, "--kube-context")
		}
	}
}

func TestEnrichedExplainCLIContextAndTUIProjection(t *testing.T) {
	_, _ = setupExplainContextReplay(t)
	oldNamespace, oldFormat, oldContext, oldBounded, oldTUI := explainNamespace, explainFormat, explainContext, explainBounded, explainTUI
	oldField, oldAPI, oldExpected, oldRefresh, oldRecording, oldConnected := explainFieldPath, explainAPIVersion, explainExpectedRevision, explainRefresh, explainRecording, explainWithConfigHub
	t.Cleanup(func() {
		explainNamespace, explainFormat, explainContext, explainBounded, explainTUI = oldNamespace, oldFormat, oldContext, oldBounded, oldTUI
		explainFieldPath, explainAPIVersion, explainExpectedRevision, explainRefresh, explainRecording, explainWithConfigHub = oldField, oldAPI, oldExpected, oldRefresh, oldRecording, oldConnected
	})
	explainNamespace, explainFormat, explainContext = "team-b", "json", "alpha-context"
	explainBounded, explainTUI, explainRefresh, explainWithConfigHub = false, false, false, false
	explainFieldPath, explainAPIVersion, explainExpectedRevision, explainRecording = "", "", "", ""
	command := &cobra.Command{}
	command.SetContext(context.Background())
	command.Flags().String("kube-context", "", "")
	require.NoError(t, command.Flags().Set("kube-context", "alpha-context"))
	var runErr error
	output := captureStdout(t, func() { runErr = runExplain(command, []string{"deploy/api"}) })
	require.NoError(t, runErr)
	var summary ExplainSummary
	require.NoError(t, json.Unmarshal([]byte(output), &summary))
	require.Equal(t, "alpha-context", summary.KubernetesContext)
	require.Equal(t, "team-b", summary.Namespace)
	require.Contains(t, renderExplainText(summary, PresentationMode(""), false, HintContext{}), "alpha-context")
	require.Contains(t, renderExplainMarkdown(summary, PresentationMode(""), false, HintContext{}), "alpha-context")
	gateway := newMCPGateway(func(ctx context.Context, args []string) (string, error) {
		c := &cobra.Command{}
		c.SetContext(ctx)
		c.Flags().StringVar(&explainContext, "kube-context", "", "")
		c.Flags().StringVarP(&explainNamespace, "namespace", "n", "", "")
		c.Flags().StringVar(&explainFormat, "format", "json", "")
		c.Flags().StringVar(&explainFieldPath, "field-path", "", "")
		require.NoError(t, c.Flags().Parse(args[1:]))
		var err error
		out := captureStdout(t, func() { err = runExplain(c, c.Flags().Args()) })
		return out, err
	})
	params, err := json.Marshal(map[string]interface{}{"name": "explain", "arguments": map[string]interface{}{"resource": "deploy/api", "namespace": "team-b", "context": "alpha-context"}})
	require.NoError(t, err)
	result := gateway.callTool(context.Background(), params)
	require.Equal(t, false, result["isError"], "%v", result)
	var mcpSummary ExplainSummary
	require.NoError(t, json.Unmarshal([]byte(result["content"].([]map[string]string)[0]["text"]), &mcpSummary))
	require.Equal(t, summary.KubernetesContext, mcpSummary.KubernetesContext)
	require.Equal(t, summary.Namespace, mcpSummary.Namespace)
	require.Equal(t, summary.Source, mcpSummary.Source)
	oldRun := runEnrichedExplainTUI
	t.Cleanup(func() { runEnrichedExplainTUI = oldRun })
	var tuiSummary ExplainSummary
	runEnrichedExplainTUI = func(s ExplainSummary) error { tuiSummary = s; return nil }
	explainTUI = true
	require.NoError(t, runExplain(command, []string{"deploy/api"}))
	require.Equal(t, summary.Resource, tuiSummary.Resource)
	require.Equal(t, summary.Owner, tuiSummary.Owner)
	require.Equal(t, summary.KubernetesContext, tuiSummary.KubernetesContext)
}

func TestEnrichedExplainConnectedSpaceAndPartialComparison(t *testing.T) {
	f, _ := setupExplainContextReplay(t)
	stubConnectedGate(t, nil)
	oldConnected, oldDryWet := compareConnectedFn, loadCompareDryWetSnapshotFn
	oldRun, oldRequire := runGitOpsCubCommand, requireGitOpsConfigHubFn
	t.Cleanup(func() {
		compareConnectedFn, loadCompareDryWetSnapshotFn = oldConnected, oldDryWet
		runGitOpsCubCommand, requireGitOpsConfigHubFn = oldRun, oldRequire
	})
	compareConnectedFn = func() bool { return true }
	var compared bool
	loadCompareDryWetSnapshotFn = func(ctx context.Context, unit, space string, ref compareResourceRef) (compareDryWetResult, error) {
		compared = true
		require.Equal(t, "unit-alpha", unit)
		require.Equal(t, "space-alpha", space)
		require.Equal(t, "team-b", ref.Namespace)
		return compareDryWetResult{Dry: &compareSideSummary{Kind: ref.Kind, Name: ref.Name, Namespace: ref.Namespace}, Notes: []string{"recorded WET read denied"}}, nil
	}
	requireGitOpsConfigHubFn = func() error { return nil }
	var cubReads int
	runGitOpsCubCommand = func(ctx context.Context, args []string) (string, error) {
		cubReads++
		require.Contains(t, strings.Join(args, " "), "separate-space")
		require.NotContains(t, strings.Join(args, " "), "alpha-context")
		return "[]", nil
	}
	s, err := newExplainSessionForSelection(clusterContextSelection{name: "alpha-context", explicit: true})
	require.NoError(t, err)
	summary, err := observeExplainWithSession(context.Background(), s, "Deployment", "api", "team-b", explainObservationOptions{Delivery: traceConfigHubDeliveryFlags{Enabled: true, Space: "separate-space", Since: "24h", StaleAfter: "15m"}})
	require.NoError(t, err)
	require.True(t, compared)
	require.Equal(t, 3, cubReads)
	require.NotNil(t, summary.DeliveryEvidence)
	require.Equal(t, "separate-space", summary.DeliveryEvidence.Scope.Space)
	require.Nil(t, summary.ThreeWay, "partial operands cannot establish agreement or a disagreement classification")
	require.NotEmpty(t, summary.Omissions)
	require.Contains(t, strings.Join(summary.Notes, " "), "recorded WET read denied")
	for _, request := range f.requests {
		require.True(t, strings.HasPrefix(request, "alpha.invalid"), request)
	}
}
