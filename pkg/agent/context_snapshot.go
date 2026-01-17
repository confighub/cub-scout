// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// ContextSnapshot represents a point-in-time view of cluster state optimized for AI consumption
type ContextSnapshot struct {
	SnapshotTime time.Time `json:"snapshot_time"`
	Cluster      string    `json:"cluster"`
	Namespace    string    `json:"namespace,omitempty"` // Empty means all namespaces

	Summary          SnapshotSummary           `json:"summary"`
	CriticalIssues   []CriticalIssue           `json:"critical_issues,omitempty"`
	RecentChanges    []RecentChange            `json:"recent_changes,omitempty"`
	OwnershipBreakdown map[string]int          `json:"ownership_breakdown"`
	DependencyGraph  map[string]DependencyInfo `json:"dependency_graph,omitempty"`
}

// SnapshotSummary provides high-level cluster health
type SnapshotSummary struct {
	TotalResources int `json:"total_resources"`
	Healthy        int `json:"healthy"`
	Degraded       int `json:"degraded"`
	Critical       int `json:"critical"`
	Unmanaged      int `json:"unmanaged"`
}

// CriticalIssue represents a critical problem in the cluster
type CriticalIssue struct {
	Resource    string `json:"resource"`
	Namespace   string `json:"namespace,omitempty"`
	Issue       string `json:"issue"`
	Since       string `json:"since"`
	Owner       string `json:"owner"`
	Explanation string `json:"explanation,omitempty"`
}

// RecentChange represents a recent modification
type RecentChange struct {
	Time     string `json:"time"`
	Resource string `json:"resource"`
	Change   string `json:"change"`
	Source   string `json:"source"`
	Commit   string `json:"commit,omitempty"`
}

// DependencyInfo describes resource dependencies
type DependencyInfo struct {
	DependsOn   []string `json:"depends_on,omitempty"`
	DependedBy  []string `json:"depended_by,omitempty"`
}

// ContextSnapshotBuilder builds context snapshots
type ContextSnapshotBuilder struct {
	client      dynamic.Interface
	clusterName string
}

// NewContextSnapshotBuilder creates a new context snapshot builder
func NewContextSnapshotBuilder(client dynamic.Interface, clusterName string) *ContextSnapshotBuilder {
	return &ContextSnapshotBuilder{
		client:      client,
		clusterName: clusterName,
	}
}

// Build creates a context snapshot for the given namespace (or all namespaces if empty)
func (b *ContextSnapshotBuilder) Build(ctx context.Context, namespace string) (*ContextSnapshot, error) {
	snapshot := &ContextSnapshot{
		SnapshotTime:       time.Now(),
		Cluster:            b.clusterName,
		Namespace:          namespace,
		OwnershipBreakdown: make(map[string]int),
		DependencyGraph:    make(map[string]DependencyInfo),
	}

	// Collect workloads
	workloads, err := b.collectWorkloads(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to collect workloads: %w", err)
	}

	// Analyze workloads
	for _, w := range workloads {
		// Count ownership
		ownership := DetectOwnership(w)
		ownerType := ownership.Type
		if ownerType == OwnerUnknown {
			ownerType = "Native"
		}
		snapshot.OwnershipBreakdown[ownerType]++
		snapshot.Summary.TotalResources++

		// Determine health
		health := b.assessHealth(w)
		switch health {
		case "healthy":
			snapshot.Summary.Healthy++
		case "degraded":
			snapshot.Summary.Degraded++
		case "critical":
			snapshot.Summary.Critical++
			snapshot.CriticalIssues = append(snapshot.CriticalIssues, b.createCriticalIssue(w, ownership))
		}

		// Count unmanaged
		if ownership.Type == OwnerUnknown {
			snapshot.Summary.Unmanaged++
		}
	}

	// Collect recent events for changes
	events, err := b.collectRecentEvents(ctx, namespace, 1*time.Hour)
	if err == nil {
		snapshot.RecentChanges = b.processEvents(events)
	}

	// Build dependency graph for critical issues
	for _, issue := range snapshot.CriticalIssues {
		deps := b.findDependencies(ctx, issue.Resource, issue.Namespace)
		if len(deps.DependsOn) > 0 || len(deps.DependedBy) > 0 {
			key := fmt.Sprintf("%s/%s", issue.Resource, issue.Namespace)
			snapshot.DependencyGraph[key] = deps
		}
	}

	return snapshot, nil
}

