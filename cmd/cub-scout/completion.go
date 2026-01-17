// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// Namespace completion cache (avoid repeated API calls during tab-complete)
var (
	cachedNamespaces     []string
	namespaceCacheExpiry time.Time
	namespaceCacheMu     sync.Mutex
)

// completeNamespaces returns available namespaces from current kubectl context
func completeNamespaces(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	namespaceCacheMu.Lock()
	defer namespaceCacheMu.Unlock()

	// Return cache if fresh (3 second TTL)
	if time.Now().Before(namespaceCacheExpiry) && len(cachedNamespaces) > 0 {
		return filterPrefix(cachedNamespaces, toComplete), cobra.ShellCompDirectiveNoFileComp
	}

	cfg, err := buildConfig()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Quick timeout for completion - don't block shell
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	list, err := dynClient.Resource(schema.GroupVersionResource{
		Version:  "v1",
		Resource: "namespaces",
	}).List(ctx, v1.ListOptions{})
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var namespaces []string
	for _, item := range list.Items {
		namespaces = append(namespaces, item.GetName())
	}

	// Update cache
	cachedNamespaces = namespaces
	namespaceCacheExpiry = time.Now().Add(3 * time.Second)

	return filterPrefix(namespaces, toComplete), cobra.ShellCompDirectiveNoFileComp
}

// completeKinds returns common Kubernetes resource kinds
func completeKinds(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Common workload and config kinds that cub-agent typically queries
	kinds := []string{
		// Workloads
		"Deployment",
		"StatefulSet",
		"DaemonSet",
		"ReplicaSet",
		"Pod",
		"Job",
		"CronJob",
		// Config
		"ConfigMap",
		"Secret",
		// Networking
		"Service",
		"Ingress",
		"NetworkPolicy",
		// Storage
		"PersistentVolumeClaim",
		// Flux
		"GitRepository",
		"Kustomization",
		"HelmRelease",
		"HelmRepository",
		// Argo CD
		"Application",
		"ApplicationSet",
		"AppProject",
	}
	return filterPrefix(kinds, toComplete), cobra.ShellCompDirectiveNoFileComp
}

// completeOwners returns valid owner types for --owner flag
func completeOwners(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	owners := []string{
		"Flux",
		"ArgoCD",
		"Helm",
		"ConfigHub",
		"Native",
	}
	return filterPrefix(owners, toComplete), cobra.ShellCompDirectiveNoFileComp
}


// filterPrefix filters strings by prefix (case-insensitive)
func filterPrefix(items []string, prefix string) []string {
	if prefix == "" {
		return items
	}
	var filtered []string
	lowerPrefix := strings.ToLower(prefix)
	for _, item := range items {
		if strings.HasPrefix(strings.ToLower(item), lowerPrefix) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
