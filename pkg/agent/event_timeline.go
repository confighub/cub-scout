// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// EventTimeline holds events for a workload and related resources
type EventTimeline struct {
	WorkloadEvents []TimelineEvent `json:"workloadEvents,omitempty"`
	PodEvents      []TimelineEvent `json:"podEvents,omitempty"`
	AllEvents      []TimelineEvent `json:"allEvents"` // Merged and sorted timeline
}

// TimelineEvent extends K8sEvent with source resource info and explanation
type TimelineEvent struct {
	K8sEvent
	ResourceKind string `json:"resourceKind"` // Pod, Deployment, ReplicaSet, etc.
	ResourceName string `json:"resourceName"`
	Explanation  string `json:"explanation,omitempty"`  // Human-readable explanation
	Suggestion   string `json:"suggestion,omitempty"`   // Suggested action
	Severity     string `json:"severity"`               // info, warning, error
}

// EventTimelineFetcher fetches and organizes events into a timeline
type EventTimelineFetcher struct {
	clientset *kubernetes.Clientset
}

// NewEventTimelineFetcher creates a new event timeline fetcher
func NewEventTimelineFetcher(clientset *kubernetes.Clientset) *EventTimelineFetcher {
	return &EventTimelineFetcher{
		clientset: clientset,
	}
}

// FetchForWorkload fetches events for a workload and its pods
func (f *EventTimelineFetcher) FetchForWorkload(ctx context.Context, namespace, kind, name string, podNames []string) (*EventTimeline, error) {
	timeline := &EventTimeline{}

	if f.clientset == nil {
		return timeline, nil
	}

	// Fetch all events in the namespace (more efficient than multiple filtered queries)
	events, err := f.clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}

	// Create a set of pod names for quick lookup
	podSet := make(map[string]bool)
	for _, p := range podNames {
		podSet[p] = true
	}

	// Filter and categorize events
	for _, event := range events.Items {
		te := eventToTimelineEvent(&event)

		// Check if this event is for the workload
		if event.InvolvedObject.Kind == kind && event.InvolvedObject.Name == name {
			timeline.WorkloadEvents = append(timeline.WorkloadEvents, te)
		}

		// Check if this event is for a related pod
		if event.InvolvedObject.Kind == "Pod" && podSet[event.InvolvedObject.Name] {
			timeline.PodEvents = append(timeline.PodEvents, te)
		}

		// Also include ReplicaSet events that have the workload name prefix
		if event.InvolvedObject.Kind == "ReplicaSet" && strings.HasPrefix(event.InvolvedObject.Name, name+"-") {
			timeline.WorkloadEvents = append(timeline.WorkloadEvents, te)
		}
	}

	// Merge all events for the combined timeline
	allEvents := make([]TimelineEvent, 0, len(timeline.WorkloadEvents)+len(timeline.PodEvents))
	allEvents = append(allEvents, timeline.WorkloadEvents...)
	allEvents = append(allEvents, timeline.PodEvents...)

	// Sort by timestamp (most recent first)
	sort.Slice(allEvents, func(i, j int) bool {
		ti := getEventTime(allEvents[i])
		tj := getEventTime(allEvents[j])
		return ti.After(tj)
	})

	timeline.AllEvents = allEvents

	return timeline, nil
}

// eventToTimelineEvent converts a corev1.Event to TimelineEvent with explanations
func eventToTimelineEvent(event *corev1.Event) TimelineEvent {
	var firstTime, lastTime *time.Time
	if !event.FirstTimestamp.IsZero() {
		t := event.FirstTimestamp.Time
		firstTime = &t
	}
	if !event.LastTimestamp.IsZero() {
		t := event.LastTimestamp.Time
		lastTime = &t
	}

	te := TimelineEvent{
		K8sEvent: K8sEvent{
			Type:           event.Type,
			Reason:         event.Reason,
			Message:        event.Message,
			FirstTimestamp: firstTime,
			LastTimestamp:  lastTime,
			Count:          event.Count,
		},
		ResourceKind: event.InvolvedObject.Kind,
		ResourceName: event.InvolvedObject.Name,
		Severity:     getSeverity(event.Type, event.Reason),
	}

	// Add explanation and suggestion based on event reason
	if exp, ok := eventExplanations[event.Reason]; ok {
		te.Explanation = exp.explanation
		te.Suggestion = exp.suggestion
	}

	return te
}

