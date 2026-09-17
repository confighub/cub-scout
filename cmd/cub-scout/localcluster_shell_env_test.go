// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
)

// The TUI shell must not set a variable the cub CLI reads. CUB_CONTEXT selects
// cub's own context and fails on an unknown name; exporting the kube context
// there broke every cub command in the shell.
func TestTUIShellEnvLeavesCubVariablesAlone(t *testing.T) {
	cubReads := []string{"CUB_CONTEXT", "CUB_CONFIG", "CUB_SPACE", "CUB_SERVER", "CUB_TOKEN"}

	env := tuiShellEnv([]string{"PATH=/bin"}, "kind-demo", "demo", "prod", true)
	for _, kv := range env {
		for _, name := range cubReads {
			if strings.HasPrefix(kv, name+"=") {
				t.Fatalf("env sets %s, which cub reads: %v", kv, env)
			}
		}
	}
	want := []string{"PATH=/bin", "CUB_SCOUT_TUI=1", "CUB_SCOUT_KUBE_CONTEXT=kind-demo", "CUB_SCOUT_CLUSTER=demo", "CUB_SCOUT_NAMESPACE=prod", "CUB_SCOUT_CONNECTED=1"}
	if strings.Join(env, "|") != strings.Join(want, "|") {
		t.Fatalf("env = %v, want %v", env, want)
	}

	// A value the user set before starting the TUI is passed through unchanged.
	env = tuiShellEnv([]string{"CUB_CONTEXT=my-cub-context"}, "kind-demo", "", "", false)
	if strings.Join(env, "|") != "CUB_CONTEXT=my-cub-context|CUB_SCOUT_TUI=1|CUB_SCOUT_KUBE_CONTEXT=kind-demo" {
		t.Fatalf("env = %v, want the user's CUB_CONTEXT kept and nothing else of cub's set", env)
	}
}
