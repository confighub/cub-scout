// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strings"

	"github.com/confighub/cub-scout/pkg/hub"
)

// Where a resolved ConfigHub space came from. Output reports it, so a reader
// can tell a space they chose from one that was picked up from the environment.
const (
	spaceSourceFlag           = "flag"
	spaceSourceResource       = "resource"
	spaceSourceEnv            = "CUB_SPACE"
	spaceSourceContextDefault = "cub-context-default"
)

// allConfigHubSpaces is cub's explicit "every space in the organization".
const allConfigHubSpaces = "*"

// configHubSpace is a ConfigHub space and how it was chosen.
type configHubSpace struct {
	Slug   string
	Source string
}

func (s configHubSpace) IsSet() bool { return s.Slug != "" }

// cubContextDefaultSpaceFn reads the cub context's default space. A context
// need not have one, and a space that is set there is ambient state that any
// other shell sharing the cub config can change, so it is the last resort.
var cubContextDefaultSpaceFn = func() string {
	cubCtx, _, err := getStatusCubContext()
	if err != nil || cubCtx == nil {
		return ""
	}
	return strings.TrimSpace(cubCtx.Settings.DefaultSpace)
}

// resolveConfigHubSpace answers "which ConfigHub space?" in one place.
//
// Precedence: what the command line or the resource itself says, then
// CUB_SPACE, then the cub context's default space if it has one. If none of
// those gives a space the result is unset. It never falls back to every space:
// a caller that gets an unset space must refuse or report an omission, because
// a `cub ... list` run without --space does not fail, it reads a scope nobody
// chose.
func resolveConfigHubSpace(explicit, explicitSource string) configHubSpace {
	if space := strings.TrimSpace(explicit); space != "" {
		if explicitSource == "" {
			explicitSource = spaceSourceFlag
		}
		return configHubSpace{Slug: space, Source: explicitSource}
	}
	if space := strings.TrimSpace(hub.PluginSpace()); space != "" {
		return configHubSpace{Slug: space, Source: spaceSourceEnv}
	}
	if space := strings.TrimSpace(cubContextDefaultSpaceFn()); space != "" {
		return configHubSpace{Slug: space, Source: spaceSourceContextDefault}
	}
	return configHubSpace{}
}

// requireConfigHubSpace resolves a space for a command that cannot run without
// one. flag is how the user supplies it, for example "--space".
func requireConfigHubSpace(feature, flag, explicit string) (configHubSpace, error) {
	space := resolveConfigHubSpace(explicit, spaceSourceFlag)
	if !space.IsSet() {
		return space, errNoConfigHubSpace(feature, flag)
	}
	return space, nil
}

func errNoConfigHubSpace(feature, flag string) error {
	how := "set CUB_SPACE=<slug>"
	if flag != "" {
		how = fmt.Sprintf("pass %s <slug> (or %s '%s' for every space), or set CUB_SPACE=<slug>", flag, flag, allConfigHubSpaces)
	}
	return fmt.Errorf("%s needs a ConfigHub space and none was given: %s. No space is assumed, because the cub context has no default space set", feature, how)
}

// withConfigHubSpace appends an explicit --space to cub arguments. Every
// space-scoped cub call goes through it so the scope is always on the command
// line and never left to the cub context.
func withConfigHubSpace(args []string, space string) []string {
	return append(args, "--space", strings.TrimSpace(space))
}

// mcpConfigHubSpace resolves the space for a connected MCP tool whose `space`
// argument is optional. An omitted argument used to mean "whatever the cub
// context defaults to", which an agent calling the tool cannot see.
func mcpConfigHubSpace(tool string, arguments map[string]interface{}) (string, error) {
	space := resolveConfigHubSpace(argString(arguments, "space"), spaceSourceFlag)
	if !space.IsSet() {
		return "", fmt.Errorf("%s needs a ConfigHub space: pass the `space` argument (use '%s' only for a deliberate all-spaces read); none was given, CUB_SPACE is unset and the cub context has no default space", tool, allConfigHubSpaces)
	}
	return space.Slug, nil
}

// withConfigHubSpaceOrTargets scopes a cub read by space, by targets, or both.
// A target given without a space must carry its own: `<space>/<slug>` or a
// UUID. A bare target slug would be resolved against the cub context's default
// space, which is the hidden input this file exists to remove.
func withConfigHubSpaceOrTargets(args []string, space string, targets []string) ([]string, error) {
	space = strings.TrimSpace(space)
	if space != "" {
		args = withConfigHubSpace(args, space)
	}
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if space == "" && !strings.Contains(target, "/") && !looksLikeUUID(target) {
			return nil, fmt.Errorf("target %q names no space: pass `space`, or name the target as <space>/%s or by its UUID", target, target)
		}
		args = append(args, "--target", target)
	}
	return args, nil
}

func looksLikeUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, r := range value {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
				return false
			}
		}
	}
	return true
}
