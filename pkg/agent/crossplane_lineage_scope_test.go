// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"reflect"
	"slices"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

func lineageScopeObject(api, kind, ns, name, uid string) *unstructured.Unstructured {
	o := &unstructured.Unstructured{Object: map[string]interface{}{}}
	o.SetAPIVersion(api)
	o.SetKind(kind)
	o.SetNamespace(ns)
	o.SetName(name)
	o.SetUID(types.UID(uid))
	return o
}
func lineageScopeChild() *unstructured.Unstructured {
	child := lineageScopeObject("v1", "ConfigMap", "team-a", "child", "child-uid")
	child.SetLabels(map[string]string{"crossplane.io/composite": "parent"})
	return child
}
func lineageScopeBothOrders(t *testing.T, child *unstructured.Unstructured, objects []*unstructured.Unstructured) *CrossplaneLineage {
	t.Helper()
	forward, ok := ResolveCrossplaneLineage(child, objects)
	if !ok {
		t.Fatal("lineage absent")
	}
	reversed := append([]*unstructured.Unstructured{}, objects...)
	slices.Reverse(reversed)
	backward, ok := ResolveCrossplaneLineage(child, reversed)
	if !ok || !reflect.DeepEqual(forward, backward) {
		t.Fatalf("order-dependent lineage: %+v vs %+v", forward, backward)
	}
	return forward
}
func TestCrossplaneLineageLabelRespectsNamespace(t *testing.T) {
	child := lineageScopeChild()
	own := lineageScopeObject("platform.example.org/v1", "XApp", "team-a", "parent", "a")
	foreign := lineageScopeObject("platform.example.org/v1", "XApp", "team-b", "parent", "b")
	got := lineageScopeBothOrders(t, child, []*unstructured.Unstructured{foreign, own})
	if !got.Composite.Present || got.Composite.Ref.Namespace != "team-a" {
		t.Fatalf("wrong namespace: %+v", got)
	}
	got = lineageScopeBothOrders(t, child, []*unstructured.Unstructured{foreign})
	if got.Composite.Present || got.Composite.Ref.Group != "" || got.Composite.Ref.Namespace != "" || !slices.Contains(got.Evidence, "xr:unresolved") {
		t.Fatalf("joined foreign candidate: %+v", got)
	}
}
func TestCrossplaneLineageLabelAmbiguityStaysUnresolved(t *testing.T) {
	child := lineageScopeChild()
	own := lineageScopeObject("platform.example.org/v1", "XApp", "team-a", "parent", "a")
	for _, other := range []*unstructured.Unstructured{lineageScopeObject("different.example.org/v1", "XOther", "team-a", "parent", "b"), lineageScopeObject("platform.example.org/v1", "XApp", "", "parent", "b"), own.DeepCopy()} {
		got := lineageScopeBothOrders(t, child, []*unstructured.Unstructured{own, other})
		if got.Composite.Present || got.Composite.Ref.Kind != "CompositeResource" || got.Composite.Ref.Group != "" || !slices.Contains(got.Evidence, "xr:ambiguous") {
			t.Fatalf("ambiguous parent asserted: %+v", got)
		}
	}
}
func TestCrossplaneLineageOwnerRefScopeAndUID(t *testing.T) {
	child := lineageScopeChild()
	child.SetLabels(nil)
	child.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "example.crossplane.io/v1", Kind: "XApp", Name: "parent", UID: "a"}})
	own := lineageScopeObject("example.crossplane.io/v1", "XApp", "team-a", "parent", "a")
	old := lineageScopeObject("example.crossplane.io/v1", "XApp", "", "parent", "old")
	got := lineageScopeBothOrders(t, child, []*unstructured.Unstructured{old, own})
	if !got.Composite.Present || got.Composite.Ref.Namespace != "team-a" || got.Composite.Ref.Version != "v1" {
		t.Fatalf("namespaced owner unresolved: %+v", got)
	}
	got = lineageScopeBothOrders(t, child, []*unstructured.Unstructured{old})
	if got.Composite.Present || !slices.Contains(got.Evidence, "xr:owner_uid_not_observed") {
		t.Fatalf("replaced UID asserted: %+v", got)
	}
	got = lineageScopeBothOrders(t, child, []*unstructured.Unstructured{own, own.DeepCopy()})
	if got.Composite.Present || !slices.Contains(got.Evidence, "xr:ambiguous") {
		t.Fatalf("duplicate owner asserted: %+v", got)
	}
	own.SetNamespace("")
	got = lineageScopeBothOrders(t, child, []*unstructured.Unstructured{own})
	if !got.Composite.Present || got.Composite.Ref.Namespace != "" {
		t.Fatal("legacy cluster parent lost")
	}
	withoutUID := child.DeepCopy()
	withoutUID.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "example.crossplane.io/v1", Kind: "XApp", Name: "missing"}})
	missing := lineageScopeBothOrders(t, withoutUID, []*unstructured.Unstructured{own})
	if missing.Composite.Present || !slices.Contains(missing.Evidence, "xr:unresolved") {
		t.Fatalf("missing unversioned-instance owner marker: %+v", missing)
	}
	own.SetNamespace("team-b")
	got = lineageScopeBothOrders(t, child, []*unstructured.Unstructured{own})
	if got.Composite.Present {
		t.Fatal("foreign owner asserted")
	}
}
func TestCrossplaneLineageClaimAmbiguityStaysUnresolved(t *testing.T) {
	child := lineageScopeChild()
	child.SetLabels(map[string]string{"crossplane.io/composite": "parent", "crossplane.io/claim-name": "claim", "crossplane.io/claim-namespace": "team-a"})
	parent := lineageScopeObject("platform.example.org/v1", "XApp", "team-a", "parent", "a")
	claim := lineageScopeObject("platform.example.org/v1", "App", "team-a", "claim", "c")
	other := lineageScopeObject("other.example.org/v1", "Other", "team-a", "claim", "d")
	got := lineageScopeBothOrders(t, child, []*unstructured.Unstructured{parent, claim, other})
	if !got.Composite.Present || got.Claim == nil || got.Claim.Present || got.Claim.Ref.Kind != "Claim" || !slices.Contains(got.Evidence, "claim:ambiguous") {
		t.Fatalf("ambiguous claim asserted: %+v", got)
	}
}

func TestCrossplaneLineageWholeInventoryRefusesSelfParents(t *testing.T) {
	child := lineageScopeChild()
	child.SetLabels(map[string]string{"crossplane.io/composite": "child", "crossplane.io/claim-name": "child", "crossplane.io/claim-namespace": "team-a"})
	got := lineageScopeBothOrders(t, child, []*unstructured.Unstructured{child, child.DeepCopy()})
	if got.Composite.Present || got.Claim == nil || got.Claim.Present || !slices.Contains(got.Evidence, "xr:unresolved") {
		t.Fatalf("self label join: %+v", got)
	}
	child.SetLabels(nil)
	child.SetAPIVersion("example.crossplane.io/v1")
	child.SetKind("XApp")
	child.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: child.GetAPIVersion(), Kind: child.GetKind(), Name: child.GetName(), UID: child.GetUID()}})
	got = lineageScopeBothOrders(t, child, []*unstructured.Unstructured{child})
	if got.Composite.Present || !slices.Contains(got.Evidence, "xr:owner_uid_not_observed") {
		t.Fatalf("self owner join: %+v", got)
	}
}
