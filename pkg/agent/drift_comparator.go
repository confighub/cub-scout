// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package agent provides drift detection for v0.14.4.
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

	// IncludeResources enables resource requests/limits comparison
	IncludeResources bool

	// IncludePullPolicy enables image pull policy comparison
	IncludePullPolicy bool
}

// DefaultDriftOptions returns sensible defaults for v0.14.4.
func DefaultDriftOptions() DriftOptions {
	return DriftOptions{
		IncludeReplicas:   true,
		IncludeImages:     true,
		IncludeEnv:        true, // v0.14.4 PR1
		IncludeResources:  true, // v0.14.4 PR2
		IncludePullPolicy: true, // v0.14.4 PR3
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

	// Compare environment variables (v0.14.4+)
	if c.options.IncludeEnv {
		envFindings := c.compareEnvVars(objID, desired, live)
		findings = append(findings, envFindings...)
	}

	// Compare resource requests/limits (v0.14.4+)
	if c.options.IncludeResources {
		resourceFindings := c.compareResources(objID, desired, live)
		findings = append(findings, resourceFindings...)
	}

	// Compare image pull policy (v0.14.4+)
	if c.options.IncludePullPolicy {
		policyFindings := c.comparePullPolicy(objID, desired, live)
		findings = append(findings, policyFindings...)
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

// compareEnvVars compares environment variables between desired and live containers.
// Findings use path format: spec.template.spec.containers[name=<container>].env[name=<VAR>]
// Classification: config, Severity: warning
func (c *DriftComparator) compareEnvVars(objID DriftObjectID, desired, live map[string]interface{}) []DriftFinding {
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

	// Compare each desired container's env vars
	for _, desiredContainer := range desiredContainers {
		containerName, _ := getStringField(desiredContainer, "name")
		if containerName == "" {
			continue
		}

		liveContainer, exists := liveByName[containerName]
		if !exists {
			// Container doesn't exist in live - skip (container drift is different)
			continue
		}

		// Extract and compare env vars
		desiredEnv := extractEnvVars(desiredContainer)
		liveEnv := extractEnvVars(liveContainer)

		// Build map of live env vars by name
		liveEnvByName := make(map[string]string)
		for _, e := range liveEnv {
			liveEnvByName[e.Name] = e.Value
		}

		// Track which live vars we've seen (for detecting removed vars)
		seenLiveVars := make(map[string]bool)

		// Check desired vars against live (sorted for determinism)
		sortEnvVarsByName(desiredEnv)
		for _, dv := range desiredEnv {
			path := fmt.Sprintf("spec.template.spec.containers[name=%s].env[name=%s]", containerName, dv.Name)

			liveValue, existsInLive := liveEnvByName[dv.Name]
			seenLiveVars[dv.Name] = true

			if !existsInLive {
				// Var in desired but not in live (removed)
				finding := NewDriftFinding(
					objID,
					path,
					dv.Value,
					nil, // nil indicates missing
					DriftConfig,
					DriftSeverityWarning,
				)
				findings = append(findings, finding)
			} else if dv.Value != liveValue {
				// Var exists but value changed
				finding := NewDriftFinding(
					objID,
					path,
					dv.Value,
					liveValue,
					DriftConfig,
					DriftSeverityWarning,
				)
				findings = append(findings, finding)
			}
		}

		// Check for vars in live but not in desired (added vars)
		sortEnvVarsByName(liveEnv)
		for _, lv := range liveEnv {
			if !seenLiveVars[lv.Name] {
				path := fmt.Sprintf("spec.template.spec.containers[name=%s].env[name=%s]", containerName, lv.Name)
				finding := NewDriftFinding(
					objID,
					path,
					nil, // nil indicates missing in desired
					lv.Value,
					DriftConfig,
					DriftSeverityWarning,
				)
				findings = append(findings, finding)
			}
		}
	}

	return findings
}

// envVar represents an environment variable name-value pair.
type envVar struct {
	Name  string
	Value string
}

// extractEnvVars extracts environment variables from a container.
func extractEnvVars(container map[string]interface{}) []envVar {
	envList, ok := container["env"].([]interface{})
	if !ok {
		return nil
	}

	var vars []envVar
	for _, e := range envList {
		envMap, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := getStringField(envMap, "name")
		value, _ := getStringField(envMap, "value")
		if name != "" {
			vars = append(vars, envVar{Name: name, Value: value})
		}
	}
	return vars
}

// sortEnvVarsByName sorts env vars by name for deterministic comparison.
func sortEnvVarsByName(vars []envVar) {
	for i := 0; i < len(vars)-1; i++ {
		for j := i + 1; j < len(vars); j++ {
			if vars[i].Name > vars[j].Name {
				vars[i], vars[j] = vars[j], vars[i]
			}
		}
	}
}

// compareResources compares resource requests/limits between desired and live containers.
// Findings use path format: spec.template.spec.containers[name=<container>].resources.<type>.<resource>
// Classification: capacity
// Severity: warning (normal drift), critical (invalid configs like limits < requests)
func (c *DriftComparator) compareResources(objID DriftObjectID, desired, live map[string]interface{}) []DriftFinding {
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

	// Compare each desired container's resources
	for _, desiredContainer := range desiredContainers {
		containerName, _ := getStringField(desiredContainer, "name")
		if containerName == "" {
			continue
		}

		liveContainer, exists := liveByName[containerName]
		if !exists {
			continue
		}

		// Extract resources
		desiredResources := extractResources(desiredContainer)
		liveResources := extractResources(liveContainer)

		// Compare requests (sorted for determinism: cpu, memory)
		for _, resourceName := range []string{"cpu", "memory"} {
			desiredReq := desiredResources.Requests[resourceName]
			liveReq := liveResources.Requests[resourceName]

			if desiredReq != liveReq && (desiredReq != "" || liveReq != "") {
				path := fmt.Sprintf("spec.template.spec.containers[name=%s].resources.requests.%s", containerName, resourceName)
				severity := classifyResourceSeverity(desiredReq, liveReq, desiredResources.Limits[resourceName], liveResources.Limits[resourceName])
				finding := NewDriftFinding(
					objID,
					path,
					normalizeResourceValue(desiredReq),
					normalizeResourceValue(liveReq),
					DriftCapacity,
					severity,
				)
				findings = append(findings, finding)
			}
		}

		// Compare limits (sorted for determinism: cpu, memory)
		for _, resourceName := range []string{"cpu", "memory"} {
			desiredLim := desiredResources.Limits[resourceName]
			liveLim := liveResources.Limits[resourceName]

			if desiredLim != liveLim && (desiredLim != "" || liveLim != "") {
				path := fmt.Sprintf("spec.template.spec.containers[name=%s].resources.limits.%s", containerName, resourceName)
				severity := classifyResourceSeverity(desiredResources.Requests[resourceName], liveResources.Requests[resourceName], desiredLim, liveLim)
				finding := NewDriftFinding(
					objID,
					path,
					normalizeResourceValue(desiredLim),
					normalizeResourceValue(liveLim),
					DriftCapacity,
					severity,
				)
				findings = append(findings, finding)
			}
		}
	}

	return findings
}

// containerResources holds extracted resource requests and limits.
type containerResources struct {
	Requests map[string]string
	Limits   map[string]string
}

// extractResources extracts resource requests and limits from a container.
func extractResources(container map[string]interface{}) containerResources {
	result := containerResources{
		Requests: make(map[string]string),
		Limits:   make(map[string]string),
	}

	resources, ok := container["resources"].(map[string]interface{})
	if !ok {
		return result
	}

	// Extract requests
	if requests, ok := resources["requests"].(map[string]interface{}); ok {
		for k, v := range requests {
			if s, ok := v.(string); ok {
				result.Requests[k] = s
			}
		}
	}

	// Extract limits
	if limits, ok := resources["limits"].(map[string]interface{}); ok {
		for k, v := range limits {
			if s, ok := v.(string); ok {
				result.Limits[k] = s
			}
		}
	}

	return result
}

// classifyResourceSeverity determines severity for resource drift.
// Returns critical if the live config is invalid (e.g., limits < requests).
// Returns warning for normal drift.
func classifyResourceSeverity(desiredReq, liveReq, desiredLim, liveLim string) DriftSeverity {
	// Check if live config is invalid (limits < requests)
	if liveReq != "" && liveLim != "" {
		reqValue := parseResourceQuantity(liveReq)
		limValue := parseResourceQuantity(liveLim)
		if reqValue > 0 && limValue > 0 && limValue < reqValue {
			return DriftSeverityCritical
		}
	}
	return DriftSeverityWarning
}

// parseResourceQuantity parses a Kubernetes resource quantity to a comparable float.
// This is a simplified parser for common formats (e.g., "100m", "1Gi", "500Mi").
func parseResourceQuantity(s string) float64 {
	if s == "" {
		return 0
	}

	// Handle CPU millicores (e.g., "100m", "500m")
	if strings.HasSuffix(s, "m") {
		val, err := strconv.ParseFloat(strings.TrimSuffix(s, "m"), 64)
		if err == nil {
			return val / 1000
		}
	}

	// Handle memory with suffixes
	multipliers := map[string]float64{
		"Ki": 1024,
		"Mi": 1024 * 1024,
		"Gi": 1024 * 1024 * 1024,
		"Ti": 1024 * 1024 * 1024 * 1024,
		"K":  1000,
		"M":  1000 * 1000,
		"G":  1000 * 1000 * 1000,
		"T":  1000 * 1000 * 1000 * 1000,
	}

	for suffix, mult := range multipliers {
		if strings.HasSuffix(s, suffix) {
			val, err := strconv.ParseFloat(strings.TrimSuffix(s, suffix), 64)
			if err == nil {
				return val * mult
			}
		}
	}

	// Try parsing as plain number
	val, err := strconv.ParseFloat(s, 64)
	if err == nil {
		return val
	}

	return 0
}

// normalizeResourceValue returns nil for empty strings, otherwise the value.
func normalizeResourceValue(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// comparePullPolicy compares imagePullPolicy between desired and live containers.
// Findings use path format: spec.template.spec.containers[name=<container>].imagePullPolicy
// Classification: rollout, Severity: warning
func (c *DriftComparator) comparePullPolicy(objID DriftObjectID, desired, live map[string]interface{}) []DriftFinding {
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

	// Compare each desired container's pull policy
	for _, desiredContainer := range desiredContainers {
		containerName, _ := getStringField(desiredContainer, "name")
		if containerName == "" {
			continue
		}

		liveContainer, exists := liveByName[containerName]
		if !exists {
			continue
		}

		// Get pull policies (note: Kubernetes defaults to IfNotPresent for tagged images)
		desiredPolicy, _ := getStringField(desiredContainer, "imagePullPolicy")
		livePolicy, _ := getStringField(liveContainer, "imagePullPolicy")

		// Only report drift if both are explicitly set and different,
		// or if one is set and the other isn't
		if desiredPolicy != livePolicy && (desiredPolicy != "" || livePolicy != "") {
			path := fmt.Sprintf("spec.template.spec.containers[name=%s].imagePullPolicy", containerName)
			finding := NewDriftFinding(
				objID,
				path,
				normalizePolicyValue(desiredPolicy),
				normalizePolicyValue(livePolicy),
				DriftRollout,
				DriftSeverityWarning,
			)
			findings = append(findings, finding)
		}
	}

	return findings
}

// normalizePolicyValue returns nil for empty strings, otherwise the value.
func normalizePolicyValue(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
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
