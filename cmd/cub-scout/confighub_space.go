// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strings"

	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/confighub/cub-scout/pkg/hub"
)

// Where a resolved ConfigHub space came from. Output reports it, so a reader
// can tell a space named on the command line or by the resource from one taken
// from the environment.
const (
	spaceSourceFlag     = "flag"
	spaceSourceResource = "resource"
	spaceSourceEnv      = "CUB_SPACE"
	// spaceSourceCommandDefault is a command's documented scope when nothing
	// names a space, such as every space for `map fleet`.
	spaceSourceCommandDefault = "command-default"
)

// allConfigHubSpaces is cub's explicit "every space in the organization".
const allConfigHubSpaces = "*"

// unresolvedConfigHubSpace is sent as --space if a caller ever passes an empty
// space. It cannot be a space slug, so cub refuses the call.
const unresolvedConfigHubSpace = "(no-space-resolved)"

// configHubSpace is a ConfigHub space and how it was chosen.
type configHubSpace struct {
	Slug   string
	Source string
}

func (s configHubSpace) IsSet() bool { return s.Slug != "" }

func (s configHubSpace) IsAll() bool { return s.Slug == allConfigHubSpaces }

// Scope is the space as a command reports it in its output.
func (s configHubSpace) Scope() *configHubScope {
	if !s.IsSet() {
		return nil
	}
	return &configHubScope{Space: s.Slug, SpaceSource: s.Source}
}

// configHubScope names the ConfigHub space a command read and how it was chosen.
type configHubScope struct {
	Space       string `json:"space"`
	SpaceSource string `json:"spaceSource"`
}

// resolveConfigHubSpace answers "which ConfigHub space?" in one place: the
// value named on the command line, else CUB_SPACE, else nothing.
//
// The cub context is not consulted. cub has no default space from v0.5.2: every
// command names its space, and a list run without --space covers the whole
// organization. Config files written by older cub still carry a defaultSpace
// setting that cub ignores, and reading it here would scope cub-scout to a
// space cub itself no longer uses.
//
// CUB_SPACE is the space variable of cub's plugin environment. cub itself does
// not read it and, from v0.5.2, does not set it, so a value is one the user
// exported, and output reports it as the source. cub before v0.5.2 set it to the
// context's default space when it ran cub-scout as a plugin, so under an older
// cub the value can be that default.
//
// An unset result is never widened to every space. The caller refuses, or
// reports an omission.
func resolveConfigHubSpace(flagValue string) configHubSpace {
	if space := strings.TrimSpace(flagValue); space != "" {
		return configHubSpace{Slug: space, Source: spaceSourceFlag}
	}
	if space := hub.PluginSpace(); space != "" {
		return configHubSpace{Slug: space, Source: spaceSourceEnv}
	}
	return configHubSpace{}
}

// requireConfigHubSpace resolves a space for a command that cannot run without
// one. flag is how the user supplies it, for example "--space".
func requireConfigHubSpace(feature, flag, flagValue string) (configHubSpace, error) {
	space := resolveConfigHubSpace(flagValue)
	if !space.IsSet() {
		how := "set CUB_SPACE=<slug>"
		if flag != "" {
			how = fmt.Sprintf("pass %s <slug> (or %s '%s' for every space), or set CUB_SPACE=<slug>", flag, flag, allConfigHubSpaces)
		}
		return space, errNoConfigHubSpace(feature, how)
	}
	return space, nil
}

// requireSingleConfigHubSpace is requireConfigHubSpace for a command that
// matches entities by slug. A slug is unique only within one space, so such a
// command cannot read every space. why says what the slug match is.
func requireSingleConfigHubSpace(feature, flag, flagValue, why string) (configHubSpace, error) {
	space := resolveConfigHubSpace(flagValue)
	if !space.IsSet() {
		how := "set CUB_SPACE=<slug>"
		if flag != "" {
			how = fmt.Sprintf("pass %s <slug>, or set CUB_SPACE=<slug>", flag)
		}
		return space, errNoConfigHubSpace(feature, how)
	}
	if space.IsAll() {
		return space, fmt.Errorf("%s reads one ConfigHub space, not '%s' (from %s): %s", feature, allConfigHubSpaces, space.Source, why)
	}
	return space, nil
}

