// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"fmt"
	"sort"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
)

// Workload convergence per-object status values.
const (
	WorkloadConvergedConverged    = "converged"
	WorkloadConvergedProgressing  = "progressing"
	WorkloadConvergedFailed       = "failed"
	WorkloadConvergedMissing      = "missing"
	WorkloadConvergedInconclusive = "inconclusive"
)

// WorkloadsConvergedEvidence is attached under predicate.evidence.workloads
// for workloads-converged receipts. Where object-set-matches proves presence
// + authored-field match and deliberately strips status, this predicate
// reads status to prove the desired workloads actually became usable —
// closing the "receipt PASS while the pod is in CreateContainerConfigError"
// false green (helm-expt finding F3, confighub/cub-scout#476).
type WorkloadsConvergedEvidence struct {
	DesiredSource ObjectSetSource                  `json:"desiredSource"`
	Scope         ObjectSetScope                   `json:"scope"`
	GraceWindow   string                           `json:"graceWindow,omitempty"`
	ObservedAt    string                           `json:"observedAt"`
	DesiredDigest string                           `json:"desiredDigest"`
	LiveDigest    string                           `json:"liveDigest"`
	Summary       WorkloadsConvergedSummary        `json:"summary"`
	Workloads     []WorkloadConvergedObjectSummary `json:"workloads"`
}

type WorkloadsConvergedSummary struct {
	Desired      int `json:"desired"`
	Converged    int `json:"converged"`
	Progressing  int `json:"progressing"`
	Failed       int `json:"failed"`
	Missing      int `json:"missing"`
	Inconclusive int `json:"inconclusive"`
}

type WorkloadConvergedObjectSummary struct {
	ID                  ObjectSetObjectID   `json:"id"`
	Status              string              `json:"status"`
	KstatusStatus       string              `json:"kstatusStatus,omitempty"`
	KstatusMessage      string              `json:"kstatusMessage,omitempty"`
	Generation          int64               `json:"generation,omitempty"`
	ObservedGeneration  int64               `json:"observedGeneration,omitempty"`
	AgeSeconds          int64               `json:"ageSeconds,omitempty"`
	ProgressAgeSeconds  int64               `json:"progressAgeSeconds,omitempty"`
	ProgressClockSource string              `json:"progressClockSource,omitempty"`
	PodReasons          []WorkloadPodReason `json:"podReasons,omitempty"`
	Error               string              `json:"error,omitempty"`
}

// WorkloadPodReason is a pod-level failure reason surfaced for a not-ready
// workload, classified by the shared podWaitingFailure helper.
type WorkloadPodReason struct {
	Pod       string `json:"pod"`
	Container string `json:"container,omitempty"`
	Reason    string `json:"reason"`
	Message   string `json:"message,omitempty"`
}

// WorkloadConvergedObservedObject is one desired workload plus the live
// object observed for that identity and (optionally) the pods that belong to
// it. Error is interpreted as missing unless Inconclusive is true, in which
// case the object could not be classified.
type WorkloadConvergedObservedObject struct {
	Desired      *unstructured.Unstructured
	Live         *unstructured.Unstructured
	Pods         []*unstructured.Unstructured
	Error        string
	Inconclusive bool
}

