// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// This file contains the MCP "structuredContent" layer: the logic that
// augments a tool's JSON output with a normalized `data` envelope plus
// canonical ConfigHub trust URLs and structured next-step hints.
//
// It is split out of mcp.go to keep the gateway plumbing (request/response,
// tool registry, argument validation, stdio framing) separate from the
// JSON extraction and hint assembly helpers. Adding a new structured-output
// tool should only require a case in buildMCPStructuredContent and a new
// buildMCP*StructuredContent helper here.

package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// buildMCPStructuredContent is the dispatch entry point called by the gateway
// in mcp.go. It parses the tool's JSON output and, for tools that support a
// structuredContent envelope, builds a normalized map with:
//
//   - data:                  the original payload
//   - nextSteps:              ordered [StructuredHint] next-step hints
//   - confighubUrl:           canonical unit detail URL, when resolvable
//   - confighubRevisionsUrl:  canonical revision-history URL, when resolvable
//
// Tools that do not need structured content return nil, in which case the
// gateway falls back to plain text content only.
func buildMCPStructuredContent(toolName, output string) interface{} {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return nil
	}

	var payload interface{}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return nil
	}

	switch toolName {
	case "compare_three_way":
		return mcpWrapStructuredData(payload)
	case "confighub_units":
		return buildMCPUnitsStructuredContent(payload)
	case "confighub_unit_get":
		return buildMCPUnitGetStructuredContent(payload)
	case "confighub_changesets":
		return buildMCPChangesetsStructuredContent(payload)
	default:
		return nil
	}
}

// mcpWrapStructuredData produces the base envelope for a tool output that
// already carries its own nextSteps / confighubUrl / confighubRevisionsUrl
// fields (e.g. compare_three_way). It promotes those fields to the top of
// the envelope so MCP clients can read them without re-parsing `data`.
func mcpWrapStructuredData(payload interface{}) map[string]interface{} {
	wrapped := map[string]interface{}{
		"data": payload,
	}
	if obj, ok := payload.(map[string]interface{}); ok {
		if nextSteps, ok := obj["nextSteps"]; ok {
			wrapped["nextSteps"] = nextSteps
		}
		if url, ok := obj["confighubUrl"]; ok {
			wrapped["confighubUrl"] = url
		}
		if url, ok := obj["confighubRevisionsUrl"]; ok {
			wrapped["confighubRevisionsUrl"] = url
		}
	}
	return wrapped
}

func buildMCPUnitsStructuredContent(payload interface{}) interface{} {
	wrapped := mcpWrapStructuredData(payload)
	ref, ok := mcpExtractUnitRef(payload)
	if !ok {
		return wrapped
	}

	detailURL := mcpUnitDetailURL(ref)
	revisionsURL := mcpUnitRevisionsURL(ref)
	state, _ := mcpExtractUnitRevisionState(payload)
	hints := make([]Hint, 0, 3)
	if revisionHint := mcpUnitRevisionHint(state, revisionsURL, false); revisionHint != nil {
		hints = append(hints, *revisionHint)
	}
	hints = append(hints, Hint{
		Command:      mcpUnitGetCommand(ref),
		Rationale:    "Inspect the first returned ConfigHub unit to get exact revision and live-state facts.",
		ConfigHubURL: detailURL,
		Priority:     hintPriorityHigh,
		ActionType:   ActionReadOnly,
	})
	sortHints(hints)
	wrapped["nextSteps"] = HintsToStructured(hints)
	if detailURL != "" {
		wrapped["confighubUrl"] = detailURL
	}
	if revisionsURL != "" {
		wrapped["confighubRevisionsUrl"] = revisionsURL
	}
	return wrapped
}

func buildMCPUnitGetStructuredContent(payload interface{}) interface{} {
	wrapped := mcpWrapStructuredData(payload)
	ref, ok := mcpExtractUnitRef(payload)
	if !ok {
		return wrapped
	}

	detailURL := mcpUnitDetailURL(ref)
	revisionsURL := mcpUnitRevisionsURL(ref)
	state, _ := mcpExtractUnitRevisionState(payload)
	hints := make([]Hint, 0, 3)
	if revisionHint := mcpUnitRevisionHint(state, revisionsURL, true); revisionHint != nil {
		hints = append(hints, *revisionHint)
	}
	if detailURL != "" {
		hints = append(hints, Hint{
			Rationale:    "Open the exact ConfigHub unit to review current facts and trust surfaces.",
			ConfigHubURL: detailURL,
			Priority:     hintPriorityNormal,
			ActionType:   ActionReadOnly,
		})
	}
	sortHints(hints)
	wrapped["nextSteps"] = HintsToStructured(hints)
	if detailURL != "" {
		wrapped["confighubUrl"] = detailURL
	}
	if revisionsURL != "" {
		wrapped["confighubRevisionsUrl"] = revisionsURL
	}
	return wrapped
}

