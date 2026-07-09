// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestBuildRolloutDecisionForWorkload_PASSWhenCurrent(t *testing.T) {
	decision, ok := BuildRolloutDecisionForWorkload(liveDeployment("web", 1), nil, 0, workloadsObservedAt)
	if !ok {
		t.Fatal("decision not built")
	}
	if decision.Verdict != VerdictPASS {
		t.Fatalf("verdict = %s, want PASS", decision.Verdict)
	}
	if decision.Progress.Phase != RolloutProgressComplete {
		t.Fatalf("phase = %s, want complete", decision.Progress.Phase)
	}
	if decision.Reason != RolloutReasonConverged {
		t.Fatalf("reason = %s, want workload_converged", decision.Reason)
	}
}

func TestBuildRolloutDecisionForWorkload_WATCHWhenGenerationStale(t *testing.T) {
	live := liveDeployment("web", 0)
	live.SetGeneration(2)

	decision, ok := BuildRolloutDecisionForWorkload(live, nil, time.Minute, workloadsObservedAt)
	if !ok {
		t.Fatal("decision not built")
	}
	if decision.Verdict != VerdictWATCH {
		t.Fatalf("verdict = %s, want WATCH", decision.Verdict)
	}
	if decision.Progress.Phase != RolloutProgressApplied {
		t.Fatalf("phase = %s, want applied", decision.Progress.Phase)
	}
	if decision.Reason != RolloutReasonStaleGeneration {
		t.Fatalf("reason = %s, want stale_generation", decision.Reason)
	}
	if decision.Evidence.Generation != 2 || decision.Evidence.ObservedGeneration != 1 {
		t.Fatalf("generation evidence = %d/%d, want 2/1", decision.Evidence.Generation, decision.Evidence.ObservedGeneration)
	}
}

func TestBuildRolloutDecisionForWorkload_BLOCKWhenPodRuntimeFailed(t *testing.T) {
	live := liveDeployment("web", 0)
	decision, ok := BuildRolloutDecisionForWorkload(live, []*unstructured.Unstructured{
		waitingPod("web-abc", "CreateContainerConfigError"),
	}, 0, workloadsObservedAt)
	if !ok {
		t.Fatal("decision not built")
	}
	if decision.Verdict != VerdictBLOCK {
		t.Fatalf("verdict = %s, want BLOCK", decision.Verdict)
	}
	if decision.Reason != RolloutReasonRuntimeFailed {
		t.Fatalf("reason = %s, want runtime_failed", decision.Reason)
	}
	if len(decision.Evidence.PodReasons) != 1 {
		t.Fatalf("pod reasons = %d, want 1", len(decision.Evidence.PodReasons))
	}
}

func TestBuildRolloutDecisionForWorkload_BLOCKWhenPastGrace(t *testing.T) {
	live := liveDeployment("web", 0)
	setCreationTimestamp(live, workloadsObservedAt.Add(-10*time.Minute))

	decision, ok := BuildRolloutDecisionForWorkload(live, nil, time.Minute, workloadsObservedAt)
	if !ok {
		t.Fatal("decision not built")
	}
	if decision.Verdict != VerdictBLOCK {
		t.Fatalf("verdict = %s, want BLOCK", decision.Verdict)
	}
	if decision.Progress.Phase != RolloutProgressStalled {
		t.Fatalf("phase = %s, want stalled", decision.Progress.Phase)
	}
	if decision.Reason != RolloutReasonProgressStalled {
		t.Fatalf("reason = %s, want progress_stalled", decision.Reason)
	}
}

func TestBuildRolloutDecisionSetFromWorkloadsConvergedEvidence_UsesMaxSeverityVerdict(t *testing.T) {
	evidence, err := BuildWorkloadsConvergedEvidence(
		ObjectSetSource{Type: "file", Ref: "rendered.yaml"},
		ObjectSetScope{Kind: "namespace", Namespace: "prod"},
		0,
		[]WorkloadConvergedObservedObject{
			{Desired: desiredDeployment("ready"), Live: liveDeployment("ready", 1)},
			{Desired: desiredDeployment("failed"), Live: liveDeployment("failed", 0), Pods: []*unstructured.Unstructured{
				waitingPod("failed-abc", "CrashLoopBackOff"),
			}},
		},
		workloadsObservedAt,
	)
	if err != nil {
		t.Fatalf("BuildWorkloadsConvergedEvidence: %v", err)
	}

	decisionSet := BuildRolloutDecisionSetFromWorkloadsConvergedEvidence(evidence)
	if decisionSet.Verdict != VerdictBLOCK {
		t.Fatalf("verdict = %s, want BLOCK", decisionSet.Verdict)
	}
	if decisionSet.Reason != RolloutReasonRuntimeFailed {
		t.Fatalf("reason = %s, want runtime_failed", decisionSet.Reason)
	}
	if len(decisionSet.Decisions) != 2 {
		t.Fatalf("decisions = %d, want 2", len(decisionSet.Decisions))
	}
}
