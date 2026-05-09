// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
)

// fakeProjectionRunner builds a viewCubRunner that maps argument-substring
// patterns to JSON responses. Same shape as fakeCubOutput in
// compare_three_way_view_test.go, but kept local to this file so the tests
// stay self-contained.
func fakeProjectionRunner(t *testing.T, responses map[string]string) {
	t.Helper()
	orig := viewCubRunner
	viewCubRunner = func(_ context.Context, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		for pattern, resp := range responses {
			if strings.Contains(joined, pattern) {
				return []byte(resp), nil
			}
		}
		return nil, fmt.Errorf("fakeProjectionRunner: no response for %q", joined)
	}
	t.Cleanup(func() { viewCubRunner = orig })
}

// TestExtractColumnsSpec covers the four ColumnSource shapes the View
// model carries (#391 scope #2).
func TestExtractColumnsSpec(t *testing.T) {
	view := map[string]interface{}{
		"Columns": []interface{}{
			map[string]interface{}{
				"Name": "Slug",
				"ColumnSource": map[string]interface{}{
					"MetadataAttribute": "slug",
				},
			},
			map[string]interface{}{
				"Name": "Replicas",
				"ColumnSource": map[string]interface{}{
					"DataPath": map[string]interface{}{
						"Path": ".spec.replicas",
					},
				},
			},
			map[string]interface{}{
				"Name": "EnvLabel",
				"ColumnSource": map[string]interface{}{
					"MetadataExpression": "labels['env']",
				},
			},
			map[string]interface{}{
				"Name": "DerivedReady",
				"ColumnSource": map[string]interface{}{
					"DataExpression": "spec.replicas > 0",
				},
			},
		},
	}
	cols, err := extractColumnsSpec(view)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cols) != 4 {
		t.Fatalf("expected 4 columns, got %d", len(cols))
	}
	if cols[0].evalKind() != "metadata_attribute" {
		t.Errorf("col 0 kind = %q, want metadata_attribute", cols[0].evalKind())
	}
	if cols[1].evalKind() != "data_path" {
		t.Errorf("col 1 kind = %q, want data_path", cols[1].evalKind())
	}
	if cols[1].DataPath != ".spec.replicas" {
		t.Errorf("col 1 DataPath = %q, want .spec.replicas", cols[1].DataPath)
	}
	if cols[2].evalKind() != "metadata_expression" {
		t.Errorf("col 2 kind = %q, want metadata_expression", cols[2].evalKind())
	}
	if cols[3].evalKind() != "data_expression" {
		t.Errorf("col 3 kind = %q, want data_expression", cols[3].evalKind())
	}
}

// TestEvalCell_MetadataAttribute covers the single evaluator type
// supported in v0.1 of `views project`.
func TestEvalCell_MetadataAttribute(t *testing.T) {
	col := ViewColumnSpec{Name: "Slug", MetadataAttribute: "slug"}
	unit := map[string]interface{}{
		"slug":  "payment-api",
		"name":  "Payment API",
		"value": float64(42),
	}
	got, ok := col.evalCell(unit)
	if !ok {
		t.Fatal("evalCell returned ok=false for supported eval type")
	}
	if got != "payment-api" {
		t.Errorf("got %q, want payment-api", got)
	}

	// PascalCase MetadataAttribute → camelCase JSON field
	col2 := ViewColumnSpec{Name: "Slug", MetadataAttribute: "Slug"}
	got2, _ := col2.evalCell(unit)
	if got2 != "payment-api" {
		t.Errorf("PascalCase fallback: got %q, want payment-api", got2)
	}

	// Numeric value renders without decimal noise
	col3 := ViewColumnSpec{Name: "Value", MetadataAttribute: "value"}
	got3, _ := col3.evalCell(unit)
	if got3 != "42" {
		t.Errorf("numeric render: got %q, want 42", got3)
	}

	// Missing field → empty cell, still supported
	col4 := ViewColumnSpec{Name: "Missing", MetadataAttribute: "nonexistent"}
	got4, ok4 := col4.evalCell(unit)
	if !ok4 {
		t.Error("missing field should still report ok=true (just empty cell)")
	}
	if got4 != "" {
		t.Errorf("missing field: got %q, want empty", got4)
	}
}

