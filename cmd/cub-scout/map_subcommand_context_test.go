// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// selectMapContext sets --kube-context on a real map subcommand for one test
// and restores the flag, so the package-level command is left as it was.
func selectMapContext(t *testing.T, cmd *cobra.Command, selection string) {
	t.Helper()
	flag := cmd.Flags().Lookup("kube-context")
	require.NotNil(t, flag, "map %s has no --kube-context flag", cmd.Name())
	require.NoError(t, cmd.Flags().Set("kube-context", selection))
	cmd.SetContext(context.Background())
	t.Cleanup(func() {
		_ = flag.Value.Set("")
		flag.Changed = false
	})
}

// #804: sixteen map subcommands read the ambient context even when the
// caller had pinned another one for `map list`. Every cluster-reading map
// subcommand must read only the selected context and must refuse a missing
// or blank selection instead of falling back.
func TestMapSubcommandsReadOnlyTheSelectedContext(t *testing.T) {
	args := map[string][]string{"actions": {"deployment/api"}}
	for _, cmd := range mapClusterReadCommands() {
		if cmd == mapListCmd {
			// Covered by TestMapListExplicitContextSelectsOnlyNamedServerWithoutConfigMutation;
			// the full inventory read is slow under client-side rate limiting.
			continue
		}
		t.Run(cmd.Name(), func(t *testing.T) {
			require.NotNil(t, cmd.RunE, "map %s has no RunE", cmd.Name())
			selected, ambient := newCountedKubeServer(t), newCountedKubeServer(t)
			path, before := resolverKubeconfig(t, "ambient", map[string]string{"selected": selected.server.URL, "ambient": ambient.server.URL})
			t.Setenv("KUBECONFIG", path)

			selectMapContext(t, cmd, "selected")
			var err error
			captureStdout(t, func() { err = cmd.RunE(cmd, args[cmd.Name()]) })
			require.NoError(t, err)
			require.NotZero(t, selected.requests.Load(), "the selected context was not read")
			require.Zero(t, ambient.requests.Load(), "the ambient context was read: %v", ambient.paths)

			for _, refused := range []string{"missing", ""} {
				seen := selected.requests.Load()
				require.NoError(t, cmd.Flags().Set("kube-context", refused))
				captureStdout(t, func() { err = cmd.RunE(cmd, args[cmd.Name()]) })
				require.Error(t, err, "--kube-context=%q must refuse", refused)
				require.Equal(t, seen, selected.requests.Load(), "--kube-context=%q read a cluster before refusing", refused)
				require.Zero(t, ambient.requests.Load(), "--kube-context=%q fell back to the ambient context", refused)
			}

			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, after, "the kubeconfig was modified")
		})
	}
}

// A new map subcommand that reads a cluster must join mapClusterReadCommands
// and so gain --kube-context. The three exemptions read ConfigHub or nothing.
func TestEveryMapSubcommandIsClassifiedForContextSelection(t *testing.T) {
	bound := map[*cobra.Command]bool{}
	for _, cmd := range mapClusterReadCommands() {
		bound[cmd] = true
		require.NotNil(t, cmd.Flags().Lookup("kube-context"), "map %s has no --kube-context flag", cmd.Name())
	}
	exempt := map[*cobra.Command]bool{mapHubCmd: true, mapFleetCmd: true, mapQueriesCmd: true}
	for _, cmd := range mapCmd.Commands() {
		if cmd.Name() == "help" || cmd.Name() == "completion" {
			continue
		}
		require.True(t, bound[cmd] || exempt[cmd], "map %s is neither in mapClusterReadCommands nor exempt as a non-cluster command", cmd.Name())
		if exempt[cmd] {
			require.Nil(t, cmd.Flags().Lookup("kube-context"), "map %s does not read a cluster but accepts --kube-context", cmd.Name())
		}
	}
}
