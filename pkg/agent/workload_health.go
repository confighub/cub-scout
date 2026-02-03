// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// WorkloadHealthStatus contains health info for a workload
type WorkloadHealthStatus struct {
	Kind                string     `json:"kind"`
	Name                string     `json:"name"`
	Namespace           string     `json:"namespace"`
	Replicas            int32      `json:"replicas"`
	ReadyReplicas       int32      `json:"readyReplicas"`
	AvailableReplicas   int32      `json:"availableReplicas"`
	UnavailableReplicas int32      `json:"unavailableReplicas"`
	PodIssues           []PodIssue `json:"podIssues,omitempty"`
	RecentEvents        []K8sEvent `json:"recentEvents,omitempty"`
}

// IsHealthy returns true if all replicas are ready
func (w *WorkloadHealthStatus) IsHealthy() bool {
	return w.ReadyReplicas >= w.Replicas && len(w.PodIssues) == 0
}

// StatusSummary returns a short status string
func (w *WorkloadHealthStatus) StatusSummary() string {
	if w.IsHealthy() {
		return fmt.Sprintf("%d/%d ready", w.ReadyReplicas, w.Replicas)
	}
	if len(w.PodIssues) > 0 {
		return w.PodIssues[0].ContainerStatus
	}
	return fmt.Sprintf("%d/%d ready", w.ReadyReplicas, w.Replicas)
}

// PodIssue represents a problem with a specific pod
type PodIssue struct {
	PodName         string `json:"podName"`
	Phase           string `json:"phase"`
	ContainerStatus string `json:"containerStatus"`
	Reason          string `json:"reason,omitempty"`
	Message         string `json:"message,omitempty"`
	RestartCount    int32  `json:"restartCount"`
}

// WorkloadHealthChecker checks health of workloads
type WorkloadHealthChecker struct {
	dynClient dynamic.Interface
	clientset *kubernetes.Clientset
}

// NewWorkloadHealthChecker creates a new health checker
func NewWorkloadHealthChecker(dynClient dynamic.Interface, clientset *kubernetes.Clientset) *WorkloadHealthChecker {
	return &WorkloadHealthChecker{
		dynClient: dynClient,
		clientset: clientset,
	}
}

// FindUnhealthyWorkloads returns workloads that have health issues
func (c *WorkloadHealthChecker) FindUnhealthyWorkloads(ctx context.Context, namespace string) ([]WorkloadHealthStatus, error) {
	var results []WorkloadHealthStatus

	// Check Deployments
	deployments, err := c.checkDeployments(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("checking deployments: %w", err)
	}
	for _, d := range deployments {
		if !d.IsHealthy() {
			results = append(results, d)
		}
	}

	// Check StatefulSets
	statefulsets, err := c.checkStatefulSets(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("checking statefulsets: %w", err)
	}
	for _, s := range statefulsets {
		if !s.IsHealthy() {
			results = append(results, s)
		}
	}

	// Check DaemonSets
	daemonsets, err := c.checkDaemonSets(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("checking daemonsets: %w", err)
	}
	for _, d := range daemonsets {
		if !d.IsHealthy() {
			results = append(results, d)
		}
	}

	// Sort by severity (more pod issues = higher priority)
	sort.Slice(results, func(i, j int) bool {
		// Sort by number of issues descending
		if len(results[i].PodIssues) != len(results[j].PodIssues) {
			return len(results[i].PodIssues) > len(results[j].PodIssues)
		}
		// Then by unavailable replicas
		if results[i].UnavailableReplicas != results[j].UnavailableReplicas {
			return results[i].UnavailableReplicas > results[j].UnavailableReplicas
		}
		// Then alphabetically
		return results[i].Name < results[j].Name
	})

	return results, nil
}