// collectWorkloads collects workload resources
func (b *ContextSnapshotBuilder) collectWorkloads(ctx context.Context, namespace string) ([]*unstructured.Unstructured, error) {
	var workloads []*unstructured.Unstructured

	gvrs := []schema.GroupVersionResource{
		{Group: "apps", Version: "v1", Resource: "deployments"},
		{Group: "apps", Version: "v1", Resource: "statefulsets"},
		{Group: "apps", Version: "v1", Resource: "daemonsets"},
	}

	for _, gvr := range gvrs {
		var list *unstructured.UnstructuredList
		var err error
		if namespace != "" {
			list, err = b.client.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
		} else {
			list, err = b.client.Resource(gvr).List(ctx, metav1.ListOptions{})
		}
		if err != nil {
			continue
		}
		for i := range list.Items {
			workloads = append(workloads, &list.Items[i])
		}
	}

	return workloads, nil
}

// assessHealth determines the health of a workload
func (b *ContextSnapshotBuilder) assessHealth(resource *unstructured.Unstructured) string {
	status, _, _ := unstructured.NestedMap(resource.Object, "status")
	if status == nil {
		return "unknown"
	}

	// Check replicas
	replicas, _, _ := unstructured.NestedInt64(status, "replicas")
	readyReplicas, _, _ := unstructured.NestedInt64(status, "readyReplicas")
	availableReplicas, _, _ := unstructured.NestedInt64(status, "availableReplicas")

	if replicas == 0 {
		return "unknown"
	}

	if readyReplicas == 0 && availableReplicas == 0 {
		return "critical"
	}

	if readyReplicas < replicas || availableReplicas < replicas {
		return "degraded"
	}

	return "healthy"
}

// createCriticalIssue creates a critical issue from a workload
func (b *ContextSnapshotBuilder) createCriticalIssue(resource *unstructured.Unstructured, ownership Ownership) CriticalIssue {
	status, _, _ := unstructured.NestedMap(resource.Object, "status")

	replicas, _, _ := unstructured.NestedInt64(status, "replicas")
	readyReplicas, _, _ := unstructured.NestedInt64(status, "readyReplicas")

	owner := ownership.Type
	if owner == OwnerUnknown {
		owner = "Native"
	}
	if ownership.Name != "" {
		owner = fmt.Sprintf("%s/%s", owner, ownership.Name)
	}

	issue := CriticalIssue{
		Resource:  fmt.Sprintf("%s/%s", resource.GetKind(), resource.GetName()),
		Namespace: resource.GetNamespace(),
		Issue:     fmt.Sprintf("%d/%d replicas ready", readyReplicas, replicas),
		Owner:     owner,
	}

	// Try to get more info from conditions
	conditions, _, _ := unstructured.NestedSlice(status, "conditions")
	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		condType, _ := cond["type"].(string)
		condStatus, _ := cond["status"].(string)
		if condType == "Available" && condStatus == "False" {
			if msg, ok := cond["message"].(string); ok {
				issue.Explanation = msg
			}
			if lastTransition, ok := cond["lastTransitionTime"].(string); ok {
				if t, err := time.Parse(time.RFC3339, lastTransition); err == nil {
					issue.Since = formatRelativeTime(time.Since(t))
				}
			}
		}
	}

	return issue
}

