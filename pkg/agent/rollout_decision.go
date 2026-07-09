// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
)

const (
	RolloutProgressPending    = "pending"
	RolloutProgressApplied    = "applied"
	RolloutProgressRollingOut = "rolling_out"
	RolloutProgressStalled    = "stalled"
	RolloutProgressComplete   = "complete"
	RolloutProgressUnknown    = "unknown"
)

const (
	RolloutReasonConverged       = "workload_converged"
	RolloutReasonProgressing     = "rollout_progressing"
	RolloutReasonStaleGeneration = "stale_generation"
	RolloutReasonProgressStalled = "progress_stalled"
	RolloutReasonRuntimeFailed   = "runtime_failed"
	RolloutReasonRolloutFailed   = "rollout_failed"
	RolloutReasonMissing         = "workload_missing"
	RolloutReasonEvidenceMissing = "evidence_missing"
)

// RolloutDecision is the shared current-change decision model used by
// diagnostic surfaces and receipt predicates. It is evidence-shaped: callers
// can render the concise verdict while reviewers can inspect the raw status
// and generation signals that led to it.
type RolloutDecision struct {
	Resource  ObjectSetObjectID       `json:"resource"`
	Progress  RolloutProgress         `json:"progress"`
	Verdict   ReceiptVerdict          `json:"verdict"`
	Reason    string                  `json:"reason"`
	Message   string                  `json:"message,omitempty"`
	Evidence  RolloutDecisionEvidence `json:"evidence"`
	Omissions []Omission              `json:"omissions,omitempty"`
}

type RolloutProgress struct {
	Phase              string `json:"phase"`
	ClockSource        string `json:"clockSource,omitempty"`
	ProgressAgeSeconds int64  `json:"progressAgeSeconds,omitempty"`
}

type RolloutDecisionEvidence struct {
	KstatusStatus      string              `json:"kstatusStatus,omitempty"`
	KstatusMessage     string              `json:"kstatusMessage,omitempty"`
	Generation         int64               `json:"generation,omitempty"`
	ObservedGeneration int64               `json:"observedGeneration,omitempty"`
	PodReasons         []WorkloadPodReason `json:"podReasons,omitempty"`
	ObservedAt         string              `json:"observedAt,omitempty"`
}

type RolloutDecisionSet struct {
	Verdict   ReceiptVerdict            `json:"verdict"`
	Reason    string                    `json:"reason"`
	Summary   WorkloadsConvergedSummary `json:"summary"`
	Decisions []RolloutDecision         `json:"decisions"`
}

// BuildRolloutDecisionForWorkload builds a decision for one live workload.
// It is pure and read-only: callers provide the live object and any related
// pods they already fetched.
func BuildRolloutDecisionForWorkload(live *unstructured.Unstructured, pods []*unstructured.Unstructured, graceWindow time.Duration, observedAt time.Time) (RolloutDecision, bool) {
	if live == nil || !IsRolloutWorkloadKind(live.GetKind()) {
		return RolloutDecision{}, false
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}

	st, msg := WorkloadConvergence(live)
	progressAge, progressAgeKnown, progressSource, generation, observedGeneration := workloadProgressClock(live, observedAt)
	podReasons := make([]WorkloadPodReason, 0)
	for _, pod := range pods {
		if _, container, _, reason, message, ok := podWaitingFailure(pod); ok {
			podReasons = append(podReasons, WorkloadPodReason{
				Pod:       pod.GetName(),
				Container: container,
				Reason:    reason,
				Message:   clampReason(message, 160),
			})
		}
	}

	summary := WorkloadConvergedObjectSummary{
		ID:                  NewObjectSetObjectID(live),
		Status:              classifyConvergence(st, podReasons, progressAge, progressAgeKnown, graceWindow),
		KstatusStatus:       st.String(),
		KstatusMessage:      msg,
		Generation:          generation,
		ObservedGeneration:  observedGeneration,
		ProgressClockSource: progressSource,
		PodReasons:          podReasons,
	}
	if progressAgeKnown {
		summary.ProgressAgeSeconds = int64(progressAge.Seconds())
	}
	return RolloutDecisionFromWorkloadSummary(summary, observedAt), true
}

func IsRolloutWorkloadKind(kind string) bool {
	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet", "Job", "Pod":
		return true
	default:
		return false
	}
}

