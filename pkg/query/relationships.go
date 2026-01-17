// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package query provides relationship queries for finding resource dependencies.
package query

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// Reference represents a reference from one resource to another
type Reference struct {
	From     ResourceID `json:"from"`
	To       ResourceID `json:"to"`
	Type     string     `json:"type"` // volume, envFrom, env, selector, backend, scaleTarget
	Path     string     `json:"path"` // JSONPath to the reference
}

// ResourceID identifies a Kubernetes resource
type ResourceID struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

func (r ResourceID) String() string {
	if r.Namespace != "" {
		return fmt.Sprintf("%s/%s (ns: %s)", r.Kind, r.Name, r.Namespace)
	}
	return fmt.Sprintf("%s/%s", r.Kind, r.Name)
}

// RelationshipFinder finds references between resources
type RelationshipFinder struct {
	client dynamic.Interface
}

// NewRelationshipFinder creates a new relationship finder
func NewRelationshipFinder(client dynamic.Interface) *RelationshipFinder {
	return &RelationshipFinder{client: client}
}

// FindReferences finds all resources that reference the target resource
func (rf *RelationshipFinder) FindReferences(ctx context.Context, targetKind, targetName, targetNamespace string) ([]Reference, error) {
	var refs []Reference

	target := ResourceID{Kind: targetKind, Name: targetName, Namespace: targetNamespace}

	switch strings.ToLower(targetKind) {
	case "configmap":
		r, err := rf.findConfigMapReferences(ctx, targetName, targetNamespace)
		if err != nil {
			return nil, err
		}
		refs = append(refs, r...)

	case "secret":
		r, err := rf.findSecretReferences(ctx, targetName, targetNamespace)
		if err != nil {
			return nil, err
		}
		refs = append(refs, r...)

	case "service":
		r, err := rf.findServiceReferences(ctx, targetName, targetNamespace)
		if err != nil {
			return nil, err
		}
		refs = append(refs, r...)

	case "deployment", "statefulset", "daemonset":
		r, err := rf.findWorkloadReferences(ctx, targetKind, targetName, targetNamespace)
		if err != nil {
			return nil, err
		}
		refs = append(refs, r...)

	case "persistentvolumeclaim", "pvc":
		r, err := rf.findPVCReferences(ctx, targetName, targetNamespace)
		if err != nil {
			return nil, err
		}
		refs = append(refs, r...)

	default:
		return nil, fmt.Errorf("relationship queries not supported for kind: %s", targetKind)
	}

	// Set the target on all references
	for i := range refs {
		refs[i].To = target
	}

	return refs, nil
}

// findConfigMapReferences finds all resources that reference a ConfigMap
func (rf *RelationshipFinder) findConfigMapReferences(ctx context.Context, name, namespace string) ([]Reference, error) {
	var refs []Reference

	// Check Deployments, StatefulSets, DaemonSets, Jobs, CronJobs
	workloadGVRs := []schema.GroupVersionResource{
		{Group: "apps", Version: "v1", Resource: "deployments"},
		{Group: "apps", Version: "v1", Resource: "statefulsets"},
		{Group: "apps", Version: "v1", Resource: "daemonsets"},
		{Group: "batch", Version: "v1", Resource: "jobs"},
		{Group: "batch", Version: "v1", Resource: "cronjobs"},
	}

	for _, gvr := range workloadGVRs {
		list, err := rf.client.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			continue // Skip if we can't list this resource type
		}

		for _, item := range list.Items {
			workloadRefs := rf.findConfigMapRefsInWorkload(&item, name)
			for _, ref := range workloadRefs {
				refs = append(refs, Reference{
					From: ResourceID{
						Kind:      item.GetKind(),
						Name:      item.GetName(),
						Namespace: item.GetNamespace(),
					},
					Type: ref.Type,
					Path: ref.Path,
				})
			}
		}
	}

	return refs, nil
}

