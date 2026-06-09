// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var workloadsObservedAt = time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)

func desiredDeployment(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": "prod",
		},
		"spec": map[string]interface{}{"replicas": int64(1)},
	}}
}

// liveDeployment builds a Deployment whose status drives kstatus: ready>0
// yields Current (converged), ready==0 yields InProgress (availableReplicas
// below updatedReplicas).
func liveDeployment(name string, ready int64) *unstructured.Unstructured {
	avail := "False"
	if ready > 0 {
		avail = "True"
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":       name,
			"namespace":  "prod",
			"generation": int64(1),
		},
		"spec": map[string]interface{}{"replicas": int64(1)},
		"status": map[string]interface{}{
			"observedGeneration": int64(1),
			"replicas":           int64(1),
			"updatedReplicas":    int64(1),
			"readyReplicas":      ready,
			"availableReplicas":  ready,
			"conditions": []interface{}{
				map[string]interface{}{"type": "Available", "status": avail},
				map[string]interface{}{"type": "Progressing", "status": "True", "reason": "NewReplicaSetAvailable"},
			},
		},
	}}
}

func waitingPod(name, reason string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]interface{}{"name": name, "namespace": "prod"},
		"status": map[string]interface{}{
			"containerStatuses": []interface{}{
				map[string]interface{}{
					"name": "web",
					"state": map[string]interface{}{
						"waiting": map[string]interface{}{"reason": reason, "message": `secret "app-db-secret" not found`},
					},
				},
			},
		},
	}}
}

func buildWorkloadsReceipt(t *testing.T, grace time.Duration, observed []WorkloadConvergedObservedObject) Statement {
	t.Helper()
	evidence, err := BuildWorkloadsConvergedEvidence(
		ObjectSetSource{Type: "file", Ref: "rendered.yaml"},
		ObjectSetScope{Kind: "namespace", Namespace: "prod"},
		grace, observed, workloadsObservedAt,
	)
	if err != nil {
		t.Fatalf("BuildWorkloadsConvergedEvidence: %v", err)
	}
	stmt, err := BuildWorkloadsConvergedReceipt(BuildWorkloadsConvergedReceiptInput{
		Evidence:   evidence,
		Verifier:   Verifier{Tool: "cub-scout", Version: "test"},
		VerifiedAt: workloadsObservedAt,
	})
	if err != nil {
		t.Fatalf("BuildWorkloadsConvergedReceipt: %v", err)
	}
	if stmt.Predicate.PredicateName != string(PredicateWorkloadsConverged) {
		t.Fatalf("predicate = %s", stmt.Predicate.PredicateName)
	}
	if err := VerifyStatementFingerprint(stmt); err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	return stmt
}

func TestWorkloadsConverged_PASSWhenReady(t *testing.T) {
	stmt := buildWorkloadsReceipt(t, 0, []WorkloadConvergedObservedObject{
		{Desired: desiredDeployment("web"), Live: liveDeployment("web", 1)},
	})
	if stmt.Predicate.Verdict != VerdictPASS {
		t.Fatalf("verdict = %s, want PASS", stmt.Predicate.Verdict)
	}
	if stmt.Predicate.Evidence.Workloads.Summary.Converged != 1 {
		t.Fatalf("converged = %d, want 1", stmt.Predicate.Evidence.Workloads.Summary.Converged)
	}
}

func TestWorkloadsConverged_WATCHWhileProgressingInGrace(t *testing.T) {
	stmt := buildWorkloadsReceipt(t, 10*time.Minute, []WorkloadConvergedObservedObject{
		{Desired: desiredDeployment("web"), Live: liveDeployment("web", 0)},
	})
	if stmt.Predicate.Verdict != VerdictWATCH {
		t.Fatalf("verdict = %s, want WATCH (kstatus=%s)", stmt.Predicate.Verdict, stmt.Predicate.Evidence.Workloads.Workloads[0].KstatusStatus)
	}
}

func TestWorkloadsConverged_BLOCKWhenStuckPastGrace(t *testing.T) {
	live := liveDeployment("web", 0)
	_ = unstructured.SetNestedField(live.Object, "2020-01-01T00:00:00Z", "metadata", "creationTimestamp")
	stmt := buildWorkloadsReceipt(t, 1*time.Second, []WorkloadConvergedObservedObject{
		{Desired: desiredDeployment("web"), Live: live},
	})
	if stmt.Predicate.Verdict != VerdictBLOCK {
		t.Fatalf("verdict = %s, want BLOCK", stmt.Predicate.Verdict)
	}
}

// The helm-expt F3 reproduction: the Deployment is present but its pod is
// wedged in CreateContainerConfigError because a required Secret is absent.
// object-set-matches would PASS; workloads-converged must BLOCK.
func TestWorkloadsConverged_BLOCKOnCreateContainerConfigError(t *testing.T) {
	stmt := buildWorkloadsReceipt(t, 10*time.Minute, []WorkloadConvergedObservedObject{
		{
			Desired: desiredDeployment("web"),
			Live:    liveDeployment("web", 0),
			Pods:    []*unstructured.Unstructured{waitingPod("web-abc", "CreateContainerConfigError")},
		},
	})
	if stmt.Predicate.Verdict != VerdictBLOCK {
		t.Fatalf("verdict = %s, want BLOCK", stmt.Predicate.Verdict)
	}
	w := stmt.Predicate.Evidence.Workloads.Workloads[0]
	if w.Status != WorkloadConvergedFailed {
		t.Fatalf("workload status = %s, want failed", w.Status)
	}
	if len(w.PodReasons) == 0 || w.PodReasons[0].Reason != "CreateContainerConfigError" {
		t.Fatalf("expected CreateContainerConfigError pod reason, got %+v", w.PodReasons)
	}
}

func TestWorkloadsConverged_BLOCKWhenMissing(t *testing.T) {
	stmt := buildWorkloadsReceipt(t, 0, []WorkloadConvergedObservedObject{
		{Desired: desiredDeployment("web"), Live: nil},
	})
	if stmt.Predicate.Verdict != VerdictBLOCK {
		t.Fatalf("verdict = %s, want BLOCK", stmt.Predicate.Verdict)
	}
	if stmt.Predicate.Evidence.Workloads.Summary.Missing != 1 {
		t.Fatalf("missing = %d, want 1", stmt.Predicate.Evidence.Workloads.Summary.Missing)
	}
}

func TestWorkloadsConverged_INCONCLUSIVEOnLookupGap(t *testing.T) {
	stmt := buildWorkloadsReceipt(t, 0, []WorkloadConvergedObservedObject{
		{Desired: desiredDeployment("web"), Error: "no matches for kind FutureThing", Inconclusive: true},
	})
	if stmt.Predicate.Verdict != VerdictINCONCLUSIVE {
		t.Fatalf("verdict = %s, want INCONCLUSIVE", stmt.Predicate.Verdict)
	}
	found := false
	for _, o := range stmt.Predicate.Omissions {
		if o.Missing == OmissionWorkloadConvergenceCoverage {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected %s omission", OmissionWorkloadConvergenceCoverage)
	}
}
