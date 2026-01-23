// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package query

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// DriftedResource represents a resource that has drifted from its declared state
type DriftedResource struct {
	Resource ResourceID    `json:"resource"`
	Changes  []DriftChange `json:"changes"`
}

// DriftChange represents a single change between declared and live state
type DriftChange struct {
	Path     string      `json:"path"`
	Declared interface{} `json:"declared"`
	Live     interface{} `json:"live"`
}

// DriftDetector detects drift between declared and live state
type DriftDetector struct {
	client dynamic.Interface
}

// NewDriftDetector creates a new drift detector
func NewDriftDetector(client dynamic.Interface) *DriftDetector {
	return &DriftDetector{client: client}
}

// FieldsToIgnore contains fields that should be ignored when comparing
var FieldsToIgnore = map[string]bool{
	"metadata.resourceVersion":   true,
	"metadata.uid":               true,
	"metadata.generation":        true,
	"metadata.creationTimestamp": true,
	"metadata.managedFields":     true,
	"metadata.selfLink":          true,
	"metadata.annotations.kubectl.kubernetes.io/last-applied-configuration": true,
	"status": true,
}

// FindDriftedResources finds all resources that have drifted from their declared state
func (dd *DriftDetector) FindDriftedResources(ctx context.Context, namespace string) ([]DriftedResource, error) {
	var drifted []DriftedResource

	// Check common workload types
	gvrs := []schema.GroupVersionResource{
		{Group: "apps", Version: "v1", Resource: "deployments"},
		{Group: "apps", Version: "v1", Resource: "statefulsets"},
		{Group: "apps", Version: "v1", Resource: "daemonsets"},
		{Version: "v1", Resource: "services"},
		{Version: "v1", Resource: "configmaps"},
	}

	for _, gvr := range gvrs {
		var list *unstructured.UnstructuredList
		var err error
		if namespace != "" {
			list, err = dd.client.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
		} else {
			list, err = dd.client.Resource(gvr).List(ctx, metav1.ListOptions{})
		}
		if err != nil {
			continue
		}

		for _, item := range list.Items {
			drift, err := dd.DetectDrift(&item)
			if err != nil || drift == nil {
				continue
			}
			if len(drift.Changes) > 0 {
				drifted = append(drifted, *drift)
			}
		}
	}

	return drifted, nil
}

// DetectDrift compares the live resource against the last-applied-configuration annotation
func (dd *DriftDetector) DetectDrift(resource *unstructured.Unstructured) (*DriftedResource, error) {
	annotations := resource.GetAnnotations()
	if annotations == nil {
		return nil, nil
	}

	lastApplied, ok := annotations["kubectl.kubernetes.io/last-applied-configuration"]
	if !ok || lastApplied == "" {
		return nil, nil // No last-applied-configuration, can't detect drift
	}

	// Parse the last-applied-configuration
	var declared map[string]interface{}
	if err := json.Unmarshal([]byte(lastApplied), &declared); err != nil {
		return nil, fmt.Errorf("failed to parse last-applied-configuration: %w", err)
	}

	// Get the live state
	live := resource.Object

	// Compare
	changes := dd.compare("", declared, live)

	if len(changes) == 0 {
		return nil, nil
	}

	return &DriftedResource{
		Resource: ResourceID{
			Kind:      resource.GetKind(),
			Name:      resource.GetName(),
			Namespace: resource.GetNamespace(),
		},
		Changes: changes,
	}, nil
}