// getEventTime returns the most relevant timestamp for an event
func getEventTime(e TimelineEvent) time.Time {
	if e.LastTimestamp != nil {
		return *e.LastTimestamp
	}
	if e.FirstTimestamp != nil {
		return *e.FirstTimestamp
	}
	return time.Time{}
}

// getSeverity determines event severity based on type and reason
func getSeverity(eventType, reason string) string {
	if eventType == "Warning" {
		// Some warnings are actually errors
		switch reason {
		case "FailedScheduling", "FailedMount", "FailedAttachVolume",
			"BackOff", "CrashLoopBackOff", "Failed", "FailedCreate",
			"FailedKillPod", "ErrImagePull", "ImagePullBackOff":
			return "error"
		}
		return "warning"
	}
	return "info"
}

// eventExplanation holds the explanation and suggestion for an event reason
type eventExplanation struct {
	explanation string
	suggestion  string
}

// eventExplanations maps event reasons to human-readable explanations
var eventExplanations = map[string]eventExplanation{
	// Pod lifecycle events
	"Scheduled": {
		explanation: "Pod was assigned to a node",
		suggestion:  "",
	},
	"Pulled": {
		explanation: "Container image was successfully pulled",
		suggestion:  "",
	},
	"Created": {
		explanation: "Container was created",
		suggestion:  "",
	},
	"Started": {
		explanation: "Container started successfully",
		suggestion:  "",
	},
	"Killing": {
		explanation: "Container is being terminated",
		suggestion:  "Check if this is expected (scale down, rolling update, or manual delete)",
	},

	// Scheduling issues
	"FailedScheduling": {
		explanation: "Kubernetes could not find a suitable node for the pod",
		suggestion:  "Check node resources (CPU, memory), taints/tolerations, and node selectors",
	},
	"Preempted": {
		explanation: "Pod was evicted to make room for higher-priority pods",
		suggestion:  "Consider increasing pod priority or cluster capacity",
	},

	// Image issues
	"Pulling": {
		explanation: "Container image is being pulled from registry",
		suggestion:  "",
	},
	"ErrImagePull": {
		explanation: "Failed to pull container image",
		suggestion:  "Check image name, tag, and registry credentials (imagePullSecrets)",
	},
	"ImagePullBackOff": {
		explanation: "Repeated failures pulling container image",
		suggestion:  "Verify image exists, check registry access, and validate imagePullSecrets",
	},

	// Container issues
	"BackOff": {
		explanation: "Container is in restart backoff (waiting before retry)",
		suggestion:  "Check container logs with: kubectl logs <pod> --previous",
	},
	"CrashLoopBackOff": {
		explanation: "Container keeps crashing and restarting",
		suggestion:  "Check logs for crash reason, verify config/secrets, and check resource limits",
	},
	"Failed": {
		explanation: "Container failed to start or exited with error",
		suggestion:  "Check container logs and exit code",
	},
	"Unhealthy": {
		explanation: "Container failed liveness or readiness probe",
		suggestion:  "Check probe configuration, application health endpoint, and startup time",
	},
	"ProbeWarning": {
		explanation: "Health probe returned a warning",
		suggestion:  "Check probe thresholds and application response times",
	},
	"ContainerGCFailed": {
		explanation: "Failed to garbage collect old containers",
		suggestion:  "Check node disk space and container runtime",
	},

	// Volume issues
	"FailedMount": {
		explanation: "Failed to mount a volume to the pod",
		suggestion:  "Check volume exists, permissions, and node access to storage",
	},
	"FailedAttachVolume": {
		explanation: "Failed to attach volume to node",
		suggestion:  "Check volume availability and cloud provider status",
	},
	"SuccessfulAttachVolume": {
		explanation: "Volume was successfully attached to the node",
		suggestion:  "",
	},
	"VolumeResizeFailed": {
		explanation: "Failed to resize persistent volume",
		suggestion:  "Check storage class supports resizing and CSI driver status",
	},

	// Deployment events
	"ScalingReplicaSet": {
		explanation: "Deployment is scaling the ReplicaSet (up or down)",
		suggestion:  "",
	},
	"SuccessfulCreate": {
		explanation: "Successfully created a new resource",
		suggestion:  "",
	},
	"FailedCreate": {
		explanation: "Failed to create a resource",
		suggestion:  "Check RBAC permissions and resource quotas",
	},
	"SuccessfulDelete": {
		explanation: "Successfully deleted a resource",
		suggestion:  "",
	},
	"FailedKillPod": {
		explanation: "Failed to terminate a pod",
		suggestion:  "Pod may have a stuck process or finalizer",
	},

	// Node events
	"NodeReady": {
		explanation: "Node is ready to accept pods",
		suggestion:  "",
	},
	"NodeNotReady": {
		explanation: "Node is not ready to accept pods",
		suggestion:  "Check node health, kubelet status, and network connectivity",
	},
	"NodeSchedulable": {
		explanation: "Node is available for scheduling",
		suggestion:  "",
	},
	"NodeNotSchedulable": {
		explanation: "Node is marked unschedulable (cordoned)",
		suggestion:  "Node may be under maintenance",
	},
	"Rebooted": {
		explanation: "Node was rebooted",
		suggestion:  "Check if this was planned maintenance",
	},

	// HPA events
	"SuccessfulRescale": {
		explanation: "HPA successfully scaled the workload",
		suggestion:  "",
	},
	"FailedRescale": {
		explanation: "HPA failed to scale the workload",
		suggestion:  "Check HPA configuration and target metrics",
	},
	"FailedGetResourceMetric": {
		explanation: "HPA could not get resource metrics",
		suggestion:  "Check metrics-server is running and pod has resource requests",
	},
	"FailedComputeMetricsReplicas": {
		explanation: "HPA could not compute desired replicas",
		suggestion:  "Check HPA metric configuration and threshold values",
	},

	// Network events
	"FailedCreatePodSandbox": {
		explanation: "Failed to create pod network namespace",
		suggestion:  "Check CNI plugin status and node network configuration",
	},
	"NetworkNotReady": {
		explanation: "Pod network is not ready",
		suggestion:  "Check CNI plugin and network policy controller",
	},

	// Service/Endpoint events
	"NoPods": {
		explanation: "Service has no pods available",
		suggestion:  "Check pod labels match service selector",
	},
	"RegisteredNode": {
		explanation: "Node registered with the control plane",
		suggestion:  "",
	},

	// Eviction events
	"Evicted": {
		explanation: "Pod was evicted from the node",
		suggestion:  "Check for node resource pressure (memory, disk, PIDs)",
	},
	"EvictionThresholdMet": {
		explanation: "Node reached eviction threshold",
		suggestion:  "Node is under resource pressure, pods may be evicted",
	},
}