// BuildWorkloadsConvergedEvidence classifies each desired workload's live
// readiness using kstatus, enriches not-ready workloads with pod-level
// failure reasons, and applies the grace window to distinguish "progressing"
// (WATCH) from "failed" (BLOCK). It is pure: callers do the file and cluster
// reads before invoking.
func BuildWorkloadsConvergedEvidence(source ObjectSetSource, scope ObjectSetScope, graceWindow time.Duration, observed []WorkloadConvergedObservedObject, observedAt time.Time) (WorkloadsConvergedEvidence, error) {
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}

	summaries := make([]WorkloadConvergedObjectSummary, 0, len(observed))
	desiredSet := make([]interface{}, 0, len(observed))
	liveSet := make([]interface{}, 0, len(observed))
	seen := map[string]struct{}{}

	for _, obs := range observed {
		if obs.Desired == nil {
			continue
		}
		id := NewObjectSetObjectID(obs.Desired)
		if _, dup := seen[id.Key()]; dup {
			return WorkloadsConvergedEvidence{}, fmt.Errorf("duplicate desired object identity %s", id.String())
		}
		seen[id.Key()] = struct{}{}

		desiredSet = append(desiredSet, objectSetDigestEntry(id, ComparableObject(obs.Desired)))

		summary := WorkloadConvergedObjectSummary{ID: id}
		switch {
		case obs.Inconclusive:
			summary.Status = WorkloadConvergedInconclusive
			summary.Error = obs.Error
		case obs.Live == nil:
			summary.Status = WorkloadConvergedMissing
			if obs.Error != "" {
				summary.Error = obs.Error
			} else {
				summary.Error = "live object not found"
			}
		default:
			st, msg := WorkloadConvergence(obs.Live)
			summary.KstatusStatus = st.String()
			summary.KstatusMessage = msg
			if a, ok := unstructuredAge(obs.Live, observedAt); ok {
				summary.AgeSeconds = int64(a.Seconds())
			}
			progressAge, progressAgeKnown, progressSource, generation, observedGeneration := workloadProgressClock(obs.Live, observedAt)
			if generation > 0 {
				summary.Generation = generation
			}
			if observedGeneration > 0 {
				summary.ObservedGeneration = observedGeneration
			}
			if progressSource != "" {
				summary.ProgressClockSource = progressSource
			}
			if progressAgeKnown {
				summary.ProgressAgeSeconds = int64(progressAge.Seconds())
			}
			for _, pod := range obs.Pods {
				if _, container, _, reason, message, ok := podWaitingFailure(pod); ok {
					summary.PodReasons = append(summary.PodReasons, WorkloadPodReason{
						Pod:       pod.GetName(),
						Container: container,
						Reason:    reason,
						Message:   clampReason(message, 160),
					})
				}
			}
			summary.Status = classifyConvergence(st, summary.PodReasons, progressAge, progressAgeKnown, graceWindow)
		}

		summaries = append(summaries, summary)
		liveSet = append(liveSet, objectSetDigestEntry(id, map[string]interface{}{
			"status":  summary.Status,
			"kstatus": summary.KstatusStatus,
		}))
	}

	sort.Slice(summaries, func(i, j int) bool { return summaries[i].ID.Key() < summaries[j].ID.Key() })
	sort.Slice(desiredSet, func(i, j int) bool { return digestEntryKey(desiredSet[i]) < digestEntryKey(desiredSet[j]) })
	sort.Slice(liveSet, func(i, j int) bool { return digestEntryKey(liveSet[i]) < digestEntryKey(liveSet[j]) })

	desiredDigest, err := digestJSON(desiredSet)
	if err != nil {
		return WorkloadsConvergedEvidence{}, fmt.Errorf("digest desired workload set: %w", err)
	}
	liveDigest, err := digestJSON(liveSet)
	if err != nil {
		return WorkloadsConvergedEvidence{}, fmt.Errorf("digest live workload set: %w", err)
	}

	summary := WorkloadsConvergedSummary{Desired: len(summaries)}
	for _, w := range summaries {
		switch w.Status {
		case WorkloadConvergedConverged:
			summary.Converged++
		case WorkloadConvergedProgressing:
			summary.Progressing++
		case WorkloadConvergedFailed:
			summary.Failed++
		case WorkloadConvergedMissing:
			summary.Missing++
		case WorkloadConvergedInconclusive:
			summary.Inconclusive++
		}
	}
	source.ObjectCount = summary.Desired

	graceStr := ""
	if graceWindow > 0 {
		graceStr = graceWindow.String()
	}

	return WorkloadsConvergedEvidence{
		DesiredSource: source,
		Scope:         scope,
		GraceWindow:   graceStr,
		ObservedAt:    observedAt.UTC().Format(time.RFC3339),
		DesiredDigest: desiredDigest,
		LiveDigest:    liveDigest,
		Summary:       summary,
		Workloads:     summaries,
	}, nil
}

// classifyConvergence maps a kstatus result (plus pod reasons, progress age,
// and the grace window) to a per-workload convergence status.
//
//   - A hard pod failure reason (CrashLoopBackOff / ImagePullBackOff /
//     CreateContainerConfigError, etc.) will not resolve by waiting, so it
//     is failed regardless of grace.
//   - kstatus Current -> converged; Failed/Terminating -> failed;
//     NotFound -> missing; Unknown -> inconclusive.
//   - kstatus InProgress is progressing while within the grace window, and
//     failed once the best available progress clock passes the window (a
//     stuck rollout). If no reliable progress clock is available, it remains
//     progressing rather than fabricating a timeout.
func classifyConvergence(st status.Status, podReasons []WorkloadPodReason, progressAge time.Duration, progressAgeKnown bool, grace time.Duration) string {
	if len(podReasons) > 0 {
		return WorkloadConvergedFailed
	}
	switch st {
	case status.CurrentStatus:
		return WorkloadConvergedConverged
	case status.FailedStatus, status.TerminatingStatus:
		return WorkloadConvergedFailed
	case status.NotFoundStatus:
		return WorkloadConvergedMissing
	case status.InProgressStatus:
		if grace > 0 && progressAgeKnown && progressAge > grace {
			return WorkloadConvergedFailed
		}
		return WorkloadConvergedProgressing
	default:
		return WorkloadConvergedInconclusive
	}
}