// FindAllWorkloads returns all workloads regardless of health
func (c *WorkloadHealthChecker) FindAllWorkloads(ctx context.Context, namespace string) ([]WorkloadHealthStatus, error) {
	var results []WorkloadHealthStatus

	deployments, err := c.checkDeployments(ctx, namespace)
	if err != nil {
		return nil, err
	}
	results = append(results, deployments...)

	statefulsets, err := c.checkStatefulSets(ctx, namespace)
	if err != nil {
		return nil, err
	}
	results = append(results, statefulsets...)

	daemonsets, err := c.checkDaemonSets(ctx, namespace)
	if err != nil {
		return nil, err
	}
	results = append(results, daemonsets...)

	// Sort alphabetically by namespace then name
	sort.Slice(results, func(i, j int) bool {
		if results[i].Namespace != results[j].Namespace {
			return results[i].Namespace < results[j].Namespace
		}
		return results[i].Name < results[j].Name
	})

	return results, nil
}

// GetWorkloadHealth gets health status for a specific workload
func (c *WorkloadHealthChecker) GetWorkloadHealth(ctx context.Context, kind, name, namespace string) (*WorkloadHealthStatus, error) {
	switch kind {
	case "Deployment":
		return c.getDeploymentHealth(ctx, name, namespace)
	case "StatefulSet":
		return c.getStatefulSetHealth(ctx, name, namespace)
	case "DaemonSet":
		return c.getDaemonSetHealth(ctx, name, namespace)
	default:
		return nil, fmt.Errorf("unsupported workload kind: %s", kind)
	}
}

// checkDeployments checks all Deployments in a namespace
func (c *WorkloadHealthChecker) checkDeployments(ctx context.Context, namespace string) ([]WorkloadHealthStatus, error) {
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

	var list *unstructured.UnstructuredList
	var err error
	if namespace == "" {
		list, err = c.dynClient.Resource(gvr).List(ctx, metav1.ListOptions{})
	} else {
		list, err = c.dynClient.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, err
	}

	var results []WorkloadHealthStatus
	for _, item := range list.Items {
		status := c.parseDeploymentStatus(&item)
		// Get pod issues
		podIssues, err := c.getPodIssuesForSelector(ctx, item.GetNamespace(), item.GetLabels())
		if err == nil {
			status.PodIssues = podIssues
		}
		results = append(results, status)
	}

	return results, nil
}

// checkStatefulSets checks all StatefulSets in a namespace
func (c *WorkloadHealthChecker) checkStatefulSets(ctx context.Context, namespace string) ([]WorkloadHealthStatus, error) {
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}

	var list *unstructured.UnstructuredList
	var err error
	if namespace == "" {
		list, err = c.dynClient.Resource(gvr).List(ctx, metav1.ListOptions{})
	} else {
		list, err = c.dynClient.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, err
	}

	var results []WorkloadHealthStatus
	for _, item := range list.Items {
		status := c.parseStatefulSetStatus(&item)
		// Get pod issues
		podIssues, err := c.getPodIssuesForSelector(ctx, item.GetNamespace(), item.GetLabels())
		if err == nil {
			status.PodIssues = podIssues
		}
		results = append(results, status)
	}

	return results, nil
}

// checkDaemonSets checks all DaemonSets in a namespace
func (c *WorkloadHealthChecker) checkDaemonSets(ctx context.Context, namespace string) ([]WorkloadHealthStatus, error) {
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}

	var list *unstructured.UnstructuredList
	var err error
	if namespace == "" {
		list, err = c.dynClient.Resource(gvr).List(ctx, metav1.ListOptions{})
	} else {
		list, err = c.dynClient.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, err
	}

	var results []WorkloadHealthStatus
	for _, item := range list.Items {
		status := c.parseDaemonSetStatus(&item)
		// Get pod issues
		podIssues, err := c.getPodIssuesForSelector(ctx, item.GetNamespace(), item.GetLabels())
		if err == nil {
			status.PodIssues = podIssues
		}
		results = append(results, status)
	}

	return results, nil
}

// getDeploymentHealth gets health for a specific Deployment
func (c *WorkloadHealthChecker) getDeploymentHealth(ctx context.Context, name, namespace string) (*WorkloadHealthStatus, error) {
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	obj, err := c.dynClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	status := c.parseDeploymentStatus(obj)

	// Get pod issues
	podIssues, err := c.getPodIssuesForSelector(ctx, namespace, obj.GetLabels())
	if err == nil {
		status.PodIssues = podIssues
	}

	// Get events
	events, err := c.getEventsForResource(ctx, namespace, "Deployment", name)
	if err == nil {
		status.RecentEvents = events
	}

	return &status, nil
}

