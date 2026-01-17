// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package query

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// DanglingReference represents a reference to a non-existent resource
type DanglingReference struct {
	From        ResourceID `json:"from"`
	To          ResourceID `json:"to"`
	Type        string     `json:"type"`   // selector, scaleTarget, backend, volume, etc.
	Reason      string     `json:"reason"` // "not found", "no matching pods", etc.
	Suggestion  string     `json:"suggestion,omitempty"`
}

// DanglingFinder finds dangling references in the cluster
type DanglingFinder struct {
	client dynamic.Interface
}

// NewDanglingFinder creates a new dangling reference finder
func NewDanglingFinder(client dynamic.Interface) *DanglingFinder {
	return &DanglingFinder{client: client}
}

// FindAll finds all dangling references in a namespace (or cluster-wide if namespace is empty)
func (df *DanglingFinder) FindAll(ctx context.Context, namespace string) ([]DanglingReference, error) {
	var dangling []DanglingReference

	// Find services with no matching pods
	svcDangling, err := df.findDanglingServices(ctx, namespace)
	if err == nil {
		dangling = append(dangling, svcDangling...)
	}

	// Find HPAs with missing scale targets
	hpaDangling, err := df.findDanglingHPAs(ctx, namespace)
	if err == nil {
		dangling = append(dangling, hpaDangling...)
	}

	// Find Ingresses with missing services
	ingDangling, err := df.findDanglingIngresses(ctx, namespace)
	if err == nil {
		dangling = append(dangling, ingDangling...)
	}

	// Find PVCs not mounted by any pod
	pvcDangling, err := df.findUnmountedPVCs(ctx, namespace)
	if err == nil {
		dangling = append(dangling, pvcDangling...)
	}

	// Find PDBs with no matching pods
	pdbDangling, err := df.findDanglingPDBs(ctx, namespace)
	if err == nil {
		dangling = append(dangling, pdbDangling...)
	}

	return dangling, nil
}

