// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Every first-class controller kind doctor lists must be optional: a cluster
// that does not serve it is an ordinary cluster, not a partial inventory.
// ArtifactGenerator's group was not in the list, because the kind had been
// registered under a group that does not serve it.
func TestDoctorTreatsEveryFirstClassControllerKindAsOptional(t *testing.T) {
	for _, gvr := range firstClassControllerGVRs() {
		notServed := apierrors.NewNotFound(schema.GroupResource{Group: gvr.Group, Resource: gvr.Resource}, "")
		notServed.ErrStatus.Message = "the server could not find the requested resource"
		if !optionalAPIUnavailable(gvr, notServed) {
			t.Errorf("%s: a cluster without this kind would be reported as a partial inventory", gvr)
		}
	}
}

func TestFluxArtifactKindsUseTheGroupsARealClusterServes(t *testing.T) {
	want := map[string]string{
		"ExternalArtifact":  "externalartifacts.source.toolkit.fluxcd.io",
		"ArtifactGenerator": "artifactgenerators.source.extensions.fluxcd.io",
	}
	for _, spec := range fluxOperatorControllerResources() {
		if name, ok := want[spec.Kind]; ok {
			if got := spec.GVR.Resource + "." + spec.GVR.Group; got != name {
				t.Errorf("%s is listed as %s; a Flux 2.10 cluster serves %s (api-resources, CI run 43451238398)", spec.Kind, got, name)
			}
			delete(want, spec.Kind)
		}
	}
	if len(want) != 0 {
		t.Errorf("kinds not registered: %v", want)
	}
}
