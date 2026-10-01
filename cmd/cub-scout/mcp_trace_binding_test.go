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
	"sync"
	"testing"

	"github.com/confighub/cub-scout/v2/internal/mapsvc"
	"github.com/stretchr/testify/require"
)

// This injected MCP runner still runs the actual root Cobra parser and the
// command's real runTrace handler. It avoids starting a second test binary;
// runMCPToolCommand's process creation/environment boundary is not exercised.
func mcpTraceParserRunner(t *testing.T, capturedArgs *[]string) mcpToolRunner {
	t.Helper()
	return func(ctx context.Context, args []string) (string, error) {
		*capturedArgs = append([]string(nil), args...)
		rootCmd.SetArgs(args)
		var commandErr error
		output := captureStdout(t, func() { commandErr = rootCmd.ExecuteContext(ctx) })
		return strings.TrimSpace(output), commandErr
	}
}

func mcpTraceCall(gateway *mcpGateway, arguments map[string]interface{}) *mcpResponse {
	params, _ := json.Marshal(map[string]interface{}{"name": "trace", "arguments": arguments})
	return gateway.handleRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/call", Params: params})
}

func setupMCPTraceParserState(t *testing.T) {
	t.Helper()
	prepareTraceSelectionTest(t)
	oldContext := traceKubeContext
	flag := traceCmd.Flags().Lookup("kube-context")
	oldChanged := flag.Changed
	oldRootContext := rootCmd.Context()
	t.Cleanup(func() {
		traceKubeContext = oldContext
		flag.Changed = oldChanged
		rootCmd.SetArgs(nil)
		rootCmd.SetContext(oldRootContext)
	})
	traceApp = ""
	traceNamespace = "delivery"
}

func TestMCPTraceCallRunsParserAndBindsExplicitContext(t *testing.T) {
	setupMCPTraceParserState(t)
	alpha := newMCPTraceBindingServer(t, "alpha")
	beta := newMCPTraceBindingServer(t, "beta")
	configPath := filepath.Join(t.TempDir(), "config")
	before := writeTraceKubeconfig(t, configPath, "beta-context", alpha.URL, beta.URL)
	t.Setenv("KUBECONFIG", configPath)
	var args []string
	gateway := newMCPGateway(mcpTraceParserRunner(t, &args))
	response := mcpTraceCall(gateway, map[string]interface{}{"resource": "application/api", "context": "alpha-context", "namespace": "delivery"})
	require.Nil(t, response.Error)
	wire := decodeMCPResult(t, response.Result)
	require.False(t, wire.IsError, "MCP content: %+v", wire.Content)
	require.Len(t, wire.Content, 1)
	var output mapsvc.TraceOutput
	require.NoError(t, json.Unmarshal([]byte(wire.Content[0].Text), &output))
	require.Equal(t, "alpha-context", output.Context)
	require.Len(t, output.Chain, 2)
	chainJSON, err := json.Marshal(output.Chain)
	require.NoError(t, err)
	require.Contains(t, string(chainJSON), "alpha/repo")
	require.NotContains(t, string(chainJSON), "beta/repo")
	require.Len(t, output.Warnings, 1)
	require.Contains(t, output.Warnings[0], "Events unavailable")
	require.Equal(t, []string{"trace", "application/api", "--kube-context", "alpha-context", "-n", "delivery", "--format", "json"}, args)
	require.Equal(t, 2, alpha.count("GET", "/apis/argoproj.io/v1alpha1/namespaces/delivery/applications/api")) // trace + timing
	require.Equal(t, 1, alpha.count("GET", "/api/v1/namespaces/delivery/events"))

	require.NotContains(t, wire.Content[0].Text, "alpha-token")
	require.NotContains(t, wire.Content[0].Text, "beta-token")
	beta.assertNoRequests(t)
	after, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestMCPTraceCallRejectsInvalidAndMissingContextsWithoutReads(t *testing.T) {
	for _, tc := range []struct {
		name    string
		context interface{}
		fixture string
		want    string
	}{
		{name: "non-string", context: 42, want: "context must be a non-empty kubeconfig context name"},
		{name: "null", context: nil, want: "context must be a non-empty kubeconfig context name"},
		{name: "empty", context: "", want: "context must be a non-empty kubeconfig context name"},
		{name: "blank", context: "  ", want: "context must be a non-empty kubeconfig context name"},
		{name: "unknown context", context: "does-not-exist", want: "does-not-exist"},
		{name: "fixture conflict", context: "alpha-context", fixture: "fixture.json", want: "fixture input"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupMCPTraceParserState(t)
			alpha := newMCPTraceBindingServer(t, "alpha")
			beta := newMCPTraceBindingServer(t, "beta")
			path := filepath.Join(t.TempDir(), "config")
			writeTraceKubeconfig(t, path, "beta-context", alpha.URL, beta.URL)
			t.Setenv("KUBECONFIG", path)
			if tc.fixture != "" {
				t.Setenv("CUB_SCOUT_TEST_TRACE_JSON", tc.fixture)
			}
			var args []string
			gateway := newMCPGateway(mcpTraceParserRunner(t, &args))
			response := mcpTraceCall(gateway, map[string]interface{}{"resource": "application/api", "context": tc.context, "namespace": "delivery"})
			require.Nil(t, response.Error)
			wire := decodeMCPResult(t, response.Result)
			require.True(t, wire.IsError)
			require.Contains(t, strings.Join(contentTexts(wire.Content), "\n"), tc.want)
			require.Equal(t, 0, alpha.requestCount())
			require.Equal(t, 0, beta.requestCount())
		})
	}
}

