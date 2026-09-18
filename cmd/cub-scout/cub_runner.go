// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// This file is the one place cub-scout runs `cub`.
//
// Three things about cub can only be checked here, because the AST guards read
// source and a call's arguments are often built at run time — from a resource's
// labels, a comma list of targets, a wizard's proposal:
//
//   - **The space.** cub v0.5.2 removed the default space. A list with no
//     `--space` spans the whole organization and a single-entity call falls back
//     to resolving a bare slug across it, so a call that forgot its space
//     returns plausible data from spaces nobody asked about. #560 fixed every
//     literal call site and guarded them with TestEveryCubCallNamesItsSpace;
//     this refuses the ones the guard cannot see.
//   - **Removed subcommands.** A removed subcommand of an existing command
//     exits 0 and prints the group's help, so `cub unit livedata` (#564, #571)
//     and `cub unit apply` (#571) returned 38 lines of help where cub-scout
//     expected data, or reported success having done nothing.
//   - **Removed flags.** `cub --version` and `--json` are rejected or
//     deprecated; the AST guards cover the literals, this covers the rest.
//
// Refusing before spawning also gives a better message than cub's own: it names
// the command cub-scout was about to run and what was missing from it.

// cubSpaceFreeCommands are the subcommands that name no space. Everything else
// is treated as space-scoped, so a cub subcommand added later fails closed:
// cub-scout refuses it until someone decides which it is.
//
// Keys are the leading words of the command. A one-word key matches any
// subcommand of it.
var cubSpaceFreeCommands = [][]string{
	{"auth"},    // login, status, get-token
	{"context"}, // get, list, use
	{"version"},
	{"help"},
	{"completion"},
	{"organization"},
	{"user"},
	{"space", "list"},  // the spaces themselves are the org-wide question
	{"space", "count"}, //
	// A space command names its space as the positional, not with --space.
	{"space", "create"},
	{"space", "get"},
	{"space", "delete"},
	{"space", "update"},
}

// cubRemovedCommands are subcommand pairs cub no longer has. A removed
// subcommand exits 0 printing help, so the caller cannot notice on its own.
var cubRemovedCommands = map[string]string{
	"unit livedata":  "removed April 2026; a unit's config data is `cub unit data`, and ConfigHub has no live-data endpoint (#564, #571)",
	"unit livestate": "removed April 2026 with unit livedata (#564)",
	"unit apply":     "removed July 2026; nothing applies a unit on demand — see cub_unit_apply.go (#571)",
	"unit destroy":   "removed July 2026 with unit apply",
	"unit import":    "removed July 2026 with unit apply",
	"unit refresh":   "removed July 2026; the current verb is `cub k8s refresh`",
}

// The `gitops` group was removed from cub in July 2026 as well (#573), and it is
// deliberately not listed above. A removed *subcommand* of a command cub still
// has is what this runner exists to catch, because cub exits 0 and prints the
// group's help, so the caller cannot tell. An unknown *top-level* command exits
// 1 saying so, which is honest, and a plugin can supply one — refusing it here
// would break a user who has that plugin installed.

// cubRemovedFlags are flags cub rejects, or is about to.
var cubRemovedFlags = map[string]string{
	"--version": "cub takes `version` as a subcommand and rejects the flag",
	"--json":    "deprecated in favour of `-o json`, which prints no notice on stderr (#563)",
}

// cubCommand builds the `cub` command cub-scout will run, or refuses it.
//
// It runs nothing itself, so a caller keeps stdin, stdout and how the output is
// read — `unit update <slug> -` needs stdin, the import debug paths want the
// combined output, the TUI wants an exit status. Callers that only want stdout
// use cubStdout or cubText.
func cubCommand(ctx context.Context, args ...string) (*exec.Cmd, error) {
	if err := checkCubArgs(args); err != nil {
		return nil, err
	}
	if ctx == nil {
		return exec.Command("cub", args...), nil
	}
	return exec.CommandContext(ctx, "cub", args...), nil
}

// cubStdout runs a `cub` command and returns its stdout alone, with whatever it
// printed on stderr in the error. This is what every read path wants: cub prints
// notices on stderr, and output read with CombinedOutput carries them into
// values that are then parsed, compared, or written back.
func cubStdout(ctx context.Context, args ...string) ([]byte, error) {
	cmd, err := cubCommand(ctx, args...)
	if err != nil {
		return nil, err
	}
	return commandStdout(cmd)
}

