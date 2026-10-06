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
