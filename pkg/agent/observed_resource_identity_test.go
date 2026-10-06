// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func observedClusterFixture(uid string) ClusterIdentityEvidence {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	return ClusterIdentityEvidence{Context: "same-label", ID: uid, Identity: "verified", IDSource: "v1/Namespace/kube-system", ObservedAt: &now}
}

func observedObjectFixture() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]interface{}{"name": "api", "namespace": "team-a", "uid": "object-instance"},
	}}
}

func TestObservedResourceIdentitySeparatesCollidingNamesAndInstances(t *testing.T) {
	first, err := NewObservedResourceIdentity(observedClusterFixture("cluster-a"), observedObjectFixture(), ObservedResourceNamespaced)
	if err != nil {
		t.Fatal(err)
	}
	key, err := first.MergeKey()
	if err != nil {
		t.Fatal(err)
	}
	secondCluster, err := NewObservedResourceIdentity(observedClusterFixture("cluster-b"), observedObjectFixture(), ObservedResourceNamespaced)
	if err != nil {
		t.Fatal(err)
	}
	secondKey, err := secondCluster.MergeKey()
	if err != nil || secondKey == key {
		t.Fatalf("same-label cluster collision: %s %s %v", key, secondKey, err)
	}
	for _, change := range []func(*ObservedResourceIdentity){
		func(r *ObservedResourceIdentity) { r.ClusterID = "cluster-b" },
		func(r *ObservedResourceIdentity) { r.UID = "recreated-instance" },
		func(r *ObservedResourceIdentity) { r.Namespace = "team-b" },
		func(r *ObservedResourceIdentity) { r.Name = "other" },
		func(r *ObservedResourceIdentity) { r.Kind = "StatefulSet" },
		func(r *ObservedResourceIdentity) { r.Group = "other.example"; r.APIVersion = "other.example/v1" },
	} {
		second := first
		change(&second)
		next, err := second.MergeKey()
		if err != nil || next == key {
			t.Fatalf("collision %s / %s: %v", key, next, err)
		}
	}
	otherVersion := first
	otherVersion.APIVersion = "apps/v1beta1"
	next, err := otherVersion.MergeKey()
	if err != nil || next != key {
		t.Fatalf("served version changed instance: %s %v", next, err)
	}
	var fields [7]string
	if json.Unmarshal([]byte(key), &fields) != nil || fields != [7]string{"v1/Namespace/kube-system", "cluster-a", "apps", "Deployment", "team-a", "api", "object-instance"} {
		t.Fatalf("ambiguous key %s", key)
	}
}

func TestObservedResourceIdentityKeyEscapesOpaqueUIDDelimiters(t *testing.T) {
	ref, err := NewObservedResourceIdentity(observedClusterFixture(`cluster|"a",`), observedObjectFixture(), ObservedResourceNamespaced)
	if err != nil {
		t.Fatal(err)
	}
	ref.UID = `instance|"b",`
	key, err := ref.MergeKey()
	if err != nil {
		t.Fatal(err)
	}
	var values []string
	if json.Unmarshal([]byte(key), &values) != nil || len(values) != 7 || values[1] != ref.ClusterID || values[6] != ref.UID {
		t.Fatalf("key lost field boundaries %s", key)
	}
	other := ref
	other.ClusterID, other.UID = ref.UID, ref.ClusterID
	next, err := other.MergeKey()
	if err != nil || key == next {
		t.Fatalf("delimited UID collision %s %s %v", key, next, err)
	}
}

