// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import "k8s.io/apimachinery/pkg/runtime/schema"

// mapResourcesForKind narrows only GVRs whose canonical Kind is known. A
// configured custom resource has no Kind in its config schema, so it remains
// eligible for listing to avoid hiding a custom resource with the same Kind.
// Empty or unsupported filters retain the historical unfiltered request set.
func mapResourcesForKind(resources []schema.GroupVersionResource, requestedKind string) []schema.GroupVersionResource {
	if requestedKind == "" {
		return append([]schema.GroupVersionResource(nil), resources...)
	}

	kindByGVR := map[schema.GroupVersionResource]string{
		{Group: "apps", Version: "v1", Resource: "deployments"}:                           "Deployment",
		{Group: "apps", Version: "v1", Resource: "statefulsets"}:                          "StatefulSet",
		{Group: "apps", Version: "v1", Resource: "daemonsets"}:                            "DaemonSet",
		{Group: "batch", Version: "v1", Resource: "jobs"}:                                 "Job",
		{Group: "batch", Version: "v1", Resource: "cronjobs"}:                             "CronJob",
		{Group: "", Version: "v1", Resource: "services"}:                                  "Service",
		{Group: "", Version: "v1", Resource: "configmaps"}:                                "ConfigMap",
		{Group: "", Version: "v1", Resource: "secrets"}:                                   "Secret",
		{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"}:                "Ingress",
		{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"}:   "GitRepository",
		{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"}: "Kustomization",
		{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"}:        "HelmRelease",
		{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}:             "Application",
	}
	knownKinds := make(map[string]struct{}, len(kindByGVR))
	for _, kind := range kindByGVR {
		knownKinds[kind] = struct{}{}
	}
	for _, spec := range firstClassControllerResources() {
		kindByGVR[spec.GVR] = spec.Kind
		knownKinds[spec.Kind] = struct{}{}
	}
	if _, known := knownKinds[requestedKind]; !known {
		return append([]schema.GroupVersionResource(nil), resources...)
	}

	selected := make([]schema.GroupVersionResource, 0, len(resources))
	for _, gvr := range resources {
		kind, known := kindByGVR[gvr]
		if !known || kind == requestedKind {
			selected = append(selected, gvr)
		}
	}
	return selected
}
