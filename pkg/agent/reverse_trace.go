// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// ReverseTracer walks ownerReferences to find the GitOps source
type ReverseTracer struct {
	client dynamic.Interface
}

// NewReverseTracer creates a new reverse tracer
func NewReverseTracer(client dynamic.Interface) *ReverseTracer {
	return &ReverseTracer{client: client}
}

// ReverseTraceResult contains the full chain from resource to Git source
type ReverseTraceResult struct {
	// Object is the starting resource
	Object ResourceRef `json:"object"`

	// K8sChain is the Kubernetes ownership chain (Pod → ReplicaSet → Deployment)
	K8sChain []ChainLink `json:"k8sChain"`

	// GitOpsChain is the GitOps chain (Deployment → Kustomization → GitRepository)
	// This is populated by calling the appropriate tool tracer
	GitOpsChain []ChainLink `json:"gitOpsChain,omitempty"`

	// Owner indicates the detected owner type
	Owner string `json:"owner"` // "flux", "argo", "helm", "confighub", "native"

	// OwnerDetails contains additional ownership info
	OwnerDetails *Ownership `json:"ownerDetails,omitempty"`

	// TopResource is the top of the K8s ownership chain
	TopResource *ResourceRef `json:"topResource,omitempty"`

	// OrphanMeta contains metadata for orphan/native resources
	OrphanMeta *OrphanMetadata `json:"orphanMeta,omitempty"`

	// Error contains any error encountered
	Error string `json:"error,omitempty"`

	// TracedAt is when the trace was performed
	TracedAt time.Time `json:"tracedAt"`
}

// OrphanMetadata contains information about orphan (native/unmanaged) resources
type OrphanMetadata struct {
	// LastAppliedConfig is the kubectl.kubernetes.io/last-applied-configuration annotation
	LastAppliedConfig string `json:"lastAppliedConfig,omitempty"`

	// CreatedAt is when the resource was created
	CreatedAt *time.Time `json:"createdAt,omitempty"`

	// Labels are the resource labels
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations are the resource annotations (excluding last-applied-config)
	Annotations map[string]string `json:"annotations,omitempty"`
}

// Trace performs a reverse trace starting from any resource
func (r *ReverseTracer) Trace(ctx context.Context, kind, name, namespace string) (*ReverseTraceResult, error) {
	result := &ReverseTraceResult{
		Object: ResourceRef{
			Kind:      kind,
			Name:      name,
			Namespace: namespace,
		},
		K8sChain: []ChainLink{},
		TracedAt: time.Now(),
	}

	// Fetch the starting resource
	gvr, err := KindToGVR(kind)
	if err != nil {
		result.Error = fmt.Sprintf("unknown resource kind: %s", kind)
		return result, nil
	}

	resource, err := r.client.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		result.Error = fmt.Sprintf("failed to get resource: %v", err)
		return result, nil
	}

	// Add starting resource to chain
	result.K8sChain = append(result.K8sChain, r.resourceToChainLink(resource))

	// Walk ownerReferences
	current := resource
	for {
		owners := current.GetOwnerReferences()
		if len(owners) == 0 {
			break
		}

		// Get the controller owner (or first owner)
		var owner metav1.OwnerReference
		for _, o := range owners {
			if o.Controller != nil && *o.Controller {
				owner = o
				break
			}
		}
		if owner.Name == "" && len(owners) > 0 {
			owner = owners[0]
		}

		// Fetch the owner resource
		ownerGVR, err := APIVersionKindToGVR(owner.APIVersion, owner.Kind)
		if err != nil {
			// Can't resolve owner, stop here
			break
		}

		ownerResource, err := r.client.Resource(ownerGVR).Namespace(namespace).Get(ctx, owner.Name, metav1.GetOptions{})
		if err != nil {
			// Can't fetch owner, stop here
			break
		}

		result.K8sChain = append(result.K8sChain, r.resourceToChainLink(ownerResource))
		current = ownerResource
	}

	// The top of the chain is the last item
	if len(result.K8sChain) > 0 {
		top := result.K8sChain[len(result.K8sChain)-1]
		result.TopResource = &ResourceRef{
			Kind:      top.Kind,
			Name:      top.Name,
			Namespace: top.Namespace,
		}
	}

	// Detect ownership of the top resource
	topResource := current
	ownership := DetectOwnership(topResource)
	result.OwnerDetails = &ownership

	switch ownership.Type {
	case OwnerFlux:
		result.Owner = "flux"
	case OwnerArgo:
		result.Owner = "argo"
	case OwnerHelm:
		result.Owner = "helm"
	case OwnerConfigHub:
		result.Owner = "confighub"
	case OwnerTerraform:
		result.Owner = "terraform"
	default:
		result.Owner = "native"
		// Populate orphan metadata for native resources
		result.OrphanMeta = extractOrphanMetadata(topResource)
	}

	return result, nil
}