func errNoConfigHubSpace(feature, how string) error {
	return fmt.Errorf("%s needs a ConfigHub space and none was given: %s. cub has no default space, so none is assumed", feature, how)
}

// withConfigHubSpace returns cub arguments with an explicit --space appended.
//
// The space must already be resolved. cub reads a missing or empty --space as
// every space in the organization, so an empty value must never reach it; if
// one does, a value no space can have is sent instead and cub refuses the call.
// The arguments are copied, so a caller's slice is never written through.
func withConfigHubSpace(args []string, space string) []string {
	space = strings.TrimSpace(space)
	if space == "" {
		space = unresolvedConfigHubSpace
	}
	out := make([]string, 0, len(args)+2)
	out = append(out, args...)
	return append(out, "--space", space)
}

// withConfigHubSpaceFromRef returns cub arguments that end with a reference
// carrying its own space, `<space>/<slug>` or an ID, so no --space is sent. A
// reference without a space gets the unresolved placeholder, and cub refuses
// the call.
func withConfigHubSpaceFromRef(args []string, ref string) []string {
	ref = strings.TrimSpace(ref)
	out := make([]string, 0, len(args)+3)
	out = append(out, args...)
	out = append(out, ref)
	if !configHubRefNamesSpace(ref) {
		out = append(out, "--space", unresolvedConfigHubSpace)
	}
	return out
}

// mcpConfigHubSpace reads the space a connected MCP tool call names. The call
// must name it: an agent cannot see the environment of the server it calls, so
// CUB_SPACE is not consulted here.
func mcpConfigHubSpace(tool string, arguments map[string]interface{}) (string, error) {
	space := argString(arguments, "space")
	if space == "" {
		return "", fmt.Errorf("%s needs a ConfigHub space: pass the `space` argument (use '%s' only for a deliberate all-spaces read). cub has no default space, so none is assumed", tool, allConfigHubSpaces)
	}
	return space, nil
}

// withConfigHubSpaceOrTargets scopes a cub read by space, by targets, or both.
// A target that is not read within one named space must carry its own space:
// `<space>/<slug>` or a UUID. cub resolves a bare target slug across the whole
// organization, where the same slug can name targets in several spaces. cub
// also splits a --target value on commas, so each part is checked.
func withConfigHubSpaceOrTargets(args []string, space string, targets []string) ([]string, error) {
	space = strings.TrimSpace(space)
	if space != "" {
		args = withConfigHubSpace(args, space)
	}
	for _, target := range targets {
		target = strings.TrimSpace(target)
		for _, ref := range strings.Split(target, ",") {
			ref = strings.TrimSpace(ref)
			if ref == "" {
				return nil, fmt.Errorf("target %q has an empty entry", target)
			}
			if (space == "" || space == allConfigHubSpaces) && !configHubRefNamesSpace(ref) {
				return nil, fmt.Errorf("target %q names no space: pass a single `space`, or name the target as <space>/%s or by its UUID", ref, ref)
			}
		}
		args = append(args, "--target", target)
	}
	return args, nil
}

// configHubRefNamesSpace reports whether a reference resolves without a space:
// `<space>/<slug>` with both parts present and a real space, or a UUID. `*/x`
// names every space, not one.
func configHubRefNamesSpace(ref string) bool {
	if agent.IsUUID(ref) {
		return true
	}
	space, slug, ok := strings.Cut(ref, "/")
	space, slug = strings.TrimSpace(space), strings.TrimSpace(slug)
	return ok && space != "" && space != allConfigHubSpaces && slug != "" && !strings.Contains(slug, "/")
}
