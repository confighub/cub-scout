// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// TimingEnricher adds timing information to trace results
type TimingEnricher struct {
	client dynamic.Interface
}

// NewTimingEnricher creates a new timing enricher
func NewTimingEnricher(client dynamic.Interface) *TimingEnricher {
	return &TimingEnricher{client: client}
}

// EnrichChainWithTiming adds LastTransitionTime to chain links by fetching resource status
func (e *TimingEnricher) EnrichChainWithTiming(ctx context.Context, chain []ChainLink) []ChainLink {
	for i := range chain {
		link := &chain[i]
		timing := e.getResourceTiming(ctx, link.Kind, link.Name, link.Namespace)
		if timing != nil {
			link.LastTransitionTime = timing
		}
	}
	return chain
}

// getResourceTiming fetches a resource and extracts its timing information
func (e *TimingEnricher) getResourceTiming(ctx context.Context, kind, name, namespace string) *time.Time {
	gvr := kindToTimingGVR(kind)
	if gvr.Resource == "" {
		return nil
	}

	var resource *unstructured.Unstructured
	var err error

	if namespace != "" {
		resource, err = e.client.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	} else {
		resource, err = e.client.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
	}
	if err != nil {
		return nil
	}

	return extractTimingFromResource(resource, kind)
}

// extractTimingFromResource extracts the appropriate timestamp from a resource
func extractTimingFromResource(resource *unstructured.Unstructured, kind string) *time.Time {
	// Try different timing fields based on resource kind

	// Flux Kustomization: status.lastAttemptedRevisionTime or status.lastHandledReconcileAt
	if kind == "Kustomization" {
		// Try lastHandledReconcileAt first (more accurate for when it last reconciled)
		if ts := getNestedTime(resource, "status", "lastHandledReconcileAt"); ts != nil {
			return ts
		}
		// Fall back to lastAttemptedRevisionTime
		if ts := getNestedTime(resource, "status", "lastAttemptedRevisionTime"); ts != nil {
			return ts
		}
	}

	// Flux HelmRelease: status.lastAttemptedRevisionTime
	if kind == "HelmRelease" {
		if ts := getNestedTime(resource, "status", "lastAttemptedRevisionTime"); ts != nil {
			return ts
		}
	}

	// Flux sources (GitRepository, OCIRepository, etc.): status.artifact.lastUpdateTime
	if kind == "GitRepository" || kind == "OCIRepository" || kind == "HelmRepository" || kind == "Bucket" {
		if ts := getNestedTime(resource, "status", "artifact", "lastUpdateTime"); ts != nil {
			return ts
		}
	}

	// ArgoCD Application: status.operationState.startedAt or status.reconciledAt
	if kind == "Application" {
		// Try operationState.finishedAt for completed operations
		if ts := getNestedTime(resource, "status", "operationState", "finishedAt"); ts != nil {
			return ts
		}
		// Fall back to startedAt
		if ts := getNestedTime(resource, "status", "operationState", "startedAt"); ts != nil {
			return ts
		}
		// Or reconciledAt
		if ts := getNestedTime(resource, "status", "reconciledAt"); ts != nil {
			return ts
		}
	}

	// Generic fallback: check status.conditions for Ready condition's lastTransitionTime
	conditions, found, _ := unstructured.NestedSlice(resource.Object, "status", "conditions")
	if found {
		for _, c := range conditions {
			condition, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			condType, _, _ := unstructured.NestedString(condition, "type")
			if condType == "Ready" || condType == "Available" {
				if ts := getTimeFromMap(condition, "lastTransitionTime"); ts != nil {
					return ts
				}
			}
		}
	}

	return nil
}

// getNestedTime extracts a time value from nested fields
func getNestedTime(resource *unstructured.Unstructured, fields ...string) *time.Time {
	value, found, _ := unstructured.NestedString(resource.Object, fields...)
	if !found || value == "" {
		return nil
	}
	return parseTime(value)
}

// getTimeFromMap extracts a time value from a map
func getTimeFromMap(m map[string]interface{}, key string) *time.Time {
	value, ok := m[key].(string)
	if !ok || value == "" {
		return nil
	}
	return parseTime(value)
}

// parseTime parses various time formats used in Kubernetes
func parseTime(s string) *time.Time {
	// Try RFC3339
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		return &t
	}

	// Try RFC3339Nano
	t, err = time.Parse(time.RFC3339Nano, s)
	if err == nil {
		return &t
	}

	// Try Kubernetes timestamp format
	t, err = time.Parse("2006-01-02T15:04:05Z", s)
	if err == nil {
		return &t
	}

	return nil
}

// kindToTimingGVR maps resource kinds to GVRs for timing enrichment
func kindToTimingGVR(kind string) schema.GroupVersionResource {
	switch kind {
	case "Kustomization":
		return schema.GroupVersionResource{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"}
	case "HelmRelease":
		return schema.GroupVersionResource{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"}
	case "GitRepository":
		return schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"}
	case "OCIRepository":
		return schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1beta2", Resource: "ocirepositories"}
	case "HelmRepository":
		return schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "helmrepositories"}
	case "Bucket":
		return schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1beta2", Resource: "buckets"}
	case "Application":
		return schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}
	case "Deployment":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	case "StatefulSet":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}
	case "DaemonSet":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}
	default:
		return schema.GroupVersionResource{}
	}
}
