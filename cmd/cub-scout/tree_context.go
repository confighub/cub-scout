// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT
package main

import (
	"context"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	"k8s.io/client-go/rest"
)

type treeBindingKey struct{}
type treeChildConfigKey struct{}

func treeContextBinding(ctx context.Context) *localClusterBinding {
	if ctx == nil {
		return nil
	}
	binding, _ := ctx.Value(treeBindingKey{}).(*localClusterBinding)
	return binding
}
func treeClusterConfig(ctx context.Context) (*rest.Config, error) {
	if binding := treeContextBinding(ctx); binding != nil {
		if binding.err != nil {
			return nil, binding.err
		}
		return rest.CopyConfig(binding.config), nil
	}
	return buildConfig()
}
func commandOrTreeClusterConfig(cmd *cobra.Command) (*rest.Config, error) {
	if treeContextBinding(cmd.Context()) != nil {
		return treeClusterConfig(cmd.Context())
	}
	cfg, _, err := buildClusterConfigForCommand(cmd)
	return cfg, err
}
func treeKubectlCommand(ctx context.Context, args ...string) *exec.Cmd {
	if child, ok := ctx.Value(treeChildConfigKey{}).(*traceChildKubeconfig); ok {
		args = append([]string{"--kubeconfig", child.Path, "--context", child.Context}, args...)
		args = append(args, "--cache-dir", filepath.Join(filepath.Dir(child.Path), "cache"))
	}
	return exec.CommandContext(ctx, "kubectl", args...)
}

// boundCommandContext returns the command's context carrying its explicit
// --kube-context selection, if it has one. The selection is resolved once,
// here; a missing or blank explicit selection is an error, never a fallback
// to the ambient context. Without the flag the context is returned unchanged
// and readers keep the legacy ambient behaviour through treeClusterConfig.
func boundCommandContext(cmd *cobra.Command) (context.Context, error) {
	if cmd == nil {
		// Callers that invoke a run function directly, without a command,
		// have no flag to carry a selection.
		return context.Background(), nil
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if treeContextBinding(ctx) != nil {
		return ctx, nil
	}
	selection, err := clusterContextSelectionFromFlag(cmd)
	if err != nil {
		return nil, err
	}
	if !selection.explicit {
		return ctx, nil
	}
	binding := resolveLocalClusterBindingForSelection(selection)
	if binding.err != nil {
		return nil, binding.err
	}
	return context.WithValue(ctx, treeBindingKey{}, binding), nil
}

// mapClusterReadCommands lists every map subcommand that reads a cluster.
// Each accepts --kube-context. map hub, map fleet and map queries read
// ConfigHub or nothing, so a Kubernetes selector does not apply to them.
func mapClusterReadCommands() []*cobra.Command {
	return []*cobra.Command{
		mapListCmd, mapStatusCmd, mapProblemsCmd, mapDeployersCmd, mapWorkloadsCmd,
		mapDriftCmd, mapSprawlCmd, mapDashboardCmd, mapBypassCmd, mapCrashesCmd,
		mapOrphansCmd, mapHooksCmd, mapCronjobsCmd, mapJobsCmd, mapActionsCmd,
		mapActivityCmd, mapPreviewsCmd, mapClusterDataCmd, mapAppHierarchyCmd,
		mapMeaningCmd, mapPatternsCmd,
	}
}

// boundContextLabel names the context a command's reads go to: the explicit
// selection carried by ctx, or the ambient current-context when there is none.
// A label printed beside evidence must come from the same capture as the reads.
func boundContextLabel(ctx context.Context) string {
	if binding := treeContextBinding(ctx); binding != nil {
		return binding.context
	}
	return getCurrentContext()
}

// contextBoundReadCommands lists the read commands outside map and tree that
// accept --kube-context through boundCommandContext (#809).
func contextBoundReadCommands() []*cobra.Command {
	return []*cobra.Command{
		debugCmd, driftCmd, compareDriftCmd, graphExplainCmd,
		patternsDetectCmd, patternsExplainCmd, contextPackCmd, receiptVerifyCmd,
		combinedCmd, compareObjectSetCmd,
	}
}

func init() {
	for _, cmd := range contextBoundReadCommands() {
		if cmd.Flags().Lookup("kube-context") == nil {
			cmd.Flags().String("kube-context", "", "Use this exact Kubernetes context for every read in this command (no fallback)")
		}
	}
}
