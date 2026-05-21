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
	// link (extracted from Link.Bindings). See Bindings below for the
	// per-field breakdown when expanded.
	BindingsCount int `json:"bindingsCount,omitempty"`

	// Bindings is the per-field expansion of Link.Bindings — C2 of the
	// attribution layer. Each entry pairs a downstream path on this unit
	// with the upstream path on the producer unit that feeds it. Best-
	// effort: parsed from the JSON shape ConfigHub returns; unknown shapes
	// fall back to an empty list while BindingsCount still reflects the
	// raw count.
	Bindings []FieldBinding `json:"bindings,omitempty"`
}

// FieldBinding captures one entry from a Link's Bindings list — the
// extraction of a value from an upstream unit's path into a downstream unit's
// path. Optional TransformExpr captures any Go template / CEL expression
// applied between source and target.
type FieldBinding struct {
	// DownstreamPath is the canonical field-path within the downstream unit
	// where the bound value lands (e.g., ".spec.replicas").
	DownstreamPath string `json:"downstreamPath,omitempty"`

	// UpstreamPath is the canonical field-path within the upstream unit
	// that supplies the value.
	UpstreamPath string `json:"upstreamPath,omitempty"`

	// TransformExpr is the optional transform expression (Go template, CEL,
	// or similar) applied between source and target.
	TransformExpr string `json:"transformExpr,omitempty"`
}

// FieldBindingSource references the specific Link + binding entry that
// produced a given field mismatch's value. Surfaced on compareFieldMismatch
// when the field name maps to a known canonical path and a matching binding
// is found in IncomingBindings.
type FieldBindingSource struct {
	// LinkID is the unique identifier of the Link entity.
	LinkID string `json:"linkId,omitempty"`

	// LinkSlug is the Link's URL-safe identifier.
	LinkSlug string `json:"linkSlug,omitempty"`

	// UpstreamUnitID is the producer unit feeding this field.
	UpstreamUnitID string `json:"upstreamUnitId,omitempty"`

	// UpstreamPath is the path within the upstream unit that supplies
	// the value.
	UpstreamPath string `json:"upstreamPath,omitempty"`

	// TransformExpr is the optional transform applied (Go template, CEL).
	TransformExpr string `json:"transformExpr,omitempty"`
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
		entry.Bindings = expandBindings(r.Bindings)
		out = append(out, entry)
	}
	return out
}

// expandBindings parses Link.Bindings into structured FieldBinding entries.
// ConfigHub's Bindings field shape is evolving; this function handles two
// common JSON layouts and falls back to nil on unrecognized shapes:
//
//   1. Array of objects: [{downstreamPath, upstreamPath, transformExpr}]
//   2. Object keyed by downstream path: {".spec.x": {upstreamPath, ...}}
//
// Unknown shapes return nil — BindingsCount still reflects the raw count so
// callers can detect "bindings exist but we couldn't expand them."
func expandBindings(raw json.RawMessage) []FieldBinding {
	if len(raw) == 0 {
		return nil
	}

	// Shape 1: array of objects.
	type bindingObj struct {
		DownstreamPath string `json:"downstreamPath"`
		UpstreamPath   string `json:"upstreamPath"`
		TransformExpr  string `json:"transformExpr"`
		// Some schemas use Target/Source naming. Captured as fallback.
		Target    string `json:"target"`
		Source    string `json:"source"`
		Transform string `json:"transform"`
	}
	var asArray []bindingObj
	if err := json.Unmarshal(raw, &asArray); err == nil && len(asArray) > 0 {
		out := make([]FieldBinding, 0, len(asArray))
		for _, b := range asArray {
			fb := FieldBinding{
				DownstreamPath: firstNonEmptyBindingStr(b.DownstreamPath, b.Target),
				UpstreamPath:   firstNonEmptyBindingStr(b.UpstreamPath, b.Source),
				TransformExpr:  firstNonEmptyBindingStr(b.TransformExpr, b.Transform),
			}
			if fb.DownstreamPath != "" || fb.UpstreamPath != "" || fb.TransformExpr != "" {
				out = append(out, fb)
			}
		}
		if len(out) > 0 {
			return out
		}
	}

	// Shape 2: object keyed by downstream path.
	var asObject map[string]bindingObj
	if err := json.Unmarshal(raw, &asObject); err == nil && len(asObject) > 0 {
		out := make([]FieldBinding, 0, len(asObject))
		for downstream, b := range asObject {
			fb := FieldBinding{
				DownstreamPath: downstream,
				UpstreamPath:   firstNonEmptyBindingStr(b.UpstreamPath, b.Source),
				TransformExpr:  firstNonEmptyBindingStr(b.TransformExpr, b.Transform),
			}
			out = append(out, fb)
		}
		return out
	}

	return nil
}

func firstNonEmptyBindingStr(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// LookupFieldBindingSource searches incomingBindings for a binding whose
// DownstreamPath matches the given canonical path. Returns the first match
// (deterministic by IncomingBindings iteration order). The caller is
// expected to map a compareFieldMismatch.Field name to its canonical path
// before calling (see compareFieldToPath).
func LookupFieldBindingSource(downstreamPath string, incomingBindings []IncomingBinding) *FieldBindingSource {
	downstreamPath = strings.TrimSpace(downstreamPath)
	if downstreamPath == "" || len(incomingBindings) == 0 {
		return nil
	}
	for _, link := range incomingBindings {
		for _, b := range link.Bindings {
			if b.DownstreamPath == downstreamPath {
				return &FieldBindingSource{
					LinkID:         link.LinkID,
					LinkSlug:       link.Slug,
					UpstreamUnitID: link.ToUnitID,
					UpstreamPath:   b.UpstreamPath,
					TransformExpr:  b.TransformExpr,
				}
			}
		}
	}
	return nil
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
