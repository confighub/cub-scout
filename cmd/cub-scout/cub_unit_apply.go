// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import "fmt"

// cub removed `unit apply` in July 2026, together with `unit destroy`,
// `unit import`, `unit refresh` and the whole `gitops` command group. There is
// no replacement verb in cub v0.5.2: `cub unit update` carries
// `--wait` ("wait for completion", default true), and a unit's target is served
// by its worker.
//
// The dead call did not fail loudly. `cub unit apply <slug> --space <s>` exits
// **0** and prints the `cub unit` help, so the three sites that ran it either
// reported success without applying anything — the TUI import step did exactly
// that — or, with `--wait`, died on "unknown flag" and blamed the worker.
//
// cub-scout does not guess a replacement. It is a read-only observer: it says
// what it did, says plainly what it did not do, and names the command that can
// answer the rest. ConfigHub's own unit JSON carries no applied or live
// revision on this cub (checked across every unit with a target), so there is
// no delivery fact to report in its place either.

// unitApplyUnavailable explains why cub-scout does not apply a unit, and what
// to run to find out whether the target has the change.
func unitApplyUnavailable(space, unitSlug string) error {
	return fmt.Errorf("cub-scout did not apply unit %s in space %s: cub has no `unit apply` command (removed in July 2026), so ConfigHub delivers a unit's revision through its target's worker. To see whether the target has it, run `cub-scout compare <kind>/<name> -n <namespace>`", unitSlug, space)
}