type mcpTraceResultWire struct {
	IsError bool `json:"isError"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

func decodeMCPResult(t *testing.T, result interface{}) mcpTraceResultWire {
	t.Helper()
	data, err := json.Marshal(result)
	require.NoError(t, err)
	var wire mcpTraceResultWire
	require.NoError(t, json.Unmarshal(data, &wire))
	return wire
}

func contentTexts(content []struct {
	Type string `json:"type"`
	Text string `json:"text"`
}) []string {
	values := make([]string, 0, len(content))
	for _, item := range content {
		values = append(values, item.Text)
	}
	return values
}

type mcpTraceBindingServer struct {
	URL      string
	mu       sync.Mutex
	requests map[string]int
	server   *httptest.Server
}

func newMCPTraceBindingServer(t *testing.T, marker string) *mcpTraceBindingServer {
	t.Helper()
	fixture := &mcpTraceBindingServer{requests: map[string]int{}}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fixture.mu.Lock()
		fixture.requests[r.Method+" "+r.URL.Path]++
		fixture.mu.Unlock()
		switch r.URL.Path {
		case "/apis/argoproj.io/v1alpha1/namespaces/delivery/applications/api":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"apiVersion":"argoproj.io/v1alpha1","kind":"Application","metadata":{"name":"api","namespace":"delivery","uid":"%s-application"},"spec":{"source":{"repoURL":"https://git.example.invalid/%s/repo.git","targetRevision":"%s-revision","path":"."}},"status":{"sync":{"status":"Synced","revision":"%s-revision"},"health":{"status":"Healthy"}}}`, marker, marker, marker, marker)

		case "/api/v1/namespaces/delivery/events":
			writeTraceStatus(w, http.StatusForbidden, "events unavailable")
		default:
			http.NotFound(w, r)
		}
	}))
	fixture.URL = fixture.server.URL
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (f *mcpTraceBindingServer) count(method, path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[method+" "+path]
}
func (f *mcpTraceBindingServer) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	total := 0
	for _, n := range f.requests {
		total += n
	}
	return total
}
func (f *mcpTraceBindingServer) assertNoRequests(t *testing.T) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	require.Empty(t, f.requests)
}
