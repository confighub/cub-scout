// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// CrossRefDetector detects cross-owner references in resources
type CrossRefDetector struct {
	client dynamic.Interface
}

// NewCrossRefDetector creates a new cross-reference detector
func NewCrossRefDetector(client dynamic.Interface) *CrossRefDetector {
	return &CrossRefDetector{client: client}
}

// DetectCrossReferences finds resources referenced by the given resource that have different owners
func (d *CrossRefDetector) DetectCrossReferences(ctx context.Context, resource *unstructured.Unstructured, resourceOwner *Ownership) ([]CrossReference, error) {
	var crossRefs []CrossReference

	kind := resource.GetKind()
	namespace := resource.GetNamespace()

	// Extract references based on resource kind
	var refs []resourceReference
	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet":
		refs = extractWorkloadReferences(resource)
	case "Pod":
		refs = extractPodReferences(resource)
	default:
		// Other kinds don't have cross-references we track
		return nil, nil
	}

	// For each reference, check if it exists and detect its owner
	for _, ref := range refs {
		crossRef := CrossReference{
			Ref: ResourceRef{
				Kind:      ref.kind,
				Name:      ref.name,
				Namespace: namespace,
			},
			RefType: ref.refType,
		}

		// Try to fetch the referenced resource
		gvr := kindToGVR(ref.kind)
		if gvr.Resource == "" {
			crossRef.Status = "unknown"
			crossRef.Message = "unknown resource kind"
			crossRefs = append(crossRefs, crossRef)
			continue
		}

		refResource, err := d.client.Resource(gvr).Namespace(namespace).Get(ctx, ref.name, metav1.GetOptions{})
		if err != nil {
			crossRef.Status = "missing"
			crossRef.Message = err.Error()
			crossRefs = append(crossRefs, crossRef)
			continue
		}

		// Detect the owner of the referenced resource
		refOwner := DetectOwnership(refResource)
		crossRef.Owner = &refOwner
		crossRef.Status = "exists"

		// Only include if owner is different (cross-owner reference)
		if resourceOwner != nil && refOwner.Type != resourceOwner.Type {
			crossRefs = append(crossRefs, crossRef)
		}
	}

	return crossRefs, nil
}

// resourceReference represents an extracted reference to another resource
type resourceReference struct {
	kind    string
	name    string
	refType string
}

// extractWorkloadReferences extracts Secret and ConfigMap references from a workload (Deployment/StatefulSet/DaemonSet)
func extractWorkloadReferences(resource *unstructured.Unstructured) []resourceReference {
	var refs []resourceReference
	seen := make(map[string]bool)

	// Get pod template spec
	template, found, _ := unstructured.NestedMap(resource.Object, "spec", "template", "spec")
	if !found {
		return refs
	}

	// Extract from containers
	containers, _, _ := unstructured.NestedSlice(template, "containers")
	for _, c := range containers {
		container, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		refs = append(refs, extractContainerReferences(container, seen)...)
	}

	// Extract from init containers
	initContainers, _, _ := unstructured.NestedSlice(template, "initContainers")
	for _, c := range initContainers {
		container, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		refs = append(refs, extractContainerReferences(container, seen)...)
	}

	// Extract from volumes
	volumes, _, _ := unstructured.NestedSlice(template, "volumes")
	refs = append(refs, extractVolumeReferences(volumes, seen)...)

	return refs
}

// extractPodReferences extracts Secret and ConfigMap references from a Pod
func extractPodReferences(resource *unstructured.Unstructured) []resourceReference {
	var refs []resourceReference
	seen := make(map[string]bool)

	spec, found, _ := unstructured.NestedMap(resource.Object, "spec")
	if !found {
		return refs
	}

	// Extract from containers
	containers, _, _ := unstructured.NestedSlice(spec, "containers")
	for _, c := range containers {
		container, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		refs = append(refs, extractContainerReferences(container, seen)...)
	}

	// Extract from init containers
	initContainers, _, _ := unstructured.NestedSlice(spec, "initContainers")
	for _, c := range initContainers {
		container, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		refs = append(refs, extractContainerReferences(container, seen)...)
	}

	// Extract from volumes
	volumes, _, _ := unstructured.NestedSlice(spec, "volumes")
	refs = append(refs, extractVolumeReferences(volumes, seen)...)

	return refs
}