// TestEvalCell_PlaceholdersForUnsupportedTypes locks in that v0.1
// renders explicit placeholders for the three not-yet-supported
// evaluator types so column headers still appear and operators see
// the gap rather than getting silent empty cells.
func TestEvalCell_PlaceholdersForUnsupportedTypes(t *testing.T) {
	cases := []struct {
		col      ViewColumnSpec
		wantText string
	}{
		{ViewColumnSpec{Name: "C", MetadataExpression: "labels['x']"}, "<cel: not yet supported>"},
		{ViewColumnSpec{Name: "C", DataPath: ".spec.replicas"}, "<jsonpath: not yet supported>"},
		{ViewColumnSpec{Name: "C", DataExpression: "spec.x > 0"}, "<cel: not yet supported>"},
	}
	for _, tc := range cases {
		got, ok := tc.col.evalCell(map[string]interface{}{})
		if ok {
			t.Errorf("eval for %s should report ok=false until supported", tc.col.evalKind())
		}
		if got != tc.wantText {
			t.Errorf("placeholder for %s: got %q, want %q", tc.col.evalKind(), got, tc.wantText)
		}
	}
}

// TestRenderProjectionTable verifies the ASCII table output shape:
// header line, separator, one row per unit, deterministic column
// widths.
func TestRenderProjectionTable(t *testing.T) {
	pv := ProjectedView{
		UUID:  "806aac53-236c-446d-8ad6-91d6daf6810e",
		Space: "demo",
		Columns: []ViewColumnSpec{
			{Name: "Slug", MetadataAttribute: "slug"},
			{Name: "Owner", MetadataAttribute: "owner"},
		},
		Rows: []projectionRow{
			{"Slug": "payment-api", "Owner": "platform"},
			{"Slug": "checkout-svc", "Owner": "checkout"},
		},
	}
	var buf bytes.Buffer
	if err := renderProjectionTable(&buf, pv); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Slug") {
		t.Error("table missing 'Slug' header")
	}
	if !strings.Contains(out, "payment-api") {
		t.Error("table missing 'payment-api' row")
	}
	if !strings.Contains(out, "checkout-svc") {
		t.Error("table missing 'checkout-svc' row")
	}
	if !strings.Contains(out, "----") {
		t.Error("table missing separator")
	}
}

// TestRenderProjectionTable_NoRows surfaces the empty-result case
// rather than producing a silent header-only output.
func TestRenderProjectionTable_NoRows(t *testing.T) {
	pv := ProjectedView{
		UUID:    "x",
		Columns: []ViewColumnSpec{{Name: "Slug", MetadataAttribute: "slug"}},
		Rows:    []projectionRow{},
	}
	var buf bytes.Buffer
	if err := renderProjectionTable(&buf, pv); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "no matching units") {
		t.Errorf("expected 'no matching units' marker, got: %q", buf.String())
	}
}

// TestListUnitsForFilter_ParsesUnitMetadata covers the projection-
// shaped sibling of listUnitSlugsForFilter — must return the full
// metadata blob (not just slugs) so column evaluators can read
// arbitrary fields.
func TestListUnitsForFilter_ParsesUnitMetadata(t *testing.T) {
	const unitsJSON = `[
		{"slug": "payment-api", "owner": "platform", "labels": {"env": "prod"}},
		{"slug": "checkout-svc", "owner": "checkout", "labels": {"env": "prod"}}
	]`
	fakeProjectionRunner(t, map[string]string{
		"unit list": unitsJSON,
	})
	units, err := listUnitsForFilter(context.Background(), "metadata.labels.env = 'prod'", "*")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(units) != 2 {
		t.Fatalf("expected 2 units, got %d", len(units))
	}
	if units[0]["slug"] != "payment-api" {
		t.Errorf("units[0].slug = %v, want payment-api", units[0]["slug"])
	}
	labels, ok := units[0]["labels"].(map[string]interface{})
	if !ok {
		t.Fatalf("units[0].labels is not a map: %T", units[0]["labels"])
	}
	if labels["env"] != "prod" {
		t.Errorf("units[0].labels.env = %v, want prod", labels["env"])
	}
}