// findSecretReferences finds all resources that reference a Secret
func (rf *RelationshipFinder) findSecretReferences(ctx context.Context, name, namespace string) ([]Reference, error) {
	var refs []Reference

	// Check workloads
	workloadGVRs := []schema.GroupVersionResource{
		{Group: "apps", Version: "v1", Resource: "deployments"},
		{Group: "apps", Version: "v1", Resource: "statefulsets"},
		{Group: "apps", Version: "v1", Resource: "daemonsets"},
		{Group: "batch", Version: "v1", Resource: "jobs"},
		{Group: "batch", Version: "v1", Resource: "cronjobs"},
	}

	for _, gvr := range workloadGVRs {
		list, err := rf.client.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}

		for _, item := range list.Items {
			workloadRefs := rf.findSecretRefsInWorkload(&item, name)
			for _, ref := range workloadRefs {
				refs = append(refs, Reference{
					From: ResourceID{
						Kind:      item.GetKind(),
						Name:      item.GetName(),
						Namespace: item.GetNamespace(),
					},
					Type: ref.Type,
					Path: ref.Path,
				})
			}
		}
	}

	// Check Ingresses for TLS secrets
	ingressGVR := schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"}
	ingresses, err := rf.client.Resource(ingressGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, ing := range ingresses.Items {
			if rf.ingressReferencesSecret(&ing, name) {
				refs = append(refs, Reference{
					From: ResourceID{
						Kind:      "Ingress",
						Name:      ing.GetName(),
						Namespace: ing.GetNamespace(),
					},
					Type: "tls",
					Path: "spec.tls[].secretName",
				})
			}
		}
	}

	// Check ServiceAccounts for imagePullSecrets
	saGVR := schema.GroupVersionResource{Version: "v1", Resource: "serviceaccounts"}
	sas, err := rf.client.Resource(saGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, sa := range sas.Items {
			if rf.serviceAccountReferencesSecret(&sa, name) {
				refs = append(refs, Reference{
					From: ResourceID{
						Kind:      "ServiceAccount",
						Name:      sa.GetName(),
						Namespace: sa.GetNamespace(),
					},
					Type: "imagePullSecret",
					Path: "imagePullSecrets[].name",
				})
			}
		}
	}

	return refs, nil
}

// findServiceReferences finds all resources that reference a Service
func (rf *RelationshipFinder) findServiceReferences(ctx context.Context, name, namespace string) ([]Reference, error) {
	var refs []Reference

	// Check Ingresses
	ingressGVR := schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"}
	ingresses, err := rf.client.Resource(ingressGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, ing := range ingresses.Items {
			if rf.ingressReferencesService(&ing, name) {
				refs = append(refs, Reference{
					From: ResourceID{
						Kind:      "Ingress",
						Name:      ing.GetName(),
						Namespace: ing.GetNamespace(),
					},
					Type: "backend",
					Path: "spec.rules[].http.paths[].backend.service.name",
				})
			}
		}
	}

	return refs, nil
}

// findWorkloadReferences finds references to a workload (HPA, PDB, Service)
func (rf *RelationshipFinder) findWorkloadReferences(ctx context.Context, kind, name, namespace string) ([]Reference, error) {
	var refs []Reference

	// Check HPAs
	hpaGVR := schema.GroupVersionResource{Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"}
	hpas, err := rf.client.Resource(hpaGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, hpa := range hpas.Items {
			if rf.hpaReferencesWorkload(&hpa, kind, name) {
				refs = append(refs, Reference{
					From: ResourceID{
						Kind:      "HorizontalPodAutoscaler",
						Name:      hpa.GetName(),
						Namespace: hpa.GetNamespace(),
					},
					Type: "scaleTarget",
					Path: "spec.scaleTargetRef",
				})
			}
		}
	}

	// Check PDBs (need to check selector)
	pdbGVR := schema.GroupVersionResource{Group: "policy", Version: "v1", Resource: "poddisruptionbudgets"}
	pdbs, err := rf.client.Resource(pdbGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		// Get the workload's selector
		workload, err := rf.getWorkload(ctx, kind, name, namespace)
		if err == nil {
			workloadLabels := rf.getWorkloadPodLabels(workload)
			for _, pdb := range pdbs.Items {
				if rf.pdbSelectsLabels(&pdb, workloadLabels) {
					refs = append(refs, Reference{
						From: ResourceID{
							Kind:      "PodDisruptionBudget",
							Name:      pdb.GetName(),
							Namespace: pdb.GetNamespace(),
						},
						Type: "selector",
						Path: "spec.selector",
					})
				}
			}
		}
	}

	return refs, nil
}

// findPVCReferences finds resources mounting a PVC
func (rf *RelationshipFinder) findPVCReferences(ctx context.Context, name, namespace string) ([]Reference, error) {
	var refs []Reference

	// Check Pods directly mounting this PVC
	podGVR := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	pods, err := rf.client.Resource(podGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			if rf.podMountsPVC(&pod, name) {
				refs = append(refs, Reference{
					From: ResourceID{
						Kind:      "Pod",
						Name:      pod.GetName(),
						Namespace: pod.GetNamespace(),
					},
					Type: "volume",
					Path: "spec.volumes[].persistentVolumeClaim.claimName",
				})
			}
		}
	}

	// Check workloads
	workloadGVRs := []schema.GroupVersionResource{
		{Group: "apps", Version: "v1", Resource: "deployments"},
		{Group: "apps", Version: "v1", Resource: "statefulsets"},
		{Group: "apps", Version: "v1", Resource: "daemonsets"},
	}

	for _, gvr := range workloadGVRs {
		list, err := rf.client.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}

		for _, item := range list.Items {
			if rf.workloadMountsPVC(&item, name) {
				refs = append(refs, Reference{
					From: ResourceID{
						Kind:      item.GetKind(),
						Name:      item.GetName(),
						Namespace: item.GetNamespace(),
					},
					Type: "volume",
					Path: "spec.template.spec.volumes[].persistentVolumeClaim.claimName",
				})
			}
		}
	}

	return refs, nil
}