// collectRecentEvents collects events from the past duration
func (b *ContextSnapshotBuilder) collectRecentEvents(ctx context.Context, namespace string, duration time.Duration) ([]unstructured.Unstructured, error) {
	eventGVR := schema.GroupVersionResource{Version: "v1", Resource: "events"}

	var list *unstructured.UnstructuredList
	var err error
	if namespace != "" {
		list, err = b.client.Resource(eventGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	} else {
		list, err = b.client.Resource(eventGVR).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().Add(-duration)
	var recent []unstructured.Unstructured

	for _, event := range list.Items {
		lastTimestamp, _, _ := unstructured.NestedString(event.Object, "lastTimestamp")
		if lastTimestamp == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, lastTimestamp)
		if err != nil {
			continue
		}
		if t.After(cutoff) {
			recent = append(recent, event)
		}
	}

	// Sort by time descending
	sort.Slice(recent, func(i, j int) bool {
		ti, _, _ := unstructured.NestedString(recent[i].Object, "lastTimestamp")
		tj, _, _ := unstructured.NestedString(recent[j].Object, "lastTimestamp")
		return ti > tj
	})

	// Limit to 20 most recent
	if len(recent) > 20 {
		recent = recent[:20]
	}

	return recent, nil
}

// processEvents converts events to RecentChanges
func (b *ContextSnapshotBuilder) processEvents(events []unstructured.Unstructured) []RecentChange {
	var changes []RecentChange

	for _, event := range events {
		involvedObject, _, _ := unstructured.NestedMap(event.Object, "involvedObject")
		if involvedObject == nil {
			continue
		}

		kind, _ := involvedObject["kind"].(string)
		name, _ := involvedObject["name"].(string)
		reason, _, _ := unstructured.NestedString(event.Object, "reason")
		message, _, _ := unstructured.NestedString(event.Object, "message")
		lastTimestamp, _, _ := unstructured.NestedString(event.Object, "lastTimestamp")

		// Parse timestamp to relative time
		timeStr := lastTimestamp
		if t, err := time.Parse(time.RFC3339, lastTimestamp); err == nil {
			timeStr = formatRelativeTime(time.Since(t))
		}

		// Determine source from event source component
		source, _, _ := unstructured.NestedString(event.Object, "source", "component")
		sourceType := b.categorizeSource(source)

		changes = append(changes, RecentChange{
			Time:     timeStr,
			Resource: fmt.Sprintf("%s/%s", kind, name),
			Change:   fmt.Sprintf("%s: %s", reason, truncate(message, 100)),
			Source:   sourceType,
		})
	}

	return changes
}

// categorizeSource categorizes the event source
func (b *ContextSnapshotBuilder) categorizeSource(component string) string {
	switch {
	case strings.Contains(component, "kustomize-controller"):
		return "Flux"
	case strings.Contains(component, "helm-controller"):
		return "Flux (Helm)"
	case strings.Contains(component, "source-controller"):
		return "Flux (Source)"
	case strings.Contains(component, "argocd"):
		return "ArgoCD"
	case strings.Contains(component, "horizontal-pod-autoscaler"):
		return "HPA"
	case strings.Contains(component, "deployment-controller"):
		return "Kubernetes"
	case strings.Contains(component, "replicaset-controller"):
		return "Kubernetes"
	default:
		return component
	}
}

// findDependencies finds dependencies for a resource
func (b *ContextSnapshotBuilder) findDependencies(ctx context.Context, resource, namespace string) DependencyInfo {
	deps := DependencyInfo{}

	// Parse resource kind/name
	parts := strings.SplitN(resource, "/", 2)
	if len(parts) != 2 {
		return deps
	}
	kind := parts[0]
	name := parts[1]

	// Get the workload
	var gvr schema.GroupVersionResource
	switch kind {
	case "Deployment":
		gvr = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	case "StatefulSet":
		gvr = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}
	default:
		return deps
	}

	workload, err := b.client.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return deps
	}

	// Find ConfigMaps and Secrets
	volumes, _, _ := unstructured.NestedSlice(workload.Object, "spec", "template", "spec", "volumes")
	for _, v := range volumes {
		vol, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		if cm, ok := vol["configMap"].(map[string]interface{}); ok {
			if cmName, ok := cm["name"].(string); ok {
				deps.DependsOn = append(deps.DependsOn, fmt.Sprintf("ConfigMap/%s", cmName))
			}
		}
		if secret, ok := vol["secret"].(map[string]interface{}); ok {
			if secretName, ok := secret["secretName"].(string); ok {
				deps.DependsOn = append(deps.DependsOn, fmt.Sprintf("Secret/%s", secretName))
			}
		}
	}

	// Find Services (would need label selector matching)
	// Find HPAs (would need scaleTargetRef matching)

	return deps
}

func formatRelativeTime(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// ToJSON converts the snapshot to JSON
func (s *ContextSnapshot) ToJSON() (string, error) {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ToCompactJSON converts the snapshot to compact JSON
func (s *ContextSnapshot) ToCompactJSON() (string, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
