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
		{"--recording", path, "--since", "1h"},
		{"--recording", path, "--count"},
		{"--recording", path, "--names-only"},
		{"--recording", path, "--namespace", "prod", "--namespace-prefix", "pro"},
		{"--recording", path, "--namespace-prefix="},
		{"--recording", path, "--owner="},
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
	for _, args := range []string{`{"namespace":"prod","namespace_prefix":"pro"}`, `{"namespace_prefix":""}`, `{"kind":42}`, `{"recording":"other"}`, `{"context":"live"}`, `{"with_confighub":true}`, `{"query":"@saved"}`, `{"owner":"Flux "}`} {
		result := gateway.callTool(context.Background(), json.RawMessage(`{"name":"map","arguments":`+args+`}`))
		if result["isError"] != true {
			t.Errorf("MCPaccepted%s:%+v", args, result)
		}
	}
}

func TestRecordedMapOwnershipEvidenceConflictsOnlyWithSummary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recording.yaml")
	if err := os.WriteFile(path, []byte(recordedExplainDeployment), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"--recording", path, "--api-version", "apps/v1", "--kind", "Deployment", "--ownership-evidence", "--format", "json"}
	cmd := recordedMapTestCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("full recorded output with ownership evidence was rejected: %v", err)
	}
	var full RecordedMapReport
	if err := json.Unmarshal(out.Bytes(), &full); err != nil {
		t.Fatal(err)
	}
	if full.Schema != "map-list-recorded.v1" || len(full.Resources) != 1 || full.Resources[0].OwnershipDetection == nil {
		t.Fatalf("full recorded output did not preserve detector evidence: %#v", full)
	}

	cmd = recordedMapTestCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs(append(append([]string(nil), args...), "--summary"))
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--ownership-evidence cannot be combined with --summary") {
		t.Fatalf("summary plus ownership evidence should fail clearly, got %v", err)
	}
}