// extractContainerReferences extracts references from a container's env and envFrom
func extractContainerReferences(container map[string]interface{}, seen map[string]bool) []resourceReference {
	var refs []resourceReference

	// Extract from envFrom
	envFrom, _, _ := unstructured.NestedSlice(container, "envFrom")
	for _, ef := range envFrom {
		envFromEntry, ok := ef.(map[string]interface{})
		if !ok {
			continue
		}

		// configMapRef
		if cmRef, found, _ := unstructured.NestedMap(envFromEntry, "configMapRef"); found {
			if name, ok := cmRef["name"].(string); ok && name != "" {
				key := "ConfigMap:" + name
				if !seen[key] {
					seen[key] = true
					refs = append(refs, resourceReference{
						kind:    "ConfigMap",
						name:    name,
						refType: "envFrom.configMapRef",
					})
				}
			}
		}

		// secretRef
		if secretRef, found, _ := unstructured.NestedMap(envFromEntry, "secretRef"); found {
			if name, ok := secretRef["name"].(string); ok && name != "" {
				key := "Secret:" + name
				if !seen[key] {
					seen[key] = true
					refs = append(refs, resourceReference{
						kind:    "Secret",
						name:    name,
						refType: "envFrom.secretRef",
					})
				}
			}
		}
	}

	// Extract from env[].valueFrom
	env, _, _ := unstructured.NestedSlice(container, "env")
	for _, e := range env {
		envEntry, ok := e.(map[string]interface{})
		if !ok {
			continue
		}

		valueFrom, found, _ := unstructured.NestedMap(envEntry, "valueFrom")
		if !found {
			continue
		}

		// configMapKeyRef
		if cmKeyRef, found, _ := unstructured.NestedMap(valueFrom, "configMapKeyRef"); found {
			if name, ok := cmKeyRef["name"].(string); ok && name != "" {
				key := "ConfigMap:" + name
				if !seen[key] {
					seen[key] = true
					refs = append(refs, resourceReference{
						kind:    "ConfigMap",
						name:    name,
						refType: "env.valueFrom.configMapKeyRef",
					})
				}
			}
		}

		// secretKeyRef
		if secretKeyRef, found, _ := unstructured.NestedMap(valueFrom, "secretKeyRef"); found {
			if name, ok := secretKeyRef["name"].(string); ok && name != "" {
				key := "Secret:" + name
				if !seen[key] {
					seen[key] = true
					refs = append(refs, resourceReference{
						kind:    "Secret",
						name:    name,
						refType: "env.valueFrom.secretKeyRef",
					})
				}
			}
		}
	}

	return refs
}

// extractVolumeReferences extracts Secret and ConfigMap references from volumes
func extractVolumeReferences(volumes []interface{}, seen map[string]bool) []resourceReference {
	var refs []resourceReference

	for _, v := range volumes {
		volume, ok := v.(map[string]interface{})
		if !ok {
			continue
		}

		// configMap volume
		if cm, found, _ := unstructured.NestedMap(volume, "configMap"); found {
			if name, ok := cm["name"].(string); ok && name != "" {
				key := "ConfigMap:" + name
				if !seen[key] {
					seen[key] = true
					refs = append(refs, resourceReference{
						kind:    "ConfigMap",
						name:    name,
						refType: "volume.configMap",
					})
				}
			}
		}

		// secret volume
		if secret, found, _ := unstructured.NestedMap(volume, "secret"); found {
			if name, ok := secret["secretName"].(string); ok && name != "" {
				key := "Secret:" + name
				if !seen[key] {
					seen[key] = true
					refs = append(refs, resourceReference{
						kind:    "Secret",
						name:    name,
						refType: "volume.secret",
					})
				}
			}
		}

		// projected volumes can have secrets and configmaps
		if projected, found, _ := unstructured.NestedMap(volume, "projected"); found {
			sources, _, _ := unstructured.NestedSlice(projected, "sources")
			for _, s := range sources {
				source, ok := s.(map[string]interface{})
				if !ok {
					continue
				}

				if cm, found, _ := unstructured.NestedMap(source, "configMap"); found {
					if name, ok := cm["name"].(string); ok && name != "" {
						key := "ConfigMap:" + name
						if !seen[key] {
							seen[key] = true
							refs = append(refs, resourceReference{
								kind:    "ConfigMap",
								name:    name,
								refType: "volume.projected.configMap",
							})
						}
					}
				}

				if secret, found, _ := unstructured.NestedMap(source, "secret"); found {
					if name, ok := secret["name"].(string); ok && name != "" {
						key := "Secret:" + name
						if !seen[key] {
							seen[key] = true
							refs = append(refs, resourceReference{
								kind:    "Secret",
								name:    name,
								refType: "volume.projected.secret",
							})
						}
					}
				}
			}
		}
	}

	return refs
}

// kindToGVR maps a kind to its GroupVersionResource
func kindToGVR(kind string) schema.GroupVersionResource {
	switch kind {
	case "ConfigMap":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	case "Secret":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
	default:
		return schema.GroupVersionResource{}
	}
}
