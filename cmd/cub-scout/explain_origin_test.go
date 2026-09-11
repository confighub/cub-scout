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
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func boundedOriginFixture(t *testing.T) *unstructured.Unstructured {
	t.Helper()
	data, err := os.ReadFile("../../examples/bounded-resource-read/origin-deployment.json")
	require.NoError(t, err)
	obj := &unstructured.Unstructured{}
	require.NoError(t, obj.UnmarshalJSON(data))
	return obj
}

func originFixtureSummary(obj *unstructured.Unstructured) ExplainSummary {
	return buildBoundedExplainSummary(obj, agent.BoundedReadEvidence{
		Available: true, Context: "cluster-a", Cache: "miss",
		Resource:   agent.BoundedResourceRef{APIVersion: obj.GetAPIVersion(), Kind: obj.GetKind(), Namespace: obj.GetNamespace(), Name: obj.GetName()},
		ObservedAt: time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC),
	}, nil)
}

func TestBoundedOriginFixtureRenderingAndAuthority(t *testing.T) {
	obj := boundedOriginFixture(t)
	summary := originFixtureSummary(obj)
	require.NotNil(t, summary.ConfigHubOrigin)
	require.EqualValues(t, 7, *summary.ConfigHubOrigin.RevisionNum)
	require.Equal(t, "ArgoCD", summary.Owner, "source metadata does not replace the delivery controller")
	require.NotNil(t, summary.CurrentChange)
	require.NotEqual(t, agent.VerdictPASS, summary.CurrentChange.Verdict)
	require.Nil(t, summary.DeliveryEvidence)
	require.Empty(t, summary.ConfigHubURL, "no guessed server or browser URL")
	for _, rendered := range []string{
		renderExplainText(summary, PresentationHuman, true, HintContext{}),
		renderExplainMarkdown(summary, PresentationHuman, true, HintContext{}),
	} {
		require.Contains(t, rendered, summary.ConfigHubOrigin.Summary())
		require.Contains(t, rendered, "not independently verified")
		require.Contains(t, rendered, "no release, target, component, variant, or cluster binding was verified")
	}
	annotations := obj.GetAnnotations()
	delete(annotations, "argocd.argoproj.io/tracking-id")
	obj.SetAnnotations(annotations)
	require.Equal(t, "Unknown", originFixtureSummary(obj).Owner, "origin alone is not a new ownership detector")
	annotations["confighub.com/SpaceID"] = "conflicting-space"
	obj.SetAnnotations(annotations)
	conflict := originFixtureSummary(obj)
	require.Nil(t, conflict.ConfigHubOrigin)
	require.Equal(t, summary.CurrentChange.Verdict, conflict.CurrentChange.Verdict)
	require.Equal(t, "warning", conflict.Omissions[len(conflict.Omissions)-1].Severity)
	for _, rendered := range []string{
		renderExplainText(conflict, PresentationHuman, true, HintContext{}),
		renderExplainMarkdown(conflict, PresentationHuman, true, HintContext{}),
	} {
		require.Contains(t, rendered, "Origin evidence unavailable")
		require.Contains(t, rendered, "conflicts")
		require.NotContains(t, rendered, "unitId=")
	}
	failure := buildBoundedExplainSummary(obj, agent.BoundedReadEvidence{}, fmt.Errorf("forbidden"))
	require.Nil(t, failure.ConfigHubOrigin, "failed reads cannot expose earlier origin")
}

func TestBoundedOriginTUI(t *testing.T) {
	obj := boundedOriginFixture(t)
	for _, size := range [][2]int{{100, 30}, {40, 12}, {20, 5}} {
		m := LocalClusterModel{ready: true, width: size[0], height: size[1], contextName: "cluster-a", boundedContext: "cluster-a", entries: []MapEntry{
			{APIVersion: obj.GetAPIVersion(), Kind: obj.GetKind(), Namespace: obj.GetNamespace(), Name: obj.GetName()},
		}}
		m.openBoundedExplain()
		m.boundedPanel.observe = func(context.Context, agent.BoundedResourceRef, string, bool) (ExplainSummary, error) {
			return originFixtureSummary(obj), nil
		}
		m.acceptBoundedExplain(m.boundedPanel.read(false)().(boundedExplainMsg))
		require.Contains(t, m.boundedPanel.content, "revisionNum=7")
		require.Contains(t, m.boundedPanel.content, "--kube-context 'cluster-a'")
		lines := strings.Split(ansi.Strip(m.View()), "\n")
		require.LessOrEqual(t, len(lines), size[1])
		for _, line := range lines {
			require.LessOrEqual(t, ansi.StringWidth(line), size[0])
		}
	}
}

