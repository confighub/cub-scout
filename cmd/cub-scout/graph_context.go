// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/confighub/cub-scout/v2/internal/graph"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// No CLI/global flag or subprocess fallback: TUI actions retain captured scope.
func exportGraphFromBinding(binding *localClusterBinding, namespace, format, outputPath string) error {
	if binding == nil || binding.err != nil || binding.config == nil {
		return fmt.Errorf("selected Kubernetes context is unavailable: %v", bindingError(binding))
	}
	client, err := kubernetes.NewForConfig(rest.CopyConfig(binding.config))
	if err != nil {
		return fmt.Errorf("create graph client: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	g := graph.NewGraph(binding.context)
	if err := graph.NewCollector(client, binding.context).CollectOwnershipChain(ctx, g, namespace); err != nil {
		return fmt.Errorf("collect graph: %w", err)
	}
	data, err := renderGraphOutput(g, format, 300)
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, data, 0644)
}
