// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func setCreationTimestamp(u *unstructured.Unstructured, ts time.Time) {
	u.SetCreationTimestamp(metav1.NewTime(ts))
}

func setProgressConditionTime(u *unstructured.Unstructured, field string, ts time.Time) {
	conditions, found, err := unstructured.NestedSlice(u.Object, "status", "conditions")
	if err != nil || !found {
		return
	}
	for i, raw := range conditions {
		cond, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if typ, _ := cond["type"].(string); typ == "Progressing" {
			cond[field] = ts.UTC().Format(time.RFC3339Nano)
			conditions[i] = cond
			_ = unstructured.SetNestedSlice(u.Object, conditions, "status", "conditions")
			return
		}
	}
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
	setCreationTimestamp(live, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
	stmt := buildWorkloadsReceipt(t, 1*time.Second, []WorkloadConvergedObservedObject{
		{Desired: desiredDeployment("web"), Live: live},
	})
	if stmt.Predicate.Verdict != VerdictBLOCK {
		t.Fatalf("verdict = %s, want BLOCK", stmt.Predicate.Verdict)
	}
}

func TestWorkloadsConverged_WATCHWhenProgressIsFreshOnOldDeployment(t *testing.T) {
	live := liveDeployment("web", 0)
	setCreationTimestamp(live, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
	setProgressConditionTime(live, "lastUpdateTime", workloadsObservedAt.Add(-30*time.Second))

	stmt := buildWorkloadsReceipt(t, time.Minute, []WorkloadConvergedObservedObject{
		{Desired: desiredDeployment("web"), Live: live},
	})
	if stmt.Predicate.Verdict != VerdictWATCH {
		t.Fatalf("verdict = %s, want WATCH", stmt.Predicate.Verdict)
	}
	w := stmt.Predicate.Evidence.Workloads.Workloads[0]
	if w.ProgressClockSource != "status.conditions[Progressing].lastUpdateTime" {
		t.Fatalf("progressClockSource = %q", w.ProgressClockSource)
	}
	if w.ProgressAgeSeconds <= 0 || w.ProgressAgeSeconds > 60 {
		t.Fatalf("progressAgeSeconds = %d, want fresh progress within grace", w.ProgressAgeSeconds)
	}
}

func TestWorkloadsConverged_UsesFractionalProgressTimestamp(t *testing.T) {
	live := liveDeployment("web", 0)
	setCreationTimestamp(live, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
	setProgressConditionTime(live, "lastUpdateTime", workloadsObservedAt.Add(-30*time.Second).Add(250*time.Millisecond))

	stmt := buildWorkloadsReceipt(t, time.Minute, []WorkloadConvergedObservedObject{
		{Desired: desiredDeployment("web"), Live: live},
	})
	if stmt.Predicate.Verdict != VerdictWATCH {
		t.Fatalf("verdict = %s, want WATCH", stmt.Predicate.Verdict)
	}
	w := stmt.Predicate.Evidence.Workloads.Workloads[0]
	if w.ProgressClockSource != "status.conditions[Progressing].lastUpdateTime" {
		t.Fatalf("progressClockSource = %q", w.ProgressClockSource)
	}
	if w.ProgressAgeSeconds != 29 {
		t.Fatalf("progressAgeSeconds = %d, want fractional timestamp truncated to 29 seconds", w.ProgressAgeSeconds)
	}
}

func TestWorkloadsConverged_WATCHWhenObservedGenerationLagsOldObject(t *testing.T) {
	live := liveDeployment("web", 1)
	setCreationTimestamp(live, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
	_ = unstructured.SetNestedField(live.Object, int64(2), "metadata", "generation")
	_ = unstructured.SetNestedField(live.Object, int64(1), "status", "observedGeneration")

	stmt := buildWorkloadsReceipt(t, time.Second, []WorkloadConvergedObservedObject{
		{Desired: desiredDeployment("web"), Live: live},
	})
	if stmt.Predicate.Verdict != VerdictWATCH {
		t.Fatalf("verdict = %s, want WATCH", stmt.Predicate.Verdict)
	}
	w := stmt.Predicate.Evidence.Workloads.Workloads[0]
	if w.Generation != 2 || w.ObservedGeneration != 1 {
		t.Fatalf("generation evidence = %d/%d, want 2/1", w.Generation, w.ObservedGeneration)
	}
	if w.ProgressClockSource != "status.observedGeneration<metadata.generation" {
		t.Fatalf("progressClockSource = %q", w.ProgressClockSource)
	}
	if w.ProgressAgeSeconds != 0 {
		t.Fatalf("progressAgeSeconds = %d, want omitted/zero for stale status", w.ProgressAgeSeconds)
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