func TestBoundedOriginMCPRefreshAndIsolation(t *testing.T) {
	fixture := boundedOriginFixture(t)
	var requests atomic.Int32
	var revision atomic.Int64
	var fail atomic.Bool
	revision.Store(7)
	newServer := func(spaceID string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			w.Header().Set("Content-Type", "application/json")
			if r.Method != http.MethodGet {
				t.Errorf("unexpected method %s", r.Method)
			}
			if r.URL.Path == "/apis/apps/v1" {
				fmt.Fprint(w, `{"groupVersion":"apps/v1","resources":[{"name":"deployments","kind":"Deployment","namespaced":true,"verbs":["get"]}]}`)
				return
			}
			namespace := "team-a"
			if r.URL.Path == "/apis/apps/v1/namespaces/team-b/deployments/api" {
				namespace = "team-b"
			} else if r.URL.Path != "/apis/apps/v1/namespaces/team-a/deployments/api" {
				t.Errorf("unexpected path %s", r.URL.Path)
			}
			if fail.Load() {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			obj := fixture.DeepCopy()
			obj.SetNamespace(namespace)
			annotations := obj.GetAnnotations()
			annotations[agent.ConfigHubOriginAnnotation] = fmt.Sprintf(`{"spaceId":%q,"unitSlug":"api","unitId":%q,"revisionNum":%d}`, spaceID, namespace, revision.Load())
			obj.SetAnnotations(annotations)
			_ = json.NewEncoder(w).Encode(obj)
		}))
	}
	server, other := newServer("space-a"), newServer("space-b")
	defer server.Close()
	defer other.Close()
	configPath := boundedTestConfig(t, server.URL)
	config, err := clientcmd.LoadFromFile(configPath)
	require.NoError(t, err)
	config.Clusters["server-b"] = &clientcmdapi.Cluster{Server: other.URL}
	config.Contexts["cluster-b"].Cluster = "server-b"
	data, err := clientcmd.Write(*config)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, data, 0600))
	noCalls := func(context.Context, []string) (string, error) {
		t.Fatal("no extra subprocess or connected command allowed")
		return "", nil
	}
	gateway := newMCPGatewayWithMode(boundedMCPRunner(noCalls), noCalls, true)
	args := map[string]interface{}{"bounded": true, "resource": "Deployment/api", "api_version": "apps/v1", "context": "cluster-a", "namespace": "team-a"}
	call := func() ExplainSummary {
		params, err := json.Marshal(map[string]interface{}{"name": "explain", "arguments": args})
		require.NoError(t, err)
		result := gateway.callTool(context.Background(), params)
		require.Equal(t, false, result["isError"])
		var summary ExplainSummary
		require.NoError(t, json.Unmarshal([]byte(result["content"].([]map[string]string)[0]["text"]), &summary))
		return summary
	}
	first := call()
	require.Equal(t, "space-a", first.ConfigHubOrigin.SpaceID)
	revision.Store(8)
	hit := call()
	require.Equal(t, first.ConfigHubOrigin, hit.ConfigHubOrigin)
	require.Equal(t, first.ResourceRead.ObservedAt, hit.ResourceRead.ObservedAt)
	require.EqualValues(t, 2, requests.Load())
	args["refresh"] = true
	require.EqualValues(t, 8, *call().ConfigHubOrigin.RevisionNum)
	require.EqualValues(t, 4, requests.Load())
	delete(args, "refresh")
	args["namespace"] = "team-b"
	require.Equal(t, "team-b", call().ConfigHubOrigin.UnitID)
	require.EqualValues(t, 6, requests.Load())
	args["context"] = "cluster-b"
	require.Equal(t, "space-b", call().ConfigHubOrigin.SpaceID)
	require.EqualValues(t, 8, requests.Load())
	revision.Store(9007199254740993)
	args["refresh"] = true
	require.EqualValues(t, 9007199254740993, *call().ConfigHubOrigin.RevisionNum, "MCP JSON preserves the exact revision above 2^53")
	require.EqualValues(t, 10, requests.Load())
	fail.Store(true)
	require.Nil(t, call().ConfigHubOrigin)
	require.EqualValues(t, 12, requests.Load())
	delete(args, "refresh")
	require.Nil(t, call().ConfigHubOrigin, "failed refresh cannot restore cached identity")
	require.EqualValues(t, 14, requests.Load())
}

func TestBoundedOriginCLIAndPlugin(t *testing.T) {
	obj := boundedOriginFixture(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method %s", r.Method)
		}
		switch r.URL.Path {
		case "/apis/apps/v1":
			fmt.Fprint(w, `{"groupVersion":"apps/v1","resources":[{"name":"deployments","kind":"Deployment","namespaced":true,"verbs":["get"]}]}`)
		case "/apis/apps/v1/namespaces/team-a/deployments/api":
			_ = json.NewEncoder(w).Encode(obj)
		default:
			t.Errorf("unexpected read %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	boundedTestConfig(t, server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "cub-scout")
	output, err := exec.CommandContext(ctx, "go", "build", "-o", binary, ".").CombinedOutput()
	require.NoError(t, err, "%s", output)
	for _, plugin := range []string{"", "1"} {
		for _, format := range []string{"json", "ascii", "md"} {
			cmd := exec.CommandContext(ctx, binary, "explain", "Deployment/api", "--namespace", "team-a", "--bounded", "--api-version", "apps/v1", "--kube-context", "cluster-a", "--format", format)
			cmd.Env = append(os.Environ(), "CUB_PLUGIN="+plugin)
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", output)
			if format == "json" {
				var summary ExplainSummary
				require.NoError(t, json.Unmarshal(output, &summary))
				require.NotNil(t, summary.ConfigHubOrigin)
				require.EqualValues(t, 7, *summary.ConfigHubOrigin.RevisionNum)
				require.Equal(t, "ArgoCD", summary.Owner)
				require.Equal(t, agent.BoundedReadCounts{Discovery: 1, Object: 1}, summary.ResourceRead.Reads)
			} else {
				require.Contains(t, string(output), "Origin annotation")
				require.Contains(t, string(output), "revisionNum=7")
			}
		}
	}
	require.EqualValues(t, 12, requests.Load())
}
