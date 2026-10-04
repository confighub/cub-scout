// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

const recordedPageFixture = `apiVersion: v1
kind: List
items:
- apiVersion: apps/v1
  kind: Deployment
  metadata: {name: e, namespace: team-b}
- apiVersion: apps/v1
  kind: Deployment
  metadata: {name: c, namespace: team-a}
- apiVersion: apps/v1
  kind: Deployment
  metadata: {name: a, namespace: team-a, labels: {app.kubernetes.io/managed-by: Helm}}
- apiVersion: apps/v1
  kind: Deployment
  metadata: {name: d, namespace: team-b}
- apiVersion: apps/v1
  kind: Deployment
  metadata: {name: b, namespace: team-a, labels: {kustomize.toolkit.fluxcd.io/name: apps, kustomize.toolkit.fluxcd.io/namespace: flux-system}}
- apiVersion: v1
  kind: ConfigMap
  metadata: {name: excluded, namespace: team-a}
`

func recordedPageReport(t *testing.T) RecordedMapReport {
	t.Helper()
	report, err := buildRecordedMapReport(loadRecordedMapTestSnapshot(t, []byte(recordedPageFixture)), scaleRecordedMapScope())
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func TestRecordedMapPagesPreserveFullScopeAndPartition(t *testing.T) {
	report := recordedPageReport(t)
	original, _ := json.Marshal(report)
	var union []RecordedMapResource
	cursor := ""
	for i, wantCount := range []int{2, 2, 1} {
		page, err := buildRecordedMapPage(report, 2, cursor)
		if err != nil {
			t.Fatal(err)
		}
		if page.Schema != recordedMapPageSchema || page.Pagination.Offset != i*2 || page.Pagination.ReturnedCount != wantCount || len(page.Resources) != wantCount {
			t.Fatalf("bad page: %+v", page)
		}
		if page.SelectedCount != 5 || page.ExcludedFromScope != 1 || !reflect.DeepEqual(page.OwnerCounts, report.OwnerCounts) || !reflect.DeepEqual(page.Provenance, report.Provenance) || page.Provenance.CaptureTime != "unknown" || page.Provenance.CaptureCompleteness != "unknown" {
			t.Fatalf("lost full-scope facts: %+v", page)
		}
		union = append(union, page.Resources...)
		cursor = page.Pagination.NextCursor
		if (i < 2) != (cursor != "") {
			t.Fatal("incorrect continuation")
		}
	}
	if !reflect.DeepEqual(union, report.Resources) {
		t.Fatal("pages do not partition full report")
	}
	after, _ := json.Marshal(report)
	if !bytes.Equal(original, after) || bytes.Contains(original, []byte(`"pagination"`)) {
		t.Fatal("default report mutated or wire contract changed")
	}
	page, _ := buildRecordedMapPage(report, 500, "")
	page.Resources[0].Name = "changed"
	if report.Resources[0].Name == "changed" {
		t.Fatal("page aliases source resource slice")
	}
	empty := report
	empty.Resources = nil
	empty.SelectedCount = 0
	page, err := buildRecordedMapPage(empty, 2, "")
	if err != nil || page.Resources == nil || len(page.Resources) != 0 || page.Pagination.NextCursor != "" {
		t.Fatalf("empty page: %+v %v", page, err)
	}
}

func TestRecordedMapPageCursorRefusals(t *testing.T) {
	report := recordedPageReport(t)
	good := encodeRecordedMapCursor(report, 2, 2)
	parsed, err := decodeRecordedMapCursor(good)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"version", "input", "scope", "size", "zero", "unaligned", "end", "negative"} {
		changed := parsed
		switch field {
		case "version":
			changed.Version = 2
		case "input":
			changed.InputSHA256 = strings.Repeat("0", 64)
		case "scope":
			changed.ScopeSHA256 = strings.Repeat("0", 64)
		case "size":
			changed.PageSize = 3
		case "zero":
			changed.Offset = 0
		case "unaligned":
			changed.Offset = 1
		case "end":
			changed.Offset = 6
		case "negative":
			changed.Offset = -2
		}
		raw, _ := json.Marshal(changed)
		if _, err := buildRecordedMapPage(report, 2, base64.RawURLEncoding.EncodeToString(raw)); err == nil {
			t.Errorf("accepted %s", field)
		}
	}
	raw, _ := json.Marshal(parsed)
	malformed := []string{"!", strings.Repeat("A", 2049), base64.RawURLEncoding.EncodeToString(append(raw, ' ')), base64.RawURLEncoding.EncodeToString([]byte(strings.Replace(string(raw), `"version":1`, `"version":1,"unknown":true`, 1))), base64.RawURLEncoding.EncodeToString([]byte(strings.Replace(string(raw), `"version":1`, `"version":1,"version":1`, 1))), base64.RawURLEncoding.EncodeToString([]byte(strings.Replace(string(raw), `"version"`, `"Version"`, 1))), base64.RawURLEncoding.EncodeToString([]byte(strings.Replace(string(raw), `"offset":2`, `"offset":2.5`, 1)))}
	for _, value := range malformed {
		if _, err := buildRecordedMapPage(report, 2, value); err == nil {
			t.Error("accepted malformed cursor")
		}
	}
	for _, size := range []int{-1, 0, 501} {
		if _, err := buildRecordedMapPage(report, size, ""); err == nil {
			t.Errorf("accepted size %d", size)
		}
	}
	for _, value := range []interface{}{true, "2", nil, 1.5, math.NaN(), math.Inf(1), json.Number("2.5"), 0, 501} {
		if _, err := recordedMapPageSizeArgument(value); err == nil {
			t.Errorf("accepted size %#v", value)
		}
	}
	for _, value := range []interface{}{2, float64(2), json.Number("2")} {
		if size, err := recordedMapPageSizeArgument(value); err != nil || size != 2 {
			t.Errorf("valid size refused: %v", err)
		}
	}
	changed := report
	changed.Scope.Owner = "Helm"
	if _, err := buildRecordedMapPage(changed, 2, good); err == nil {
		t.Fatal("accepted changed filter")
	}
	page, _ := buildRecordedMapPage(report, 2, "")
	if _, err := buildRecordedMapPage(page, 2, ""); err == nil {
		t.Fatal("repaginated partial report")
	}
}