func TestRecordedMapSummaryAndOwnerViewsMatchAcrossSurfaces(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected live read", http.StatusInternalServerError)
	}))
	defer server.Close()
	boundedTestConfig(t, server.URL)
	t.Setenv("CUB_SCOUT_TEST_MAP_ENTRIES_JSON", "/must-not-read-this-live-test-hook")
	path := filepath.Join("..", "..", "evals", "fixtures", "scale", "cluster", "deployments.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := loadRecordedMapTestSnapshot(t, raw)
	fullScope := scaleRecordedMapScope()
	full, err := buildRecordedMapReport(snapshot, fullScope)
	if err != nil {
		t.Fatal(err)
	}
	nativeScope := fullScope
	nativeScope.Owner = "Native"
	native, err := buildRecordedMapReport(snapshot, nativeScope)
	if err != nil {
		t.Fatal(err)
	}
	if native.SelectedCount != 12 || native.ExcludedFromScope != 290 || native.OwnerCounts["Native"] != 12 || len(native.Resources) != 12 {
		t.Fatalf("native owner view = selected %d excluded %d counts %#v rows %d", native.SelectedCount, native.ExcludedFromScope, native.OwnerCounts, len(native.Resources))
	}
	if native.Scope.Owner != "Native" {
		t.Fatalf("owner filter missing from scope: %#v", native.Scope)
	}
	for _, resource := range native.Resources {
		if resource.Owner != "Native" || resource.OwnershipDetection == nil || resource.OwnershipDetection.Status != "no_known_marker" {
			t.Fatalf("Native result contains non-Native evidence: %#v", resource)
		}
	}

	fullSummary, err := buildRecordedMapSummary(snapshot, fullScope)
	if err != nil {
		t.Fatal(err)
	}
	nativeSummary, err := buildRecordedMapSummary(snapshot, nativeScope)
	if err != nil {
		t.Fatal(err)
	}
	if fullSummary.Schema != "map-list-recorded-summary.v1" || fullSummary.View != "summary" || fullSummary.SelectedCount != full.SelectedCount || fullSummary.ExcludedFromScope != full.ExcludedFromScope || !reflect.DeepEqual(fullSummary.OwnerCounts, full.OwnerCounts) {
		t.Fatalf("summary differs from full aggregation: summary=%#v full=%#v", fullSummary, full)
	}
	if nativeSummary.SelectedCount != native.SelectedCount || nativeSummary.ExcludedFromScope != native.ExcludedFromScope || !reflect.DeepEqual(nativeSummary.OwnerCounts, native.OwnerCounts) {
		t.Fatalf("filtered summary differs from filtered full view: %#v vs %#v", nativeSummary, native)
	}
	fullWire, err := json.Marshal(full)
	if err != nil {
		t.Fatal(err)
	}
	summaryWire, err := json.Marshal(fullSummary)
	if err != nil {
		t.Fatal(err)
	}
	nativeWire, err := json.Marshal(native)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaryWire) >= len(fullWire) || len(nativeWire) >= len(fullWire) {
		t.Fatalf("opt-in views did not reduce serialized output bytes: full=%d summary=%d native=%d", len(fullWire), len(summaryWire), len(nativeWire))
	}
	t.Logf("offline serialized payload bytes (not a spend measurement): full=%d summary=%d native=%d", len(fullWire), len(summaryWire), len(nativeWire))
	var summaryFields map[string]json.RawMessage
	if err := json.Unmarshal(summaryWire, &summaryFields); err != nil {
		t.Fatal(err)
	}
	if _, exists := summaryFields["resources"]; exists {
		t.Fatalf("summary schema encodes resource rows: %s", summaryWire)
	}
	if !strings.Contains(string(summaryWire), `"view":"summary"`) || !strings.Contains(string(summaryWire), "perObjectEvidenceGuide") {
		t.Fatalf("summary schema does not identify the omitted-row view: %s", summaryWire)
	}
	var fullFields map[string]json.RawMessage
	if err := json.Unmarshal(fullWire, &fullFields); err != nil {
		t.Fatal(err)
	}
	var fullScopeFields map[string]json.RawMessage
	if err := json.Unmarshal(fullFields["scope"], &fullScopeFields); err != nil {
		t.Fatal(err)
	}
	if _, exists := fullScopeFields["owner"]; exists {
		t.Fatalf("default full response changed by an empty owner field: %s", fullWire)
	}

	// CLI: summary and owner-scoped full output use the same model.
	cmd := recordedMapTestCommand()
	var cliOut bytes.Buffer
	cmd.SetOut(&cliOut)
	cmd.SetArgs([]string{"--recording", path, "--api-version", "apps/v1", "--kind", "Deployment", "--namespace-prefix", "team-", "--summary", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var cliSummary RecordedMapSummary
	if err := json.Unmarshal(cliOut.Bytes(), &cliSummary); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cliSummary, fullSummary) {
		t.Fatalf("CLI summary mismatch: got=%#v want=%#v", cliSummary, fullSummary)
	}
	for _, format := range []string{"ascii", "md"} {
		cmd = recordedMapTestCommand()
		cliOut.Reset()
		cmd.SetOut(&cliOut)
		cmd.SetArgs([]string{"--recording", path, "--api-version", "apps/v1", "--kind", "Deployment", "--namespace-prefix", "team-", "--summary", "--format", format})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		for _, fact := range []string{"View: summary", "Selected: 300", "Native: 12"} {
			if !strings.Contains(strings.ToLower(cliOut.String()), strings.ToLower(fact)) {
				t.Errorf("CLI %s summary missing %q: %s", format, fact, cliOut.String())
			}
		}
		if strings.Contains(cliOut.String(), "team-02/auth") {
			t.Errorf("CLI %s summary emitted a per-object row", format)
		}
	}
	cmd = recordedMapTestCommand()
	cliOut.Reset()
	cmd.SetOut(&cliOut)
	cmd.SetArgs([]string{"--recording", path, "--api-version", "apps/v1", "--kind", "Deployment", "--namespace-prefix", "team-", "--owner", "Native", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var cliNative RecordedMapReport
	if err := json.Unmarshal(cliOut.Bytes(), &cliNative); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cliNative, native) {
		t.Fatalf("CLI owner view mismatch: got=%#v want=%#v", cliNative, native)
	}

	// MCP: matching summary=true and owner arguments return those same shapes.
	gateway := newRecordedMCPGateway(snapshot)
	tool := recordedMapTool(snapshot)
	properties, ok := tool.Descriptor.InputSchema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("recorded map MCP schema has no properties")
	}
	ownerSchema, ok := properties["owner"].(map[string]interface{})
	if !ok || !reflect.DeepEqual(ownerSchema["enum"], recordedMapOwnerNames) {
		t.Fatalf("MCP owner schema does not publish canonical owners: %#v", properties["owner"])
	}
	if summarySchema, ok := properties["summary"].(map[string]interface{}); !ok || summarySchema["type"] != "boolean" {
		t.Fatalf("MCP summary schema is not boolean: %#v", properties["summary"])
	}
	for _, test := range []struct {
		args string
		want interface{}
	}{
		{`{"api_version":"apps/v1","kind":"Deployment","namespace_prefix":"team-","summary":true}`, &fullSummary},
		{`{"api_version":"apps/v1","kind":"Deployment","namespace_prefix":"team-","owner":"Native"}`, &native},
	} {
		result := gateway.callTool(context.Background(), json.RawMessage(`{"name":"map","arguments":`+test.args+`}`))
		if result["isError"] == true {
			t.Fatalf("MCP failed for %s: %#v", test.args, result)
		}
		resultWire, _ := json.Marshal(result)
		var response struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(resultWire, &response); err != nil || len(response.Content) != 1 {
			t.Fatalf("MCP response: %s %v", resultWire, err)
		}
		got := reflect.New(reflect.TypeOf(test.want).Elem()).Interface()
		if err := json.Unmarshal([]byte(response.Content[0].Text), got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Fatalf("MCP view mismatch for %s: got=%#v want=%#v", test.args, got, test.want)
		}
	}
	for _, args := range []string{`{"api_version":"apps/v1","kind":"Deployment","owner":"flux"}`, `{"api_version":"apps/v1","kind":"Deployment","owner":""}`, `{"api_version":"apps/v1","kind":"Deployment","summary":"true"}`} {
		result := gateway.callTool(context.Background(), json.RawMessage(`{"name":"map","arguments":`+args+`}`))
		if result["isError"] != true {
			t.Errorf("MCP accepted invalid view %s: %#v", args, result)
		}
	}

	for _, format := range []string{"ascii", "md"} {
		var out bytes.Buffer
		if err := writeRecordedMapSummary(&out, fullSummary, format); err != nil {
			t.Fatal(err)
		}
		for _, fact := range []string{"View: summary", "Selected: 300", "Native: 12", "per-object detector evidence is omitted"} {
			if !strings.Contains(strings.ToLower(out.String()), strings.ToLower(fact)) {
				t.Errorf("%s summary missing %q: %s", format, fact, out.String())
			}
		}
		if strings.Contains(out.String(), "team-02/auth") {
			t.Errorf("%s summary contains object identity", format)
		}
	}
	viewer := newRecordedMapSummaryViewer(fullSummary)
	if !strings.Contains(viewer.content, "View: summary") || !strings.Contains(strings.ToLower(viewer.content), "per-object detector evidence is omitted") || strings.Contains(viewer.content, "team-02/auth") {
		t.Fatalf("TUI summary content is not a row-free summary: %s", viewer.content)
	}
	if requests.Load() != 0 {
		t.Fatalf("recorded summary/owner views made %d live API requests", requests.Load())
	}
}