// getStatefulSetHealth gets health for a specific StatefulSet
func (c *WorkloadHealthChecker) getStatefulSetHealth(ctx context.Context, name, namespace string) (*WorkloadHealthStatus, error) {
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}
	obj, err := c.dynClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	status := c.parseStatefulSetStatus(obj)

	// Get pod issues
	podIssues, err := c.getPodIssuesForSelector(ctx, namespace, obj.GetLabels())
	if err == nil {
		status.PodIssues = podIssues
	}

	// Get events
	events, err := c.getEventsForResource(ctx, namespace, "StatefulSet", name)
	if err == nil {
		status.RecentEvents = events
	}

	return &status, nil
}

// getDaemonSetHealth gets health for a specific DaemonSet
func (c *WorkloadHealthChecker) getDaemonSetHealth(ctx context.Context, name, namespace string) (*WorkloadHealthStatus, error) {
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}
	obj, err := c.dynClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	status := c.parseDaemonSetStatus(obj)

	// Get pod issues
	podIssues, err := c.getPodIssuesForSelector(ctx, namespace, obj.GetLabels())
	if err == nil {
		status.PodIssues = podIssues
	}

	// Get events
	events, err := c.getEventsForResource(ctx, namespace, "DaemonSet", name)
	if err == nil {
		status.RecentEvents = events
	}

	return &status, nil
}

// parseDeploymentStatus extracts status from a Deployment
func (c *WorkloadHealthChecker) parseDeploymentStatus(obj *unstructured.Unstructured) WorkloadHealthStatus {
	status := WorkloadHealthStatus{
		Kind:      "Deployment",
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}

	// Extract spec.replicas
	if replicas, found, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas"); found {
		status.Replicas = int32(replicas)
	} else {
		status.Replicas = 1 // Default
	}

	// Extract status fields
	if ready, found, _ := unstructured.NestedInt64(obj.Object, "status", "readyReplicas"); found {
		status.ReadyReplicas = int32(ready)
	}
	if available, found, _ := unstructured.NestedInt64(obj.Object, "status", "availableReplicas"); found {
		status.AvailableReplicas = int32(available)
	}
	if unavailable, found, _ := unstructured.NestedInt64(obj.Object, "status", "unavailableReplicas"); found {
		status.UnavailableReplicas = int32(unavailable)
	}

	return status
}

// parseStatefulSetStatus extracts status from a StatefulSet
func (c *WorkloadHealthChecker) parseStatefulSetStatus(obj *unstructured.Unstructured) WorkloadHealthStatus {
	status := WorkloadHealthStatus{
		Kind:      "StatefulSet",
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}

	// Extract spec.replicas
	if replicas, found, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas"); found {
		status.Replicas = int32(replicas)
	} else {
		status.Replicas = 1
	}

	// Extract status.readyReplicas
	if ready, found, _ := unstructured.NestedInt64(obj.Object, "status", "readyReplicas"); found {
		status.ReadyReplicas = int32(ready)
	}

	// Calculate unavailable
	status.UnavailableReplicas = status.Replicas - status.ReadyReplicas
	if status.UnavailableReplicas < 0 {
		status.UnavailableReplicas = 0
	}

	return status
}

// parseDaemonSetStatus extracts status from a DaemonSet
func (c *WorkloadHealthChecker) parseDaemonSetStatus(obj *unstructured.Unstructured) WorkloadHealthStatus {
	status := WorkloadHealthStatus{
		Kind:      "DaemonSet",
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}

	// DaemonSets use desiredNumberScheduled as "replicas"
	if desired, found, _ := unstructured.NestedInt64(obj.Object, "status", "desiredNumberScheduled"); found {
		status.Replicas = int32(desired)
	}
	if ready, found, _ := unstructured.NestedInt64(obj.Object, "status", "numberReady"); found {
		status.ReadyReplicas = int32(ready)
	}
	if available, found, _ := unstructured.NestedInt64(obj.Object, "status", "numberAvailable"); found {
		status.AvailableReplicas = int32(available)
	}
	if unavailable, found, _ := unstructured.NestedInt64(obj.Object, "status", "numberUnavailable"); found {
		status.UnavailableReplicas = int32(unavailable)
	}

	return status
}