// Helper functions

type refInfo struct {
	Type string
	Path string
}

func (rf *RelationshipFinder) findConfigMapRefsInWorkload(workload *unstructured.Unstructured, cmName string) []refInfo {
	var refs []refInfo
	containers := rf.getContainers(workload)

	for _, c := range containers {
		container, ok := c.(map[string]interface{})
		if !ok {
			continue
		}

		// Check envFrom
		if envFrom, ok := container["envFrom"].([]interface{}); ok {
			for _, ef := range envFrom {
				if efMap, ok := ef.(map[string]interface{}); ok {
					if cmRef, ok := efMap["configMapRef"].(map[string]interface{}); ok {
						if name, ok := cmRef["name"].(string); ok && name == cmName {
							refs = append(refs, refInfo{Type: "envFrom", Path: "spec.template.spec.containers[].envFrom[].configMapRef"})
						}
					}
				}
			}
		}

		// Check env valueFrom
		if env, ok := container["env"].([]interface{}); ok {
			for _, e := range env {
				if envMap, ok := e.(map[string]interface{}); ok {
					if valueFrom, ok := envMap["valueFrom"].(map[string]interface{}); ok {
						if cmKeyRef, ok := valueFrom["configMapKeyRef"].(map[string]interface{}); ok {
							if name, ok := cmKeyRef["name"].(string); ok && name == cmName {
								refs = append(refs, refInfo{Type: "env", Path: "spec.template.spec.containers[].env[].valueFrom.configMapKeyRef"})
							}
						}
					}
				}
			}
		}
	}

	// Check volumes
	volumes := rf.getVolumes(workload)
	for _, v := range volumes {
		vol, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		if cm, ok := vol["configMap"].(map[string]interface{}); ok {
			if name, ok := cm["name"].(string); ok && name == cmName {
				refs = append(refs, refInfo{Type: "volume", Path: "spec.template.spec.volumes[].configMap"})
			}
		}
	}

	return refs
}

func (rf *RelationshipFinder) findSecretRefsInWorkload(workload *unstructured.Unstructured, secretName string) []refInfo {
	var refs []refInfo
	containers := rf.getContainers(workload)

	for _, c := range containers {
		container, ok := c.(map[string]interface{})
		if !ok {
			continue
		}

		// Check envFrom
		if envFrom, ok := container["envFrom"].([]interface{}); ok {
			for _, ef := range envFrom {
				if efMap, ok := ef.(map[string]interface{}); ok {
					if secretRef, ok := efMap["secretRef"].(map[string]interface{}); ok {
						if name, ok := secretRef["name"].(string); ok && name == secretName {
							refs = append(refs, refInfo{Type: "envFrom", Path: "spec.template.spec.containers[].envFrom[].secretRef"})
						}
					}
				}
			}
		}

		// Check env valueFrom
		if env, ok := container["env"].([]interface{}); ok {
			for _, e := range env {
				if envMap, ok := e.(map[string]interface{}); ok {
					if valueFrom, ok := envMap["valueFrom"].(map[string]interface{}); ok {
						if secretKeyRef, ok := valueFrom["secretKeyRef"].(map[string]interface{}); ok {
							if name, ok := secretKeyRef["name"].(string); ok && name == secretName {
								refs = append(refs, refInfo{Type: "env", Path: "spec.template.spec.containers[].env[].valueFrom.secretKeyRef"})
							}
						}
					}
				}
			}
		}
	}

	// Check volumes
	volumes := rf.getVolumes(workload)
	for _, v := range volumes {
		vol, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		if secret, ok := vol["secret"].(map[string]interface{}); ok {
			if name, ok := secret["secretName"].(string); ok && name == secretName {
				refs = append(refs, refInfo{Type: "volume", Path: "spec.template.spec.volumes[].secret"})
			}
		}
	}

	return refs
}