func RolloutDecisionFromWorkloadSummary(summary WorkloadConvergedObjectSummary, observedAt time.Time) RolloutDecision {
	decision := RolloutDecision{
		Resource: summary.ID,
		Progress: RolloutProgress{
			Phase:              RolloutProgressUnknown,
			ClockSource:        summary.ProgressClockSource,
			ProgressAgeSeconds: summary.ProgressAgeSeconds,
		},
		Verdict: VerdictINCONCLUSIVE,
		Reason:  RolloutReasonEvidenceMissing,
		Message: summary.KstatusMessage,
		Evidence: RolloutDecisionEvidence{
			KstatusStatus:      summary.KstatusStatus,
			KstatusMessage:     summary.KstatusMessage,
			Generation:         summary.Generation,
			ObservedGeneration: summary.ObservedGeneration,
			PodReasons:         summary.PodReasons,
		},
	}
	if !observedAt.IsZero() {
		decision.Evidence.ObservedAt = observedAt.UTC().Format(time.RFC3339)
	}

	switch summary.Status {
	case WorkloadConvergedConverged:
		decision.Progress.Phase = RolloutProgressComplete
		decision.Verdict = VerdictPASS
		decision.Reason = RolloutReasonConverged
	case WorkloadConvergedProgressing:
		decision.Verdict = VerdictWATCH
		if summary.ProgressClockSource == "status.observedGeneration<metadata.generation" {
			decision.Progress.Phase = RolloutProgressApplied
			decision.Reason = RolloutReasonStaleGeneration
		} else {
			decision.Progress.Phase = RolloutProgressRollingOut
			decision.Reason = RolloutReasonProgressing
		}
	case WorkloadConvergedFailed:
		decision.Progress.Phase = RolloutProgressStalled
		decision.Verdict = VerdictBLOCK
		decision.Reason = rolloutFailureReason(summary)
	case WorkloadConvergedMissing:
		decision.Progress.Phase = RolloutProgressPending
		decision.Verdict = VerdictBLOCK
		decision.Reason = RolloutReasonMissing
		if summary.Error != "" {
			decision.Message = summary.Error
		}
	case WorkloadConvergedInconclusive:
		decision.Progress.Phase = RolloutProgressUnknown
		decision.Verdict = VerdictINCONCLUSIVE
		decision.Reason = RolloutReasonEvidenceMissing
		if summary.Error != "" {
			decision.Message = summary.Error
		}
		decision.Omissions = append(decision.Omissions, Omission{
			Missing:  OmissionWorkloadConvergenceCoverage,
			Reason:   "workload rollout evidence could not be classified",
			Severity: "warning",
		})
	}

	return decision
}

func rolloutFailureReason(summary WorkloadConvergedObjectSummary) string {
	if len(summary.PodReasons) > 0 {
		return RolloutReasonRuntimeFailed
	}
	switch status.Status(summary.KstatusStatus) {
	case status.FailedStatus, status.TerminatingStatus:
		return RolloutReasonRolloutFailed
	default:
		return RolloutReasonProgressStalled
	}
}

func BuildRolloutDecisionSetFromWorkloadsConvergedEvidence(evidence WorkloadsConvergedEvidence) RolloutDecisionSet {
	observedAt, _ := time.Parse(time.RFC3339, evidence.ObservedAt)
	decisions := make([]RolloutDecision, 0, len(evidence.Workloads))
	for _, workload := range evidence.Workloads {
		decisions = append(decisions, RolloutDecisionFromWorkloadSummary(workload, observedAt))
	}
	verdict := WorkloadsConvergedVerdict(evidence.Summary)
	return RolloutDecisionSet{
		Verdict:   verdict,
		Reason:    rolloutSetReason(verdict, decisions),
		Summary:   evidence.Summary,
		Decisions: decisions,
	}
}

func WorkloadsConvergedVerdict(summary WorkloadsConvergedSummary) ReceiptVerdict {
	switch {
	case summary.Failed > 0 || summary.Missing > 0:
		return VerdictBLOCK
	case summary.Inconclusive > 0:
		return VerdictINCONCLUSIVE
	case summary.Progressing > 0:
		return VerdictWATCH
	default:
		return VerdictPASS
	}
}

func rolloutSetReason(verdict ReceiptVerdict, decisions []RolloutDecision) string {
	if len(decisions) == 0 {
		return RolloutReasonEvidenceMissing
	}
	for _, decision := range decisions {
		if decision.Verdict == verdict {
			return decision.Reason
		}
	}
	return RolloutReasonEvidenceMissing
}