func TestRecordedMapOwnerFilterSupportsKubernetesOwnerReferencesAndRejectsUnknownOwner(t *testing.T) {
	const raw = `apiVersion: v1
kind: List
items:
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: child
    namespace: team-a
    ownerReferences:
    - apiVersion: apps/v1
      kind: ReplicaSet
      name: child-123
      uid: uid-123
- apiVersion: apps/v1
  kind: Deployment
  metadata: {name: native, namespace: team-a}
`
	snapshot := loadRecordedMapTestSnapshot(t, []byte(raw))
	scope := RecordedMapScope{APIVersion: "apps/v1", Kind: "Deployment", Owner: "Kubernetes"}
	got, err := buildRecordedMapReport(snapshot, scope)
	if err != nil {
		t.Fatal(err)
	}
	if got.SelectedCount != 1 || got.ExcludedFromScope != 1 || got.OwnerCounts["Kubernetes"] != 1 || got.Resources[0].Name != "child" {
		t.Fatalf("Kubernetes owner-reference selection = %#v", got)
	}
	for _, owner := range []string{"flux", "unknown", "Native "} {
		if err := validateRecordedMapScope(RecordedMapScope{APIVersion: "apps/v1", Kind: "Deployment", Owner: owner}); err == nil {
			t.Errorf("accepted non-canonical/empty recorded owner %q", owner)
		}
	}
}
