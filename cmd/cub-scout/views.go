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
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/confighub/cub-scout/pkg/hub"
)

var (
	viewsResolveFormat string
	viewsResolveSpace  string
	viewsOpenPrintOnly bool
)

var viewsCmd = &cobra.Command{
	Use:   "views",
	Short: "Work with ConfigHub Views (read-only)",
	Long: `Work with ConfigHub Views — saved filter+projection specs operators curate
in the View Explorer (https://hub.confighub.com/x/view-explorer).

Subcommands:
  resolve <uuid-or-url>   fetch the View as structured JSON
  open    <uuid-or-url>   open the View Explorer URL in the browser

The remaining #391 scope items (--view flag on map list / compare three-way,
View column projection in TUI Hub view, reality overlay composing View
columns with #393 source-truth verdicts) land as dedicated follow-up PRs.`,
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
	viewsCmd.AddCommand(viewsOpenCmd)
	viewsResolveCmd.Flags().StringVar(&viewsResolveFormat, "format", "json", "Output format. v0.1 supports: json")
	viewsResolveCmd.Flags().StringVar(&viewsResolveSpace, "space", "*", "Space to search. Defaults to org-wide ('*') so a UUID alone is sufficient")
	viewsOpenCmd.Flags().BoolVar(&viewsOpenPrintOnly, "print", false, "Print the View Explorer URL to stdout instead of opening the browser (useful for scripts and headless environments)")
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

// cubRunner is the function signature for shelling out to `cub`. The
// package-level var viewCubRunner holds the production implementation;
// tests inject a fake to avoid real subprocess calls.
type cubRunner func(ctx context.Context, args ...string) ([]byte, error)

// viewCubRunner is the seam used by fetchView and listUnitSlugsForFilter.
// Tests replace it with a function that returns prefab JSON, giving full
// coverage of the multi-hop resolution path without a real `cub` binary.
var viewCubRunner cubRunner = func(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "cub", args...).Output()
}

// fetchView shells out to `cub view get` to fetch the View JSON. The
// `--space "*"` default makes a UUID alone sufficient (org-wide
// search); narrower callers can pass --space <slug>.
func fetchView(ctx context.Context, uuid, space string) (map[string]interface{}, error) {
	out, err := viewCubRunner(ctx, "view", "get", uuid, "--space", space, "--json")
	if err != nil {
		return nil, fmt.Errorf("cub view get failed: %w", err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse cub output: %w", err)
	}
	return raw, nil
}

// extractWhereClause reads the Filter.Where string from a `cub view get`
// JSON blob. Returns an empty string (not an error) if the view has no
// Where clause so callers can decide whether to block or skip.
func extractWhereClause(view map[string]interface{}) (string, error) {
	filter, ok := view["Filter"]
	if !ok {
		return "", nil
	}
	filterMap, ok := filter.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("view Filter is not an object")
	}
	where, _ := filterMap["Where"].(string)
	return where, nil
}

// listUnitSlugsForFilter runs `cub unit list --where '<clause>'` and
// returns the matching unit slugs. Uses viewCubRunner so tests can inject
// prefab JSON without a live ConfigHub instance.
func listUnitSlugsForFilter(ctx context.Context, whereClause, space string) ([]string, error) {
	out, err := viewCubRunner(ctx, "unit", "list", "--space", space, "--where", whereClause, "-o", "json")
	if err != nil {
		return nil, fmt.Errorf("cub unit list: %w", err)
	}
	var units []struct {
		Slug string `json:"slug"`
	}
	if err := json.Unmarshal(out, &units); err != nil {
		return nil, fmt.Errorf("parse unit list: %w", err)
	}
	slugs := make([]string, 0, len(units))
	for _, u := range units {
		if u.Slug != "" {
			slugs = append(slugs, u.Slug)
		}
	}
	return slugs, nil
}

func emitResolvedView(rv ResolvedView) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rv)
}

