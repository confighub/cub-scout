// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package mapsvc

import (
	"fmt"
	"sort"
	"strings"
)

// OwnershipTreeOpts configures ownership tree rendering.
type OwnershipTreeOpts struct {
	// ShowKind includes the resource Kind in output.
	ShowKind bool

	// ShowOwnerRef includes the owner reference (e.g., "→ kustomization/apps").
	ShowOwnerRef bool

	// MaxDepth limits items shown per owner group (0 = unlimited).
	// If exceeded, a "(+N more)" line is appended.
	MaxDepth int

	// FilterOwner filters to a single owner (empty = show all).
	FilterOwner string
}

// OwnershipTreeLine represents a single line in the ownership tree output.
// Callers can use this to apply custom styling.
type OwnershipTreeLine struct {
	IsHeader   bool   // True for owner header lines
	Owner      string // Owner name (for headers and entries)
	Count      int    // Entry count (for headers only)
	Connector  string // Tree connector (├── or └──)
	Namespace  string // Resource namespace
	Name       string // Resource name
	Kind       string // Resource kind (if ShowKind)
	OwnerRef   string // Owner reference (if ShowOwnerRef)
	IsLastItem bool   // True if last item in owner group
}

// FormatOwnershipTree renders entries grouped by owner as an ASCII tree.
// This is the canonical renderer used by both CLI and TUI.
// Returns plain text without colors - callers apply styling as needed.
//
// Output format:
//
//	Owner (count)
//	  ├── namespace/name [Kind] [→ ownerRef]
//	  └── namespace/name [Kind] [→ ownerRef]
func FormatOwnershipTree(entries []Entry, opts OwnershipTreeOpts) string {
	lines := BuildOwnershipTreeLines(entries, opts)
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(FormatOwnershipTreeLine(line))
	}
	return b.String()
}

// BuildOwnershipTreeLines returns structured lines for custom rendering.
// This allows callers to apply their own styling while maintaining canonical structure.
func BuildOwnershipTreeLines(entries []Entry, opts OwnershipTreeOpts) []OwnershipTreeLine {
	var lines []OwnershipTreeLine

	// Apply owner filter if specified
	if opts.FilterOwner != "" {
		filtered := make([]Entry, 0)
		for _, e := range entries {
			if strings.EqualFold(e.Owner, opts.FilterOwner) {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	// Group by owner
	byOwner := make(map[string][]Entry)
	for _, e := range entries {
		byOwner[e.Owner] = append(byOwner[e.Owner], e)
	}

	for _, owner := range CanonicalOwnerOrder {
		ownerEntries := byOwner[owner]
		if len(ownerEntries) == 0 {
			continue
		}

		// Owner header (always shows total count, even if depth-limited)
		lines = append(lines, OwnershipTreeLine{
			IsHeader: true,
			Owner:    owner,
			Count:    len(ownerEntries),
		})

		// Sort by namespace, then name, then kind (deterministic)
		sort.Slice(ownerEntries, func(i, j int) bool {
			if ownerEntries[i].Namespace != ownerEntries[j].Namespace {
				return ownerEntries[i].Namespace < ownerEntries[j].Namespace
			}
			if ownerEntries[i].Name != ownerEntries[j].Name {
				return ownerEntries[i].Name < ownerEntries[j].Name
			}
			return ownerEntries[i].Kind < ownerEntries[j].Kind
		})

		// Apply depth limit
		showCount := len(ownerEntries)
		skippedCount := 0
		if opts.MaxDepth > 0 && len(ownerEntries) > opts.MaxDepth {
			skippedCount = len(ownerEntries) - opts.MaxDepth
			showCount = opts.MaxDepth
		}

		// Entry lines
		for i := 0; i < showCount; i++ {
			e := ownerEntries[i]
			isLast := i == showCount-1 && skippedCount == 0
			connector := "├──"
			if isLast {
				connector = "└──"
			}

			line := OwnershipTreeLine{
				IsHeader:   false,
				Owner:      owner,
				Connector:  connector,
				Namespace:  e.Namespace,
				Name:       e.Name,
				IsLastItem: isLast,
			}

			if opts.ShowKind {
				line.Kind = e.Kind
			}
			if opts.ShowOwnerRef && e.OwnerDetails != nil && e.OwnerDetails["name"] != "" {
				line.OwnerRef = e.OwnerDetails["name"]
				if lineage := strings.TrimSpace(e.OwnerDetails["lineage"]); lineage != "" {
					line.OwnerRef += " <- " + lineage
				}
			}

			lines = append(lines, line)
		}

		// Add "(+N more)" line if depth-limited
		if skippedCount > 0 {
			lines = append(lines, OwnershipTreeLine{
				Connector: "└──",
				Name:      fmt.Sprintf("(+%d more)", skippedCount),
			})
		}

		// Empty line after each owner group
		lines = append(lines, OwnershipTreeLine{})
	}

	return lines
}

// FormatOwnershipTreeLine formats a single line as plain text.
func FormatOwnershipTreeLine(line OwnershipTreeLine) string {
	if line.IsHeader {
		return fmt.Sprintf("%s (%d)\n", line.Owner, line.Count)
	}
	if line.Connector == "" {
		return "\n" // Empty separator line
	}

	result := fmt.Sprintf("  %s %s/%s", line.Connector, line.Namespace, line.Name)
	if line.Kind != "" {
		result += " " + line.Kind
	}
	if line.OwnerRef != "" {
		result += " → " + line.OwnerRef
	}
	return result + "\n"
}
