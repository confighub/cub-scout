// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRecordedMapJSONBudgetExactBoundaryAndNoClipping(t *testing.T) {
	report := recordedPageReport(t)
	report.Resources[0].Name = "escaped<☃>\u0085"
	page, err := buildRecordedMapPage(report, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := loadRecordedMapTestSnapshot(t, []byte(recordedPageFixture))
	summary, err := buildRecordedMapSummary(snapshot, scaleRecordedMapScope())
	if err != nil {
		t.Fatal(err)
	}
	scaleRaw, err := os.ReadFile(filepath.Join("..", "..", "evals", "fixtures", "scale", "cluster", "deployments.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	scale, err := buildRecordedMapReport(loadRecordedMapTestSnapshot(t, scaleRaw), scaleRecordedMapScope())
	if err != nil {
		t.Fatal(err)
	}
	if scale.SelectedCount != 300 {
		t.Fatal("scale control lost scope")
	}
	for _, model := range []interface{}{report, page, summary, scale} {
		raw, err := json.Marshal(model)
		if err != nil {
			t.Fatal(err)
		}
		if err := checkRecordedMapJSONBudget(model, len(raw)); err != nil {
			t.Fatalf("exact boundary refused: %v", err)
		}
		if err := checkRecordedMapJSONBudget(model, len(raw)-1); err == nil || !strings.Contains(err.Error(), strconv.Itoa(len(raw))) || !strings.Contains(err.Error(), "report data only") {
			t.Fatalf("missing honest oversize refusal: %v", err)
		}
		after, _ := json.Marshal(model)
		if !bytes.Equal(raw, after) {
			t.Fatal("budget clipped report")
		}
	}
	empty := report
	empty.Resources = []RecordedMapResource{}
	empty.SelectedCount = 0
	empty.OwnerCounts = map[string]int{}
	if err := checkRecordedMapJSONBudget(empty, 1); err == nil {
		t.Fatal("empty report bypassed envelope budget")
	}
	if err := checkRecordedMapJSONBudget(report, 0); err != nil {
		t.Fatal("default changed")
	}
}

func TestRecordedMapJSONBudgetCLIFormatsAndEarlyRefusal(t *testing.T) {
	t.Setenv("KUBECONFIG", "/must-not-bind-live-config")
	report := recordedPageReport(t)
	page, _ := buildRecordedMapPage(report, 2, "")
	raw, _ := json.Marshal(page)
	path := filepath.Join(t.TempDir(), "input.yaml")
	if err := os.WriteFile(path, []byte(recordedPageFixture), 0600); err != nil {
		t.Fatal(err)
	}
	base := []string{"--recording", path, "--api-version", "apps/v1", "--kind", "Deployment", "--namespace-prefix", "team-", "--page-size", "2"}
	for _, format := range []string{"ascii", "json", "md"} {
		for _, limit := range []int{len(raw), len(raw) - 1} {
			cmd := recordedMapTestCommand()
			cmd.SilenceUsage = true
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&bytes.Buffer{})
			args := append(append([]string{}, base...), "--format", format, "--max-report-json-bytes", strconv.Itoa(limit))
			cmd.SetArgs(args)
			err := cmd.Execute()
			if limit < len(raw) {
				if err == nil || out.Len() != 0 {
					t.Fatalf("%s wrote refused report (%d bytes): %v", format, out.Len(), err)
				}
				continue
			}
			var want bytes.Buffer
			_ = writeRecordedMapReport(&want, page, format)
			if err != nil || out.String() != want.String() {
				t.Fatalf("%s budgeted output changed: %v", format, err)
			}
			if format == "json" && out.Len() != limit+1 {
				t.Fatal("JSON newline scope not explicit")
			}
		}
	}
	for _, args := range [][]string{{"--max-report-json-bytes", "1"}, {"--recording", "/must-not-read", "--api-version", "apps/v1", "--kind", "Deployment", "--max-report-json-bytes", "0"}, {"--recording", "/must-not-read", "--api-version", "apps/v1", "--kind", "Deployment", "--max-report-json-bytes", strconv.Itoa(recordedMapMaxReportJSONBytes + 1)}} {
		cmd := recordedMapTestCommand()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs(args)
		err := cmd.Execute()
		if err == nil || strings.Contains(err.Error(), "must-not-read") || strings.Contains(err.Error(), "must-not-bind") {
			t.Fatalf("budget refusal after I/O: %v", err)
		}
	}
}

func TestRecordedMapJSONBudgetMCPDataScopeAndStrictArguments(t *testing.T) {
	snapshot := loadRecordedMapTestSnapshot(t, []byte(recordedPageFixture))
	tool := recordedMapTool(snapshot)
	report := recordedPageReport(t)
	page, _ := buildRecordedMapPage(report, 2, "")
	raw, _ := json.Marshal(page)
	args := map[string]interface{}{"api_version": "apps/v1", "kind": "Deployment", "namespace_prefix": "team-", "page_size": 2, "max_report_json_bytes": len(raw)}
	built, err := tool.BuildArgs(args)
	if err != nil {
		t.Fatal(err)
	}
	output, err := tool.Runner(context.Background(), built)
	if err != nil || output != string(raw) {
		t.Fatalf("exact budget: %v", err)
	}
	gateway := newRecordedMCPGateway(snapshot)
	req, _ := json.Marshal(map[string]interface{}{"name": "map", "arguments": args})
	result := gateway.callTool(context.Background(), req)
	wire, _ := json.Marshal(result)
	if result["isError"] == true || len(wire) <= len(raw) {
		t.Fatalf("lost report or confused transport scope: %s", wire)
	}
	args["max_report_json_bytes"] = len(raw) - 1
	req, _ = json.Marshal(map[string]interface{}{"name": "map", "arguments": args})
	result = gateway.callTool(context.Background(), req)
	if result["isError"] != true || result["structuredContent"] != nil {
		t.Fatalf("oversize became inventory: %+v", result)
	}
	for _, value := range []interface{}{true, "2", nil, 0, -1, 1.5, math.NaN(), math.Inf(1), json.Number("2.5"), recordedMapMaxReportJSONBytes + 1} {
		args["max_report_json_bytes"] = value
		if _, err := tool.BuildArgs(args); err == nil {
			t.Errorf("accepted budget %#v", value)
		}
	}
	for _, value := range []interface{}{1, float64(1), json.Number("1"), recordedMapMaxReportJSONBytes} {
		if _, err := recordedMapJSONBudgetArgument(value); err != nil {
			t.Errorf("valid budget refused: %v", err)
		}
	}
}

func TestRecordedMapJSONBudgetTUIRefusalKeepsPageAndCursor(t *testing.T) {
	// A valid long DNS subdomain name makes the next page larger than the first.
	rawFixture := strings.Replace(recordedPageFixture, "name: c,", "name: "+strings.Repeat("c", 253)+",", 1)
	report, err := buildRecordedMapReport(loadRecordedMapTestSnapshot(t, []byte(rawFixture)), scaleRecordedMapScope())
	if err != nil {
		t.Fatal(err)
	}
	first, _ := buildRecordedMapPage(report, 2, "")
	firstRaw, _ := json.Marshal(first)
	viewer, err := newRecordedMapBudgetedPagedViewer(report, 2, "", len(firstRaw))
	if err != nil {
		t.Fatal(err)
	}
	before := viewer.pager
	updated, effect := viewer.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	refused := updated.(recordedMapViewer)
	if effect != nil || refused.pager != before || refused.content != viewer.content || refused.pageError == "" || !strings.Contains(refused.View(), "Page refused:") {
		t.Fatal("oversize advanced page or started I/O")
	}
	if !reflect.DeepEqual(refused.pager.Page, first) {
		t.Fatal("refusal altered report")
	}
	// A larger budget returns the exact next page, without clipping facts.
	pager := *refused.pager
	pager.JSONBudget = recordedMapMaxReportJSONBytes
	refused.pager = &pager
	updated, effect = refused.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	accepted := updated.(recordedMapViewer)
	if effect != nil || accepted.pageError != "" || accepted.pager.Page.Pagination.Offset != 2 || len(accepted.pager.Page.Resources) != 2 {
		t.Fatal("recovery changed navigation")
	}
	if _, err := newRecordedMapBudgetedPagedViewer(report, 2, "", len(firstRaw)-1); err == nil {
		t.Fatal("initial oversize viewer started")
	}
}