func (rf *RelationshipFinder) getContainers(workload *unstructured.Unstructured) []interface{} {
	// For CronJobs, path is different
	if workload.GetKind() == "CronJob" {
		containers, _, _ := unstructured.NestedSlice(workload.Object, "spec", "jobTemplate", "spec", "template", "spec", "containers")
		return containers
	}
	containers, _, _ := unstructured.NestedSlice(workload.Object, "spec", "template", "spec", "containers")
	return containers
}

func (rf *RelationshipFinder) getVolumes(workload *unstructured.Unstructured) []interface{} {
	if workload.GetKind() == "CronJob" {
		volumes, _, _ := unstructured.NestedSlice(workload.Object, "spec", "jobTemplate", "spec", "template", "spec", "volumes")
		return volumes
	}
	volumes, _, _ := unstructured.NestedSlice(workload.Object, "spec", "template", "spec", "volumes")
	return volumes
}

func (rf *RelationshipFinder) ingressReferencesSecret(ing *unstructured.Unstructured, secretName string) bool {
	tls, _, _ := unstructured.NestedSlice(ing.Object, "spec", "tls")
	for _, t := range tls {
		if tlsMap, ok := t.(map[string]interface{}); ok {
			if name, ok := tlsMap["secretName"].(string); ok && name == secretName {
				return true
			}
		}
	}
	return false
}

func (rf *RelationshipFinder) ingressReferencesService(ing *unstructured.Unstructured, svcName string) bool {
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
			if name, ok := service["name"].(string); ok && name == svcName {
				return true
			}
		}
	}
	return false
}

func (rf *RelationshipFinder) serviceAccountReferencesSecret(sa *unstructured.Unstructured, secretName string) bool {
	secrets, _, _ := unstructured.NestedSlice(sa.Object, "secrets")
	for _, s := range secrets {
		if sMap, ok := s.(map[string]interface{}); ok {
			if name, ok := sMap["name"].(string); ok && name == secretName {
				return true
			}
		}
	}
	imagePullSecrets, _, _ := unstructured.NestedSlice(sa.Object, "imagePullSecrets")
	for _, s := range imagePullSecrets {
		if sMap, ok := s.(map[string]interface{}); ok {
			if name, ok := sMap["name"].(string); ok && name == secretName {
				return true
			}
		}
	}
	return false
}

func (rf *RelationshipFinder) hpaReferencesWorkload(hpa *unstructured.Unstructured, kind, name string) bool {
	scaleTargetRef, _, _ := unstructured.NestedMap(hpa.Object, "spec", "scaleTargetRef")
	targetKind, _ := scaleTargetRef["kind"].(string)
	targetName, _ := scaleTargetRef["name"].(string)
	return strings.EqualFold(targetKind, kind) && targetName == name
}

func (rf *RelationshipFinder) getWorkload(ctx context.Context, kind, name, namespace string) (*unstructured.Unstructured, error) {
	var gvr schema.GroupVersionResource
	switch strings.ToLower(kind) {
	case "deployment":
		gvr = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	case "statefulset":
		gvr = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}
	case "daemonset":
		gvr = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}
	default:
		return nil, fmt.Errorf("unsupported workload kind: %s", kind)
	}
	return rf.client.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (rf *RelationshipFinder) getWorkloadPodLabels(workload *unstructured.Unstructured) map[string]string {
	labels, _, _ := unstructured.NestedStringMap(workload.Object, "spec", "template", "metadata", "labels")
	return labels
}

func (rf *RelationshipFinder) pdbSelectsLabels(pdb *unstructured.Unstructured, labels map[string]string) bool {
	selector, _, _ := unstructured.NestedMap(pdb.Object, "spec", "selector")
	matchLabels, _, _ := unstructured.NestedStringMap(selector, "matchLabels")

	if len(matchLabels) == 0 {
		return false
	}

	for k, v := range matchLabels {
		if labels[k] != v {
			return false
		}
	}
	return true
}

func (rf *RelationshipFinder) podMountsPVC(pod *unstructured.Unstructured, pvcName string) bool {
	volumes, _, _ := unstructured.NestedSlice(pod.Object, "spec", "volumes")
	for _, v := range volumes {
		vol, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		if pvc, ok := vol["persistentVolumeClaim"].(map[string]interface{}); ok {
			if name, ok := pvc["claimName"].(string); ok && name == pvcName {
				return true
			}
		}
	}
	return false
}

func (rf *RelationshipFinder) workloadMountsPVC(workload *unstructured.Unstructured, pvcName string) bool {
	volumes := rf.getVolumes(workload)
	for _, v := range volumes {
		vol, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		if pvc, ok := vol["persistentVolumeClaim"].(map[string]interface{}); ok {
			if name, ok := pvc["claimName"].(string); ok && name == pvcName {
				return true
			}
		}
	}
	return false
}
