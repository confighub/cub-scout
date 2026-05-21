// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
)

// TestSourceTruthHelpListsAllStrategies prevents the help text from drifting
// out of sync with the strategy enum in pkg/agent. Every value returned by
// agent.AllStrategies() must appear in both the long help text and the
// --strategy flag description.
//
// This test is the drift-resistance gate referenced in #450: when the enum
// expanded from 4 strategies to 9 in #418 (Phase 2), the help text was not
// updated, leaving operators reading stale guidance. Adding a new strategy
// in code now requires the help to mention it, or this test fails.
func TestSourceTruthHelpListsAllStrategies(t *testing.T) {
	all := agent.AllStrategies()
	if len(all) == 0 {
		t.Fatal("agent.AllStrategies() returned no strategies — fixture broken")
	}

	long := sourceTruthCmd.Long
	flag := sourceTruthCmd.Flags().Lookup("strategy")
	if flag == nil {
		t.Fatal("--strategy flag not registered")
	}

	for _, s := range all {
		strategy := string(s)
		if !strings.Contains(long, strategy) {
			t.Errorf("source-truth Long help does not mention strategy %q — update the Long block in source_truth.go to list the new strategy", strategy)
		}
		if !strings.Contains(flag.Usage, strategy) {
			t.Errorf("--strategy flag help does not mention %q — sourceTruthStrategyFlagHelp() builds this dynamically; check that agent.AllStrategies() includes it", strategy)
		}
	}
}

// TestSourceTruthFlagHelpDynamic verifies the flag-help builder produces
// the same set of strategies as agent.AllStrategies(), in order. Decoupling
// catches the case where someone replaces the dynamic builder with a
// hand-maintained string that then drifts.
func TestSourceTruthFlagHelpDynamic(t *testing.T) {
	help := sourceTruthStrategyFlagHelp()
	all := agent.AllStrategies()
	for _, s := range all {
		if !strings.Contains(help, string(s)) {
			t.Errorf("sourceTruthStrategyFlagHelp() does not contain %q; help text was: %s", string(s), help)
		}
	}
	// The phrase "Required" anchors the help text against drift in the
	// surrounding prose.
	if !strings.Contains(help, "Required") {
		t.Errorf("sourceTruthStrategyFlagHelp() missing the Required marker; got: %s", help)
	}
}
