// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestFluxOperatorControllerResourcesAreFirstClass(t *testing.T) {
	want := map[string]schema.GroupVersionResource{
		"FluxInstance":             {Group: "fluxcd.controlplane.io", Version: "v1", Resource: "fluxinstances"},
		"FluxReport":               {Group: "fluxcd.controlplane.io", Version: "v1", Resource: "fluxreports"},
		"ResourceSet":              {Group: "fluxcd.controlplane.io", Version: "v1", Resource: "resourcesets"},
		"ResourceSetInputProvider": {Group: "fluxcd.controlplane.io", Version: "v1", Resource: "resourcesetinputproviders"},
		"ExternalArtifact":         {Group: "fluxcd.controlplane.io", Version: "v1", Resource: "externalartifacts"},
		"ArtifactGenerator":        {Group: "fluxcd.controlplane.io", Version: "v1", Resource: "artifactgenerators"},
	}
	got := map[string]controllerResourceSpec{}
	for _, spec := range firstClassControllerResources() {
		got[spec.Kind] = spec
	}

	for kind, gvr := range want {
		spec, ok := got[kind]
		if !ok {
			t.Fatalf("firstClassControllerResources missing %s", kind)
		}
		if spec.GVR != gvr {
			t.Fatalf("%s GVR = %#v, want %#v", kind, spec.GVR, gvr)
		}
		if !spec.Namespaced {
			t.Fatalf("%s Namespaced = false, want true", kind)
		}
		if spec.Owner != "Flux" {
			t.Fatalf("%s Owner = %q, want Flux", kind, spec.Owner)
		}
	}
}