func buildMCPChangesetsStructuredContent(payload interface{}) interface{} {
	wrapped := mcpWrapStructuredData(payload)
	ref, ok := mcpExtractChangeSetRef(payload)
	if !ok {
		return wrapped
	}

	hints := []Hint{{
		Command:    mcpChangeSetGetCommand(ref),
		Rationale:  "Inspect the first returned ChangeSet to review the exact governed receipt and approval details.",
		Priority:   hintPriorityHigh,
		ActionType: ActionReadOnly,
	}}
	wrapped["nextSteps"] = HintsToStructured(hints)
	return wrapped
}

// mcpUnitRef is the normalized identity of a ConfigHub unit extracted from
// tool JSON, regardless of whether the keys are PascalCase (as emitted by
// `cub unit list --json`) or camelCase (as used internally).
type mcpUnitRef struct {
	UnitSlug  string
	UnitID    string
	SpaceSlug string
	SpaceID   string
}

// mcpUnitRevisionState captures head/live/last-applied revision numbers for
// a unit. Presence bits distinguish "unknown" from "zero" so hint logic can
// decide whether a revision comparison is meaningful.
type mcpUnitRevisionState struct {
	HeadRevision        int
	LiveRevision        int
	LastAppliedRevision int
	HasHeadRevision     bool
	HasLiveRevision     bool
	HasLastApplied      bool
}

// mcpChangeSetRef is the normalized identity of a ConfigHub ChangeSet.
type mcpChangeSetRef struct {
	ChangeSetSlug string
	ChangeSetID   string
	SpaceSlug     string
}

func mcpExtractUnitRef(payload interface{}) (mcpUnitRef, bool) {
	for _, item := range historyExtractItems(payload) {
		ref := mcpUnitRefFromItem(item)
		if ref.UnitSlug != "" || ref.UnitID != "" {
			return ref, true
		}
	}
	return mcpUnitRef{}, false
}

func mcpExtractUnitRevisionState(payload interface{}) (mcpUnitRevisionState, bool) {
	for _, item := range historyExtractItems(payload) {
		state := mcpUnitRevisionStateFromItem(item)
		if state.HasHeadRevision || state.HasLiveRevision || state.HasLastApplied {
			return state, true
		}
	}
	return mcpUnitRevisionState{}, false
}

func mcpUnitRefFromItem(item map[string]interface{}) mcpUnitRef {
	unitObj := mcpNestedMap(item, "Unit", "unit")
	if unitObj == nil {
		unitObj = item
	}
	spaceObj := mcpNestedMap(item, "Space", "space")
	if spaceObj == nil {
		spaceObj = mcpNestedMap(unitObj, "Space", "space")
	}

	ref := mcpUnitRef{
		UnitSlug:  mcpFirstString(unitObj, "Slug", "slug", "Name", "name"),
		UnitID:    mcpFirstString(unitObj, "UnitID", "unitId", "ID", "id"),
		SpaceSlug: mcpFirstString(spaceObj, "Slug", "slug", "Name", "name"),
		SpaceID:   mcpFirstString(spaceObj, "SpaceID", "spaceId", "ID", "id"),
	}
	if ref.UnitSlug == "" {
		ref.UnitSlug = mcpFirstString(item, "Slug", "slug", "Name", "name")
	}
	if ref.UnitID == "" {
		ref.UnitID = mcpFirstString(item, "UnitID", "unitId", "ID", "id")
	}
	if ref.SpaceSlug == "" {
		ref.SpaceSlug = mcpFirstString(unitObj, "SpaceSlug", "spaceSlug", "SpaceName", "spaceName")
	}
	if ref.SpaceSlug == "" {
		ref.SpaceSlug = mcpFirstString(item, "SpaceSlug", "spaceSlug", "SpaceName", "spaceName")
	}
	if ref.SpaceID == "" {
		ref.SpaceID = mcpFirstString(unitObj, "SpaceID", "spaceId")
	}
	if ref.SpaceID == "" {
		ref.SpaceID = mcpFirstString(item, "SpaceID", "spaceId")
	}
	return ref
}

