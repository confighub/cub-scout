// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

const customResourceConfigEnvVar = "CUB_SCOUT_RESOURCE_CONFIG"

var defaultMapWatchResources = []schema.GroupVersionResource{
	{Group: "apps", Version: "v1", Resource: "deployments"},
	{Group: "apps", Version: "v1", Resource: "statefulsets"},
	{Group: "apps", Version: "v1", Resource: "daemonsets"},
	{Group: "", Version: "v1", Resource: "services"},
	{Group: "", Version: "v1", Resource: "configmaps"},
	{Group: "", Version: "v1", Resource: "secrets"},
	{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
	{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"},
	{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"},
	{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"},
	{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"},
}

type customResourcesFile struct {
	Resources []customResourceSpec `json:"resources" yaml:"resources"`
}

type customResourceSpec struct {
	Group    string                    `json:"group" yaml:"group"`
	Version  string                    `json:"version" yaml:"version"`
	Resource string                    `json:"resource" yaml:"resource"`
	Status   *customResourceStatusSpec `json:"status,omitempty" yaml:"status,omitempty"`
}

type customResourceStatusSpec struct {
	HealthPath        string   `json:"healthPath,omitempty" yaml:"healthPath,omitempty"`
	HealthyValues     []string `json:"healthyValues,omitempty" yaml:"healthyValues,omitempty"`
	DegradedValues    []string `json:"degradedValues,omitempty" yaml:"degradedValues,omitempty"`
	ProgressingValues []string `json:"progressingValues,omitempty" yaml:"progressingValues,omitempty"`
}

type customResourceConfig struct {
	GVR schema.GroupVersionResource
}

type customResourceConfigCache struct {
	mu          sync.RWMutex
	path        string
	modTime     time.Time
	size        int64
	configs     []customResourceConfig
	warnedToken string
}

var mapWatchCustomResourceCache = customResourceConfigCache{}
var mapWatchLoadCustomResourceConfigs = loadCustomResourceConfigs

func collectMapResourceList() []schema.GroupVersionResource {
	return buildMapWatchResourceList()
}

func collectWatchResourceList() []schema.GroupVersionResource {
	return buildMapWatchResourceList()
}

func buildMapWatchResourceList() []schema.GroupVersionResource {
	configs := mapWatchLoadCustomResourceConfigs()
	out := append([]schema.GroupVersionResource(nil), defaultMapWatchResources...)

	seen := map[string]struct{}{}
	for _, gvr := range out {
		seen[resourceKey(gvr)] = struct{}{}
	}
	for _, config := range configs {
		key := resourceKey(config.GVR)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, config.GVR)
	}

	return out
}

func loadCustomResourceConfigs() []customResourceConfig {
	path := resolveCustomResourceConfigPath()
	if path == "" {
		return nil
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			mapWatchCustomResourceCache.mu.Lock()
			mapWatchCustomResourceCache.path = path
			mapWatchCustomResourceCache.modTime = time.Time{}
			mapWatchCustomResourceCache.size = 0
			mapWatchCustomResourceCache.configs = nil
			mapWatchCustomResourceCache.warnedToken = ""
			mapWatchCustomResourceCache.mu.Unlock()
			return nil
		}
		warnCustomResourceConfig(path, time.Time{}, 0, fmt.Sprintf("failed to stat config: %v", err))
		return nil
	}

	mapWatchCustomResourceCache.mu.RLock()
	if mapWatchCustomResourceCache.path == path &&
		mapWatchCustomResourceCache.modTime.Equal(info.ModTime()) &&
		mapWatchCustomResourceCache.size == info.Size() {
		configs := append([]customResourceConfig(nil), mapWatchCustomResourceCache.configs...)
		mapWatchCustomResourceCache.mu.RUnlock()
		return configs
	}
	mapWatchCustomResourceCache.mu.RUnlock()

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		warnCustomResourceConfig(path, info.ModTime(), info.Size(), fmt.Sprintf("failed to read config: %v", readErr))
		return nil
	}

	var parsed customResourcesFile
	if unmarshalErr := yaml.Unmarshal(data, &parsed); unmarshalErr != nil {
		warnCustomResourceConfig(path, info.ModTime(), info.Size(), fmt.Sprintf("invalid YAML: %v", unmarshalErr))
		return nil
	}

	configs := compileCustomResourceConfigs(parsed.Resources)

	mapWatchCustomResourceCache.mu.Lock()
	mapWatchCustomResourceCache.path = path
	mapWatchCustomResourceCache.modTime = info.ModTime()
	mapWatchCustomResourceCache.size = info.Size()
	mapWatchCustomResourceCache.configs = configs
	mapWatchCustomResourceCache.warnedToken = ""
	mapWatchCustomResourceCache.mu.Unlock()

	return append([]customResourceConfig(nil), configs...)
}

func compileCustomResourceConfigs(specs []customResourceSpec) []customResourceConfig {
	if len(specs) == 0 {
		return nil
	}

	out := make([]customResourceConfig, 0, len(specs))
	for _, spec := range specs {
		group := strings.TrimSpace(spec.Group)
		version := strings.TrimSpace(spec.Version)
		resource := strings.TrimSpace(spec.Resource)
		if group == "" || version == "" || resource == "" {
			continue
		}

		out = append(out, customResourceConfig{
			GVR: schema.GroupVersionResource{
				Group:    group,
				Version:  version,
				Resource: resource,
			},
		})
	}
	return out
}

func resolveCustomResourceConfigPath() string {
	if override := strings.TrimSpace(os.Getenv(customResourceConfigEnvVar)); override != "" {
		return override
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	return filepath.Join(home, ".cub-scout", "resources.yaml")
}

func warnCustomResourceConfig(path string, modTime time.Time, size int64, reason string) {
	token := fmt.Sprintf("%s|%d|%d|%s", path, modTime.UnixNano(), size, reason)

	mapWatchCustomResourceCache.mu.Lock()
	defer mapWatchCustomResourceCache.mu.Unlock()

	if mapWatchCustomResourceCache.warnedToken == token {
		return
	}

	mapWatchCustomResourceCache.warnedToken = token
	mapWatchCustomResourceCache.configs = nil
	mapWatchCustomResourceCache.path = path
	mapWatchCustomResourceCache.modTime = modTime
	mapWatchCustomResourceCache.size = size

	fmt.Fprintf(os.Stderr, "Warning: custom resource config ignored (%s): %s\n", path, reason)
}

func resourceKey(gvr schema.GroupVersionResource) string {
	return strings.TrimSpace(gvr.Group) + "|" + strings.TrimSpace(gvr.Version) + "|" + strings.TrimSpace(gvr.Resource)
}

func resetCustomResourceConfigCacheForTest() {
	mapWatchCustomResourceCache.mu.Lock()
	defer mapWatchCustomResourceCache.mu.Unlock()
	mapWatchCustomResourceCache.path = ""
	mapWatchCustomResourceCache.modTime = time.Time{}
	mapWatchCustomResourceCache.size = 0
	mapWatchCustomResourceCache.configs = nil
	mapWatchCustomResourceCache.warnedToken = ""
}
