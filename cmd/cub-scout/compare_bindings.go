// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// IncomingBinding describes a ConfigHub Link whose downstream unit is the
// resource's owning unit. Each entry represents "this unit's config is
// influenced by upstream unit X via update type Y" — the variant-management
// directed graph cub-scout surfaces as evidence (C1 of the attribution layer,
// issue #435).
//
// Fields are a curated subset of the Link entity (see `cub link explain`).
// The classifier intentionally does not expand the full Bindings list at C1;
// per-field binding attribution is C2.
type IncomingBinding struct {
	// LinkID is the unique identifier for the link.
	LinkID string `json:"linkId,omitempty"`

	// Slug is the URL-safe identifier — useful for `cub link get <slug>`.
	Slug string `json:"slug,omitempty"`

	// DisplayName is the human-friendly name when set.
	DisplayName string `json:"displayName,omitempty"`

	// UpdateType is the operation performed by the link: NeedsProvides,
	// UpgradeUnits, MergeUnits, Insert, Upsert, or future Transform.
	UpdateType string `json:"updateType,omitempty"`

	// ToUnitID identifies the upstream (producer) Unit.
	ToUnitID string `json:"toUnitId,omitempty"`

	// ToSpaceID identifies the upstream Unit's space.
	ToSpaceID string `json:"toSpaceId,omitempty"`

	// AutoUpdate is true when the link auto-propagates upstream changes
	// to the downstream unit.
	AutoUpdate bool `json:"autoUpdate,omitempty"`

	// WhereResource is the optional filter selecting which upstream
	// resources are eligible for propagation.
	WhereResource string `json:"whereResource,omitempty"`

	// BindingsCount is the number of explicit binding expressions on the
	// link (extracted from Link.Bindings). The detailed per-field
	// breakdown is C2 territory.
	BindingsCount int `json:"bindingsCount,omitempty"`
}

// linkRunner is the abstraction over `cub link` shell invocations used by the
// incoming-binding collector. Production wires this to exec.CommandContext;
// tests inject a fake that returns prefab JSON.
type linkRunner func(ctx context.Context, args ...string) ([]byte, error)

var compareLinkRunner linkRunner = func(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "cub", args...).Output()
}

// collectIncomingBindings queries `cub link list` for all links whose
// FromUnitID matches the given downstream unit. Each matching link is
// returned as an IncomingBinding for surfacing on compareResourceResult.
//
// Best-effort: returns nil on any auth failure, parse error, or empty result
// — bindings are enrichment evidence, not a hard dependency. Returning nil
// matches the rest of the connected enrichment pattern (attribution,
// gitSource).
func collectIncomingBindings(ctx context.Context, unitID, space string) []IncomingBinding {
	if strings.TrimSpace(unitID) == "" {
		return nil
	}
	space = strings.TrimSpace(space)
	if space == "" {
		space = "*"
	}
	where := fmt.Sprintf("FromUnitID = '%s'", strings.ReplaceAll(unitID, "'", ""))
	out, err := compareLinkRunner(ctx, "link", "list", "--space", space, "--where", where, "-o", "json", "--quiet")
	if err != nil || len(out) == 0 {
		return nil
	}
	return parseLinkListJSON(out)
}

// parseLinkListJSON decodes the JSON array `cub link list -o json` returns
// into IncomingBinding values. Unknown fields are ignored; unparseable input
// yields nil so callers can treat the result as "no bindings observed."
func parseLinkListJSON(raw []byte) []IncomingBinding {
	type rawLink struct {
		LinkID        string          `json:"LinkID"`
		Slug          string          `json:"Slug"`
		DisplayName   string          `json:"DisplayName"`
		UpdateType    string          `json:"UpdateType"`
		ToUnitID      string          `json:"ToUnitID"`
		ToSpaceID     string          `json:"ToSpaceID"`
		AutoUpdate    bool            `json:"AutoUpdate"`
		WhereResource string          `json:"WhereResource"`
		Bindings      json.RawMessage `json:"Bindings"`
	}
	var rows []rawLink
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil
	}
	if len(rows) == 0 {
		return nil
	}
	out := make([]IncomingBinding, 0, len(rows))
	for _, r := range rows {
		entry := IncomingBinding{
			LinkID:        strings.TrimSpace(r.LinkID),
			Slug:          strings.TrimSpace(r.Slug),
			DisplayName:   strings.TrimSpace(r.DisplayName),
			UpdateType:    strings.TrimSpace(r.UpdateType),
			ToUnitID:      strings.TrimSpace(r.ToUnitID),
			ToSpaceID:     strings.TrimSpace(r.ToSpaceID),
			AutoUpdate:    r.AutoUpdate,
			WhereResource: strings.TrimSpace(r.WhereResource),
		}
		entry.BindingsCount = countBindings(r.Bindings)
		out = append(out, entry)
	}
	return out
}

// countBindings returns the number of binding entries on a Link without
// expanding their structure. The Bindings field is a list of arbitrary
// shape per ConfigHub schema; for C1 we only need the count.
func countBindings(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	// Try a list of objects; many BindingList shapes serialize that way.
	var asArray []json.RawMessage
	if err := json.Unmarshal(raw, &asArray); err == nil {
		return len(asArray)
	}
	// Some shapes wrap entries in an object — try that and sum the entries.
	var asObject map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asObject); err == nil {
		return len(asObject)
	}
	return 0
}

func renderIncomingBindingsASCII(b *strings.Builder, bindings []IncomingBinding) {
	if len(bindings) == 0 {
		return
	}
	b.WriteString("\nIncoming Bindings (ConfigHub)\n")
	for _, link := range bindings {
		display := link.DisplayName
		if display == "" {
			display = link.Slug
		}
		if display == "" {
			display = link.LinkID
		}
		line := fmt.Sprintf("  - %s [%s]", display, link.UpdateType)
		if link.ToUnitID != "" {
			line += fmt.Sprintf(" <- unit:%s", link.ToUnitID)
		}
		if link.AutoUpdate {
			line += " auto-update"
		}
		if link.BindingsCount > 0 {
			line += fmt.Sprintf(" bindings=%d", link.BindingsCount)
		}
		b.WriteString(line + "\n")
	}
}

func renderIncomingBindingsMarkdown(b *strings.Builder, bindings []IncomingBinding) {
	if len(bindings) == 0 {
		return
	}
	b.WriteString("\n### Incoming Bindings (ConfigHub)\n\n")
	b.WriteString("| Link | Update Type | Upstream Unit | Auto | Bindings |\n")
	b.WriteString("|---|---|---|---|---:|\n")
	for _, link := range bindings {
		display := link.DisplayName
		if display == "" {
			display = link.Slug
		}
		if display == "" {
			display = link.LinkID
		}
		auto := "no"
		if link.AutoUpdate {
			auto = "yes"
		}
		b.WriteString(fmt.Sprintf("| `%s` | `%s` | `%s` | %s | %d |\n",
			display,
			link.UpdateType,
			link.ToUnitID,
			auto,
			link.BindingsCount,
		))
	}
	b.WriteString("\n")
}