func mcpUnitRevisionStateFromItem(item map[string]interface{}) mcpUnitRevisionState {
	unitObj := mcpNestedMap(item, "Unit", "unit")
	if unitObj == nil {
		unitObj = item
	}

	state := mcpUnitRevisionState{}
	if value, ok := mcpFirstInt(unitObj, "HeadRevisionNum", "headRevisionNum"); ok {
		state.HeadRevision = value
		state.HasHeadRevision = true
	}
	if value, ok := mcpFirstInt(unitObj, "LiveRevisionNum", "liveRevisionNum"); ok {
		state.LiveRevision = value
		state.HasLiveRevision = true
	}
	if value, ok := mcpFirstInt(unitObj, "LastAppliedRevisionNum", "lastAppliedRevisionNum"); ok {
		state.LastAppliedRevision = value
		state.HasLastApplied = true
	}
	if !state.HasHeadRevision {
		if value, ok := mcpFirstInt(item, "HeadRevisionNum", "headRevisionNum"); ok {
			state.HeadRevision = value
			state.HasHeadRevision = true
		}
	}
	if !state.HasLiveRevision {
		if value, ok := mcpFirstInt(item, "LiveRevisionNum", "liveRevisionNum"); ok {
			state.LiveRevision = value
			state.HasLiveRevision = true
		}
	}
	if !state.HasLastApplied {
		if value, ok := mcpFirstInt(item, "LastAppliedRevisionNum", "lastAppliedRevisionNum"); ok {
			state.LastAppliedRevision = value
			state.HasLastApplied = true
		}
	}
	return state
}

func mcpExtractChangeSetRef(payload interface{}) (mcpChangeSetRef, bool) {
	for _, item := range historyExtractItems(payload) {
		ref := mcpChangeSetRefFromItem(item)
		if ref.ChangeSetSlug != "" || ref.ChangeSetID != "" {
			return ref, true
		}
	}
	return mcpChangeSetRef{}, false
}

func mcpChangeSetRefFromItem(item map[string]interface{}) mcpChangeSetRef {
	changeSetObj := mcpNestedMap(item, "ChangeSet", "changeSet", "changeset")
	if changeSetObj == nil {
		changeSetObj = item
	}
	spaceObj := mcpNestedMap(item, "Space", "space")

	ref := mcpChangeSetRef{
		ChangeSetSlug: mcpFirstString(changeSetObj, "Slug", "slug", "Name", "name"),
		ChangeSetID:   mcpFirstString(changeSetObj, "ChangeSetID", "changeSetId", "ID", "id"),
		SpaceSlug:     mcpFirstString(spaceObj, "Slug", "slug", "Name", "name"),
	}
	if ref.ChangeSetSlug == "" {
		ref.ChangeSetSlug = mcpFirstString(item, "Slug", "slug", "Name", "name")
	}
	if ref.ChangeSetID == "" {
		ref.ChangeSetID = mcpFirstString(item, "ChangeSetID", "changeSetId", "ID", "id")
	}
	if ref.SpaceSlug == "" {
		ref.SpaceSlug = mcpFirstString(item, "SpaceSlug", "spaceSlug", "SpaceName", "spaceName")
	}
	return ref
}

// mcpNestedMap returns the first sub-object under any of the given keys.
// It accepts both PascalCase and camelCase variants because the `cub` JSON
// surface is not locked.
func mcpNestedMap(item map[string]interface{}, keys ...string) map[string]interface{} {
	for _, key := range keys {
		raw, ok := item[key]
		if !ok || raw == nil {
			continue
		}
		if nested, ok := raw.(map[string]interface{}); ok {
			return nested
		}
	}
	return nil
}

// mcpFirstString returns the first non-empty trimmed string found under any
// of the given keys.
func mcpFirstString(item map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		raw, ok := item[key]
		if !ok || raw == nil {
			continue
		}
		if value, ok := raw.(string); ok {
			value = strings.TrimSpace(value)
			if value != "" {
				return value
			}
		}
	}
	return ""
}

