// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"slices"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

func kroScopeObject(api, kind, name, namespace, uid string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(api)
	obj.SetKind(kind)
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.SetUID(types.UID(uid))
	return obj
}

func TestKroOwnerReferenceScopeAndUID(t *testing.T) {
	for _, tc := range []struct {
		name, api, kind, ns, uid, ownerUID, reason string
		present, duplicate                         bool
	}{
		{name: "same namespace", api: "apps.kro.run/v1", kind: "WebApp", ns: "prod", uid: "parent", ownerUID: "parent", present: true},
		{name: "cluster parent", api: "apps.kro.run/v1", kind: "WebApp", uid: "parent", ownerUID: "parent", present: true},
		{name: "foreign namespace", api: "apps.kro.run/v1", kind: "WebApp", ns: "dev", uid: "parent", ownerUID: "parent", reason: "owner_uid_not_observed"},
		{name: "wrong kind", api: "apps.kro.run/v1", kind: "Other", ns: "prod", uid: "parent", ownerUID: "parent", reason: "owner_uid_not_observed"},
		{name: "wrong version", api: "apps.kro.run/v2", kind: "WebApp", ns: "prod", uid: "parent", ownerUID: "parent", reason: "owner_uid_not_observed"},
		{name: "replaced UID", api: "apps.kro.run/v1", kind: "WebApp", ns: "prod", uid: "replacement", ownerUID: "parent", reason: "owner_uid_not_observed"},
		{name: "duplicate candidate", api: "apps.kro.run/v1", kind: "WebApp", ns: "prod", uid: "parent", ownerUID: "parent", duplicate: true, reason: "ambiguous"},
		{name: "legacy no UID", api: "apps.kro.run/v1", kind: "WebApp", ns: "prod", uid: "parent", present: true},
		{name: "foreign without UID", api: "apps.kro.run/v1", kind: "WebApp", ns: "dev", uid: "parent", reason: "unresolved"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			child := kroScopeObject("apps/v1", "Deployment", "child", "prod", "child")
			child.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "apps.kro.run/v1", Kind: "WebApp", Name: "parent", UID: types.UID(tc.ownerUID)}})
			parent := kroScopeObject(tc.api, tc.kind, "parent", tc.ns, tc.uid)
			objects := []*unstructured.Unstructured{child, parent}
			if tc.duplicate {
				objects = append(objects, parent.DeepCopy())
			}
			for _, reverse := range []bool{false, true} {
				if reverse {
					slices.Reverse(objects)
				}
				lineage, ok := ResolveKroLineage(child, objects)
				if !ok || lineage == nil || lineage.Instance.Present != tc.present {
					t.Fatalf("incorrect present state: %+v", lineage)
				}
				if lineage.Instance.Ref.Group != "apps.kro.run" || lineage.Instance.Ref.Version != "v1" || lineage.Instance.Ref.Kind != "WebApp" {
					t.Fatalf("changed owner type: %+v", lineage.Instance)
				}
				if tc.present {
					if lineage.Instance.Ref.Namespace != tc.ns {
						t.Fatal("changed parent namespace")
					}
				} else {
					if lineage.Instance.Ref.Namespace != "" {
						t.Fatal("guessed unresolved parent namespace")
					}
					if !slices.Contains(lineage.Evidence, "instance:"+tc.reason) {
						t.Fatalf("missing reason %s: %v", tc.reason, lineage.Evidence)
					}
				}
			}
		})
	}
}

func TestKroDefinitionOwnerReferenceScopeAndUID(t *testing.T) {
	for _, tc := range []struct {
		name, api, kind, ns, uid string
		present, duplicate       bool
	}{
		{name: "cluster definition", api: "kro.run/v1", kind: "ResourceGraphDefinition", uid: "definition", present: true},
		{name: "namespaced definition", api: "kro.run/v1", kind: "ResourceGraphDefinition", ns: "prod", uid: "definition"},
		{name: "wrong kind", api: "kro.run/v1", kind: "Other", uid: "definition"},
		{name: "wrong version", api: "kro.run/v2", kind: "ResourceGraphDefinition", uid: "definition"},
		{name: "stale UID", api: "kro.run/v1", kind: "ResourceGraphDefinition", uid: "replaced"},
		{name: "duplicate", api: "kro.run/v1", kind: "ResourceGraphDefinition", uid: "definition", duplicate: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			instance := kroScopeObject("apps.kro.run/v1", "WebApp", "parent", "prod", "instance")
			instance.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "kro.run/v1", Kind: "ResourceGraphDefinition", Name: "definition", UID: "definition"}})
			definition := kroScopeObject(tc.api, tc.kind, "definition", tc.ns, tc.uid)
			child := kroScopeObject("apps/v1", "Deployment", "child", "prod", "child")
			child.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "apps.kro.run/v1", Kind: "WebApp", Name: "parent", UID: "instance"}})
			objects := []*unstructured.Unstructured{child, instance, definition}
			if tc.duplicate {
				objects = append(objects, definition.DeepCopy())
			}
			for _, reverse := range []bool{false, true} {
				if reverse {
					slices.Reverse(objects)
				}
				lineage, ok := ResolveKroLineage(child, objects)
				if !ok || lineage.Definition == nil || lineage.Definition.Present != tc.present {
					t.Fatalf("incorrect definition: %+v", lineage)
				}
				if !tc.present && !slices.Contains(lineage.Evidence, "definition:owner_uid_not_observed") && !slices.Contains(lineage.Evidence, "definition:ambiguous") {
					t.Fatalf("missing definition omission: %v", lineage.Evidence)
				}
			}
		})
	}
}

func TestKroOwnerReferenceAmbiguousScopeAndSelf(t *testing.T) {
	child := kroScopeObject("apps/v1", "Deployment", "child", "prod", "child")
	child.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "apps.kro.run/v1", Kind: "WebApp", Name: "parent"}})
	local := kroScopeObject("apps.kro.run/v1", "WebApp", "parent", "prod", "local")
	cluster := kroScopeObject("apps.kro.run/v1", "WebApp", "parent", "", "cluster")
	objects := []*unstructured.Unstructured{child, local, cluster}
	for _, reverse := range []bool{false, true} {
		if reverse {
			slices.Reverse(objects)
		}
		lineage, ok := ResolveKroLineage(child, objects)
		if !ok || lineage.Instance.Present || !slices.Contains(lineage.Evidence, "instance:ambiguous") {
			t.Fatalf("guessed parent scope: %+v", lineage)
		}
	}
	owners := child.GetOwnerReferences()
	owners[0].UID = "local"
	child.SetOwnerReferences(owners)
	lineage, ok := ResolveKroLineage(child, objects)
	if !ok || !lineage.Instance.Present || lineage.Instance.Ref.Namespace != "prod" {
		t.Fatalf("UID did not distinguish candidates: %+v", lineage)
	}
	self := kroScopeObject("apps.kro.run/v1", "WebApp", "self", "prod", "self-uid")
	self.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "apps.kro.run/v1", Kind: "WebApp", Name: "self", UID: "self-uid"}})
	lineage, ok = ResolveKroLineage(self, []*unstructured.Unstructured{self, self.DeepCopy()})
	if !ok || lineage.Instance.Present || !slices.Contains(lineage.Evidence, "instance:owner_uid_not_observed") {
		t.Fatalf("asserted self owner: %+v", lineage)
	}
	lineage, ok = ResolveKroLineageWithIndex(child, nil)
	if !ok || lineage.Instance.Present || lineage.Instance.Ref.Namespace != "" {
		t.Fatalf("missing inventory asserted owner: %+v", lineage)
	}
}
