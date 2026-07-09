// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type controllerResourceSpec struct {
	GVR        schema.GroupVersionResource
	Kind       string
	Namespaced bool
	Owner      string
}

func firstClassControllerResources() []controllerResourceSpec {
	out := make([]controllerResourceSpec, 0, len(fluxOperatorControllerResources())+len(sveltosControllerResources())+len(modelplaneControllerResources()))
	out = append(out, fluxOperatorControllerResources()...)
	out = append(out, sveltosControllerResources()...)
	out = append(out, modelplaneControllerResources()...)
	return out
}

func firstClassControllerGVRs() []schema.GroupVersionResource {
	specs := firstClassControllerResources()
	out := make([]schema.GroupVersionResource, 0, len(specs))
	for _, spec := range specs {
		out = append(out, spec.GVR)
	}
	return out
}

func fluxOperatorControllerResources() []controllerResourceSpec {
	return []controllerResourceSpec{
		{GVR: schema.GroupVersionResource{Group: "fluxcd.controlplane.io", Version: "v1", Resource: "fluxinstances"}, Kind: "FluxInstance", Namespaced: true, Owner: "Flux"},
		{GVR: schema.GroupVersionResource{Group: "fluxcd.controlplane.io", Version: "v1", Resource: "fluxreports"}, Kind: "FluxReport", Namespaced: true, Owner: "Flux"},
		{GVR: schema.GroupVersionResource{Group: "fluxcd.controlplane.io", Version: "v1", Resource: "resourcesets"}, Kind: "ResourceSet", Namespaced: true, Owner: "Flux"},
		{GVR: schema.GroupVersionResource{Group: "fluxcd.controlplane.io", Version: "v1", Resource: "resourcesetinputproviders"}, Kind: "ResourceSetInputProvider", Namespaced: true, Owner: "Flux"},
		{GVR: schema.GroupVersionResource{Group: "fluxcd.controlplane.io", Version: "v1", Resource: "externalartifacts"}, Kind: "ExternalArtifact", Namespaced: true, Owner: "Flux"},
		{GVR: schema.GroupVersionResource{Group: "fluxcd.controlplane.io", Version: "v1", Resource: "artifactgenerators"}, Kind: "ArtifactGenerator", Namespaced: true, Owner: "Flux"},
	}
}

func sveltosControllerResources() []controllerResourceSpec {
	return []controllerResourceSpec{
		{GVR: schema.GroupVersionResource{Group: "config.projectsveltos.io", Version: "v1beta1", Resource: "clusterprofiles"}, Kind: "ClusterProfile", Owner: "Sveltos"},
		{GVR: schema.GroupVersionResource{Group: "config.projectsveltos.io", Version: "v1beta1", Resource: "profiles"}, Kind: "Profile", Namespaced: true, Owner: "Sveltos"},
		{GVR: schema.GroupVersionResource{Group: "config.projectsveltos.io", Version: "v1beta1", Resource: "clustersummaries"}, Kind: "ClusterSummary", Namespaced: true, Owner: "Sveltos"},
		{GVR: schema.GroupVersionResource{Group: "config.projectsveltos.io", Version: "v1beta1", Resource: "clusterconfigurations"}, Kind: "ClusterConfiguration", Namespaced: true, Owner: "Sveltos"},
		{GVR: schema.GroupVersionResource{Group: "config.projectsveltos.io", Version: "v1beta1", Resource: "clusterreports"}, Kind: "ClusterReport", Namespaced: true, Owner: "Sveltos"},
		{GVR: schema.GroupVersionResource{Group: "config.projectsveltos.io", Version: "v1beta1", Resource: "clusterpromotions"}, Kind: "ClusterPromotion", Owner: "Sveltos"},
		{GVR: schema.GroupVersionResource{Group: "lib.projectsveltos.io", Version: "v1beta1", Resource: "eventsources"}, Kind: "EventSource", Owner: "Sveltos"},
		{GVR: schema.GroupVersionResource{Group: "lib.projectsveltos.io", Version: "v1beta1", Resource: "eventtriggers"}, Kind: "EventTrigger", Owner: "Sveltos"},
		{GVR: schema.GroupVersionResource{Group: "lib.projectsveltos.io", Version: "v1beta1", Resource: "clusterhealthchecks"}, Kind: "ClusterHealthCheck", Owner: "Sveltos"},
		{GVR: schema.GroupVersionResource{Group: "lib.projectsveltos.io", Version: "v1beta1", Resource: "healthcheckreports"}, Kind: "HealthCheckReport", Namespaced: true, Owner: "Sveltos"},
		{GVR: schema.GroupVersionResource{Group: "lib.projectsveltos.io", Version: "v1beta1", Resource: "eventreports"}, Kind: "EventReport", Namespaced: true, Owner: "Sveltos"},
	}
}

