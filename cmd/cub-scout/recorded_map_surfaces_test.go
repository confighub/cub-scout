// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func recordedMapTestCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "list", RunE: runMapList}
	addRecordedMapFlags(cmd)
	for _, name := range []string{"kind", "namespace", "kube-context", "owner", "query", "since"} {
		cmd.Flags().String(name, "", "")
	}
	for _, name := range []string{"json", "verbose", "summary", "count", "names-only", "explain", "ownership-evidence"} {
		cmd.Flags().Bool(name, false, "")
	}
	cmd.Flags().String("format", "ascii", "")
	return cmd
}

func TestRecordedMapSurfacesShareModelWithoutLiveReads(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected live read", 500)
	}))
	defer server.Close()
	boundedTestConfig(t, server.URL)
	t.Setenv("CUB_SCOUT_TEST_MAP_ENTRIES_JSON", "/must-not-read-this-live-test-hook")
	path := filepath.Join(t.TempDir(), "recording.yaml")
	if err := os.WriteFile(path, []byte(recordedExplainDeployment), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := readRecordedObjectSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	scope := RecordedMapScope{APIVersion: "apps/v1", Kind: "Deployment", NamespacePrefix: "pro"}
	want, err := buildRecordedMapReport(snapshot, scope)
	if err != nil {
		t.Fatal(err)
	}
	cmd := recordedMapTestCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--recording", path, "--api-version", "apps/v1", "--kind", "Deployment", "--namespace-prefix", "pro", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var got RecordedMapReport
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CLI/model mismatch: got%+v want%+v", got, want)
	}
	gateway := newRecordedMCPGateway(snapshot)
	result := gateway.callTool(context.Background(), json.RawMessage(`{"name":"map","arguments":{"api_version":"apps/v1","kind":"Deployment","namespace_prefix":"pro"}}`))
	if result["isError"] == true {
		t.Fatalf("MCPfailed:%+v", result)
	}
	wire, _ := json.Marshal(result)
	var response struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(wire, &response); err != nil || len(response.Content) != 1 {
		t.Fatalf("MCPresponse:%s %v", wire, err)
	}
	if err := json.Unmarshal([]byte(response.Content[0].Text), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MCP/model mismatch:%+v", got)
	}
	for _, format := range []string{"ascii", "md"} {
		var rendered bytes.Buffer
		if err := writeRecordedMapReport(&rendered, want, format); err != nil {
			t.Fatal(err)
		}
		for _, fact := range []string{want.Provenance.SHA256, "Selected: 1", "excluded by scope: 0", "Capture time: unknown", "capture completeness: unknown", "Native means no built-in owner marker observed", "Helm: 1", "Deployment", "api"} {
			if !strings.Contains(rendered.String(), fact) {
				t.Errorf("%s missing %q", format, fact)
			}
		}
	}
	viewer := newRecordedMapViewer(want)
	if viewer.content != renderRecordedMapReport(want, "ascii") || viewer.Init() != nil {
		t.Fatal("TUI diverges from shared model or starts work")
	}
	updated, cmdEffect := viewer.Update(tea.WindowSizeMsg{Width: 140, Height: 60})
	if cmdEffect != nil || !strings.Contains(updated.View(), "capture completeness: unknown") {
		t.Fatal("TUI lacks visible limits")
	}
	_, quit := updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if quit == nil {
		t.Fatal("TUI has no quit action")
	}
	if requests.Load() != 0 {
		t.Fatalf("recorded surfaces contactedcluster%d times", requests.Load())
	}
	// Startup owns the bytes: changing the file cannot change an existing tool.
	if err := os.WriteFile(path, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	again := gateway.callTool(context.Background(), json.RawMessage(`{"name":"map","arguments":{"api_version":"apps/v1","kind":"Deployment","namespace_prefix":"pro"}}`))
	againWire, _ := json.Marshal(again)
	if string(wire) != string(againWire) {
		t.Fatal("MCP reread mutable file")
	}
}

func TestRecordedMapRejectsLiveOptionsAndInvalidScope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recording.yaml")
	if err := os.WriteFile(path, []byte(recordedExplainDeployment), 0600); err != nil {
		t.Fatal(err)
	}
	cases := [][]string{
		{"--recording", path, "--kube-context", "ignored"},
		{"--recording", path, "--query", "@saved"},
		{"--recording", path, "--owner", "Native"},
		{"--recording", path, "--since", "1h"},
		{"--recording", path, "--summary"},
		{"--recording", path, "--count"},
		{"--recording", path, "--names-only"},
		{"--recording", path, "--namespace", "prod", "--namespace-prefix", "pro"},
		{"--recording", path, "--namespace-prefix="},
		{"--recording", path, "--tui", "--format", "json"},
		{"--recording", path, "--format", "csv"},
		{"--recording", path, "unexpected"},
		{"--recording="},
		{"--api-version", "apps/v1"}, {"--namespace-prefix", "team-"}, {"--tui"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			cmd := recordedMapTestCommand()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(args)
			if err := cmd.Execute(); err == nil {
				t.Fatalf("accepted %v", args)
			}
		})
	}
	snapshot := recordedExplainTestSnapshot(t, recordedExplainDeployment)
	gateway := newRecordedMCPGateway(snapshot)
	for _, args := range []string{`{"namespace":"prod","namespace_prefix":"pro"}`, `{"namespace_prefix":""}`, `{"kind":42}`, `{"recording":"other"}`, `{"context":"live"}`, `{"with_confighub":true}`, `{"query":"@saved"}`, `{"owner":"Native"}`} {
		result := gateway.callTool(context.Background(), json.RawMessage(`{"name":"map","arguments":`+args+`}`))
		if result["isError"] != true {
			t.Errorf("MCPaccepted%s:%+v", args, result)
		}
	}
}
