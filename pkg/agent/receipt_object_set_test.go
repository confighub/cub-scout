// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func objectSetDeployment(replicas int64, image string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":      "api",
			"namespace": "prod",
			"labels": map[string]interface{}{
				"app": "api",
			},
		},
		"spec": map[string]interface{}{
			"replicas": replicas,
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{"name": "api", "image": image},
					},
				},
			},
		},
	}}
}

func TestObjectSetReceipt_PASSIgnoresServerDefaults(t *testing.T) {
	desired := objectSetDeployment(3, "example/api:v1")
	live := objectSetDeployment(3, "example/api:v1")
	_ = unstructured.SetNestedField(live.Object, "10.0.0.10", "spec", "clusterIP")
	_ = unstructured.SetNestedField(live.Object, "Always", "spec", "template", "spec", "containers", "0", "imagePullPolicy")
	_ = unstructured.SetNestedField(live.Object, "123", "metadata", "resourceVersion")

	evidence, err := BuildObjectSetEvidence(
		ObjectSetSource{Type: "file", Ref: "rendered.yaml"},
		ObjectSetScope{Kind: "namespace", Namespace: "prod"},
		[]ObjectSetObservedObject{{Desired: desired, Live: live}},
	)
	if err != nil {
		t.Fatalf("BuildObjectSetEvidence: %v", err)
	}
	stmt, err := BuildObjectSetReceipt(BuildObjectSetReceiptInput{
		Evidence:   evidence,
		Verifier:   Verifier{Tool: "cub-scout", Version: "test"},
		VerifiedAt: time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildObjectSetReceipt: %v", err)
	}
	if stmt.Predicate.Verdict != VerdictPASS {
		t.Fatalf("verdict = %s, want PASS", stmt.Predicate.Verdict)
	}
	if stmt.Predicate.PredicateName != string(PredicateObjectSetMatches) {
		t.Fatalf("predicate = %s", stmt.Predicate.PredicateName)
	}
	if stmt.Predicate.Evidence.ObjectSet.Summary.Matched != 1 {
		t.Fatalf("matched = %d, want 1", stmt.Predicate.Evidence.ObjectSet.Summary.Matched)
	}
	if err := VerifyStatementFingerprint(stmt); err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
}

func TestObjectSetReceipt_BLOCKOnAuthoredFieldMismatch(t *testing.T) {
	desired := objectSetDeployment(3, "example/api:v1")
	live := objectSetDeployment(3, "example/api:v2")

	evidence, err := BuildObjectSetEvidence(
		ObjectSetSource{Type: "file", Ref: "rendered.yaml"},
		ObjectSetScope{Kind: "namespace", Namespace: "prod"},
		[]ObjectSetObservedObject{{Desired: desired, Live: live}},
	)
	if err != nil {
		t.Fatalf("BuildObjectSetEvidence: %v", err)
	}
	stmt, err := BuildObjectSetReceipt(BuildObjectSetReceiptInput{
		Evidence: evidence,
		Verifier: Verifier{Tool: "cub-scout", Version: "test"},
	})
	if err != nil {
		t.Fatalf("BuildObjectSetReceipt: %v", err)
	}
	if stmt.Predicate.Verdict != VerdictBLOCK {
		t.Fatalf("verdict = %s, want BLOCK", stmt.Predicate.Verdict)
	}
	obj := stmt.Predicate.Evidence.ObjectSet.Objects[0]
	if obj.Status != ObjectSetObjectMismatched {
		t.Fatalf("object status = %s, want mismatched", obj.Status)
	}
	if len(obj.Differences) == 0 {
		t.Fatal("expected at least one diff")
	}
}

func TestObjectSetReceipt_INCONCLUSIVEOnLookupGap(t *testing.T) {
	desired := objectSetDeployment(3, "example/api:v1")
	evidence, err := BuildObjectSetEvidence(
		ObjectSetSource{Type: "file", Ref: "rendered.yaml"},
		ObjectSetScope{Kind: "namespace", Namespace: "prod"},
		[]ObjectSetObservedObject{{
			Desired:      desired,
			Error:        "no matches for kind FutureThing",
			Inconclusive: true,
		}},
	)
	if err != nil {
		t.Fatalf("BuildObjectSetEvidence: %v", err)
	}
	stmt, err := BuildObjectSetReceipt(BuildObjectSetReceiptInput{
		Evidence: evidence,
		Verifier: Verifier{Tool: "cub-scout", Version: "test"},
	})
	if err != nil {
		t.Fatalf("BuildObjectSetReceipt: %v", err)
	}
	if stmt.Predicate.Verdict != VerdictINCONCLUSIVE {
		t.Fatalf("verdict = %s, want INCONCLUSIVE", stmt.Predicate.Verdict)
	}
	found := false
	for _, omission := range stmt.Predicate.Omissions {
		if omission.Missing == OmissionObjectSetCoverage {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected %s omission", OmissionObjectSetCoverage)
	}
}