// findDanglingServices finds services with no matching pods
func (df *DanglingFinder) findDanglingServices(ctx context.Context, namespace string) ([]DanglingReference, error) {
	var dangling []DanglingReference

	svcGVR := schema.GroupVersionResource{Version: "v1", Resource: "services"}
	podGVR := schema.GroupVersionResource{Version: "v1", Resource: "pods"}

	var services *unstructured.UnstructuredList
	var err error
	if namespace != "" {
		services, err = df.client.Resource(svcGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	} else {
		services, err = df.client.Resource(svcGVR).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, err
	}

	for _, svc := range services.Items {
		// Skip ExternalName and headless services
		svcType, _, _ := unstructured.NestedString(svc.Object, "spec", "type")
		if svcType == "ExternalName" {
			continue
		}
		clusterIP, _, _ := unstructured.NestedString(svc.Object, "spec", "clusterIP")
		if clusterIP == "None" {
			continue // Headless service
		}

		// Get selector
		selector, _, _ := unstructured.NestedStringMap(svc.Object, "spec", "selector")
		if len(selector) == 0 {
			continue // No selector = external service
		}

		// Build label selector
		labelSelector := labels.Set(selector).AsSelector().String()

		// Find matching pods
		ns := svc.GetNamespace()
		pods, err := df.client.Resource(podGVR).Namespace(ns).List(ctx, metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err != nil {
			continue
		}

		if len(pods.Items) == 0 {
			dangling = append(dangling, DanglingReference{
				From: ResourceID{
					Kind:      "Service",
					Name:      svc.GetName(),
					Namespace: ns,
				},
				To: ResourceID{
					Kind: "Pod",
					Name: fmt.Sprintf("(selector: %s)", labelSelector),
				},
				Type:       "selector",
				Reason:     "no matching pods",
				Suggestion: "Check if the deployment exists and has matching labels",
			})
		}
	}

	return dangling, nil
}

// findDanglingHPAs finds HPAs with missing scale targets
func (df *DanglingFinder) findDanglingHPAs(ctx context.Context, namespace string) ([]DanglingReference, error) {
	var dangling []DanglingReference

	hpaGVR := schema.GroupVersionResource{Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"}

	var hpas *unstructured.UnstructuredList
	var err error
	if namespace != "" {
		hpas, err = df.client.Resource(hpaGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	} else {
		hpas, err = df.client.Resource(hpaGVR).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		// Try v1 API
		hpaGVR = schema.GroupVersionResource{Group: "autoscaling", Version: "v1", Resource: "horizontalpodautoscalers"}
		if namespace != "" {
			hpas, err = df.client.Resource(hpaGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
		} else {
			hpas, err = df.client.Resource(hpaGVR).List(ctx, metav1.ListOptions{})
		}
		if err != nil {
			return nil, err
		}
	}

	for _, hpa := range hpas.Items {
		scaleTargetRef, _, _ := unstructured.NestedMap(hpa.Object, "spec", "scaleTargetRef")
		if scaleTargetRef == nil {
			continue
		}

		targetKind, _ := scaleTargetRef["kind"].(string)
		targetName, _ := scaleTargetRef["name"].(string)
		targetAPIVersion, _ := scaleTargetRef["apiVersion"].(string)

		// Try to get the target
		targetGVR := df.kindToGVR(targetKind, targetAPIVersion)
		ns := hpa.GetNamespace()

		_, err := df.client.Resource(targetGVR).Namespace(ns).Get(ctx, targetName, metav1.GetOptions{})
		if err != nil {
			dangling = append(dangling, DanglingReference{
				From: ResourceID{
					Kind:      "HorizontalPodAutoscaler",
					Name:      hpa.GetName(),
					Namespace: ns,
				},
				To: ResourceID{
					Kind:      targetKind,
					Name:      targetName,
					Namespace: ns,
				},
				Type:       "scaleTarget",
				Reason:     "target not found",
				Suggestion: fmt.Sprintf("Create %s/%s or delete this HPA", targetKind, targetName),
			})
		}
	}

	return dangling, nil
}

// findDanglingIngresses finds Ingresses with missing backend services
func (df *DanglingFinder) findDanglingIngresses(ctx context.Context, namespace string) ([]DanglingReference, error) {
	var dangling []DanglingReference

	ingGVR := schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"}
	svcGVR := schema.GroupVersionResource{Version: "v1", Resource: "services"}

	var ingresses *unstructured.UnstructuredList
	var err error
	if namespace != "" {
		ingresses, err = df.client.Resource(ingGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	} else {
		ingresses, err = df.client.Resource(ingGVR).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, err
	}

	for _, ing := range ingresses.Items {
		ns := ing.GetNamespace()
		rules, _, _ := unstructured.NestedSlice(ing.Object, "spec", "rules")

		for _, r := range rules {
			rule, ok := r.(map[string]interface{})
			if !ok {
				continue
			}
			http, ok := rule["http"].(map[string]interface{})
			if !ok {
				continue
			}
			paths, ok := http["paths"].([]interface{})
			if !ok {
				continue
			}

			for _, p := range paths {
				path, ok := p.(map[string]interface{})
				if !ok {
					continue
				}
				backend, ok := path["backend"].(map[string]interface{})
				if !ok {
					continue
				}
				service, ok := backend["service"].(map[string]interface{})
				if !ok {
					continue
				}
				svcName, _ := service["name"].(string)
				if svcName == "" {
					continue
				}

				// Check if service exists
				_, err := df.client.Resource(svcGVR).Namespace(ns).Get(ctx, svcName, metav1.GetOptions{})
				if err != nil {
					dangling = append(dangling, DanglingReference{
						From: ResourceID{
							Kind:      "Ingress",
							Name:      ing.GetName(),
							Namespace: ns,
						},
						To: ResourceID{
							Kind:      "Service",
							Name:      svcName,
							Namespace: ns,
						},
						Type:       "backend",
						Reason:     "service not found",
						Suggestion: fmt.Sprintf("Create Service/%s or update Ingress backend", svcName),
					})
				}
			}
		}
	}

	return dangling, nil
}

// findUnmountedPVCs finds PVCs not mounted by any pod
func (df *DanglingFinder) findUnmountedPVCs(ctx context.Context, namespace string) ([]DanglingReference, error) {
	var dangling []DanglingReference

	pvcGVR := schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumeclaims"}
	podGVR := schema.GroupVersionResource{Version: "v1", Resource: "pods"}

	var pvcs *unstructured.UnstructuredList
	var err error
	if namespace != "" {
		pvcs, err = df.client.Resource(pvcGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	} else {
		pvcs, err = df.client.Resource(pvcGVR).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, err
	}

	// Build set of mounted PVCs
	mountedPVCs := make(map[string]bool)

	var pods *unstructured.UnstructuredList
	if namespace != "" {
		pods, err = df.client.Resource(podGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	} else {
		pods, err = df.client.Resource(podGVR).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, err
	}

	for _, pod := range pods.Items {
		volumes, _, _ := unstructured.NestedSlice(pod.Object, "spec", "volumes")
		for _, v := range volumes {
			vol, ok := v.(map[string]interface{})
			if !ok {
				continue
			}
			if pvc, ok := vol["persistentVolumeClaim"].(map[string]interface{}); ok {
				if claimName, ok := pvc["claimName"].(string); ok {
					key := fmt.Sprintf("%s/%s", pod.GetNamespace(), claimName)
					mountedPVCs[key] = true
				}
			}
		}
	}

	// Check which PVCs are not mounted
	for _, pvc := range pvcs.Items {
		key := fmt.Sprintf("%s/%s", pvc.GetNamespace(), pvc.GetName())
		if !mountedPVCs[key] {
			// Check if PVC is bound
			phase, _, _ := unstructured.NestedString(pvc.Object, "status", "phase")
			if phase == "Bound" {
				dangling = append(dangling, DanglingReference{
					From: ResourceID{
						Kind:      "PersistentVolumeClaim",
						Name:      pvc.GetName(),
						Namespace: pvc.GetNamespace(),
					},
					To: ResourceID{
						Kind: "Pod",
						Name: "(none)",
					},
					Type:       "volume",
					Reason:     "not mounted by any pod",
					Suggestion: "Delete if no longer needed (may contain data!)",
				})
			}
		}
	}

	return dangling, nil
}

// findDanglingPDBs finds PDBs with no matching pods
func (df *DanglingFinder) findDanglingPDBs(ctx context.Context, namespace string) ([]DanglingReference, error) {
	var dangling []DanglingReference

	pdbGVR := schema.GroupVersionResource{Group: "policy", Version: "v1", Resource: "poddisruptionbudgets"}
	podGVR := schema.GroupVersionResource{Version: "v1", Resource: "pods"}

	var pdbs *unstructured.UnstructuredList
	var err error
	if namespace != "" {
		pdbs, err = df.client.Resource(pdbGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	} else {
		pdbs, err = df.client.Resource(pdbGVR).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, err
	}

	for _, pdb := range pdbs.Items {
		selector, _, _ := unstructured.NestedMap(pdb.Object, "spec", "selector")
		if selector == nil {
			continue
		}

		matchLabels, _, _ := unstructured.NestedStringMap(selector, "matchLabels")
		if len(matchLabels) == 0 {
			continue
		}

		labelSelector := labels.Set(matchLabels).AsSelector().String()
		ns := pdb.GetNamespace()

		pods, err := df.client.Resource(podGVR).Namespace(ns).List(ctx, metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err != nil {
			continue
		}

		if len(pods.Items) == 0 {
			dangling = append(dangling, DanglingReference{
				From: ResourceID{
					Kind:      "PodDisruptionBudget",
					Name:      pdb.GetName(),
					Namespace: ns,
				},
				To: ResourceID{
					Kind: "Pod",
					Name: fmt.Sprintf("(selector: %s)", labelSelector),
				},
				Type:       "selector",
				Reason:     "no matching pods",
				Suggestion: "Check if the deployment exists or delete this PDB",
			})
		}
	}

	return dangling, nil
}

// kindToGVR maps a kind to its GroupVersionResource
func (df *DanglingFinder) kindToGVR(kind, apiVersion string) schema.GroupVersionResource {
	// Parse apiVersion (e.g., "apps/v1" -> group="apps", version="v1")
	var group, version string
	if strings.Contains(apiVersion, "/") {
		parts := strings.SplitN(apiVersion, "/", 2)
		group = parts[0]
		version = parts[1]
	} else {
		version = apiVersion
	}

	// Map kind to resource name (lowercase + plural)
	resource := strings.ToLower(kind) + "s"

	// Handle special cases
	switch strings.ToLower(kind) {
	case "deployment":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	case "statefulset":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}
	case "daemonset":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}
	case "replicaset":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}
	}

	return schema.GroupVersionResource{Group: group, Version: version, Resource: resource}
}
