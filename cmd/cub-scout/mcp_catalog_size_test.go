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
// 2026-11-13, after a first shortening, in bytes of compact JSON as `tools/list` returns it, rounded up
// to the next hundred. A change that grows a tool past its ceiling, or the
// catalog past its total, fails here: raise the number in the same change,
// on purpose, and say why. Shrinking needs no permission; lower the ceiling
// when you do. (Agent plan, package C; four bytes is roughly one token.)
var mcpCatalogToolCeilings = map[string]int{
	"compare_source_truth":      1900,
	"compare_three_way":         1400,
	"confighub_attestations":    600,
	"confighub_changeorder_get": 1000,
	"confighub_changesets":      1000,
	"confighub_k8s_resources":   2000,
	"confighub_k8s_types":       1600,
	"confighub_live_status":     1300,
	"confighub_releases":        900,
	"confighub_resources":       1800,
	"confighub_unit_events":     900,
	"confighub_unit_get":        1100,
	"confighub_units":           1100,
	"doctor":                    1700,
	"explain":                   2800,
	"gitops_settings":           2400,
	"gitops_status":             1900,
	"map":                       2300,
	"release_check":             2200,
	"scan":                      700,
	"trace":                     1600,
}

// Measured 15,120 bytes for the 8 standalone tools and 31,126 for all 21,
// after the first shortening (17,465 and 33,947 before it).
var mcpCatalogTotalCeilings = map[bool]int{false: 15200, true: 31200}

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

// The core profile's whole point is its size: 9,292 bytes for the standalone
// session and 12,239 with ConfigHub, measured on 2026-11-13, against 15,120
// and 31,126 for every tool. The dispatcher's own entry and its index are
// held too, since they are the price of reaching the rest.
func TestMCPCoreProfileStaysWithinItsDeclaredSize(t *testing.T) {
	ceilings := map[bool]int{false: 9300, true: 12300}
	for _, connected := range []bool{false, true} {
		gateway := profileGateway(t, "core", connected, nil)
		raw, _ := json.Marshal(gateway.toolsForList())
		t.Logf("core, connected=%v: %d tools, %d bytes", connected, len(gateway.toolsForList()), len(raw))
		if len(raw) > ceilings[connected] {
			t.Errorf("core profile, connected=%v: %d bytes, over its ceiling of %d", connected, len(raw), ceilings[connected])
		}
	}
	more, _ := json.Marshal(mcpMoreToolDescriptor())
	if len(more) > 900 {
		t.Errorf("cub_scout_more is %d bytes in the catalog, over its ceiling of 900", len(more))
	}
	index, _ := callText(t, profileGateway(t, "core", true, nil), mcpMoreToolName, nil)
	t.Logf("cub_scout_more: %d bytes in the catalog; connected index %d bytes", len(more), len(index))
	if len(index) > 2800 {
		t.Errorf("the connected index is %d bytes, over its ceiling of 2800", len(index))
	}
}
