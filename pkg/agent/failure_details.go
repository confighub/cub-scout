// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// FailureStage indicates where in the GitOps pipeline a failure occurred
type FailureStage string

const (
	// StageSource indicates failure during source fetch (Git, OCI, Helm)
	StageSource FailureStage = "source"

	// StageApply indicates failure during apply/reconcile
	StageApply FailureStage = "apply"

	// StageBuild indicates failure during build (kustomize build, helm template)
	StageBuild FailureStage = "build"

	// StageSync indicates failure during sync (Argo CD specific)
	StageSync FailureStage = "sync"

	// StageHealthy indicates no failure
	StageHealthy FailureStage = "healthy"

	// StageUnknown indicates unknown state
	StageUnknown FailureStage = "unknown"
)

// FailureDetails contains extracted failure information from a GitOps resource
type FailureDetails struct {
	// Stage is where the failure occurred: source, build, apply, sync, healthy, unknown
	Stage FailureStage `json:"stage"`

	// Ready indicates if the resource is ready
	Ready bool `json:"ready"`

	// Suspended indicates if the resource is suspended
	Suspended bool `json:"suspended"`

	// Reason is the condition reason (e.g., "AuthenticationFailed", "BuildFailed")
	Reason string `json:"reason,omitempty"`

	// Message is the human-readable failure message
	Message string `json:"message,omitempty"`

	// LastTransitionTime is when the condition last changed
	LastTransitionTime *time.Time `json:"lastTransitionTime,omitempty"`

	// Revision information
	LastAppliedRevision   string `json:"lastAppliedRevision,omitempty"`
	LastAttemptedRevision string `json:"lastAttemptedRevision,omitempty"`

	// Source-specific details (for OCIRepository, GitRepository)
	SourceURL      string `json:"sourceURL,omitempty"`
	ArtifactDigest string `json:"artifactDigest,omitempty"`

	// Argo-specific details
	SyncStatus   string `json:"syncStatus,omitempty"`
	HealthStatus string `json:"healthStatus,omitempty"`
	OperationPhase string `json:"operationPhase,omitempty"`
}

// IsHealthy returns true if the resource is healthy
func (f FailureDetails) IsHealthy() bool {
	return f.Stage == StageHealthy && f.Ready && !f.Suspended
}

// HasRevisionMismatch returns true if lastApplied != lastAttempted
func (f FailureDetails) HasRevisionMismatch() bool {
	return f.LastAppliedRevision != "" &&
		f.LastAttemptedRevision != "" &&
		f.LastAppliedRevision != f.LastAttemptedRevision
}

// ExtractFluxSourceFailure extracts failure details from Flux source resources
// Supports: OCIRepository, GitRepository, HelmRepository, Bucket
func ExtractFluxSourceFailure(resource *unstructured.Unstructured) FailureDetails {
	details := FailureDetails{
		Stage: StageUnknown,
		Ready: false,
	}

	if resource == nil {
		return details
	}

	// Extract URL
	url, _, _ := unstructured.NestedString(resource.Object, "spec", "url")
	details.SourceURL = url

	// Extract artifact information
	if artifact, found, _ := unstructured.NestedMap(resource.Object, "status", "artifact"); found {
		if digest, ok := artifact["revision"].(string); ok {
			details.ArtifactDigest = digest
		}
	}

	// Extract conditions
	conditions, found, _ := unstructured.NestedSlice(resource.Object, "status", "conditions")
	if !found {
		return details
	}

	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}

		condType, _ := cond["type"].(string)
		status, _ := cond["status"].(string)
		reason, _ := cond["reason"].(string)
		message, _ := cond["message"].(string)

		if condType == "Ready" {
			details.Reason = reason
			details.Message = message
			details.Ready = status == "True"

			if timeStr, ok := cond["lastTransitionTime"].(string); ok {
				if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
					details.LastTransitionTime = &t
				}
			}

			// Determine failure stage from reason
			if status == "True" {
				details.Stage = StageHealthy
			} else {
				details.Stage = classifySourceFailure(reason, message)
			}
			break
		}
	}

	return details
}

// ExtractFluxDeployerFailure extracts failure details from Flux deployer resources
// Supports: Kustomization, HelmRelease
func ExtractFluxDeployerFailure(resource *unstructured.Unstructured) FailureDetails {
	details := FailureDetails{
		Stage: StageUnknown,
		Ready: false,
	}

	if resource == nil {
		return details
	}

	// Check suspended
	suspended, _, _ := unstructured.NestedBool(resource.Object, "spec", "suspend")
	details.Suspended = suspended

	// Extract revision information
	details.LastAppliedRevision, _, _ = unstructured.NestedString(resource.Object, "status", "lastAppliedRevision")
	details.LastAttemptedRevision, _, _ = unstructured.NestedString(resource.Object, "status", "lastAttemptedRevision")

	// Extract conditions
	conditions, found, _ := unstructured.NestedSlice(resource.Object, "status", "conditions")
	if !found {
		if suspended {
			details.Stage = StageHealthy
			details.Reason = "Suspended"
			details.Message = "Resource is suspended"
		}
		return details
	}

	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}

		condType, _ := cond["type"].(string)
		status, _ := cond["status"].(string)
		reason, _ := cond["reason"].(string)
		message, _ := cond["message"].(string)

		if condType == "Ready" {
			details.Reason = reason
			details.Message = message
			details.Ready = status == "True"

			if timeStr, ok := cond["lastTransitionTime"].(string); ok {
				if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
					details.LastTransitionTime = &t
				}
			}

			// Determine failure stage from reason
			if status == "True" {
				details.Stage = StageHealthy
			} else {
				details.Stage = classifyDeployerFailure(reason, message)
			}
			break
		}
	}

	// Special case: suspended resources are considered healthy
	if details.Suspended && details.Stage != StageHealthy {
		details.Stage = StageHealthy
	}

	return details
}