// extractOrphanMetadata extracts useful metadata for orphan/native resources
func extractOrphanMetadata(resource *unstructured.Unstructured) *OrphanMetadata {
	meta := &OrphanMetadata{}

	// Get creation timestamp
	creationTime := resource.GetCreationTimestamp()
	if !creationTime.IsZero() {
		t := creationTime.Time
		meta.CreatedAt = &t
	}

	// Get labels (copy to avoid mutation)
	if labels := resource.GetLabels(); len(labels) > 0 {
		meta.Labels = make(map[string]string, len(labels))
		for k, v := range labels {
			meta.Labels[k] = v
		}
	}

	// Get annotations and extract last-applied-configuration
	annotations := resource.GetAnnotations()
	if annotations != nil {
		if lastApplied, ok := annotations["kubectl.kubernetes.io/last-applied-configuration"]; ok {
			meta.LastAppliedConfig = lastApplied
		}

		// Copy other annotations (excluding last-applied-config which can be large)
		meta.Annotations = make(map[string]string)
		for k, v := range annotations {
			if k != "kubectl.kubernetes.io/last-applied-configuration" {
				meta.Annotations[k] = v
			}
		}
	}

	return meta
}

// resourceToChainLink converts an unstructured resource to a ChainLink
func (r *ReverseTracer) resourceToChainLink(resource *unstructured.Unstructured) ChainLink {
	link := ChainLink{
		Kind:      resource.GetKind(),
		Name:      resource.GetName(),
		Namespace: resource.GetNamespace(),
		Ready:     true, // Default to ready, will update based on status
	}

	// Try to extract status
	status, found, _ := unstructured.NestedMap(resource.Object, "status")
	if found {
		// Check for common ready conditions
		conditions, ok, _ := unstructured.NestedSlice(status, "conditions")
		if ok {
			for _, c := range conditions {
				cond, ok := c.(map[string]interface{})
				if !ok {
					continue
				}
				condType, _ := cond["type"].(string)
				condStatus, _ := cond["status"].(string)
				if condType == "Ready" || condType == "Available" {
					link.Ready = condStatus == "True"
					if reason, ok := cond["reason"].(string); ok {
						link.StatusReason = reason
					}
					if msg, ok := cond["message"].(string); ok {
						link.Message = msg
					}
					break
				}
			}
		}

		// Check replicas for Deployments/StatefulSets/DaemonSets
		if replicas, ok, _ := unstructured.NestedInt64(status, "replicas"); ok {
			readyReplicas, _, _ := unstructured.NestedInt64(status, "readyReplicas")
			link.Status = fmt.Sprintf("%d/%d ready", readyReplicas, replicas)
			link.Ready = readyReplicas == replicas && replicas > 0
		}

		// Check phase for Pods
		if phase, ok, _ := unstructured.NestedString(status, "phase"); ok {
			link.Status = phase
			link.Ready = phase == "Running" || phase == "Succeeded"
		}
	}

	// Get creation timestamp
	creationTime := resource.GetCreationTimestamp()
	if !creationTime.IsZero() {
		t := creationTime.Time
		link.LastTransitionTime = &t
	}

	return link
}

