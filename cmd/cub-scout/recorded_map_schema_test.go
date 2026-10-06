// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"os"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

// The published schema writes the owner names out five times and refuses any
// other. The CI verifier only validates output for owners its fixtures happen
// to contain, so a new owner would break the schema unnoticed. Every copy must
// equal the names the recorded report can emit.
func TestRecordedInventorySchemaOwnersMatchReportOwners(t *testing.T) {
	raw, err := os.ReadFile("../../docs/reference/schemas/recorded-inventory.v1.schema.json")
	require.NoError(t, err)
	var schema struct {
		Defs map[string]struct {
			Properties map[string]struct {
				Enum       []string                   `json:"enum"`
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	require.NoError(t, json.Unmarshal(raw, &schema))

	want := append([]string(nil), recordedMapOwnerNames...)
	sort.Strings(want)
	checked := 0
	for _, site := range []struct{ def, property string }{
		{"scope", "owner"}, {"resource", "owner"},
		{"full", "ownerCounts"}, {"summary", "ownerCounts"}, {"page", "ownerCounts"},
	} {
		property, ok := schema.Defs[site.def].Properties[site.property]
		require.True(t, ok, "schema has no $defs.%s.properties.%s", site.def, site.property)
		got := append([]string(nil), property.Enum...)
		for name := range property.Properties {
			got = append(got, name)
		}
		sort.Strings(got)
		require.Equal(t, want, got, "$defs.%s.properties.%s", site.def, site.property)
		checked++
	}
	require.Equal(t, 5, checked)
}