// installFakeWorkloadIndex swaps the buildWorkloadIndexFn seam for the
// test duration so reality-overlay tests don't need a real cluster
// (#391 scope #3).
func installFakeWorkloadIndex(t *testing.T, index map[string]WorkloadInfo) {
	t.Helper()
	orig := buildWorkloadIndexFn
	buildWorkloadIndexFn = func() (map[string]WorkloadInfo, error) {
		return index, nil
	}
	t.Cleanup(func() { buildWorkloadIndexFn = orig })
}

// fakeViewWithColumns builds a minimal `cub view get` JSON blob with
// both Filter.Where and Columns populated.
func fakeViewWithColumns(uuid, whereClause string, columns []map[string]interface{}) string {
	cols := make([]interface{}, len(columns))
	for i, c := range columns {
		cols[i] = c
	}
	view := map[string]interface{}{
		"UUID": uuid,
		"Filter": map[string]interface{}{
			"Where": whereClause,
		},
		"Columns": cols,
	}
	b, _ := json.Marshal(view)
	return string(b)
}

// fakeUnitListJSONFromMaps returns a `cub unit list -o json` blob
// from arbitrary metadata maps. (Distinct from the slug-only helper
// in compare_three_way_view_test.go.)
func fakeUnitListJSONFromMaps(units []map[string]interface{}) string {
	b, _ := json.Marshal(units)
	return string(b)
}

// TestComputeRealityCells covers the four states of the synthetic
// columns: applied + Ready, applied + NotReady, not applied, no slug.
func TestComputeRealityCells(t *testing.T) {
	index := map[string]WorkloadInfo{
		"payment-api":  {UnitSlug: "payment-api", Ready: true, Kind: "Deployment", Name: "payment-api"},
		"checkout-svc": {UnitSlug: "checkout-svc", Ready: false, Kind: "Deployment", Name: "checkout-svc"},
	}

	cases := []struct {
		slug        string
		wantApplied string
		wantStatus  string
	}{
		{"payment-api", "yes", "Ready"},
		{"checkout-svc", "yes", "NotReady"},
		{"orphan-unit", "no", "(not applied)"},
		{"", "?", "(no slug)"},
	}
	for _, tc := range cases {
		t.Run(tc.slug, func(t *testing.T) {
			gotApplied, gotStatus := computeRealityCells(tc.slug, index)
			if gotApplied != tc.wantApplied {
				t.Errorf("Applied? for %q = %q, want %q", tc.slug, gotApplied, tc.wantApplied)
			}
			if gotStatus != tc.wantStatus {
				t.Errorf("LiveStatus for %q = %q, want %q", tc.slug, gotStatus, tc.wantStatus)
			}
		})
	}
}

// TestBuildProjectedView_WithReality exercises the full overlay
// flow: fetch view, list units, build workload index, append
// synthetic columns. Uses both fake seams (cubRunner + workload
// index).
func TestBuildProjectedView_WithReality(t *testing.T) {
	const viewUUID = "806aac53-236c-446d-8ad6-91d6daf6810e"
	const whereClause = "metadata.labels.team = 'payments'"

	fakeProjectionRunner(t, map[string]string{
		"view get " + viewUUID: fakeViewWithColumns(viewUUID, whereClause, []map[string]interface{}{
			{
				"Name": "Slug",
				"ColumnSource": map[string]interface{}{
					"MetadataAttribute": "slug",
				},
			},
		}),
		"unit list": fakeUnitListJSONFromMaps([]map[string]interface{}{
			{"slug": "payment-api"},
			{"slug": "checkout-svc"},
			{"slug": "future-unit"}, // declared in ConfigHub, not yet applied
		}),
	})

	installFakeWorkloadIndex(t, map[string]WorkloadInfo{
		"payment-api":  {UnitSlug: "payment-api", Ready: true},
		"checkout-svc": {UnitSlug: "checkout-svc", Ready: false},
		// future-unit intentionally absent — overlay marks unapplied
	})

	pv, err := buildProjectedView(context.Background(), mockViewRef(viewUUID), "*", true)
	if err != nil {
		t.Fatalf("buildProjectedView failed: %v", err)
	}

	// Columns should include the synthetic reality columns at the end.
	gotColNames := make([]string, len(pv.Columns))
	for i, c := range pv.Columns {
		gotColNames[i] = c.Name
	}
	want := []string{"Slug", "Applied?", "LiveStatus"}
	for i, w := range want {
		if i >= len(gotColNames) || gotColNames[i] != w {
			t.Errorf("columns = %v, want prefix %v", gotColNames, want)
			break
		}
	}

	if len(pv.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(pv.Rows))
	}
	bySlug := map[string]projectionRow{}
	for _, r := range pv.Rows {
		bySlug[r["Slug"]] = r
	}
	if bySlug["payment-api"]["Applied?"] != "yes" || bySlug["payment-api"]["LiveStatus"] != "Ready" {
		t.Errorf("payment-api row = %v", bySlug["payment-api"])
	}
	if bySlug["checkout-svc"]["Applied?"] != "yes" || bySlug["checkout-svc"]["LiveStatus"] != "NotReady" {
		t.Errorf("checkout-svc row = %v", bySlug["checkout-svc"])
	}
	if bySlug["future-unit"]["Applied?"] != "no" || bySlug["future-unit"]["LiveStatus"] != "(not applied)" {
		t.Errorf("future-unit row = %v", bySlug["future-unit"])
	}
}

