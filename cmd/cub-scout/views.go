// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// cub-scout views — v0.1 of the View Explorer integration per #391. This
// file establishes the URL-as-positional convention and the View
// resolution shape downstream commands consume.
//
// v0.1 ships a single subcommand: `cub-scout views resolve`. It accepts
// a UUID or a View Explorer URL, fetches the View via `cub view get`,
// and prints structured JSON. The output shape is the contract future
// commands (--view flag on map list / compare three-way / etc.) consume
// when they need to scope to the units a View selects.
//
// What this file does NOT do:
//
//   - Wire --view onto compare three-way / map list. Each command has
//     its own scope mechanics; v0.1 keeps them separate so the URL
//     convention can land first and the integrations follow as
//     dedicated PRs (council framing — no over-bundling).
//   - Render View columns or Hub-view filtering. Those are scope items
//     #2 and following on #391, deferred to follow-ups.
//
// This file lives under `views` (top-level) rather than under `compare`
// because resolution is orthogonal to comparison — it serves any
// command or operator that wants the resolved View bundle.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/confighub/cub-scout/pkg/hub"
)

var (
	viewsResolveFormat string
	viewsResolveSpace  string
)

var viewsCmd = &cobra.Command{
	Use:   "views",
	Short: "Work with ConfigHub Views (read-only, v0.1)",
	Long: `Work with ConfigHub Views — saved filter+projection specs operators curate
in the View Explorer (https://hub.confighub.com/x/view-explorer).

v0.1 ships the URL-as-positional convention and a resolver subcommand.
Future PRs wire --view onto map list, compare three-way, and the TUI
Hub view (see #391 scope items).`,
}

var viewsResolveCmd = &cobra.Command{
	Use:   "resolve <uuid-or-url>",
	Short: "Resolve a View reference and print its structured JSON",
	Long: `Resolve a ConfigHub View reference and print structured JSON describing
the View, its filter clauses, and the originating identity.

Accepts either form:

  cub-scout views resolve 806aac53-236c-446d-8ad6-91d6daf6810e
  cub-scout views resolve https://hub.confighub.com/x/view-explorer?view=806aac53-236c-446d-8ad6-91d6daf6810e

The URL form is preferred — it is the canonical share-link operators
copy from the View Explorer address bar, so "paste from browser, run
cub-scout" is the cheapest GUI -> CLI bridge (#391 design rationale).

Output is JSON only in v0.1.

Requires connected mode (cub auth login or CONFIGHUB_API_KEY).`,
	Args: cobra.ExactArgs(1),
	RunE: runViewsResolve,
}

func init() {
	rootCmd.AddCommand(viewsCmd)
	viewsCmd.AddCommand(viewsResolveCmd)
	viewsResolveCmd.Flags().StringVar(&viewsResolveFormat, "format", "json", "Output format. v0.1 supports: json")
	viewsResolveCmd.Flags().StringVar(&viewsResolveSpace, "space", "*", "Space to search. Defaults to org-wide ('*') so a UUID alone is sufficient")
}

// ResolvedView is the JSON contract `views resolve` emits. Future
// commands that consume View resolution import this shape — keeping the
// UUID and OriginalURL fields lets them deep-link back to the GUI.
type ResolvedView struct {
	UUID        string                 `json:"uuid"`
	SourceForm  agent.ViewRefSource    `json:"source_form"`           // "uuid" | "url"
	OriginalURL string                 `json:"original_url,omitempty"`
	Extras      map[string][]string    `json:"extras,omitempty"`      // Round-tripped query params (group, etc.)
	Space       string                 `json:"space,omitempty"`
	View        map[string]interface{} `json:"view,omitempty"`        // Verbatim from `cub view get`
}

func runViewsResolve(cmd *cobra.Command, args []string) error {
	format := strings.ToLower(strings.TrimSpace(viewsResolveFormat))
	if format != "json" {
		return fmt.Errorf("invalid --format %q (v0.1 supports: json)", viewsResolveFormat)
	}

	ref, err := agent.ParseViewRef(args[0])
	if err != nil {
		return fmt.Errorf("parse view reference: %w", err)
	}

	if hub.QuickMode() != hub.Connected {
		return fmt.Errorf("views resolve requires ConfigHub authentication (run `cub auth login` or set CONFIGHUB_API_KEY)")
	}

	out := ResolvedView{
		UUID:        ref.UUID,
		SourceForm:  ref.SourceForm,
		OriginalURL: ref.OriginalURL,
		Space:       viewsResolveSpace,
	}
	if len(ref.Extras) > 0 {
		out.Extras = map[string][]string(ref.Extras)
	}

	view, err := fetchView(cmd.Context(), ref.UUID, viewsResolveSpace)
	if err != nil {
		return fmt.Errorf("fetch view: %w", err)
	}
	out.View = view

	return emitResolvedView(out)
}

// fetchView shells out to `cub view get` to fetch the View JSON. The
// `--space "*"` default makes a UUID alone sufficient (org-wide
// search); narrower callers can pass --space <slug>.
func fetchView(ctx context.Context, uuid, space string) (map[string]interface{}, error) {
	args := []string{"view", "get", uuid, "--space", space, "--json"}
	out, err := exec.CommandContext(ctx, "cub", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("cub view get failed: %w", err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse cub output: %w", err)
	}
	return raw, nil
}

func emitResolvedView(rv ResolvedView) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rv)
}
