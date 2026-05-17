// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// cubUnitListEntry captures the small set of unit list fields that multiple
// connected features depend on. The parser is intentionally tolerant because
// the cub JSON envelope has evolved over time.
type cubUnitListEntry struct {
	UnitSlug        string
	UnitID          string
	SpaceSlug       string
	SpaceID         string
	TargetSlug      string
	HeadRevisionNum int
	LiveRevisionNum int
	Raw             map[string]interface{}
}

func parseCubUnitListJSON(raw []byte) ([]cubUnitListEntry, error) {
	payload, err := parseCubJSONPayload(raw)
	if err != nil {
		return nil, err
	}

	items := cubExtractItems(payload)
	out := make([]cubUnitListEntry, 0, len(items))
	for _, item := range items {
		ref := mcpUnitRefFromItem(item)
		state, _ := mcpExtractUnitRevisionState(item)

		unitObj := mcpNestedMap(item, "Unit", "unit")
		if unitObj == nil {
			unitObj = item
		}

		targetSlug := mcpFirstString(unitObj, "TargetSlug", "targetSlug", "Target", "target")
		if targetSlug == "" {
			if targetObj := mcpNestedMap(item, "Target", "target"); targetObj != nil {
				targetSlug = mcpFirstString(targetObj, "Slug", "slug", "Name", "name")
			}
		}

		out = append(out, cubUnitListEntry{
			UnitSlug:        ref.UnitSlug,
			UnitID:          ref.UnitID,
			SpaceSlug:       ref.SpaceSlug,
			SpaceID:         ref.SpaceID,
			TargetSlug:      targetSlug,
			HeadRevisionNum: state.HeadRevision,
			LiveRevisionNum: state.LiveRevision,
			Raw:             item,
		})
	}
	return out, nil
}

func parseCubTargetListJSON(raw []byte) ([]cubTargetRef, error) {
	payload, err := parseCubJSONPayload(raw)
	if err != nil {
		return nil, err
	}

	items := cubExtractItems(payload)
	out := make([]cubTargetRef, 0, len(items))
	for _, item := range items {
		targetObj := mcpNestedMap(item, "Target", "target")
		if targetObj == nil {
			targetObj = item
		}

		slug := mcpFirstString(targetObj, "Slug", "slug", "Name", "name")
		if slug == "" {
			continue
		}

		out = append(out, cubTargetRef{
			Slug:         slug,
			ProviderType: mcpFirstString(targetObj, "ProviderType", "providerType"),
			Toolchain:    mcpFirstString(targetObj, "ToolchainType", "toolchainType"),
		})
	}
	return out, nil
}

func parseCubWorkerListJSON(raw []byte) ([]WorkerListItem, error) {
	payload, err := parseCubJSONPayload(raw)
	if err != nil {
		return nil, err
	}

	items := cubExtractItems(payload)
	out := make([]WorkerListItem, 0, len(items))
	for _, item := range items {
		workerObj := mcpNestedMap(item, "Worker", "worker")
		if workerObj == nil {
			workerObj = item
		}

		out = append(out, WorkerListItem{
			Name:      mcpFirstString(workerObj, "Slug", "slug", "Name", "name"),
			Cluster:   mcpFirstString(workerObj, "Cluster", "cluster", "BridgeHandle", "bridgeHandle"),
			Condition: mcpFirstString(workerObj, "Condition", "condition", "Status", "status"),
		})
	}
	return out, nil
}

func parseCubContextJSON(raw []byte) (*statusCubContext, error) {
	payload, err := parseCubJSONPayload(raw)
	if err != nil {
		return nil, err
	}

	item, ok := payload.(map[string]interface{})
	if !ok {
		items := cubExtractItems(payload)
		if len(items) == 0 {
			return nil, fmt.Errorf("unexpected cub context payload")
		}
		item = items[0]
	}

	ctx := &statusCubContext{
		Name: mcpFirstString(item, "Name", "name"),
	}

	if coord := mcpNestedMap(item, "Coordinate", "coordinate"); coord != nil {
		ctx.Coordinate.ServerURL = mcpFirstString(coord, "ServerURL", "serverURL")
		ctx.Coordinate.OrganizationID = mcpFirstString(coord, "OrganizationID", "organizationID")
	}
	if settings := mcpNestedMap(item, "Settings", "settings"); settings != nil {
		ctx.Settings.DefaultSpace = mcpFirstString(settings, "DefaultSpace", "defaultSpace")
	}

	if ctx.Name == "" && ctx.Coordinate.ServerURL == "" && ctx.Settings.DefaultSpace == "" {
		return nil, fmt.Errorf("missing expected cub context fields")
	}
	return ctx, nil
}

func parseCubJSONPayload(raw []byte) (interface{}, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, fmt.Errorf("empty cub JSON output")
	}

	var payload interface{}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func cubExtractItems(payload interface{}) []map[string]interface{} {
	switch typed := payload.(type) {
	case []interface{}:
		return historyArrayToObjects(typed)
	case map[string]interface{}:
		for _, key := range []string{
			"items", "Items",
			"data", "Data",
			"results", "Results",
			"units", "Units",
			"targets", "Targets",
			"workers", "Workers",
			"changesets", "ChangeSets",
		} {
			if arr, ok := typed[key].([]interface{}); ok {
				return historyArrayToObjects(arr)
			}
		}
		return []map[string]interface{}{typed}
	default:
		return nil
	}
}