// ExtractArgoFailure extracts failure details from Argo CD Applications
func ExtractArgoFailure(resource *unstructured.Unstructured) FailureDetails {
	details := FailureDetails{
		Stage: StageUnknown,
		Ready: false,
	}

	if resource == nil {
		return details
	}

	// Extract sync status
	details.SyncStatus, _, _ = unstructured.NestedString(resource.Object, "status", "sync", "status")
	details.LastAttemptedRevision, _, _ = unstructured.NestedString(resource.Object, "status", "sync", "revision")

	// Extract health status
	details.HealthStatus, _, _ = unstructured.NestedString(resource.Object, "status", "health", "status")
	healthMessage, _, _ := unstructured.NestedString(resource.Object, "status", "health", "message")

	// Extract operation state
	details.OperationPhase, _, _ = unstructured.NestedString(resource.Object, "status", "operationState", "phase")
	opMessage, _, _ := unstructured.NestedString(resource.Object, "status", "operationState", "message")

	// Determine readiness
	details.Ready = details.SyncStatus == "Synced" && details.HealthStatus == "Healthy"

	// Build failure message
	if opMessage != "" {
		details.Message = opMessage
	} else if healthMessage != "" {
		details.Message = healthMessage
	}

	// Classify failure stage
	if details.Ready {
		details.Stage = StageHealthy
	} else {
		details.Stage = classifyArgoFailure(details.SyncStatus, details.HealthStatus, details.OperationPhase, details.Message)
	}

	// Extract operation state timestamp
	if finishedAt, found, _ := unstructured.NestedString(resource.Object, "status", "operationState", "finishedAt"); found {
		if t, err := time.Parse(time.RFC3339, finishedAt); err == nil {
			details.LastTransitionTime = &t
		}
	}

	// Set reason from operation phase or health status
	if details.OperationPhase != "" && details.OperationPhase != "Succeeded" {
		details.Reason = details.OperationPhase
	} else if details.HealthStatus != "" && details.HealthStatus != "Healthy" {
		details.Reason = details.HealthStatus
	}

	return details
}

// classifySourceFailure determines the failure stage from Flux source condition reason
func classifySourceFailure(reason, message string) FailureStage {
	reason = strings.ToLower(reason)
	message = strings.ToLower(message)

	// Authentication failures
	if strings.Contains(reason, "auth") || strings.Contains(message, "unauthorized") ||
		strings.Contains(message, "authentication") || strings.Contains(message, "forbidden") {
		return StageSource
	}

	// Network/fetch failures
	if strings.Contains(reason, "fetch") || strings.Contains(reason, "pull") ||
		strings.Contains(message, "connection") || strings.Contains(message, "timeout") ||
		strings.Contains(message, "manifest unknown") || strings.Contains(message, "not found") {
		return StageSource
	}

	// OCI-specific failures
	if strings.Contains(reason, "oci") {
		return StageSource
	}

	return StageSource // Default to source for source resources
}

// classifyDeployerFailure determines the failure stage from Flux deployer condition reason
func classifyDeployerFailure(reason, message string) FailureStage {
	reason = strings.ToLower(reason)
	message = strings.ToLower(message)

	// Build failures (kustomize build, helm template)
	if strings.Contains(reason, "build") || strings.Contains(message, "kustomize build") ||
		strings.Contains(message, "accumulating resources") || strings.Contains(message, "yaml:") ||
		strings.Contains(message, "template") {
		return StageBuild
	}

	// Source not ready (depends on source failure)
	if strings.Contains(reason, "artifact") || strings.Contains(message, "source") ||
		strings.Contains(message, "artifact not found") {
		return StageSource
	}

	// Apply/reconciliation failures
	if strings.Contains(reason, "reconcil") || strings.Contains(reason, "apply") ||
		strings.Contains(message, "dry-run failed") || strings.Contains(message, "validation") ||
		strings.Contains(message, "server rejected") {
		return StageApply
	}

	// Health check failures
	if strings.Contains(reason, "health") || strings.Contains(message, "health check") {
		return StageApply
	}

	return StageApply // Default to apply for deployer resources
}

// classifyArgoFailure determines the failure stage from Argo CD status
func classifyArgoFailure(syncStatus, healthStatus, opPhase, message string) FailureStage {
	syncStatus = strings.ToLower(syncStatus)
	healthStatus = strings.ToLower(healthStatus)
	opPhase = strings.ToLower(opPhase)
	message = strings.ToLower(message)

	// Operation errors (fetch, template, etc.)
	if opPhase == "error" {
		if strings.Contains(message, "fetch") || strings.Contains(message, "chart") ||
			strings.Contains(message, "repository") {
			return StageSource
		}
		return StageSync
	}

	// Operation failed
	if opPhase == "failed" {
		return StageSync
	}

	// Sync failures
	if syncStatus == "outofsync" || syncStatus == "unknown" {
		return StageSync
	}

	// Health failures (after successful sync)
	if healthStatus == "degraded" || healthStatus == "missing" {
		return StageApply
	}

	return StageUnknown
}