// TestBuildProjectedView_WithoutReality verifies the default does
// NOT call the workload-index helper and does NOT add synthetic
// columns. Locks in the opt-in behaviour.
func TestBuildProjectedView_WithoutReality(t *testing.T) {
	const viewUUID = "806aac53-236c-446d-8ad6-91d6daf6810e"
	fakeProjectionRunner(t, map[string]string{
		"view get " + viewUUID: fakeViewWithColumns(viewUUID, "metadata.labels.x = 'y'", []map[string]interface{}{
			{"Name": "Slug", "ColumnSource": map[string]interface{}{"MetadataAttribute": "slug"}},
		}),
		"unit list": fakeUnitListJSONFromMaps([]map[string]interface{}{
			{"slug": "payment-api"},
		}),
	})

	called := false
	orig := buildWorkloadIndexFn
	buildWorkloadIndexFn = func() (map[string]WorkloadInfo, error) {
		called = true
		return nil, nil
	}
	t.Cleanup(func() { buildWorkloadIndexFn = orig })

	pv, err := buildProjectedView(context.Background(), mockViewRef(viewUUID), "*", false)
	if err != nil {
		t.Fatalf("buildProjectedView failed: %v", err)
	}
	if called {
		t.Error("buildWorkloadIndexFn was called even without --with-reality")
	}
	for _, c := range pv.Columns {
		if isRealityColumn(c.Name) {
			t.Errorf("synthetic column %q present without --with-reality", c.Name)
		}
	}
}

// mockViewRef is a small helper to construct an agent.ViewRef value
// for tests (the real one is built by ParseViewRef on user input).
func mockViewRef(uuid string) *agent.ViewRef {
	return &agent.ViewRef{UUID: uuid, SourceForm: agent.ViewRefUUID}
}

// TestProjectedViewJSON_RoundTrip locks in the JSON contract shape so
// downstream consumers (the future TUI overlay, Pilot, demo scripts)
// can pin against it.
func TestProjectedViewJSON_RoundTrip(t *testing.T) {
	pv := ProjectedView{
		UUID:  "806aac53",
		Space: "demo",
		Columns: []ViewColumnSpec{
			{Name: "Slug", MetadataAttribute: "slug"},
		},
		Rows: []projectionRow{
			{"Slug": "payment-api"},
		},
	}
	b, err := json.Marshal(pv)
	if err != nil {
		t.Fatal(err)
	}
	var got ProjectedView
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	if got.UUID != pv.UUID || got.Space != pv.Space {
		t.Errorf("round-trip identity lost: %+v", got)
	}
	if len(got.Columns) != 1 || got.Columns[0].MetadataAttribute != "slug" {
		t.Errorf("round-trip columns: %+v", got.Columns)
	}
	if len(got.Rows) != 1 || got.Rows[0]["Slug"] != "payment-api" {
		t.Errorf("round-trip rows: %+v", got.Rows)
	}
}