// viewsOpenCmd implements #391 scope item #4 — the GUI ↔ CLI handoff
// helper. Operators paste a URL into cub-scout for read-only
// observation; this command reverses the trip when they want the
// authoring view in the browser.
//
// `cub-scout views open <uuid-or-url>` opens the canonical View
// Explorer URL for the reference. URLs that already point at View
// Explorer round-trip; UUIDs are constructed against `hub.WebBaseURL`
// (the same base configHubUnitDetailURL uses, so on-prem deployments
// resolve correctly).
//
// Connected mode is NOT required to print or open a URL — the URL
// itself is local construction. Auth only matters once the operator
// reaches the View Explorer page in the browser.
var viewsOpenCmd = &cobra.Command{
	Use:   "open <uuid-or-url>",
	Short: "Open the View Explorer URL for a View reference in the browser",
	Long: `Open the canonical View Explorer URL for a View reference in your
default browser. Closes the GUI <-> CLI loop:

  - paste a URL into ` + "`" + `cub-scout views resolve` + "`" + ` to read its bundle
  - run ` + "`" + `cub-scout views open` + "`" + ` to switch back to the authoring GUI

Accepts the same input shapes ` + "`" + `views resolve` + "`" + ` does — bare UUID or a
View Explorer URL — so the same identifier round-trips between
commands without copy-paste edits.

Connected mode is NOT required to construct or open a URL. Auth only
matters once the browser reaches View Explorer.

Use ` + "`" + `--print` + "`" + ` to emit the URL to stdout instead of opening the
browser. Useful in headless environments and as the right-hand side
of a pipe.`,
	Args: cobra.ExactArgs(1),
	RunE: runViewsOpen,
}

func runViewsOpen(cmd *cobra.Command, args []string) error {
	ref, err := agent.ParseViewRef(args[0])
	if err != nil {
		return fmt.Errorf("parse view reference: %w", err)
	}

	// Construct the canonical View Explorer URL. If the operator passed
	// a URL we already have a verified form, but we re-build from the
	// canonical base + UUID so an on-prem hostname overrides whatever
	// hostname the input URL had — `--space` and host preferences are
	// set in cub-scout's hub config, not in pasted URLs.
	url := viewExplorerURL(ref.UUID)

	if viewsOpenPrintOnly {
		fmt.Println(url)
		return nil
	}

	if err := openInBrowser(url); err != nil {
		// Fall back to printing the URL so the operator can copy it
		// manually — better than failing silently in headless or
		// permission-restricted environments.
		fmt.Fprintf(os.Stderr, "Could not open browser (%v).\nView Explorer URL: %s\n", err, url)
		return err
	}
	return nil
}

// viewExplorerURL builds the canonical View Explorer URL for a View
// UUID. Anchored at `hub.HubBaseURL` because View Explorer lives on
// the product UI host (e.g. https://hub.confighub.com/x/view-explorer)
// rather than the marketing site (`hub.WebBaseURL`,
// https://confighub.com). On-prem deployments are expected to override
// HubBaseURL via the existing config mechanism, so this URL builder
// follows the same convention as other product-UI deep-links in
// cub-scout.
func viewExplorerURL(uuid string) string {
	base := strings.TrimRight(hub.HubBaseURL, "/")
	return fmt.Sprintf("%s/x/view-explorer?view=%s", base, uuid)
}

// openInBrowser opens url in the operator's default browser. Uses the
// platform-appropriate command and refuses to spawn anything else if
// the URL fails our parser pre-check.
//
// Security: the URL is built from `hub.WebBaseURL` and a parsed UUID,
// neither of which can carry shell metacharacters past the parser. We
// still pass url as a single argv slot (not via a shell) to be belt
// and suspenders.
func openInBrowser(url string) error {
	var (
		cmd  string
		args []string
	)
	switch runtimeGOOS() {
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}
	return exec.Command(cmd, args...).Start()
}

// runtimeGOOS is split out so the test can inject a fake value without
// the build-tags dance, and to avoid a hard import cycle if tests need
// to swap behaviour. Default delegates to runtime.GOOS.
var runtimeGOOS = func() string { return runtime.GOOS }

