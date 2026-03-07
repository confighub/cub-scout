// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"sigs.k8s.io/yaml"
)

const customDetectorsEnvVar = "CUB_SCOUT_OWNERSHIP_DETECTORS"

type customMatcher struct {
	Key   string
	Value *string
}

type customOwnershipDetector struct {
	Name        string
	OwnerName   string
	OwnerType   string
	Labels      []customMatcher
	Annotations []customMatcher
}

type customDetectorCache struct {
	mu          sync.RWMutex
	path        string
	modTime     time.Time
	size        int64
	detectors   []customOwnershipDetector
	warnedToken string
}

var ownershipCustomCache = customDetectorCache{}

type customDetectorsFile struct {
	Detectors []customDetectorSpec `json:"detectors" yaml:"detectors"`
}

type customDetectorSpec struct {
	Name        string            `json:"name" yaml:"name"`
	Labels      []customMatchSpec `json:"labels" yaml:"labels"`
	Annotations []customMatchSpec `json:"annotations" yaml:"annotations"`
	OwnerName   string            `json:"owner_name" yaml:"owner_name"`
	OwnerType   string            `json:"owner_type" yaml:"owner_type"`
}

type customMatchSpec struct {
	Key   string  `json:"key" yaml:"key"`
	Value *string `json:"value,omitempty" yaml:"value,omitempty"`
}

func detectCustomOwnership(labels, annotations map[string]string) Ownership {
	detectors := loadCustomOwnershipDetectors()
	for _, detector := range detectors {
		if matchesCustomDetector(detector, labels, annotations) {
			return Ownership{
				Type:       detector.OwnerType,
				SubType:    "custom-detector",
				Name:       detector.OwnerName,
				Source:     "custom:" + detector.Name,
				Confidence: "high",
			}
		}
	}
	return Ownership{}
}

func loadCustomOwnershipDetectors() []customOwnershipDetector {
	path := resolveCustomDetectorsPath()
	if path == "" {
		return nil
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			ownershipCustomCache.mu.Lock()
			ownershipCustomCache.path = path
			ownershipCustomCache.modTime = time.Time{}
			ownershipCustomCache.size = 0
			ownershipCustomCache.detectors = nil
			ownershipCustomCache.warnedToken = ""
			ownershipCustomCache.mu.Unlock()
			return nil
		}
		warnCustomOwnershipConfig(path, time.Time{}, 0, fmt.Sprintf("failed to stat config: %v", err))
		return nil
	}

	ownershipCustomCache.mu.RLock()
	if ownershipCustomCache.path == path &&
		ownershipCustomCache.modTime.Equal(info.ModTime()) &&
		ownershipCustomCache.size == info.Size() {
		detectors := append([]customOwnershipDetector(nil), ownershipCustomCache.detectors...)
		ownershipCustomCache.mu.RUnlock()
		return detectors
	}
	ownershipCustomCache.mu.RUnlock()

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		warnCustomOwnershipConfig(path, info.ModTime(), info.Size(), fmt.Sprintf("failed to read config: %v", readErr))
		return nil
	}

	var parsed customDetectorsFile
	if unmarshalErr := yaml.Unmarshal(data, &parsed); unmarshalErr != nil {
		warnCustomOwnershipConfig(path, info.ModTime(), info.Size(), fmt.Sprintf("invalid YAML: %v", unmarshalErr))
		return nil
	}

	detectors := compileCustomDetectors(parsed.Detectors)

	ownershipCustomCache.mu.Lock()
	ownershipCustomCache.path = path
	ownershipCustomCache.modTime = info.ModTime()
	ownershipCustomCache.size = info.Size()
	ownershipCustomCache.detectors = detectors
	ownershipCustomCache.warnedToken = ""
	ownershipCustomCache.mu.Unlock()

	return append([]customOwnershipDetector(nil), detectors...)
}

func resolveCustomDetectorsPath() string {
	if override := strings.TrimSpace(os.Getenv(customDetectorsEnvVar)); override != "" {
		return override
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	return filepath.Join(home, ".cub-scout", "detectors.yaml")
}

func compileCustomDetectors(specs []customDetectorSpec) []customOwnershipDetector {
	if len(specs) == 0 {
		return nil
	}

	detectors := make([]customOwnershipDetector, 0, len(specs))
	for _, spec := range specs {
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			continue
		}

		ownerName := strings.TrimSpace(spec.OwnerName)
		if ownerName == "" {
			ownerName = name
		}

		ownerType := strings.ToLower(strings.TrimSpace(spec.OwnerType))
		if ownerType == "" {
			ownerType = OwnerCustom
		}

		labels := compileCustomMatchers(spec.Labels)
		annotations := compileCustomMatchers(spec.Annotations)
		if len(labels) == 0 && len(annotations) == 0 {
			continue
		}

		detectors = append(detectors, customOwnershipDetector{
			Name:        name,
			OwnerName:   ownerName,
			OwnerType:   ownerType,
			Labels:      labels,
			Annotations: annotations,
		})
	}

	return detectors
}

func compileCustomMatchers(specs []customMatchSpec) []customMatcher {
	matchers := make([]customMatcher, 0, len(specs))
	for _, spec := range specs {
		key := strings.TrimSpace(spec.Key)
		if key == "" {
			continue
		}
		matchers = append(matchers, customMatcher{
			Key:   key,
			Value: normalizeCustomMatchValue(spec.Value),
		})
	}
	return matchers
}

func normalizeCustomMatchValue(value *string) *string {
	if value == nil {
		return nil
	}
	v := strings.TrimSpace(*value)
	return &v
}

func matchesCustomDetector(detector customOwnershipDetector, labels, annotations map[string]string) bool {
	return matchesAllCustomMatchers(detector.Labels, labels) &&
		matchesAllCustomMatchers(detector.Annotations, annotations)
}

func matchesAllCustomMatchers(matchers []customMatcher, values map[string]string) bool {
	if len(matchers) == 0 {
		return true
	}
	for _, matcher := range matchers {
		got, ok := values[matcher.Key]
		if !ok {
			return false
		}
		if matcher.Value != nil && got != *matcher.Value {
			return false
		}
	}
	return true
}

func warnCustomOwnershipConfig(path string, modTime time.Time, size int64, reason string) {
	token := fmt.Sprintf("%s|%d|%d|%s", path, modTime.UnixNano(), size, reason)

	ownershipCustomCache.mu.Lock()
	defer ownershipCustomCache.mu.Unlock()

	if ownershipCustomCache.warnedToken == token {
		return
	}
	ownershipCustomCache.warnedToken = token
	ownershipCustomCache.detectors = nil
	ownershipCustomCache.path = path
	ownershipCustomCache.modTime = modTime
	ownershipCustomCache.size = size

	fmt.Fprintf(os.Stderr, "Warning: custom ownership detector config ignored (%s): %s\n", path, reason)
}
