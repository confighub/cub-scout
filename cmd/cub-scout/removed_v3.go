// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// rejectRemovedFlags installs a FlagErrorFunc on cmd that turns Cobra's
// "unknown flag: --<name>" into an error naming the 3.0 replacement.
// The flag itself is not registered on cmd.Flags(), so it never appears in
// --help or completion and cannot be silently ignored (#595).
func rejectRemovedFlags(cmd *cobra.Command, replacements map[string]string) {
	prev := cmd.FlagErrorFunc()
	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		if err != nil {
			msg := err.Error()
			for name, replacement := range replacements {
				if strings.Contains(msg, "unknown flag: --"+name) {
					return fmt.Errorf("flag --%s was removed in 3.0; %s (see docs/migration-3.0.md)", name, replacement)
				}
			}
		}
		if prev != nil {
			return prev(c, err)
		}
		return err
	})
}

// executeRootCmd executes rootCmd and rewrites unknown-command errors for
// commands removed in 3.0 so callers see the replacement instead of a bare
// "unknown command" message (#595, #562).
func executeRootCmd(args []string) error {
	if err := rootCmd.Execute(); err != nil {
		return wrapRemovedCommandError(args, err)
	}
	return nil
}

func wrapRemovedCommandError(args []string, err error) error {
	if err == nil {
		return nil
	}
	positional := firstPositionalArgs(args, 2)
	if len(positional) >= 1 && positional[0] == "hierarchy" {
		return fmt.Errorf(`command "hierarchy" was removed in 3.0; use "map --hub" or "map hub" instead (see docs/migration-3.0.md)`)
	}
	if len(positional) >= 2 && positional[0] == "fleet" && positional[1] == "outliers" {
		return fmt.Errorf(`command "fleet outliers" was removed in 3.0 with no replacement (see docs/migration-3.0.md)`)
	}
	return err
}

func firstPositionalArgs(args []string, limit int) []string {
	out := make([]string, 0, limit)
	for _, a := range args {
		if a == "--" {
			break
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		out = append(out, a)
		if len(out) >= limit {
			break
		}
	}
	return out
}
