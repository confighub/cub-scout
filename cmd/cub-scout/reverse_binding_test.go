// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	"k8s.io/client-go/tools/clientcmd"
)

func TestRunReverseTraceWithSessionUsesCapturedServerAfterRetarget(t *testing.T) {
	serverFor := func(owner, uid string) (*httptest.Server, *atomic.Int32) {
		requests := &atomic.Int32{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path != "/apis/apps/v1/namespaces/team-a/deployments/api" {
				http.NotFound(w, r)
				return
			}
			_, _ = fmt.Fprintf(w, `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":"team-a","uid":%q,"labels":{"kustomize.toolkit.fluxcd.io/name":%q,"kustomize.toolkit.fluxcd.io/namespace":"flux-system"}}}`, uid, owner)
		}))
		t.Cleanup(server.Close)
		return server, requests
	}
	alpha, alphaRequests := serverFor("alpha-app", "uid-alpha")
	beta, betaRequests := serverFor("beta-app", "uid-beta")
	path, original := resolverKubeconfig(t, "alpha", map[string]string{"alpha": alpha.URL, "beta": beta.URL})
	config, selected, err := resolveClusterConfig("alpha", true, resolverRules(path), nil)
	if err != nil {
		t.Fatal(err)
	}
	session, err := newTraceSession(config, selected)
	if err != nil {
		t.Fatal(err)
	}
	retargeted, err := clientcmd.LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	retargeted.CurrentContext = "beta"
	if err := clientcmd.WriteToFile(*retargeted, path); err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(original, updated) {
		t.Fatal("test did not retarget the source kubeconfig")
	}

	oldFormat, oldJSON := traceFormat, traceJSON
	traceFormat, traceJSON = "json", false
	t.Cleanup(func() { traceFormat, traceJSON = oldFormat, oldJSON })
	var runErr error
	output := captureStdout(t, func() {
		runErr = runReverseTraceWithSession(context.Background(), session, "Deployment", "api", "team-a")
	})
	if runErr != nil {
		t.Fatalf("runReverseTraceWithSession() error = %v", runErr)
	}
	var result agent.ReverseTraceResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode reverse trace JSON: %v\n%s", err, output)
	}
	if result.Context != "alpha" {
		t.Fatalf("context label = %q, want alpha", result.Context)
	}
	if result.Owner != "flux" || len(result.K8sChain) != 1 {
		t.Fatalf("reverse result came from the wrong server: owner=%q chain=%#v", result.Owner, result.K8sChain)
	}
	if alphaRequests.Load() != 1 || betaRequests.Load() != 0 {
		t.Fatalf("requests alpha=%d beta=%d; want only captured alpha endpoint", alphaRequests.Load(), betaRequests.Load())
	}
	traceFormat, traceJSON = "md", false
	var markdownErr error
	markdown := captureStdout(t, func() {
		markdownErr = runReverseTraceWithSession(context.Background(), session, "Deployment", "api", "team-a")
	})
	if markdownErr != nil || !strings.Contains(markdown, "## Reverse trace:") || !strings.Contains(markdown, "Kubernetes context: alpha") {
		t.Fatalf("Markdown output/error = %q / %v", markdown, markdownErr)
	}
	if alphaRequests.Load() != 2 || betaRequests.Load() != 0 {
		t.Fatalf("after Markdown requests alpha=%d beta=%d; want captured alpha endpoint only", alphaRequests.Load(), betaRequests.Load())
	}
}

func TestRunReverseTraceWithSessionRejectsNilOrInvalidWithoutReads(t *testing.T) {
	for _, session := range []*traceSession{nil, {}} {
		err := runReverseTraceWithSession(context.Background(), session, "Deployment", "api", "team-a")
		if err == nil || !strings.Contains(err.Error(), "trace session") {
			t.Fatalf("runReverseTraceWithSession(%#v) error = %v, want session error", session, err)
		}
	}
	t.Setenv("KUBECONFIG", "/definitely/not/a/real/kubeconfig")
	if err := runReverseTraceWithSession(context.Background(), nil, "Deployment", "api", "team-a"); err == nil || !strings.Contains(err.Error(), "trace session") {
		t.Fatalf("nil session with invalid ambient KUBECONFIG error = %v, want local session failure", err)
	}
}

func TestReverseTraceHumanAndMarkdownKeepPartialChainAndContext(t *testing.T) {
	result := &agent.ReverseTraceResult{
		Object:  agent.ResourceRef{Kind: "Pod", Name: "api-0", Namespace: "team-a"},
		Context: "selected-team",
		Error:   "owner Deployment was not readable",
		K8sChain: []agent.ChainLink{
			{Kind: "Pod", Name: "api-0", Namespace: "team-a", Ready: true},
			{Kind: "ReplicaSet", Name: "api-7f6f9", Namespace: "team-a", Ready: true},
		},
		Owner: "flux",
	}
	var ascii bytes.Buffer
	if err := renderReverseTraceHuman(&ascii, result, false); err != nil {
		t.Fatalf("renderReverseTraceHuman() error = %v", err)
	}
	for _, want := range []string{"selected-team (selection label; not a stable cluster ID)", "owner Deployment was not readable", "Pod", "api-0", "ReplicaSet", "api-7f6f9"} {
		if !strings.Contains(ascii.String(), want) {
			t.Errorf("ASCII reverse trace missing %q:\n%s", want, ascii.String())
		}
	}
	var markdown bytes.Buffer
	if err := renderReverseTraceMarkdown(&markdown, result); err != nil {
		t.Fatalf("renderReverseTraceMarkdown() error = %v", err)
	}
	for _, want := range []string{"## Reverse trace:", "selected-team (selection label; not a stable cluster ID)", "owner Deployment was not readable", "Pod/api-0", "ReplicaSet/api-7f6f9"} {
		if !strings.Contains(markdown.String(), want) {
			t.Errorf("Markdown reverse trace missing %q:\n%s", want, markdown.String())
		}
	}
}

func TestReverseTraceFormatsIncludeContextAdditively(t *testing.T) {
	result := &agent.ReverseTraceResult{
		Object:   agent.ResourceRef{Kind: "Deployment", Name: "api", Namespace: "team-a"},
		Context:  "alpha",
		K8sChain: []agent.ChainLink{{Kind: "Deployment", Name: "api", Namespace: "team-a", Ready: true}},
		Owner:    "native",
	}
	var output bytes.Buffer
	if err := outputReverseTraceJSONTo(&output, result); err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) < 5 || string(fields["context"]) != `"alpha"` {
		t.Fatalf("reverse JSON fields = %s; expected existing model plus context label", output.String())
	}
}

func TestReverseTraceRenderersRejectNilResult(t *testing.T) {
	if err := renderReverseTraceHuman(&bytes.Buffer{}, nil, false); err == nil {
		t.Fatal("human renderer accepted nil result")
	}
	if err := renderReverseTraceMarkdown(&bytes.Buffer{}, nil); err == nil {
		t.Fatal("Markdown renderer accepted nil result")
	}
}
