// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package agent provides drift-debug correlation helpers for v0.14.5.
//
// These are pure join functions that correlate drift findings with
// logs and events via shared object identity.
//
// Contract: No new JSON fields. No schema changes. No interpretations.
// These functions return references only - narrative is added at render time.
//
// See docs/semantic-contract.md for the full model.
package agent

// DriftCorrelation holds the result of correlating drift findings with debug signals.
// This is a reference structure only - it does not introduce new facts.
type DriftCorrelation struct {
	// ObjectID is the shared identity (e.g., "Deployment:prod/api")
	ObjectID string

	// Findings are drift findings for this object (reference to existing facts)
	Findings []DriftFinding

	// Events are timeline events for this object (reference to existing facts)
	Events []TimelineEvent

	// Logs are container logs for this object (reference to existing facts)
	Logs []ContainerLogResult

	// HasDrift indicates whether any drift exists for this object
	HasDrift bool

	// HasFailure indicates whether any failure events/logs exist for this object
	HasFailure bool
}

// FindingsForObject filters drift findings by object_id.
// Returns all findings where object_id matches the given ID.
func FindingsForObject(findings []DriftFinding, objectID string) []DriftFinding {
	var result []DriftFinding
	for _, f := range findings {
		if f.ObjectID == objectID {
			result = append(result, f)
		}
	}
	return result
}

// FindingsForObjectID filters drift findings by DriftObjectID components.
// This is a convenience wrapper when you have kind/namespace/name separately.
func FindingsForObjectID(findings []DriftFinding, kind, namespace, name string) []DriftFinding {
	objectID := formatObjectID(kind, namespace, name)
	return FindingsForObject(findings, objectID)
}

// EventsForObject filters timeline events by resource identity.
// Matches on ResourceKind and ResourceName (namespace is assumed from context).
func EventsForObject(events []TimelineEvent, kind, name string) []TimelineEvent {
	var result []TimelineEvent
	for _, e := range events {
		if e.ResourceKind == kind && e.ResourceName == name {
			result = append(result, e)
		}
	}
	return result
}

// EventsForFindings returns all events that correlate with the given findings.
// Correlation is by object identity (kind + name).
func EventsForFindings(findings []DriftFinding, events []TimelineEvent) []TimelineEvent {
	// Build set of object identities from findings
	objects := make(map[string]bool)
	for _, f := range findings {
		key := f.Object.Kind + ":" + f.Object.Name
		objects[key] = true
	}

	// Filter events matching any finding object
	var result []TimelineEvent
	for _, e := range events {
		key := e.ResourceKind + ":" + e.ResourceName
		if objects[key] {
			result = append(result, e)
		}
	}
	return result
}

// LogsForObject filters container logs by workload identity.
// For a Deployment, this matches pods belonging to that deployment.
// The podNames slice should contain all pod names for the workload.
func LogsForObject(logs []ContainerLogResult, namespace string, podNames []string) []ContainerLogResult {
	podSet := make(map[string]bool)
	for _, p := range podNames {
		podSet[p] = true
	}

	var result []ContainerLogResult
	for _, l := range logs {
		if l.Namespace == namespace && podSet[l.PodName] {
			result = append(result, l)
		}
	}
	return result
}

// LogsForFindings returns all logs that correlate with the given findings.
// This requires a mapping from finding objects to their pod names.
func LogsForFindings(findings []DriftFinding, logs []ContainerLogResult, objectToPods map[string][]string) []ContainerLogResult {
	var result []ContainerLogResult
	seen := make(map[string]bool) // Deduplicate by pod+container

	for _, f := range findings {
		objectID := f.ObjectID
		podNames := objectToPods[objectID]

		for _, l := range logs {
			if l.Namespace == f.Object.Namespace {
				for _, podName := range podNames {
					if l.PodName == podName {
						key := l.Namespace + "/" + l.PodName + "/" + l.ContainerName
						if !seen[key] {
							seen[key] = true
							result = append(result, l)
						}
					}
				}
			}
		}
	}
	return result
}

// CorrelateAll builds a correlation structure for a given object.
// This is the main entry point for correlation - it joins all available signals.
func CorrelateAll(objectID string, kind, namespace, name string, findings []DriftFinding, events []TimelineEvent, logs []ContainerLogResult, podNames []string) DriftCorrelation {
	corr := DriftCorrelation{
		ObjectID: objectID,
	}

	// Filter findings for this object
	corr.Findings = FindingsForObject(findings, objectID)
	corr.HasDrift = len(corr.Findings) > 0

	// Filter events for this object
	corr.Events = EventsForObject(events, kind, name)

	// Filter logs for this object's pods
	corr.Logs = LogsForObject(logs, namespace, podNames)

	// Determine if there are failures
	corr.HasFailure = hasFailureSignals(corr.Events, corr.Logs)

	return corr
}

// ObjectsWithDrift returns unique object IDs from a list of findings.
func ObjectsWithDrift(findings []DriftFinding) []string {
	seen := make(map[string]bool)
	var result []string

	for _, f := range findings {
		if !seen[f.ObjectID] {
			seen[f.ObjectID] = true
			result = append(result, f.ObjectID)
		}
	}
	return result
}

// ObjectsWithCriticalDrift returns object IDs that have critical severity drift.
func ObjectsWithCriticalDrift(findings []DriftFinding) []string {
	seen := make(map[string]bool)
	var result []string

	for _, f := range findings {
		if f.Severity == DriftSeverityCritical && !seen[f.ObjectID] {
			seen[f.ObjectID] = true
			result = append(result, f.ObjectID)
		}
	}
	return result
}

// Helper functions

// formatObjectID creates an object_id string from components.
func formatObjectID(kind, namespace, name string) string {
	if namespace != "" {
		return kind + ":" + namespace + "/" + name
	}
	return kind + ":" + name
}

// hasFailureSignals checks if any events or logs indicate failure.
// This is a simple heuristic - events with type "Warning" or logs with error patterns.
func hasFailureSignals(events []TimelineEvent, logs []ContainerLogResult) bool {
	// Check events for warnings/errors
	for _, e := range events {
		if e.Type == "Warning" {
			return true
		}
		if e.Severity == "error" || e.Severity == "warning" {
			return true
		}
	}

	// Check logs for error patterns
	for _, l := range logs {
		if l.Error != "" {
			return true
		}
		for _, p := range l.Patterns {
			if p.Type == "error" || p.Type == "panic" || p.Type == "fatal" {
				return true
			}
		}
	}

	return false
}