func TestRecordedMapPageTUIContinuesLoadedModel(t *testing.T) {
	report := recordedPageReport(t)
	viewer, err := newRecordedMapPagedViewer(report, 2, encodeRecordedMapCursor(report, 2, 4))
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		key    rune
		offset int
	}{{'n', 4}, {'p', 2}, {'p', 0}, {'p', 0}, {'n', 2}, {'n', 4}} {
		model, effect := viewer.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{step.key}})
		if effect != nil {
			t.Fatal("pagination started I/O")
		}
		viewer = model.(recordedMapViewer)
		if viewer.pager.Page.Pagination.Offset != step.offset || viewer.content != renderRecordedMapReport(viewer.pager.Page, "ascii") {
			t.Fatal("TUI page diverged")
		}
	}
	if !strings.Contains(viewer.View(), "n/p") {
		t.Fatal("missing pagination help")
	}
}

func TestRecordedMapPageCLIAndMCP(t *testing.T) {
	t.Setenv("KUBECONFIG", "/must-not-bind-live-config")
	report := recordedPageReport(t)
	path := filepath.Join(t.TempDir(), "input.yaml")
	if err := os.WriteFile(path, []byte(recordedPageFixture), 0600); err != nil {
		t.Fatal(err)
	}
	expected, _ := buildRecordedMapPage(report, 2, "")
	for _, format := range []string{"ascii", "json", "md"} {
		cmd := recordedMapTestCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"--recording", path, "--api-version", "apps/v1", "--kind", "Deployment", "--namespace-prefix", "team-", "--page-size", "2", "--format", format})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		var want bytes.Buffer
		if err := writeRecordedMapReport(&want, expected, format); err != nil {
			t.Fatal(err)
		}
		if out.String() != want.String() {
			t.Fatalf("%s CLI mismatch", format)
		}
	}
	gateway := newRecordedMCPGateway(loadRecordedMapTestSnapshot(t, []byte(recordedPageFixture)))
	cursor := ""
	for _, offset := range []int{0, 2, 4} {
		args := map[string]interface{}{"api_version": "apps/v1", "kind": "Deployment", "namespace_prefix": "team-", "page_size": 2}
		if cursor != "" {
			args["cursor"] = cursor
		}
		request, _ := json.Marshal(map[string]interface{}{"name": "map", "arguments": args})
		result := gateway.callTool(context.Background(), request)
		wire, _ := json.Marshal(result)
		var response struct {
			Structured struct {
				Data RecordedMapReport `json:"data"`
			} `json:"structuredContent"`
		}
		if err := json.Unmarshal(wire, &response); err != nil {
			t.Fatal(err)
		}
		page := response.Structured.Data
		if result["isError"] == true || page.Pagination == nil || page.Pagination.Offset != offset {
			t.Fatalf("MCP page: %s", wire)
		}
		cursor = page.Pagination.NextCursor
	}
	for _, extra := range []map[string]interface{}{{"cursor": "bad"}, {"page_size": 1.5}, {"page_size": 2, "summary": true}, {"page_size": 2, "cursor": ""}, {"page_size": 2, "surprise": true}} {
		args := map[string]interface{}{"api_version": "apps/v1", "kind": "Deployment"}
		for k, v := range extra {
			args[k] = v
		}
		req, _ := json.Marshal(map[string]interface{}{"name": "map", "arguments": args})
		if result := gateway.callTool(context.Background(), req); result["isError"] != true {
			t.Fatalf("accepted invalid MCP args: %+v", args)
		}
	}
	// Canceled callers and unknown tools cannot start pagination work.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tool := recordedMapTool(loadRecordedMapTestSnapshot(t, []byte(recordedPageFixture)))
	args, err := tool.BuildArgs(map[string]interface{}{"api_version": "apps/v1", "kind": "Deployment", "page_size": 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tool.Runner(ctx, args); err != context.Canceled {
		t.Fatalf("canceled runner: %v", err)
	}
	if result := gateway.callTool(context.Background(), json.RawMessage(`{"name":"unknown-page-tool","arguments":{"page_size":2}}`)); result["isError"] != true {
		t.Fatal("unknown tool accepted")
	}
	// The gateway holds startup bytes, and continuation never rereads the path.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := recordedMapTool(loadRecordedMapTestSnapshot(t, []byte(recordedPageFixture))).Runner(context.Background(), []string{`{"scope":{"apiVersion":"apps/v1","kind":"Deployment"},"summary":true,"pageSize":2}`}); err == nil {
		t.Fatal("runner ignored incompatible page options")
	}
}

func TestRecordedMapPageCLIRefusesBeforeBindingOrFileRead(t *testing.T) {
	t.Setenv("KUBECONFIG", "/must-not-bind-live-config")
	for _, args := range [][]string{{"--page-size", "2"}, {"--cursor", "bad"}, {"--recording", "/must-not-read", "--api-version", "apps/v1", "--kind", "Deployment", "--page-size", "2", "--summary"}, {"--recording", "/must-not-read", "--api-version", "apps/v1", "--kind", "Deployment", "--cursor", "bad"}, {"--recording", "/must-not-read", "--api-version", "apps/v1", "--kind", "Deployment", "--page-size", "0"}} {
		cmd := recordedMapTestCommand()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs(args)
		err := cmd.Execute()
		if err == nil || strings.Contains(err.Error(), "must-not-read") || strings.Contains(err.Error(), "must-not-bind") {
			t.Fatalf("refusal after I/O: %v", err)
		}
	}
}
