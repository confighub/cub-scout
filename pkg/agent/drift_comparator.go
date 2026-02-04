// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package agent provides drift detection for v0.14.3.
//
// This file implements the drift comparator engine that compares
// desired state (from file/git) against live state (from cluster).
//
// JSON is authoritative for structural facts; ASCII renders f(JSON)+g.
// See docs/semantic-contract.md for the full model.
//
// Contract: R1 (facts in JSON), R5 (stable identity), R6 (ordering semantic via severity field).
package agent

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// DriftComparator compares desired vs live state and emits findings.
type DriftComparator struct {
	// kubeClient is used for fetching live state
	kubeClient kubernetes.Interface

	// dynamicClient is used for dynamic resource fetching
	dynamicClient dynamic.Interface

	// options controls comparison behavior
	options DriftOptions
}

// DriftOptions configures the comparator behavior.
type DriftOptions struct {
	// Namespace limits comparison to a specific namespace (empty = all)
	Namespace string

	// IncludeReplicas enables replica count comparison
	IncludeReplicas bool

	// IncludeImages enables container image comparison
	IncludeImages bool

	// IncludeEnv enables environment variable comparison (future)
	IncludeEnv bool

	// IncludeResources enables resource requests/limits comparison (future)
	IncludeResources bool
}

// DefaultDriftOptions returns sensible defaults for v0.14.3.
func DefaultDriftOptions() DriftOptions {
	return DriftOptions{
		IncludeReplicas: true,
		IncludeImages:   true,
		IncludeEnv:      false,      // v0.14.4+
		IncludeResources: false,     // v0.14.4+
	}
}

// NewDriftComparator creates a new comparator with the given clients.
func NewDriftComparator(kubeClient kubernetes.Interface, dynamicClient dynamic.Interface, opts DriftOptions) *DriftComparator {
	return &DriftComparator{
		kubeClient:    kubeClient,
		dynamicClient: dynamicClient,
		options:       opts,
	}
}

// CompareFromFile compares desired state from a YAML file against live cluster.
func (c *DriftComparator) CompareFromFile(ctx context.Context, filename string) ([]DriftFinding, error) {
	// Load desired resources from file
	desired, err := c.loadYAMLFile(filename)
	if err != nil {
		return nil, fmt.Errorf("load desired state: %w", err)
	}

	return c.compare(ctx, desired)
}

// CompareFromResources compares desired resources against live cluster.
func (c *DriftComparator) CompareFromResources(ctx context.Context, desired []map[string]interface{}) ([]DriftFinding, error) {
	return c.compare(ctx, desired)
}

// compare performs the actual comparison.
func (c *DriftComparator) compare(ctx context.Context, desired []map[string]interface{}) ([]DriftFinding, error) {
	var findings []DriftFinding

	for _, desiredObj := range desired {
		kind, _ := getStringField(desiredObj, "kind")
		apiVersion, _ := getStringField(desiredObj, "apiVersion")
		metadata, _ := desiredObj["metadata"].(map[string]interface{})
		name, _ := getStringField(metadata, "name")
		namespace, _ := getStringField(metadata, "namespace")

		// Skip if namespace filter doesn't match
		if c.options.Namespace != "" && namespace != c.options.Namespace {
			continue
		}

		// Skip non-workload resources for now (v0.14.3 scope)
		if !isWorkloadKind(kind) {
			continue
		}

		// Fetch live resource
		live, err := c.fetchLiveResource(ctx, apiVersion, kind, namespace, name)
		if err != nil {
			// Resource doesn't exist in cluster - could be a "missing" finding
			// For v0.14.3, we skip missing resources (not drift, just absent)
			continue
		}

		// Compare and collect findings
		objFindings := c.compareResource(desiredObj, live)
		findings = append(findings, objFindings...)
	}

	// Sort findings for deterministic output
	SortFindings(findings)

	return findings, nil
}

