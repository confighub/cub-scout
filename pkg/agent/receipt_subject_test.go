// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func makeLiveDeployment(modifiers ...func(*unstructured.Unstructured)) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":              "api",
				"namespace":         "prod",
				"uid":               "11111111-1111-1111-1111-111111111111",
				"resourceVersion":   "12345",
				"generation":        int64(7),
				"creationTimestamp": "2026-05-01T00:00:00Z",
				"managedFields": []interface{}{
					map[string]interface{}{
						"manager":   "argocd-controller",
						"operation": "Apply",
					},
				},
				"labels": map[string]interface{}{
					"app": "api",
				},
			},
			"spec": map[string]interface{}{
				"replicas": int64(3),
			},
			"status": map[string]interface{}{
				"observedGeneration": int64(6),
				"replicas":           int64(3),
				"availableReplicas":  int64(3),
			},
		},
	}
	for _, m := range modifiers {
		m(obj)
	}
	obj.SetGroupVersionKind(obj.GroupVersionKind()) // exercise GVK round-trip
	return obj
}

func TestBuildK8sLiveSubject_BasicShape(t *testing.T) {
	obj := makeLiveDeployment()
	sub, err := BuildK8sLiveSubject(obj)
	if err != nil {
		t.Fatalf("BuildK8sLiveSubject: %v", err)
	}
	if want := "k8s-live://apps/v1/Deployment/prod/api"; sub.Name != want {
		t.Errorf("subject name: got %q, want %q", sub.Name, want)
	}
	if got, ok := sub.Digest["sha256"]; !ok || len(got) != 64 {
		t.Errorf("subject digest expected sha256 hex (64 chars); got map=%v", sub.Digest)
	}
}

func TestBuildK8sLiveSubject_PrunesDynamicFields(t *testing.T) {
	// Two objects that differ ONLY in dynamic fields (status, managedFields,
	// resourceVersion, generation, uid, creationTimestamp) must produce the
	// same digest. This is what makes the receipt stable across reconcile
	// heartbeats.
	a := makeLiveDeployment()
	b := makeLiveDeployment(func(o *unstructured.Unstructured) {
		md := o.Object["metadata"].(map[string]interface{})
		md["resourceVersion"] = "99999"
		md["generation"] = int64(42)
		md["uid"] = "22222222-2222-2222-2222-222222222222"
		md["creationTimestamp"] = metav1.Now().UTC().Format("2006-01-02T15:04:05Z")
		md["managedFields"] = []interface{}{
			map[string]interface{}{"manager": "different-string"},
		}
		o.Object["status"] = map[string]interface{}{"observedGeneration": int64(999)}
	})

	subA, _ := BuildK8sLiveSubject(a)
	subB, _ := BuildK8sLiveSubject(b)
	if subA.Digest["sha256"] != subB.Digest["sha256"] {
		t.Errorf("digests should be equal after pruning dynamic fields; got A=%s B=%s",
			subA.Digest["sha256"], subB.Digest["sha256"])
	}
}

func TestBuildK8sLiveSubject_DigestChangesOnSpec(t *testing.T) {
	// Changing a non-dynamic field (spec.replicas) must change the digest.
	a := makeLiveDeployment()
	b := makeLiveDeployment(func(o *unstructured.Unstructured) {
		o.Object["spec"] = map[string]interface{}{"replicas": int64(5)}
	})

	subA, _ := BuildK8sLiveSubject(a)
	subB, _ := BuildK8sLiveSubject(b)
	if subA.Digest["sha256"] == subB.Digest["sha256"] {
		t.Errorf("digests should differ when spec changes; got same %s", subA.Digest["sha256"])
	}
}

func TestBuildK8sLiveSubject_NilObject(t *testing.T) {
	_, err := BuildK8sLiveSubject(nil)
	if err == nil {
		t.Error("BuildK8sLiveSubject(nil) must error")
	}
}

func TestBuildK8sLiveSubject_DoesNotMutateCaller(t *testing.T) {
	// Ensure pruning happens on a clone, not the caller's object.
	obj := makeLiveDeployment()
	_, err := BuildK8sLiveSubject(obj)
	if err != nil {
		t.Fatalf("BuildK8sLiveSubject: %v", err)
	}
	md := obj.Object["metadata"].(map[string]interface{})
	if _, ok := md["managedFields"]; !ok {
		t.Error("BuildK8sLiveSubject must not mutate caller's managedFields")
	}
	if _, ok := obj.Object["status"]; !ok {
		t.Error("BuildK8sLiveSubject must not mutate caller's status")
	}
}

func TestBuildK8sLiveSubject_ClusterScopedResource(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRole",
			"metadata": map[string]interface{}{
				"name": "viewer",
			},
		},
	}
	sub, err := BuildK8sLiveSubject(obj)
	if err != nil {
		t.Fatalf("BuildK8sLiveSubject: %v", err)
	}
	// Cluster-scoped subject name omits the namespace segment.
	if want := "k8s-live://rbac.authorization.k8s.io/v1/ClusterRole/viewer"; sub.Name != want {
		t.Errorf("cluster-scoped name: got %q, want %q", sub.Name, want)
	}
}

func TestBuildConfigHubUnitSubject_BasicShape(t *testing.T) {
	body := []byte(`{"kind":"Unit","spec":{"data":"..."}}`)
	sub, err := BuildConfigHubUnitSubject("payments-api", 42, body)
	if err != nil {
		t.Fatalf("BuildConfigHubUnitSubject: %v", err)
	}
	if want := "confighub-unit://payments-api@rev=42"; sub.Name != want {
		t.Errorf("subject name: got %q, want %q", sub.Name, want)
	}
	if got, ok := sub.Digest["sha256"]; !ok || len(got) != 64 {
		t.Errorf("expected sha256 hex; got %v", sub.Digest)
	}
}

func TestBuildConfigHubUnitSubject_RejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name    string
		slug    string
		rev     int
		body    []byte
		wantErr string
	}{
		{"empty slug", "", 1, []byte("x"), "empty unit slug"},
		{"zero revision", "payments-api", 0, []byte("x"), "non-positive revision"},
		{"negative revision", "payments-api", -1, []byte("x"), "non-positive revision"},
		{"empty body", "payments-api", 1, nil, "empty canonical body"},
		{"empty body slice", "payments-api", 1, []byte{}, "empty canonical body"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildConfigHubUnitSubject(tc.slug, tc.rev, tc.body)
			if err == nil {
				t.Errorf("expected error; got nil")
				return
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q must mention %q", err.Error(), tc.wantErr)
			}
		})
	}
}