func TestObservedResourceIdentityDoesNotInferOrMutateMetadata(t *testing.T) {
	obj := observedObjectFixture()
	before := obj.DeepCopy()
	ref, err := NewObservedResourceIdentity(observedClusterFixture("cluster-a"), obj, ObservedResourceNamespaced)
	if err != nil || ref.APIVersion != "apps/v1" || ref.Group != "apps" {
		t.Fatalf("%+v %v", ref, err)
	}
	a, _ := json.Marshal(obj)
	b, _ := json.Marshal(before)
	if string(a) != string(b) {
		t.Fatal("constructor mutated object")
	}
	for _, field := range []string{"apiVersion", "kind"} {
		copy := obj.DeepCopy()
		delete(copy.Object, field)
		copy.Object["metadata"].(map[string]interface{})["labels"] = map[string]interface{}{"app.kubernetes.io/managed-by": "Helm"}
		if _, err := NewObservedResourceIdentity(observedClusterFixture("cluster-a"), copy, ObservedResourceNamespaced); err == nil {
			t.Fatalf("inferred %s", field)
		}
	}
	clusterObject := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]interface{}{"name": "team-a", "uid": "namespace-instance"}}}
	ref, err = NewObservedResourceIdentity(observedClusterFixture("cluster-a"), clusterObject, ObservedResourceCluster)
	if err != nil || ref.Group != "" || ref.Namespace != "" {
		t.Fatalf("core cluster identity %+v %v", ref, err)
	}
}

func TestObservedResourceIdentityRefusesUnknownOrMalformedCluster(t *testing.T) {
	for _, change := range []func(*ClusterIdentityEvidence){
		func(c *ClusterIdentityEvidence) {
			c.Identity = "unverified"
			c.Context = "cluster-a"
			c.APIServer = "https://same-name.example"
		},
		func(c *ClusterIdentityEvidence) { c.ID = "" },
		func(c *ClusterIdentityEvidence) { c.ID = strings.Repeat("x", 513) },
		func(c *ClusterIdentityEvidence) { c.IDSource = "context-label" },
		func(c *ClusterIdentityEvidence) { c.ObservedAt = nil },
		func(c *ClusterIdentityEvidence) { zero := time.Time{}; c.ObservedAt = &zero },
		func(c *ClusterIdentityEvidence) { c.Omission = "forbidden" },
	} {
		cluster := observedClusterFixture("cluster-a")
		change(&cluster)
		if ref, err := NewObservedResourceIdentity(cluster, observedObjectFixture(), ObservedResourceNamespaced); err == nil || ref != (ObservedResourceIdentity{}) {
			t.Fatalf("accepted %+v", cluster)
		}
	}
}

func TestObservedResourceIdentityRefusesMalformedObjectScopeAndKeys(t *testing.T) {
	for _, path := range [][]string{{"apiVersion"}, {"kind"}, {"metadata", "name"}, {"metadata", "uid"}, {"metadata", "namespace"}} {
		for _, value := range []interface{}{nil, "", false, strings.Repeat("x", 513), string([]byte{0xff})} {
			obj := observedObjectFixture()
			if err := unstructured.SetNestedField(obj.Object, value, path...); err != nil {
				t.Fatal(err)
			}
			if _, err := NewObservedResourceIdentity(observedClusterFixture("cluster-a"), obj, ObservedResourceNamespaced); err == nil {
				t.Fatalf("accepted %v=%v", path, value)
			}
		}
	}
	for _, scope := range []ObservedResourceScope{ObservedResourceCluster, "", "unknown"} {
		if _, err := NewObservedResourceIdentity(observedClusterFixture("cluster-a"), observedObjectFixture(), scope); err == nil {
			t.Fatalf("accepted scope %s", scope)
		}
	}
	for _, namespace := range []interface{}{false, nil} {
		obj := observedObjectFixture()
		obj.Object["metadata"].(map[string]interface{})["namespace"] = namespace
		if _, err := NewObservedResourceIdentity(observedClusterFixture("cluster-a"), obj, ObservedResourceCluster); err == nil {
			t.Fatalf("accepted malformed cluster namespace %v", namespace)
		}
	}
	if _, err := NewObservedResourceIdentity(observedClusterFixture("cluster-a"), nil, ObservedResourceNamespaced); err == nil {
		t.Fatal("accepted nil object")
	}
	ref, err := NewObservedResourceIdentity(observedClusterFixture("cluster-a"), observedObjectFixture(), ObservedResourceNamespaced)
	if err != nil {
		t.Fatal(err)
	}
	ref.APIVersion = "other.example/v1"
	if key, err := ref.MergeKey(); err == nil || key != "" {
		t.Fatal("key accepted mismatched group/version")
	}
	if key, err := (ObservedResourceIdentity{}).MergeKey(); err == nil || key != "" {
		t.Fatal("key accepted zero identity")
	}
}
