// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// recordedUnitList is what `cub unit list -o json` printed on a real ConfigHub
// v0.8.3 (see the fixture's NOTICE): a list of envelopes, each with the Unit's
// own fields under "Unit".
func recordedUnitList(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "confighub-sdk-parity-v083-recorded", "unit-list.json"))
	require.NoError(t, err)
	return string(data)
}

// #852: the view lookups read the list cub really prints. They were written
// and tested against a flat `[{"slug": ...}]`, which cub does not print, so
// every real entry decoded to nothing.
func TestViewLookupsReadTheUnitListCubPrints(t *testing.T) {
	const viewUUID = "806aac53-236c-446d-8ad6-91d6daf6810e"
	fakeProjectionRunner(t, map[string]string{
		"view get " + viewUUID: fakeViewWithColumns(viewUUID, "Slug LIKE 'parity-%'", []map[string]interface{}{
			{"Name": "Slug", "ColumnSource": map[string]interface{}{"MetadataAttribute": "Slug"}},
			{"Name": "As the default column spells it", "ColumnSource": map[string]interface{}{"MetadataAttribute": "slug"}},
			{"Name": "Toolchain", "ColumnSource": map[string]interface{}{"MetadataAttribute": "ToolchainType"}},
			{"Name": "Revision", "ColumnSource": map[string]interface{}{"MetadataAttribute": "HeadRevisionNum"}},
			{"Name": "Not a field", "ColumnSource": map[string]interface{}{"MetadataAttribute": "NoSuchField"}},
			// A Unit has no field called Unit or Space: the envelope's own
			// keys are not the Unit's fields.
			{"Name": "Envelope key", "ColumnSource": map[string]interface{}{"MetadataAttribute": "Space"}},
		}),
		"unit list": recordedUnitList(t),
	})

	slugs, err := listUnitSlugsForFilter(context.Background(), "Slug LIKE 'parity-%'", "*")
	require.NoError(t, err)
	require.Equal(t, []string{"parity-other", "parity-unit"}, slugs)

	installFakeWorkloadIndex(t, map[string]WorkloadInfo{})
	pv, err := buildProjectedView(context.Background(), mockViewRef(viewUUID), "*", true)
	require.NoError(t, err)
	require.Len(t, pv.Rows, 2)
	for i, slug := range []string{"parity-other", "parity-unit"} {
		row := pv.Rows[i]
		require.Equal(t, slug, row["Slug"])
		require.Equal(t, slug, row["As the default column spells it"])
		require.Equal(t, "Kubernetes/YAML", row["Toolchain"])
		require.Equal(t, "2", row["Revision"])
		require.Equal(t, "", row["Not a field"])
		require.Equal(t, "", row["Envelope key"])
		// The reality columns are joined on the Unit's slug, so they need
		// it too: with no workloads known, a Unit with a slug is "not
		// applied", which is a different cell from a Unit with no slug.
		wantApplied, wantStatus := computeRealityCells(slug, map[string]WorkloadInfo{})
		require.Equal(t, wantApplied, row["Applied?"])
		require.Equal(t, wantStatus, row["LiveStatus"])
	}
}

// A field is matched exactly before it is matched without regard to case, and
// the looser match does not depend on map order.
func TestViewUnitFieldMatchesExactlyFirst(t *testing.T) {
	entry := map[string]interface{}{"Unit": map[string]interface{}{"slug": "lower", "Slug": "exact", "SLUG": "upper", "Count": float64(3)}}
	for i := 0; i < 50; i++ {
		value, ok := viewUnitField(entry, "Slug")
		require.True(t, ok)
		require.Equal(t, "exact", value)
		value, ok = viewUnitField(entry, "sLUG")
		require.True(t, ok)
		require.Equal(t, "upper", value, "the first key in sorted order that matches")
	}
	require.Equal(t, "", viewUnitString(entry, "Count"), "a field that is not a string is not a slug")
	_, ok := viewUnitField(entry, "Unit")
	require.False(t, ok)

	// An entry with no Unit object is read as the Unit itself.
	value, ok := viewUnitField(map[string]interface{}{"slug": "flat"}, "Slug")
	require.True(t, ok)
	require.Equal(t, "flat", value)
}

// recordedView is one file of what cub printed for a real View on ConfigHub
// v0.8.3 (see the fixture's NOTICE).
func recordedView(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "confighub-views-v083-recorded", name))
	require.NoError(t, err)
	return string(data)
}

const recordedViewID = "cbfe0f5b-1398-4dab-8f5f-d8fb57993197"

// viewRunner answers `cub view get` with the recorded View and `cub unit list`
// with units, and keeps the arguments of each call.
func viewRunner(t *testing.T, units string) *[][]string {
	t.Helper()
	calls := &[][]string{}
	orig := viewCubRunner
	t.Cleanup(func() { viewCubRunner = orig })
	viewCubRunner = func(_ context.Context, args ...string) ([]byte, error) {
		*calls = append(*calls, args)
		switch strings.Join(args[:2], " ") {
		case "view get":
			return []byte(recordedView(t, "view-get.json")), nil
		case "unit list":
			return []byte(units), nil
		}
		return nil, fmt.Errorf("unexpected cub %s", strings.Join(args, " "))
	}
	return calls
}