// KindToGVR maps a kind to its GroupVersionResource
func KindToGVR(kind string) (schema.GroupVersionResource, error) {
	switch kind {
	case "Pod", "pod", "pods":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}, nil
	case "ReplicaSet", "replicaset", "replicasets":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}, nil
	case "Deployment", "deployment", "deployments", "deploy":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, nil
	case "StatefulSet", "statefulset", "statefulsets", "sts":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}, nil
	case "DaemonSet", "daemonset", "daemonsets", "ds":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}, nil
	case "Job", "job", "jobs":
		return schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}, nil
	case "CronJob", "cronjob", "cronjobs":
		return schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "cronjobs"}, nil
	case "Service", "service", "services", "svc":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}, nil
	case "ConfigMap", "configmap", "configmaps", "cm":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}, nil
	case "Secret", "secret", "secrets":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}, nil
	case "Ingress", "ingress", "ingresses", "ing":
		return schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"}, nil
	case "ServiceAccount", "serviceaccount", "serviceaccounts", "sa":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "serviceaccounts"}, nil
	case "PersistentVolumeClaim", "persistentvolumeclaim", "persistentvolumeclaims", "pvc":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumeclaims"}, nil
	case "Namespace", "namespace", "namespaces", "ns":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}, nil
	// Flux resources
	case "Kustomization", "kustomization", "kustomizations", "ks":
		return schema.GroupVersionResource{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"}, nil
	case "HelmRelease", "helmrelease", "helmreleases", "hr":
		return schema.GroupVersionResource{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"}, nil
	case "GitRepository", "gitrepository", "gitrepositories":
		return schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"}, nil
	case "OCIRepository", "ocirepository", "ocirepositories":
		return schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1beta2", Resource: "ocirepositories"}, nil
	case "HelmRepository", "helmrepository", "helmrepositories":
		return schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "helmrepositories"}, nil
	case "Bucket", "bucket", "buckets":
		return schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1beta2", Resource: "buckets"}, nil
	// Argo CD resources
	case "Application", "application", "applications", "app":
		return schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}, nil
	case "ApplicationSet", "applicationset", "applicationsets", "appset":
		return schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applicationsets"}, nil
	default:
		return schema.GroupVersionResource{}, fmt.Errorf("unknown kind: %s", kind)
	}
}

// APIVersionKindToGVR converts apiVersion and kind to GVR
func APIVersionKindToGVR(apiVersion, kind string) (schema.GroupVersionResource, error) {
	// Parse apiVersion into group and version
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return schema.GroupVersionResource{}, err
	}

	// Map kind to resource (pluralize)
	resource := KindToResource(kind)
	if resource == "" {
		return schema.GroupVersionResource{}, fmt.Errorf("unknown kind: %s", kind)
	}

	return schema.GroupVersionResource{
		Group:    gv.Group,
		Version:  gv.Version,
		Resource: resource,
	}, nil
}

// KindToResource maps a kind to its resource name (plural)
func KindToResource(kind string) string {
	switch kind {
	case "Pod":
		return "pods"
	case "ReplicaSet":
		return "replicasets"
	case "Deployment":
		return "deployments"
	case "StatefulSet":
		return "statefulsets"
	case "DaemonSet":
		return "daemonsets"
	case "Job":
		return "jobs"
	case "CronJob":
		return "cronjobs"
	case "Service":
		return "services"
	case "ConfigMap":
		return "configmaps"
	case "Secret":
		return "secrets"
	case "Ingress":
		return "ingresses"
	case "ServiceAccount":
		return "serviceaccounts"
	case "PersistentVolumeClaim":
		return "persistentvolumeclaims"
	case "Namespace":
		return "namespaces"
	case "Kustomization":
		return "kustomizations"
	case "HelmRelease":
		return "helmreleases"
	case "GitRepository":
		return "gitrepositories"
	case "OCIRepository":
		return "ocirepositories"
	case "HelmRepository":
		return "helmrepositories"
	case "Bucket":
		return "buckets"
	case "Application":
		return "applications"
	case "ApplicationSet":
		return "applicationsets"
	default:
		return ""
	}
}