func modelplaneControllerResources() []controllerResourceSpec {
	return []controllerResourceSpec{
		{GVR: schema.GroupVersionResource{Group: "modelplane.ai", Version: "v1alpha1", Resource: "inferencegateways"}, Kind: "InferenceGateway", Namespaced: true, Owner: "Modelplane"},
		{GVR: schema.GroupVersionResource{Group: "modelplane.ai", Version: "v1alpha1", Resource: "inferenceclasses"}, Kind: "InferenceClass", Namespaced: true, Owner: "Modelplane"},
		{GVR: schema.GroupVersionResource{Group: "modelplane.ai", Version: "v1alpha1", Resource: "inferenceclusters"}, Kind: "InferenceCluster", Namespaced: true, Owner: "Modelplane"},
		{GVR: schema.GroupVersionResource{Group: "modelplane.ai", Version: "v1alpha1", Resource: "modeldeployments"}, Kind: "ModelDeployment", Namespaced: true, Owner: "Modelplane"},
		{GVR: schema.GroupVersionResource{Group: "modelplane.ai", Version: "v1alpha1", Resource: "modelservices"}, Kind: "ModelService", Namespaced: true, Owner: "Modelplane"},
		{GVR: schema.GroupVersionResource{Group: "modelplane.ai", Version: "v1alpha1", Resource: "modelendpoints"}, Kind: "ModelEndpoint", Namespaced: true, Owner: "Modelplane"},
		{GVR: schema.GroupVersionResource{Group: "modelplane.ai", Version: "v1alpha1", Resource: "modelcaches"}, Kind: "ModelCache", Namespaced: true, Owner: "Modelplane"},
		{GVR: schema.GroupVersionResource{Group: "modelplane.ai", Version: "v1alpha1", Resource: "modelreplicas"}, Kind: "ModelReplica", Namespaced: true, Owner: "Modelplane"},
		{GVR: schema.GroupVersionResource{Group: "infrastructure.modelplane.ai", Version: "v1alpha1", Resource: "eksclusters"}, Kind: "EKSCluster", Namespaced: true, Owner: "Modelplane"},
		{GVR: schema.GroupVersionResource{Group: "infrastructure.modelplane.ai", Version: "v1alpha1", Resource: "gkeclusters"}, Kind: "GKECluster", Namespaced: true, Owner: "Modelplane"},
		{GVR: schema.GroupVersionResource{Group: "infrastructure.modelplane.ai", Version: "v1alpha1", Resource: "servingstacks"}, Kind: "ServingStack", Namespaced: true, Owner: "Modelplane"},
	}
}

func controllerResourceByKind(kind string) (controllerResourceSpec, bool) {
	for _, spec := range firstClassControllerResources() {
		if strings.EqualFold(spec.Kind, kind) {
			return spec, true
		}
	}
	return controllerResourceSpec{}, false
}

func normalizeFirstClassControllerKind(kind string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(kind))
	key = strings.ReplaceAll(key, "-", "")
	key = strings.ReplaceAll(key, "_", "")
	if key == "" {
		return "", false
	}
	aliases := map[string]string{
		"md": "ModelDeployment",
		"ms": "ModelService",
		"me": "ModelEndpoint",
		"mr": "ModelReplica",
		"mc": "ModelCache",
		"ig": "InferenceGateway",
		"ic": "InferenceClass",
	}
	if v := aliases[key]; v != "" {
		return v, true
	}
	for _, spec := range firstClassControllerResources() {
		kindKey := strings.ToLower(spec.Kind)
		kindKey = strings.ReplaceAll(kindKey, "-", "")
		kindKey = strings.ReplaceAll(kindKey, "_", "")
		if key == kindKey || key == strings.ToLower(spec.GVR.Resource) {
			return spec.Kind, true
		}
	}
	return "", false
}

func listControllerResource(ctx context.Context, dynClient dynamic.Interface, spec controllerResourceSpec, namespace string) (*unstructured.UnstructuredList, error) {
	if dynClient == nil {
		return nil, fmt.Errorf("dynamic client is nil")
	}
	if spec.Namespaced && strings.TrimSpace(namespace) != "" {
		return dynClient.Resource(spec.GVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	}
	return dynClient.Resource(spec.GVR).List(ctx, metav1.ListOptions{})
}

func getControllerResource(ctx context.Context, dynClient dynamic.Interface, spec controllerResourceSpec, name, namespace string) (*unstructured.Unstructured, error) {
	if dynClient == nil {
		return nil, fmt.Errorf("dynamic client is nil")
	}
	if spec.Namespaced {
		ns := strings.TrimSpace(namespace)
		if ns == "" {
			return nil, fmt.Errorf("%s is namespaced; pass --namespace", spec.Kind)
		}
		return dynClient.Resource(spec.GVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	}
	return dynClient.Resource(spec.GVR).Get(ctx, name, metav1.GetOptions{})
}