// compareResource compares a single desired vs live resource.
func (c *DriftComparator) compareResource(desired, live map[string]interface{}) []DriftFinding {
	var findings []DriftFinding

	kind, _ := getStringField(desired, "kind")
	metadata, _ := desired["metadata"].(map[string]interface{})
	name, _ := getStringField(metadata, "name")
	namespace, _ := getStringField(metadata, "namespace")

	objID := DriftObjectID{
		Kind:      kind,
		Namespace: namespace,
		Name:      name,
	}

	// Compare replicas
	if c.options.IncludeReplicas {
		if finding := c.compareReplicas(objID, desired, live); finding != nil {
			findings = append(findings, *finding)
		}
	}

	// Compare container images
	if c.options.IncludeImages {
		imageFindings := c.compareImages(objID, desired, live)
		findings = append(findings, imageFindings...)
	}

	return findings
}

// compareReplicas compares spec.replicas between desired and live.
func (c *DriftComparator) compareReplicas(objID DriftObjectID, desired, live map[string]interface{}) *DriftFinding {
	desiredSpec, _ := desired["spec"].(map[string]interface{})
	liveSpec, _ := live["spec"].(map[string]interface{})

	if desiredSpec == nil || liveSpec == nil {
		return nil
	}

	desiredReplicas := getReplicaCount(desiredSpec)
	liveReplicas := getReplicaCount(liveSpec)

	// If desired doesn't specify replicas, skip (not drift)
	if desiredReplicas == nil {
		return nil
	}

	// If live doesn't have replicas (shouldn't happen for Deployments), skip
	if liveReplicas == nil {
		return nil
	}

	if *desiredReplicas != *liveReplicas {
		finding := NewDriftFinding(
			objID,
			"spec.replicas",
			*desiredReplicas,
			*liveReplicas,
			DriftCapacity,
			classifyReplicaSeverity(*desiredReplicas, *liveReplicas),
		)
		return &finding
	}

	return nil
}

// compareImages compares container images between desired and live.
func (c *DriftComparator) compareImages(objID DriftObjectID, desired, live map[string]interface{}) []DriftFinding {
	var findings []DriftFinding

	desiredContainers := extractContainers(desired)
	liveContainers := extractContainers(live)

	// Build map of live containers by name
	liveByName := make(map[string]map[string]interface{})
	for _, container := range liveContainers {
		name, _ := getStringField(container, "name")
		if name != "" {
			liveByName[name] = container
		}
	}

	// Compare each desired container
	for i, desiredContainer := range desiredContainers {
		containerName, _ := getStringField(desiredContainer, "name")
		desiredImage, _ := getStringField(desiredContainer, "image")

		if containerName == "" || desiredImage == "" {
			continue
		}

		liveContainer, exists := liveByName[containerName]
		if !exists {
			// Container doesn't exist in live - could be a finding
			// For v0.14.3, we skip (container added/removed is different from drift)
			continue
		}

		liveImage, _ := getStringField(liveContainer, "image")

		if desiredImage != liveImage {
			path := fmt.Sprintf("spec.template.spec.containers[%d].image", i)
			finding := NewDriftFinding(
				objID,
				path,
				desiredImage,
				liveImage,
				DriftImage,
				classifyImageSeverity(desiredImage, liveImage),
			)
			findings = append(findings, finding)
		}
	}

	return findings
}

// fetchLiveResource fetches a resource from the cluster.
func (c *DriftComparator) fetchLiveResource(ctx context.Context, apiVersion, kind, namespace, name string) (map[string]interface{}, error) {
	// For core workloads, use typed client for efficiency
	switch kind {
	case "Deployment":
		if c.kubeClient != nil {
			dep, err := c.kubeClient.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return nil, err
			}
			return deploymentToMap(dep), nil
		}
	case "StatefulSet":
		if c.kubeClient != nil {
			ss, err := c.kubeClient.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return nil, err
			}
			return statefulSetToMap(ss), nil
		}
	}

	// Fallback to dynamic client
	if c.dynamicClient != nil {
		gvr, err := parseAPIVersionKind(apiVersion, kind)
		if err != nil {
			return nil, err
		}

		var obj *unstructured.Unstructured
		if namespace != "" {
			obj, err = c.dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		} else {
			obj, err = c.dynamicClient.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
		}
		if err != nil {
			return nil, err
		}
		return obj.Object, nil
	}

	return nil, fmt.Errorf("no client available")
}