// mcpFirstInt returns the first integer-valued field under any of the given
// keys. Accepts float64 (the default JSON number type), int, int64, and
// numeric strings.
func mcpFirstInt(item map[string]interface{}, keys ...string) (int, bool) {
	for _, key := range keys {
		raw, ok := item[key]
		if !ok || raw == nil {
			continue
		}
		switch value := raw.(type) {
		case float64:
			return int(value), true
		case int:
			return value, true
		case int64:
			return int(value), true
		case string:
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if parsed, err := strconv.Atoi(value); err == nil {
				return parsed, true
			}
		}
	}
	return 0, false
}

func mcpUnitGetCommand(ref mcpUnitRef) string {
	target := strings.TrimSpace(ref.UnitSlug)
	if target == "" {
		target = strings.TrimSpace(ref.UnitID)
	}
	if target == "" {
		return ""
	}
	args := []string{"cub", "unit", "get", target, "--json"}
	if ref.SpaceSlug != "" {
		args = append(args, "--space", ref.SpaceSlug)
	}
	return strings.Join(args, " ")
}

func mcpChangeSetGetCommand(ref mcpChangeSetRef) string {
	target := strings.TrimSpace(ref.ChangeSetSlug)
	if target == "" {
		target = strings.TrimSpace(ref.ChangeSetID)
	}
	if target == "" {
		return ""
	}
	args := []string{"cub", "changeset", "get", target, "--json"}
	if ref.SpaceSlug != "" {
		args = append(args, "--space", ref.SpaceSlug)
	}
	return strings.Join(args, " ")
}

func mcpUnitDetailURL(ref mcpUnitRef) string {
	return configHubUnitDetailURL(ref.SpaceID, ref.UnitID)
}

func mcpUnitRevisionsURL(ref mcpUnitRef) string {
	return configHubUnitRevisionsURL(ref.SpaceID, ref.UnitID)
}

// mcpUnitRevisionHint is the decision table for revision-aware guidance.
// It interprets head vs live vs last-applied revision numbers and emits a
// priority + rationale + action type appropriate to the state.
//
// When exact is true, the caller is looking at a single unit (e.g. from
// confighub_unit_get) and a generic "open revision history" hint is upgraded
// to high priority to make the next action explicit.
func mcpUnitRevisionHint(state mcpUnitRevisionState, revisionsURL string, exact bool) *Hint {
	if revisionsURL == "" {
		return nil
	}
	switch {
	case state.HasHeadRevision && state.HasLiveRevision && state.HeadRevision > state.LiveRevision:
		return &Hint{
			Rationale:    fmt.Sprintf("Head revision %d is ahead of live revision %d; review revision history now before treating this unit as converged.", state.HeadRevision, state.LiveRevision),
			ConfigHubURL: revisionsURL,
			Priority:     hintPriorityHigh,
			ActionType:   ActionReadOnly,
		}
	case state.HasHeadRevision && state.HasLastApplied && state.HeadRevision > state.LastAppliedRevision:
		return &Hint{
			Rationale:    fmt.Sprintf("Head revision %d is ahead of last applied revision %d; review revision history now to understand what is still pending.", state.HeadRevision, state.LastAppliedRevision),
			ConfigHubURL: revisionsURL,
			Priority:     hintPriorityHigh,
			ActionType:   ActionReadOnly,
		}
	case state.HasHeadRevision && state.HasLiveRevision && state.HeadRevision == state.LiveRevision:
		return &Hint{
			Rationale:    fmt.Sprintf("Head and live revisions both point at %d; open revision history if you need the approval trail or compare view.", state.HeadRevision),
			ConfigHubURL: revisionsURL,
			Priority:     hintPriorityNormal,
			ActionType:   ActionReadOnly,
		}
	case exact:
		return &Hint{
			Rationale:    "Open revision history to inspect the approval trail and compare revisions in the GUI.",
			ConfigHubURL: revisionsURL,
			Priority:     hintPriorityHigh,
			ActionType:   ActionReadOnly,
		}
	default:
		return &Hint{
			Rationale:    "Open revision history if you need the audit trail and compare-ready review surface for this unit.",
			ConfigHubURL: revisionsURL,
			Priority:     hintPriorityNormal,
			ActionType:   ActionReadOnly,
		}
	}
}