// BuildWorkloadsConvergedReceiptInput is the input to
// BuildWorkloadsConvergedReceipt.
type BuildWorkloadsConvergedReceiptInput struct {
	Evidence          WorkloadsConvergedEvidence
	Verifier          Verifier
	VerifiedAt        time.Time
	InputAttestations []VerifiedAttestationRef
}

// BuildWorkloadsConvergedReceipt builds a single receipt asserting that the
// desired workloads reached a ready/converged state in the live cluster. It
// is pure: callers do file and cluster reads before invoking.
//
// Verdict (severity order BLOCK > INCONCLUSIVE > WATCH > PASS):
//   - BLOCK        any workload failed or missing
//   - INCONCLUSIVE else any workload could not be classified
//   - WATCH        else any workload still progressing within grace
//   - PASS         every desired workload converged
func BuildWorkloadsConvergedReceipt(in BuildWorkloadsConvergedReceiptInput) (Statement, error) {
	if in.Evidence.Summary.Desired == 0 {
		return Statement{}, fmt.Errorf("workloads-converged receipt: no desired workloads")
	}
	if in.Evidence.DesiredDigest == "" {
		return Statement{}, fmt.Errorf("workloads-converged receipt: missing desired digest")
	}
	if in.Evidence.LiveDigest == "" {
		return Statement{}, fmt.Errorf("workloads-converged receipt: missing live digest")
	}

	verifiedAt := in.VerifiedAt
	if verifiedAt.IsZero() {
		verifiedAt = time.Now().UTC()
	}

	s := in.Evidence.Summary
	decisionSet := BuildRolloutDecisionSetFromWorkloadsConvergedEvidence(in.Evidence)
	verdict := decisionSet.Verdict

	omissions := []Omission{
		{
			Missing:  OmissionWorkloadConvergence,
			Reason:   "workloads-converged reflects readiness observed at verifiedAt; it does not prove the workloads stay converged afterward",
			Severity: "info",
		},
	}
	if s.Inconclusive > 0 {
		omissions = append(omissions, Omission{
			Missing:  OmissionWorkloadConvergenceCoverage,
			Reason:   fmt.Sprintf("%d desired workload(s) could not be classified because live lookup or API mapping was inconclusive", s.Inconclusive),
			Severity: "warning",
		})
	}

	nextSteps := []ReceiptNextStep{}
	switch verdict {
	case VerdictPASS:
		nextSteps = append(nextSteps, ReceiptNextStep{
			ActionType:  "read-only",
			Reason:      "every desired workload reached a ready/converged state",
			NextCommand: "cub-scout doctor",
			NextSurface: "cub-scout",
		})
	case VerdictWATCH:
		nextSteps = append(nextSteps, ReceiptNextStep{
			ActionType:  "waiting",
			Reason:      "one or more workloads are still rolling out within the grace window; re-verify after they settle",
			NextCommand: "cub-scout map status",
			NextSurface: "cub-scout",
		})
	case VerdictBLOCK:
		nextSteps = append(nextSteps, ReceiptNextStep{
			ActionType:  "read-only",
			Reason:      "one or more desired workloads failed to converge (not ready past grace, missing, or wedged pods); inspect before accepting the install",
			NextCommand: "cub-scout doctor",
			NextSurface: "cub-scout",
		})
	case VerdictINCONCLUSIVE:
		nextSteps = append(nextSteps, ReceiptNextStep{
			ActionType:  "read-only",
			Reason:      "one or more workloads could not be classified; verify API availability and object identity",
			NextCommand: "cub-scout map list",
			NextSurface: "cub-scout",
		})
	}
	filteredSteps, stepOmissions := FilterNextSteps(nextSteps)
	omissions = append(omissions, stepOmissions...)

	inputAttestations := make([]AttestationRef, 0, len(in.InputAttestations))
	for i, v := range in.InputAttestations {
		if v.IsZero() {
			return Statement{}, fmt.Errorf("workloads-converged receipt: inputAttestations[%d] is a zero-value VerifiedAttestationRef; construct via BuildAttestationRef or BuildAttestationRefsFromPaths", i)
		}
		inputAttestations = append(inputAttestations, v.Ref())
	}

	claim := "desired workloads converged to a ready state in the live cluster"
	if in.Evidence.DesiredSource.Ref != "" {
		claim = fmt.Sprintf("desired workloads from %s converged to a ready state in the live cluster", in.Evidence.DesiredSource.Ref)
	}
	scope := Scope{
		Kind:      "WorkloadSet",
		Name:      "rendered-install",
		Namespace: in.Evidence.Scope.Namespace,
	}

	evidence := in.Evidence
	stmt := Statement{
		Type: StatementType,
		Subject: []Subject{
			BuildRenderedObjectSetSubject(in.Evidence.DesiredDigest),
			BuildLiveWorkloadsSubject(in.Evidence.Scope, in.Evidence.LiveDigest),
		},
		PredicateType: PredicateTypeReceiptV1,
		Predicate: Predicate{
			Version:           PredicateVersion,
			Claim:             claim,
			Scope:             scope,
			Verifier:          in.Verifier,
			VerifiedAt:        verifiedAt.UTC().Format(time.RFC3339),
			PredicateName:     string(PredicateWorkloadsConverged),
			Verdict:           verdict,
			Evidence:          Evidence{Workloads: &evidence},
			Omissions:         omissions,
			InputAttestations: inputAttestations,
			NextSteps:         filteredSteps,
		},
	}

	if err := StampFingerprint(&stmt); err != nil {
		return Statement{}, fmt.Errorf("workloads-converged receipt: stamp fingerprint: %w", err)
	}
	return stmt, nil
}