// GetEventExplanation returns the explanation for an event reason
func GetEventExplanation(reason string) string {
	if exp, ok := eventExplanations[reason]; ok {
		return exp.explanation
	}
	return ""
}

// GetEventSuggestion returns the suggestion for an event reason
func GetEventSuggestion(reason string) string {
	if exp, ok := eventExplanations[reason]; ok {
		return exp.suggestion
	}
	return ""
}

// ResourceEvent is a compact event model for bounded display in explain/trace.
// This is simpler than TimelineEvent and optimized for troubleshooting output.
type ResourceEvent struct {
	Type      string     `json:"type"`              // Normal or Warning
	Reason    string     `json:"reason"`            // e.g., Pulled, BackOff, FailedScheduling
	Message   string     `json:"message"`           // Human-readable message
	Count     int32      `json:"count,omitempty"`   // Number of occurrences
	Age       string     `json:"age"`               // Human-readable age (e.g., "5m", "2h")
	LastSeen  *time.Time `json:"lastSeen"`          // Last occurrence timestamp
	Severity  string     `json:"severity"`          // info, warning, error
	Source    string     `json:"source,omitempty"`  // Component that generated the event
	FirstSeen *time.Time `json:"firstSeen"`         // First occurrence timestamp
}

// ResourceEventSummary summarizes events for a resource.
type ResourceEventSummary struct {
	Events       []ResourceEvent `json:"events"`
	TotalCount   int             `json:"totalCount"`   // Total events found
	WarningCount int             `json:"warningCount"` // Number of warning events
	ErrorCount   int             `json:"errorCount"`   // Number of error-severity events
}