// compare recursively compares two maps and returns differences
func (dd *DriftDetector) compare(path string, declared, live interface{}) []DriftChange {
	var changes []DriftChange

	// Check if this path should be ignored
	if dd.shouldIgnore(path) {
		return nil
	}

	// Handle nil cases
	if declared == nil && live == nil {
		return nil
	}
	if declared == nil {
		// Live has something declared doesn't - this is drift
		changes = append(changes, DriftChange{
			Path:     path,
			Declared: nil,
			Live:     live,
		})
		return changes
	}
	if live == nil {
		// Declared has something live doesn't - this is drift
		changes = append(changes, DriftChange{
			Path:     path,
			Declared: declared,
			Live:     nil,
		})
		return changes
	}

	// Type check
	declaredType := reflect.TypeOf(declared)
	liveType := reflect.TypeOf(live)
	if declaredType != liveType {
		// Handle number type mismatches (float64 vs int64)
		if dd.numbersEqual(declared, live) {
			return nil
		}
		changes = append(changes, DriftChange{
			Path:     path,
			Declared: declared,
			Live:     live,
		})
		return changes
	}

	// Compare based on type
	switch declaredVal := declared.(type) {
	case map[string]interface{}:
		liveMap := live.(map[string]interface{})

		// Check all keys in declared
		for k, v := range declaredVal {
			newPath := dd.joinPath(path, k)
			subChanges := dd.compare(newPath, v, liveMap[k])
			changes = append(changes, subChanges...)
		}

		// Check for keys in live but not in declared
		for k, v := range liveMap {
			if _, exists := declaredVal[k]; !exists {
				newPath := dd.joinPath(path, k)
				if !dd.shouldIgnore(newPath) {
					changes = append(changes, DriftChange{
						Path:     newPath,
						Declared: nil,
						Live:     v,
					})
				}
			}
		}

	case []interface{}:
		liveSlice := live.([]interface{})

		// Simple length check first
		if len(declaredVal) != len(liveSlice) {
			changes = append(changes, DriftChange{
				Path:     path,
				Declared: declaredVal,
				Live:     liveSlice,
			})
			return changes
		}

		// Compare each element
		for i := range declaredVal {
			newPath := fmt.Sprintf("%s[%d]", path, i)
			subChanges := dd.compare(newPath, declaredVal[i], liveSlice[i])
			changes = append(changes, subChanges...)
		}

	default:
		// Scalar comparison
		if !reflect.DeepEqual(declared, live) {
			changes = append(changes, DriftChange{
				Path:     path,
				Declared: declared,
				Live:     live,
			})
		}
	}

	return changes
}

// shouldIgnore returns true if this path should be ignored
func (dd *DriftDetector) shouldIgnore(path string) bool {
	// Exact match
	if FieldsToIgnore[path] {
		return true
	}

	// Prefix match for nested paths
	for ignorePath := range FieldsToIgnore {
		if strings.HasPrefix(path, ignorePath+".") || strings.HasPrefix(path, ignorePath+"[") {
			return true
		}
	}

	// Ignore status entirely
	if path == "status" || strings.HasPrefix(path, "status.") || strings.HasPrefix(path, "status[") {
		return true
	}

	// Ignore certain metadata fields
	if strings.HasPrefix(path, "metadata.managedFields") {
		return true
	}

	return false
}

// joinPath joins path components
func (dd *DriftDetector) joinPath(base, key string) string {
	if base == "" {
		return key
	}
	return base + "." + key
}

// numbersEqual checks if two values are numerically equal despite type differences
func (dd *DriftDetector) numbersEqual(a, b interface{}) bool {
	// Convert to float64 for comparison
	var aFloat, bFloat float64
	var aOk, bOk bool

	switch v := a.(type) {
	case float64:
		aFloat, aOk = v, true
	case int64:
		aFloat, aOk = float64(v), true
	case int:
		aFloat, aOk = float64(v), true
	}

	switch v := b.(type) {
	case float64:
		bFloat, bOk = v, true
	case int64:
		bFloat, bOk = float64(v), true
	case int:
		bFloat, bOk = float64(v), true
	}

	if aOk && bOk {
		return aFloat == bFloat
	}
	return false
}

// FormatDrift returns a human-readable representation of drift
func FormatDrift(drifted []DriftedResource) string {
	if len(drifted) == 0 {
		return "No drift detected"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d drifted resources:\n\n", len(drifted)))

	for _, d := range drifted {
		sb.WriteString(fmt.Sprintf("%s/%s", d.Resource.Kind, d.Resource.Name))
		if d.Resource.Namespace != "" {
			sb.WriteString(fmt.Sprintf(" (ns: %s)", d.Resource.Namespace))
		}
		sb.WriteString("\n")

		for _, c := range d.Changes {
			sb.WriteString(fmt.Sprintf("  %s:\n", c.Path))
			sb.WriteString(fmt.Sprintf("    declared: %v\n", formatValue(c.Declared)))
			sb.WriteString(fmt.Sprintf("    live:     %v\n", formatValue(c.Live)))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func formatValue(v interface{}) string {
	if v == nil {
		return "<not set>"
	}
	switch val := v.(type) {
	case string:
		if len(val) > 50 {
			return val[:50] + "..."
		}
		return val
	case map[string]interface{}, []interface{}:
		b, _ := json.Marshal(val)
		s := string(b)
		if len(s) > 100 {
			return s[:100] + "..."
		}
		return s
	default:
		return fmt.Sprintf("%v", v)
	}
}