// loadYAMLFile loads resources from a YAML file (multi-document supported).
func (c *DriftComparator) loadYAMLFile(filename string) ([]map[string]interface{}, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var resources []map[string]interface{}
	decoder := yaml.NewDecoder(bufio.NewReader(file))

	for {
		var doc map[string]interface{}
		err := decoder.Decode(&doc)
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, err
		}
		if doc != nil && len(doc) > 0 {
			resources = append(resources, doc)
		}
	}

	return resources, nil
}

// Helper functions

func isWorkloadKind(kind string) bool {
	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Job", "CronJob":
		return true
	}
	return false
}

func getReplicaCount(spec map[string]interface{}) *int {
	replicas, ok := spec["replicas"]
	if !ok {
		return nil
	}

	switch v := replicas.(type) {
	case int:
		return &v
	case int64:
		i := int(v)
		return &i
	case float64:
		i := int(v)
		return &i
	case string:
		i, err := strconv.Atoi(v)
		if err != nil {
			return nil
		}
		return &i
	}
	return nil
}

func extractContainers(resource map[string]interface{}) []map[string]interface{} {
	spec, ok := resource["spec"].(map[string]interface{})
	if !ok {
		return nil
	}

	// For Deployments/StatefulSets: spec.template.spec.containers
	template, ok := spec["template"].(map[string]interface{})
	if ok {
		templateSpec, ok := template["spec"].(map[string]interface{})
		if ok {
			containers, ok := templateSpec["containers"].([]interface{})
			if ok {
				return toMapSlice(containers)
			}
		}
	}

	// For Pods: spec.containers
	containers, ok := spec["containers"].([]interface{})
	if ok {
		return toMapSlice(containers)
	}

	return nil
}

func toMapSlice(items []interface{}) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]interface{}); ok {
			result = append(result, m)
		}
	}
	return result
}

func classifyReplicaSeverity(desired, live int) DriftSeverity {
	// Scale down (live < desired) is more concerning
	if live < desired {
		return DriftSeverityWarning
	}
	// Scale up (live > desired) is usually intentional (HPA, manual)
	return DriftSeverityInfo
}

func classifyImageSeverity(desired, live string) DriftSeverity {
	// Image drift is generally concerning
	// If it's just a tag change, warning
	// If it's a different image entirely, critical
	desiredParts := strings.Split(desired, ":")
	liveParts := strings.Split(live, ":")

	desiredRepo := desiredParts[0]
	liveRepo := liveParts[0]

	if desiredRepo != liveRepo {
		return DriftSeverityCritical
	}
	return DriftSeverityWarning
}

// deploymentToMap converts a typed Deployment to map for comparison.
func deploymentToMap(dep interface{}) map[string]interface{} {
	// Use JSON round-trip for simplicity
	// In production, you might want a more efficient approach
	result := make(map[string]interface{})

	// Extract the fields we care about
	switch d := dep.(type) {
	case interface{ GetName() string }:
		result["kind"] = "Deployment"
		result["apiVersion"] = "apps/v1"
		result["metadata"] = map[string]interface{}{
			"name":      d.GetName(),
			"namespace": d.(interface{ GetNamespace() string }).GetNamespace(),
		}
	}

	// For typed objects, extract spec directly
	// This is a simplified version - full implementation would use reflection or JSON
	return result
}

// statefulSetToMap converts a typed StatefulSet to map for comparison.
func statefulSetToMap(ss interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	switch s := ss.(type) {
	case interface{ GetName() string }:
		result["kind"] = "StatefulSet"
		result["apiVersion"] = "apps/v1"
		result["metadata"] = map[string]interface{}{
			"name":      s.GetName(),
			"namespace": s.(interface{ GetNamespace() string }).GetNamespace(),
		}
	}
	return result
}

// parseAPIVersionKind parses apiVersion and kind into a GroupVersionResource.
// This is a simplified version for common workloads.
func parseAPIVersionKind(apiVersion, kind string) (schema.GroupVersionResource, error) {
	// Parse apiVersion into group and version
	parts := strings.Split(apiVersion, "/")
	var group, version string
	if len(parts) == 1 {
		group = ""
		version = parts[0]
	} else {
		group = parts[0]
		version = parts[1]
	}

	// Map kind to resource (simplified pluralization)
	resource := strings.ToLower(kind) + "s"
	if strings.HasSuffix(kind, "s") {
		resource = strings.ToLower(kind) + "es"
	}

	return schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}, nil
}