func workloadProgressClock(u *unstructured.Unstructured, now time.Time) (time.Duration, bool, string, int64, int64) {
	if u == nil {
		return 0, false, "", 0, 0
	}
	generation := u.GetGeneration()
	observedGeneration, _, _ := unstructured.NestedInt64(u.Object, "status", "observedGeneration")
	if generation > 0 && observedGeneration > 0 && observedGeneration < generation {
		return 0, false, "status.observedGeneration<metadata.generation", generation, observedGeneration
	}
	if ts, source, ok := workloadProgressTimestamp(u); ok {
		age := now.Sub(ts)
		if age < 0 {
			age = 0
		}
		return age, true, source, generation, observedGeneration
	}
	if age, ok := unstructuredAge(u, now); ok {
		return age, true, "metadata.creationTimestamp", generation, observedGeneration
	}
	return 0, false, "", generation, observedGeneration
}

func workloadProgressTimestamp(u *unstructured.Unstructured) (time.Time, string, bool) {
	conditions, found, err := unstructured.NestedSlice(u.Object, "status", "conditions")
	if err != nil || !found {
		return time.Time{}, "", false
	}

	if ts, source, ok := conditionTimestampByType(conditions, "Progressing"); ok {
		return ts, source, true
	}

	var newest time.Time
	var newestSource string
	for _, raw := range conditions {
		cond, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		ts, field, ok := conditionTimestamp(cond)
		if !ok || (!newest.IsZero() && !ts.After(newest)) {
			continue
		}
		newest = ts
		typ, _ := cond["type"].(string)
		if typ == "" {
			newestSource = "status.conditions." + field
		} else {
			newestSource = "status.conditions[" + typ + "]." + field
		}
	}
	if newest.IsZero() {
		return time.Time{}, "", false
	}
	return newest, newestSource, true
}

func conditionTimestampByType(conditions []interface{}, wantType string) (time.Time, string, bool) {
	for _, raw := range conditions {
		cond, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		typ, _ := cond["type"].(string)
		if typ != wantType {
			continue
		}
		ts, field, ok := conditionTimestamp(cond)
		if !ok {
			return time.Time{}, "", false
		}
		return ts, "status.conditions[" + wantType + "]." + field, true
	}
	return time.Time{}, "", false
}

func conditionTimestamp(cond map[string]interface{}) (time.Time, string, bool) {
	for _, field := range []string{"lastUpdateTime", "lastTransitionTime"} {
		raw, ok := cond[field].(string)
		if !ok || raw == "" {
			continue
		}
		ts, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			continue
		}
		return ts, field, true
	}
	return time.Time{}, "", false
}

// BuildLiveWorkloadsSubject builds the live-workloads subject for a
// workloads-converged receipt.
func BuildLiveWorkloadsSubject(scope ObjectSetScope, digest string) Subject {
	name := SubjectSchemeK8sLiveWorkloads + "cluster"
	if scope.Namespace != "" {
		name = SubjectSchemeK8sLiveWorkloads + "namespace/" + scope.Namespace
	}
	return Subject{
		Name:   name,
		Digest: map[string]string{"sha256": digest},
	}
}

func unstructuredAge(u *unstructured.Unstructured, now time.Time) (time.Duration, bool) {
	if u == nil {
		return 0, false
	}
	created := u.GetCreationTimestamp().Time
	if created.IsZero() {
		return 0, false
	}
	age := now.Sub(created)
	if age < 0 {
		return 0, true
	}
	return age, true
}

func clampReason(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