// getPodIssuesForSelector gets pod issues for pods matching a label selector
func (c *WorkloadHealthChecker) getPodIssuesForSelector(ctx context.Context, namespace string, labels map[string]string) ([]PodIssue, error) {
	if c.clientset == nil {
		return nil, nil
	}

	// Build label selector
	selector := metav1.LabelSelector{MatchLabels: labels}
	labelSelector, err := metav1.LabelSelectorAsSelector(&selector)
	if err != nil {
		return nil, err
	}

	pods, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector.String(),
	})
	if err != nil {
		return nil, err
	}

	var issues []PodIssue
	for _, pod := range pods.Items {
		issue := extractPodIssue(&pod)
		if issue != nil {
			issues = append(issues, *issue)
		}
	}

	return issues, nil
}

// extractPodIssue extracts issues from a pod, returns nil if pod is healthy
func extractPodIssue(pod *corev1.Pod) *PodIssue {
	// Check if pod is in a bad phase
	if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodPending {
		issue := &PodIssue{
			PodName: pod.Name,
			Phase:   string(pod.Status.Phase),
		}

		// Get reason from conditions
		for _, cond := range pod.Status.Conditions {
			if cond.Status == corev1.ConditionFalse {
				issue.Reason = string(cond.Reason)
				issue.Message = cond.Message
				break
			}
		}

		return issue
	}

	// Check container statuses
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil {
			reason := cs.State.Waiting.Reason
			// These are problematic waiting states
			if reason == "CrashLoopBackOff" || reason == "ImagePullBackOff" ||
				reason == "ErrImagePull" || reason == "CreateContainerConfigError" ||
				reason == "InvalidImageName" {
				return &PodIssue{
					PodName:         pod.Name,
					Phase:           string(pod.Status.Phase),
					ContainerStatus: reason,
					Reason:          reason,
					Message:         cs.State.Waiting.Message,
					RestartCount:    cs.RestartCount,
				}
			}
		}

		if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
			return &PodIssue{
				PodName:         pod.Name,
				Phase:           string(pod.Status.Phase),
				ContainerStatus: cs.State.Terminated.Reason,
				Reason:          cs.State.Terminated.Reason,
				Message:         cs.State.Terminated.Message,
				RestartCount:    cs.RestartCount,
			}
		}

		// High restart count is also an issue
		if cs.RestartCount > 5 {
			return &PodIssue{
				PodName:         pod.Name,
				Phase:           string(pod.Status.Phase),
				ContainerStatus: "HighRestarts",
				Reason:          fmt.Sprintf("Container has restarted %d times", cs.RestartCount),
				RestartCount:    cs.RestartCount,
			}
		}
	}

	return nil
}

// getEventsForResource gets recent events for a resource
func (c *WorkloadHealthChecker) getEventsForResource(ctx context.Context, namespace, kind, name string) ([]K8sEvent, error) {
	if c.clientset == nil {
		return nil, nil
	}

	events, err := c.clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.kind=%s,involvedObject.name=%s", kind, name),
	})
	if err != nil {
		return nil, err
	}

	var results []K8sEvent
	for _, event := range events.Items {
		firstTime := event.FirstTimestamp.Time
		lastTime := event.LastTimestamp.Time
		results = append(results, K8sEvent{
			Type:           event.Type,
			Reason:         event.Reason,
			Message:        event.Message,
			Count:          event.Count,
			FirstTimestamp: &firstTime,
			LastTimestamp:  &lastTime,
		})
	}

	// Sort by last seen descending (most recent first)
	sort.Slice(results, func(i, j int) bool {
		if results[i].LastTimestamp == nil {
			return false
		}
		if results[j].LastTimestamp == nil {
			return true
		}
		return results[i].LastTimestamp.After(*results[j].LastTimestamp)
	})

	// Limit to 10 most recent
	if len(results) > 10 {
		results = results[:10]
	}

	return results, nil
}
