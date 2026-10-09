// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"os"
	"path/filepath"
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