// #852: a real View, projected. Its columns are where cub prints them, and
// each cell is the value ConfigHub evaluated for that column.
func TestViewsProjectUsesTheColumnsAndValuesOfARealView(t *testing.T) {
	var view map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(recordedView(t, "view-get.json")), &view))
	columns, err := extractColumnsSpec(view)
	require.NoError(t, err)
	names := []string{}
	for _, column := range columns {
		names = append(names, column.Name)
	}
	wantColumns := []string{"Unit.Slug", "Unit.DisplayName", "Unit.HeadRevisionNum", "Space.Slug", "Labels.tier"}
	require.Equal(t, wantColumns, names)
	where, err := extractWhereClause(view)
	require.NoError(t, err)
	require.Equal(t, "Slug LIKE 'shape-%'", where)

	calls := viewRunner(t, recordedView(t, "unit-list-with-view.json"))
	installFakeWorkloadIndex(t, map[string]WorkloadInfo{})
	pv, err := buildProjectedView(context.Background(), mockViewRef(recordedViewID), "a-space", true)
	require.NoError(t, err)

	// The list is asked for with the View, so that ConfigHub evaluates its
	// columns, and with the View's filter, as before.
	require.Equal(t, []string{"unit", "list", "--space", "a-space", "--where", "Slug LIKE 'shape-%'", "--view", recordedViewID, "-o", "json"}, (*calls)[1])

	names = names[:0]
	for _, column := range pv.Columns {
		names = append(names, column.Name)
	}
	require.Equal(t, append(append([]string{}, wantColumns...), "Applied?", "LiveStatus"), names)
	require.Empty(t, pv.Omissions)
	require.Len(t, pv.Rows, 2)
	const space = "scout-view-shapes-1791576327562898382"
	for i, slug := range []string{"shape-two", "shape-one"} {
		wantApplied, wantStatus := computeRealityCells(slug, map[string]WorkloadInfo{})
		require.Equal(t, projectionRow{
			"Unit.Slug": slug, "Unit.DisplayName": slug, "Unit.HeadRevisionNum": "2", "Space.Slug": space, "Labels.tier": "recorded",
			"Applied?": wantApplied, "LiveStatus": wantStatus,
		}, pv.Rows[i])
	}

	var table bytes.Buffer
	require.NoError(t, renderProjectionTable(&table, pv))
	require.Contains(t, table.String(), "Labels.tier")
	require.Contains(t, table.String(), "recorded")
	require.NotContains(t, table.String(), "has no value")
}

// When ConfigHub evaluates nothing, a column cub-scout cannot work out is
// reported as missing. An empty cell alone would read as an empty value.
func TestViewsProjectSaysWhichColumnsItHasNoValueFor(t *testing.T) {
	// The same Units as cub prints them without --view: no ViewColumns.
	var entries []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(recordedView(t, "unit-list-with-view.json")), &entries))
	plain := make([]map[string]interface{}, len(entries))
	for i, entry := range entries {
		plain[i] = map[string]interface{}{"Unit": entry["Unit"], "Space": entry["Space"]}
	}
	encoded, err := json.Marshal(plain)
	require.NoError(t, err)

	viewRunner(t, string(encoded))
	pv, err := buildProjectedView(context.Background(), mockViewRef(recordedViewID), "a-space", false)
	require.NoError(t, err)
	require.Len(t, pv.Rows, 2)
	require.Len(t, pv.Omissions, 5)
	for i, column := range []string{"Unit.Slug", "Unit.DisplayName", "Unit.HeadRevisionNum", "Space.Slug", "Labels.tier"} {
		require.Equal(t, projectionOmission{Column: column, Reason: "not_evaluated", Units: 2}, pv.Omissions[i])
		require.Equal(t, "", pv.Rows[0][column])
	}
	var table bytes.Buffer
	require.NoError(t, renderProjectionTable(&table, pv))
	require.Contains(t, table.String(), `column "Labels.tier" has no value for 2 unit(s): ConfigHub evaluated no columns`)

	// ConfigHub evaluated the columns but left one out for one Unit, and
	// returned another with an empty value. Only the first is missing.
	first := entries[0]["ViewColumns"].([]interface{})
	entries[0]["ViewColumns"] = append(append([]interface{}{}, first[:3]...), map[string]interface{}{"Name": "Labels.tier"})
	encoded, err = json.Marshal(entries)
	require.NoError(t, err)
	viewRunner(t, string(encoded))
	pv, err = buildProjectedView(context.Background(), mockViewRef(recordedViewID), "a-space", false)
	require.NoError(t, err)
	require.Equal(t, []projectionOmission{{Column: "Space.Slug", Reason: "not_returned", Units: 1}}, pv.Omissions)
	require.Equal(t, "", pv.Rows[0]["Labels.tier"])
	require.Equal(t, "recorded", pv.Rows[1]["Labels.tier"])
	require.Equal(t, "scout-view-shapes-1791576327562898382", pv.Rows[1]["Space.Slug"])

	encodedJSON, err := json.Marshal(pv)
	require.NoError(t, err)
	require.Contains(t, string(encodedJSON), `"omissions":[{"column":"Space.Slug","reason":"not_returned","units":1}]`)
}