// FetchRecentEvents fetches bounded recent events for a resource.
// It prioritizes Warning events over Normal, then sorts by most recent.
// maxEvents limits the number of events returned (default 5 if <= 0).
func (f *EventTimelineFetcher) FetchRecentEvents(ctx context.Context, namespace, kind, name string, maxEvents int) (*ResourceEventSummary, error) {
	if maxEvents <= 0 {
		maxEvents = 5
	}

	summary := &ResourceEventSummary{
		Events: make([]ResourceEvent, 0),
	}

	if f.clientset == nil {
		return summary, nil
	}

	// Use field selector to filter events for this specific resource
	fieldSelector := fmt.Sprintf("involvedObject.name=%s,involvedObject.kind=%s", name, kind)
	events, err := f.clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fieldSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}

	summary.TotalCount = len(events.Items)

	// Convert to ResourceEvent and count warnings/errors
	resourceEvents := make([]ResourceEvent, 0, len(events.Items))
	for _, event := range events.Items {
		severity := getSeverity(event.Type, event.Reason)
		if event.Type == "Warning" {
			summary.WarningCount++
		}
		if severity == "error" {
			summary.ErrorCount++
		}

		var firstTime, lastTime *time.Time
		if !event.FirstTimestamp.IsZero() {
			t := event.FirstTimestamp.Time
			firstTime = &t
		}
		if !event.LastTimestamp.IsZero() {
			t := event.LastTimestamp.Time
			lastTime = &t
		}

		// For new-style events, use eventTime if timestamps are zero
		if firstTime == nil && !event.EventTime.IsZero() {
			t := event.EventTime.Time
			firstTime = &t
			lastTime = &t
		}

		re := ResourceEvent{
			Type:      event.Type,
			Reason:    event.Reason,
			Message:   truncateEventMessage(event.Message, 200),
			Count:     event.Count,
			Severity:  severity,
			Source:    event.Source.Component,
			FirstSeen: firstTime,
			LastSeen:  lastTime,
		}

		// Calculate age string
		if lastTime != nil {
			re.Age = formatAge(time.Since(*lastTime))
		}

		resourceEvents = append(resourceEvents, re)
	}

	// Sort: warnings/errors first, then by most recent
	sort.Slice(resourceEvents, func(i, j int) bool {
		// Prioritize by severity (error > warning > info)
		si, sj := severityRank(resourceEvents[i].Severity), severityRank(resourceEvents[j].Severity)
		if si != sj {
			return si > sj
		}
		// Then by most recent
		ti := getResourceEventTime(resourceEvents[i])
		tj := getResourceEventTime(resourceEvents[j])
		return ti.After(tj)
	})

	// Limit to maxEvents
	if len(resourceEvents) > maxEvents {
		resourceEvents = resourceEvents[:maxEvents]
	}

	summary.Events = resourceEvents
	return summary, nil
}

// severityRank returns a numeric rank for sorting (higher = more severe)
func severityRank(severity string) int {
	switch severity {
	case "error":
		return 2
	case "warning":
		return 1
	default:
		return 0
	}
}

// getResourceEventTime returns the most relevant time for a ResourceEvent
func getResourceEventTime(e ResourceEvent) time.Time {
	if e.LastSeen != nil {
		return *e.LastSeen
	}
	if e.FirstSeen != nil {
		return *e.FirstSeen
	}
	return time.Time{}
}

// truncateEventMessage truncates a message to maxLen characters
func truncateEventMessage(msg string, maxLen int) string {
	msg = strings.TrimSpace(msg)
	if len(msg) <= maxLen {
		return msg
	}
	return msg[:maxLen-3] + "..."
}

// formatAge formats a duration as a human-readable age string
func formatAge(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
