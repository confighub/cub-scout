// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// ViewOptions holds shared filter/view configuration for CLI ↔ TUI symmetry.
// The same struct is used by CLI commands and TUI initialization.
//
// CLI flags map directly to these fields:
//
//	--owner     → Owner
//	--namespace → Namespace
//	--depth     → Depth
//	--kind      → Kind
//
// This ensures identical behavior and defaults across interfaces.
type ViewOptions struct {
	// Owner filters by ownership type (Flux, ArgoCD, Helm, ConfigHub, Native, etc.)
	Owner string

	// Namespace filters by Kubernetes namespace
	Namespace string

	// Depth limits tree/hierarchy depth (0 = unlimited)
	Depth int

	// Kind filters by Kubernetes resource kind (Deployment, StatefulSet, etc.)
	Kind string
}

// Global ViewOptions populated by flags (for CLI commands)
var sharedViewOpts ViewOptions

// AddSharedViewFlags adds the common view/filter flags to a command.
// Use this for any command that supports filtering by owner, namespace, depth, or kind.
func AddSharedViewFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&sharedViewOpts.Owner, "owner", "",
		"Filter by owner (Flux, ArgoCD, Helm, ConfigHub, Crossplane, kro, Native)")
	cmd.Flags().StringVar(&sharedViewOpts.Namespace, "namespace", "",
		"Filter by namespace")
	cmd.Flags().IntVar(&sharedViewOpts.Depth, "depth", 0,
		"Limit tree depth (0 = unlimited)")
	cmd.Flags().StringVar(&sharedViewOpts.Kind, "kind", "",
		"Filter by resource kind (Deployment, StatefulSet, etc.)")
}

// HasFilters returns true if any filter is active
func (v ViewOptions) HasFilters() bool {
	return v.Owner != "" || v.Namespace != "" || v.Depth > 0 || v.Kind != ""
}

// String returns a human-readable summary of active filters
func (v ViewOptions) String() string {
	if !v.HasFilters() {
		return ""
	}

	var parts []string
	if v.Owner != "" {
		parts = append(parts, fmt.Sprintf("owner=%s", v.Owner))
	}
	if v.Namespace != "" {
		parts = append(parts, fmt.Sprintf("ns=%s", v.Namespace))
	}
	if v.Depth > 0 {
		parts = append(parts, fmt.Sprintf("depth=%d", v.Depth))
	}
	if v.Kind != "" {
		parts = append(parts, fmt.Sprintf("kind=%s", v.Kind))
	}
	return strings.Join(parts, " ")
}

// FilterStripText returns the filter strip text for TUI display.
// Returns empty string if no filters are active.
func (v ViewOptions) FilterStripText() string {
	if !v.HasFilters() {
		return ""
	}
	return "Filters: " + v.String()
}

// ValidOwners returns the list of valid owner types for validation/completion
var ValidOwners = []string{
	"Flux",
	"ArgoCD",
	"Helm",
	"ConfigHub",
	"Crossplane",
	"kro",
	"Terraform",
	"Native",
}

// ValidateOwner checks if the owner value is valid
func ValidateOwner(owner string) error {
	if owner == "" {
		return nil
	}
	for _, valid := range ValidOwners {
		if strings.EqualFold(owner, valid) {
			return nil
		}
	}
	return fmt.Errorf("invalid owner %q (valid: %s)", owner, strings.Join(ValidOwners, ", "))
}

// NormalizeOwner normalizes owner to canonical case (e.g., "flux" → "Flux")
func NormalizeOwner(owner string) string {
	for _, valid := range ValidOwners {
		if strings.EqualFold(owner, valid) {
			return valid
		}
	}
	return owner
}