// cubText is cubStdout, trimmed, for the callers that work in strings.
func cubText(ctx context.Context, args ...string) (string, error) {
	out, err := cubStdout(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("cub %s failed: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// checkCubArgs is the refusal, separate from cubCommand so every case can be
// tested without spawning anything.
func checkCubArgs(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("cub needs a subcommand")
	}
	name := cubCommandName(args)

	if why, removed := cubRemovedCommands[name]; removed {
		return fmt.Errorf("cub %s no longer exists: %s", name, why)
	}
	// A one-word group, such as `gitops`, is removed whatever follows it.
	if why, removed := cubRemovedCommands[cubCommandWords(args)[0]]; removed {
		return fmt.Errorf("cub %s no longer exists: %s", cubCommandWords(args)[0], why)
	}
	for _, arg := range args {
		flag := arg
		if index := strings.Index(flag, "="); index > 0 {
			flag = flag[:index]
		}
		if why, removed := cubRemovedFlags[flag]; removed {
			return fmt.Errorf("cub %s cannot take %s: %s", name, flag, why)
		}
	}

	if cubCommandNeedsNoSpace(args) {
		return nil
	}
	space, named := cubArgsSpace(args)
	if !named {
		return fmt.Errorf("cub %s reads one ConfigHub space and none was named: pass --space <slug> (or --space '%s' for a deliberate read of every space), name the entity as <space>/<slug>, or pass its ID. cub has no default space, so none is assumed",
			name, allConfigHubSpaces)
	}
	if space == unresolvedConfigHubSpace {
		return fmt.Errorf("cub %s was given no ConfigHub space: cub-scout resolved none and will not read every space instead. Pass --space <slug> or set CUB_SPACE=<slug>", name)
	}
	return nil
}

// cubCommandNeedsNoSpace reports whether the command names no space.
func cubCommandNeedsNoSpace(args []string) bool {
	words := cubCommandWords(args)
	for _, free := range cubSpaceFreeCommands {
		if len(words) < len(free) {
			continue
		}
		match := true
		for i, word := range free {
			if words[i] != word {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// cubCommandWords are the leading non-flag words, which is how cub itself names
// a subcommand ("unit list", "worker run"). Always at least one element.
func cubCommandWords(args []string) []string {
	words := []string{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			break
		}
		words = append(words, arg)
		if len(words) == 2 {
			break
		}
	}
	if len(words) == 0 {
		words = []string{""}
	}
	return words
}

// cubCommandName is the subcommand as cub prints it.
func cubCommandName(args []string) string {
	return strings.Join(cubCommandWords(args), " ")
}

// cubArgsSpace reports the space the arguments name, and whether they name one
// at all. A --space flag is the space; otherwise a <space>/<slug> or ID
// reference carries its own, which is what cub accepts in place of the flag.
func cubArgsSpace(args []string) (string, bool) {
	for i, arg := range args {
		switch {
		case arg == "--space":
			if i+1 < len(args) {
				return strings.TrimSpace(args[i+1]), true
			}
			// A trailing --space names nothing; cub would reject it too.
			return "", false
		case strings.HasPrefix(arg, "--space="):
			return strings.TrimSpace(strings.TrimPrefix(arg, "--space=")), true
		}
	}
	for _, ref := range cubEntityReferences(args) {
		if configHubRefNamesSpace(ref) {
			return "", true
		}
	}
	return "", false
}

// cubEntityReferences are the positional arguments after the subcommand words:
// the entity a single-entity call names.
//
// A token that follows a flag is that flag's value, not a reference. Some flags
// are booleans, so a reference right after one is skipped and the call is
// refused rather than accepted — cub-scout always passes --space, so this
// errs towards refusing a call that named its space some other way.
func cubEntityReferences(args []string) []string {
	var refs []string
	words := 0
	skipNext := false
	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if strings.HasPrefix(arg, "-") {
			// "-" is cub's stdin marker, and --flag=value carries its own.
			skipNext = arg != "-" && !strings.Contains(arg, "=")
			continue
		}
		if words < 2 {
			words++
			continue
		}
		refs = append(refs, arg)
	}
	return refs
}
