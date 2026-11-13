// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"sort"
	"testing"
)

// An agent pays for the whole tool catalog at the start of every session,
// before it has asked anything. These ceilings are the sizes measured on
// 2026-11-13, in bytes of compact JSON as `tools/list` returns it, rounded up
// to the next hundred. A change that grows a tool past its ceiling, or the
// catalog past its total, fails here: raise the number in the same change,
// on purpose, and say why. Shrinking needs no permission; lower the ceiling
// when you do. (Agent plan, package C; four bytes is roughly one token.)
var mcpCatalogToolCeilings = map[string]int{
	"compare_source_truth":      1900,
	"compare_three_way":         1500,
	"confighub_attestations":    600,
	"confighub_changeorder_get": 1000,
	"confighub_changesets":      1000,
	"confighub_k8s_resources":   2100,
	"confighub_k8s_types":       1800,
	"confighub_live_status":     1300,
	"confighub_releases":        900,
	"confighub_resources":       1900,
	"confighub_unit_events":     1000,
	"confighub_unit_get":        1100,
	"confighub_units":           1100,
	"doctor":                    1900,
	"explain":                   3000,
	"gitops_settings":           3000,
	"gitops_status":             2200,
	"map":                       2800,
	"release_check":             2300,
	"scan":                      800,
	"trace":                     1700,
}

// Measured 17,465 bytes for the 8 standalone tools and 33,947 for all 21.
var mcpCatalogTotalCeilings = map[bool]int{false: 17500, true: 34000}

func mcpCatalogSizes(connected bool) (map[string]int, int) {
	gateway := newMCPGatewayWithMode(nil, nil, connected)
	sizes := map[string]int{}
	list := gateway.toolsForList()
	for _, tool := range list {
		raw, _ := json.Marshal(tool)
		sizes[tool.Name] = len(raw)
	}
	raw, _ := json.Marshal(list)
	return sizes, len(raw)
}

func TestMCPCatalogStaysWithinItsDeclaredSize(t *testing.T) {
	for _, connected := range []bool{false, true} {
		sizes, total := mcpCatalogSizes(connected)
		names := make([]string, 0, len(sizes))
		for name := range sizes {
			names = append(names, name)
		}
		sort.Strings(names)
		t.Logf("connected=%v: %d tools, %d bytes", connected, len(names), total)
		for _, name := range names {
			ceiling, declared := mcpCatalogToolCeilings[name]
			switch {
			case !declared:
				t.Errorf("tool %q (%d bytes) declares no size ceiling; add one to mcpCatalogToolCeilings", name, sizes[name])
			case sizes[name] > ceiling:
				t.Errorf("tool %q is %d bytes in the catalog, over its ceiling of %d; shorten it, or raise the ceiling on purpose", name, sizes[name], ceiling)
			}
		}
		if ceiling := mcpCatalogTotalCeilings[connected]; total > ceiling {
			t.Errorf("connected=%v: the catalog is %d bytes, over its ceiling of %d", connected, total, ceiling)
		}
	}
}
